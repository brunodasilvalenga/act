# Plan 004: Make self-upgrade binary replacement atomic

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/updater/updater.go`
> If the file changed since this plan was written, compare the
> "Current state" excerpt against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`replaceBinary` (`internal/updater/updater.go:196-219`) deletes the
currently-running binary (`os.Remove(dst)`) *before* creating and writing the
replacement (`os.OpenFile(dst, ...)`). If `OpenFile` or the subsequent
`io.Copy` fails for any reason — permission error, disk full, read-only
filesystem remount, antivirus lock on Windows — the function returns an
error, but `dst` (the `act` binary itself) has already been unlinked. The
user is left with no `act` executable at all: `command not found`, requiring
a full manual reinstall for a CLI whose only self-repair mechanism is
itself. The fix is to write the new binary to a temp file in the same
directory as `dst`, then atomically `os.Rename` it over `dst` — an operation
that never leaves the destination path in a missing/half-written state on
the same filesystem.

## Current state

`internal/updater/updater.go:196-219` (full `replaceBinary` function):

```go
func replaceBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer srcFile.Close()

	// Remove old binary first (handles "text file busy" on some systems)
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove old binary: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to write new binary: %w", err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy new binary: %w", err)
	}

	return nil
}
```

The comment on line 203 ("handles 'text file busy' on some systems")
explains *why* the old binary is removed first: on Unix, you cannot
overwrite the inode of a currently-executing binary via `O_TRUNC` in some
configurations, so removing the directory entry first (which doesn't affect
the still-running process's open file descriptor) is the correct Unix
approach. That reasoning is still valid and must be preserved — the fix is
NOT "don't remove `dst`", it's "write the new content to a temp file first,
then rename it into place, so the window where `dst` doesn't exist is
eliminated by relying on `os.Rename`'s atomicity rather than
remove-then-create's non-atomicity."

`replaceBinary` is called from `Upgrade` at
`internal/updater/updater.go:106`:
```go
	if err := replaceBinary(binary, execPath); err != nil {
		return err
	}
```
where `binary` is a temp file already created by `extractBinary` (see
`internal/updater/extract.go:44-57` and `:79-91`, both of which write to
`os.CreateTemp("", "act-bin-*")` and `os.Chmod(..., 0755)`), and `execPath`
is `os.Executable()`'s result. No change to the caller is needed.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (this package) | `go test ./internal/updater/...` | exit 0, all pass, including new tests |
| Full test suite | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `internal/updater/updater.go` (the `replaceBinary` function only)
- `internal/updater/updater_test.go` (create, or extend if plan `003`'s
  executor already created this file — see note below)

**Out of scope** (do NOT touch, even though they look required):
- `internal/updater/extract.go` — the temp-file creation for the *extracted*
  binary is a separate, already-safe step; not touched by this plan.
- The `Upgrade` function's call site (`internal/updater/updater.go:106`) —
  the call signature (`replaceBinary(binary, execPath)`) does not change.
- Any change to file permissions philosophy — keep `0755` for the installed
  binary; only the *mechanism* of writing it changes, not the mode.

**Note on interaction with plan 003**: if plan `003` (fail-closed checksum
verification) has already run and created `internal/updater/updater_test.go`
with a `selectReleaseAssets` test, add this plan's new test(s) to that same
file rather than creating a duplicate. If plan `003` has not run yet, create
the file fresh as shown in Step 3 below.

## Git workflow

- Branch: `advisor/004-atomic-binary-replace`
- Single commit; message style example: `fix: make binary replacement atomic during self-upgrade`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Rewrite `replaceBinary` to use a temp-file-then-rename pattern

Replace the full body of `replaceBinary`
(`internal/updater/updater.go:196-219`) with:

```go
func replaceBinary(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer srcFile.Close()

	dstDir := filepath.Dir(dst)
	tmpDst, err := os.CreateTemp(dstDir, ".act-new-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for new binary: %w", err)
	}
	tmpDstPath := tmpDst.Name()
	defer os.Remove(tmpDstPath) // no-op if rename below succeeds

	if _, err := io.Copy(tmpDst, srcFile); err != nil {
		tmpDst.Close()
		return fmt.Errorf("failed to copy new binary: %w", err)
	}
	if err := tmpDst.Close(); err != nil {
		return fmt.Errorf("failed to finalize new binary: %w", err)
	}

	if err := os.Chmod(tmpDstPath, 0755); err != nil {
		return fmt.Errorf("failed to set permissions on new binary: %w", err)
	}

	// os.Rename is atomic on the same filesystem: dst is never left
	// missing or partially written, unlike remove-then-create.
	if err := os.Rename(tmpDstPath, dst); err != nil {
		return fmt.Errorf("failed to replace old binary: %w", err)
	}

	return nil
}
```

You must add `"path/filepath"` to the existing import block at the top of
`internal/updater/updater.go` (it currently imports `crypto/sha256`,
`encoding/hex`, `encoding/json`, `fmt`, `io`, `net/http`, `os`, `runtime`,
`strings` — add `"path/filepath"` alongside them, keeping the block
alphabetically grouped per Go convention, matching this repo's existing
import style).

Note: `os.CreateTemp(dstDir, ".act-new-*")` creates the temp file in the
*same directory* as the target binary (not the OS temp dir) — this is
required for `os.Rename` to be atomic; renaming across filesystems/volumes
is not atomic and can fail with `EXDEV`. Using `dstDir` (the directory
containing `execPath`, i.e. wherever `act` is installed — `/usr/local/bin`,
a Homebrew Cellar path, etc.) guarantees same-filesystem rename.

**Verify**: `go build ./internal/updater/...` → exit 0.

### Step 2: Confirm `os.IsNotExist`/"text file busy" concern is still handled

Unlike the old code, this version never calls `os.Remove(dst)` on the
*old* binary at all — `os.Rename` overwrites the destination path directly,
which works correctly even while the old binary is the currently-running
process's executable on both Unix (rename over an open-but-unlinked-safe
file is standard) and Windows (verify in Step 4 below, since Windows file
locking semantics differ from Unix). This eliminates the original code's
"text file busy" workaround because the rename-based approach doesn't hit
that failure mode in the first place — you are not truncating/reopening the
same inode a process holds open, you're swapping the directory entry.

No code change needed for this step — it's a confirmation, not an action.
If you're unsure whether Windows `os.Rename` can replace a running
executable's file, do not assume; proceed to Step 4's cross-compile check
and flag it in your final report if you find contrary evidence (e.g. from
Go's `os` package documentation for your Go version).

### Step 3: Add a test proving replacement is atomic and content-correct

Add to `internal/updater/updater_test.go` (create if it doesn't already
exist from plan `003`; if it exists, append this test function to it,
keeping the existing `package updater` declaration and imports merged):

```go
func TestReplaceBinary(t *testing.T) {
	dir := t.TempDir()
	dstPath := filepath.Join(dir, "act")
	srcPath := filepath.Join(dir, "act-new")

	if err := os.WriteFile(dstPath, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("setup: failed to write old binary: %v", err)
	}
	if err := os.WriteFile(srcPath, []byte("new binary content"), 0644); err != nil {
		t.Fatalf("setup: failed to write new binary: %v", err)
	}

	if err := replaceBinary(srcPath, dstPath); err != nil {
		t.Fatalf("replaceBinary failed: %v", err)
	}

	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read replaced binary: %v", err)
	}
	if string(got) != "new binary content" {
		t.Errorf("expected dst to contain new binary content, got %q", string(got))
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("failed to stat replaced binary: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected mode 0755, got %v", info.Mode().Perm())
	}

	// No leftover temp file in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "act" && e.Name() != "act-new" {
			t.Errorf("unexpected leftover file in dst dir: %s", e.Name())
		}
	}
}

func TestReplaceBinaryMissingSource(t *testing.T) {
	dir := t.TempDir()
	dstPath := filepath.Join(dir, "act")
	if err := os.WriteFile(dstPath, []byte("old binary content"), 0755); err != nil {
		t.Fatalf("setup: failed to write old binary: %v", err)
	}

	err := replaceBinary(filepath.Join(dir, "does-not-exist"), dstPath)
	if err == nil {
		t.Fatal("expected error when source binary is missing, got nil")
	}

	// dst must be untouched — this is the core regression check: the old
	// remove-then-create implementation would have already deleted dst by
	// this point if the failure happened after the remove; the new
	// implementation must never touch dst until the final rename succeeds.
	got, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("dst was removed even though replaceBinary failed: %v", err)
	}
	if string(got) != "old binary content" {
		t.Errorf("dst content changed despite replaceBinary failing: got %q", string(got))
	}
}
```

Add `"path/filepath"` to the test file's imports if not already present
(alongside `"os"` and `"testing"`).

`TestReplaceBinaryMissingSource` is the regression test that directly
exercises this plan's purpose: it proves that when the copy step can't even
start (source doesn't exist), the destination binary is left completely
intact — which was NOT true of the original remove-then-create code once
`os.Remove(dst)` had already run.

**Verify**: `go test ./internal/updater/... -run TestReplaceBinary -v` →
both tests pass.

### Step 4: Cross-compile check

```
GOOS=windows GOARCH=amd64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
```

**Verify**: all three exit 0. If any fails specifically due to
`path/filepath` or `os.Rename` usage in a platform-incompatible way, STOP
and report — that would indicate a real cross-platform gap in this fix that
needs investigation, not a workaround.

### Step 5: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass.

## Test plan

- Extend or create `internal/updater/updater_test.go` with
  `TestReplaceBinary` (happy path: content and permissions correct after
  replace, no leftover temp files) and `TestReplaceBinaryMissingSource` (the
  regression case: destination is untouched when the copy can't proceed).
- Both tests use `t.TempDir()` and real files on disk — no mocking needed,
  since `replaceBinary` only does filesystem I/O, no network or AWS calls.
- Verification: `go test ./internal/updater/... -v` → all pass, including
  these two new tests.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `replaceBinary` no longer calls `os.Remove(dst)` before the new content
      is fully written — `grep -n 'os.Remove(dst)' internal/updater/updater.go`
      returns no matches
- [ ] `replaceBinary` uses `os.CreateTemp(dstDir, ...)` + `os.Rename` to
      perform the swap
- [ ] `go build ./...` exits 0
- [ ] `GOOS=windows go build ./...`, `GOOS=linux go build ./...`,
      `GOOS=darwin go build ./...` all exit 0
- [ ] `go test ./...` exits 0, including `TestReplaceBinary` and
      `TestReplaceBinaryMissingSource`
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/updater/updater.go:196-219` doesn't match the excerpt
  above (drift since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- Cross-compiling for Windows reveals that `os.Rename` cannot replace a
  currently-open/running executable file on that platform (Windows file
  locking can behave differently from Unix in some configurations) — if you
  find authoritative evidence of this (not speculation), STOP and report it
  rather than shipping a fix that might not actually work on Windows; do not
  attempt a Windows-specific workaround without operator sign-off, since
  that would expand this plan's scope significantly.

## Maintenance notes

- The original code's comment about "text file busy" was Unix-specific
  folklore; the new implementation sidesteps that failure mode by design
  (see Step 2) — a future maintainer should not need to reintroduce
  remove-then-create.
- A reviewer should scrutinize: that the temp file is created in `dstDir`
  (same directory as the target), not the OS temp directory — this is the
  detail that makes `os.Rename` atomic; if a future edit changes the temp
  file's location, atomicity silently breaks again.
- If plan `003` executes before or after this plan, both modify
  `internal/updater/updater.go` in different functions (`Upgrade`'s checksum
  branch vs. `replaceBinary`) — they should not conflict, but whichever
  executes second should re-read the file fresh rather than assuming the
  other plan's diff didn't touch nearby lines.
