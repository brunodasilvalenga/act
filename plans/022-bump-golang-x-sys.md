# Plan 022: Bump `golang.org/x/sys` past v0.44.0 to resolve GO-2026-5024

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- go.mod go.sum`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: migration
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`go.mod` currently pins `golang.org/x/sys v0.36.0` as an indirect dependency (pulled in transitively through `bubbletea`, `x/term`, and `cancelreader`). This version is affected by advisory **GO-2026-5024**, an integer overflow in `NewNTUnicodeString` in `golang.org/x/sys/windows`, fixed upstream in v0.44.0. `act` ships real Windows binaries — `.goreleaser.yml` lists `windows` as a build target and CI (`.github/workflows/ci.yml`) builds and tests on `windows-latest` — so this isn't a hypothetical platform. Today `govulncheck` confirms the vulnerable symbol isn't reachable from this code (0 active exploit path), but this exact finding will keep resurfacing on every future dependency/security scan, and it could become live the moment `bubbletea`/`x/term`/`cancelreader` start calling more of `x/sys/windows`'s internals. Bumping now is a pure version-pin update with no source code changes — the fix is cheap, so there's no reason to carry a known advisory in `go.sum` when the remediation is a one-line dependency bump.

## Current state

- `go.mod` (repo root) — the full file, currently 28 lines:
  ```
  module github.com/brunodasilvalenga/act

  go 1.26.5

  require (
  	github.com/charmbracelet/bubbletea v1.3.10
  	github.com/charmbracelet/lipgloss v1.1.0
  )

  require (
  	github.com/aymanbagabas/go-osc52/v2 v2.0.1 // indirect
  	github.com/charmbracelet/colorprofile v0.2.3-0.20250311203215-f60798e515dc // indirect
  	github.com/charmbracelet/x/ansi v0.10.1 // indirect
  	github.com/charmbracelet/x/cellbuf v0.0.13-0.20250311204145-2c3ea96c31dd // indirect
  	github.com/charmbracelet/x/term v0.2.1 // indirect
  	github.com/erikgeiser/coninput v0.0.0-20211004153227-1c3628e74d0f // indirect
  	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
  	github.com/mattn/go-isatty v0.0.20 // indirect
  	github.com/mattn/go-localereader v0.0.1 // indirect
  	github.com/mattn/go-runewidth v0.0.16 // indirect
  	github.com/muesli/ansi v0.0.0-20230316100256-276c6243b2f6 // indirect
  	github.com/muesli/cancelreader v0.2.2 // indirect
  	github.com/muesli/termenv v0.16.0 // indirect
  	github.com/rivo/uniseg v0.4.7 // indirect
  	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
  	golang.org/x/sys v0.36.0 // indirect
  	golang.org/x/text v0.3.8 // indirect
  )
  ```
  The line that needs to change is `golang.org/x/sys v0.36.0 // indirect` (line 26).

- `go.sum` — grep result for the current pinned lines:
  ```
  golang.org/x/sys v0.0.0-20210809222454-d867a43fc93e/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
  golang.org/x/sys v0.6.0/go.mod h1:oPkhp1MJrh7nUepCBck5+mAzfO9JrbApNNgaTdGDITg=
  golang.org/x/sys v0.36.0 h1:KVRy2GtZBrk1cBYA7MKu5bEZFxQk4NIDV6RLVcC8o0k=
  golang.org/x/sys v0.36.0/go.mod h1:OgkHotnGiDImocRcuBABYBEXf8A9a87e/uXjp9XT3ks=
  ```
  These `v0.36.0` lines will be replaced by `go mod tidy` after the bump (the older `v0.0.0-...`/`v0.6.0` lines are unrelated leftover `go.mod`-hash-only entries from transitive resolution history and may or may not disappear — do not hand-edit `go.sum`, only regenerate it via `go mod tidy`).

- `.goreleaser.yml` (repo root, full file, ~44 lines) — confirms `windows` is an actual shipped build target, not speculative:
  ```yaml
  builds:
    - binary: act
      env:
        - CGO_ENABLED=0
      goos:
        - linux
        - darwin
        - windows
      goarch:
        - amd64
        - arm64
  ```

- `.github/workflows/ci.yml` — the `build` job matrix is `os: [ubuntu-latest, macos-latest, windows-latest]` and runs `go build -v ./...`, `gofmt -l .`, `go vet ./...`, `go test -v ./...` on all three OSes. This confirms Windows is a real CI target, but the executor of this plan will only be able to run these commands locally on their own OS — full cross-platform confirmation happens only when this change is pushed and CI runs on all three matrix entries. Since this plan says not to push/PR unless told, that full confirmation is explicitly deferred to whoever merges this branch (see Done criteria).

- `govulncheck` output against the current module graph — run on 2026-07-20 via `go run golang.org/x/vuln/cmd/govulncheck@latest -show verbose ./...` from the repo root, real output (abridged to the relevant sections):
  ```
  Govulncheck scanned the following 17 modules and the go1.26.5 standard library:
    github.com/brunodasilvalenga/act
    ... (bubbletea, lipgloss, colorprofile, x/ansi, x/cellbuf, x/term, go-colorful,
         go-isatty, go-runewidth, muesli/ansi, muesli/cancelreader, muesli/termenv,
         rivo/uniseg, xo/terminfo, go-osc52) ...
    golang.org/x/sys@v0.36.0

  === Symbol Results ===

  No vulnerabilities found.

  === Package Results ===

  No other vulnerabilities found.

  === Module Results ===

  Vulnerability #1: GO-2026-5024
      Invoking integer overflow in NewNTUnicodeString in golang.org/x/sys/windows
    More info: https://pkg.go.dev/vuln/GO-2026-5024
    Module: golang.org/x/sys
      Found in: golang.org/x/sys@v0.36.0
      Fixed in: golang.org/x/sys@v0.44.0
      Platforms: windows

  Your code is affected by 0 vulnerabilities.
  This scan also found 0 vulnerabilities in packages you import and 1
  vulnerability in modules you require, but your code doesn't appear to call these
  vulnerabilities.
  ```
  This is the exact advisory ID (`GO-2026-5024`) and affected package (`golang.org/x/sys/windows`, symbol `NewNTUnicodeString`) the fix needs to eliminate from the "modules you require" list.

- Direct-import check — confirmed via `grep -rn "golang.org/x/sys" --include="*.go" .` from the repo root: **zero matches**. `golang.org/x/sys` is not imported directly anywhere in this repo's own `.go` files; it is pulled in purely transitively (by `bubbletea`, `charmbracelet/x/term`, and `muesli/cancelreader`). This means the blast radius of this bump is entirely bounded by whether those three transitive dependencies still build and pass their own tests against the newer `x/sys` — which `go build ./...` and `go test ./...` will directly verify. There is no first-party code path in `act` that needs to be touched or reviewed for `x/sys` API changes.

- Available versions at plan-writing time — `go list -m -versions golang.org/x/sys` showed (tail of the list): `... v0.43.0 v0.44.0 v0.45.0 v0.46.0 v0.47.0`. Latest at plan-writing time is `v0.47.0`, but the executor should not hardcode this — let `go get golang.org/x/sys@latest` resolve the actual latest at execution time, since a newer patch may exist by then. The hard requirement is **`>= v0.44.0`** (the version GO-2026-5024 says is fixed).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Bump the dependency | `go get golang.org/x/sys@latest` | exit 0; `go.mod`'s `golang.org/x/sys` line shows a version `>= v0.44.0` |
| Verify resolved version | `go list -m golang.org/x/sys` | prints `golang.org/x/sys vX.Y.Z` with X.Y.Z >= 0.44.0 |
| Tidy modules | `go mod tidy` | exit 0, no output (or only routine "downloading" messages) |
| Build | `go build -v ./...` | exit 0, no errors |
| Format check | `gofmt -l .` | exit 0, empty output |
| Vet | `go vet ./...` | exit 0, no output |
| Test | `go test -v ./...` | exit 0, all tests `PASS` |
| Makefile test target (fmt+vet+test) | `make test` | exit 0 |
| Re-run vuln scan | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | output no longer lists `GO-2026-5024` anywhere |

(These match the repo's `Makefile` targets `fmt`, `vet`, `test` and the CI steps in `.github/workflows/ci.yml`.)

## Scope

**In scope** (the only files you should modify):
- `go.mod`
- `go.sum`

**Out of scope** (do NOT touch, even though they look related):
- Any application `.go` file — this plan makes zero source code changes. It is confirmed (see "Current state") that `golang.org/x/sys` has no direct import in this repo, so there is nothing in first-party code to update.
- `.goreleaser.yml`, `.github/workflows/ci.yml` — read-only references for this plan; no change needed to build/CI config to accomplish the bump.
- Any other dependency's version (e.g. `bubbletea`, `x/text`, `charmbracelet/x/term`) — only bump `golang.org/x/sys`. If `go mod tidy` naturally moves other indirect version pins as a side effect of dependency graph resolution, that is expected and fine to leave as-is; do not manually bump anything else.

## Git workflow

- Branch: `advisor/022-bump-golang-x-sys`
- Commit message style: this repo uses Conventional Commits (see `git log --oneline`, e.g. `0f142b4 fix: let act doctor run without AWS CLI already installed`, `e7b1861 fix: bump go toolchain to close reachable stdlib CVEs`). Since this is a dependency version bump with a security angle (not a new feature and not a behavioral bug in `act` itself), use a `fix:` prefix to match the precedent set by the toolchain-bump commit (`e7b1861`), e.g.:
  ```
  fix: bump golang.org/x/sys to resolve GO-2026-5024
  ```
- Single commit for the whole change (`go.mod` + `go.sum` together) is fine — there is only one logical unit of work here.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Bump `golang.org/x/sys`

From the repo root, run:
```
go get golang.org/x/sys@latest
```

**Verify**: `go list -m golang.org/x/sys` → prints `golang.org/x/sys vX.Y.Z` where X.Y.Z is `>= v0.44.0` (e.g. `v0.47.0` or newer). If the resolved version is somehow below `v0.44.0` (should not happen with `@latest`, but check), STOP — see "STOP conditions".

### Step 2: Tidy modules

```
go mod tidy
```

**Verify**: exit code 0. Then confirm `go.mod`'s indirect-requires block still has exactly one `golang.org/x/sys` line and it is `>= v0.44.0`:
```
grep "golang.org/x/sys" go.mod
```
Expected: one line, e.g. `golang.org/x/sys v0.47.0 // indirect` (exact patch version may differ).

### Step 3: Build, format, vet, test

Run each of these and confirm the expected result before moving on:
```
go build -v ./...
gofmt -l .
go vet ./...
go test -v ./...
```
Or equivalently, `make test` (runs `fmt`, `vet`, then `test` per the Makefile).

**Verify**: `go build -v ./...` exits 0; `gofmt -l .` prints nothing; `go vet ./...` exits 0 with no output; `go test -v ./...` exits 0 and every test line shows `PASS` (no `FAIL`).

### Step 4: Re-run govulncheck and confirm the advisory is gone

```
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

**Verify**: the output's "Module Results" / summary no longer mentions `GO-2026-5024` or `golang.org/x/sys` in the vulnerability list. The expected new summary is either "No vulnerabilities found." outright, or a summary line stating 0 vulnerabilities in modules you require (down from the prior "1 vulnerability in modules you require"). If `GO-2026-5024` (or any new `golang.org/x/sys` advisory) still appears, STOP — see "STOP conditions".

### Step 5: Review the diff and commit

```
git status
git diff go.mod go.sum
```

**Verify**: `git status` shows only `go.mod` and `go.sum` as modified (no other files touched). Then commit:
```
git checkout -b advisor/022-bump-golang-x-sys
git add go.mod go.sum
git commit -m "fix: bump golang.org/x/sys to resolve GO-2026-5024"
```

**Verify**: `git log --oneline -1` shows the new commit on branch `advisor/022-bump-golang-x-sys`.

## Test plan

- No new tests are needed — this plan makes no source code changes, so there is no new behavior to characterize. The existing test suite (`go test -v ./...`) is the full regression check: if any of `bubbletea`, `charmbracelet/x/term`, or `muesli/cancelreader` broke against the newer `golang.org/x/sys`, the existing tests (which exercise `internal/tui`, `internal/doctor`, `internal/updater`, `internal/config`, `internal/aws`) would be expected to surface it via build failure or test failure, since those packages depend on the bubbletea/term stack.
- Verification: `go test -v ./...` → all existing tests pass, same pass/fail set as before the bump (no new failures introduced).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go list -m golang.org/x/sys` prints a version `>= v0.44.0`
- [ ] `go build -v ./...` exits 0
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` exits 0 with no output
- [ ] `go test -v ./...` exits 0, no `FAIL` lines
- [ ] `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` output no longer contains `GO-2026-5024`
- [ ] `git status` / `git diff --stat` shows only `go.mod` and `go.sum` changed
- [ ] `plans/README.md` status row for plan 022 updated (unless a reviewer told you they own the index)
- [ ] Local build/test/vet/fmt pass on the executor's own OS is sufficient to close this plan; full cross-platform confirmation (Windows/macOS/Linux via the CI matrix in `.github/workflows/ci.yml`) only happens once this branch is pushed and CI runs — that confirmation is deferred to whoever merges this branch, since this plan does not push or open a PR.

## STOP conditions

Stop and report back (do not improvise) if:

- The current `golang.org/x/sys` line in `go.mod` is not `v0.36.0 // indirect` (i.e. the codebase has drifted since this plan was written — someone already bumped it, or bumped it to something unexpected). Compare against the "Current state" excerpt before making any change.
- `go get golang.org/x/sys@latest` resolves to a version below `v0.44.0` (should not happen, but if the module proxy or go.sum state produces this, the fix is incomplete).
- `go build ./...` or `go test ./...` fails after the bump — this would mean `bubbletea`, `charmbracelet/x/term`, or `muesli/cancelreader` are incompatible with the newer `golang.org/x/sys` API. Do not attempt to patch around this by pinning a different intermediate version without reporting it first; this is a signal the dependency chain needs a coordinated bump (e.g. bubbletea itself may also need updating), which is outside this plan's scope.
- `govulncheck` still reports `GO-2026-5024` (or any `golang.org/x/sys` advisory) after the bump and `go mod tidy`.
- The fix appears to require touching any `.go` file (it should not — this repo has no direct import of `golang.org/x/sys`, confirmed via `grep -rn "golang.org/x/sys" --include="*.go" .` returning zero matches at plan-writing time). If that grep now returns matches, the "Current state" assumption is false — stop and report.
- A step's verification fails twice after a reasonable fix attempt (e.g. retrying `go mod tidy` once after clearing the module cache).

## Maintenance notes

- This is a pure indirect-dependency version bump with no first-party code changes, so the main thing a reviewer should scrutinize is the `go.sum` diff (should be a clean, tooling-generated diff — no hand-edits) and that `go.mod`'s only changed line is the `golang.org/x/sys` version.
- Because `golang.org/x/sys` is pulled in transitively by `bubbletea`, `charmbracelet/x/term`, and `muesli/cancelreader`, any future bump of those three should double check `golang.org/x/sys`'s resolved version hasn't regressed below `v0.44.0` — run `go list -m golang.org/x/sys` after any bubbletea-family dependency update as a quick sanity check.
- If a future `govulncheck` run surfaces a new advisory in `golang.org/x/sys/windows` (or any transitive Windows-specific package), treat it with the same urgency as this one: `act` genuinely ships Windows binaries per `.goreleaser.yml`, so Windows-only advisories in the dependency tree are not safely ignorable just because they don't show up as "reachable" today — bubbletea/term/cancelreader's own internal usage of `x/sys/windows` can change between their releases and make a previously-unreachable symbol reachable.
- No follow-up work is deferred out of this plan — it is self-contained and complete once the bump lands.
