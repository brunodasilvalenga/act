# Plan 041: Make `act doctor` warn when the OpenSSH client (`ssh`/`scp`) is missing

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, add a new row for plan 041 to the
> status table in `plans/README.md` (it currently has no row for 041 — add
> one after the row for plan 036).
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/doctor/doctor.go internal/doctor/doctor_test.go README.md`
> If any of these three files changed since this plan was written, re-read
> them in full and compare against the "Current state" excerpts below before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

README.md's Prerequisites section documents the OpenSSH client (`ssh`/`scp`)
as required for `act ec2 ssh` / `act ec2 cp`, and `act doctor`'s own help
text claims it "Checks that all required tools are installed." In reality,
`act doctor`'s check list (`internal/doctor/doctor.go`) never looks for `ssh`
or `scp` at all. A user can run `act doctor` — or the documented unattended
pattern `act doctor --fix --skip-confirm` used in provisioning scripts — see
"All checks passed!", and then have `act ec2 ssh` or `act ec2 cp` fail
immediately with a missing-binary error. This is exactly the class of
problem `doctor` exists to catch, and it is most damaging for the
`--skip-confirm` unattended case, where there is no interactive moment for a
human to notice the gap. This plan closes that gap for `ssh`/`scp` with a
low-risk, warning-level check (not a hard failure, since only 2 of the CLI's
many subcommands need these binaries) and updates README.md so it no longer
implies broader `doctor` coverage than actually exists.

## Current state

- `internal/doctor/doctor.go` — all `doctor` check functions and the `Run`
  orchestrator that calls them, in this exact order.
- `internal/doctor/doctor_test.go` — the check functions' test suite.
- `internal/doctor/fix.go` — the `--fix` remediation runner (`runFixes`);
  read-only reference, not modified by this plan (see "Why no change to
  fix.go" below).
- `README.md` — the Prerequisites section (lines 32–39) that documents
  `ssh`/`scp`/RDP-client requirements.
- `help.go` — `printDoctorHelp()` (lines 304–331), which already says
  `act doctor` "Checks that all required tools are installed" — this wording
  does not need to change; making it *true* is the point of this plan.

### `Run`, verbatim, `internal/doctor/doctor.go:50-123`

```go
func Run(profile, region, env, version string, fix, skipConfirm bool) error {
	// results is pre-sized so each check writes into a fixed, known index.
	// This preserves the exact print order below regardless of which of
	// checkCredentials/checkVersion finishes first.
	results := make([]result, 7)
	results[0] = checkAWSCLI()
	results[1] = checkSessionManagerPlugin()
	results[3] = checkRegion(region, env)
	results[4] = checkProfile(profile, env)
	results[5] = checkConfigFile()

	// checkCredentials (index 2) and checkVersion (index 6) are the only
	// two checks that make a network call (AWS STS and the GitHub API,
	// respectively). Neither depends on the other or on any of the checks
	// above, so run them concurrently to pay the max of their two
	// round-trips instead of the sum. Each goroutine writes to its own
	// slice index — no two goroutines (and no sequential code above)
	// write index 2 or 6 — and Wait() is the synchronization point that
	// makes those writes visible to this goroutine before they're read
	// below, so this has no data race.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		results[2] = checkCredentials(profile, region, env)
	}()
	go func() {
		defer wg.Done()
		results[6] = checkVersion(version)
	}()
	wg.Wait()

	if fix {
		recheck := func(name string) result {
			switch name {
			case "AWS CLI":
				return checkAWSCLI()
			case "Session Manager plugin":
				return checkSessionManagerPlugin()
			default:
				return result{Name: name, Status: statusFail, Detail: "unknown check"}
			}
		}
		results, _ = runFixes(results, skipConfirm, recheck)
	}
	// ... printing loop follows (unchanged by this plan)
```

Note the comment explains *why* the slice is pre-sized and indexed: two
checks (`checkCredentials`, `checkVersion`) run concurrently in goroutines
writing to fixed indices 2 and 6; everything else is sequential. This plan
adds one more **sequential** check and must not disturb indices 0–6 or the
concurrency pattern.

### The two exemplar `exec.LookPath`-based checks to model the new one on

`checkAWSCLI`, `internal/doctor/doctor.go:125-149` (uses `statusFail` because
almost every `act` command needs the AWS CLI):

```go
func checkAWSCLI() result {
	path, err := exec.LookPath("aws")
	if err != nil {
		return result{
			Name:   "AWS CLI",
			Status: statusFail,
			Detail: "not found. Install: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
			Fix: &fixAction{
				Describe: describeAWSCLIInstall,
				Apply:    installAWSCLI,
			},
		}
	}

	out, err := exec.Command("aws", "--version").Output()
	ver := strings.TrimSpace(string(out))
	if err != nil || ver == "" {
		ver = "unknown version"
	}
	return result{
		Name:   "AWS CLI",
		Status: statusPass,
		Detail: fmt.Sprintf("%s (%s)", path, ver),
	}
}
```

`checkSessionManagerPlugin`, `internal/doctor/doctor.go:151-169` (same
`exec.LookPath` shape, `statusFail`, has a `Fix` action):

```go
func checkSessionManagerPlugin() result {
	path, err := exec.LookPath("session-manager-plugin")
	if err != nil {
		return result{
			Name:   "Session Manager plugin",
			Status: statusFail,
			Detail: "not found. Install: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html",
			Fix: &fixAction{
				Describe: describeSSMPluginInstall,
				Apply:    installSSMPlugin,
			},
		}
	}
	return result{
		Name:   "Session Manager plugin",
		Status: statusPass,
		Detail: path,
	}
}
```

### The `statusWarn` exemplar: `checkRegion`, `internal/doctor/doctor.go:211-225`

```go
func checkRegion(flagRegion, env string) result {
	resolved := config.ResolveRegion(flagRegion, env)
	if resolved == "" {
		return result{
			Name:   "Region",
			Status: statusWarn,
			Detail: "not configured (use --region, AWS_REGION, or ~/.act.json)",
		}
	}
	return result{
		Name:   "Region",
		Status: statusPass,
		Detail: resolved,
	}
}
```

`checkProfile` (`doctor.go:227-241`) follows the identical shape. Both use
`statusWarn` for a condition that is "not strictly required for everything,
but here's a heads up" — exactly the severity that fits `ssh`/`scp`, which
only 2 of the CLI's many subcommands need. Confirmed by grepping the
codebase: `ssh`/`scp` are `exec.LookPath`'d or `exec.Command`'d only in
`internal/aws/ssh_unix.go:22`, `internal/aws/ssh_windows.go` (unconditional
`exec.Command("ssh", ...)`), and `internal/aws/scp.go:49` — i.e., only by the
`ec2 ssh` and `ec2 cp` code paths. No other subcommand touches them. This
confirms `statusWarn` (not `statusFail`) is the right severity: unlike the
AWS CLI, a user who never runs `ec2 ssh`/`ec2 cp` should not get a red,
exit-code-1 `doctor` failure for a tool they don't need.

### `result` / `status` types, `internal/doctor/doctor.go:16-42`

```go
type status int

const (
	statusPass status = iota
	statusWarn
	statusFail
)

type result struct {
	Name   string
	Status status
	Detail string
	Fix    *fixAction // nil when this check has no automated fix
}
```

### Why no change to `fix.go`

`runFixes`, `internal/doctor/fix.go:44-47`:

```go
	for i, r := range results {
		if r.Status != statusFail || r.Fix == nil {
			continue
		}
```

It iterates the whole `results` slice and only acts on entries that are
`statusFail` **and** have a non-nil `Fix`. The new check will be
`statusWarn` with `Fix: nil` (matching `checkRegion`/`checkProfile`/
`checkConfigFile`, none of which set `Fix`), so `runFixes` will automatically
skip it with no code change required in `fix.go`. Do not add a `Fix` action
for this check — see "Out of scope" below for why.

### RDP-client detection: explicitly out of scope

The finding also flagged that README.md documents an RDP client as required
for `act ec2 rdp` (README.md:39) but `doctor` never checks for one either.
`act ec2 rdp`'s actual launch logic is in `internal/aws/rdp_unix.go:44-53`:

```go
	if openClient {
		switch runtime.GOOS {
		case "darwin":
			url := fmt.Sprintf("rdp://full%%20address=s:localhost:%d", localPort)
			exec.Command("open", url).Start()
		default:
			fmt.Println("Connect with your RDP client to localhost:" + fmt.Sprint(localPort))
		}
	}
```

On macOS it shells out to `open rdp://...` and relies on the OS's registered
handler for the `rdp:` URL scheme (e.g. Microsoft Remote Desktop, if
installed) — there is no single canonical `exec.LookPath`-able binary name
for "an RDP client is installed" the way there is for `ssh`/`aws`/
`session-manager-plugin`. Building a reliable cross-platform "is an RDP
client present" check (probing `open -Ra`, checking Windows for `mstsc.exe`
on `PATH`, etc.) is a materially different and fuzzier problem than the
`ssh`/`scp` check, and risks false positives/negatives that make `doctor`
*less* trustworthy, not more. This plan deliberately does **not** attempt
it. It is a documented gap and a candidate for a separate follow-up plan,
not part of this one — see "Maintenance notes" below.

## Commands you will need

| Purpose               | Command                                                                                   | Expected on success               |
|------------------------|--------------------------------------------------------------------------------------------|------------------------------------|
| Build                  | `go build ./...`                                                                            | exit 0, no output                  |
| Vet                    | `go vet ./...`                                                                              | exit 0, no output                  |
| Format check           | `gofmt -l internal/doctor/doctor.go internal/doctor/doctor_test.go`                        | exit 0, no output (no files listed)|
| Doctor package tests   | `go test ./internal/doctor/... -race -v`                                                    | all pass, includes `TestCheckSSHClient` |
| Full test suite        | `go test ./...`                                                                             | exit 0, all pass                   |

(Verified against this repo during recon: `go build ./...` and `go vet ./...`
both currently exit 0 with no output at commit `0802c51`.)

## Scope

**In scope** (the only files you should modify):
- `internal/doctor/doctor.go` — add `checkSSHClient()`, wire it into `Run`.
- `internal/doctor/doctor_test.go` — add `TestCheckSSHClient`, extend
  `TestRunResultOrder`'s scaffold to document the new 8th index.
- `README.md` — clarify, in the Prerequisites section, that `doctor` now
  checks for `ssh`/`scp` (warn-level) and does not check for an RDP client.
- `plans/README.md` — add the status row for plan 041 only (per the
  executor-instructions header above); do not otherwise edit this file.

**Out of scope** (do NOT touch, even though they look related):
- `internal/doctor/fix.go` — no change needed; `runFixes` already skips
  `statusWarn`/`Fix:nil` results automatically (see "Why no change to
  fix.go" above).
- `help.go` (`printDoctorHelp`) — its existing text ("Checks that all
  required tools are installed") is already accurate once this plan lands;
  no wording change is needed.
- `internal/aws/rdp_unix.go`, `internal/aws/rdp_windows.go`, and any RDP-client
  detection logic — explicitly deferred, see "RDP-client detection" above.
- Adding a `Fix`/auto-install action for `ssh`/`scp` — OpenSSH client
  installation is highly platform-specific (macOS ships it by default;
  Linux distros vary; Windows has an optional feature) and no finding
  requires auto-remediation here. Keep this check detection-only, matching
  `checkRegion`/`checkProfile`/`checkConfigFile`, none of which have a `Fix`.
- Any change to `results` indices 0–6 or the `sync.WaitGroup` concurrency
  block in `Run` (`doctor.go:70-80`) — the new check must be a plain
  sequential append at index 7, added *after* `wg.Wait()`.

## Git workflow

- Branch: `advisor/041-add-doctor-ssh-scp-client-check` (matches this repo's
  convention, e.g. `advisor/036-ec2-ssh-push-key-instance-connect`,
  `advisor/035-add-ec2-cp-command`).
- Commit per step or one commit for the whole change if steps are small
  enough to land atomically; message style matches this repo's conventional
  commits, e.g. `feat: add doctor --fix to auto-remediate missing AWS CLI /
  Session Manager plugin` (from `git log`). Suggested message for this plan:
  `feat: warn in act doctor when ssh/scp is missing`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add `checkSSHClient()` to `internal/doctor/doctor.go`

Add the following function immediately after `checkSessionManagerPlugin`
(after line 169, before `func checkCredentials`), grouping it with the other
"external tool present" checks:

```go
// checkSSHClient reports whether the OpenSSH client binaries (ssh, scp) are
// on PATH. Unlike checkAWSCLI/checkSessionManagerPlugin, this is statusWarn
// rather than statusFail: only `act ec2 ssh` and `act ec2 cp` need these
// binaries, so a user who never runs those subcommands should not get a
// red, exit-code-1 doctor failure for a tool they don't use.
func checkSSHClient() result {
	_, sshErr := exec.LookPath("ssh")
	_, scpErr := exec.LookPath("scp")

	if sshErr == nil && scpErr == nil {
		return result{
			Name:   "SSH client",
			Status: statusPass,
			Detail: "ssh and scp found (used by `act ec2 ssh` / `act ec2 cp`)",
		}
	}

	var missing []string
	if sshErr != nil {
		missing = append(missing, "ssh")
	}
	if scpErr != nil {
		missing = append(missing, "scp")
	}

	return result{
		Name:   "SSH client",
		Status: statusWarn,
		Detail: fmt.Sprintf(
			"%s not found (only needed for `act ec2 ssh` / `act ec2 cp`). Install the OpenSSH client: https://www.openssh.com/",
			strings.Join(missing, ", "),
		),
	}
}
```

This uses only `fmt`, `exec`, and `strings`, all of which are already
imported in `doctor.go` (see the import block at `doctor.go:1-14`) — no
import changes needed.

**Verify**: `go build ./...` → exit 0, no output.

### Step 2: Wire the new check into `Run`

In `internal/doctor/doctor.go`, inside `Run` (currently lines 50-80):

1. Change:
   ```go
   	results := make([]result, 7)
   ```
   to:
   ```go
   	results := make([]result, 8)
   ```
2. Immediately after the existing `wg.Wait()` line (currently line 80) and
   before the existing `if fix {` line (currently line 82), add:
   ```go

   	// checkSSHClient is a plain sequential check (like indices 0/1/3/4/5
   	// above) — it does not need to run concurrently since it does not
   	// make a network call, and it must come after wg.Wait() so it does
   	// not race with the goroutines writing indices 2/6 above.
   	results[7] = checkSSHClient()
   ```

Do not renumber or otherwise touch indices 0–6, the `checkCredentials`/
`checkVersion` goroutines, or the `if fix { ... }` block — this plan adds
one new sequential entry at the end only.

**Verify**: `go build ./... && go vet ./...` → both exit 0, no output.
Also run: `grep -n "make(\[\]result, 8)" internal/doctor/doctor.go` →
exactly 1 match.

### Step 3: Add `TestCheckSSHClient` to `internal/doctor/doctor_test.go`

`checkAWSCLI` and `checkSessionManagerPlugin` — the two existing
`exec.LookPath`-based checks — currently have **no direct unit tests** in
`doctor_test.go` (confirmed: the file's only tests are `TestExtractJSON`,
`TestCheckRegion(WithEnv)`, `TestCheckProfile(WithEnv)`, and
`TestRunResultOrder`). This is a real, pre-existing coverage gap for
`LookPath`-based checks, not something to paper over: since whether `ssh`/
`scp` are installed depends on the machine running the test, a test cannot
assert a specific pass/warn outcome without mocking `exec.LookPath` (which
the existing two checks don't do either — don't introduce dependency
injection here just for this check, that would be an inconsistent,
un-asked-for pattern change).

Add this test, which is deterministic regardless of whether `ssh`/`scp` are
actually present on the machine running `go test`:

```go
func TestCheckSSHClient(t *testing.T) {
	r := checkSSHClient()
	if r.Name != "SSH client" {
		t.Errorf("expected Name %q, got %q", "SSH client", r.Name)
	}
	if r.Status != statusPass && r.Status != statusWarn {
		t.Errorf("expected statusPass or statusWarn (never statusFail), got %v", r.Status)
	}
	if r.Status == statusWarn && !strings.Contains(r.Detail, "not found") {
		t.Errorf("expected Detail to mention 'not found' when warning, got %q", r.Detail)
	}
	if r.Fix != nil {
		t.Errorf("expected no automated Fix action for checkSSHClient, got one")
	}
}
```

This requires adding `"strings"` to the import block at the top of
`doctor_test.go` (currently `"os"`, `"path/filepath"`, `"runtime"`, `"sync"`,
`"testing"` — `"strings"` is not yet imported there).

**Verify**: `go test ./internal/doctor/... -race -run TestCheckSSHClient -v`
→ `PASS`, exit 0.

### Step 4: Keep `TestRunResultOrder`'s scaffold in sync

`TestRunResultOrder` (`doctor_test.go:124-168`) documents, via a synthetic
dummy slice (not a call to the real `Run`), the exact indexing/concurrency
shape used by `Run`. It currently only builds indices 0–6. Since `Run` now
has 8 entries (0–7), extend the scaffold so the test's doc-comment stays
truthful:

1. Update the doc comment above the test (currently starting "exercises the
   exact concurrency shape used by Run: a pre-sized []result slice, 5
   sequential writes to indices 0/1/3/4/5, and two goroutines...") to also
   mention the new sequential write at index 7.
2. Add `"SSH client"` to the end of `wantNames`.
3. Change `results := make([]result, 7)` to `results := make([]result, 8)`.
4. After `wg.Wait()` in the test body, add: `results[7] = dummy("SSH client")`.

**Verify**: `go test ./internal/doctor/... -race -run TestRunResultOrder -v`
→ `PASS`, exit 0, for all 20 iterations.

### Step 5: Update README.md's Prerequisites section

Current text, `README.md:38-39`:

```
- For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to add your local public key to the target's `authorized_keys` via SSM Run Command (only needs the SSM Agent — no extra on-instance agent; the key persists until removed, and `--push-key` prints the exact command to remove it); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance
- For `ec2 rdp`: An RDP client (macOS: Microsoft Remote Desktop or built-in; Windows: mstsc)
```

Replace with (append a clause to each line, keep everything else identical):

```
- For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to add your local public key to the target's `authorized_keys` via SSM Run Command (only needs the SSM Agent — no extra on-instance agent; the key persists until removed, and `--push-key` prints the exact command to remove it); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance. `act doctor` checks for `ssh`/`scp` on PATH and warns (does not fail) if either is missing.
- For `ec2 rdp`: An RDP client (macOS: Microsoft Remote Desktop or built-in; Windows: mstsc). `act doctor` does not currently check for an RDP client.
```

This is the `CLAUDE.md`-mandated README update ("Keep README examples
consistent with actual CLI help output" / update docs after a feature
change) and directly closes the documentation-vs-behavior gap this plan was
written to fix — including for the part (RDP) this plan does not implement,
so the doc no longer overclaims there either.

**Verify**: `grep -n "act doctor.*checks for .ssh./.scp." README.md` → 1
match. `grep -n "does not currently check for an RDP client" README.md` → 1
match.

## Test plan

- New test: `TestCheckSSHClient` in `internal/doctor/doctor_test.go` (Step
  3) — covers: correct `Name`, `Status` is one of the two valid values, and
  `Detail` wording invariant per status. **Coverage limitation, stated
  honestly**: like the pre-existing `checkAWSCLI`/`checkSessionManagerPlugin`
  (which have zero direct tests), this test cannot force the "found" vs.
  "missing" branch deterministically without mocking `exec.LookPath`, which
  would be a bigger, inconsistent change to introduce. It verifies the
  function's contract, not both of its branches individually — this matches
  the existing test suite's honest limitation for this class of check, not
  a new one introduced by this plan.
- Structural test: `TestRunResultOrder` extended (Step 4) to prove the new
  index-7 write is race-free under `-race` and preserves print order.
- Pattern followed: `TestCheckRegion`/`TestCheckProfile` for the "assert
  Name/Status/Detail on a directly-called check function" shape;
  `TestRunResultOrder` for the "prove the indexing/concurrency scaffold is
  race-free" shape. No new test infrastructure introduced.
- Verification: `go test ./internal/doctor/... -race -v` → all tests pass,
  including the new `TestCheckSSHClient` and the extended
  `TestRunResultOrder`. Then `go test ./...` → exit 0, all packages pass.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0, no output
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l internal/doctor/doctor.go internal/doctor/doctor_test.go` exits 0, no output
- [ ] `go test ./... -race` exits 0, all pass
- [ ] `grep -n "func checkSSHClient" internal/doctor/doctor.go` → 1 match
- [ ] `grep -n "results\[7\] = checkSSHClient()" internal/doctor/doctor.go` → 1 match
- [ ] `grep -n "make(\[\]result, 8)" internal/doctor/doctor.go` → 1 match
- [ ] `grep -n "func TestCheckSSHClient" internal/doctor/doctor_test.go` → 1 match
- [ ] `grep -n "act doctor.*checks for .ssh./.scp." README.md` → 1 match
- [ ] `grep -n "does not currently check for an RDP client" README.md` → 1 match
- [ ] `git status` shows modifications only in: `internal/doctor/doctor.go`, `internal/doctor/doctor_test.go`, `README.md`, `plans/README.md`
- [ ] `plans/README.md` has a new status row for plan 041

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match the excerpts
  above (the codebase has drifted since this plan was written at commit
  `0802c51`) — re-read the live files and reconcile before proceeding, per
  the drift check at the top of this plan.
- `go build`, `go vet`, or `go test ./internal/doctor/...` fail and do not
  resolve after one reasonable fix attempt.
- The fix appears to require touching `internal/doctor/fix.go`,
  `internal/aws/rdp_unix.go`, `internal/aws/rdp_windows.go`, or `help.go` —
  all are explicitly out of scope for this plan (see "Out of scope").
- You discover `ssh` or `scp` is now also required by some other, non-`ec2
  ssh`/`ec2 cp` subcommand (re-run `grep -rn '"ssh"\|"scp"' internal/aws/*.go
  main.go` to check) — if so, the `statusWarn` severity recommendation in
  this plan may need to be `statusFail` instead; stop and report rather than
  silently changing the severity.
- You are tempted to add RDP-client detection "while you're in here" —
  don't; it is explicitly deferred (see "RDP-client detection" above). Stop
  and report if you believe it should be in scope after all, rather than
  expanding scope unilaterally.

## Maintenance notes

- If a future change makes `ssh`/`scp` a hard dependency of more
  subcommands (e.g., a new feature that shells out to `ssh` outside of
  `ec2 ssh`/`ec2 cp`), revisit whether `checkSSHClient` should become
  `statusFail` for that case, or whether a second, more targeted check is
  warranted.
- RDP-client detection remains an open, explicitly-deferred gap (see
  "RDP-client detection" in "Current state"). A future follow-up plan could
  scope a best-effort, platform-specific heuristic (e.g., `open -Ra
  "Microsoft Remote Desktop"` on macOS, checking for `mstsc.exe` on Windows
  `PATH`) but should treat false negatives/positives as a first-class risk,
  since a wrong "RDP client missing" warning would itself erode trust in
  `doctor`.
- A reviewer should scrutinize: (1) that indices 0–6 and the
  `sync.WaitGroup` block in `Run` are byte-for-byte unchanged except for the
  slice size bump to 8 and the one new line after `wg.Wait()`; (2) that the
  new check is genuinely `statusWarn` (not `statusFail`) in the merged code;
  (3) that the README wording change doesn't overclaim (it should say "does
  not currently check", not imply full RDP coverage exists).
