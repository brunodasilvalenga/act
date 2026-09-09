# Plan 043: Reject `act ec2 ssh --push-key` against Windows targets before sending an SSM command

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/ssh_key_push.go internal/aws/ec2.go internal/aws/ssm.go cmd_ec2.go`
> If any of these files changed since this plan was written, re-read them in
> full before proceeding — do not trust the line numbers below. In
> particular, `plans/037-fix-push-key-dedup-check.md` (written in parallel by
> a different agent) also touches `internal/aws/ssh_key_push.go` — if that
> plan has already landed, the shell-script/marker/dedup lines inside
> `PushSSHKeyViaSSM` may differ from the excerpt below. That is fine as long
> as the function's *signature* and the top of its body are where you
> expected; if the whole function has been restructured beyond recognition,
> treat it as a STOP condition and report rather than guessing.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (soft overlap with plans/037-fix-push-key-dedup-check.md — see Maintenance notes; no hard ordering dependency, different region of the same function)
- **Category**: bug
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

`act ec2 ssh --push-key` (implemented by `PushSSHKeyViaSSM` in
`internal/aws/ssh_key_push.go`) unconditionally sends its SSM Run Command as
`AWS-RunShellScript` — a POSIX shell script using `getent`, `chown`, and
`.ssh/authorized_keys` — regardless of what platform the target instance
actually runs. Nothing in the call chain (`runSSH` in `cmd_ec2.go`) checks
the target's platform before calling it. If a user runs
`act ec2 ssh --push-key --target <windows-instance-id>`, or picks a Windows
instance from the interactive picker (the picker used by `ec2 ssh` is
unfiltered — see "Current state" below), the SSM Agent on that instance will
try to interpret a POSIX shell script and fail with a confusing, low-level
SSM-side error instead of a clear message from `act` explaining that
`--push-key` doesn't apply to Windows targets. This plan adds an
up-front platform check that fails fast with an actionable error, before any
SSM command is sent. It deliberately does **not** attempt to build a real
Windows equivalent of `--push-key` — see Scope below for why.

## Current state

Files involved, each with its role:

- `internal/aws/ssh_key_push.go` — defines `PushSSHKeyViaSSM`, the function
  that builds and sends the POSIX shell script. This is what needs the
  guard.
- `internal/aws/ssm.go` — defines `DocumentForPlatform`, the existing
  convention for picking an SSM document by platform. Reuse its exact
  case-insensitive check.
- `internal/aws/ec2.go` — defines `Instance` (has a `Platform` field) and
  the instance-listing functions that populate it. You will add one small
  new function here for the one case where platform isn't already known.
- `cmd_ec2.go` — defines `runSSH`, the only call site of
  `PushSSHKeyViaSSM`. This is where the target instance ID (and, after this
  plan, its platform) is resolved.
- `internal/aws/ssh_key_push_test.go` — existing unit tests for this
  package; add the new guard's test here, following the existing style.
- `README.md` — line 38 and line 153 document `--push-key`; needs a one-line
  addition noting the Windows restriction (per this repo's CLAUDE.md rule:
  "After adding a new feature, command, or flag, always update README.md").

### `PushSSHKeyViaSSM` today (`internal/aws/ssh_key_push.go:47-94`)

```go
func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath string) (string, error) {
	keyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("reading public key: %w", err)
	}
	key := strings.TrimSpace(string(keyBytes))
	if key == "" {
		return "", fmt.Errorf("public key file %s is empty", publicKeyPath)
	}
	if strings.ContainsAny(key, "\n\r") {
		return "", fmt.Errorf("public key file %s contains more than one line; expected a single public key", publicKeyPath)
	}

	marker := fmt.Sprintf("act-push-key-%d", time.Now().UnixNano())
	keyLine := fmt.Sprintf("%s act-push-key %s", key, marker)

	osUserQ := shellSingleQuote(osUser)
	ownerQ := shellSingleQuote(osUser + ":" + osUser)
	keyLineQ := shellSingleQuote(keyLine)

	script := fmt.Sprintf(`set -e
homedir=$(getent passwd %s | cut -d: -f6)
if [ -z "$homedir" ]; then echo "act-push-key: no such user %s" >&2; exit 1; fi
mkdir -p "$homedir/.ssh"
touch "$homedir/.ssh/authorized_keys"
chmod 700 "$homedir/.ssh"
chmod 600 "$homedir/.ssh/authorized_keys"
chown %s "$homedir/.ssh" "$homedir/.ssh/authorized_keys"
grep -qxF %s "$homedir/.ssh/authorized_keys" || echo %s >> "$homedir/.ssh/authorized_keys"
echo "$homedir/.ssh/authorized_keys"`, osUserQ, osUserQ, ownerQ, keyLineQ, keyLineQ)

	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
	if err != nil {
		return "", fmt.Errorf("sending push-key command: %w", err)
	}
	// ... waits for the command, returns a removal command
```

Note: `PushSSHKeyViaSSM` has exactly one call site, `cmd_ec2.go:66`. The
exact body of the shell-script section (lines 60-76) may have shifted if
`plans/037-fix-push-key-dedup-check.md` has landed since this plan was
written — that plan touches the `grep -qxF` dedup line specifically, not the
function signature or the top of the body. Re-read the live file; the parts
this plan cares about are the function *signature* (line 47) and adding a
check as the very first statement in the function body.

### `DocumentForPlatform`, the existing convention (`internal/aws/ssm.go:11-18`)

```go
// DocumentForPlatform returns the SSM Run Command document for the given
// EC2 Platform value ("windows" for Windows instances, "" or "linux" otherwise).
func DocumentForPlatform(platform string) string {
	if strings.EqualFold(platform, "windows") {
		return "AWS-RunPowerShellScript"
	}
	return "AWS-RunShellScript"
}
```

Its test, the style to follow (`internal/aws/ssm_test.go:5-25`):

```go
func TestDocumentForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "windows lowercase", platform: "windows", want: "AWS-RunPowerShellScript"},
		{name: "windows mixed case", platform: "Windows", want: "AWS-RunPowerShellScript"},
		{name: "empty is linux", platform: "", want: "AWS-RunShellScript"},
		{name: "linux explicit", platform: "linux", want: "AWS-RunShellScript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DocumentForPlatform(tt.platform)
			if got != tt.want {
				t.Errorf("DocumentForPlatform(%q) = %q, want %q", tt.platform, got, tt.want)
			}
		})
	}
}
```

### `Instance` struct and how `Platform` is populated (`internal/aws/ec2.go:10-16, 41-97, 99-160`)

```go
type Instance struct {
	InstanceID   string
	Name         string
	PrivateIP    string
	InstanceType string
	Platform     string
}
```

`ListRunningInstances` (used by `ec2 connect`, `ec2 ssh`'s picker, `ec2 cp`,
`ec2 forward`, `rds` bastion picking — it is **not** filtered to Linux) and
`ListWindowsInstances` (used by `ec2 rdp`'s picker) both populate `Platform`
straight from the AWS CLI's `Platform` field on each `describe-instances`
result (`internal/aws/ec2.go:91` and `:150`: `Platform: inst.Platform,`).
Critically, `ec2 ssh`'s picker uses `ListRunningInstances`, which returns
**both** Linux and Windows instances — so a Windows instance can already be
selected interactively for `ec2 ssh --push-key`, not just via `--target`.

There is currently no function in `internal/aws/ec2.go` to look up a single
instance's platform by ID (needed for the `--target`-given case — see Step
2 below).

### `runSSH`, the one call site (`cmd_ec2.go:28-80`)

```go
func runSSH(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	pushKey := fs.Bool("push-key", false, "Add local SSH public key to the target's authorized_keys via SSM before connecting")
	pushKeyPath := fs.String("push-key-path", "", "Path to public key to push (default: ~/.ssh/id_ed25519.pub or ~/.ssh/id_rsa.pub)")
	fs.Parse(subArgs)

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	sshUser := *user
	if sshUser == "" {
		picked, err := tui.RunPicker("Select SSH user", []string{"ec2-user", "ubuntu", "root", "ssm-user"})
		if err != nil || picked == "" {
			sshUser = "ec2-user"
		} else {
			sshUser = picked
		}
	}

	if *pushKey {
		keyPath := *pushKeyPath
		if keyPath == "" {
			var err error
			keyPath, err = aws.DefaultSSHPublicKeyPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		removeCmd, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via SSM (added to authorized_keys).\n", keyPath, sshUser, instanceID)
		fmt.Fprintf(os.Stderr, "To remove it later: %s\n", removeCmd)
	}

	err := aws.StartSSHSession(instanceID, profile, region, sshUser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting SSH session: %v\n", err)
		os.Exit(1)
	}
}
```

`instanceID` is resolved from either `*target` (no platform info available)
or `pickInstance(loadFunc)`. `pickInstance` (`helpers.go:29-39`) discards
platform — it only returns `selected.InstanceID` (a bare `string`), and it
is shared by 6 call sites across `cmd_ec2.go`, `cmd_rds.go`, and
`cmd_forward.go`, so changing its signature is out of scope (too invasive
for this fix — see Scope).

### The existing sibling convention: `runSSMRun` already threads platform through the picker (`cmd_ssm.go:45-63`)

```go
	instanceID := *target
	var platform string
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		selected, err := tui.Run(loadFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if selected == nil {
			os.Exit(0)
		}
		instanceID = selected.InstanceID
		platform = selected.Platform
	}

	document := aws.DocumentForPlatform(platform)
```

This is exactly the pattern to copy into `runSSH`'s picker branch: call
`tui.Run(loadFunc)` directly instead of `pickInstance(loadFunc)` when you
need `Platform` too. Note that `runSSMRun` leaves `platform` as the zero
value `""` when `--target` is given directly (it never looks the instance
up) — that is an existing, accepted gap in this codebase for `ssm run`, not
something this plan is asked to fix there. For `--push-key` specifically we
*do* close that gap (see Step 2), because the audited finding's own example
is exactly `--push-key --target <windows-instance-id>`.

### The extraction convention to follow for testability (`internal/aws/scp.go:9-18`)

```go
// scpEndpoints computes the local and remote-spec scp arguments for a
// given copy direction.
func scpEndpoints(instanceID, user, source, dest string, download bool) (localArg, remoteSpecArg string, remoteFirst bool) {
	remotePath, localPath := dest, source
	if download {
		remotePath, localPath = source, dest
	}
	remoteSpec := fmt.Sprintf("%s@%s:%s", user, instanceID, remotePath)
	return localPath, remoteSpec, download
}
```

`CopyFile` (which does real network I/O) calls this pure helper and only the
pure helper is unit tested. Follow the same shape: extract a small pure
`rejectIfWindows(platform string) error` function, call it from the top of
`PushSSHKeyViaSSM`, and unit-test only the pure function.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0, no output |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | exit 0, no output (no files listed) |
| Test (aws package) | `go test ./internal/aws/...` | `ok` for `internal/aws`, all tests pass |
| Test (whole repo) | `go test ./...` | `ok` for every package |
| Full gate (matches `make test`) | `make test` | exit 0 (runs fmt, vet, then `go test ./...`) |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ssh_key_push.go` — add `rejectIfWindows`, change
  `PushSSHKeyViaSSM`'s signature to accept `platform string` and call the
  guard first.
- `internal/aws/ssh_key_push_test.go` — add `TestRejectIfWindows`.
- `internal/aws/ec2.go` — add one new function,
  `DescribeInstancePlatform(instanceID, profile, region string) (string, error)`,
  used only for the `--target`-given path (see Step 2).
- `cmd_ec2.go` — update `runSSH` to resolve and thread `platform` through to
  `PushSSHKeyViaSSM`, and to call `DescribeInstancePlatform` in the
  `--target`-given + `--push-key` case.
- `README.md` — one-line addition noting the Windows restriction (see
  Step 5).
- `plans/README.md` — add/update this plan's status row (last step, once
  everything else is verified green).

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ssm.go`'s `DocumentForPlatform` — read-only reference, do
  not modify it; this plan reuses its exact `strings.EqualFold(platform,
  "windows")` convention rather than importing/calling it, because
  `DocumentForPlatform`'s job is "pick a document," while this plan's job is
  "refuse before picking a document at all." Do not refactor
  `PushSSHKeyViaSSM` to call `DocumentForPlatform` and then send a
  PowerShell version of the script — that would require rewriting the
  script for Windows OpenSSH's actual authorized_keys layout and permission
  model, which is a materially larger feature, not this bug fix.
- `helpers.go`'s `pickInstance` — do not change its signature. It is shared
  by 6 call sites (`cmd_ec2.go:19,43,98,146`, `cmd_rds.go:77`,
  `cmd_forward.go:34`); broadening it to return platform for everyone is a
  bigger, riskier change than this fix needs. `runSSH`'s picker branch
  should call `tui.Run(loadFunc)` directly instead (see Step 2), exactly as
  `runSSMRun` already does — it does not need `pickInstance` at all once
  changed.
- `cmd_ssm.go`'s `runSSMRun` — it has the same "platform stays unknown when
  `--target` is given" characteristic mentioned above; do not "fix" it as a
  drive-by. That is a different, pre-existing, already-accepted gap in a
  different command and is not part of this finding.
- The grep/marker dedup logic inside `PushSSHKeyViaSSM`'s shell script
  (currently around `internal/aws/ssh_key_push.go:60-76`, but re-check —
  see the drift-check note at the top) — that is
  `plans/037-fix-push-key-dedup-check.md`'s job, not this plan's. Do not
  touch those lines beyond what's needed to insert the guard before them.
- Any attempt to make `--push-key` work against Windows targets (e.g. a
  PowerShell script that edits `ProgramData\ssh\administrators_authorized_keys`
  or a per-user `authorized_keys`) — explicitly deferred, see Maintenance
  notes.

## Git workflow

- Branch: `advisor/043-guard-push-key-non-linux-target`
- One commit per step is fine, or a single commit for the whole plan —
  match this repo's observed style of one squashed commit per merged plan
  (see `git log --oneline`, e.g. `28c2800 feat: add --push-key to act ec2
  ssh via EC2 Instance Connect`, `c81703b fix: push --push-key via SSM Run
  Command instead of EC2 Instance Connect`). Use a `fix:` prefix (this is a
  bug fix, not a new feature) — e.g. `fix: reject --push-key against
  Windows targets before sending SSM command`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Extract and add the pure Windows-rejection guard

In `internal/aws/ssh_key_push.go`, add a new function near the top of the
file (after `shellSingleQuote`, before `PushSSHKeyViaSSM`):

```go
// rejectIfWindows returns an error if platform indicates a Windows
// instance, using the same case-insensitive convention DocumentForPlatform
// uses in ssm.go. --push-key's shell script is POSIX-specific (getent,
// chown, .ssh/authorized_keys) and has no Windows equivalent, so
// PushSSHKeyViaSSM must refuse before sending any SSM command rather than
// letting AWS-RunShellScript fail confusingly against a Windows instance.
func rejectIfWindows(platform string) error {
	if strings.EqualFold(platform, "windows") {
		return fmt.Errorf("--push-key is not supported for Windows targets (target platform: %s); Windows instances don't use authorized_keys the same way — see 'act ec2 rdp' for Windows access", platform)
	}
	return nil
}
```

Then change `PushSSHKeyViaSSM`'s signature from:

```go
func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath string) (string, error) {
```

to:

```go
func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath, platform string) (string, error) {
	if err := rejectIfWindows(platform); err != nil {
		return "", err
	}
```

(i.e. the guard call becomes the very first statement in the function
body, before the existing `keyBytes, err := os.ReadFile(publicKeyPath)`
line.) Also update the function's doc comment above it to mention the new
parameter, e.g. append a sentence: "platform is the target's EC2 Platform
value (as in Instance.Platform / DocumentForPlatform); Windows targets are
rejected immediately since this function's script is POSIX-only."

**Verify**: `go build ./internal/aws/...` → fails with a compile error
naming `cmd_ec2.go`'s call to `PushSSHKeyViaSSM` (wrong argument count) —
this is expected at this point, since the call site isn't updated yet. Do
not treat this as a problem; proceed to Step 2 and it will resolve. If
instead `internal/aws` itself fails to build (e.g. a syntax error in your
edit), fix that before moving on.

### Step 2: Add a single-instance platform lookup and thread platform through `runSSH`

In `internal/aws/ec2.go`, add this new function after `ListWindowsInstances`
(reuse the existing `describeOutput` struct already defined at the top of
this file — do not redefine it):

```go
// DescribeInstancePlatform looks up the Platform value for a single
// instance by ID. It exists for callers that already have an instance ID
// from elsewhere (e.g. a --target flag) rather than from ListRunningInstances
// or ListWindowsInstances, which already carry Platform on the Instance
// they return — use this only when Platform isn't already known, to avoid
// an extra AWS API call in the common case.
func DescribeInstancePlatform(instanceID, profile, region string) (string, error) {
	args := []string{"ec2", "describe-instances",
		"--instance-ids", instanceID,
		"--output", "json",
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}

	var result describeOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse aws output: %w", err)
	}
	for _, r := range result.Reservations {
		for _, inst := range r.Instances {
			return inst.Platform, nil
		}
	}
	return "", fmt.Errorf("instance %s not found", instanceID)
}
```

In `cmd_ec2.go`, update `runSSH` (currently lines 28-80). Replace the
instance-resolution block:

```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}
```

with a version that also captures platform, mirroring `runSSMRun`'s
existing pattern (`cmd_ssm.go:45-61`) exactly:

```go
	instanceID := *target
	var platform string
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		selected, err := tui.Run(loadFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if selected == nil {
			os.Exit(0)
		}
		instanceID = selected.InstanceID
		platform = selected.Platform
	}
```

This removes the only use of `pickInstance` inside `runSSH` — that's fine,
`pickInstance` is still used by the other 5 call sites listed in Scope and
must remain unchanged. `cmd_ec2.go` already imports `internal/tui` as `tui`
(used a few lines below for `tui.RunPicker`), so no new import is needed.

Then, in the `if *pushKey {` block, add the `--target`-given lookup and pass
`platform` through. Replace:

```go
	if *pushKey {
		keyPath := *pushKeyPath
		if keyPath == "" {
			var err error
			keyPath, err = aws.DefaultSSHPublicKeyPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		removeCmd, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
```

with:

```go
	if *pushKey {
		if *target != "" {
			// The instance was given directly rather than chosen from the
			// picker, so we don't have its Platform yet — look it up now.
			// This only runs for --push-key + --target together, not the
			// common picker path above (which already has Platform for
			// free from ListRunningInstances).
			p, err := aws.DescribeInstancePlatform(instanceID, profile, region)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error checking target platform: %v\n", err)
				os.Exit(1)
			}
			platform = p
		}
		keyPath := *pushKeyPath
		if keyPath == "" {
			var err error
			keyPath, err = aws.DefaultSSHPublicKeyPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		removeCmd, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath, platform)
```

(The rest of the block — the `if err != nil` handling and the two
`fmt.Fprintf` lines after it — stays exactly as-is.)

**Verify**: `go build ./...` → exit 0, no output.

**Verify**: `go vet ./...` → exit 0, no output.

### Step 3: Add unit tests for the new pure guard

In `internal/aws/ssh_key_push_test.go`, add a new test function following
`TestDocumentForPlatform`'s exact table-test shape
(`internal/aws/ssm_test.go:5-25`):

```go
func TestRejectIfWindows(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{name: "windows lowercase", platform: "windows", wantErr: true},
		{name: "windows mixed case", platform: "Windows", wantErr: true},
		{name: "empty is linux", platform: "", wantErr: false},
		{name: "linux explicit", platform: "linux", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectIfWindows(tt.platform)
			if tt.wantErr && err == nil {
				t.Errorf("rejectIfWindows(%q) = nil, want an error", tt.platform)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("rejectIfWindows(%q) = %v, want nil", tt.platform, err)
			}
		})
	}
}
```

Also add one focused test confirming the error message is actionable (not
just non-nil), since that message is user-facing:

```go
func TestRejectIfWindowsErrorMentionsRDP(t *testing.T) {
	err := rejectIfWindows("windows")
	if err == nil {
		t.Fatal("expected an error for windows platform")
	}
	if !strings.Contains(err.Error(), "ec2 rdp") {
		t.Errorf("rejectIfWindows error = %q, want it to mention 'ec2 rdp'", err.Error())
	}
}
```

This second test needs `"strings"` imported in
`internal/aws/ssh_key_push_test.go` — check the existing import block at the
top of that file first; add `"strings"` to it if it isn't already there.

Do **not** attempt to unit test `PushSSHKeyViaSSM` itself (it does real
`aws` CLI network I/O — `SendCommand`/`WaitForCommandInvocation` — and this
package has no exec-mocking harness; note that `ListRunningInstances`,
`ListWindowsInstances`, and now `DescribeInstancePlatform` are likewise
untested for the same reason — this matches the existing pattern in this
file, where only pure helpers like `findSSHPublicKey` and
`shellSingleQuote` have tests).

**Verify**: `go test ./internal/aws/... -run 'TestRejectIfWindows' -v` →
all subtests `PASS`, overall `ok`.

### Step 4: Run the full test/build/vet/fmt gate

**Verify**: `make test` → exit 0. This runs `gofmt -l .` (must print
nothing), `go vet ./...` (must print nothing), then `go test ./...` (every
package reports `ok`).

If `gofmt -l .` lists any file you touched, run `gofmt -w <file>` and
re-verify.

### Step 5: Update README.md

`README.md:38` currently reads (single line, wraps in the file):

```
- For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to add your local public key to the target's `authorized_keys` via SSM Run Command (only needs the SSM Agent — no extra on-instance agent; the key persists until removed, and `--push-key` prints the exact command to remove it); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance
```

Append a Windows caveat to this same bullet, right before the `; for `ec2
cp`` clause, so the line reads:

```
- For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to add your local public key to the target's `authorized_keys` via SSM Run Command (only needs the SSM Agent — no extra on-instance agent; the key persists until removed, and `--push-key` prints the exact command to remove it; not supported for Windows targets — use `ec2 rdp` instead); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance
```

Do not touch the separate, pre-existing "via EC2 Instance Connect" wording
issue on `README.md:153` (`act ec2 ssh --push-key   # push your local
pubkey via EC2 Instance Connect first`) — that line is stale for an
unrelated reason (the implementation moved from EC2 Instance Connect to SSM
in commit `c81703b`, before this plan) and is out of scope here; fixing it
is not part of this finding.

**Verify**: `grep -n "not supported for Windows targets" README.md` →
exactly one match, on the line at/near 38.

## Test plan

- New tests: `TestRejectIfWindows` and
  `TestRejectIfWindowsErrorMentionsRDP` in
  `internal/aws/ssh_key_push_test.go`, covering: Windows lowercase, Windows
  mixed-case, empty-string (must be treated as Linux, matching
  `DocumentForPlatform`'s convention), explicit `"linux"`, and that the
  error message references `ec2 rdp` as the actionable alternative.
- Structural pattern to model after: `TestDocumentForPlatform` in
  `internal/aws/ssm_test.go:5-25`.
- No new test for `DescribeInstancePlatform` or the `runSSH` wiring itself —
  both require real `aws` CLI I/O / a live picker session; this matches the
  existing untested state of `ListRunningInstances`/`ListWindowsInstances`
  and `runSSH`'s other logic in this codebase.
- Verification: `go test ./...` → all packages `ok`, including the two new
  test functions passing under `internal/aws`.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` exits 0, no output
- [ ] `go test ./...` exits 0, all packages `ok`
- [ ] `go test ./internal/aws/... -run TestRejectIfWindows -v` shows all
      subtests `PASS`
- [ ] `grep -n "func rejectIfWindows" internal/aws/ssh_key_push.go` returns
      exactly one match
- [ ] `grep -n "func DescribeInstancePlatform" internal/aws/ec2.go` returns
      exactly one match
- [ ] `grep -n "platform string) (string, error)" internal/aws/ssh_key_push.go`
      matches `PushSSHKeyViaSSM`'s new signature (confirms the parameter was
      actually added, not just documented)
- [ ] `grep -n "not supported for Windows targets" README.md` returns
      exactly one match
- [ ] `git status --porcelain` shows changes only in: `internal/aws/ssh_key_push.go`,
      `internal/aws/ssh_key_push_test.go`, `internal/aws/ec2.go`,
      `cmd_ec2.go`, `README.md`, `plans/README.md`
- [ ] `plans/README.md` status row for plan 043 updated to reflect the
      outcome

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ssh_key_push.go`, `internal/aws/ec2.go`,
  `internal/aws/ssm.go`, or `cmd_ec2.go` doesn't match the excerpts in
  "Current state" in a way that changes the shape of the fix (e.g.
  `PushSSHKeyViaSSM`'s signature already has more/fewer parameters than
  shown, or `runSSH` no longer has a single `if *pushKey {` block).
- `plans/037-fix-push-key-dedup-check.md` has landed and its changes to
  `internal/aws/ssh_key_push.go` conflict with inserting the
  `rejectIfWindows` call as the function's first statement — in that case,
  re-read the live function body, insert the guard as the first statement
  regardless of what the dedup logic downstream now looks like, and note in
  your commit message that you adapted around plan 037's changes.
- A verification command in Step 2, 3, or 4 fails twice after a reasonable
  fix attempt.
- The fix appears to require touching `helpers.go`'s `pickInstance`,
  `cmd_ssm.go`, or `internal/aws/ssm.go` — those are explicitly out of
  scope; report why the minimal approach in this plan doesn't work rather
  than expanding scope yourself.
- You discover that `ec2 ssh`'s picker (`ListRunningInstances`) has since
  been changed to exclude Windows instances, or that `pickInstance`/`tui.Run`
  no longer expose `Instance.Platform` — either would mean a key assumption
  in this plan ("Windows instances are currently reachable via both
  `--target` and the picker for `ec2 ssh --push-key`") is false, and the
  fix's scope should be reconsidered before proceeding.

## Maintenance notes

- **Parallel plan overlap**: `plans/037-fix-push-key-dedup-check.md` also
  modifies `internal/aws/ssh_key_push.go` (the `grep -qxF` dedup-marker
  logic later in `PushSSHKeyViaSSM`'s body) and likely
  `internal/aws/ssh_key_push_test.go`. There's no hard ordering dependency
  — this plan's guard sits at the very top of the function, before the
  dedup logic — but whichever plan merges second should rebase/re-verify
  against the other's changes to that same file rather than assume a clean
  merge.
- **Deferred, not fixed**: real Windows support for `--push-key` (a
  PowerShell script targeting OpenSSH-for-Windows' actual authorized_keys
  location and ACL model) is a bigger feature, not a bug fix, and was
  explicitly not attempted here. If a future plan tackles it, it should
  replace the `rejectIfWindows` call in `PushSSHKeyViaSSM` with a branch
  that builds and sends a `AWS-RunPowerShellScript` payload (mirroring
  `DocumentForPlatform`'s branching, not `rejectIfWindows`'s all-or-nothing
  check).
- **Known accepted gap, left alone**: `cmd_ssm.go`'s `runSSMRun` still sends
  commands as `AWS-RunShellScript` by default when `--target` is given
  directly (its `platform` variable is only populated via the picker
  branch) — this plan does not touch that function. A future plan could
  give `runSSMRun` the same `DescribeInstancePlatform`-based lookup added
  here in Step 2, if that gap is ever prioritized.
- A reviewer should scrutinize: that `DescribeInstancePlatform` is only
  called when `*pushKey && *target != ""` (not on every `ec2 ssh`
  invocation — that would add an unwanted extra AWS API call to the common
  path), and that the picker branch's switch from `pickInstance(loadFunc)`
  to `tui.Run(loadFunc)` in `runSSH` didn't change picker UX (it shouldn't —
  `pickInstance` is a thin wrapper around the same `tui.Run` call plus
  `os.Exit` handling, which the inlined code reproduces).
