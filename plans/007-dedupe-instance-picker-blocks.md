# Plan 007: Extract the duplicated "pick instance" block in main.go into a helper

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- main.go`
> If the file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

Five functions in `main.go` — `runConnect`, `runForward`, `runRDS`,
`runSSH`, `runRDP` — each contain an identical 13-line sequence: build a
`loadFunc` closure calling an `aws.List*Instances` variant, call
`tui.Run(loadFunc)`, check `err != nil` (print to stderr + `os.Exit(1)`),
check `selected == nil` (`os.Exit(0)`), then read
`selected.InstanceID`. Any future change to this convention — e.g.
distinguishing "no instances found" from "AWS API error" in the exit
message, or changing the exit code — currently requires editing 5 near-
identical blocks by hand, and a missed site produces a UX inconsistency
between subcommands that's easy to overlook in review. Extracting a single
helper makes the convention change-once-apply-everywhere and shrinks
`main.go` by roughly 50 lines.

## Current state

The five call sites (confirmed via `grep -n "tui.Run(loadFunc)" main.go`,
which returns exactly 5 matches):

1. `main.go:484-496` (`runConnect`):
```go
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

	err = aws.StartSession(selected.InstanceID, profile, region)
```

2. `main.go:524-538` (`runForward`):
```go
	instanceID := *target
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
	}
```

3. `main.go:666-681` (`runRDS`, inside the bastion-picking branch):
```go
	bastionID := *bastion
	if bastionID == "" {
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
		bastionID = selected.InstanceID
	}
```

4. `main.go:787-801` (`runSSH`):
```go
	instanceID := *target
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
	}
```

5. `main.go:860-874` (`runRDP`):
```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListWindowsInstances(profile, region, tags)
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
	}
```

Note site 5 (`runRDP`) uses `aws.ListWindowsInstances` while the other four
use `aws.ListRunningInstances` — the helper must take the `loadFunc` (or the
list function) as a parameter, not hardcode `ListRunningInstances`.

`tui.Run`'s signature (`internal/tui/tui.go:252`):
```go
func Run(loadFunc func() ([]aws.Instance, error)) (*aws.Instance, error) {
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all pass |
| Confirm dedup | `grep -c "tui.Run(loadFunc)" main.go` | returns `1` (only inside the new helper) |

## Scope

**In scope** (the only files you should modify):
- `main.go` only — add one new helper function, then update the 5 call
  sites (`runConnect`, `runForward`, `runRDS`, `runSSH`, `runRDP`) to use it.

**Out of scope** (do NOT touch, even though they look related):
- `internal/tui/tui.go`, `internal/tui/picker.go`, `internal/tui/ecs.go` —
  no changes to the TUI package itself.
- The `selected == nil` check at `main.go:596` inside `runECS` (ECS *task*
  picking, not EC2 *instance* picking) — different type (`aws.ECSTask`, not
  `aws.Instance`), out of scope for this plan since the helper you're
  writing is typed specifically for `aws.Instance`.
- Do not change any exit codes, error message wording, or control flow
  behavior — this is a pure refactor; the CLI's observable behavior (stdout/
  stderr/exit codes) must be identical before and after.

## Git workflow

- Branch: `advisor/007-dedupe-instance-picker-blocks`
- Single commit; message style example: `feat: add ec2 ssh, tag filtering, remote forwarding, RDS, ECS logs, and favorites commands` (commit `9a68e0b`, for feature-shaped precedent) — for this refactor:
  `refactor: extract shared instance-picker helper in main.go`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add the helper function

Add this new function to `main.go`, near the other helpers (`parseTags`,
`hasHelp`) — a good location is directly after `parseTags`
(`main.go:464-479`):

```go
// pickInstance runs the interactive picker for the given loadFunc and
// returns the selected instance ID. It exits the process (0) if the user
// quits the picker without selecting, and exits (1) on error — matching
// the behavior every call site had before this helper was extracted.
func pickInstance(loadFunc func() ([]aws.Instance, error)) string {
	selected, err := tui.Run(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}
	return selected.InstanceID
}
```

**Verify**: `go build ./...` → exit 0 (there will be "declared and not used"
type errors until you update call sites in the following steps — that's
expected; if you want to verify this step in isolation, temporarily
reference `pickInstance` with `_ = pickInstance` and remove that line once
Step 2 begins).

### Step 2: Update `runConnect` (site 1)

In `runConnect` (`main.go:481-503`), replace:

```go
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

	err = aws.StartSession(selected.InstanceID, profile, region)
```

with:

```go
	loadFunc := func() ([]aws.Instance, error) {
		return aws.ListRunningInstances(profile, region, tags)
	}

	instanceID := pickInstance(loadFunc)

	err := aws.StartSession(instanceID, profile, region)
```

Note the `:=` on the last line changes from `err =` to `err :=` because
`err` is no longer declared earlier in this function by the removed
`tui.Run` call — check the rest of `runConnect` for any other use of `err`
in this scope before finalizing (there is none; the function ends shortly
after with the existing `if err != nil { ... }` check unchanged).

**Verify**: `go build ./...` → exit 0.

### Step 3: Update `runForward` (site 2)

In `runForward` (`main.go:505-555`), replace:

```go
	instanceID := *target
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
	}
```

with:

```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}
```

**Verify**: `go build ./...` → exit 0.

### Step 4: Update `runRDS` (site 3)

In `runRDS` (`main.go:607-689`), replace the bastion-picking block:

```go
	bastionID := *bastion
	if bastionID == "" {
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
		bastionID = selected.InstanceID
	}
```

with:

```go
	bastionID := *bastion
	if bastionID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		bastionID = pickInstance(loadFunc)
	}
```

**Verify**: `go build ./...` → exit 0.

### Step 5: Update `runSSH` (site 4)

In `runSSH` (`main.go:779-818`), replace:

```go
	instanceID := *target
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
	}
```

with:

```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}
```

**Verify**: `go build ./...` → exit 0.

### Step 6: Update `runRDP` (site 5)

In `runRDP` (`main.go:850-893`), replace:

```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListWindowsInstances(profile, region, tags)
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
	}
```

with:

```go
	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListWindowsInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}
```

Note this site correctly keeps `aws.ListWindowsInstances` (not
`ListRunningInstances`) — the helper is agnostic to which list function the
closure calls.

**Verify**: `go build ./...` → exit 0.

### Step 7: Confirm full deduplication and run tests

**Verify**: `grep -c "tui.Run(loadFunc)" main.go` → returns `1` (the only
remaining call is inside `pickInstance` itself).

**Verify**: `go test ./...` → exit 0, all pass (no test currently exercises
`main.go`, so this confirms no other package broke — see plan `011` for
adding `main_test.go` coverage, which is a separate, larger effort).

## Test plan

`main.go` has zero existing tests and no established pattern for testing
`run*` functions (they call `os.Exit` directly, which is difficult to test
without refactoring further — out of scope here). This plan is a pure,
mechanical refactor with identical before/after behavior; the verification
gate is: (1) the code still compiles (`go build ./...`), (2) the full test
suite still passes unchanged (`go test ./...` — no test currently touches
this code, so "unchanged" here means no other package's tests regress), and
(3) a manual read-through confirming each of the 5 call sites produces
byte-identical CLI behavior to before. If plan `011` (add `main_test.go`
characterization tests) executes before this plan, prefer running this
plan's steps only after those characterization tests exist and pass, so
their pre/post diff would catch any behavioral regression this refactor
might accidentally introduce — but this plan does not depend on plan `011`
strictly (see dependency note in `plans/README.md`).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `pickInstance` helper function exists in `main.go`
- [ ] `grep -c "tui.Run(loadFunc)" main.go` returns `1`
- [ ] `grep -c "pickInstance(loadFunc)" main.go` returns `5` (one call per
      original site: runConnect, runForward, runRDS, runSSH, runRDP)
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `git status` shows only `main.go` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any of the 5 call sites don't match the excerpts above (drift since this
  plan was written) — re-read the whole function containing the mismatched
  site before changing it.
- A step's verification fails twice after a reasonable fix attempt.
- You find a 6th call site not listed here (re-run
  `grep -n "tui.Run(loadFunc)" main.go` yourself before starting — if it
  returns a different count than 5, STOP and report the actual count and
  locations rather than proceeding with this plan's fixed step list).

## Maintenance notes

- Any future subcommand that needs an EC2 instance picker should call
  `pickInstance(loadFunc)` from the start, rather than reintroducing the
  inline pattern this plan removes.
- A reviewer should scrutinize: that no site's exit-code or error-message
  behavior changed — diff each site against its "before" version in this
  plan to confirm byte-for-byte equivalence in observable behavior.
- This plan does not address the larger `main.go` god-file issue (see plan
  `011`) — `pickInstance` should move together with the functions that use
  it if/when `main.go` is later split into per-command files.
