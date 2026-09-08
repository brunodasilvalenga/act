# Plan 026: Extract the duplicated "pick ECS cluster" block in main.go into a helper

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go`
> If `main.go` changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `aa50614`, 2026-07-21

## Why this matters

`runECS` and `runLogs` in `main.go` each contain an identical ~19-line block
that resolves the target ECS cluster: check the `--cluster` flag, call
`aws.ListECSClusters`, handle the list error and the empty-list case, run
`tui.RunPicker("Select ECS Cluster", clusters)`, handle the picker error and
empty-selection case, and assign the result. This is the exact kind of
duplication plan `007` already fixed once for EC2 instance-picking (see
`pickInstance`, added in commit `dd54243`) — this cluster-picking case was
added later and missed that cleanup. Any future change to the convention
(e.g. distinguishing "no clusters" from "API error" in the exit message, or
changing an exit code) currently requires editing two near-identical blocks
by hand, and a missed site produces a UX inconsistency between `act ecs` and
`act ecs logs` that's easy to overlook in review. Extracting a single helper
makes the convention change-once-apply-everywhere and removes about 19 lines
from `main.go`.

## Current state

Confirmed via `grep -n "Select ECS Cluster" main.go`, which returns exactly
2 matches (at the time of writing, lines 714 and 923 — re-run this yourself
before starting, since line numbers shift as other plans land).

1. `main.go:696-744` (`runECS`), cluster-pick block at lines 702-723:
```go
func runECS(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("ecs", flag.ExitOnError)
	cluster := fs.String("cluster", "", "ECS cluster name")
	service := fs.String("service", "", "Filter tasks by service name")
	fs.Parse(subArgs)

	clusterName := *cluster
	if clusterName == "" {
		clusters, err := aws.ListECSClusters(profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
			os.Exit(1)
		}
		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select ECS Cluster", clusters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		clusterName = picked
	}

	serviceName := *service
	loadFunc := func() ([]aws.ECSTask, error) {
		return aws.ListECSTasks(clusterName, profile, region, serviceName)
	}

	selected, err := tui.RunECS(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}

	err = aws.StartECSExec(selected.ClusterName, selected.TaskID, selected.ContainerName, profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting ECS exec: %v\n", err)
		os.Exit(1)
	}
}
```
After the cluster is resolved, `runECS` diverges: it filters/lists ECS
*tasks* via `tui.RunECS` and starts an exec session. That part is NOT
duplicated with `runLogs` and must not be touched.

2. `main.go:903-932` (`runLogs`, first half only — full function is longer),
cluster-pick block at lines 912-932 — byte-identical in logic to the block
above (whitespace-only differences from being at a different indentation
context; confirm with a diff before editing, see Step 1):
```go
func runLogs(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	cluster := fs.String("cluster", "", "ECS cluster name")
	service := fs.String("service", "", "ECS service name")
	logGroup := fs.String("log-group", "", "Override log group")
	since := fs.String("since", "5m", "How far back to start")
	noFollow := fs.Bool("no-follow", false, "Disable follow mode")
	fs.Parse(subArgs)

	clusterName := *cluster
	if clusterName == "" {
		clusters, err := aws.ListECSClusters(profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
			os.Exit(1)
		}
		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
			os.Exit(0)
		}
		picked, err := tui.RunPicker("Select ECS Cluster", clusters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		clusterName = picked
	}

	serviceName := *service
	if serviceName == "" {
		services, err := aws.ListECSServices(clusterName, profile, region)
		...
```
`runLogs` also uses a local variable named `cluster` for its `--cluster`
flag (`cluster := fs.String("cluster", "", "ECS cluster name")`) — same
flag-variable name as `runECS`. Both functions read the flag via `*cluster`.

After the cluster is resolved, `runLogs` goes on to pick an ECS *service*
(another `tui.RunPicker` block, structurally similar but for services, not
clusters) and then a log group. **Do not touch the service-picker or
log-group-picker blocks** — they are out of scope for this plan (see
"Maintenance notes" for why).

The existing precedent to model this extraction on — `pickInstance` in
`main.go` (~lines 603-617, added by commit `dd54243996c8` / plan `007`):
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
`pickInstance` takes a `loadFunc` closure because `tui.Run` is generic over
the loader. Your new helper is different: `aws.ListECSClusters(profile,
region)` takes `profile, region` directly (no closure needed), so the
cleanest signature is `func pickECSCluster(profile, region, clusterFlag
string) string` — it takes the raw flag value, does the "already set via
flag" short-circuit internally, and returns the resolved cluster name; it
follows the same exit-code idiom as `pickInstance` (exit 1 on list error,
exit 0 on empty list or empty selection) but does not need a closure
parameter since there's only one possible loader (`aws.ListECSClusters`).

`ListECSClusters`'s exact signature (`internal/aws/ecs.go:42`):
```go
func ListECSClusters(profile, region string) ([]string, error) {
```

## Commands you will need

| Purpose        | Command              | Expected on success        |
|-----------------|----------------------|-----------------------------|
| Build           | `go build -v ./...`  | exit 0                      |
| Vet             | `go vet ./...`       | exit 0, no output           |
| Format check    | `gofmt -l .`         | exit 0, no output (no files listed) |
| Test            | `go test -v ./...`   | exit 0, all pass            |

(These match `.github/workflows/ci.yml` exactly — build, gofmt check, vet,
then test, in that order.)

## Scope

**In scope** (the only file you should modify):
- `main.go` only — add one new helper function (`pickECSCluster`), then
  update the two call sites (`runECS`, `runLogs`) to use it.

**Out of scope** (do NOT touch, even though it looks related):
- The service-picker block inside `runLogs` (~lines 934-954, picking via
  `aws.ListECSServices` + `tui.RunPicker("Select ECS Service", services)`)
  — it looks similarly duplicatable in shape but has no second call site
  today, so extracting it now would be speculative generalization. Flagged
  as a follow-up in "Maintenance notes", not fixed here.
- The log-group-picker block inside `runLogs` (~lines 956-980) — same
  reasoning, single call site, out of scope.
- `internal/aws/ecs.go` — no changes needed; the new helper only calls the
  already-exported `ListECSClusters`.
- `internal/tui/` — no changes to the TUI package.
- Any change to exit codes, error message wording, or control flow — this
  is a pure refactor; observable CLI behavior (stdout/stderr/exit codes)
  must be identical before and after.

## Git workflow

- Branch: `advisor/026-dedupe-ecs-cluster-picker`
- Single commit; message style example from this repo's precedent commit
  (`dd54243`, plan `007`): `refactor: extract shared instance-picker helper
  in main.go` — for this plan, use:
  `refactor: extract shared ECS cluster-picker helper in main.go`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Confirm the two call sites still match

Run `grep -n "Select ECS Cluster" main.go`. It must return exactly 2 lines.
Read 20 lines above and below each match and confirm they match the
"Current state" excerpts above (ignoring line-number drift). If the count
is not 2, or the block content has materially changed (not just line
numbers), STOP and report — do not proceed with edits based on a stale
excerpt.

**Verify**: `grep -c "Select ECS Cluster" main.go` → `2`.

### Step 2: Add the `pickECSCluster` helper

Add this function to `main.go` directly after `pickInstance` (~line 617,
right before the `// parseCommands` comment):

```go
// pickECSCluster resolves the target ECS cluster: it returns clusterFlag
// unchanged if non-empty, otherwise it lists clusters and runs the
// interactive picker. It exits the process (0) if there are no clusters or
// the user quits the picker without selecting, and exits (1) on a list or
// picker error — matching the behavior every call site had before this
// helper was extracted.
func pickECSCluster(profile, region, clusterFlag string) string {
	if clusterFlag != "" {
		return clusterFlag
	}

	clusters, err := aws.ListECSClusters(profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
		os.Exit(1)
	}
	if len(clusters) == 0 {
		fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
		os.Exit(0)
	}

	picked, err := tui.RunPicker("Select ECS Cluster", clusters)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if picked == "" {
		os.Exit(0)
	}
	return picked
}
```

**Verify**: `go build -v ./...` → exit 0. (The new function is unused until
Step 3/4 — Go does not error on unused top-level functions, only unused
local variables/imports, so this should build clean immediately.)

### Step 3: Update `runECS` to use the helper

In `runECS` (~lines 696-744), replace:

```go
	clusterName := *cluster
	if clusterName == "" {
		clusters, err := aws.ListECSClusters(profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
			os.Exit(1)
		}
		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select ECS Cluster", clusters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		clusterName = picked
	}
```

with:

```go
	clusterName := pickECSCluster(profile, region, *cluster)
```

**Verify**: `go build -v ./...` → exit 0.

### Step 4: Update `runLogs` to use the helper

In `runLogs` (~lines 903-989), replace:

```go
	clusterName := *cluster
	if clusterName == "" {
		clusters, err := aws.ListECSClusters(profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
			os.Exit(1)
		}
		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
			os.Exit(0)
		}
		picked, err := tui.RunPicker("Select ECS Cluster", clusters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		clusterName = picked
	}
```

with:

```go
	clusterName := pickECSCluster(profile, region, *cluster)
```

Do not touch anything after this block — the service-picker and
log-group-picker code that follows in `runLogs` stays exactly as-is.

**Verify**: `go build -v ./...` → exit 0.

### Step 5: Confirm full deduplication and run the full check suite

**Verify**: `grep -c "Select ECS Cluster" main.go` → `1` (the only remaining
occurrence is inside `pickECSCluster` itself).

**Verify**: `grep -c "pickECSCluster(profile, region" main.go` → `3` (the
function definition itself, plus one call in `runECS`, plus one call in
`runLogs` — if your grep pattern also matches the `func pickECSCluster(...)`
line, expect 3 total; if you adjust the pattern to exclude the definition,
expect 2 call sites).

**Verify**: `go build -v ./...` → exit 0.

**Verify**: `gofmt -l .` → no output (empty).

**Verify**: `go vet ./...` → exit 0, no output.

**Verify**: `go test -v ./...` → exit 0, all pass.

## Test plan

`main.go` has no test coverage for `run*` functions in general (they call
`os.Exit` directly and drive interactive TUI pickers, which are not
unit-testable without process-boundary tricks) — check yourself with
`grep -n "pickInstance" main_test.go`, which returns no matches, confirming
this repo's precedent (`pickInstance`, extracted by plan `007`) also has no
dedicated unit test. `pickECSCluster` has the same untestability property
and should not gain one as part of this plan — do not attempt to add a test
that mocks `os.Exit` or the TUI picker; that is a larger effort out of
scope here.

The verification for this plan is: (1) the code still compiles
(`go build -v ./...`), (2) `gofmt -l .` and `go vet ./...` are clean, (3)
the full existing test suite still passes unchanged (`go test -v ./...` —
no existing test touches this code path, so "unchanged" means no other
package's tests regress), and (4) a manual read-through confirming both
call sites (`runECS`, `runLogs`) now produce byte-identical CLI behavior to
before — this is a pure, mechanical extraction with no logic change, so the
diff for each site should show only the 19-line block replaced by the
one-line call.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `pickECSCluster` helper function exists in `main.go`, placed directly
      after `pickInstance`
- [ ] `grep -c "Select ECS Cluster" main.go` returns `1`
- [ ] `runECS` and `runLogs` each contain a line matching
      `clusterName := pickECSCluster(profile, region, *cluster)`
- [ ] `go build -v ./...` exits 0
- [ ] `gofmt -l .` produces no output
- [ ] `go vet ./...` exits 0
- [ ] `go test -v ./...` exits 0, all pass
- [ ] `git status` shows only `main.go` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `grep -n "Select ECS Cluster" main.go` returns a count other than 2
  before you start editing — a third call site would mean this plan's
  two-site assumption is wrong; report the actual locations instead of
  guessing how to extend the plan.
- Either of the two blocks doesn't match the excerpts in "Current state"
  (the codebase has drifted since this plan was written) — re-read the
  whole containing function before changing it, and if the divergence is
  more than cosmetic (e.g. different error messages, different exit codes
  between the two sites), STOP and report rather than picking one behavior
  to standardize on.
- A step's verification fails twice after a reasonable fix attempt.
- You find that `runECS`'s `--cluster` flag variable or `runLogs`'s
  `--cluster` flag variable is not named `cluster` (i.e. `*cluster` does
  not compile at a call site) — re-check the `flag.NewFlagSet` block for
  that function and use the actual variable name; if the fix requires
  anything beyond substituting the correct identifier, STOP and report.

## Maintenance notes

- Any future ECS subcommand that needs to resolve a cluster from a
  `--cluster` flag should call `pickECSCluster(profile, region, *cluster)`
  from the start, rather than reintroducing the inline pattern this plan
  removes.
- A reviewer should scrutinize: that neither call site's exit code or error
  message wording changed — diff each site against its "before" version in
  this plan to confirm byte-for-byte equivalence in observable behavior.
- Follow-up deferred out of this plan: `runLogs` also contains a
  service-picker block (`aws.ListECSServices` + `tui.RunPicker("Select ECS
  Service", services)`, ~lines 934-954) that is structurally similar to the
  cluster-picker this plan extracts. It currently has only one call site,
  so extracting it now would be speculative generalization with no
  duplication to justify it — revisit if/when a second call site for
  service-picking appears (e.g. an `act ecs restart` command), at which
  point a `pickECSService(profile, region, cluster, serviceFlag string)
  string` helper following this same pattern would be the right move.
</content>
