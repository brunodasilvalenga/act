# Plan 032: Run `act doctor`'s two network-bound checks concurrently

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/doctor/doctor.go internal/doctor/doctor_test.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: MED — this introduces the repo's first genuinely concurrent code
  in `internal/`. The two existing `go func(){...}()` calls
  (`internal/aws/rdp_unix.go:60`, `internal/aws/rdp_windows.go:53`) each
  write into a private, per-call `chan error` and nothing else — they do not
  establish a pattern for "multiple goroutines writing into a shared slice",
  which is what this plan introduces. Data-race potential is real if two
  goroutines are ever made to write the same slice index, or if the main
  goroutine reads a result before `Wait()` returns. `go test -race` is a
  hard gate for this reason (see Done criteria).
- **Depends on**: none
- **Category**: perf
- **Planned at**: commit `aa50614`, 2026-07-21

## Why this matters

`doctor.Run` (`internal/doctor/doctor.go:49-101`) runs 7 checks in strict
sequence. Two of them are the only ones that leave the machine over the
network: `checkCredentials` shells out to `aws sts get-caller-identity`,
which is a real round-trip to AWS's STS endpoint; `checkVersion` calls
`updater.CheckLatestVersion()`, which does an `http.Get` to the GitHub
releases API. Neither check's inputs or outputs depend on the other, and
neither depends on any of the other 5 checks (`checkAWSCLI`,
`checkSessionManagerPlugin`, `checkRegion`, `checkProfile`,
`checkConfigFile` — all local/near-instant, see "Current state" below).
Because all 7 run one after another, `act doctor` pays the **sum** of the
two network round-trips on every invocation instead of the **max** of the
two. For a command whose entire purpose is being a fast "is my setup OK"
sanity check, this needlessly doubles the network-bound portion of its
wall-clock time. Running `checkCredentials` and `checkVersion` concurrently
(while leaving the 5 local checks untouched, still sequential) fixes this
with a small, contained change and no behavior change to any check's logic
or to the printed output.

## Current state

- `internal/doctor/doctor.go` — the only file whose behavior changes.
  `Run` (lines 49-101) as it exists today:

  ```go
  // internal/doctor/doctor.go:49-101
  func Run(profile, region, version string, fix, skipConfirm bool) error {
      results := []result{
          checkAWSCLI(),
          checkSessionManagerPlugin(),
          checkCredentials(profile, region),
          checkRegion(region),
          checkProfile(profile),
          checkConfigFile(),
          checkVersion(version),
      }

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

      fmt.Println()
      hasFailure := false
      for _, r := range results {
          var icon string
          var style lipgloss.Style
          switch r.Status {
          case statusPass:
              icon = "✓"
              style = passStyle
          case statusWarn:
              icon = "○"
              style = warnStyle
          case statusFail:
              icon = "✗"
              style = failStyle
              hasFailure = true
          }
          fmt.Printf("%s %s: %s\n", style.Render(icon), r.Name, r.Detail)
      }
      fmt.Println()

      if hasFailure {
          fmt.Println(failStyle.Render("Some checks failed. Fix the issues above."))
          os.Exit(1)
      }
      fmt.Println(passStyle.Render("All checks passed!"))
      return nil
  }
  ```

  Critical constraint: `results` is built as a slice literal in a fixed
  order, and the printing loop (`for _, r := range results`) prints them in
  that same order. Users and any future test may reasonably depend on "AWS
  CLI" always printing before "Session Manager plugin", "AWS credentials"
  always printing third, etc. The fix in this plan MUST preserve this exact
  order — it changes ONLY internal execution timing, never the printed
  order or content. This rules out any approach where goroutines append to
  a slice/channel in whichever order they finish; positions must be
  pre-assigned.

- The 7 check functions, confirmed by reading each one fully
  (`internal/doctor/doctor.go:103-278`):
  - `checkAWSCLI()` (103-127) — `exec.LookPath("aws")` + `exec.Command("aws",
    "--version")`. Local, fast. No network call.
  - `checkSessionManagerPlugin()` (129-147) — `exec.LookPath`. Local, fast.
    No network call.
  - `checkCredentials(profile, region string)` (149-187) — calls
    `config.ResolveProfile`/`config.ResolveRegion` (both read `~/.act.json`
    from disk, fast/local), then `exec.Command("aws", "sts",
    "get-caller-identity", ...).Output()` — this is the network-bound call
    (round-trip to AWS STS via the `aws` CLI subprocess).
  - `checkRegion(flagRegion string)` (189-203) — `config.ResolveRegion`
    only. Local, fast. No network call.
  - `checkProfile(flagProfile string)` (205-219) — `config.ResolveProfile`
    only. Local, fast. No network call.
  - `checkConfigFile()` (221-246) — `os.UserHomeDir()`, `os.Stat`,
    `config.Load()` (reads `~/.act.json`). Local, fast. No network call.
  - `checkVersion(version string)` (248-278) — if `version == "dev"`,
    returns immediately (no network call). Otherwise calls
    `updater.CheckLatestVersion()` (`internal/updater/updater.go:28-45`),
    which does `http.Get(repoAPI)` against the GitHub releases API — this is
    the second network-bound call.

  Confirmed: only `checkCredentials` and `checkVersion` touch the network;
  the other 5 are local disk/PATH lookups.

- Why concurrency is safe here: `checkCredentials` and `checkVersion` are
  pure functions of their string-typed inputs (`profile, region` /
  `version`). Reading them fully again confirms neither writes to any
  package-level variable, global, or any state shared with the other — each
  only builds and returns its own `result` value. That means two goroutines
  running them concurrently cannot race with each other *as long as*: (1)
  each goroutine writes its returned `result` into a **different**,
  pre-assigned index of the `results` slice — never the same index as
  another goroutine, and never an index also written by the sequential code
  — and (2) the main goroutine calls `sync.WaitGroup.Wait()` and does not
  read any `results[i]` written by a goroutine until after `Wait()` returns
  (Go's memory model only guarantees the main goroutine sees a goroutine's
  writes after the corresponding synchronization point — `Wait()` is that
  synchronization point here). This plan's Step 1 satisfies both conditions
  by pre-sizing `results` to length 7 and having each goroutine write to its
  own literal index (2 and 6) with no other goroutine or sequential code
  ever writing those same two indices concurrently.

- `internal/doctor/doctor_test.go` (107 lines, read in full) — existing
  tests: `TestExtractJSON` (table test on the pure `extractJSON` helper),
  `TestCheckRegion`, `TestCheckProfile` (both use the
  `overrideHomeForDoctorTest(t, dir)` helper at line 62-67, which
  temporarily points `HOME` at a `t.TempDir()` via `os.Setenv`/
  `t.Cleanup`). There is no existing test that calls `Run` itself (it calls
  `os.Exit(1)` on any failing check, making it hard to unit-test directly —
  this plan does not change that; see "Out of scope" below). Any new test
  this plan adds for checks that touch `~/.act.json` must use this same
  `overrideHomeForDoctorTest` pattern — do not invent a new one.

- Confirmed via `grep -rn "sync\.\|goroutine\|go func" --include="*.go"
  internal/`: the only existing goroutines in this repo are in
  `internal/aws/rdp_unix.go:60` and `internal/aws/rdp_windows.go:53`
  (`go func() { done <- cmd.Wait() }()`), each writing into its own
  private, single-purpose `chan error` — not a shared slice. No file in
  `internal/` currently imports `"sync"`. This plan introduces the repo's
  first use of `sync.WaitGroup` and the first case of multiple goroutines
  writing into a shared slice — flagged again in "Maintenance notes" for
  the human reviewer.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output (no unformatted files) |
| Test | `go test -v ./...` | all tests pass, exit 0 |
| Race detector (mandatory for this plan) | `go test -race ./internal/doctor/...` | `ok`, exit 0, no `DATA RACE` report |

## Scope

**In scope**:
- `internal/doctor/doctor.go` — only the `Run` function's internals (how it
  invokes and collects the 7 checks). No check function's own logic
  changes.
- `internal/doctor/doctor_test.go` — add tests per "Test plan" below.
- `plans/README.md` (status row update when done).

**Out of scope** (do NOT touch, even though related):
- The internals of `checkCredentials`, `checkVersion`, or any of the other
  5 check functions — they keep their exact current signatures, logic, and
  return values. Only *how `Run` calls and collects them* changes.
- `runFixes` and the `--fix`/`--skip-confirm` flow (`internal/doctor/
  doctor.go` — the `if fix { ... }` block inside `Run`, plus whatever
  `runFixes` does elsewhere in the package). It is already sequential and
  interactive by necessity (it may prompt the user and re-run
  `checkAWSCLI`/`checkSessionManagerPlugin` after applying a fix) — leave
  it untouched. Do not parallelize anything inside the `if fix { ... }`
  branch.
- Parallelizing any of the 5 local/fast checks (`checkAWSCLI`,
  `checkSessionManagerPlugin`, `checkRegion`, `checkProfile`,
  `checkConfigFile`). They stay exactly as they are today: sequential, in
  their current relative order. There is no measurable wall-clock benefit
  to parallelizing local `exec.LookPath`/`os.Stat`/disk-read calls, and
  doing so only adds unnecessary goroutine/race surface for near-zero gain.
  Do not extend this plan's pattern to them.
- Any change to the printed output format, wording, icons, or the final
  print order of the 7 results. This plan changes only internal execution
  timing.
- Any change to `internal/updater/updater.go` or `internal/config/
  config.go`.

## Git workflow

- Branch: `advisor/032-parallelize-doctor-checks`.
- Commit message style (Conventional Commits, per `git log --oneline -10`,
  e.g. `fix: let act doctor run without AWS CLI already installed`). No
  existing commit in this repo uses a `perf:` prefix — this will be the
  first. Use: `perf: run doctor's network-bound checks concurrently`.
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Restructure `Run` to launch `checkCredentials` and `checkVersion` concurrently

In `internal/doctor/doctor.go`, replace the `results := []result{...}`
slice literal with a pre-sized slice, run the 5 local checks sequentially
in their current relative order writing into their current indices, then
launch the two network-bound checks in goroutines synchronized with a
`sync.WaitGroup`, each writing into its own pre-assigned index. Target
shape:

```go
// internal/doctor/doctor.go
import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/updater"
	"github.com/charmbracelet/lipgloss"
)

// ...

func Run(profile, region, version string, fix, skipConfirm bool) error {
	// results is pre-sized so each check writes into a fixed, known index.
	// This preserves the exact print order below regardless of which of
	// checkCredentials/checkVersion finishes first.
	results := make([]result, 7)
	results[0] = checkAWSCLI()
	results[1] = checkSessionManagerPlugin()
	results[3] = checkRegion(region)
	results[4] = checkProfile(profile)
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
		results[2] = checkCredentials(profile, region)
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

	fmt.Println()
	hasFailure := false
	for _, r := range results {
		var icon string
		var style lipgloss.Style
		switch r.Status {
		case statusPass:
			icon = "✓"
			style = passStyle
		case statusWarn:
			icon = "○"
			style = warnStyle
		case statusFail:
			icon = "✗"
			style = failStyle
			hasFailure = true
		}
		fmt.Printf("%s %s: %s\n", style.Render(icon), r.Name, r.Detail)
	}
	fmt.Println()

	if hasFailure {
		fmt.Println(failStyle.Render("Some checks failed. Fix the issues above."))
		os.Exit(1)
	}
	fmt.Println(passStyle.Render("All checks passed!"))
	return nil
}
```

Notes on this shape:
- Everything from `if fix {` to the end of the function is byte-for-byte
  unchanged from today — only the block that builds `results` (previously
  the slice literal, now the 5 sequential assignments + the two goroutines
  + `wg.Wait()`) changes.
- The indices (0=AWS CLI, 1=Session Manager plugin, 2=AWS credentials,
  3=Region, 4=Profile, 5=Config, 6=Version) match the original literal's
  order exactly — this is what keeps the printed order identical.
- Add `"sync"` to the import block (alphabetically after `"strings"`, per
  Go's standard import ordering, which this file already follows).
- Do not introduce a `results []result` with `append` — `append` from
  multiple goroutines on the same slice is a real data race even if you
  think you've avoided index collisions; the pre-sized `make([]result, 7)`
  with fixed-index writes is the only correct approach here and is what
  this plan specifies. Do not substitute a channel- or errgroup-based
  design unless you determine it is materially simpler — if you do,
  it must still (a) preserve the exact 0-6 print order and (b) pass
  `go test -race`.

**Verify**: `go build -v ./...` → exit 0.

### Step 2: Run the full verification pass, including the race detector

Run, in order:

1. `go vet ./...` → exit 0, no output.
2. `gofmt -l .` → no output. If `doctor.go` is listed, run `gofmt -w
   internal/doctor/doctor.go` and re-check.
3. `go test -v ./...` → all existing tests pass, exit 0. `TestCheckRegion`
   and `TestCheckProfile` in particular must still pass unchanged (they
   call `checkRegion`/`checkProfile` directly, not through `Run`, so this
   plan's change to `Run` should not affect them at all — if it does,
   that's a signal something in Step 1 broke isolation between checks;
   stop and investigate before proceeding).
4. `go test -race ./internal/doctor/...` → `ok`, exit 0, **no** `DATA RACE`
   output. This is the mandatory gate for this plan (see "Why this
   matters" / Status "Risk"). If it reports a race, do not suppress or
   work around it — re-read Step 1's indexing and re-verify no two
   goroutines (or a goroutine and the sequential code) write the same
   `results[i]`.

**Verify**: all four commands above pass as described.

### Step 3: Manually confirm print order and correctness with a real build

Build the binary and run `act doctor` for real, to confirm the observable
output is unchanged (same 7 lines, same order, same content shape) and
that it still exits non-zero on failure and zero on success, matching
pre-plan behavior:

```
go build -o /tmp/act-doctor-verify .
/tmp/act-doctor-verify doctor; echo "exit=$?"
```

Expected: 7 report lines print in this exact order — AWS CLI, Session
Manager plugin, AWS credentials, Region, Profile, Config, Version — each
prefixed with `✓`/`○`/`✗ ` and a name/detail matching the pre-plan format
(compare against a run of `act doctor` built from `git stash` / the
previous commit if you want a direct diff — the text content of each line
should be identical, only wall-clock timing changes). Exit code depends on
your local AWS credential/CLI state, as it did before this plan; the point
of this check is order and format, not exit code.

Run it 3 times in a row to build confidence that goroutine scheduling never
reorders the printed lines:

```
for i in 1 2 3; do /tmp/act-doctor-verify doctor > /tmp/doctor-run-$i.txt 2>&1; done
diff /tmp/doctor-run-1.txt /tmp/doctor-run-2.txt
diff /tmp/doctor-run-1.txt /tmp/doctor-run-3.txt
```

Expected: the three output files may legitimately differ in the "AWS
credentials" or "Version" detail text if network conditions changed between
runs (e.g. a transient STS error), but the **order of the 7 check names**
(the sequence of `AWS CLI:`, `Session Manager plugin:`, `AWS credentials:`,
`Region:`, `Profile:`, `Config:`, `Version:` labels) must be identical
across all three runs. If you have a stable environment (credentials and
network unchanged between runs), the files should `diff` as identical.

Clean up: `rm -f /tmp/act-doctor-verify /tmp/doctor-run-*.txt`.

**Verify**: all three runs show the 7 check labels in the same order;
`diff` shows no reordering.

## Test plan

- New test in `internal/doctor/doctor_test.go`: `TestRunResultOrder` (or
  similar) is not straightforward to write directly against `Run` because
  `Run` calls `os.Exit(1)` on any failing check and shells out to the real
  `aws` CLI / GitHub API — this repo's existing tests avoid testing `Run`
  directly for the same reason (confirmed: no existing test calls `Run`).
  Instead, add a narrower, deterministic test that exercises the exact
  concurrency shape from Step 1 without depending on `Run`'s side effects:
  write a small helper test that builds a `[]result` of length 7 the same
  way Step 1 does (pre-size, sequential writes to indices 0/1/3/4/5, two
  goroutines with a `WaitGroup` writing to indices 2/6 using two dummy
  functions instead of `checkCredentials`/`checkVersion`), and asserts
  after `Wait()` that all 7 indices are non-zero-value / contain the
  expected `Name` at the expected position. This proves the indexing
  pattern is race-free and order-preserving without invoking real AWS/HTTP
  calls. Model the test file structure (table-driven, `t.Run` subtests) on
  the existing `TestExtractJSON` in the same file.
- If, on inspecting `checkCredentials`/`checkVersion`, you find a way to
  make them independently testable with fakes/mocks already exists in this
  package (re-check `runFixes` and any test helpers) and it is trivial to
  reuse for a fuller integration-style test of `Run`'s ordering, you may
  add that instead/in addition — but do not spend more than one extra
  iteration on this; the goroutine-shape unit test above is the minimum
  bar and is sufficient to satisfy Done criteria.
- Any test touching `checkRegion`/`checkProfile`/`checkConfigFile`
  (directly or via a full `Run`-shaped test) must use
  `overrideHomeForDoctorTest(t, dir)` (doctor_test.go:62-67), matching
  `TestCheckRegion`/`TestCheckProfile`.
- Verification: `go test -v ./...` → all pass, including the new test(s).
  `go test -race ./internal/doctor/...` → `ok`, no race.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./...` exits 0, all existing tests still pass plus the
      new test(s) from "Test plan"
- [ ] `go test -race ./internal/doctor/...` exits 0 with `ok` and no `DATA
      RACE` report — mandatory, not optional
- [ ] Manual run from Step 3 shows the 7 check labels in the same order
      (`AWS CLI`, `Session Manager plugin`, `AWS credentials`, `Region`,
      `Profile`, `Config`, `Version`) across 3 consecutive runs
- [ ] `internal/doctor/doctor.go`'s `checkAWSCLI`, `checkSessionManagerPlugin`,
      `checkCredentials`, `checkRegion`, `checkProfile`, `checkConfigFile`,
      `checkVersion`, and `runFixes` function bodies are byte-for-byte
      unchanged from before this plan (only `Run` changed) —
      `git diff internal/doctor/doctor.go` shows edits confined to the
      `Run` function
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 032 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the "Current state" locations doesn't match what's excerpted
  above (drift since this plan was written).
- `go test -race` reports a data race after your Step 1 implementation and
  it is not immediately obvious why (e.g. you've double-checked indices
  0-6 are each written exactly once and by exactly one goroutine/sequential
  statement, and the race persists) — do not silence or work around a race
  detector finding; stop and report the exact race output.
- You find that `checkCredentials` or `checkVersion` are not actually pure
  functions of their inputs (e.g. either turns out to mutate a package-level
  variable, cache, or shared struct you missed on inspection) — this
  invalidates the safety argument in "Why this matters" and needs a
  different plan, not a workaround.
- Preserving the exact print order turns out to be impossible with the
  goroutine approach for a reason not anticipated here.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- This is the first code in `internal/` to use `sync.WaitGroup` and the
  first case of multiple goroutines writing into a shared slice (as
  opposed to the existing `rdp_unix.go`/`rdp_windows.go` pattern of one
  goroutine writing into its own private channel). A reviewer should
  scrutinize the index assignments in Step 1 particularly carefully: any
  future edit to `Run` that adds an 8th check, reorders the existing 5
  sequential assignments, or changes which two checks run concurrently
  must re-verify that no two goroutines (or a goroutine and sequential
  code) ever write the same slice index, and must re-run `go test -race
  ./internal/doctor/...` before merging.
- If a future check is added that is also network-bound (e.g. a
  connectivity check against some other AWS endpoint), it's a reasonable
  candidate to fold into this same `WaitGroup`/pre-sized-slice pattern —
  just add it as a third `wg.Add`'d goroutine with its own dedicated index,
  following Step 1's shape exactly.
- Deliberately deferred, not done here: parallelizing the 5 local checks
  (no measurable benefit, added complexity — see "Out of scope"); changing
  `runFixes`'s sequential/interactive flow (out of scope, different
  concern); adding a timeout/context-cancellation around the network calls
  in `checkCredentials`/`checkVersion` (a real possible follow-up if either
  ever hangs, but a separate, larger change involving `context.Context`
  plumbing into `exec.CommandContext`/an `http.Client` with a `Context` —
  not attempted in this plan).
