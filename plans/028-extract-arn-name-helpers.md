# Plan 028: Extract and test ARN-suffix / ECS-group-name helpers in `internal/aws`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/aws/ecs.go internal/aws/logs.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW (pure, behavior-preserving extraction of existing inline
  logic into named functions, plus new tests only — no exported function's
  signature or observable output changes)
- **Depends on**: none
- **Category**: tech-debt

## Why this matters

`internal/aws/ecs.go` and `internal/aws/logs.go` each independently re-derive
"the last `/`-delimited segment of an ARN" (three call sites total, two of
them byte-for-byte identical inline code in two different files), and
`ecs.go` separately re-derives "strip the `service:` prefix from an ECS
task's `Group` field" — all with zero test coverage. `internal/aws/ec2.go`
already established the pattern this codebase uses for exactly this
situation: pull a pure string transformation out into a small named
function with a doc comment, and cover it with a table-driven test in
`ec2_test.go` (see `escapeTagFilterValue` and `normalizePasswordOutput`,
added by plan 013). `ecs.go`/`logs.go` never got the same treatment. Today,
edge cases are unverified by any test and only "work" as a side effect of
`strings.Split`/`strings.TrimPrefix` semantics nobody wrote down: an ARN
with no `/` happens to work because `strings.Split` on a string with no
separator returns a 1-element slice (so `parts[len(parts)-1]` is the whole
string) — not because any code documents that contract. A `Group` field
that isn't `service:`-prefixed (the real ECS case for standalone tasks not
launched by a service) is handled by an `if/else` that is more code than
necessary. Extracting and testing these closes the coverage gap and removes
duplicated logic, matching the established `ec2.go` convention.

## Current state

- `internal/aws/ecs.go` — package `aws`. Two of the three call sites live
  here.
  - `ListECSClusters`, lines 65-69:
    ```go
    clusters := make([]string, len(result.ClusterArns))
    for i, arn := range result.ClusterArns {
        parts := strings.Split(arn, "/")
        clusters[i] = parts[len(parts)-1]
    }
    ```
  - `ListECSTasks`, lines 130-139 (inside the `for _, task := range descResult.Tasks` loop):
    ```go
    taskParts := strings.Split(task.TaskArn, "/")
    taskID := taskParts[len(taskParts)-1]

    serviceName := ""
    if strings.HasPrefix(task.Group, "service:") {
        serviceName = strings.TrimPrefix(task.Group, "service:")
    } else {
        serviceName = task.Group
    }
    ```
- `internal/aws/logs.go` — also package `aws` (confirm the `package aws`
  line at the top of the file before editing; it is line 1 as of this
  writing). Third call site.
  - `ListECSServices`, lines 55-59:
    ```go
    services := make([]string, len(result.ServiceArns))
    for i, arn := range result.ServiceArns {
        parts := strings.Split(arn, "/")
        services[i] = parts[len(parts)-1]
    }
    ```
  - This is byte-for-byte the same pattern as `ListECSClusters` in `ecs.go`
    above.
- `internal/aws/ec2.go`, lines 158-174 — the exemplar to match the style
  of. Two small pure functions, each with a doc comment explaining *why*
  the transformation is needed (not just what it does):
  ```go
  // escapeTagFilterValue escapes characters that are significant in AWS CLI
  // shorthand filter syntax (commas separate multiple filter values) so a
  // literal comma in a tag value is not misinterpreted as a value separator.
  func escapeTagFilterValue(value string) string {
      return strings.ReplaceAll(value, ",", "\\,")
  }

  // normalizePasswordOutput converts AWS CLI's literal "None" text output
  // (produced when --query selects a null field under --output text) into an
  // empty string, so callers can use a simple non-empty check.
  func normalizePasswordOutput(raw string) string {
      trimmed := strings.TrimSpace(raw)
      if trimmed == "None" {
          return ""
      }
      return trimmed
  }
  ```
- `internal/aws/ec2_test.go`, lines 39-78 — the exemplar test structure to
  mirror exactly: same package (`aws`, no `_test` suffix on the package
  name), a `[]struct{ name, in ..., want ... }` table, `t.Run(tt.name, ...)`
  per case, plain `if got != want { t.Errorf(...) }` — no assertion library.
  Model your new test file on this file's `TestNormalizePasswordOutput` and
  `TestEscapeTagFilterValue` functions.
- Verified stdlib behavior (confirmed by running `go doc strings.TrimPrefix`
  during planning, not assumed): *"TrimPrefix returns s without the
  provided leading prefix string. If s doesn't start with prefix, s is
  returned unchanged."* This means the existing
  `if strings.HasPrefix(...) { TrimPrefix(...) } else { group }` block in
  `ListECSTasks` is redundant — `strings.TrimPrefix(group, "service:")`
  alone already produces `group` unchanged when the prefix is absent. The
  new helper can and should be a one-liner.
- `internal/aws/*_test.go` currently existing (confirmed via `ls`):
  `ec2_test.go`, `ssh_validate_test.go`, `ssm_test.go`. There is no
  `ecs_test.go` yet — you are creating a new file, not editing one.

## Commands you will need

| Purpose         | Command                     | Expected on success        |
|------------------|-----------------------------|-----------------------------|
| Build            | `go build -v ./...`         | exit 0, `Go build: Success` (or equivalent silent success) |
| Vet              | `go vet ./...`               | exit 0, no output           |
| Format check     | `gofmt -l .`                 | exit 0, no file names printed |
| Test             | `go test -v ./...`           | exit 0, all tests print `PASS`, including new ones |
| Test (package)   | `go test -v ./internal/aws/...` | exit 0, all `PASS`       |

(Verified by running each of these against the current `main` at commit
`aa50614` — all pass cleanly before this plan's changes: build succeeds,
`go vet` is silent, `gofmt -l .` prints nothing, `go test ./...` reports
112 tests passed across 6 packages.)

## Scope

**In scope** (the only files you should modify or create):
- `internal/aws/ecs.go` — add the two new helper functions; update
  `ListECSClusters` and `ListECSTasks` to call them.
- `internal/aws/logs.go` — update `ListECSServices` to call the ARN-suffix
  helper (defined in `ecs.go`; no import needed since both files are
  package `aws`).
- `internal/aws/ecs_test.go` (new file) — table-driven tests for both new
  helpers.

**Out of scope** (do NOT touch, even though it looks related):
- `internal/aws/ec2.go` — already has its own tested helpers
  (`escapeTagFilterValue`, `normalizePasswordOutput`) added by plan 013;
  this plan does not touch EC2 logic at all.
- Any exported function's signature (`ListECSClusters`, `ListECSTasks`,
  `ListECSServices` keep identical parameters and return types).
- Any change to `ECSTask`, `Instance`, or other struct definitions.
- `internal/aws/ssm.go`, `internal/aws/ssh_validate.go`, or their tests —
  unrelated to this finding.

## Git workflow

- Branch: `advisor/028-extract-arn-name-helpers`
- Single commit for this plan's change (it is one small, atomic refactor +
  test addition). Match this repo's Conventional Commits style, observed in
  `git log --oneline`: e.g. `refactor: use global --profile/--region flags`,
  `test: add characterization tests for main.go flag parsing`, `fix: treat
  AWS CLI "None" text output as no password data`. Since this change is
  primarily an extraction with new tests as the verification vehicle, use a
  `refactor:` prefix, e.g.:
  `refactor: extract arnSuffix/serviceNameFromGroup helpers in internal/aws`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add the two helper functions to `internal/aws/ecs.go`

Add these two functions to `internal/aws/ecs.go` (placement: after the
`listClustersOutput` type and before `ListECSClusters`, or anywhere else at
package scope in the file — Go doesn't care about declaration order within
a package, but grouping them together near the top for discoverability is
preferred, matching how `ec2.go` groups `escapeTagFilterValue` and
`normalizePasswordOutput` near their point of use):

```go
// arnSuffix returns the last "/"-delimited segment of an ARN — the
// resource name/ID that AWS CLI list-* commands return as a full ARN but
// that this tool displays and passes to other commands as a bare name.
// logs.go's ListECSServices also uses this helper.
func arnSuffix(arn string) string {
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

// serviceNameFromGroup extracts the service name from an ECS task's Group
// field. AWS formats Group as "service:<name>" for tasks launched by a
// service, and leaves it as some other value (e.g. "family:<name>") for
// standalone tasks not launched by a service; TrimPrefix returns the input
// unchanged when the prefix isn't present, so this already handles both
// cases correctly.
func serviceNameFromGroup(group string) string {
	return strings.TrimPrefix(group, "service:")
}
```

**Verify**: `gofmt -l internal/aws/ecs.go` → no output (file is already
correctly formatted).

### Step 2: Replace the inline logic in `ListECSClusters`

In `internal/aws/ecs.go`, replace:

```go
	clusters := make([]string, len(result.ClusterArns))
	for i, arn := range result.ClusterArns {
		parts := strings.Split(arn, "/")
		clusters[i] = parts[len(parts)-1]
	}
```

with:

```go
	clusters := make([]string, len(result.ClusterArns))
	for i, arn := range result.ClusterArns {
		clusters[i] = arnSuffix(arn)
	}
```

**Verify**: `go build -v ./...` → exit 0.

### Step 3: Replace the inline logic in `ListECSTasks`

In `internal/aws/ecs.go`, replace:

```go
		taskParts := strings.Split(task.TaskArn, "/")
		taskID := taskParts[len(taskParts)-1]

		serviceName := ""
		if strings.HasPrefix(task.Group, "service:") {
			serviceName = strings.TrimPrefix(task.Group, "service:")
		} else {
			serviceName = task.Group
		}
```

with:

```go
		taskID := arnSuffix(task.TaskArn)
		serviceName := serviceNameFromGroup(task.Group)
```

**Verify**: `go build -v ./...` → exit 0. Also run
`grep -n "strings\." internal/aws/ecs.go` and confirm `strings.Split` and
`strings.HasPrefix` no longer appear anywhere in this file (only
`strings.TrimSpace` from the existing error-handling paths, and the calls
inside your two new helper functions, should remain).

### Step 4: Replace the inline logic in `ListECSServices` (`internal/aws/logs.go`)

In `internal/aws/logs.go`, replace:

```go
	services := make([]string, len(result.ServiceArns))
	for i, arn := range result.ServiceArns {
		parts := strings.Split(arn, "/")
		services[i] = parts[len(parts)-1]
	}
```

with:

```go
	services := make([]string, len(result.ServiceArns))
	for i, arn := range result.ServiceArns {
		services[i] = arnSuffix(arn)
	}
```

`arnSuffix` is defined in `ecs.go` but callable from `logs.go` with no
import, since both files declare `package aws`. Confirm `logs.go` no longer
uses `strings.Split` anywhere after this edit — if it does not use any
other `strings.*` function, you may need to remove the now-unused
`"strings"` import from `logs.go`'s import block (check with `go build`;
an unused import is a compile error in Go, so the build itself will tell
you if this is needed).

**Verify**: `go build -v ./...` → exit 0. If it fails with
`"strings" imported and not used`, remove the `"strings"` import line from
`internal/aws/logs.go` and rebuild.

### Step 5: Add `internal/aws/ecs_test.go` with table-driven tests

Create `internal/aws/ecs_test.go`, package `aws`, modeled directly on the
structure of `TestNormalizePasswordOutput` and `TestEscapeTagFilterValue` in
`internal/aws/ec2_test.go` (see "Current state" above for that exact
pattern). Cover:

For `arnSuffix`:
- normal ARN with multiple `/` segments, e.g.
  `"arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster"` →
  `"my-cluster"`
- ARN with no `/` at all, e.g. `"my-cluster"` → `"my-cluster"` (degrades to
  returning the whole string unchanged — document this in the test case
  name, e.g. `"no slash"`)
- empty string → `""`
- an ARN with a trailing `/` (edge case: last segment is empty), e.g.
  `"arn:aws:ecs:us-east-1:123456789012:cluster/"` → `""` (this documents
  the actual behavior of `strings.Split` + take-last-element; do not treat
  this as a bug to fix, just record the real behavior in a test)

For `serviceNameFromGroup`:
- `Group` with the `service:` prefix, e.g. `"service:my-service"` →
  `"my-service"`
- `Group` without the prefix (standalone task), e.g. `"family:my-task"` →
  `"family:my-task"` (unchanged — this is the case that was previously
  handled by the `else` branch of the removed `if/else`)
- empty `Group` string → `""`

Example shape (fill in exact test names/table rows per the list above —
this is the pattern, not a literal file to copy verbatim without checking
it compiles):

```go
package aws

import "testing"

func TestArnSuffix(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{"multi-segment arn", "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster", "my-cluster"},
		{"no slash", "my-cluster", "my-cluster"},
		{"empty string", "", ""},
		{"trailing slash", "arn:aws:ecs:us-east-1:123456789012:cluster/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arnSuffix(tt.arn); got != tt.want {
				t.Errorf("arnSuffix(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		})
	}
}

func TestServiceNameFromGroup(t *testing.T) {
	tests := []struct {
		name  string
		group string
		want  string
	}{
		{"service prefix", "service:my-service", "my-service"},
		{"standalone task, no service prefix", "family:my-task", "family:my-task"},
		{"empty group", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceNameFromGroup(tt.group); got != tt.want {
				t.Errorf("serviceNameFromGroup(%q) = %q, want %q", tt.group, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test -v ./internal/aws/... -run 'TestArnSuffix|TestServiceNameFromGroup'`
→ exit 0, both test functions and all their subtests print `PASS`.

## Test plan

- New tests: `TestArnSuffix` and `TestServiceNameFromGroup` in the new file
  `internal/aws/ecs_test.go`, covering the cases enumerated in Step 5:
  multi-segment ARN, no-slash ARN, empty string, trailing-slash ARN (for
  `arnSuffix`); `service:`-prefixed group, non-prefixed group, empty group
  (for `serviceNameFromGroup`).
- Structural pattern to follow: `internal/aws/ec2_test.go`'s
  `TestNormalizePasswordOutput` and `TestEscapeTagFilterValue` (table-driven,
  `t.Run` per case, plain equality check).
- Full verification: `go test -v ./...` → exit 0, all existing 112 tests
  still pass, plus the new subtests from `TestArnSuffix` and
  `TestServiceNameFromGroup` (7 new subtests total: 4 + 3).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` exits 0 with no output (no unformatted files)
- [ ] `go test -v ./...` exits 0; all pre-existing tests still pass, and
      `TestArnSuffix` / `TestServiceNameFromGroup` (with all their subtests)
      appear in the output and pass
- [ ] `grep -rn "strings.Split(arn" internal/aws/` returns no matches
      (the duplicated inline ARN-suffix logic is gone from all three former
      call sites)
- [ ] `grep -n "strings.HasPrefix(task.Group" internal/aws/ecs.go` returns
      no matches
- [ ] `git status` shows changes only in `internal/aws/ecs.go`,
      `internal/aws/logs.go`, and the new `internal/aws/ecs_test.go`
- [ ] `plans/README.md` status row for plan 028 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ecs.go` lines 65-69 or 130-139, or
  `internal/aws/logs.go` lines 55-59, doesn't match the excerpts in
  "Current state" (the codebase has drifted since this plan was written —
  re-run the drift check command at the top of this file).
- `go build ./...` fails after Step 4 with an error other than the expected
  possible `"strings" imported and not used` in `logs.go` — investigate and
  report rather than guessing at further changes.
- A verification command fails twice in a row after a reasonable fix
  attempt.
- You find a fourth call site of the same "ARN suffix" or "Group prefix"
  pattern elsewhere in `internal/aws/` not listed in this plan's Scope —
  report it rather than silently expanding scope to cover it.
- Any existing test in `internal/aws/` starts failing after your changes —
  this would indicate the extraction changed observable behavior, which
  this plan explicitly must not do.

## Maintenance notes

- If a future ECS-related list function is added to `internal/aws` and
  needs to strip an ARN down to its bare resource name, it should call
  `arnSuffix` rather than re-inlining `strings.Split`/`parts[len(parts)-1]`
  again — that is the whole point of this extraction.
- A reviewer should scrutinize: (1) that the three call sites now produce
  byte-identical output to before (this is a pure refactor, so a diff of
  `go test` output/any manual `aws ecs` command output before and after
  should show zero behavioral difference), and (2) that the new test cases
  for `arnSuffix`'s trailing-slash and no-slash inputs correctly describe
  today's real (slightly quirky) behavior rather than some idealized
  behavior — this plan intentionally does not change what happens for those
  inputs, only names and tests the existing behavior.
- Deferred, not in scope here: no additional validation or error-handling
  was added for malformed ARNs (e.g. rejecting an empty string outright) —
  the helpers preserve the exact permissive behavior of the code they
  replace. If stricter ARN validation is ever wanted, that is a separate,
  larger plan (it would need to decide what callers should do with an
  invalid ARN, which today they never checked for).
