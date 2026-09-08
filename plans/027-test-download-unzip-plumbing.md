# Plan 027: Add test coverage for `downloadToTempFile` and `unzipTo`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/doctor/download.go internal/doctor/install_awscli.go`
> If either file changed since this plan was written, compare the "Current
> state" excerpts below against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW (test-only; zero production code changes)
- **Depends on**: none (soft note — see "Maintenance notes" about plan 024
  sharing this package)
- **Category**: tests
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`downloadToTempFile` (`internal/doctor/download.go`) and `unzipTo`
(`internal/doctor/install_awscli.go`) are the shared plumbing that every one
of the six OS-specific `act doctor --fix` installers
(`installAWSCLIDarwin`/`installAWSCLILinux`/`installAWSCLIWindows` in
`install_awscli.go`, plus the three `installSSMPlugin*` functions in
`install_ssmplugin.go`) delegates to for the actual network fetch and
archive extraction. Neither function has a direct test today —
`internal/doctor/fix_test.go` and `internal/doctor/doctor_test.go` only cover
`runFixes`/`fixLogPath`/`checkRegion`/`checkProfile`/`extractJSON`. This is
the exact code path `act doctor --fix` runs immediately before invoking
`sudo` (e.g. `installAWSCLILinux` unzips the downloaded bundle, then execs
`sudo ./aws/install`), so a subtle bug here — a non-200 response not
actually being rejected, a truncated/partial download not being surfaced as
an error, or a zip-slip entry not being caught by `unzipTo`'s call into
`isWithinDir` — would only surface when a real user runs the fix in the
field, potentially under `sudo`. Locking in this behavior with tests now
means a future refactor of either function fails CI instead of shipping a
regression silently.

## Current state

- `internal/doctor/download.go` — the entire file (36 lines), quoted
  exactly:

  ```go
  package doctor

  import (
      "fmt"
      "io"
      "net/http"
      "os"
  )

  // downloadToTempFile downloads url to a new temp file created with the
  // given pattern (see os.CreateTemp) and returns its path. The caller is
  // responsible for removing the file (defer os.Remove(path)).
  func downloadToTempFile(url, pattern string) (string, error) {
      resp, err := http.Get(url)
      if err != nil {
          return "", fmt.Errorf("failed to download: %w", err)
      }
      defer resp.Body.Close()

      if resp.StatusCode != http.StatusOK {
          return "", fmt.Errorf("download returned status %d", resp.StatusCode)
      }

      tmp, err := os.CreateTemp("", pattern)
      if err != nil {
          return "", fmt.Errorf("failed to create temp file: %w", err)
      }
      defer tmp.Close()

      if _, err := io.Copy(tmp, resp.Body); err != nil {
          os.Remove(tmp.Name())
          return "", fmt.Errorf("failed to write download: %w", err)
      }

      return tmp.Name(), nil
  }
  ```

  Signature: `func downloadToTempFile(url, pattern string) (string, error)`
  — `url` is a plain string, not a configurable client/base-URL. This means
  an `httptest.NewServer(...)`'s real `.URL` field (e.g.
  `"http://127.0.0.1:54321"`) can be passed directly as the `url` argument
  with zero production code changes — `http.Get(url)` will hit the local
  test server exactly like it would hit a real HTTPS endpoint. No dependency
  injection, interface, or refactor of `download.go` is needed or in scope.

- `internal/doctor/install_awscli.go:123-166` — `unzipTo`, quoted exactly:

  ```go
  // unzipTo extracts every regular file in the zip archive at zipPath into
  // destDir, preserving the archive's relative directory structure.
  func unzipTo(zipPath, destDir string) error {
      r, err := zip.OpenReader(zipPath)
      if err != nil {
          return fmt.Errorf("failed to open zip: %w", err)
      }
      defer r.Close()

      for _, f := range r.File {
          targetPath := filepath.Join(destDir, f.Name)
          if !isWithinDir(destDir, targetPath) {
              return fmt.Errorf("illegal file path in zip: %s", f.Name)
          }

          if f.FileInfo().IsDir() {
              if err := os.MkdirAll(targetPath, 0755); err != nil {
                  return err
              }
              continue
          }

          if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
              return err
          }

          rc, err := f.Open()
          if err != nil {
              return err
          }

          out, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
          if err != nil {
              rc.Close()
              return err
          }

          _, copyErr := io.Copy(out, rc)
          rc.Close()
          out.Close()
          if copyErr != nil {
              return copyErr
          }
      }
      return nil
  }
  ```

  `unzipTo` calls `isWithinDir` (defined immediately below it, lines
  168–179) on every entry to reject zip-slip traversal attempts.

- **Relationship to plan 024** (`plans/024-test-iswithindir-zipslip-guard.md`,
  already written, category `tests`, no ordering dependency on this plan):
  plan 024 adds `internal/doctor/install_awscli_test.go` with
  `TestIsWithinDir`, a pure table-driven unit test of `isWithinDir` in
  isolation (no zip files, no I/O). **This plan (027) does not duplicate
  that scope.** This plan tests `unzipTo` end-to-end against a real,
  in-memory-built zip fixture, including one entry that is a traversal
  attempt — the point is to prove the *wiring* between `unzipTo` and
  `isWithinDir` (that `unzipTo` actually calls the guard, checks its result,
  and returns the right error, and that no file is written outside
  `destDir`), not to re-test `isWithinDir`'s internal branch logic, which
  024 already covers directly. If both plans land, see "Maintenance notes"
  below on avoiding a naming collision.

- `internal/doctor/fix_test.go` (full file, 89 lines) — existing test
  conventions for this package: plain `package doctor`, `testing.T`,
  `t.TempDir()` for scratch directories, and an `overrideHomeForDoctorTest`
  helper (defined in `internal/doctor/doctor_test.go`) used only for
  HOME-dependent config checks. Neither `downloadToTempFile` nor `unzipTo`
  reads `HOME` or any config file, so `overrideHomeForDoctorTest` is **not
  needed** for this plan's tests — do not call it.

- Confirmed via `grep -rln "downloadToTempFile\|unzipTo" internal/doctor/*_test.go`
  → no output, no matches. Neither function has any existing test today.

- **Platform portability constraint** (same as plan 024): `.github/workflows/ci.yml`
  runs `go test -v ./...` on a matrix of `[ubuntu-latest, macos-latest,
  windows-latest]` (`go-version: "1.26.5"`). Consequences for this plan's
  tests:
  - Build all filesystem paths with `filepath.Join`, never hardcoded
    `/`-delimited strings.
  - Do not assert on exact Unix file permission bits (e.g. `0755`,
    `f.Mode()` exact octal value) on the extracted files — Windows does not
    have the same permission model and such an assertion would be flaky or
    wrong there. It is fine to assert the file *exists* and its *content* is
    correct; do not assert its mode bits.
  - `httptest.NewServer` binds to `127.0.0.1` on an OS-assigned port on all
    three platforms — no special-casing needed for the success/non-200
    cases.
  - For the "connection refused" case, use `httptest.NewServer` and then
    **close** it (`srv.Close()`) before calling `downloadToTempFile` with
    its `.URL` — this guarantees a connection-refused error portably on all
    three CI platforms, without hardcoding a specific port number that might
    theoretically be in use (safer and more portable than guessing a fixed
    port like `127.0.0.1:1`).

## Commands you will need

| Purpose      | Command                                                                  | Expected on success                        |
|--------------|---------------------------------------------------------------------------|---------------------------------------------|
| Build        | `go build -v ./...`                                                       | exit 0                                      |
| Vet          | `go vet ./...`                                                            | exit 0, no output                           |
| Format check | `gofmt -l .`                                                              | no output                                   |
| Run new tests| `go test -v ./... -run 'TestDownloadToTempFile|TestUnzipTo'`               | all listed subtests PASS                    |
| Full test    | `go test -v ./...`                                                        | all tests pass, exit 0                      |

## Scope

**In scope** (the only file(s) you should create):
- `internal/doctor/download_test.go` (new file — tests for
  `downloadToTempFile`)
- `internal/doctor/unzip_test.go` (new file — tests for `unzipTo`)

Two separate files are specified (not one combined file) because they map
1:1 to the two source files under test (`download.go` and the `unzipTo`
portion of `install_awscli.go`), matching this repo's existing convention of
one test file per subject area (`fix_test.go` for `fix.go`, `doctor_test.go`
for `doctor.go`). Do not merge them into a single file.

**Out of scope** (do NOT touch, even though related):
- `internal/doctor/download.go`, `internal/doctor/install_awscli.go` — this
  plan proves the *existing* behavior of `downloadToTempFile` and `unzipTo`
  is correct; it does not change either function. If a test you write
  reveals a real bug (see STOP conditions), stop and report it — do not fix
  the production code yourself as part of this plan.
- `internal/doctor/install_awscli_test.go` — this is plan 024's file (may or
  may not exist yet depending on execution order); do not create, edit, or
  duplicate `TestIsWithinDir` from it.
- `internal/doctor/fix_test.go`, `internal/doctor/doctor_test.go` — read-only
  references for conventions; do not edit.
- `internal/doctor/install_ssmplugin.go` and its (non-existent) test file —
  not in scope for this plan, even though `installSSMPlugin*` also calls
  `downloadToTempFile`. Out of scope because the plumbing under test
  (`downloadToTempFile`/`unzipTo`) is identical regardless of which caller
  invokes it; testing the shared functions directly (as this plan does) is
  sufficient and avoids duplicating near-identical test setup per caller.
- `README.md` — this is a test-only addition with no user-facing behavior,
  flag, or command change, so the "update README after adding a
  feature/flag" rule in `CLAUDE.md` does not apply here. Do not touch
  `README.md`.

## Git workflow

- Branch: `advisor/027-test-download-unzip-plumbing`
- Commit message style: Conventional Commits, matching this repo's
  test-only commit precedent from `git log --oneline`, e.g.
  `ef570e7 test: add unit tests for updater.buildAssetName and doctor.extractJSON`.
  Use: `test: add coverage for downloadToTempFile and unzipTo`
- Commit once, after Step 3 passes all verification.
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Create `internal/doctor/download_test.go` with `TestDownloadToTempFile`

Create the file with `package doctor` and a `TestDownloadToTempFile` parent
test containing subtests via `t.Run`. Required imports: `"io"`, `"net/http"`,
`"net/http/httptest"`, `"os"`, `"testing"`.

Write these three subtests:

1. **`t.Run("success", ...)`**: start `srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK); w.Write([]byte("hello from test server")) }))`, `defer srv.Close()`. Call `path, err := downloadToTempFile(srv.URL, "download-test-*.bin")`. Assert `err == nil`. Assert `os.Stat(path)` succeeds (file exists). Read the file with `os.ReadFile(path)` and assert its content equals exactly `"hello from test server"`. Then call `os.Remove(path)` and assert that returns `nil` (proves the caller can clean up as the doc comment promises). Use `defer os.Remove(path)` is not appropriate here since you are asserting the removal itself succeeds — call it directly and check the error, not via defer.

2. **`t.Run("non-200 status", ...)`**: start a test server whose handler calls `w.WriteHeader(http.StatusNotFound)` (404) and writes no body. Call `downloadToTempFile(srv.URL, "download-test-*.bin")`. Assert `err != nil`. Assert the error message contains the substring `"download returned status"` (use `strings.Contains(err.Error(), "download returned status")` — add `"strings"` to the imports). Assert the returned path is the empty string.

3. **`t.Run("connection refused", ...)`**: start an `httptest.NewServer` with a trivial handler, capture `url := srv.URL`, then immediately call `srv.Close()`. Call `downloadToTempFile(url, "download-test-*.bin")` against the now-closed server's URL. Assert `err != nil` (the underlying `http.Get` will fail with a connection-refused error, which `downloadToTempFile` wraps as `"failed to download: %w"`). Assert the returned path is the empty string. Do not assert on the exact wrapped OS-level error text (it varies by platform) — only assert `err != nil` and the path is empty.

**Verify**: `go test -v ./... -run TestDownloadToTempFile` → `TestDownloadToTempFile` and its 3 subtests (`success`, `non-200_status` or similar sanitized name, `connection_refused`) all print `PASS`, overall exit 0.

### Step 2: Create `internal/doctor/unzip_test.go` with `TestUnzipTo`

Create the file with `package doctor` and a `TestUnzipTo` parent test
containing subtests via `t.Run`. Required imports: `"archive/zip"`,
`"bytes"`, `"os"`, `"path/filepath"`, `"strings"`, `"testing"`.

Write a helper function `buildTestZip(t *testing.T, entries map[string]string) string` that:
- Creates a `var buf bytes.Buffer` and `zw := zip.NewWriter(&buf)`.
- For each `name, content := range entries` (iterate in a deterministic
  order — since Go map iteration order is random, either sort the keys
  first with `sort.Strings` (add `"sort"` import) or, simpler, accept
  `entries []struct{ name, content string }` instead of a map to avoid the
  ordering concern entirely — **use the slice-of-struct form**, not a map,
  to keep iteration order deterministic and avoid needing to import `sort`),
  call `w, err := zw.Create(name)`, check `err`, then `w.Write([]byte(content))`.
- Call `zw.Close()`.
- Create a temp file via `f, err := os.CreateTemp(t.TempDir(), "test-*.zip")`,
  write `buf.Bytes()` to it, close it, and return its path.
- Use `t.Fatalf` on any error inside the helper (call `t.Helper()` at the
  top).

So the corrected signature is: `func buildTestZip(t *testing.T, entries []struct{ name, content string }) string`.

Write these subtests, each calling `destDir := t.TempDir()` for the
extraction target:

1. **`t.Run("nested file", ...)`**: build a zip with one entry
   `{name: filepath.Join("subdir", "file.txt"), content: "nested content"}`
   (note: zip entry names use forward slashes per the zip spec — use
   `"subdir/file.txt"` as a literal string for the zip entry name itself,
   since zip format always uses `/` regardless of OS; do NOT use
   `filepath.Join` for the zip entry name, only for the expected filesystem
   path you check afterward). Call `err := unzipTo(zipPath, destDir)`.
   Assert `err == nil`. Assert `os.ReadFile(filepath.Join(destDir, "subdir", "file.txt"))`
   succeeds and equals `"nested content"`.

2. **`t.Run("top-level file", ...)`**: build a zip with one entry
   `{name: "top.txt", content: "top level content"}`. Call `unzipTo`.
   Assert `err == nil`. Assert `os.ReadFile(filepath.Join(destDir, "top.txt"))`
   equals `"top level content"`.

3. **`t.Run("traversal is rejected", ...)`**: build a zip with one entry
   named literally `"../../escaped.txt"` (this exact string, as the zip
   entry name — a clearly-fake filename, not a real path on the test
   machine) with arbitrary content e.g. `"should never be written"`. Call
   `err := unzipTo(zipPath, destDir)`. Assert `err != nil`. Assert
   `strings.Contains(err.Error(), "illegal file path in zip")`. Then compute
   the would-be escaped path as `escaped := filepath.Join(filepath.Dir(destDir), "..", "escaped.txt")`
   — actually, simplify: since `destDir` is `t.TempDir()` (something like
   `/tmp/TestUnzipTo_traversal_is_rejected/001`), a `"../../escaped.txt"`
   entry joined with `destDir` via `filepath.Join(destDir, "../../escaped.txt")`
   resolves two directories up. Compute `escapedPath := filepath.Join(destDir, "..", "..", "escaped.txt")`
   and assert `_, statErr := os.Stat(escapedPath); os.IsNotExist(statErr)`
   is `true` — i.e. confirm the file was NOT created outside `destDir`. Also
   assert, for defense in depth, that `destDir` itself is empty after the
   call: `entries, _ := os.ReadDir(destDir); len(entries) == 0`.

**Verify**: `go test -v ./... -run TestUnzipTo` → `TestUnzipTo` and its 3
subtests all print `PASS`, overall exit 0.

### Step 3: Full verification pass

Run every command from the "Commands you will need" table in order:

1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output (if either new file is unformatted, run
   `gofmt -w internal/doctor/download_test.go internal/doctor/unzip_test.go`
   and re-check).
4. `go test -v ./... -run 'TestDownloadToTempFile|TestUnzipTo'` → all 6
   subtests (3 + 3) PASS.
5. `go test -v ./...` → full suite passes, exit 0, with zero change to any
   pre-existing test's pass/fail status.

**Verify**: all five commands above complete with the stated expected
result.

## Test plan

- New test `TestDownloadToTempFile` in `internal/doctor/download_test.go`:
  subtests `success` (200 + body → correct temp file, removable), `non-200
  status` (404 → error containing "download returned status"), `connection
  refused` (closed server → error, empty path).
- New test `TestUnzipTo` in `internal/doctor/unzip_test.go`: subtests
  `nested file` (subdir/file.txt extracted correctly), `top-level file`
  (plain file extracted correctly), `traversal is rejected` (zip-slip entry
  → error containing "illegal file path in zip", nothing written outside
  `destDir`).
- Structural pattern to follow for both: the `t.Run(tt.name, func(t
  *testing.T) {...})` subtest style already used in
  `internal/doctor/doctor_test.go`'s `TestExtractJSON`, adapted here to
  individually-named `t.Run` blocks (rather than a single table slice)
  since each subtest in this plan needs a distinct server/zip fixture
  rather than a shared input/want pair.
- Verification: `go test -v ./... -run 'TestDownloadToTempFile|TestUnzipTo'`
  → all 6 subtests pass; then `go test -v ./...` → full suite passes with
  no regressions.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/doctor/download_test.go` exists and contains
      `func TestDownloadToTempFile(t *testing.T)` with 3 subtests
- [ ] `internal/doctor/unzip_test.go` exists and contains
      `func TestUnzipTo(t *testing.T)` with 3 subtests
- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./... -run 'TestDownloadToTempFile|TestUnzipTo'` passes,
      6 subtests total
- [ ] `go test -v ./...` exits 0, all tests pass (existing + new)
- [ ] No files outside `internal/doctor/download_test.go` and
      `internal/doctor/unzip_test.go` are modified or created (`git status`
      shows exactly two new files)
- [ ] `plans/README.md` status row for plan 027 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/doctor/download.go` or
  `internal/doctor/install_awscli.go:123-166` doesn't match the excerpts
  quoted in "Current state" (the code has drifted since this plan was
  written — re-derive the expected behavior before writing tests).
- Any subtest in Step 1 or Step 2 produces an actual result that
  contradicts its documented expectation — e.g. `unzipTo` does NOT reject
  the `"../../escaped.txt"` entry, or a file IS created outside `destDir`,
  or `downloadToTempFile` returns a non-empty path on error. This would
  mean a real bug (potentially security-relevant, in the zip-slip case) in
  production code. Do not "fix" the test to match the buggy behavior, and
  do not edit `download.go` or `install_awscli.go` yourself. Stop and
  report the exact input/output that contradicts the documented behavior.
- `go test -v ./...` fails on a pre-existing test (one not added by this
  plan) — that is a pre-existing or environmental issue; report it rather
  than modifying unrelated test files.
- A step's verification fails twice after a reasonable fix attempt.
- You discover that `internal/doctor/install_awscli_test.go` (plan 024's
  file) already exists and defines a helper or test name that collides with
  one you are about to add (see "Maintenance notes" below) — rename your
  own helper to something more specific (e.g. prefix with `unzipTest` /
  `downloadTest`) rather than editing plan 024's file.

## Maintenance notes

- Plan 024 (`plans/024-test-iswithindir-zipslip-guard.md`) also adds a test
  file to `internal/doctor/` (`install_awscli_test.go`) as a parallel,
  independent effort. There is no hard ordering dependency between the two
  plans, but if both land in the same package, a reviewer should check for
  compile-time collisions: duplicate helper function names or duplicate
  top-level identifiers across `internal/doctor/*_test.go` files in the same
  package will fail to compile. This plan's helper is named
  `buildTestZip` and its test names are `TestDownloadToTempFile` /
  `TestUnzipTo`, which do not overlap with plan 024's `TestIsWithinDir`; no
  action needed unless names are changed during execution.
- If `unzipTo` is ever extended to support other archive formats (tar,
  tar.gz) or `downloadToTempFile` gains retry/backoff logic, these tests are
  the first thing to re-run and extend — they pin the current
  network-failure and archive-extraction contracts that all six OS-specific
  installers rely on.
- This plan intentionally does not test the six `installAWSCLI*` /
  `installSSMPlugin*` caller functions themselves (they also shell out to
  `sudo`/`msiexec`, which is not testable in CI); it only closes the gap on
  the two pure(r) plumbing functions they all share. A future plan could
  look at making the `sudo`/install-command invocation itself injectable
  for testing, but that is a larger refactor and out of scope here.
