# Plan 011: Add characterization tests for main.go's pure helpers and help text

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
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none (but should land BEFORE plans that refactor `main.go`
  structurally, if any are later written — e.g. splitting `main.go` into
  per-command files — so that refactor has a regression net; plan 007 in
  this batch is a small, mechanical, low-risk refactor and does not strictly
  require this plan first, but running this plan first is still recommended
  if both are being executed)
- **Category**: tests
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`go test ./... -cover` reports the `main` package at 0.0% coverage, and no
`main_test.go` file exists at all (confirmed via
`find . -name main_test.go`, which returns no results). `main.go` is 971
lines — the largest file in the repo by a wide margin — containing the
CLI's entire dispatch layer, all help text, and all 9 `run*` business-logic
functions. Most of `run*`'s logic (calling `aws.*` functions, launching AWS
CLI subprocesses, driving the interactive TUI) is genuinely hard to unit
test without extensive mocking, and that's a reasonable, justified gap for
now. But several pieces of `main.go` ARE pure, deterministic, and trivially
testable with zero refactoring: `parseGlobalFlags`, `parseTags`, `hasHelp`,
and all 12 `print*Help`/`printUsage` functions (which just write a fixed
string to `os.Stderr`/`os.Stdout`). None of these have any test today. This
plan adds that layer of characterization tests — cheap insurance that any
future edit to flag parsing or help text doesn't silently break the CLI's
documented interface, and a regression net for anyone later refactoring
`main.go`'s structure (e.g. splitting it into multiple files).

## Current state

`main.go:152-186` (`parseGlobalFlags` and `hasHelp`, full functions):

```go
// parseGlobalFlags extracts --profile, --region, --env, --version from args and returns remaining args.
func parseGlobalFlags(args []string, profile, region, env *string, showVersion *bool) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version" || args[i] == "-version":
			*showVersion = true
		case args[i] == "--profile" || args[i] == "-profile":
			if i+1 < len(args) {
				i++
				*profile = args[i]
			}
		case args[i] == "--region" || args[i] == "-region":
			if i+1 < len(args) {
				i++
				*region = args[i]
			}
		case args[i] == "--env" || args[i] == "-env":
			if i+1 < len(args) {
				i++
				*env = args[i]
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

func hasHelp(args []string) bool {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return true
	}
	return false
}
```

`main.go:464-479` (`parseTags`, full function):

```go
// parseTags extracts --tag key=value flags from args, returns remaining args and tags.
func parseTags(args []string) ([]string, []string) {
	var remaining []string
	var tags []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--tag" || args[i] == "-tag" {
			if i+1 < len(args) {
				i++
				tags = append(tags, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, tags
}
```

The 12 help-printing functions (`printUsage` at line 188, `printEC2Help` at
216, `printForwardHelp` at 238, `printECSHelp` at 262, `printRDSHelp` at 286,
`printECSLogsHelp` at 312, `printEC2SSHHelp` at 339, `printFavHelp` at 366,
`printDoctorHelp` at 388, `printInitHelp` at 402, `printEC2RDPHelp` at 820)
all follow the same shape: `fmt.Fprintf(os.Stderr, \`...\`)` with a fixed
backtick-quoted string, no parameters, no conditional logic. `printVersion`
(line 957) is the one exception — it calls `updater.CheckLatestVersion()`,
which does a real network call, so it is explicitly out of scope for this
plan (see Scope below).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (main package) | `go test . -v` | exit 0, all pass, including new tests |
| Full test suite | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `main_test.go` (create — new file, package `main`)

**Out of scope** (do NOT touch, even though they look related):
- Do not modify `main.go` itself — this plan is purely additive (new test
  file only). If a function needs to be exported or restructured to be
  testable, STOP and report rather than modifying `main.go` — the functions
  targeted here (`parseGlobalFlags`, `parseTags`, `hasHelp`, `print*Help`)
  are already unexported-but-same-package-testable as-is, since
  `main_test.go` lives in `package main` and can call them directly.
- `printVersion` (line 957) — makes a real network call via
  `updater.CheckLatestVersion()`; testing it would require mocking that
  network call or refactoring `updater` to accept an injectable HTTP client,
  both out of scope for this plan.
- Any `run*` function (`runConnect`, `runForward`, `runECS`, `runRDS`,
  `runLogs`, `runSSH`, `runRDP`, `runFav`, `runInit`) — these call
  `os.Exit`, launch real AWS CLI subprocesses, and drive an interactive TUI;
  testing them requires either process-boundary tricks (spawning a
  subprocess to test `os.Exit` paths) or significant refactoring to inject
  fakes, both of which are larger efforts than this plan covers. This plan
  is scoped strictly to the pure/deterministic helpers and help-text
  functions.
- `internal/tui/`, `internal/aws/`, `internal/config/`, `internal/doctor/`,
  `internal/updater/` — no changes to any other package.

## Git workflow

- Branch: `advisor/011-add-maingo-characterization-tests`
- Single commit; message style example: `fix: make config tests cross-platform for Windows CI` (commit `4d09420`,
  for precedent on adding tests to a previously-untested area) — for this
  change: `test: add characterization tests for main.go flag parsing and help text`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Create `main_test.go` with tests for `parseGlobalFlags`

```go
package main

import (
	"reflect"
	"testing"
)

func TestParseGlobalFlags(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		wantRemaining   []string
		wantProfile     string
		wantRegion      string
		wantEnv         string
		wantShowVersion bool
	}{
		{
			name:          "no flags",
			args:          []string{"ec2"},
			wantRemaining: []string{"ec2"},
		},
		{
			name:          "profile flag",
			args:          []string{"--profile", "prod", "ec2"},
			wantRemaining: []string{"ec2"},
			wantProfile:   "prod",
		},
		{
			name:          "region flag",
			args:          []string{"--region", "us-west-2", "ec2"},
			wantRemaining: []string{"ec2"},
			wantRegion:    "us-west-2",
		},
		{
			name:          "env flag",
			args:          []string{"--env", "staging", "ec2"},
			wantRemaining: []string{"ec2"},
			wantEnv:       "staging",
		},
		{
			name:            "version flag",
			args:            []string{"--version"},
			wantRemaining:   nil,
			wantShowVersion: true,
		},
		{
			name:          "single-dash variants",
			args:          []string{"-profile", "prod", "-region", "us-east-1", "ec2"},
			wantRemaining: []string{"ec2"},
			wantProfile:   "prod",
			wantRegion:    "us-east-1",
		},
		{
			name:          "flag with no following value is dropped silently",
			args:          []string{"ec2", "--profile"},
			wantRemaining: []string{"ec2"},
		},
		{
			name:          "flags interspersed with subcommand args",
			args:          []string{"ec2", "--tag", "Name=x", "--profile", "prod"},
			wantRemaining: []string{"ec2", "--tag", "Name=x"},
			wantProfile:   "prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var profile, region, env string
			var showVersion bool
			remaining := parseGlobalFlags(tt.args, &profile, &region, &env, &showVersion)

			if !reflect.DeepEqual(remaining, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRemaining)
			}
			if profile != tt.wantProfile {
				t.Errorf("profile = %q, want %q", profile, tt.wantProfile)
			}
			if region != tt.wantRegion {
				t.Errorf("region = %q, want %q", region, tt.wantRegion)
			}
			if env != tt.wantEnv {
				t.Errorf("env = %q, want %q", env, tt.wantEnv)
			}
			if showVersion != tt.wantShowVersion {
				t.Errorf("showVersion = %v, want %v", showVersion, tt.wantShowVersion)
			}
		})
	}
}
```

**Verify**: `go test . -run TestParseGlobalFlags -v` → all subtests pass.

### Step 2: Add tests for `hasHelp`

Append to `main_test.go`:

```go
func TestHasHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"empty args", []string{}, false},
		{"help keyword", []string{"help"}, true},
		{"double-dash help", []string{"--help"}, true},
		{"short help flag", []string{"-h"}, true},
		{"not help", []string{"ec2"}, false},
		{"help not in first position", []string{"ec2", "help"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasHelp(tt.args); got != tt.want {
				t.Errorf("hasHelp(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test . -run TestHasHelp -v` → all subtests pass.

### Step 3: Add tests for `parseTags`

Append to `main_test.go`:

```go
func TestParseTags(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantRemaining []string
		wantTags      []string
	}{
		{
			name:          "no tags",
			args:          []string{"--local-port", "5432"},
			wantRemaining: []string{"--local-port", "5432"},
		},
		{
			name:          "single tag",
			args:          []string{"--tag", "Environment=prod"},
			wantRemaining: nil,
			wantTags:      []string{"Environment=prod"},
		},
		{
			name:          "multiple tags interspersed",
			args:          []string{"--tag", "Environment=prod", "--local-port", "5432", "--tag", "Team=platform"},
			wantRemaining: []string{"--local-port", "5432"},
			wantTags:      []string{"Environment=prod", "Team=platform"},
		},
		{
			name:          "single-dash tag flag",
			args:          []string{"-tag", "Name=bastion"},
			wantRemaining: nil,
			wantTags:      []string{"Name=bastion"},
		},
		{
			name:          "tag with no following value is dropped silently",
			args:          []string{"--tag"},
			wantRemaining: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			remaining, tags := parseTags(tt.args)
			if !reflect.DeepEqual(remaining, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", remaining, tt.wantRemaining)
			}
			if !reflect.DeepEqual(tags, tt.wantTags) {
				t.Errorf("tags = %v, want %v", tags, tt.wantTags)
			}
		})
	}
}
```

**Verify**: `go test . -run TestParseTags -v` → all subtests pass.

### Step 4: Add golden-output tests for the help-printing functions

Since all 12 `print*Help`/`printUsage` functions write to `os.Stderr` with
`fmt.Fprintf`, the simplest characterization approach is to check for
presence of key substrings (the flag names, the command name) rather than
byte-exact golden files — byte-exact snapshots are brittle against
intentional formatting tweaks, whereas substring checks catch the actual
regression this plan cares about (a flag silently disappearing from its own
help text). Append to `main_test.go`:

```go
func captureStderr(f func()) string {
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	f()

	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	io.Copy(&buf, r)
	return buf.String()
}

func TestHelpFunctionsContainDocumentedFlags(t *testing.T) {
	tests := []struct {
		name         string
		fn           func()
		wantContains []string
	}{
		{"printUsage", printUsage, []string{"ec2", "forward", "ecs", "rds", "fav", "init", "doctor", "upgrade", "--profile", "--region", "--env", "--version"}},
		{"printEC2Help", printEC2Help, []string{"ssh", "rdp", "--tag"}},
		{"printForwardHelp", printForwardHelp, []string{"--local-port", "--remote-port", "--target", "--remote-host", "--tag"}},
		{"printECSHelp", printECSHelp, []string{"logs", "--cluster", "--service"}},
		{"printRDSHelp", printRDSHelp, []string{"--local-port", "--bastion", "--no-bastion", "--tag"}},
		{"printECSLogsHelp", printECSLogsHelp, []string{"--cluster", "--service", "--log-group", "--since", "--no-follow"}},
		{"printEC2SSHHelp", printEC2SSHHelp, []string{"--target", "--user", "--tag"}},
		{"printFavHelp", printFavHelp, []string{"add", "rm"}},
		{"printDoctorHelp", printDoctorHelp, []string{"--profile", "--region"}},
		{"printInitHelp", printInitHelp, []string{"~/.act.json"}},
		{"printEC2RDPHelp", printEC2RDPHelp, []string{"--target", "--local-port", "--key", "--no-open", "--tag"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := captureStderr(tt.fn)
			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("%s output missing expected substring %q\nfull output:\n%s", tt.name, want, output)
				}
			}
		})
	}
}
```

This requires adding `"bytes"`, `"io"`, and `"strings"` to `main_test.go`'s
import block alongside `"reflect"` and `"testing"` (`"os"` is also needed
for `captureStderr`).

**Verify**: `go test . -run TestHelpFunctionsContainDocumentedFlags -v` →
all subtests pass.

### Step 5: Run the full main package test suite

**Verify**: `go test . -v` → all tests pass:
`TestParseGlobalFlags`, `TestHasHelp`, `TestParseTags`,
`TestHelpFunctionsContainDocumentedFlags` (with all their subtests).

### Step 6: Run the full repo test suite

**Verify**: `go test ./...` → exit 0, all packages pass, `main` package now
shows non-zero coverage (`go test ./... -cover` → the `main` package line
should no longer read `0.0% of statements`).

## Test plan

- New file: `main_test.go`, package `main`.
- `TestParseGlobalFlags`: table-driven, covers no-flags, each of the 4 flags
  individually, single-dash variants, a dangling flag with no value, and
  flags interspersed with subcommand-specific args (the realistic case,
  e.g. `--tag` passing through untouched).
- `TestHasHelp`: table-driven, covers empty args, each of the 3 recognized
  help tokens, a non-help first arg, and help appearing in a
  non-first position (must return false — only checked at index 0).
- `TestParseTags`: table-driven, covers no tags, one tag, multiple tags
  interspersed with other flags, single-dash variant, dangling flag.
- `TestHelpFunctionsContainDocumentedFlags`: table-driven over all 12
  help-printing functions (11 tested explicitly; `printVersion` excluded per
  Scope), asserting each documents the flags this plan's author confirmed by
  reading `main.go` are actually parsed by that command's `run*` function.
- Structural pattern: table-driven tests with a `name`/inputs/`want` shape,
  matching the existing convention in `internal/config/config_test.go` and
  `internal/aws/ec2_test.go`.
- Verification: `go test . -v` → all pass; `go test ./...` → all packages
  pass.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `main_test.go` exists with `TestParseGlobalFlags`, `TestHasHelp`,
      `TestParseTags`, `TestHelpFunctionsContainDocumentedFlags`
- [ ] `go test . -v` exits 0, all pass
- [ ] `go test ./...` exits 0, all pass
- [ ] `go test ./... -cover` shows the `main` package (listed as
      `github.com/brunodasilvalenga/act`) with coverage > 0.0%
- [ ] `go build ./...` exits 0
- [ ] `git status` shows only `main_test.go` added, no changes to `main.go`
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any of the 4 functions targeted (`parseGlobalFlags`, `parseTags`,
  `hasHelp`) don't match the excerpts above (drift since this plan was
  written) — re-read `main.go` in full before writing tests against
  assumed behavior.
- A step's verification fails twice after a reasonable fix attempt.
- You find that testing a help function requires capturing `os.Stdout`
  instead of `os.Stderr` — check each `print*Help` function's actual
  `fmt.Fprintf` target before assuming; per the excerpts read during
  planning, all of them use `os.Stderr`, but confirm this yourself for each
  function you test.
- Writing these tests reveals that `parseGlobalFlags`/`parseTags`/`hasHelp`
  don't actually behave as documented in their doc comments (e.g. an edge
  case that surprises you) — if so, that's a real bug finding, not a test-
  writing problem; STOP and report the discrepancy rather than writing a
  test that encodes the "wrong" behavior as correct, and rather than fixing
  the bug yourself (that would be scope creep beyond this plan).

## Maintenance notes

- Any future new global flag or `--tag`-style repeated flag should get a
  new test case added to `TestParseGlobalFlags`/`TestParseTags` rather than
  a new standalone test function — keep the table growing, not the test
  count.
- Any future new subcommand or flag should have its corresponding
  `print*Help` function's expected substrings added to
  `TestHelpFunctionsContainDocumentedFlags`'s table — this is the test that
  would have caught the drift found in plan `005` (README/help text
  mismatch) had it existed sooner, though note this test only checks
  `main.go`'s *own* internal consistency (help text vs. flags actually
  parsed), not `main.go` vs. `README.md` — that cross-file check remains
  manual per plan `005`'s maintenance notes.
- A reviewer should scrutinize: that `captureStderr` correctly restores
  `os.Stderr` even if the wrapped function panics — the current
  implementation does not use `defer` for the restore, which is acceptable
  for this narrow use (none of the 11 tested functions can panic — they're
  simple `Fprintf` calls) but should be flagged if this helper is reused for
  functions that might panic in the future.
