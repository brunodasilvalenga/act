# Plan 019: Let `act doctor` run without the AWS CLI already installed

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat b8fc16a..HEAD -- main.go main_test.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW (moves an existing check later in `main()`; every other
  command's behavior is unchanged because the check still runs before their
  dispatch case executes)
- **Depends on**: plan 018 (`act doctor --fix`, merged to `main` at commit
  `4f42fa4`) — this plan fixes the exact gap plan 018's executor flagged:
  `doctor --fix`'s AWS-CLI-install path is currently unreachable
- **Category**: bug
- **Planned at**: commit `b8fc16a`, 2026-07-17

## Why this matters

Plan 018 added `act doctor --fix`, which can download and install a missing
AWS CLI. But `main.go`'s very first lines exit the whole process with an
error if `aws` isn't in `PATH` — before any subcommand, including `doctor`,
is ever dispatched. In practice this means: if you don't have the AWS CLI
installed, running `act doctor` (to find that out) or `act doctor --fix`
(to fix it) both fail with the *generic* "aws CLI not found" error and exit
1, never reaching `doctor.Run` at all. The one command whose entire purpose
is to diagnose and (per plan 018) fix this exact problem is the one command
that can never run without the problem already being fixed. This plan moves
the guard so it still protects every other command (which do need a working
`aws` on `PATH` to do anything useful) but lets `doctor` and `doctor --fix`
run and report/fix the missing-CLI condition themselves.

## Current state

- `main.go:22-26` — the guard, today unconditional and running before
  argument parsing or subcommand dispatch:

  ```go
  // main.go:22-26
  func main() {
      if _, err := exec.LookPath("aws"); err != nil {
          fmt.Fprintf(os.Stderr, "Error: 'aws' CLI not found in PATH.\nInstall it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html\n")
          os.Exit(1)
      }

      // Parse global flags manually from os.Args
      var profile, region, env string
      var showVersion bool
      args := os.Args[1:]
      args = parseGlobalFlags(args, &profile, &region, &env, &showVersion)

      if showVersion {
          printVersion()
          os.Exit(0)
      }

      // Determine subcommand
      subcmd := ""
      if len(args) > 0 {
          subcmd = args[0]
      }

      switch subcmd {
      case "", "help", "--help", "-h":
          printUsage()
          os.Exit(0)

      case "ec2":
          ...
  ```

- `main.go:150-174` — the `doctor` dispatch case, unchanged by this plan
  (this is what plan 018 added; shown here so you can see exactly where
  `subcmd` becomes known and confirm this plan's new check placement lines
  up with it):

  ```go
  // main.go:150-174
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

- Every other `switch subcmd` case (`ec2`, `forward`, `ecs`, `ssm`, `rds`,
  `fav`, `env`, `init`, `upgrade`, plus the `""`/`help`/`--help`/`-h` case
  and the `default` "unknown command" case) genuinely does need, or is
  harmless without, a working `aws` binary:
  - `ec2`, `forward`, `ecs`, `ssm`, `rds`, `fav` all shell out to `aws` via
    `internal/aws/*.go` wrappers — they must keep failing fast if `aws` is
    missing, with the existing clear error message, rather than failing
    later with a confusing `exec: "aws": executable file not found in
    $PATH` from deep inside an AWS-CLI wrapper.
  - `env`, `init`, `""`/`help`, `upgrade`, and `default` (unknown command)
    never call the AWS CLI at all today (confirmed: `env` only touches
    `internal/config`; `init` only touches `internal/config`; `upgrade`
    only touches `internal/updater`; `help`/usage is pure printing) — so
    technically the guard is unnecessary for them too, but changing their
    behavior is out of scope (see below); this plan only carves out
    `doctor`.
  - `--version` (handled at `main.go:34-37`, before the `switch`) also does
    not call `aws` — again, out of scope; only `doctor` is carved out.

- `main_test.go` — the existing characterization-test file for `main.go`'s
  pure helpers (`TestParseGlobalFlags`, `TestHasHelp`, `TestParseTags`,
  `TestHelpFunctionsContainDocumentedFlags`). There is **no** existing test
  that exercises `main()` itself (it's untestable as written — it calls
  `os.Exit` directly and has no return value) — this plan does not change
  that; see Step 2's approach for testing the extracted logic instead of
  `main()`.

- `README.md` has no mention of the AWS-CLI-not-found error message or this
  guard — `CLAUDE.md`'s "update README after adding a feature/flag" rule
  does not apply here (this is a bug fix to existing dispatch logic, not a
  new feature, command, or flag; no flags or commands change shape). No
  README changes are needed for this plan — confirmed by grep in Step 3.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output (no unformatted files) |
| Test | `go test -v ./...` | all tests pass, exit 0 |

## Scope

**In scope**:
- `main.go` (move/guard the AWS-CLI check — additive logic change only)
- `main_test.go` (add a characterization test for the new
  `subcommandNeedsAWSCLI` helper — see Step 2)
- `plans/README.md` (status row update when done)

**Out of scope** (do NOT touch, even though related):
- Carving the guard out for any subcommand other than `doctor`
  (`env`, `init`, `upgrade`, `help`/`--version` technically don't need `aws`
  either, but changing their behavior is a separate, broader decision the
  user hasn't asked for — this plan fixes exactly the gap plan 018's
  executor flagged, nothing more).
- `internal/doctor/` — no changes needed there; `doctor.Run` and its checks
  already handle a missing `aws` correctly (that's the whole point of
  `checkAWSCLI`/`installAWSCLI`), they just never got a chance to run.
- Changing the wording of the "aws CLI not found" error message — keep it
  byte-for-byte identical, just relocate *when* it fires.
- Adding a new flag or command.

## Git workflow

- Branch: no strong convention observed (repo history shows both direct
  commits and `advisor/NNN-*`/`worktree-agent-*` branches merged via merge
  commits); use `advisor/019-fix-early-aws-cli-guard`.
- Commit message style (from `git log --oneline -5`): Conventional Commits,
  e.g. `feat: add doctor --fix to auto-remediate missing AWS CLI / Session
  Manager plugin`. Use: `fix: let act doctor run without AWS CLI already
  installed`
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Add a `subcommandNeedsAWSCLI` helper and move the guard in `main.go`

Replace the unconditional guard at the top of `main()` with logic that
determines the subcommand first, then only enforces the guard for
subcommands that need it. Restructure `main()` like this:

```go
// main.go — new shape
func main() {
	// Parse global flags manually from os.Args
	var profile, region, env string
	var showVersion bool
	args := os.Args[1:]
	args = parseGlobalFlags(args, &profile, &region, &env, &showVersion)

	// Determine subcommand
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}

	if subcommandNeedsAWSCLI(subcmd) {
		if _, err := exec.LookPath("aws"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 'aws' CLI not found in PATH.\nInstall it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html\n")
			os.Exit(1)
		}
	}

	if showVersion {
		printVersion()
		os.Exit(0)
	}

	switch subcmd {
	case "", "help", "--help", "-h":
		printUsage()
		os.Exit(0)

	case "ec2":
		...
```

**Critical ordering note**: the `if showVersion { ... }` block MUST move to
*after* the new guard, not stay where it was. The guard must run before
`showVersion`'s `os.Exit(0)` can short-circuit past it — otherwise
`act --version` with `aws` missing would silently exit 0 (printing the
version) instead of erroring like it does today, which is a real behavior
change and would fail Step 4.6's verification. `subcommandNeedsAWSCLI("")`
correctly returns `true` for the `--version`-only invocation (where `args`
after global-flag-stripping is empty, so `subcmd` is `""`), so moving the
guard ahead of the `showVersion` check preserves pre-plan behavior for
`--version` exactly. Everything from `switch subcmd {` onward is completely
unchanged — only the guard and the `showVersion` block moved, and `subcmd`
is now computed before both instead of after. Do not reorder or modify any
`case` body.

Add `subcommandNeedsAWSCLI` near `hasHelp` (main.go:228), following that
function's style (small, pure, easy to characterization-test):

```go
// subcommandNeedsAWSCLI reports whether subcmd requires the AWS CLI to be
// installed before it can do anything useful. "doctor" is the one
// exception: it diagnoses (and, with --fix, can install) a missing AWS
// CLI itself, so it must be reachable even when aws isn't on PATH yet.
func subcommandNeedsAWSCLI(subcmd string) bool {
	return subcmd != "doctor"
}
```

Note this intentionally returns `true` (guard enforced) for `env`, `init`,
`upgrade`, `""`/`help`, and unknown commands too — that preserves their
exact current behavior (see "Out of scope"); only `doctor` changes.

**Verify**: `go build -v ./...` → exit 0.

### Step 2: Add a characterization test for `subcommandNeedsAWSCLI`

Add to `main_test.go`, following the existing `TestHasHelp` table-test
pattern (main_test.go:96-115):

```go
func TestSubcommandNeedsAWSCLI(t *testing.T) {
	tests := []struct {
		name   string
		subcmd string
		want   bool
	}{
		{"doctor does not need aws cli", "doctor", false},
		{"ec2 needs aws cli", "ec2", true},
		{"forward needs aws cli", "forward", true},
		{"ecs needs aws cli", "ecs", true},
		{"ssm needs aws cli", "ssm", true},
		{"rds needs aws cli", "rds", true},
		{"fav needs aws cli", "fav", true},
		{"env needs aws cli (unchanged behavior)", "env", true},
		{"init needs aws cli (unchanged behavior)", "init", true},
		{"upgrade needs aws cli (unchanged behavior)", "upgrade", true},
		{"empty subcmd needs aws cli (unchanged behavior)", "", true},
		{"unknown subcmd needs aws cli (unchanged behavior)", "bogus", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subcommandNeedsAWSCLI(tt.subcmd); got != tt.want {
				t.Errorf("subcommandNeedsAWSCLI(%q) = %v, want %v", tt.subcmd, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test ./... -run TestSubcommandNeedsAWSCLI -v` → all 12
subtests pass.

### Step 3: Confirm no README changes are needed

Run `grep -n "aws CLI not found\|AWS CLI not found" README.md` — expect no
matches (this error message and guard have never been documented in
README, so there is nothing to update). If this grep unexpectedly returns a
match, read the surrounding README context before deciding whether it
needs updating — do not skip this check.

**Verify**: `grep -n "aws CLI not found\|AWS CLI not found" README.md` →
no output (exit 1 from grep, meaning "no matches found" — this is the
expected/passing result for this step, not a failure).

### Step 4: Full verification pass

Run the full command table from top to bottom:

1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output.
4. `go test -v ./...` → all tests pass, exit 0, including the 12 new
   subtests from Step 2.
5. Build the binary and manually verify the fix, using a temp directory
   with no `aws` on `PATH`:
   ```
   go build -o /tmp/act-verify .
   FAKEBIN=$(mktemp -d)
   PATH="$FAKEBIN" /tmp/act-verify doctor
   ```
   Expected: `doctor`'s normal report runs (prints ✗ AWS CLI: not found...
   along with the other checks), **exit code 1** (because `checkAWSCLI`
   fails, and `doctor.Run`'s existing `hasFailure` logic — unchanged by
   this plan — calls `os.Exit(1)` when any check fails). This is correct:
   the point of this plan is that `doctor` *runs and reports*, not that it
   exits 0 when the CLI truly is missing.
6. Confirm every other command still enforces the guard identically to
   before this plan:
   ```
   PATH="$FAKEBIN" /tmp/act-verify ec2 2>&1; echo "exit=$?"
   PATH="$FAKEBIN" /tmp/act-verify --version 2>&1; echo "exit=$?"
   ```
   Expected: `act ec2` prints the exact same `Error: 'aws' CLI not
   found in PATH...` message and exits 1 (unchanged from before this
   plan). `act --version` also still hits the guard and exits 1 with the
   same message (global flags are parsed before the `switch`, but
   `subcmd` for `--version` alone is `""`, which `subcommandNeedsAWSCLI`
   returns `true` for — confirming this matches pre-plan behavior is the
   point of this check).
7. Clean up: `rm -rf /tmp/act-verify "$FAKEBIN"`.

## Test plan

- `main_test.go` (Step 2): `TestSubcommandNeedsAWSCLI`, 12 subtests
  covering `doctor` (the one exception) and every other subcommand +
  empty/unknown, modeled on `TestHasHelp`'s existing table-test structure.
- No test can directly exercise `main()` (it calls `os.Exit`, matching this
  repo's existing untested-`main()` convention — confirmed no test file
  attempts this today). Step 4's manual verification with a real built
  binary and an empty-`PATH` sandbox is the closest equivalent and is
  mandatory, not optional.
- Verification: `go test -v ./...` → all pass, including the 12 new
  subtests, with zero changes to any existing test's pass/fail status.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./...` exits 0, all tests pass (existing + 12 new in
      `main_test.go`)
- [ ] `PATH=<empty-dir> act doctor` runs `doctor`'s full report (prints all
      7 check lines) instead of the old generic guard error — verified
      manually per Step 4.5
- [ ] `PATH=<empty-dir> act ec2` and `PATH=<empty-dir> act --version` both
      still print the exact pre-plan `Error: 'aws' CLI not found in
      PATH...` message and exit 1 — verified manually per Step 4.6
- [ ] `grep -n "aws CLI not found\|AWS CLI not found" README.md` returns no
      matches (no README change needed, confirmed not just assumed)
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 019 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at any "Current state" location doesn't match what's excerpted
  above (drift since this plan was written).
- You find that some other subcommand (besides `doctor`) also needs to be
  carved out to make sense — do not add it silently; that's a scope
  decision for the user, not this plan. Report it and stop instead.
- A step's verification fails twice after a reasonable fix attempt.
- Moving the guard changes any other command's observable behavior in any
  way beyond "runs slightly later in the function" — if `act ec2` (or any
  command other than `doctor`) behaves differently with `aws` missing than
  it did before this plan, stop and report; do not patch around it.

## Maintenance notes

- If a future plan wants `env`/`init`/`upgrade`/`help` to also skip the
  AWS-CLI guard (since none of them call `aws` today), extend
  `subcommandNeedsAWSCLI`'s exception list — it's already structured as a
  single predicate function precisely so that's a one-line change later,
  not a rewrite. This plan deliberately does not do that itself (see
  "Out of scope") — only `doctor` was requested.
- If a new subcommand is added later that also doesn't need `aws` on PATH
  (e.g. a hypothetical local-only command), remember to add it to
  `subcommandNeedsAWSCLI`'s exceptions if desired — the function defaults
  to `true` (guard enforced) for anything not explicitly excluded, which is
  the safe default for new commands.
- A reviewer should scrutinize: that the `switch subcmd` block's case
  bodies are byte-for-byte unchanged (only the guard placement and the new
  helper function are new), and that the manual empty-PATH verification in
  Step 4 was actually run and its output captured, not just assumed from
  the code reading clean.
