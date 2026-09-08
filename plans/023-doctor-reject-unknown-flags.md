# Plan 023: Make `act doctor` reject unknown/misspelled flags

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go main_test.go`
> If either in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`act doctor` silently accepts any flag, valid or not. A user who runs `act
doctor --totally-bogus-flag` or mistypes `act doctor --fixx` gets a
completely normal doctor pass and exit code 0 — no error, no warning, no
indication their flag was ignored. This was verified live: both commands
run the full check suite and print `All checks passed!` with **exit code
0**, as if the flag had never been typed. Every other subcommand in this
CLI (`forward`, `ecs`, `rds`, `ssm run`, `logs`, `ec2 ssh`, `ec2 rdp`) uses
Go's `flag.FlagSet` with `flag.ExitOnError`, which automatically prints
`flag provided but not defined: -whatever` to stderr and exits 2 on an
unknown flag — verified live: `act forward --bogus-flag` exits **2** with
that exact message. `doctor` is the one command that doesn't follow this
convention; it hand-rolls its own tiny flag loop that quietly collects
anything it doesn't recognize into a variable (`doctorArgs`) that is never
read again. This plan brings `doctor` in line with the rest of the CLI so
a mistyped flag produces a clear, immediate error instead of a silently
wrong result.

## Current state

- `main.go` — the `case "doctor":` dispatch block, at approximately lines
  152–176 (confirm the line numbers with `grep -n 'case "doctor"' main.go`
  before editing, in case the file has shifted). Exact code as of commit
  `aa50614`:

  ```go
  case "doctor":
      subArgs := args[1:]
      if hasHelp(subArgs) {
          printDoctorHelp()
          os.Exit(0)
      }
      fix := false
      skipConfirm := false
      var doctorArgs []string
      for _, a := range subArgs {
          switch a {
          case "--fix":
              fix = true
          case "--skip-confirm":
              skipConfirm = true
          default:
              doctorArgs = append(doctorArgs, a)
          }
      }
      if skipConfirm && !fix {
          fmt.Fprintln(os.Stderr, "Error: --skip-confirm has no effect without --fix")
          os.Exit(1)
      }
      doctor.Run(profile, region, version, fix, skipConfirm)
      os.Exit(0)
  ```

  Confirm the dead-code claim yourself: `grep -n "doctorArgs" main.go` must
  show exactly two lines — the `var doctorArgs []string` declaration and the
  `doctorArgs = append(doctorArgs, a)` inside the loop. No third reference
  exists; the collected value is discarded. This is the bug: any flag other
  than exactly `--fix` or `--skip-confirm` is silently swallowed.

- Live verification performed during planning (commit `aa50614`, binary
  built with `go build -o /tmp/act_verify .`):
  - `act doctor --totally-bogus-flag` → **exit 0**, stdout shows the full
    normal check output ending in `All checks passed!`, stderr empty.
  - `act doctor --fixx` (a plausible typo of `--fix`) → **exit 0**, same
    normal output, stderr empty, and note **`--fix`'s actual remediation
    behavior did NOT run** — the typo silently produced a no-op doctor pass
    instead of either fixing anything or erroring.
  - By contrast, `act forward --bogus-flag` (an existing `flag.FlagSet`
    based subcommand) → **exit 2**, stderr:
    ```
    flag provided but not defined: -bogus-flag
    Usage of forward:
      -local-port int
        	Local port for forwarding
      -remote-host string
        	Remote host for forwarding
      -remote-port int
        	Remote port for forwarding
      -target string
        	Target instance ID (skip instance picker)
    ```

- `main.go:653-656` — `runForward`, the pattern every other subcommand
  follows (also see `runECS` at `main.go:696-700`, `runRDS` at
  `main.go:748-753`, `runSSMRun` at `main.go:825-832`, `runLogs` at
  `main.go:903-909`):
  ```go
  func runForward(profile, region string, subArgs []string) {
      subArgs, tags := parseTags(subArgs)

      fs := flag.NewFlagSet("forward", flag.ExitOnError)
      localPort := fs.Int("local-port", 0, "Local port for forwarding")
      remotePort := fs.Int("remote-port", 0, "Remote port for forwarding")
      target := fs.String("target", "", "Target instance ID (skip instance picker)")
      remoteHost := fs.String("remote-host", "", "Remote host for forwarding")
      fs.Parse(subArgs)
      ...
  ```
  Every one of these calls `flag.NewFlagSet("<name>", flag.ExitOnError)`,
  declares each flag with `fs.Bool`/`fs.String`/`fs.Int`, then `fs.Parse(subArgs)`.
  `doctor` is the only subcommand that does NOT follow this pattern — it is
  the outlier, not the rest of the codebase.

- `main.go:5` already imports `"flag"` (used by six other subcommands), so
  no new import is needed.

- **Decision — Option A (use `flag.NewFlagSet`), not Option B (hand-rolled
  error branch)**: Adopt the exact same `flag.NewFlagSet("doctor",
  flag.ExitOnError)` pattern used by every other subcommand. This means an
  unknown flag to `doctor` will now exit with code **2** (matching stdlib
  `flag` package behavior), not code 1. This is a deliberate, verified
  choice: code 2 is what every other `flag.FlagSet`-based subcommand in
  this codebase already produces for a bad flag (confirmed above with
  `forward --bogus-flag` → exit 2), so switching `doctor` to the same
  mechanism makes it MORE consistent with the rest of the CLI, not less.
  Do not attempt Option B (hand-rolling an `else` branch that calls
  `os.Exit(1)`) — it would leave `doctor` as the only subcommand with a
  bespoke, differently-numbered error path for the same class of mistake.

- The `--skip-confirm` without `--fix` validation (`if skipConfirm && !fix
  { ... os.Exit(1) }`) is a real, correct, semantic check — it is not part
  of the bug and must be preserved unchanged, including its `os.Exit(1)`
  exit code (this checks a valid-but-nonsensical flag *combination*, which
  is a different concern from unknown-flag rejection).

- `main.go:496-522` — `printDoctorHelp()`. Verified this plan's fix does
  NOT require changing this text: the flags it documents (`--fix`,
  `--skip-confirm`) are unchanged; only how unrecognized flags are handled
  changes. Do not edit `printDoctorHelp()`.

- `main_test.go` — confirmed via `grep -n "doctorArgs\|doctor.*flag"
  main_test.go` that no existing test exercises the doctor flag-parsing
  loop (zero matches). `main_test.go:118-144` (`TestSubcommandNeedsAWSCLI`)
  and `main_test.go:210-238` (`TestHelpFunctionsContainDocumentedFlags`)
  test adjacent things but not flag parsing itself — this plan adds the
  missing coverage.

- `main_test.go` currently has no test that invokes the compiled binary as
  a subprocess (no `exec.Command` on `os.Args[0]`, no `TestMain` helper-process
  pattern) — confirmed via `grep -rn "exec.Command" main_test.go` (no
  matches). Because the code under test calls `os.Exit()` directly (inside
  `main()`'s `case "doctor":` block, which is not itself a separately
  callable function), the only way to test the new behavior end-to-end is
  to build the binary and run it as a subprocess in the test. This repo has
  no existing pattern for that in `main_test.go`, so this plan introduces
  one, following Go's standard "build and exec the test binary" approach
  used commonly for CLI `main()` testing. See Step 3 for the exact shape.

## Commands you will need

| Purpose      | Command                                   | Expected on success        |
|--------------|--------------------------------------------|-----------------------------|
| Build        | `go build -v ./...`                        | exit 0                      |
| Vet          | `go vet ./...`                             | exit 0, no output           |
| Format check | `gofmt -l .`                               | exit 0, no output (no files listed) |
| Test         | `go test -v ./...`                         | all pass, "112 passed" baseline plus new tests |
| Manual verify | `go build -o /tmp/act_plan023 . && /tmp/act_plan023 doctor --bogus-flag; echo $?` | prints `flag provided but not defined: -bogus-flag` to stderr, exit code `2` |

## Scope

**In scope** (the only files you should modify):
- `main.go` — the `case "doctor":` dispatch block only (approx. lines
  152–176; do not touch anything else in `main.go`)
- `main_test.go` — add new characterization tests (create new test
  function(s); do not modify existing tests)

**Out of scope** (do NOT touch, even though they look related):
- `internal/doctor/doctor.go` — `doctor.Run()` itself is correct and
  unaffected; this bug is purely in `main.go`'s argument parsing before
  `doctor.Run()` is ever called.
- `printDoctorHelp()` (`main.go:496-522`) — its documented flags
  (`--fix`, `--skip-confirm`) are unchanged by this fix; do not edit it.
- `README.md` — the CLAUDE.md rule to update README after adding a
  feature/flag/command does not apply here: no new flag, command, or
  behavior-visible-in-`--help` is being added. This plan only makes an
  *existing* invalid input (an unknown flag) fail loudly instead of
  silently. If, after implementing, you find the README's doctor examples
  (lines ~194-200) or its flag table no longer match `act doctor --help`
  output, note the specific mismatch in your final report — but do not
  edit README.md as part of this plan unless the mismatch is a direct
  result of a change you made in Step 1 (it should not be, since flag
  names/usage text are unchanged).
- Any other `case "..."` block in `main.go`'s switch statement.
- Any other subcommand's flag parsing (`runForward`, `runECS`, etc.) —
  those are already correct and are the pattern to copy, not files to edit.

## Git workflow

- Branch: `advisor/023-doctor-reject-unknown-flags`
- Commit message style: Conventional Commits, matching repo history, e.g.
  `fix: let act doctor run without AWS CLI already installed` (from
  `git log --oneline`, commit `0f142b4`). Use a `fix:` prefix for this
  change, e.g. `fix: reject unknown flags in act doctor`.
- One commit for the `main.go` change, one for the `main_test.go` test
  additions is fine, or combine into a single commit — either is
  acceptable since this is a small, single-purpose fix.
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Replace the hand-rolled flag loop with `flag.NewFlagSet`

In `main.go`, inside `case "doctor":`, replace this block:

```go
		fix := false
		skipConfirm := false
		var doctorArgs []string
		for _, a := range subArgs {
			switch a {
			case "--fix":
				fix = true
			case "--skip-confirm":
				skipConfirm = true
			default:
				doctorArgs = append(doctorArgs, a)
			}
		}
		if skipConfirm && !fix {
```

with:

```go
		fs := flag.NewFlagSet("doctor", flag.ExitOnError)
		fixFlag := fs.Bool("fix", false, "Attempt to automatically fix failing checks")
		skipConfirmFlag := fs.Bool("skip-confirm", false, "With --fix, run every available fix without prompting")
		fs.Parse(subArgs)
		fix := *fixFlag
		skipConfirm := *skipConfirmFlag
		if skipConfirm && !fix {
```

Leave the rest of the block (the `os.Exit(1)` error for `--skip-confirm`
without `--fix`, and the `doctor.Run(...)` / `os.Exit(0)` lines) exactly as
they are — do not change them. The variable names `fix` and `skipConfirm`
must remain identical afterward, since they're referenced unchanged by
`doctor.Run(profile, region, version, fix, skipConfirm)` two lines below.

This mirrors `runForward`'s pattern at `main.go:656-661` exactly, just
inline in the `case` block instead of inside a separate `run*` function
(the `doctor` case doesn't have a separate `runDoctor` function — the
flag parsing has always lived directly in the `switch` case, unlike other
subcommands; keep that structure, just swap the parsing mechanism).

**Verify**:
```
go build -v ./... 2>&1
```
Expected: exit 0, no compile errors. (`doctorArgs` is now gone — if you see
`declared and not used`, you missed removing a leftover reference; there
should be none since it was never read.)

### Step 2: Manually verify the new behavior

```
go build -o /tmp/act_plan023 .
/tmp/act_plan023 doctor --bogus-flag; echo "exit=$?"
```
Expected stderr: `flag provided but not defined: -bogus-flag` followed by
a `Usage of doctor:` block listing `-fix` and `-skip-confirm`. Expected
exit code: `2`.

```
/tmp/act_plan023 doctor --fix --skip-confirm; echo "exit=$?"
```
Expected: normal doctor pass proceeds with fix mode active (same as
before this change) — this combination must still work, exit code reflects
whatever the check results are (0 if all pass), not an error.

```
/tmp/act_plan023 doctor --skip-confirm; echo "exit=$?"
```
Expected: `Error: --skip-confirm has no effect without --fix` on stderr,
exit code `1` (unchanged from before — this validation still runs after
`fs.Parse`).

```
/tmp/act_plan023 doctor --help; echo "exit=$?"
```
Expected: identical help text to before this change (the `hasHelp(subArgs)`
check runs before the flag parsing changed in Step 1, so this path is
untouched), exit code `0`.

Clean up: `rm -f /tmp/act_plan023`

### Step 3: Add characterization tests in `main_test.go`

There is no existing pattern in this file for invoking the compiled binary
as a subprocess. Add one, following this shape (model it after the
existing `TestHelpFunctionsContainDocumentedFlags` test's use of
`captureStderr` at `main_test.go:195-208`, but since `case "doctor":` calls
`os.Exit()` and lives inside `main()`, you cannot call it directly from a
test in-process — build and exec the binary instead):

```go
func TestDoctorRejectsUnknownFlags(t *testing.T) {
	bin := buildTestBinary(t)

	tests := []struct {
		name       string
		args       []string
		wantExit   int
		wantStderr string
	}{
		{
			name:       "unknown flag is rejected",
			args:       []string{"doctor", "--totally-bogus-flag"},
			wantExit:   2,
			wantStderr: "flag provided but not defined: -totally-bogus-flag",
		},
		{
			name:       "misspelled --fix is rejected",
			args:       []string{"doctor", "--fixx"},
			wantExit:   2,
			wantStderr: "flag provided but not defined: -fixx",
		},
		{
			name:       "skip-confirm without fix still errors with exit 1",
			args:       []string{"doctor", "--skip-confirm"},
			wantExit:   1,
			wantStderr: "Error: --skip-confirm has no effect without --fix",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			err := cmd.Run()

			exitCode := 0
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("failed to run binary: %v", err)
			}

			if exitCode != tt.wantExit {
				t.Errorf("exit code = %d, want %d (stderr: %s)", exitCode, tt.wantExit, stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func buildTestBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := dir + "/act_test_bin"
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build test binary: %v\n%s", err, out)
	}
	return bin
}
```

Add `"os/exec"` to the import block at the top of `main_test.go` (it is
not currently imported there — confirm with `grep -n '"os/exec"'
main_test.go` before adding, to avoid a duplicate-import compile error).

Note: `--skip-confirm without --fix` needs a valid AWS profile/region
context to reach the validation check — verify during Step 4 whether it
requires network/AWS access or fails before that point. If the test is
flaky in a sandboxed CI environment (e.g. it depends on AWS credentials
being resolvable), that specific subtest may need `t.Skip()` with a
comment explaining why — but only do this if Step 4's verification shows
it's actually necessary; the `--skip-confirm`/`--fix` check happens in
`main.go` before any AWS call, so it should NOT require credentials.

**Verify**: `go vet ./...` → exit 0, no errors (confirms the new test
compiles and imports are correct).

### Step 4: Run the full test suite

```
go test -v ./... 2>&1 | tail -40
```
Expected: all tests pass, including the new `TestDoctorRejectsUnknownFlags`
subtests (look for `--- PASS: TestDoctorRejectsUnknownFlags` and its three
sub-tests in the output). No regressions in the other 112 previously
passing tests.

### Step 5: Run format and vet gates

```
gofmt -l .
go vet ./...
```
Expected: both produce no output and exit 0. If `gofmt -l .` lists
`main.go` or `main_test.go`, run `gofmt -w main.go main_test.go` and
re-check.

## Test plan

- New test: `TestDoctorRejectsUnknownFlags` in `main_test.go`, covering:
  - An unmistakably bogus flag (`--totally-bogus-flag`) → exit 2, stderr
    contains `flag provided but not defined: -totally-bogus-flag`.
  - A plausible typo of a real flag (`--fixx`) → exit 2, stderr contains
    `flag provided but not defined: -fixx` (this is the regression case:
    before this fix, this exact input silently ran a normal doctor pass
    and exited 0).
  - The pre-existing `--skip-confirm` without `--fix` validation still
    works and still exits 1 (regression guard — this plan must not break
    it while fixing the unrelated unknown-flag bug).
- No existing test in `main_test.go` exercises this path (confirmed via
  `grep -n "doctorArgs\|doctor.*flag" main_test.go` → no matches) so there
  is no prior test to model this after directly; the subprocess-exec
  pattern in Step 3 is new to this file. If a helper like `buildTestBinary`
  already exists elsewhere in the file by the time you implement this
  (check `grep -n "func buildTestBinary\|exec.Command(\"go\", \"build\"" main_test.go`
  first), reuse it instead of adding a duplicate.
- Verification: `go test -v ./...` → all pass, including the 3 new
  subtests, with no reduction in the pre-existing 112 passing tests.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` exits 0, no files listed
- [ ] `go test -v ./...` exits 0; `TestDoctorRejectsUnknownFlags` and its
      3 subtests exist and pass; no previously-passing test now fails
- [ ] `grep -n "doctorArgs" main.go` returns no matches (dead variable
      fully removed)
- [ ] `/tmp/act_plan023_verify doctor --bogus-flag; echo $?` (build fresh
      if needed) prints `flag provided but not defined: -bogus-flag` and
      exits `2`
- [ ] `act doctor --skip-confirm` (built fresh) still exits `1` with
      `Error: --skip-confirm has no effect without --fix`
- [ ] `act doctor --help` output is byte-for-byte unchanged from before
      this change
- [ ] No files outside `main.go` and `main_test.go` are modified
      (`git status` / `git diff --stat`)
- [ ] `plans/README.md` status row for plan 023 updated (unless a
      reviewer told you they own that file)

## STOP conditions

Stop and report back (do not improvise) if:

- The `case "doctor":` block in `main.go` doesn't match the "Current
  state" excerpt above (line numbers may have shifted — that's fine, but
  if the *logic* differs — e.g. `doctorArgs` is no longer dead, or a third
  flag has been added — treat it as drift and report rather than guessing
  intent).
- `fs.Parse(subArgs)` with `flag.ExitOnError` produces an exit code other
  than 2 for an unknown flag on your build (this would mean Go's stdlib
  behavior differs from what was verified during planning — report the
  actual code observed rather than editing the Done criteria yourself).
- The `--skip-confirm` validation stops working after switching to
  `flag.FlagSet` (it should not — `fs.Parse` only touches recognized flags
  and doesn't affect the `if skipConfirm && !fix` check that runs after
  it — but if it breaks, that's a sign something about flag pointer
  dereferencing went wrong, not a sign to work around it).
- Building the test binary inside `main_test.go` (Step 3) requires
  network access, CGO, or otherwise fails in ways unrelated to the
  `doctor` flag logic itself — report the exact build error rather than
  disabling the test.
- Any step's verification command fails twice after a reasonable fix
  attempt.

## Maintenance notes

- If a future plan adds a new `doctor` flag (e.g. something like the
  `--env` support discussed in a separate, unrelated plan for doctor),
  add it via `fs.Bool`/`fs.String`/etc. on the same `flag.NewFlagSet`
  introduced here — do not revert to a hand-rolled loop.
- The subprocess-exec test pattern introduced in Step 3
  (`buildTestBinary` + `exec.Command`) is the first of its kind in
  `main_test.go`. If it proves useful, a future cleanup could extract it
  into a shared test helper for other subcommands' flag-parsing tests
  (e.g. `forward`, `ecs`), but that's out of scope here — don't do it as
  part of this plan.
- A reviewer should check that the new tests actually build and run the
  real binary (not a stale one) — `buildTestBinary` uses `t.TempDir()` so
  each test run rebuilds fresh; confirm no caching issue causes false
  passes.
- The exit-code change (0 → 2 for unknown flags) is a user-visible
  behavior change, even though it's a bug fix. If any external tooling or
  CI script wraps `act doctor` and checks specifically for exit code 0/1
  (not >1) as "success/failure", a bad flag will now surprise it with
  exit 2 instead of silently succeeding — this is the intended fix, but
  worth a mention in the PR description if one is opened later.
