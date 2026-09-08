# Plan 020: Let `act env`, `act init`, and pure-config `act fav` subcommands run without the AWS CLI installed

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go main_test.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW (only widens an existing exemption predicate; every command
  that already required `aws` before this plan still requires it after, and
  the one command with mixed sub-behavior — `fav` — is handled by inspecting
  its sub-arguments, not by exempting it wholesale)
- **Depends on**: none (plan 019, which introduced `subcommandNeedsAWSCLI`,
  is already merged to `main`)
- **Category**: bug
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`main.go` gates every subcommand behind a single check: is `aws` on `PATH`?
Plan 019 already proved this check is wrong for `doctor` (the one command
whose *job* is to diagnose a missing `aws`) and carved out an exception.
The same bug class still affects three more commands that never call the
AWS CLI at all: `act init` (writes `~/.act.json` via `internal/config`
only), `act env` (list/add/rm — all `internal/config` only), and three of
`act fav`'s four behaviors (`fav list`, `fav add <id>`, `fav rm <id>` — all
`internal/config` only). Today, a brand-new user without the AWS CLI
installed cannot even run `act init` to create their config file, and an
existing user can't run `act env list` or `act fav list` to inspect config
they already have — despite neither command touching AWS at all. This is
the exact "one guard blocks a command that doesn't need what the guard
checks for" bug plan 019 fixed for `doctor`, now extended to three more
commands. The tricky part: `act fav` is not uniformly safe — a bare
`act fav` (no subcommand) launches the interactive picker and calls
`aws.StartSession`, which genuinely does need `aws`. `subcommandNeedsAWSCLI`
today only sees the top-level subcommand string (`"fav"`), so a simple
per-name boolean cannot express "safe for `fav list` but not for bare
`fav`" — this plan changes the function's signature to also take the
subcommand's remaining arguments so it can make that distinction.

## Current state

- `main.go:22-45` — `main()`'s current shape (unchanged since plan 019
  landed it):

  ```go
  // main.go:22-45
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

  Note `args` here is the slice *after* global flags (`--profile`,
  `--region`, `--env`, `--version`) have been stripped by
  `parseGlobalFlags`. `args[0]` (if present) is the subcommand name;
  `args[1:]` (if `len(args) > 1`) is that subcommand's own arguments — e.g.
  for `act fav list`, `args` is `["fav", "list"]`, so `subcmd` is `"fav"`
  and the subcommand's own args are `["list"]`. **If `len(args) <= 1`,
  slicing `args[1:]` is either empty or, when `len(args) == 0`, would panic
  with "slice bounds out of range" — this happens for bare `act` (no
  subcommand) or `act --version` alone (global flags parsed out, nothing
  left).** You must guard this in Step 1 — see the exact code shape given
  there; do not write `args[1:]` directly at the call site.

- `main.go:237-243` — the current `subcommandNeedsAWSCLI` (added by plan
  019, unchanged since):

  ```go
  // main.go:237-243
  // subcommandNeedsAWSCLI reports whether subcmd requires the AWS CLI to be
  // installed before it can do anything useful. "doctor" is the one
  // exception: it diagnoses (and, with --fix, can install) a missing AWS
  // CLI itself, so it must be reachable even when aws isn't on PATH yet.
  func subcommandNeedsAWSCLI(subcmd string) bool {
      return subcmd != "doctor"
  }
  ```

- `main.go:133-141` — the `fav` dispatch case (unchanged by this plan; shown
  so you can confirm `subArgs` here is exactly the slice this plan's new
  `subcommandNeedsAWSCLI` parameter must also see):

  ```go
  // main.go:133-141
  case "fav":
      subArgs := args[1:]
      if hasHelp(subArgs) {
          printFavHelp()
          os.Exit(0)
      }
      resolvedProfile := config.ResolveProfile(profile, env)
      resolvedRegion := config.ResolveRegion(region, env)
      runFav(resolvedProfile, resolvedRegion, subArgs)
  ```

- `main.go:1097-1167` — `runFav`, in full. This is the ground truth for the
  scope decision below — read it carefully:

  ```go
  // main.go:1097-1167
  func runFav(profile, region string, subArgs []string) {
      if len(subArgs) == 0 {
          // Show picker from favorites
          cfg := config.Load()
          if len(cfg.Favorites) == 0 {
              fmt.Fprintf(os.Stderr, "No favorites configured. Use 'act fav add <instance-id>' to add one.\n")
              os.Exit(0)
          }

          picked, err := tui.RunPicker("Select Favorite Instance", cfg.Favorites)
          if err != nil {
              fmt.Fprintf(os.Stderr, "Error: %v\n", err)
              os.Exit(1)
          }
          if picked == "" {
              os.Exit(0)
          }

          err = aws.StartSession(picked, profile, region)   // <-- needs aws
          if err != nil {
              fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
              os.Exit(1)
          }
          return
      }

      switch subArgs[0] {
      case "list":
          favorites := config.ListFavorites()               // <-- config only
          ...
      case "add":
          ...
          if err := config.AddFavorite(id); err != nil {     // <-- config only
          ...
      case "rm":
          ...
          if err := config.RemoveFavorite(id); err != nil {  // <-- config only
          ...
      default:
          fmt.Fprintf(os.Stderr, "Unknown fav subcommand: %s\n", subArgs[0])
          printFavHelp()
          os.Exit(1)
      }
  }
  ```

  Confirms exactly: bare `fav` (empty `subArgs`) is the only branch that
  calls into `internal/aws` (`aws.StartSession`); `list`/`add`/`rm`, and
  even the unknown-subcommand `default` branch, only touch
  `internal/config` or print and exit. `hasHelp(subArgs)` (checked in the
  dispatch case, before `runFav` is even called) also never touches `aws`.

- `main.go:536-584` — `runInit`, in full. Only calls `config.Exists`,
  `config.Load`, `config.Init`, `config.ConfigPath`, plus `os.Stdin`/`Getenv`
  — no `internal/aws` import use anywhere in the function.

- `main.go:1186-1242` — `runEnv`, in full. Only calls
  `config.ListEnvironments`, `config.AddEnvironment`,
  `config.RemoveEnvironment` — no `internal/aws` import use anywhere in the
  function.

- `main_test.go:118-144` — the existing characterization test for
  `subcommandNeedsAWSCLI`, which this plan's Step 1 signature change will
  break at compile time (the test currently calls the one-argument form)
  and which Step 2 rewrites:

  ```go
  // main_test.go:118-144 (current — you will replace this whole function)
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

  Note the two test cases you must **change the expectation of**, not just
  keep: `"env needs aws cli (unchanged behavior)"` (currently `want: true`)
  and `"init needs aws cli (unchanged behavior)"` (currently `want: true`).
  After this plan both must become `want: false`, and their names must
  change to stop claiming "unchanged behavior" since it's no longer true.
  `"fav needs aws cli"` (currently `want: true`, tested with subcmd `"fav"`
  alone and no sub-args concept in the old signature) must be re-expressed
  as multiple cases once the signature gains a `subArgs []string` parameter
  — see Step 2 for the exact new table.

- README.md — confirmed via live grep
  (`grep -n "aws CLI not found\|AWS CLI not found" README.md`) that this
  error message and guard are not documented anywhere in the README, same
  as plan 019 found. `CLAUDE.md`'s "update README after adding a
  feature/flag" rule does not apply — this is a bug fix to existing
  dispatch logic, no new flag or command is added, and no command's usage
  syntax changes. No README changes are needed; Step 4 re-confirms this via
  the same grep rather than assuming plan 019's finding still holds.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output |
| Test | `go test -v ./...` | all tests pass, exit 0 |

## Scope

**In scope**:
- `main.go` — change `subcommandNeedsAWSCLI`'s signature and exemption
  list; guard the `args[1:]` slice at the call site; no other line changes.
- `main_test.go` — rewrite `TestSubcommandNeedsAWSCLI` for the new
  signature and expanded exemption set.
- `plans/README.md` — status row update when done.

**Out of scope** (do NOT touch, even though related):
- `act upgrade` and the `""`/`help`/`--help`/`-h` case — both are confirmed
  AWS-CLI-free today (`upgrade` only touches `internal/updater`; help is
  pure printing) but the user has not asked for these to be exempted in
  this plan; extending further than `env`/`init`/`fav`'s config-only
  subcommands is a separate decision (see plan 019's own "Maintenance
  notes", which flagged this same boundary).
- Any change to `runFav`, `runInit`, `runEnv`, or any other function body
  inside the `switch subcmd` cases — this plan only changes *when the
  guard fires*, never what a command does once dispatched.
- Any change to the wording of the "aws CLI not found" error message.
- Adding a new flag or command.
- `internal/config/`, `internal/aws/`, `internal/doctor/` — no changes
  needed anywhere in these packages.

## Git workflow

- Branch: `advisor/020-extend-aws-cli-guard-exemption`.
- Commit message style (from `git log --oneline -5`): Conventional
  Commits, e.g. `fix: let act doctor run without AWS CLI already
  installed` (plan 019's commit `0f142b4`). Use:
  `fix: exempt env, init, and config-only fav subcommands from AWS CLI guard`
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Change `subcommandNeedsAWSCLI`'s signature and widen the exemption list

Replace the current one-argument predicate with a two-argument version
that also inspects the subcommand's own arguments, so `fav`'s bare-picker
path (which needs `aws`) can be distinguished from its config-only
subcommands (which don't):

```go
// subcommandNeedsAWSCLI reports whether subcmd (invoked with subArgs)
// requires the AWS CLI to be installed before it can do anything useful.
//
// "doctor" is exempt: it diagnoses (and, with --fix, can install) a
// missing AWS CLI itself, so it must be reachable even when aws isn't on
// PATH yet.
//
// "env" and "init" are exempt: both are pure ~/.act.json local-config
// commands (internal/config only) and never shell out to aws.
//
// "fav" is exempt only when its first sub-argument is "list", "add", or
// "rm" — those subcommands are pure local-config too. A bare "fav" (no
// sub-arguments) launches the interactive picker and starts a real SSM
// session via aws.StartSession, so it still needs aws; so does any
// unrecognized fav sub-argument, since runFav's default case is reached
// via the same dispatch path and the guard errs toward requiring aws for
// anything not explicitly known to be safe.
func subcommandNeedsAWSCLI(subcmd string, subArgs []string) bool {
	switch subcmd {
	case "doctor", "env", "init":
		return false
	case "fav":
		if len(subArgs) == 0 {
			return true
		}
		switch subArgs[0] {
		case "list", "add", "rm":
			return false
		default:
			return true
		}
	default:
		return true
	}
}
```

Update the call site in `main()` (main.go:35-40) to pass the subcommand's
own arguments, guarding against the empty-`args` case (bare `act`, or
`act --version` alone, both leave `args` with 0 or 1 elements after
global-flag parsing — slicing `args[1:]` when `len(args) == 0` panics with
"slice bounds out of range [1:0]", confirmed by reproducing it during plan
recon). Use this exact shape:

```go
	var subcmdArgs []string
	if len(args) > 1 {
		subcmdArgs = args[1:]
	}
	if subcommandNeedsAWSCLI(subcmd, subcmdArgs) {
		if _, err := exec.LookPath("aws"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 'aws' CLI not found in PATH.\nInstall it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html\n")
			os.Exit(1)
		}
	}
```

Do not change anything else in `main()` — the `switch subcmd` block and
every case body (including the `fav`, `env`, and `init` cases, which
compute their own local `subArgs := args[1:]` independently) are unchanged.
This is intentional duplication: `main()`'s existing per-case `subArgs :=
args[1:]` lines already handle their own empty-slice case correctly for
Go (slicing `args[1:]` when `len(args) >= 1` never panics — only
`len(args) == 0` panics, and none of those cases are reached unless
`subcmd` is non-empty, which requires `len(args) >= 1`). The panic risk is
specific to the *new* guard call site because it runs before the `switch`,
when `args` may still have length 0.

**Verify**: `go build -v ./...` → exit 0.

### Step 2: Rewrite `TestSubcommandNeedsAWSCLI` for the new signature

Replace the entire `TestSubcommandNeedsAWSCLI` function in `main_test.go`
(main_test.go:118-144) with a table that covers the new two-argument
signature and every branch of Step 1's logic:

```go
func TestSubcommandNeedsAWSCLI(t *testing.T) {
	tests := []struct {
		name    string
		subcmd  string
		subArgs []string
		want    bool
	}{
		{"doctor does not need aws cli", "doctor", nil, false},
		{"env does not need aws cli", "env", nil, false},
		{"env list does not need aws cli", "env", []string{"list"}, false},
		{"init does not need aws cli", "init", nil, false},
		{"bare fav needs aws cli (picker + StartSession)", "fav", nil, true},
		{"fav list does not need aws cli", "fav", []string{"list"}, false},
		{"fav add does not need aws cli", "fav", []string{"add", "i-0123456789abcdef0"}, false},
		{"fav rm does not need aws cli", "fav", []string{"rm", "i-0123456789abcdef0"}, false},
		{"fav with unknown subcommand needs aws cli", "fav", []string{"bogus"}, true},
		{"ec2 needs aws cli", "ec2", nil, true},
		{"forward needs aws cli", "forward", nil, true},
		{"ecs needs aws cli", "ecs", nil, true},
		{"ssm needs aws cli", "ssm", nil, true},
		{"rds needs aws cli", "rds", nil, true},
		{"upgrade needs aws cli (unchanged behavior)", "upgrade", nil, true},
		{"empty subcmd needs aws cli (unchanged behavior)", "", nil, true},
		{"unknown subcmd needs aws cli (unchanged behavior)", "bogus", nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := subcommandNeedsAWSCLI(tt.subcmd, tt.subArgs); got != tt.want {
				t.Errorf("subcommandNeedsAWSCLI(%q, %v) = %v, want %v", tt.subcmd, tt.subArgs, got, tt.want)
			}
		})
	}
}
```

This is 16 subtests (up from 12): the 12 original cases minus the two
whose expectation flipped (`env`, `init` — now `false`) plus 6 new cases
covering `fav`'s branch-by-subcommand logic (`env list` and `fav`'s four
sub-variants, plus the unknown-fav-subcommand case).

**Verify**: `go test ./... -run TestSubcommandNeedsAWSCLI -v` → all 16
subtests pass.

### Step 3: Confirm no README changes are needed

Run `grep -n "aws CLI not found\|AWS CLI not found" README.md` — expect no
matches, confirming (not assuming) plan 019's earlier finding still holds:
this error message and the guard mechanism have never been documented in
the README, so there is nothing here for CLAUDE.md's README-update rule to
apply to.

**Verify**: `grep -n "aws CLI not found\|AWS CLI not found" README.md` →
no output (grep exits 1, meaning "no matches" — this is the expected
passing result, not a failure).

### Step 4: Full verification pass

Run the full command table top to bottom, then manually verify the fix
with a real built binary and an AWS-CLI-free `PATH`:

1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output.
4. `go test -v ./...` → all tests pass, exit 0, including the 16 subtests
   from Step 2 (replacing the previous 12).
5. Build and manually verify the newly-exempted commands run without
   `aws` on `PATH`:
   ```
   go build -o /tmp/act-verify-020 .
   FAKEBIN=$(mktemp -d)
   FAKEHOME=$(mktemp -d)
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 env list
   printf "\n\n" | HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 init
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 fav list
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 fav add i-0123456789abcdef0
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 fav rm i-0123456789abcdef0
   ```
   Expected: every one of these five invocations exits 0 and prints its
   normal output (`env list` → "No environments configured."; `init` →
   prompts then "✓ Config written to ..."; `fav list` → "No favorites
   configured."; `fav add`/`fav rm` → "Added ...to favorites."/"Removed
   ...from favorites."). None should print the "aws CLI not found" error.
6. Confirm the bare `fav` picker path and every unaffected command still
   enforce the guard identically to before this plan:
   ```
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 fav 2>&1; echo "exit=$?"
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 ec2 2>&1; echo "exit=$?"
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 --version 2>&1; echo "exit=$?"
   ```
   Expected: all three print the exact `Error: 'aws' CLI not found in
   PATH...` message and exit 1 (unchanged from before this plan). The
   `--version`-alone case is the one that would panic if Step 1's
   `len(args) > 1` guard were done wrong (or omitted) — confirming it
   exits 1 cleanly with the expected message, not a Go panic/stack trace,
   is the specific regression this sub-check catches.
7. Confirm `act doctor` still behaves exactly as plan 019 left it (this
   plan must not regress it):
   ```
   HOME="$FAKEHOME" PATH="$FAKEBIN" /tmp/act-verify-020 doctor; echo "exit=$?"
   ```
   Expected: doctor's full report runs (prints all check lines, including
   `✗ AWS CLI: not found...`), exit code 1 (some checks fail, matching
   plan 019's documented behavior — this is correct, not a regression).
8. Clean up: `rm -rf /tmp/act-verify-020 "$FAKEBIN" "$FAKEHOME"`.

## Test plan

- `main_test.go` (Step 2): rewritten `TestSubcommandNeedsAWSCLI`, 16
  subtests covering every branch of the new two-argument
  `subcommandNeedsAWSCLI` — the exempt commands (`doctor`, `env`, `init`),
  `fav`'s four sub-cases (bare/list/add/rm) plus its unknown-subcommand
  fallback, and every command that must remain unaffected (`ec2`,
  `forward`, `ecs`, `ssm`, `rds`, `upgrade`, empty, unknown).
- No test can directly exercise `main()` (it calls `os.Exit`, matching
  this repo's existing untested-`main()` convention, same as plan 019
  documented). Step 4's manual verification with a real built binary and
  an empty-`PATH` sandbox is the closest equivalent and is mandatory, not
  optional.
- Verification: `go test -v ./...` → all pass, including the 16 new/
  changed subtests, with zero changes to any other existing test's pass/
  fail status.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./...` exits 0, all tests pass (existing + the 16-subtest
      `TestSubcommandNeedsAWSCLI`)
- [ ] `HOME=<empty-dir> PATH=<empty-dir> act env list`, `act init`,
      `act fav list`, `act fav add <id>`, `act fav rm <id>` all exit 0 and
      print their normal output instead of the AWS-CLI-not-found error —
      verified manually per Step 4.5
- [ ] `HOME=<empty-dir> PATH=<empty-dir> act fav` (bare), `act ec2`, and
      `act --version` all still print the exact pre-plan `Error: 'aws' CLI
      not found in PATH...` message and exit 1 (no panic) — verified
      manually per Step 4.6
- [ ] `act doctor` with no `aws` on `PATH` still runs its full report and
      exits 1 — verified manually per Step 4.7 (confirms no regression of
      plan 019's fix)
- [ ] `grep -n "aws CLI not found\|AWS CLI not found" README.md` returns no
      matches (no README change needed, confirmed not just assumed)
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 020 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at any "Current state" location doesn't match what's excerpted
  above (drift since this plan was written) — in particular, if `runFav`'s
  body has changed such that `list`/`add`/`rm` now call into
  `internal/aws` (re-read the current `runFav` in full before trusting
  this plan's scope decision).
- Slicing `args[1:]` at the new guard call site panics for any input during
  Step 4's manual verification — this means Step 1's `len(args) > 1` guard
  was not applied correctly; fix it before proceeding, and if a second
  attempt also panics, stop and report rather than patching around it with
  a different mechanism (e.g. a `recover()`).
- You find that `upgrade`, `""`/`help`, or any other currently-guarded
  command also seems safe to exempt — do not add it silently; that widening
  is explicitly out of scope for this plan (see "Out of scope") and is a
  separate decision for the user.
- A step's verification fails twice after a reasonable fix attempt.
- Exempting `env`/`init`/`fav`'s config-only subcommands changes any other
  command's observable behavior in any way — if `act ec2`, `act doctor`, or
  any command not named in this plan's scope behaves differently with `aws`
  missing than it did before this plan, stop and report; do not patch
  around it.

## Maintenance notes

- `subcommandNeedsAWSCLI` now takes `subArgs []string` specifically to let
  `fav` express "some of my sub-behaviors need aws, some don't." If a
  future command gains the same split (a bare invocation that needs `aws`
  plus config-only subcommands that don't), model it after the `fav` case
  in Step 1's `switch subArgs[0]` — do not add a second top-level boolean
  parameter; the existing `subArgs`-inspection pattern already generalizes.
- `upgrade` and `""`/`help`/`--help`/`-h` remain guarded (unexempted) after
  this plan even though they don't call `aws` either — this was a
  deliberate scope boundary (see "Out of scope"), not an oversight. A
  reviewer should not treat leaving them guarded as a bug in this plan.
- A reviewer should scrutinize: that the `switch subcmd` block's case
  bodies in `main()` are byte-for-byte unchanged (only the guard's call
  site, the `subcmdArgs` computation immediately above it, and the
  `subcommandNeedsAWSCLI` function signature/body are new); that the
  `len(args) > 1` guard at the new call site was actually exercised by
  Step 4.6's `--version`-alone check (a reviewer re-running that one
  command with a real built binary is the fastest way to catch a
  regression here); and that Step 4's manual verification commands were
  actually run and their output captured, not just assumed from the code
  reading clean.
