# Plan 044: Add a bounded wall-clock deadline to `WaitForCommandInvocation` so SSM polling can't hang forever

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/ssm.go internal/aws/ssh_key_push.go cmd_ssm.go internal/aws/ssm_test.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0802c51`, 2026-09-09

## Why this matters

`internal/aws/ssm.go`'s `WaitForCommandInvocation` polls `aws ssm
get-command-invocation` in an unconditional `for { ... }` loop with no
maximum iteration count and no overall wall-clock deadline of its own — it
only stops when SSM reports a terminal status or the `aws` CLI hard-fails. If
the SSM Agent on the target instance goes offline mid-command (or any other
server-side condition prevents the invocation from ever reaching a terminal
status), this client-side loop has no independent circuit breaker and polls
every 2 seconds forever. Both callers — `act ssm run` (`cmd_ssm.go`) and `act
ec2 ssh --push-key` (`internal/aws/ssh_key_push.go`) — would hang
indefinitely with no way out except Ctrl-C, and with no indication to the
user that the command is stuck versus still legitimately running. After this
plan lands, `WaitForCommandInvocation` is guaranteed to return within a
bounded time, with a clear "timed out" error that is distinguishable from an
AWS CLI failure or a command that finished with a non-Success status.

## Current state

- `internal/aws/ssm.go` — defines `SendCommand` (submits the SSM Run Command,
  taking a `timeoutSeconds int` that is SSM's own *server-side* timeout for
  the invocation) and `WaitForCommandInvocation` (polls until terminal). This
  is the file to change for the core fix.
- `cmd_ssm.go` — implements `act ssm run` (`runSSMRun`); one of the two
  callers of `WaitForCommandInvocation`.
- `internal/aws/ssh_key_push.go` — implements `act ec2 ssh --push-key`
  (`PushSSHKeyViaSSM`); the other caller of `WaitForCommandInvocation`.
- `internal/aws/ssm_test.go` — existing test file for this package; add the
  new tests here.
- `internal/doctor/install_test.go` — exemplar for making `exec.Command`-based
  code testable in this repo (see "Testability decision" below).

### `internal/aws/ssm.go` today (lines 84–127)

```go
// WaitForCommandInvocation polls ssm get-command-invocation until the
// command reaches a terminal status (Success, Failed, Cancelled, TimedOut).
func WaitForCommandInvocation(commandID, instanceID, profile, region string, pollInterval time.Duration) (CommandInvocationResult, error) {
	args := []string{"ssm", "get-command-invocation",
		"--command-id", commandID,
		"--instance-id", instanceID,
		"--output", "json",
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	for {
		cmd := exec.Command("aws", args...)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return CommandInvocationResult{}, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return CommandInvocationResult{}, err
		}

		var result commandInvocationOutput
		if err := json.Unmarshal(out, &result); err != nil {
			return CommandInvocationResult{}, fmt.Errorf("failed to parse get-command-invocation output: %w", err)
		}

		switch result.Status {
		case "Pending", "InProgress", "Delayed":
			time.Sleep(pollInterval)
			continue
		default:
			return CommandInvocationResult{
				Status:   result.Status,
				Stdout:   result.StandardOutputContent,
				Stderr:   result.StandardErrorContent,
				ExitCode: result.ResponseCode,
			}, nil
		}
	}
}
```

`SendCommand`'s signature (`internal/aws/ssm.go:28`), for reference to the
existing server-side timeout value this plan will derive the new deadline
from:

```go
func SendCommand(instanceID, profile, region, document string, commands []string, timeoutSeconds int, comment string) (string, error) {
```

### Call site 1 — `cmd_ssm.go` (confirmed, current lines 21, 65, 76)

```go
21:	timeout := fs.Int("timeout", 300, "Command timeout in seconds")
...
65:	commandID, err := aws.SendCommand(instanceID, profile, region, document, commands, *timeout, *comment)
...
76:	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
```

`*timeout` is a user-configurable `--timeout` flag (default 300s) already
threaded into `SendCommand`. This is the value to derive the new deadline
from at this call site.

### Call site 2 — `internal/aws/ssh_key_push.go` (confirmed, current lines 78, 83)

```go
78:	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
...
83:	result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
```

Here the `SendCommand` timeout is a hardcoded literal `60` (seconds), not a
flag. There is no `--timeout`-style flag for `act ec2 ssh --push-key` and
this plan does not add one.

### Deadline design decision (read this before coding)

- **Parameter style**: plain `time.Duration`, not `context.Context`.
  Confirmed by search: `grep -rn "context\." internal/aws internal/doctor`
  returns nothing — the `context` package is not used anywhere in this
  codebase's `internal/` packages. Introducing `context.Context` here would
  be the *first* use of it in the project and a bigger, inconsistent
  signature change. A plain `time.Duration` parameter matches the existing
  style of `pollInterval time.Duration` on the very same function.
- **Where the deadline value comes from**: derive it from the SSM command's
  own server-side `--timeout-seconds` (the value already passed to
  `SendCommand`), rather than an independent constant or a new CLI flag.
  Rationale: SSM itself is supposed to transition the invocation to a
  terminal status (e.g. `"TimedOut"`) once `timeoutSeconds` elapses — the
  whole point of this bug is guarding against the case where that
  server-side enforcement never fires (e.g. SSM Agent goes offline). Doubling
  `timeoutSeconds` gives comfortable headroom for polling latency/clock skew
  over the expected terminal transition, while still guaranteeing the client
  gives up on its own. This is implemented as a new small helper function
  `MaxWaitFromTimeoutSeconds` in `internal/aws/ssm.go` (see Step 1), floored
  at 1 minute (so short `--timeout` values still get a workable margin) and
  capped at 30 minutes (so a very large `--timeout` doesn't leave the client
  stuck for hours if SSM's enforcement silently fails). No new CLI flag is
  added — the deadline is derived automatically at both call sites, so this
  is not a user-facing feature addition and does not require a README update
  under this repo's "update README after adding a flag/feature" rule.
- **Testability decision**: `internal/doctor/install_test.go` is this repo's
  exemplar for making `exec.Command`-based code testable — it defines
  `noopCmd()`/`failingCmd()` helpers that run `exec.Command(os.Args[0],
  "-test.run=NONE", ...)` (the test binary itself, for a portable, reliable
  exit-0/exit-nonzero without depending on an external binary), and the
  function under test (`runDownloadAndInstall`) takes an injectable `buildCmd
  func(...) *exec.Cmd` parameter specifically to enable this.
  `WaitForCommandInvocation` has no such injection point today, and adding
  one (threading an injectable `aws`-command builder through both call
  sites) would be a materially larger refactor than this bug fix warrants —
  it is explicitly **not** done here, to keep this plan at Effort M. Instead,
  Step 1 places the deadline check as the **first thing inside the loop**,
  before the `exec.Command("aws", ...)` call. This means a test can pass a
  `maxWait` that is already in the past (e.g. `-1 * time.Second`) and the
  real, unmodified `WaitForCommandInvocation` function will return the
  timeout error immediately, without ever shelling out to `aws` — no `aws`
  binary needs to be installed or mocked, and the test is deterministic and
  fast. This gives real (not just unit-level) coverage of the timeout path
  in the actual exported function, without needing an injectable runner.
  This is the smaller of the two options presented for this fix, and it is
  disproportionate to add the injectable-runner refactor purely for this.

## Commands you will need

| Purpose   | Command                                | Expected on success              |
|-----------|-----------------------------------------|-----------------------------------|
| Build     | `go build ./...`                        | exit 0, no output                |
| Vet       | `go vet ./...`                          | exit 0, no output                |
| Format    | `gofmt -l .`                            | exit 0, no output (no files listed) |
| Tests     | `go test ./...`                         | all pass (baseline: 183 passed in 6 packages) |
| Tests (pkg)| `go test ./internal/aws/... -run 'MaxWaitFromTimeoutSeconds|WaitForCommandInvocation' -v` | new tests pass, printed `--- PASS` for each |

(All four verified clean at the "Planned at" commit before this plan was written.)

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ssm.go` — add `MaxWaitFromTimeoutSeconds`, add `maxWait
  time.Duration` parameter to `WaitForCommandInvocation`, add the deadline
  check and timeout error.
- `internal/aws/ssm_test.go` — add `TestMaxWaitFromTimeoutSeconds` and
  `TestWaitForCommandInvocation_TimesOutWhenDeadlineAlreadyPassed`.
- `cmd_ssm.go` — update the `WaitForCommandInvocation` call site to compute
  and pass `maxWait`.
- `internal/aws/ssh_key_push.go` — update the `WaitForCommandInvocation` call
  site to compute and pass `maxWait`; extract the hardcoded `60` into a named
  constant used by both the `SendCommand` and `MaxWaitFromTimeoutSeconds`
  calls so they can't drift apart.
- `plans/README.md` — update this plan's status row when done.

**Out of scope** (do NOT touch, even though related):
- Any retry-on-transient-error logic for `InvocationDoesNotExist` or other
  AWS-side transient errors right after `SendCommand`. This requires
  live-account verification of AWS SSM's actual transient-error behavior
  that cannot be done from this repo alone. It is a documented follow-up
  idea in "Maintenance notes" below, not part of this plan.
- Adding a new `--wait-timeout`-style CLI flag to `act ssm run` or `act ec2
  ssh --push-key`. The deadline is derived automatically from the existing
  `--timeout` value / hardcoded `60`; no new flag is introduced.
- Threading an injectable `aws`-command builder/runner through
  `WaitForCommandInvocation` (see "Testability decision" above) — explicitly
  deferred, not part of this plan.
- README.md — no user-facing flag or behavior is added (the timeout error
  message is new output on an already-rare failure path, not a documented
  flag), so no README update is required by this plan. If the reviewer
  disagrees, that is a judgment call for them, not this plan's executor.
- `internal/aws/ssm.go`'s `SendCommand` function itself — unchanged.

## Git workflow

- Branch: `advisor/044-add-ssm-wait-deadline` (matches this repo's observed
  convention, e.g. `advisor/036-ec2-ssh-push-key-instance-connect`).
- Commit per logical step (e.g. one commit for the `ssm.go` core change +
  tests, one for the two call-site updates), or a single commit if you
  prefer — this repo's history shows both styles. Message style: short
  imperative subject, e.g. `fix: bound WaitForCommandInvocation with a
  client-side deadline` (matches `fix: push --push-key via SSM Run Command
  instead of EC2 Instance Connect` from `git log`).
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add `MaxWaitFromTimeoutSeconds` and the deadline check in `internal/aws/ssm.go`

1a. Add this new function anywhere in `internal/aws/ssm.go` (e.g. directly
above `WaitForCommandInvocation`):

```go
// MaxWaitFromTimeoutSeconds derives a client-side polling deadline for
// WaitForCommandInvocation from the --timeout-seconds value passed to
// SendCommand for the same command. SSM itself should transition the
// invocation to a terminal status (e.g. "TimedOut") once timeoutSeconds
// elapses; doubling it gives headroom for polling latency before the client
// independently gives up, which matters if SSM's own enforcement never
// fires (e.g. the SSM Agent on the instance goes offline mid-command). The
// result is floored at 1 minute and capped at 30 minutes.
func MaxWaitFromTimeoutSeconds(timeoutSeconds int) time.Duration {
	d := time.Duration(timeoutSeconds) * 2 * time.Second
	if d < time.Minute {
		return time.Minute
	}
	if d > 30*time.Minute {
		return 30 * time.Minute
	}
	return d
}
```

1b. Change `WaitForCommandInvocation`'s signature from:

```go
func WaitForCommandInvocation(commandID, instanceID, profile, region string, pollInterval time.Duration) (CommandInvocationResult, error) {
```

to:

```go
func WaitForCommandInvocation(commandID, instanceID, profile, region string, pollInterval, maxWait time.Duration) (CommandInvocationResult, error) {
```

1c. Inside the function, immediately after the `args` slice is built (right
before the `for {` loop), add:

```go
	deadline := time.Now().Add(maxWait)
	lastStatus := "Unknown"
```

1d. Change the loop body so the deadline is checked **first**, before the
`exec.Command` call, and `lastStatus` is recorded on every successful poll.
The full loop should become:

```go
	for {
		if time.Now().After(deadline) {
			return CommandInvocationResult{}, fmt.Errorf("timed out after %s waiting for command %s to complete (last status: %s)", maxWait, commandID, lastStatus)
		}

		cmd := exec.Command("aws", args...)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return CommandInvocationResult{}, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return CommandInvocationResult{}, err
		}

		var result commandInvocationOutput
		if err := json.Unmarshal(out, &result); err != nil {
			return CommandInvocationResult{}, fmt.Errorf("failed to parse get-command-invocation output: %w", err)
		}
		lastStatus = result.Status

		switch result.Status {
		case "Pending", "InProgress", "Delayed":
			time.Sleep(pollInterval)
			continue
		default:
			return CommandInvocationResult{
				Status:   result.Status,
				Stdout:   result.StandardOutputContent,
				Stderr:   result.StandardErrorContent,
				ExitCode: result.ResponseCode,
			}, nil
		}
	}
```

Also update the doc comment above the function to mention the new deadline
behavior, e.g.:

```go
// WaitForCommandInvocation polls ssm get-command-invocation until the
// command reaches a terminal status (Success, Failed, Cancelled, TimedOut),
// or until maxWait elapses, in which case it returns a "timed out" error
// rather than polling forever.
```

**Verify**: `go build ./...` → exit 0. (This will fail until Steps 2 and 3
also update the call sites, since the signature changed — build all three
files together before verifying, or expect two "not enough arguments"
compile errors until Steps 2–3 are done.)

### Step 2: Update the `act ssm run` call site in `cmd_ssm.go`

Change (current lines 65–76):

```go
	commandID, err := aws.SendCommand(instanceID, profile, region, document, commands, *timeout, *comment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Command %s submitted to %s\n", commandID, instanceID)

	if *noWait {
		return
	}

	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
```

to:

```go
	commandID, err := aws.SendCommand(instanceID, profile, region, document, commands, *timeout, *comment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Command %s submitted to %s\n", commandID, instanceID)

	if *noWait {
		return
	}

	maxWait := aws.MaxWaitFromTimeoutSeconds(*timeout)
	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second, maxWait)
```

**Verify**: `go build ./...` → exit 0, no output.

### Step 3: Update the `act ec2 ssh --push-key` call site in `internal/aws/ssh_key_push.go`

Change (current lines 78, 83) from:

```go
	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
	if err != nil {
		return "", fmt.Errorf("sending push-key command: %w", err)
	}

	result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
```

to (introduce a named constant so the `SendCommand` timeout and the derived
deadline can't drift apart):

```go
	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), pushKeyTimeoutSeconds, "act ec2 ssh --push-key")
	if err != nil {
		return "", fmt.Errorf("sending push-key command: %w", err)
	}

	result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second, MaxWaitFromTimeoutSeconds(pushKeyTimeoutSeconds))
```

Add the constant near the top of the file (e.g. just below the `import`
block), matching the existing top-level-declaration style in this file:

```go
// pushKeyTimeoutSeconds is the SSM --timeout-seconds used for the push-key
// command; also used to derive WaitForCommandInvocation's client-side
// deadline via MaxWaitFromTimeoutSeconds, so the two values can't drift
// apart.
const pushKeyTimeoutSeconds = 60
```

**Verify**: `go build ./...` → exit 0, no output.

### Step 4: Add tests in `internal/aws/ssm_test.go`

Append to the existing file (do not remove `TestDocumentForPlatform`):

```go
func TestMaxWaitFromTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name           string
		timeoutSeconds int
		want           time.Duration
	}{
		{name: "small timeout floored to 1 minute", timeoutSeconds: 10, want: time.Minute},
		{name: "normal timeout doubled", timeoutSeconds: 300, want: 10 * time.Minute},
		{name: "large timeout capped at 30 minutes", timeoutSeconds: 5000, want: 30 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxWaitFromTimeoutSeconds(tt.timeoutSeconds)
			if got != tt.want {
				t.Errorf("MaxWaitFromTimeoutSeconds(%d) = %s, want %s", tt.timeoutSeconds, got, tt.want)
			}
		})
	}
}

func TestWaitForCommandInvocation_TimesOutWhenDeadlineAlreadyPassed(t *testing.T) {
	start := time.Now()
	result, err := WaitForCommandInvocation("cmd-1", "i-1", "", "", time.Millisecond, -1*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Errorf("expected error to contain %q, got: %v", "timed out after", err)
	}
	if !strings.Contains(err.Error(), "cmd-1") {
		t.Errorf("expected error to reference the command ID, got: %v", err)
	}
	if result != (CommandInvocationResult{}) {
		t.Errorf("expected zero-value result on timeout, got: %+v", result)
	}
	if elapsed > time.Second {
		t.Errorf("expected the already-past-deadline case to return almost immediately without shelling out to aws, took %s", elapsed)
	}
}
```

This second test relies on `maxWait` being negative so `time.Now().After(deadline)`
is true on the very first loop iteration, before `exec.Command("aws", ...)`
is ever invoked — no `aws` binary needs to be present in the test
environment (see "Testability decision" in Current state). Add `"strings"`
to the test file's imports if not already present (check first; `strings`
may need to be added alongside the existing `"testing"` import).

**Verify**: `go test ./internal/aws/... -run 'TestMaxWaitFromTimeoutSeconds|TestWaitForCommandInvocation_TimesOutWhenDeadlineAlreadyPassed' -v` → both tests print `PASS`, overall `ok`.

## Test plan

- `TestMaxWaitFromTimeoutSeconds` (`internal/aws/ssm_test.go`, new): covers
  the floor (small timeout), normal doubling, and ceiling (large timeout)
  cases of the new pure helper.
- `TestWaitForCommandInvocation_TimesOutWhenDeadlineAlreadyPassed`
  (`internal/aws/ssm_test.go`, new): covers the actual timeout path in the
  real, unmodified `WaitForCommandInvocation` function, asserting the error
  message, the zero-value result, and that it returns in well under a
  second — real, not just unit-level, coverage of the fix's core behavior.
- Structural pattern to follow: table-driven subtests as in the existing
  `TestDocumentForPlatform` in the same file, and the `noopCmd`/`failingCmd`
  exec-avoidance idea (though not the injectable-builder machinery itself)
  from `internal/doctor/install_test.go`.
- Full verification: `go test ./...` → all pass, including the 2 new test
  functions (3 new subtests for `TestMaxWaitFromTimeoutSeconds` plus 1 for
  the timeout test), on top of the pre-existing 183.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` exits 0 with no files listed
- [ ] `go test ./...` exits 0; output shows more tests passing than the
      pre-change baseline of 183
- [ ] `grep -n "maxWait time.Duration" internal/aws/ssm.go` matches the
      `WaitForCommandInvocation` signature
- [ ] `grep -n "MaxWaitFromTimeoutSeconds" cmd_ssm.go internal/aws/ssh_key_push.go`
      shows both call sites now compute and pass a `maxWait`
- [ ] `grep -n "func WaitForCommandInvocation" internal/aws/ssm.go` shows
      exactly one definition, with 6 parameters (`commandID, instanceID,
      profile, region string, pollInterval, maxWait time.Duration`)
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 044 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ssm.go:84-127`, `cmd_ssm.go:65-76`, or
  `internal/aws/ssh_key_push.go:78-83` doesn't match the excerpts in
  "Current state" (the codebase has drifted since this plan was written).
- `go build ./...` still fails after Steps 1–3 are all applied together —
  re-check that every caller of `WaitForCommandInvocation` was updated (there
  should be exactly 2: `cmd_ssm.go` and `internal/aws/ssh_key_push.go` — a
  fresh `grep -rn "WaitForCommandInvocation(" --include="*.go" .` from the
  repo root, excluding any `.claude/worktrees/` paths, should show exactly
  these 2 call sites plus the 1 definition).
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching a file outside the "In scope" list.
- You discover that `context` actually is used elsewhere in `internal/aws` or
  `internal/doctor` (contradicting the "Deadline design decision" section) —
  re-evaluate whether `context.Context` would now be the better fit before
  proceeding with the `time.Duration` approach.

## Maintenance notes

- **Follow-up explicitly deferred out of this plan**: retry-on-transient-error
  handling for AWS SSM's `InvocationDoesNotExist` error, which can occur in
  the brief window right after `SendCommand` returns, before the invocation
  record has propagated on AWS's side. This plan's `MaxWaitFromTimeoutSeconds`
  deadline is a coarse backstop and does not address that narrower race; a
  future plan could add a short bounded retry specifically for that error
  code on the *first* poll only, but doing so correctly requires verifying
  AWS SSM's actual observed transient-error behavior against a live account,
  which could not be done from static repo inspection alone.
- **What a reviewer should scrutinize**: whether doubling `timeoutSeconds`
  (with the 1-minute floor / 30-minute ceiling) is a reasonable default for
  this tool's actual usage patterns — e.g. if `act ssm run --script` is
  commonly used with very long-running deploy scripts and a large
  `--timeout`, confirm the 30-minute cap doesn't cut off legitimately
  long-running-but-healthy commands. If it does, the cap (not the doubling
  logic) is the value to revisit.
- **What future changes interact with this**: any new caller of
  `WaitForCommandInvocation` must now pass a `maxWait` — there is no default,
  so a compile error will force any new call site to make an explicit choice
  (good). If a `--wait-timeout` flag is ever added as a user-facing override,
  it should short-circuit `MaxWaitFromTimeoutSeconds` rather than compose
  with it, to avoid the deadline math becoming another moving part.
