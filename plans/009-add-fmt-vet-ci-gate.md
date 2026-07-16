# Plan 009: Add gofmt/go vet gates to CI and Makefile, fix existing drift

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- Makefile .github/workflows/ci.yml internal/doctor/doctor.go`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

Neither `.github/workflows/ci.yml` nor the `Makefile` runs `gofmt` or
`go vet` — CI only runs `go build -v ./...` and `go test -v ./...`
(`.github/workflows/ci.yml:22-26`). This gap already has a real consequence:
running `gofmt -l .` against the repo today flags
`internal/doctor/doctor.go` for a formatting inconsistency (misaligned
comment spacing in a `var (...)` block), and neither CI nor `make` would
have caught it before merge. Without `go vet` either, classes of subtle bugs
(unchecked type assertions, printf-format mismatches, unreachable code) can
land silently. This plan does three things: fixes the one existing drift
instance, adds `fmt`/`vet` Makefile targets, and adds the corresponding CI
step so future drift fails the build instead of accumulating.

## Current state

`gofmt -d internal/doctor/doctor.go` currently outputs (confirmed by running
it):

```diff
--- internal/doctor/doctor.go.orig
+++ internal/doctor/doctor.go
@@ -26,9 +26,9 @@
 }

 var (
-	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
-	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
-	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red
+	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2")) // green
+	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3")) // yellow
+	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1")) // red
)
```

The live lines (`internal/doctor/doctor.go:28-32`):

```go
var (
	passStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))  // green
	warnStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))  // yellow
	failStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))  // red
)
```

each have two spaces before the trailing `//` comment where `gofmt`
expects one.

`Makefile` (full file, 12 lines):

```makefile
BINARY=act

build:
	go build -ldflags "-X main.version=dev" -o $(BINARY) .

install: build
	mv $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

.PHONY: build install clean
```

`.github/workflows/ci.yml:9-27` (the `build` job):

```yaml
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v5

      - uses: actions/setup-go@v6
        with:
          go-version: "1.26"

      - name: Build
        run: go build -v ./...

      - name: Test
        run: go test -v ./...
```

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Check formatting | `gofmt -l .` | (before fix) prints `internal/doctor/doctor.go`; (after fix) prints nothing |
| Apply formatting | `gofmt -w internal/doctor/doctor.go` | exits 0, rewrites file in place |
| Vet | `go vet ./...` | exit 0, no output |
| Build | `go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all pass |
| Makefile fmt target | `make fmt` | exit 0, prints nothing on a clean tree |
| Makefile vet target | `make vet` | exit 0 |

## Scope

**In scope** (the only files you should modify):
- `internal/doctor/doctor.go` (formatting fix only — via `gofmt -w`, no
  manual/semantic edits)
- `Makefile` (add `fmt`, `vet` targets; wire `fmt`/`vet` into an existing or
  new `test`/`check` target if one doesn't exist)
- `.github/workflows/ci.yml` (add `gofmt`/`go vet` steps to the `build` job)

**Out of scope** (do NOT touch, even though they look related):
- Do not introduce `golangci-lint` or any third-party linter — that's a
  larger tooling addition (config file, potentially many more findings to
  triage) not covered by this plan's scope, which is specifically the
  standard-library `gofmt`/`go vet` gate.
- Do not add a `CONTRIBUTING.md` — that's a separate, optional DX
  improvement not bundled into this plan.
- `.github/workflows/release.yml` — this plan only touches `ci.yml`'s build
  job; the release workflow's own action-version lag is tracked separately
  in plan `016`.
- Any other file in `internal/doctor/` beyond the whitespace fix.

## Git workflow

- Branch: `advisor/009-add-fmt-vet-ci-gate`
- Single commit; message style example: `fix: make config tests cross-platform for Windows CI` (commit `4d09420`) for
  fix-shaped commits — for this change:
  `fix: add gofmt/go vet gates to CI and Makefile, fix existing drift`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Fix the existing formatting drift

Run `gofmt -w internal/doctor/doctor.go`. This is a mechanical, automated
fix — do not hand-edit the file.

**Verify**: `gofmt -l .` → prints nothing (no output means no files need
formatting).

**Verify**: `go build ./...` → exit 0 (confirms the formatting fix didn't
change any semantics — `gofmt` never changes program behavior, only
whitespace/style).

### Step 2: Add `fmt` and `vet` targets to the Makefile

Replace the full `Makefile` content with:

```makefile
BINARY=act

build:
	go build -ldflags "-X main.version=dev" -o $(BINARY) .

install: build
	mv $(BINARY) /usr/local/bin/$(BINARY)

clean:
	rm -f $(BINARY)

fmt:
	@test -z "$$(gofmt -l .)" || (echo "The following files are not gofmt'd:"; gofmt -l .; exit 1)

vet:
	go vet ./...

test: fmt vet
	go test ./...

.PHONY: build install clean fmt vet test
```

Note: the `test` target now runs `fmt` and `vet` as prerequisites before
`go test` — this gives a single `make test` entry point that catches all
three classes of issue, matching the spirit of "one-command way to know the
codebase works" (a DX principle this project doesn't currently have via
`make`, only via the bare `go test ./...` invocation used in CI).

**Verify**: `make fmt` → exit 0, no output (the tree is now clean per Step
1).

**Verify**: `make vet` → exit 0, no output.

**Verify**: `make test` → exit 0, runs fmt, vet, then the full test suite,
all passing.

### Step 3: Add the corresponding CI step

In `.github/workflows/ci.yml`, in the `build` job (lines 10-27), add a
`Format check` step and a `Vet` step between the existing `Build` and `Test`
steps:

```yaml
jobs:
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v5

      - uses: actions/setup-go@v6
        with:
          go-version: "1.26"

      - name: Build
        run: go build -v ./...

      - name: Format check
        shell: bash
        run: |
          UNFORMATTED=$(gofmt -l .)
          if [ -n "$UNFORMATTED" ]; then
            echo "The following files are not gofmt'd:"
            echo "$UNFORMATTED"
            exit 1
          fi

      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test -v ./...
```

Note the explicit `shell: bash` on the "Format check" step — this job runs
on a `windows-latest` runner too (per the `strategy.matrix.os` list at line
14), and the default shell there is PowerShell, where this bash-style
`$(...)`/`if [ -n ... ]` syntax would not work. Explicitly requesting `bash`
ensures the step behaves identically across all three OSes in the matrix
(GitHub-hosted Windows runners include Git Bash, which `shell: bash` uses).

**Verify**: `grep -n "Format check" .github/workflows/ci.yml` → returns a
match. (Full CI validation happens on push/PR per this repo's trigger config
at `.github/workflows/ci.yml:3-7` — you cannot run the GitHub Actions
workflow locally as part of this plan's verification; the local `make test`
equivalence in Step 2 is your pre-push confidence check.)

### Step 4: Confirm end-to-end locally

**Verify**: `gofmt -l .` → prints nothing.

**Verify**: `go vet ./...` → exit 0, no output.

**Verify**: `go test ./...` → exit 0, all pass.

## Test plan

No new Go test files are needed — this plan adds *tooling gates*
(formatting/vet checks), not application logic. The verification is that
`gofmt -l .` and `go vet ./...` both report clean on the current codebase
after Step 1's fix, and that the new CI steps and Makefile targets correctly
invoke these same checks (confirmed by running them locally in Step 4, since
the actual GitHub Actions run only happens on push/PR).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` exits 0 with no output
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `Makefile` contains `fmt`, `vet`, and `test` targets, and `.PHONY`
      includes all of `build install clean fmt vet test`
- [ ] `make test` exits 0
- [ ] `.github/workflows/ci.yml`'s `build` job contains a "Format check" step
      and a "Vet" step, both before the existing "Test" step
- [ ] `git status` shows only `internal/doctor/doctor.go`, `Makefile`, and
      `.github/workflows/ci.yml` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `gofmt -l .` reports a *different* file than `internal/doctor/doctor.go`
  before you make any change (drift since this plan was written) — if so,
  fix whatever `gofmt -l .` actually reports via `gofmt -w`, not just the
  file named in this plan.
- A step's verification fails twice after a reasonable fix attempt.
- `go vet ./...` reports any issue on the current codebase (this plan
  assumed it's currently clean, based on the recon that ran `go build ./...`
  successfully — vet issues are a superset of build issues in some cases) —
  if `go vet` reports something, STOP and report the exact output rather
  than attempting to fix vet-flagged application logic yourself; that would
  expand this plan's scope beyond a tooling-gate addition.

## Maintenance notes

- Any future PR that fails the new "Format check" or "Vet" CI step should be
  fixed by running `gofmt -w <file>` or addressing the vet finding directly
  — not by removing the CI step.
- A reviewer should scrutinize: that the Windows CI runner path for "Format
  check" actually works (the `shell: bash` directive is the key detail to
  verify doesn't regress if this step is edited later).
- This plan deliberately stops short of adding `golangci-lint` — if the team
  wants deeper static analysis later, that should be a separate plan with
  its own config-file review and initial findings triage (golangci-lint
  typically surfaces many findings on first run that need triaging, unlike
  `gofmt`/`go vet` which are much narrower).
