# Plan 036: Add `act ec2 ssh --push-key` to push a local SSH public key via EC2 Instance Connect

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 1f93220..HEAD -- main.go main_test.go internal/aws/ssh_args.go internal/aws/ssh_unix.go internal/aws/ssh_windows.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.
>
> Note: this plan was refreshed at commit `1f93220` (after plan 035, `act ec2
> cp`, landed) to update line numbers and the README prerequisites line,
> which now covers both `ec2 ssh` and `ec2 cp`. `runSSH` and
> `printEC2SSHHelp`'s bodies are unchanged in substance — only shifted by line
> count.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: direction (feature)
- **Planned at**: commit `aa50614`, 2026-09-03 (refreshed at `1f93220`, 2026-09-03)

## Why this matters

`act ec2 ssh` (`main.go:991` `runSSH`, `internal/aws/ssh_unix.go`/`ssh_windows.go`
`StartSSHSession`) opens a real OpenSSH session tunneled through an SSM
`ProxyCommand`. It only works today if the target instance's OS user already
has a matching public key in `~/.ssh/authorized_keys` — the help text says so
explicitly ("Requires an SSH key configured on the target instance",
`main.go:453`). A user with SSM/IAM access to an instance but no key on that
instance (a common case — freshly launched instances, shared bastion boxes,
onboarding a new laptop) currently has no way to get `act ec2 ssh` working
short of manually running `aws ec2-instance-connect send-ssh-public-key`
themselves outside the tool, then immediately re-running `act ec2 ssh` inside
the 60-second window that key stays valid.

AWS provides exactly this mechanism natively:
[EC2 Instance Connect's `SendSSHPublicKey`
API](https://docs.aws.amazon.com/ec2-instance-connect/latest/APIReference/API_SendSSHPublicKey.html)
pushes a public key to a specific OS user on a specific instance for 60
seconds — authenticated by IAM, not by anything already on the instance. This
plan wires a `--push-key` flag into `act ec2 ssh` that calls this API for the
already-resolved instance/user immediately before starting the SSH session,
so the push and the connect happen back-to-back inside the 60-second window
without the user manually shelling out to a second command.

## Current state

- `main.go` — subcommand dispatch and command implementations, `package main`.
  - `runSSH` (`main.go:1033-1064`) — resolves the target instance and SSH
    user, then calls `aws.StartSSHSession`:
    ```go
    func runSSH(profile, region string, subArgs []string) {
    	subArgs, tags := parseTags(subArgs)

    	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
    	target := fs.String("target", "", "Target instance ID")
    	user := fs.String("user", "", "SSH user (default: prompt)")
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

    	err := aws.StartSSHSession(instanceID, profile, region, sshUser)
    	if err != nil {
    		fmt.Fprintf(os.Stderr, "Error starting SSH session: %v\n", err)
    		os.Exit(1)
    	}
    }
    ```
  - `printEC2SSHHelp` (`main.go:452-477`) — the `act ec2 ssh --help` text,
    including the current `Flags:` and `Examples:` blocks that must gain the
    new flag.
  - The `case "ec2":` dispatch block already resolves
    `resolvedProfile`/`resolvedRegion` via `config.ResolveProfile`/
    `config.ResolveRegion` before calling `runSSH` — no dispatch change is
    needed, only `runSSH` itself.
  - `parseTags`, `pickInstance`, `hasHelp` are existing helpers in `main.go`
    used unchanged by this plan.
- `internal/aws/ec2.go` — this package's established convention: every AWS
  call shells out to the `aws` CLI via `os/exec`, never the AWS SDK (there is
  no AWS SDK dependency in `go.mod`). `GetPasswordData` (`ec2.go:176-201`) is
  the closest existing pattern to follow:
    ```go
    func GetPasswordData(instanceID, profile, region, keyPath string) (string, error) {
    	args := []string{"ec2", "get-password-data",
    		"--instance-id", instanceID,
    		"--priv-launch-key", keyPath,
    		"--output", "text",
    		"--query", "PasswordData",
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

    	return normalizePasswordOutput(string(out)), nil
    }
    ```
    Match this shape exactly: build an `args []string`, append `--profile`/
    `--region` only if non-empty, run via `exec.Command("aws", args...)`,
    unwrap `*exec.ExitError` to surface `Stderr`.
  - There is no existing file for `ec2-instance-connect` CLI calls — this is
    a new AWS CLI subcommand surface for this repo (`aws ec2-instance-connect
    send-ssh-public-key`), not part of the `aws ec2 ...` surface `ec2.go`
    wraps. Give it its own new file (see Scope) rather than adding to
    `ec2.go`.
  - Confirmed against the AWS CLI/API reference: `send-ssh-public-key`
    requires `--instance-id`, `--instance-os-user`, `--ssh-public-key`
    (accepts a `file://<path>` value — the AWS CLI reads the file itself);
    `--availability-zone` is optional (omit it — this plan does not add an
    extra `describe-instances` call just to fetch the AZ). The pushed key is
    valid for 60 seconds only, so the push must happen immediately before
    `StartSSHSession`, in the same `runSSH` invocation — never as a separate
    subcommand.
- `internal/doctor/doctor.go:222-230` (`checkConfigFile`) — existing pattern
  for resolving `~/...` paths in this repo: call `os.UserHomeDir()`, check
  errors, then build the path with string concatenation / `filepath.Join`.
  Follow this shape for the default public-key path lookup.
- `main_test.go:210-239` (`TestHelpFunctionsContainDocumentedFlags`) — a
  table test asserting each `print*Help` function's output contains its
  documented flag strings. The existing `printEC2SSHHelp` entry
  (`main_test.go:222`) is:
    ```go
    {"printEC2SSHHelp", printEC2SSHHelp, []string{"--target", "--user", "--tag"}},
    ```
  This must gain the two new flag strings.
- `README.md`:
  - Line 20: `- **SSH over SSM** — SSH to instances without open inbound ports`
    (Features list).
  - Line 36: the IAM permissions list (single line, comma-separated,
    backtick-quoted actions) — must gain
    `ec2-instance-connect:SendSSHPublicKey`.
  - Line 38: `- For `ec2 ssh`/`ec2 cp`: OpenSSH client (`ssh`/`scp`) and an SSH
    key configured on the target instance` (this line now covers both `ec2
    ssh` and `ec2 cp`, added by plan 035 after this plan was first written) —
    must be updated to describe the new `--push-key` alternative for `ec2
    ssh` specifically, while leaving the `ec2 cp` part of the sentence intact
    (that command has no push-key flag and still requires a pre-configured
    key).
  - Lines 149-152 (the `# SSH to an instance via SSM` example block, directly
    above the `# Copy a file to an instance via SSM` block) — must gain two
    `--push-key` examples.
  - The `### `ec2` vs `ec2 ssh`` comparison table (further down) — left
    alone; it compares `act ec2` vs `act ec2 ssh` broadly and does not need a
    third column for this.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | exit 0, no output (no files listed) |
| Tests | `go test ./...` | all pass, `ok` for every package |
| Full gate (matches CI) | `make test` | exit 0 |

## Scope

**In scope**:
- `internal/aws/instance_connect.go` (new) — `SendSSHPublicKey` and the
  default-public-key-path lookup.
- `internal/aws/instance_connect_test.go` (new) — unit tests for the pure,
  testable path-lookup helper.
- `main.go` — `runSSH` (add two flags and the push-then-connect call) and
  `printEC2SSHHelp` (document the two new flags and add two examples).
- `main_test.go` — extend the existing `printEC2SSHHelp` entry in
  `TestHelpFunctionsContainDocumentedFlags` with the two new flag strings.
- `README.md` — Prerequisites line for `ec2 ssh`, IAM permissions list,
  usage examples block. (Features list line 20 may optionally get a
  parenthetical; not required.)

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ssh_args.go` / `ssh_unix.go` / `ssh_windows.go` — the actual
  SSH-over-SSM connection logic is unchanged; this plan only adds a step
  *before* `aws.StartSSHSession` is called. Do not refactor
  `sshProxyArgs`/`StartSSHSession`.
- `internal/aws/ec2.go` — do not add the new function here; it wraps a
  different AWS CLI service surface (`aws ec2 ...`, not `aws
  ec2-instance-connect ...`). Keep them in separate files as this repo does
  for other service surfaces (e.g. `ssm.go` vs `ec2.go` vs `ecs.go`).
- `internal/doctor/*` — doctor does not check for the EC2 Instance Connect
  agent on target instances (it cannot — that's an instance-side check, not
  a local-machine check) and this plan does not add such a check. Out of
  scope.
- `act ec2 rdp` / `runRDP` / `GetPasswordData` — unrelated command, do not
  touch.
- Any change to the public behavior of `act ec2 ssh` when `--push-key` is
  *not* passed — the existing "requires a pre-configured key" path must
  continue to work exactly as before.
- Adding an `--availability-zone` flag or an extra `describe-instances` call
  to fetch it — the API treats it as optional; omit it (see "Current state"
  and "Maintenance notes").

## Git workflow

- Branch: `advisor/036-ec2-ssh-push-key-instance-connect`
- Commit per step or per logical unit. This repo's commit style is a short
  imperative summary line, e.g. `fix: let act doctor run without AWS CLI
  already installed` or `Add plan 019: ...` — match that style, e.g. `feat:
  add --push-key to act ec2 ssh via EC2 Instance Connect`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add `internal/aws/instance_connect.go`

Create the file with three functions:

```go
package aws

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SendSSHPublicKey pushes a local SSH public key to the given OS user on the
// given EC2 instance via EC2 Instance Connect. The pushed key is valid for
// only 60 seconds, so the caller must start the SSH session immediately
// afterward.
func SendSSHPublicKey(instanceID, profile, region, osUser, publicKeyPath string) error {
	absPath, err := filepath.Abs(publicKeyPath)
	if err != nil {
		return fmt.Errorf("resolving public key path: %w", err)
	}

	args := []string{"ec2-instance-connect", "send-ssh-public-key",
		"--instance-id", instanceID,
		"--instance-os-user", osUser,
		"--ssh-public-key", "file://" + absPath,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	_, err = cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return err
	}

	return nil
}

// DefaultSSHPublicKeyPath returns the first of the user's standard SSH
// public key files that exists, preferring id_ed25519.pub over id_rsa.pub.
func DefaultSSHPublicKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return findSSHPublicKey(home)
}

func findSSHPublicKey(homeDir string) (string, error) {
	candidates := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no default SSH public key found (checked %s); use --push-key-path to specify one", strings.Join(candidates, ", "))
}
```

This matches the exec/error-unwrap shape of `GetPasswordData`
(`internal/aws/ec2.go:176-201`) and the home-dir-resolution shape of
`checkConfigFile` (`internal/doctor/doctor.go:222-230`).

**Verify**: `go build ./...` → exit 0.

### Step 2: Add `internal/aws/instance_connect_test.go`

Test the pure, filesystem-injectable helper `findSSHPublicKey` (not
`SendSSHPublicKey`, which shells out to `aws` — this repo does not unit-test
its `exec.Command`-wrapping functions; e.g. `GetPasswordData` itself has no
test, only its pure helper `normalizePasswordOutput` does, per
`internal/aws/ec2_test.go:39-58`. Follow that same split.):

```go
package aws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSSHPublicKey(t *testing.T) {
	t.Run("prefers ed25519 over rsa", func(t *testing.T) {
		home := t.TempDir()
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"id_ed25519.pub", "id_rsa.pub"} {
			if err := os.WriteFile(filepath.Join(sshDir, name), []byte("key"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := findSSHPublicKey(home)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(sshDir, "id_ed25519.pub")
		if got != want {
			t.Errorf("findSSHPublicKey() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to rsa", func(t *testing.T) {
		home := t.TempDir()
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("key"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := findSSHPublicKey(home)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(sshDir, "id_rsa.pub")
		if got != want {
			t.Errorf("findSSHPublicKey() = %q, want %q", got, want)
		}
	})

	t.Run("errors when neither exists", func(t *testing.T) {
		home := t.TempDir()
		if _, err := findSSHPublicKey(home); err == nil {
			t.Error("expected an error when no default key exists, got nil")
		}
	})
}
```

**Verify**: `go test ./internal/aws/... -run TestFindSSHPublicKey -v` → all
three subtests `PASS`.

### Step 3: Wire `--push-key` / `--push-key-path` into `runSSH`

Edit `main.go`. Add the two flags to the existing `flag.NewFlagSet("ssh",
...)` block, and insert the push step after `sshUser` is resolved but before
`aws.StartSSHSession` is called:

```go
func runSSH(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	pushKey := fs.Bool("push-key", false, "Push local SSH public key via EC2 Instance Connect before connecting")
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
		if err := aws.SendSSHPublicKey(instanceID, profile, region, sshUser, keyPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via EC2 Instance Connect (valid 60s).\n", keyPath, sshUser, instanceID)
	}

	err := aws.StartSSHSession(instanceID, profile, region, sshUser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting SSH session: %v\n", err)
		os.Exit(1)
	}
}
```

Note: on push failure, `os.Exit(1)` rather than falling through to attempt
the SSH connection anyway — the user explicitly asked for the push, and
attempting SSH without it would just produce a confusing "permission denied"
further down instead of the real EC2 Instance Connect error.

**Verify**: `go build ./...` → exit 0.

### Step 4: Update `printEC2SSHHelp`

Edit `main.go:452-477`. Add the two flags to the `Flags:` block and two
examples to the `Examples:` block:

```go
func printEC2SSHHelp() {
	fmt.Fprintf(os.Stderr, `act ec2 ssh - SSH to EC2 instance via SSM

Usage: act [global flags] ec2 ssh [flags]

Starts a real SSH session using SSM as a ProxyCommand. This enables
SCP, rsync, agent forwarding (-A), and port forwarding (-L/-R).

Requires an SSH key configured on the target instance, or use --push-key
to push your local public key via EC2 Instance Connect first.

Flags:
  --target        Target instance ID (skip instance picker)
  --user          SSH user (default: prompt interactively)
  --tag           Filter instances by tag (key=value, can be repeated)
  --push-key      Push local SSH public key via EC2 Instance Connect before
                  connecting (valid 60 seconds; requires the EC2 Instance
                  Connect agent on the target instance)
  --push-key-path Path to public key to push (default: ~/.ssh/id_ed25519.pub
                  or ~/.ssh/id_rsa.pub)

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act ec2 ssh
  act ec2 ssh --user ubuntu
  act ec2 ssh --user ec2-user --target i-0123456789abcdef0
  act ec2 ssh --push-key
  act ec2 ssh --push-key --push-key-path ~/.ssh/my_key.pub
`)
}
```

**Verify**: `go build ./... && ./act ec2 ssh --help 2>&1 | grep push-key` →
prints both `--push-key` and `--push-key-path` lines (run from the repo
root after `go build -o act .`, or via `go run . ec2 ssh --help`).

### Step 5: Update the `TestHelpFunctionsContainDocumentedFlags` entry

Edit `main_test.go:222`:

```go
{"printEC2SSHHelp", printEC2SSHHelp, []string{"--target", "--user", "--tag", "--push-key", "--push-key-path"}},
```

**Verify**: `go test ./... -run TestHelpFunctionsContainDocumentedFlags -v` →
`PASS`, including the `printEC2SSHHelp` subtest.

### Step 6: Update `README.md`

1. Prerequisites (line 38) — replace:
   ```
   - For `ec2 ssh`/`ec2 cp`: OpenSSH client (`ssh`/`scp`) and an SSH key configured on the target instance
   ```
   with:
   ```
   - For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to push your local public key via EC2 Instance Connect (requires the EC2 Instance Connect agent on the instance — preinstalled on Amazon Linux 2/2023 and Ubuntu AMIs); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance
   ```
2. IAM permissions list (line 36) — append `` `ec2-instance-connect:SendSSHPublicKey` `` to the comma-separated list (after `` `ec2:GetPasswordData` ``).
3. Usage examples (in the `# SSH to an instance via SSM` block, directly before the `# Copy a file to an instance via SSM` block) — add:
   ```
   act ec2 ssh --push-key                                       # push your local pubkey via EC2 Instance Connect first
   act ec2 ssh --push-key --push-key-path ~/.ssh/my_key.pub     # push a specific key
   ```

**Verify**: `grep -n "push-key" README.md` → at least 3 matches (prerequisites
line, 2 example lines); `grep -n "ec2-instance-connect:SendSSHPublicKey" README.md` → 1 match.

### Step 7: Full verification gate

Run the repo's standard gate.

**Verify**: `make test` → exit 0 (runs `gofmt -l .` with zero output, `go vet
./...` with zero output, then `go test ./...` all green).

## Test plan

- New tests: `internal/aws/instance_connect_test.go` — `TestFindSSHPublicKey`
  with three subtests (prefers ed25519, falls back to rsa, errors when
  neither exists), modeled on `internal/aws/ec2_test.go`'s
  `TestNormalizePasswordOutput` table-test style but using `t.TempDir()`
  since this helper touches the filesystem.
- `SendSSHPublicKey` itself is not unit-tested, matching this repo's
  established convention that thin `exec.Command`-wrapping functions
  (`GetPasswordData`, `ListRunningInstances`, etc.) are not unit-tested —
  only their pure helpers are (`normalizePasswordOutput`,
  `escapeTagFilterValue`). Do not attempt to mock `exec.Command` to test it;
  that would be new infrastructure out of proportion to this plan.
- Manual smoke test (optional, requires real AWS access — do not block on
  this if unavailable): `act ec2 ssh --push-key --target <real-instance-id>
  --user ec2-user` against a real instance with the EC2 Instance Connect
  agent installed and appropriate IAM permissions; confirm it prints the
  "Pushed ... via EC2 Instance Connect" line and then successfully opens an
  SSH session.
- Verification: `go test ./...` → all pass, including the 3 new subtests
  under `internal/aws`.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` exits 0 with no output
- [ ] `go test ./...` exits 0; `TestFindSSHPublicKey` (3 subtests) and the
  updated `TestHelpFunctionsContainDocumentedFlags` both pass
- [ ] `go run . ec2 ssh --help 2>&1 | grep -c push-key` returns `2` or more
- [ ] `grep -c "ec2-instance-connect:SendSSHPublicKey" README.md` returns `1`
- [ ] `git status` shows changes only in: `internal/aws/instance_connect.go`,
  `internal/aws/instance_connect_test.go`, `main.go`, `main_test.go`,
  `README.md`, and `plans/README.md`

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `main.go:1033-1064` (`runSSH`) or `main.go:452-477`
  (`printEC2SSHHelp`) doesn't match the excerpts in "Current state" — the
  codebase has drifted since this plan was written.
- `internal/aws/ec2.go`'s `GetPasswordData` no longer follows the
  exec/`*exec.ExitError`-unwrap pattern shown above (i.e. this repo has
  switched to the AWS SDK or a different exec pattern) — re-verify the
  convention before writing `SendSSHPublicKey` to match it.
- You find that `aws ec2-instance-connect send-ssh-public-key` is not
  available in the AWS CLI version assumed by this repo (unlikely — it has
  been GA for years — but if `aws ec2-instance-connect help` errors in your
  environment, stop and report rather than guessing at flag names).
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching an out-of-scope file (see "Scope").

## Maintenance notes

- **60-second window is load-bearing.** `SendSSHPublicKey` must be called
  immediately before `aws.StartSSHSession` inside the same `runSSH`
  invocation, never cached or reused across separate command invocations. If
  a future change adds retry logic around `StartSSHSession`, the push must
  be retried too (or moved inside the retry loop) — a stale push means the
  retry's SSH auth will fail with an opaque `Permission denied`.
- **`--availability-zone` is intentionally omitted** from the
  `send-ssh-public-key` call — the API documents it as optional. If AWS ever
  makes it effectively required for some instance placements (e.g. Local
  Zones, Outposts), the fix is to add one `ec2 describe-instances --query
  Reservations[].Instances[].Placement.AvailabilityZone` call before the
  push — not to fetch it speculatively today.
- **This does not add any `doctor` check.** Whether an instance has the EC2
  Instance Connect agent installed and running is instance-side state that
  `act doctor` (a local-machine checker) cannot observe. If a future plan
  wants to help users diagnose "push succeeded but ssh still fails," that
  diagnostic belongs in `runSSH`'s error message for `StartSSHSession`
  failing right after a successful push, not in `doctor`.
- A reviewer should scrutinize: that the push failure path exits before
  attempting `StartSSHSession` (Step 3), and that `--push-key-path` values
  are passed through `filepath.Abs` before being embedded in the `file://`
  URI (so relative paths resolve against the process CWD correctly rather
  than being silently misinterpreted by the AWS CLI).
- Deferred, not part of this plan: no flag was added to `act ec2 rdp` for an
  equivalent Windows password-based analog — EC2 Instance Connect's
  `SendSSHPublicKey` is Linux/SSH-specific; there is no equivalent API for
  Windows RDP credentials. Not applicable, not a gap.
