# Plan 001: Bump Go toolchain to close 3 reachable stdlib CVEs

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- go.mod .github/workflows/ci.yml .github/workflows/release.yml`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

Running `govulncheck ./...` against this repo (go1.26.3, darwin/arm64) reports
3 Go standard-library vulnerabilities that are reachable from this binary's
actual call graph — not just present in the toolchain but exercised by code
this CLI runs:

- **GO-2026-5856** (`crypto/tls` Encrypted Client Hello privacy leak) — reached
  via `internal/updater/updater.go:45` (`http.Get` inside `Upgrade`), fixed in
  `go1.26.5`.
- **GO-2026-5039** (`net/textproto` unescaped error inputs) — reached via
  `internal/updater/updater.go:158` (`io.ReadAll` inside `verifyChecksum`),
  fixed in `go1.26.4`.
- **GO-2026-5037** (`crypto/x509` inefficient hostname parsing) — reached via
  `internal/aws/rdp_unix.go:51` (`aws.StartRDP` calling `fmt.Sprint` which
  eventually calls `x509.HostnameError.Error`), fixed in `go1.26.4`.

All three are pure toolchain-version issues (no application code is at
fault), and the fix is a version bump plus re-verification — no source
changes are needed. This is the cheapest, lowest-risk, highest-confidence
item in the whole audit: bump the pinned Go version everywhere it's declared,
rebuild, and confirm `govulncheck` reports clean.

## Current state

- `go.mod:3` — `go 1.26.3`
- `.github/workflows/ci.yml:18-20`:
  ```yaml
        - uses: actions/setup-go@v6
          with:
            go-version: "1.26"
  ```
- `.github/workflows/release.yml:20-22` and `:38-40` (two separate steps, one
  in the `test` job, one in the `release` job):
  ```yaml
        - uses: actions/setup-go@v5
          with:
            go-version: "1.26"
  ```

Note: `ci.yml`'s `go-version: "1.26"` (no patch component) already floats to
the latest available 1.26.x on GitHub's runners, so CI builds may already be
using a patched Go — but the pinned `go.mod` directive (`go 1.26.3`) is what
matters for local/reproducible builds and is the actual toolchain version
this repo's `go.sum`/build declares as its minimum. Fixing `go.mod` is the
part that's actually broken; the CI YAML files are already using an
unpinned minor version so no change is strictly required there, but bump
them too for clarity/consistency with the new floor.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Check current vulnerabilities | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | Exits non-zero, lists 3 vulnerabilities (before fix) |
| Build | `go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all packages pass |
| Re-check after fix | `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` | exit 0, "No vulnerabilities found." (or only non-reachable ones under "packages you import"/"modules you require") |

`govulncheck` requires network access to fetch the vulnerability database and
to download the tool itself on first run — this is a read-only diagnostic
command, not a mutation of the working tree.

## Scope

**In scope** (the only files you should modify):
- `go.mod` (the `go` directive line)
- `.github/workflows/ci.yml` (the `go-version` value, if bumping)
- `.github/workflows/release.yml` (the `go-version` value, if bumping — two occurrences)

**Out of scope** (do NOT touch, even though they look related):
- `go.sum` — do not run `go mod tidy` or otherwise regenerate; the dependency
  set is unaffected by a toolchain patch bump. If your local `go` toolchain
  auto-updates and touches `go.sum`, revert that part.
- Any application source file (`main.go`, `internal/**/*.go`) — none of the
  3 CVEs require an application code change, only a toolchain bump.
- `.goreleaser.yml` — does not pin a Go version itself; it uses whatever Go
  is installed in the release CI runner (already addressed via
  `release.yml`).

## Git workflow

- Branch: `advisor/001-bump-go-toolchain`
- Single commit for this plan; message style example from `git log`:
  `fix: bump CI go-version to 1.26 to match go.mod` (see commit `ffbc9cf`) —
  follow the same `type: description` convention, e.g.
  `fix: bump go toolchain to close reachable stdlib CVEs`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Confirm the current vulnerable baseline

Run `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` and confirm the
output lists `GO-2026-5856`, `GO-2026-5039`, and `GO-2026-5037` as reachable
("Your code is affected by 3 vulnerabilities from the Go standard library").

**Verify**: command exits with `exit status 3` and prints all three GO-IDs
above under "=== Symbol Results ===".

If the output differs (a different set of vulnerabilities, or zero — because
a newer Go release already fixed them and your local toolchain is newer than
`go1.26.3`), STOP and report what you found instead of proceeding — the fix
target may have changed since this plan was written.

### Step 2: Determine the target Go version

Each CVE's "Fixed in" version:
- GO-2026-5856 → `crypto/tls@go1.26.5`
- GO-2026-5039 → `net/textproto@go1.26.4`
- GO-2026-5037 → `crypto/x509@go1.26.4`

The target version must be `1.26.5` or later to close all three. Check
whether a Go release newer than `1.26.5` exists at the time you run this
(e.g. `go1.26.6`, or a `1.27.x` release) — if so, prefer the latest patch
release in the `1.26.x` line unless the operator's environment already has a
newer major/minor installed, in which case match what's installed. Do not
jump to a new minor version (e.g. `1.27`) unless it's already the toolchain
installed locally — that's a larger change than this plan covers.

**Verify**: `go version` on your machine reports the version you intend to
require; if it's older than `1.26.5`, STOP and report — you cannot verify
the fix without a toolchain that actually contains it.

### Step 3: Update `go.mod`

Change line 3 from:
```
go 1.26.3
```
to:
```
go 1.26.5
```
(substitute the actual target version determined in Step 2 if different).

**Verify**: `go build ./...` → exit 0.

### Step 4: Update CI workflow pins (optional but recommended for consistency)

In `.github/workflows/ci.yml:20`, change `go-version: "1.26"` to
`go-version: "1.26.5"` (or your target version) to pin explicitly rather than
floating. Do the same in `.github/workflows/release.yml:22` and `:40` (both
occurrences — the `test` job and the `release` job each have their own
`setup-go` step).

**Verify**: `grep -n 'go-version' .github/workflows/ci.yml .github/workflows/release.yml`
shows the new version string in all three locations.

### Step 5: Re-run tests and vulnerability check

**Verify**:
1. `go test ./...` → exit 0, all packages pass (matches baseline: `go test ./...` currently reports 16 tests passed across 6 packages — do not expect a different count from this change).
2. `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` → exit 0, and no longer lists GO-2026-5856, GO-2026-5039, or GO-2026-5037 as reachable.

## Test plan

No new test files are needed — this is a toolchain version bump with no
application code change. The existing test suite (`go test ./...`) is the
regression check: it must continue to pass unchanged. The verification gate
specific to this plan is the `govulncheck` re-run in Step 5, which is the
only way to confirm the fix actually took effect.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go.mod:3` declares a `go` version ≥ `1.26.5`
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, same pass count as before (16 tests, 6 packages)
- [ ] `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` exits 0 (clean) or no longer lists GO-2026-5856/5039/5037 as reachable
- [ ] `git status` shows only `go.mod` and (optionally) the two CI workflow files modified — `go.sum` unchanged
- [ ] `plans/README.md` status row for 001 updated

## STOP conditions

Stop and report back (do not improvise) if:

- Step 1's baseline check doesn't reproduce the 3 CVEs described above (the
  vulnerability landscape has changed since this plan was written).
- No Go toolchain ≥ `1.26.5` is available in your environment to verify the
  fix (do not bump `go.mod` to a version you can't actually test against).
- `go build ./...` or `go test ./...` fails after the bump for any reason
  unrelated to the version string itself — a toolchain bump should not change
  build/test behavior; if it does, that's a signal something else is wrong.
- `go.sum` changes as a side effect of running any command in this plan —
  revert it; this plan's scope does not include dependency changes.

## Maintenance notes

- Future dependency bumps (bubbletea, lipgloss) are unrelated to this plan
  and should be handled separately with their own `go.sum` diff review.
- If a future stdlib CVE is announced, the same pattern applies: run
  `govulncheck ./...`, check reachability (not just presence), and bump the
  `go.mod` `go` directive and both CI workflow files together to avoid the
  drift flagged in plan `003` (release.yml action version lag).
- A reviewer should scrutinize: that `go.sum` truly didn't change, and that
  the CI workflow version strings (if bumped) match `go.mod` exactly to avoid
  reintroducing the kind of environment drift noted in plan `003`.
