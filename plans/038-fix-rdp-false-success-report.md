# Plan 038: Stop `act ec2 rdp` from reporting a working tunnel when `aws ssm start-session` already failed

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/rdp_unix.go internal/aws/rdp_windows.go cmd_ec2.go`
> If either `rdp_unix.go` or `rdp_windows.go` changed since this plan was
> written, compare the "Current state" excerpts below against the live code
> before proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0802c51`, 2026-09-09

## Why this matters

`act ec2 rdp` starts an `aws ssm start-session` subprocess to open a local
port-forwarding tunnel to an instance's RDP port, then unconditionally
sleeps 2 seconds, prints `"RDP available at localhost:<port>"`, and (unless
`--no-open`) launches the platform RDP client — regardless of whether the
subprocess already exited with an error during those 2 seconds. If
`aws ssm start-session` fails fast (bad `--target` instance ID, SSM agent
unreachable, `session-manager-plugin` not installed, etc.), the user is told
the tunnel is ready and their RDP client pops up and fails with a generic
"connection refused" — with no indication that the real cause was an
SSM/AWS-CLI failure whose error message had already printed to stderr
moments earlier. This wastes the user's time chasing the wrong error. After
this fix, a fast subprocess failure is detected before either the success
message or the client launch happens, and the real subprocess error is
surfaced instead.

## Current state

- `internal/aws/rdp_unix.go` — `StartRDP` for all non-Windows platforms
  (`//go:build !windows`). Full current content (16 lines of logic, verified
  fresh at commit `0802c51`):

  ```go
  //go:build !windows

  package aws

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"os/signal"
  	"runtime"
  	"syscall"
  	"time"
  )

  func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
  	document := "AWS-StartPortForwardingSession"
  	params := fmt.Sprintf(`{"portNumber":["3389"],"localPortNumber":["%d"]}`, localPort)

  	args := []string{"ssm", "start-session",
  		"--target", instanceID,
  		"--document-name", document,
  		"--parameters", params,
  	}

  	if profile != "" {
  		args = append(args, "--profile", profile)
  	}
  	if region != "" {
  		args = append(args, "--region", region)
  	}

  	cmd := exec.Command("aws", args...)
  	cmd.Stdout = os.Stdout
  	cmd.Stderr = os.Stderr

  	if err := cmd.Start(); err != nil {
  		return fmt.Errorf("failed to start port forward: %w", err)
  	}

  	// Wait for tunnel to be ready
  	time.Sleep(2 * time.Second)

  	fmt.Printf("\nRDP available at localhost:%d\n", localPort)

  	if openClient {
  		switch runtime.GOOS {
  		case "darwin":
  			url := fmt.Sprintf("rdp://full%%20address=s:localhost:%d", localPort)
  			exec.Command("open", url).Start()
  		default:
  			fmt.Println("Connect with your RDP client to localhost:" + fmt.Sprint(localPort))
  		}
  	}

  	// Wait for ctrl+c or process exit
  	sigCh := make(chan os.Signal, 1)
  	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

  	done := make(chan error, 1)
  	go func() { done <- cmd.Wait() }()

  	select {
  	case <-sigCh:
  		cmd.Process.Signal(syscall.SIGTERM)
  		return nil
  	case err := <-done:
  		return err
  	}
  }
  ```

- `internal/aws/rdp_windows.go` — `StartRDP` for Windows
  (`//go:build windows`). Structurally identical apart from the client-launch
  call. Full current content (verified fresh at commit `0802c51`):

  ```go
  //go:build windows

  package aws

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"os/signal"
  	"syscall"
  	"time"
  )

  func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
  	document := "AWS-StartPortForwardingSession"
  	params := fmt.Sprintf(`{"portNumber":["3389"],"localPortNumber":["%d"]}`, localPort)

  	args := []string{"ssm", "start-session",
  		"--target", instanceID,
  		"--document-name", document,
  		"--parameters", params,
  	}

  	if profile != "" {
  		args = append(args, "--profile", profile)
  	}
  	if region != "" {
  		args = append(args, "--region", region)
  	}

  	cmd := exec.Command("aws", args...)
  	cmd.Stdout = os.Stdout
  	cmd.Stderr = os.Stderr

  	if err := cmd.Start(); err != nil {
  		return fmt.Errorf("failed to start port forward: %w", err)
  	}

  	// Wait for tunnel to be ready
  	time.Sleep(2 * time.Second)

  	fmt.Printf("\nRDP available at localhost:%d\n", localPort)

  	if openClient {
  		exec.Command("mstsc", fmt.Sprintf("/v:localhost:%d", localPort)).Start()
  	}

  	// Wait for ctrl+c or process exit
  	sigCh := make(chan os.Signal, 1)
  	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

  	done := make(chan error, 1)
  	go func() { done <- cmd.Wait() }()

  	select {
  	case <-sigCh:
  		cmd.Process.Signal(syscall.SIGTERM)
  		return nil
  	case err := <-done:
  		return err
  	}
  }
  ```

- The only caller of `StartRDP` is `runRDP` in `/Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/cmd_ec2.go`,
  currently at line 117:

  ```go
  err := aws.StartRDP(instanceID, profile, region, *localPort, !*noOpen)
  ```

  Its signature — `StartRDP(instanceID, profile, region string, localPort int, openClient bool) error` —
  is not changing in this plan, so `cmd_ec2.go` needs no edit and no README
  change is needed either (the CLI's flags and observable usage are
  unchanged; only the internal failure-detection timing changes).

- No other `internal/aws/*_unix.go`/`*_windows.go` pair in this repo has this
  "sleep then unconditionally declare success" pattern — verified by
  grepping `time.Sleep\|cmd.Wait\|available` across `forward_unix.go`,
  `forward_windows.go`, `session_unix.go`, `session_windows.go`,
  `logs_unix.go`, `logs_windows.go`, `ecs_exec_unix.go`,
  `ecs_exec_windows.go`, `ssh_unix.go`, `ssh_windows.go` — zero matches in
  any of them. This bug and its fix are scoped to the RDP files only; do not
  go looking for the same pattern elsewhere as part of this plan.

- Repo testing convention for exec-based code: `internal/doctor/install_test.go`
  (lines 15–27) defines this pattern for getting a portable, reliable
  exit-0/exit-nonzero subprocess in tests without depending on a real
  external binary:

  ```go
  // noopCmd returns a command that runs the current test binary with a
  // run-filter that matches nothing, so it exits 0 immediately. This is
  // portable across the darwin/linux/windows CI matrix, unlike relying on a
  // platform-specific binary such as "true".
  func noopCmd() *exec.Cmd {
  	return exec.Command(os.Args[0], "-test.run=NONE")
  }

  // failingCmd returns a command that reliably exits non-zero on all three
  // CI platforms, by passing the test binary an unrecognized flag.
  func failingCmd() *exec.Cmd {
  	return exec.Command(os.Args[0], "-test.run=NONE", "-nonexistent-flag-xyz")
  }
  ```

  This plan adapts that exact trick (self-exec the test binary with
  `-test.run=NONE`, optionally plus a bogus flag) inside
  `internal/aws/rdp_test.go`, since `internal/doctor`'s helpers are
  unexported and cannot be imported from `internal/aws`.

- CI (`/Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/.github/workflows/ci.yml`)
  runs `go build -v ./...`, `gofmt -l .`, `go vet ./...`, and
  `go test -v ./...` on a matrix of `ubuntu-latest`, `macos-latest`, and
  `windows-latest`. That means both `rdp_unix.go` (built on ubuntu/macos) and
  `rdp_windows.go` (built on windows) get compiled and tested for real in CI
  — this plan's new shared file (Step 1 below) has no build tag, so it
  compiles and its tests run on all three.

## Commands you will need

| Purpose              | Command                                              | Expected on success                    |
|-----------------------|-------------------------------------------------------|------------------------------------------|
| Build                 | `go build ./...`                                      | exit 0                                    |
| Cross-compile Windows | `GOOS=windows go build ./...`                          | exit 0 (checks `rdp_windows.go` compiles even on a non-Windows dev machine; it will not run its tests) |
| Vet                   | `go vet ./...`                                         | exit 0, no output                         |
| Format check          | `gofmt -l .`                                           | no output (empty)                         |
| Full test suite       | `go test ./...`                                        | all pass, `ok` for every package          |
| New tests only        | `go test ./internal/aws/... -run TestRunRDPTunnel -v`  | both new tests print `--- PASS`           |

(Commands verified against this repo's actual `go.mod` — `go 1.26.5` — and
`.github/workflows/ci.yml`, not guessed.)

## Suggested executor toolkit

No special skills needed beyond normal Go editing. If unsure whether a
change is gofmt-clean, run `gofmt -w internal/aws/rdp.go internal/aws/rdp_unix.go internal/aws/rdp_windows.go internal/aws/rdp_test.go`
before the verification commands above rather than hand-formatting.

## Scope

**In scope** (the only files you should create or modify):
- `internal/aws/rdp.go` (new file — shared, no build tag)
- `internal/aws/rdp_unix.go` (rewrite)
- `internal/aws/rdp_windows.go` (rewrite)
- `internal/aws/rdp_test.go` (new file — tests for the new shared logic)

**Out of scope** (do NOT touch, even though they look related):
- `cmd_ec2.go` — `StartRDP`'s signature and call site are unchanged.
- `README.md` — no user-visible flag, command, or usage-string change; the
  `CLAUDE.md` rule to update README after adding a feature/flag does not
  apply here, since this is a bug fix with no new/changed CLI surface.
- Any other `*_unix.go`/`*_windows.go` pair (`forward_*`, `session_*`,
  `logs_*`, `ecs_exec_*`, `ssh_*`) — confirmed in "Current state" above that
  none of them share this bug; touching them is out of scope for this plan.
- `internal/doctor/install_test.go` — read-only reference for the test
  pattern; do not modify it.

## Git workflow

- Branch: `advisor/038-fix-rdp-false-success-report`
- Single commit for this fix is fine (all four files are one logical
  change). Commit message style — match this repo's observed convention
  (imperative, prefixed by type), e.g.:
  `fix: detect fast aws ssm start-session failure before reporting RDP tunnel as ready`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add the shared, testable tunnel logic in a new file `internal/aws/rdp.go`

Create `/Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/internal/aws/rdp.go`
with no `//go:build` tag (it must compile and be tested on all three CI
platforms) and this exact content:

```go
package aws

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// buildRDPSessionCmd builds the "aws ssm start-session" command that opens
// an SSM port-forwarding tunnel from localPort on this machine to port 3389
// (RDP) on instanceID.
func buildRDPSessionCmd(instanceID, profile, region string, localPort int) *exec.Cmd {
	document := "AWS-StartPortForwardingSession"
	params := fmt.Sprintf(`{"portNumber":["3389"],"localPortNumber":["%d"]}`, localPort)

	args := []string{"ssm", "start-session",
		"--target", instanceID,
		"--document-name", document,
		"--parameters", params,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// runRDPTunnel starts cmd (an already-configured "aws ssm start-session"
// port-forwarding command) and waits up to 2 seconds for it to either come
// up or exit early.
//
// If cmd exits within that 2-second window, that is treated as a failure:
// a successful port-forwarding session runs until manually stopped
// (ctrl+c) or until the remote end closes it, so exiting within 2 seconds
// means "aws ssm start-session" itself failed (bad instance ID, SSM agent
// unreachable, session-manager-plugin missing, etc). In that case
// runRDPTunnel returns an error without printing that RDP is available and
// without invoking launchClient.
//
// If the 2-second timer elapses first, the tunnel is presumed up:
// runRDPTunnel prints the "RDP available" message, invokes launchClient
// (unless openClient is false), and then blocks until either an
// interrupt/SIGTERM (in which case it signals cmd to stop and returns nil)
// or cmd exits on its own (in which case cmd's exit error, if any, is
// returned).
func runRDPTunnel(cmd *exec.Cmd, localPort int, openClient bool, launchClient func(localPort int)) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			return fmt.Errorf("aws ssm start-session exited immediately (before the tunnel came up); check the output above for the cause")
		}
		return fmt.Errorf("aws ssm start-session failed: %w", err)
	case <-time.After(2 * time.Second):
		// Tunnel is presumed up; fall through.
	}

	fmt.Printf("\nRDP available at localhost:%d\n", localPort)

	if openClient && launchClient != nil {
		launchClient(localPort)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		cmd.Process.Signal(syscall.SIGTERM)
		return nil
	case err := <-done:
		return err
	}
}
```

**Verify**: `go build ./internal/aws/...` → exit 0 (will fail at this point
because `rdp_unix.go`/`rdp_windows.go` still define their own `StartRDP` and
now also reference the old inline logic — that's expected; this step alone
does not need to compile cleanly in isolation, proceed to Step 2 and verify
the whole package after Step 3).

### Step 2: Rewrite `internal/aws/rdp_unix.go` to use the shared helper

Replace the entire file content with:

```go
//go:build !windows

package aws

import (
	"fmt"
	"os/exec"
	"runtime"
)

func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
	cmd := buildRDPSessionCmd(instanceID, profile, region, localPort)
	return runRDPTunnel(cmd, localPort, openClient, launchRDPClientUnix)
}

func launchRDPClientUnix(localPort int) {
	switch runtime.GOOS {
	case "darwin":
		url := fmt.Sprintf("rdp://full%%20address=s:localhost:%d", localPort)
		exec.Command("open", url).Start()
	default:
		fmt.Println("Connect with your RDP client to localhost:" + fmt.Sprint(localPort))
	}
}
```

**Verify**: `go build ./internal/aws/...` on macOS/Linux → exit 0 (still
expected to fail until Step 3 fixes `rdp_windows.go`'s duplicate-symbol/
unused-import issue — no, actually `rdp_windows.go` is excluded by its build
tag on non-Windows, so this should already succeed after this step on
macOS/Linux; if it does not succeed, re-check this file's content against
the exact code above before proceeding).

### Step 3: Rewrite `internal/aws/rdp_windows.go` to use the shared helper

Replace the entire file content with:

```go
//go:build windows

package aws

import (
	"fmt"
	"os/exec"
)

func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
	cmd := buildRDPSessionCmd(instanceID, profile, region, localPort)
	return runRDPTunnel(cmd, localPort, openClient, launchRDPClientWindows)
}

func launchRDPClientWindows(localPort int) {
	exec.Command("mstsc", fmt.Sprintf("/v:localhost:%d", localPort)).Start()
}
```

**Verify**:
- `go build ./...` → exit 0
- `GOOS=windows go build ./...` → exit 0 (cross-compile check for the
  Windows-only file; this only checks compilation, it cannot run
  `rdp_windows.go`'s code path since it has no Windows tests of its own —
  all runtime testing of the shared logic happens through `rdp_test.go`
  against `rdp.go`, which is platform-agnostic)

### Step 4: Add `internal/aws/rdp_test.go`

Create `/Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/internal/aws/rdp_test.go`
with no build tag and this exact content:

```go
package aws

import (
	"os"
	"os/exec"
	"testing"
)

// fastExitCmd returns a command that runs the current test binary with a
// run-filter that matches nothing, so it exits almost immediately.
// extraArgs lets a case pass an unrecognized flag to force a non-zero exit.
// This mirrors the noopCmd()/failingCmd() pattern in
// internal/doctor/install_test.go, which uses the same trick to get a
// reliable, portable exit-0 or exit-nonzero subprocess without depending on
// a real external binary being present on the darwin/linux/windows CI
// matrix.
func fastExitCmd(extraArgs ...string) *exec.Cmd {
	args := append([]string{"-test.run=NONE"}, extraArgs...)
	return exec.Command(os.Args[0], args...)
}

func TestRunRDPTunnel_SubprocessFailsFast(t *testing.T) {
	cmd := fastExitCmd("-nonexistent-flag-xyz") // exits non-zero almost immediately
	var launched bool
	err := runRDPTunnel(cmd, 13389, true, func(int) { launched = true })
	if err == nil {
		t.Fatal("expected an error when the subprocess exits immediately with a failure, got nil")
	}
	if launched {
		t.Error("expected launchClient not to be called when the subprocess fails fast")
	}
}

func TestRunRDPTunnel_SubprocessExitsCleanFast(t *testing.T) {
	cmd := fastExitCmd() // exits 0 almost immediately
	var launched bool
	err := runRDPTunnel(cmd, 13390, true, func(int) { launched = true })
	if err == nil {
		t.Fatal("expected an error when the subprocess exits immediately, even with exit code 0")
	}
	if launched {
		t.Error("expected launchClient not to be called when the subprocess exits immediately")
	}
}
```

**Verify**: `go test ./internal/aws/... -run TestRunRDPTunnel -v` → both
`--- PASS: TestRunRDPTunnel_SubprocessFailsFast` and
`--- PASS: TestRunRDPTunnel_SubprocessExitsCleanFast` printed, `ok` at the
end, and each test completes in well under 2 seconds (if either test takes
~2+ seconds, the fast-exit subprocess is not actually exiting fast — STOP
and report rather than increasing timeouts).

### Step 5: Full verification pass

Run, in order:
1. `gofmt -l .` → no output
2. `go build ./...` → exit 0
3. `GOOS=windows go build ./...` → exit 0
4. `go vet ./...` → exit 0, no output
5. `go test ./...` → all packages `ok`, none failed
6. `git status` → only `internal/aws/rdp.go` (new), `internal/aws/rdp_unix.go`
   (modified), `internal/aws/rdp_windows.go` (modified),
   `internal/aws/rdp_test.go` (new) show as changed

## Test plan

- New tests, in `internal/aws/rdp_test.go`:
  - `TestRunRDPTunnel_SubprocessFailsFast` — subprocess exits non-zero
    within the 2-second window; asserts `runRDPTunnel` returns a non-nil
    error and never invokes `launchClient`. This is the exact regression
    this plan fixes (previously the code would have slept 2s, printed
    "available", and invoked the client-launch callback regardless).
  - `TestRunRDPTunnel_SubprocessExitsCleanFast` — subprocess exits 0 within
    the 2-second window (an edge case the finding's fix direction implies:
    "necessarily... an error at this point since a successful
    port-forwarding session runs until manually stopped" — so even a clean
    fast exit must be treated as a failure-to-establish, not a success).
- Structural pattern to model after: `internal/doctor/install_test.go`'s
  `noopCmd()`/`failingCmd()` (lines 15–27) — adapted here as `fastExitCmd`.
- **Deliberately not automated**: the "happy path" where the tunnel stays up
  past the 2-second window, `runRDPTunnel` prints "RDP available", and
  `launchClient` is invoked. Simulating a subprocess that reliably stays
  alive for >2 seconds across the darwin/linux/windows CI matrix without a
  real `aws` binary would require either (a) a portable "sleep" helper
  process (Go's standard trick is a `TestMain`-gated helper subprocess that
  checks an env var and blocks — more machinery than this bug fix
  justifies) or (b) parameterizing the 2-second delay just for tests, which
  changes `runRDPTunnel`'s signature further than needed. This is judged
  disproportionate for a fix whose regression signature is specifically
  "fails fast but is reported as success" — that path is fully covered by
  the two tests above. The happy path is unchanged by this fix (it is
  functionally identical to the old code once the timer elapses), so the
  manual verification recipe below is the deliberate substitute for
  automated happy-path coverage.
- Verification: `go test ./...` → all pass, including the 2 new tests, with
  the total `internal/aws` package test time not visibly dominated by these
  two new tests (they must each resolve in well under 2 seconds).

### Manual verification recipe (for the actual bug fix, against real `aws`)

Pick one of these two ways to force `aws ssm start-session` to fail fast,
then run `act ec2 rdp --target <id> --no-open` (keep `--no-open` off only if
you also want to visually confirm no RDP client window pops up):

1. **Nonexistent instance ID** (needs real AWS credentials configured, no
   sandbox required): run
   `act ec2 rdp --target i-000000000000000ff --no-open`
   against a real AWS account/region. `aws ssm start-session` will fail
   fast with something like `An error occurred (TargetNotConnected)` printed
   to stderr by the AWS CLI itself.
   - **Before this fix**: the tool would still print
     `RDP available at localhost:3389` after a 2-second pause.
   - **After this fix**: the tool prints the AWS CLI's error to stderr (via
     `cmd.Stderr = os.Stderr`) and then `act`'s own
     `Error: aws ssm start-session failed: exit status 254` (or similar) to
     stderr, and exits non-zero — it must NOT print
     `RDP available at localhost:...`.
2. **No AWS CLI available** (no AWS credentials needed): temporarily rename
   or shadow `aws` out of `PATH` for one invocation, e.g.
   `PATH=/usr/bin:/bin act ec2 rdp --target i-anything --no-open` on a
   machine where `/usr/bin:/bin` does not contain `aws`. This actually hits
   the earlier `cmd.Start()` error path (`"failed to start port forward"`),
   which was already handled correctly before this fix — so this specific
   variant does not exercise the new code path, but confirms the command
   still fails cleanly. Prefer variant 1 to actually exercise the fix.

Run whichever variant is feasible in your environment and confirm the
"before" vs. "after" behavior described above; there is no way to fully
automate this leg without a real or mocked SSM backend, which is out of
scope for this plan.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `gofmt -l .` produces no output
- [ ] `go build ./...` exits 0
- [ ] `GOOS=windows go build ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `go test ./...` exits 0, all packages `ok`
- [ ] `go test ./internal/aws/... -run TestRunRDPTunnel -v` shows both new
      tests as `PASS`
- [ ] `grep -n "time.Sleep(2" internal/aws/rdp_unix.go internal/aws/rdp_windows.go`
      returns no matches (the flat sleep is gone from both files)
- [ ] `grep -rn "runRDPTunnel" internal/aws/*.go` shows it defined once in
      `rdp.go` and called once each from `rdp_unix.go` and `rdp_windows.go`
- [ ] `git status` shows changes only in `internal/aws/rdp.go`,
      `internal/aws/rdp_unix.go`, `internal/aws/rdp_windows.go`,
      `internal/aws/rdp_test.go`
- [ ] `plans/README.md` status row for plan 038 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The live content of `internal/aws/rdp_unix.go` or `internal/aws/rdp_windows.go`
  does not match the "Current state" excerpts above (the codebase has
  drifted since this plan was written at commit `0802c51`).
- `cmd_ec2.go`'s call to `aws.StartRDP` (currently line 117) has a different
  signature or argument list than
  `aws.StartRDP(instanceID, profile, region, *localPort, !*noOpen)`.
- Any grep in "Current state" for the other `_unix.go`/`_windows.go` pairs
  now DOES find `time.Sleep`/`cmd.Wait`/`available` — meaning another file
  has since grown the same bug pattern and this plan's "out of scope" list
  needs to be reconsidered before proceeding.
- `go vet ./...` or `gofmt -l .` fails to go clean after one reasonable fix
  attempt (e.g. a genuine typo) — do not keep guessing.
- Either new test (`TestRunRDPTunnel_SubprocessFailsFast`,
  `TestRunRDPTunnel_SubprocessExitsCleanFast`) takes anywhere close to 2
  seconds to run — this means `fastExitCmd`'s subprocess isn't exiting as
  fast as assumed on this platform, and the assumption underlying this
  plan's test design needs re-examination rather than being patched around
  with a longer timeout.

## Maintenance notes

- If a future change makes the RDP tunnel's startup detection smarter (e.g.
  actually probing `localhost:<port>` instead of a fixed 2-second timer),
  that logic belongs in `runRDPTunnel` in `internal/aws/rdp.go` — both
  platforms already funnel through it, so a future improvement only needs
  to change one place.
- The manual verification recipe's variant 1 (nonexistent instance ID)
  requires real AWS credentials; if this project ever adds an SSM-mocking
  test harness for other AWS-backed commands, extending it to cover the
  "happy path" gap noted in the Test plan section (tunnel survives past 2
  seconds) would be a reasonable follow-up — not required by this plan.
- A reviewer of this change should scrutinize: (1) that `runRDPTunnel`'s
  first `select` really can't leak the `done` channel goroutine (it can't —
  the channel is buffered size 1 and always eventually written to once by
  `cmd.Wait()`, regardless of which `select` branch elsewhere ends up
  reading it, or if neither ever does); (2) that `launchClient` is only
  ever called after the "still running" branch, never after `done` fires
  first.
- No other command in this repo shares this bug (verified in "Current
  state"), so this plan intentionally does not open a broader audit of
  every `exec.Command`/`cmd.Start()` call site in the codebase.
