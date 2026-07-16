# Plan 008: Write ~/.act.json with 0600 permissions instead of 0644

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/config/config.go`
> If the file changed since this plan was written, compare the
> "Current state" excerpt against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`~/.act.json` — which stores the user's default AWS profile name, default
region, favorite EC2 instance IDs, and named environment→profile/region
mappings — is written with mode `0644` (world/group-readable) by both
`Init` (`internal/config/config.go:39-50`) and `Save`
(`internal/config/config.go:68-78`). On a shared/multi-user machine, any
other local user can read another user's AWS profile names, region,
favorite instance IDs, and environment naming conventions — a real, if
low-severity, internal-infrastructure disclosure. The fix is a one-character
change in each of the two `os.WriteFile` calls (`0644` → `0600`), which
matches the principle that a user's own config file should not be readable
by other local accounts by default.

## Current state

`internal/config/config.go:39-50` (`Init`):

```go
func Init(profile, region string) error {
	path := ConfigPath()
	cfg := Config{
		DefaultProfile: profile,
		DefaultRegion:  region,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
```

`internal/config/config.go:68-78` (`Save`):

```go
func Save(cfg Config) error {
	path := ConfigPath()
	if path == "" {
		return os.ErrNotExist
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}
```

Both functions are called from various places: `Init` from `main.go:456`
(`runInit`); `Save` from `AddFavorite`/`RemoveFavorite`
(`internal/config/config.go:80-101`), which are called from `main.go:932`
and `main.go:944` (`runFav`'s `add`/`rm` subcommands).

Existing test coverage for this package:
`internal/config/config_test.go` — 138 lines, includes
`TestAddRemoveFavorite` (lines 104-138) which calls `Init` and
`AddFavorite`/`RemoveFavorite` against a temp directory via `overrideHome`
(lines 10-21). This is the exemplar pattern to follow for the new test in
this plan.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (this package) | `go test ./internal/config/... -v` | exit 0, all pass, including new test |
| Full test suite | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `internal/config/config.go` (the two `os.WriteFile` calls in `Init` and
  `Save` — change `0644` to `0600`)
- `internal/config/config_test.go` (add a new test verifying the file mode)

**Out of scope** (do NOT touch, even though they look related):
- Do not add logic to `chmod` an *existing* `~/.act.json` file that was
  previously written with `0644` by an older version of `act` — that's a
  migration concern (upgrading an existing installation's file in place)
  that adds meaningful complexity (detecting the old mode, deciding whether
  to silently tighten it, handling the case where a user intentionally
  loosened it) and is out of scope for this plan. This plan only changes the
  mode used for *new* writes going forward.
- `internal/doctor/doctor.go`'s `checkConfigFile` (lines 184-209) — it only
  calls `os.Stat`/`config.Load`, doesn't write the file; no change needed.
- Any other file in `internal/config/` beyond the two `WriteFile` calls.

## Git workflow

- Branch: `advisor/008-tighten-config-file-permissions`
- Single commit; message style example: `fix: make config tests cross-platform for Windows CI` (commit `4d09420`, for
  precedent on this exact package) — for this change:
  `fix: write ~/.act.json with 0600 instead of 0644`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Tighten `Init`'s file mode

In `internal/config/config.go:49`, change:

```go
	return os.WriteFile(path, append(data, '\n'), 0644)
```

(inside `Init`) to:

```go
	return os.WriteFile(path, append(data, '\n'), 0600)
```

**Verify**: `go build ./internal/config/...` → exit 0.

### Step 2: Tighten `Save`'s file mode

In `internal/config/config.go:77`, change the identical line inside `Save`
from `0644` to `0600` as well.

**Verify**: `go build ./internal/config/...` → exit 0.

### Step 3: Add a test verifying the permission on both write paths

Add to `internal/config/config_test.go` (this repo's tests already handle
Windows cross-platform concerns via `overrideHome` and the `runtime.GOOS`
check at lines 15-19 — follow that same pattern; file mode checks behave
differently on Windows, where Unix permission bits are not fully meaningful,
so skip the assertion on Windows rather than trying to force it):

```go
func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits are not meaningful on Windows")
	}

	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	if err := Init("test-profile", "us-east-1"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	info, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected Init to write config with mode 0600, got %v", perm)
	}

	// Save (exercised indirectly via AddFavorite) must also use 0600.
	if err := AddFavorite("i-999"); err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}
	info, err = os.Stat(ConfigPath())
	if err != nil {
		t.Fatalf("failed to stat config file after Save: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected Save to write config with mode 0600, got %v", perm)
	}
}
```

This uses the existing `overrideHome(t, dir)` helper already defined at
`internal/config/config_test.go:10-21` — do not redefine it.

**Verify**: `go test ./internal/config/... -run TestConfigFilePermissions -v` →
passes (or is skipped, on Windows).

### Step 4: Run the full package test suite

**Verify**: `go test ./internal/config/... -v` → all tests pass, including
the existing `TestResolveProfile`, `TestResolveRegion`,
`TestResolveWithEnvironment`, `TestAddRemoveFavorite`, and the new
`TestConfigFilePermissions`.

### Step 5: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass.

## Test plan

- Extend `internal/config/config_test.go` with `TestConfigFilePermissions`,
  modeled directly after the existing `TestAddRemoveFavorite`
  (`internal/config/config_test.go:104-138`) for its use of `t.TempDir()` +
  `overrideHome`.
- Covers both write paths: `Init` (direct call) and `Save` (exercised
  indirectly via `AddFavorite`, matching how `TestAddRemoveFavorite` already
  exercises it).
- Skips on Windows (`runtime.GOOS == "windows"`), consistent with this
  package's existing Windows-awareness pattern at
  `internal/config/config_test.go:15-19` and the project's stated practice
  (see project memory: "make config tests cross-platform for Windows CI",
  commit `4d09420`).
- Verification: `go test ./internal/config/... -v` → all pass.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n "0644" internal/config/config.go` returns no matches
- [ ] `grep -n "0600" internal/config/config.go` returns 2 matches (one in
      `Init`, one in `Save`)
- [ ] `go build ./...` exits 0
- [ ] `go test ./internal/config/... -v` exits 0, all pass including
      `TestConfigFilePermissions`
- [ ] `go test ./...` exits 0, all packages pass
- [ ] `git status` shows only `internal/config/config.go` and
      `internal/config/config_test.go` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/config/config.go:39-50` or `:68-78` doesn't match
  the excerpts above (drift since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- The test suite reveals that some other code path depends on `~/.act.json`
  being group/world-readable (unlikely, but if you find one, STOP and report
  it rather than silently working around it).

## Maintenance notes

- Any future new `os.WriteFile` call that writes to `~/.act.json` (or any
  new per-user config file this project adds) should use `0600` from the
  start — this plan sets the pattern for the two existing sites.
- A reviewer should scrutinize: whether an existing `~/.act.json` file
  written by an older `act` binary needs a migration path (this plan
  deliberately does not add one — see "Out of scope" above); if that's
  judged necessary later, it should be its own plan, not bundled here.
