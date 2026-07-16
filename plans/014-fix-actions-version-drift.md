# Plan 014: Align release.yml action versions with ci.yml

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- .github/workflows/ci.yml .github/workflows/release.yml`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: dx
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`.github/workflows/ci.yml` (which runs on every push/PR to `main`) uses
`actions/checkout@v5` and `actions/setup-go@v6`. `.github/workflows/
release.yml` (which runs when a `v*` tag is pushed and produces the actual
shipped, checksummed binaries via GoReleaser) uses the older
`actions/checkout@v4` and `actions/setup-go@v5` — in two separate places
(the `test` job and the `release` job each have their own `setup-go` step).
This means the pipeline that builds and tests every PR runs on a different,
newer set of GitHub Actions than the pipeline that actually builds the
release artifacts users download. That's exactly the kind of environment
drift that causes "works in CI, fails/differs in release" surprises — a
build passing on `main` via `ci.yml` doesn't guarantee the same result on
the `release.yml` runner. The fix is a mechanical version-pin bump so both
workflows use the same action versions.

## Current state

`.github/workflows/ci.yml:9-20` (the `build` job's checkout/setup-go steps):

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
```

`.github/workflows/release.yml` (full file):

```yaml
name: Release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write

jobs:
  test:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Build
        run: go build -v ./...

      - name: Test
        run: go test -v ./...

  release:
    needs: test
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"

      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v6
        with:
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GH_PAT }}
```

Four occurrences to fix in `release.yml`: `actions/checkout@v4` appears
twice (line 18, in the `test` job; line 34, in the `release` job — note the
`release` job's checkout also has `with: fetch-depth: 0`, which must be
preserved), and `actions/setup-go@v5` appears twice (line 20, in the `test`
job; line 38, in the `release` job).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Confirm current versions | `grep -n "actions/checkout\|actions/setup-go" .github/workflows/*.yml` | shows v5/v6 in ci.yml, v4/v5 in release.yml (before fix) |
| Confirm fix | same command | shows v5/v6 in both files (after fix) |
| YAML sanity check | `go build ./...` | exit 0 (sanity check only — this change doesn't touch Go code) |

This is a workflow-file-only change; there is no way to fully test a GitHub
Actions workflow file locally without actually running it in GitHub Actions
(which only happens on a real push/tag). The verification here is limited to
confirming the version strings and YAML structure are correct.

## Scope

**In scope** (the only files you should modify):
- `.github/workflows/release.yml` (bump `actions/checkout@v4` → `@v5` in
  both jobs; bump `actions/setup-go@v5` → `@v6` in both jobs)

**Out of scope** (do NOT touch, even though they look related):
- `.github/workflows/ci.yml` — already on the newer versions; no change
  needed, this plan brings `release.yml` up to match it, not the other way
  around.
- `goreleaser/goreleaser-action@v6` (`release.yml`'s release job) — this is
  a different action entirely (GoReleaser, not checkout/setup-go) and is
  not part of the version-drift finding; leave it unchanged.
- `.goreleaser.yml` — no changes needed.
- Any `go-version` value change — this plan is about the *action* versions
  (`actions/checkout`, `actions/setup-go`), not the Go toolchain version
  itself (that's plan 001's concern, if it hasn't already run — if plan 001
  changed `go-version: "1.26"` to something more specific like
  `"1.26.5"` in both files, preserve that value; this plan only touches the
  `uses:` version pins, not the `with: go-version:` values).

## Git workflow

- Branch: `advisor/014-fix-actions-version-drift`
- Single commit; message style example: `ci: add test gate to release workflow` (commit `c344f7e`, direct precedent
  for a `release.yml`-focused change) — for this change:
  `ci: align release.yml action versions with ci.yml`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Bump `actions/checkout` in the `test` job

In `.github/workflows/release.yml`, change line 18 from:

```yaml
      - uses: actions/checkout@v4
```

to:

```yaml
      - uses: actions/checkout@v5
```

(This is the first `steps:` entry, inside the `test:` job, with no `with:`
block — do not confuse it with the second occurrence in Step 3, which has a
`with: fetch-depth: 0` block attached.)

**Verify**: `grep -n "actions/checkout" .github/workflows/release.yml` →
shows both occurrences; confirm the first one now reads `@v5`.

### Step 2: Bump `actions/setup-go` in the `test` job

Change line 20 from:

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
```

to:

```yaml
      - uses: actions/setup-go@v6
        with:
          go-version: "1.26"
```

(Preserve the `go-version` value exactly as it currently is in this file —
do not change it as part of this plan, even if it differs from `ci.yml`'s
current value at the time you run this, unless plan 001 already
synchronized them.)

**Verify**: `grep -n "actions/setup-go" .github/workflows/release.yml` →
shows both occurrences; confirm the first one now reads `@v6`.

### Step 3: Bump `actions/checkout` in the `release` job

Change (originally around line 34):

```yaml
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
```

to:

```yaml
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
```

Preserve the `with: fetch-depth: 0` block exactly — this is required by
GoReleaser to see the full git history/tags for changelog generation; do
not remove or alter it.

**Verify**: `grep -n "actions/checkout" .github/workflows/release.yml` →
both occurrences now read `@v5`.

### Step 4: Bump `actions/setup-go` in the `release` job

Change (originally around line 38):

```yaml
      - uses: actions/setup-go@v5
        with:
          go-version: "1.26"
```

to:

```yaml
      - uses: actions/setup-go@v6
        with:
          go-version: "1.26"
```

**Verify**: `grep -n "actions/setup-go" .github/workflows/release.yml` →
both occurrences now read `@v6`.

### Step 5: Confirm both workflow files now match on action versions

**Verify**: `grep -n "actions/checkout\|actions/setup-go" .github/workflows/ci.yml .github/workflows/release.yml` →
`ci.yml` shows `actions/checkout@v5` (×1) and `actions/setup-go@v6` (×1);
`release.yml` shows `actions/checkout@v5` (×2) and `actions/setup-go@v6`
(×2). All four version strings across both files now agree.

### Step 6: Sanity-check the repo is otherwise unaffected

**Verify**: `go build ./...` → exit 0 (this change touches no Go source, so
this just confirms nothing else was accidentally modified).

**Verify**: `git status` → only `.github/workflows/release.yml` modified.

## Test plan

No automated test applies to a GitHub Actions YAML version bump — there is
no local GitHub Actions runner in this repo's toolchain. The verification is
purely the `grep` checks in Steps 1-5, confirming the exact version strings
in both files now match, plus the `git status`/`go build` sanity check in
Step 6 confirming no unrelated file was touched. The real end-to-end
validation happens the next time a `v*` tag is pushed and `release.yml`
actually runs on GitHub's infrastructure — that is outside this plan's
scope to trigger or observe.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -c "actions/checkout@v5" .github/workflows/release.yml` returns
      `2`
- [ ] `grep -c "actions/setup-go@v6" .github/workflows/release.yml` returns
      `2`
- [ ] `grep -c "actions/checkout@v4" .github/workflows/release.yml` returns
      `0`
- [ ] `grep -c "actions/setup-go@v5" .github/workflows/release.yml` returns
      `0`
- [ ] The `release` job's checkout step still has `with: fetch-depth: 0`
      attached
- [ ] `go build ./...` exits 0
- [ ] `git status` shows only `.github/workflows/release.yml` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `.github/workflows/release.yml`'s current content doesn't match the full
  file excerpt above (drift since this plan was written) — re-read the
  whole file before editing, since a YAML structural change (e.g. reordered
  steps) could make the line-based instructions above inapplicable.
- A step's verification fails twice after a reasonable fix attempt.
- You find `ci.yml` itself has changed to a *different* version than
  `@v5`/`@v6` since this plan was written (re-check
  `grep -n "actions/checkout\|actions/setup-go" .github/workflows/ci.yml`
  yourself before Step 1) — if so, match whatever `ci.yml` currently uses,
  not the specific `@v5`/`@v6` versions hardcoded in this plan's steps.

## Maintenance notes

- Going forward, any bump to `actions/checkout` or `actions/setup-go` in
  `ci.yml` should be mirrored in `release.yml` in the same PR to prevent
  this drift from recurring — there is no automated check enforcing this
  (a lint rule for GitHub Actions version consistency across workflow files
  would be a further, separate improvement, not covered by this plan).
- A reviewer should scrutinize: that the `release` job's `fetch-depth: 0`
  setting survived the version bump — losing it would silently break
  GoReleaser's changelog generation on the next release.
