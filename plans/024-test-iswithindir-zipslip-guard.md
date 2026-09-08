# Plan 024: Add test coverage for the `isWithinDir` zip-slip guard

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/doctor/install_awscli.go`
> If that file changed since this plan was written, compare the "Current
> state" excerpt below against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW (test-only; zero production code changes)
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`isWithinDir` is the zip-slip path-traversal guard that `unzipTo` calls on
every entry extracted from a downloaded AWS-CLI / Session-Manager-plugin
zip archive (`act doctor --fix`'s auto-installer path on Linux). It is
small, pure, and security-critical — exactly the kind of boundary logic
(`..`, absolute paths, trailing separators) that's easy to get subtly wrong
in a future refactor and hard to catch by code review alone, because a bug
here only manifests when someone crafts a malicious zip. It currently has
zero test coverage: `grep -rn "isWithinDir" internal/doctor/*_test.go`
returns nothing. Adding a table-driven test locks in the four branches of
this function's current (correct) behavior so a future edit that breaks
path-traversal protection fails CI instead of shipping silently.

## Current state

- `internal/doctor/install_awscli.go` — the only file that defines or calls
  `isWithinDir`. Confirmed via `grep -rln "isWithinDir" internal/doctor/`
  → only this file. Package is `package doctor` (line 1). Full import block
  (lines 1–12):

  ```go
  package doctor

  import (
      "archive/zip"
      "fmt"
      "io"
      "os"
      "os/exec"
      "path/filepath"
      "runtime"
      "strings"
  )
  ```

- `internal/doctor/install_awscli.go:168-179` — the function under test,
  quoted exactly as it exists today:

  ```go
  // isWithinDir reports whether target is contained within dir, guarding
  // against zip-slip path traversal from malicious archive entries.
  func isWithinDir(dir, target string) bool {
      rel, err := filepath.Rel(dir, target)
      if err != nil {
          return false
      }
      if filepath.IsAbs(rel) || rel == ".." {
          return false
      }
      return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
  }
  ```

  Signature: `func isWithinDir(dir, target string) bool` — takes only two
  strings, does no I/O, touches no globals. This means a plain table-driven
  test with pure (possibly non-existent) string paths is sufficient; no
  `t.TempDir()`, no filesystem writes, and no HOME override is needed (this
  package's `overrideHomeForDoctorTest` helper in `internal/doctor/fix_test.go`
  and `internal/doctor/doctor_test.go` exists for HOME-dependent config
  checks — it does not apply here and must not be used in this test).

- `internal/doctor/install_awscli.go:121-135` — the only caller, for
  context on how the guard is used (do not modify this):

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
          ...
  ```

- No test file exists for `install_awscli.go` today. Confirmed via
  `ls internal/doctor/*_test.go` → only `internal/doctor/doctor_test.go` and
  `internal/doctor/fix_test.go` exist; `internal/doctor/install_awscli_test.go`
  does not exist and must be created by this plan.

- Repo convention for table-driven tests in this package: model this test
  on `TestExtractJSON` in `internal/doctor/doctor_test.go` (a `[]struct{name
  string; ...; want ...}` slice iterated with `t.Run(tt.name, func(t
  *testing.T) {...})`), NOT on the HOME-override tests in that same file —
  `extractJSON` (like `isWithinDir`) is a pure function with no external
  state, which is exactly why `TestExtractJSON` needs no setup/teardown.

- **Platform portability constraint**: `.github/workflows/ci.yml` runs the
  `build` job (which includes `go test -v ./...`) on a matrix of
  `[ubuntu-latest, macos-latest, windows-latest]`. `filepath.Rel`,
  `filepath.IsAbs`, and `filepath.Separator` are platform-sensitive
  (`/` on Unix, `\` on Windows; Windows also has drive-letter absolute
  paths). Test cases MUST build paths with `filepath.Join` (and, where a
  literal parent-relative path like `dir/..` is needed, `filepath.Join(dir,
  "..")`) rather than hardcoded string literals containing `/` or `\`, so
  the exact same test table passes unmodified on all three CI platforms.
  Example of the correct pattern for a two-level escape:
  `filepath.Join(dir, "..", "..", "etc", "passwd")` — never write this as a
  literal string like `"/tmp/extract/../../etc/passwd"`.

## Commands you will need

| Purpose      | Command                                              | Expected on success                   |
|--------------|-------------------------------------------------------|----------------------------------------|
| Build        | `go build -v ./...`                                   | exit 0                                 |
| Vet          | `go vet ./...`                                         | exit 0, no output                      |
| Format check | `gofmt -l .`                                           | no output                              |
| Run new test | `go test -v ./... -run TestIsWithinDir`               | `TestIsWithinDir` and all subtests PASS |
| Full test    | `go test -v ./...`                                     | all tests pass, exit 0                 |

## Scope

**In scope** (the only file you should create or modify):
- `internal/doctor/install_awscli_test.go` (new file — does not exist yet)

**Out of scope** (do NOT touch, even though related):
- `internal/doctor/install_awscli.go` — this plan proves the existing guard
  is correct; it does not change it. If your test reveals `isWithinDir`
  actually has a bug (a case where it returns the wrong answer), STOP per
  the "STOP conditions" section below and report it — do not fix
  `install_awscli.go` yourself as part of this plan.
- `internal/doctor/doctor_test.go`, `internal/doctor/fix_test.go` — do not
  edit these; they are read-only references for this plan's conventions.
- `README.md` — this is a test-only addition with no user-facing behavior,
  flag, or command change, so the "update README after adding a
  feature/flag" rule in `CLAUDE.md` does not apply here. Do not touch
  `README.md`.
- `unzipTo` or any other function in `install_awscli.go` — not in scope.

## Git workflow

- Branch: `advisor/024-test-iswithindir-zipslip-guard`
- Commit message style: Conventional Commits, matching this repo's
  test-only commit precedent from `git log --oneline`, e.g.
  `ef570e7 test: add unit tests for updater.buildAssetName and doctor.extractJSON`.
  Use: `test: add coverage for isWithinDir zip-slip guard`
- Commit once, after Step 1 passes all verification.
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Create `internal/doctor/install_awscli_test.go` with `TestIsWithinDir`

Create the new file with a table-driven test. Use `package doctor` (same
package as `install_awscli.go`, so `isWithinDir` is directly callable
without an import). Required imports: `"path/filepath"` and `"testing"`
only — no `"os"` needed, since no real filesystem paths or temp dirs are
required (the function does no I/O).

Build the table with at least these cases (you may add more, but do not
remove or weaken any of these):

1. **Normal nested file** (happy path): `dir = filepath.Join("tmp",
   "extract")`, `target = filepath.Join(dir, "subdir", "file.txt")` →
   `want = true`.
2. **Sibling escape**: `dir = filepath.Join("tmp", "extract")`, `target =
   filepath.Join("tmp", "sibling", "file.txt")` → `want = false`. (This
   exercises the `strings.HasPrefix(rel, ".."+separator)` branch, since
   `rel` here is `"..", "sibling", "file.txt"` joined.)
3. **Exact traversal — target is the parent dir itself**: `dir =
   filepath.Join("tmp", "extract")`, `target = filepath.Join(dir, "..")` →
   `want = false`. This is the literal `rel == ".."` branch — `target`
   resolves to exactly `dir`'s parent, so `filepath.Rel` returns `".."`
   with no trailing separator, which is a distinct code path from case 2
   and must be tested separately; do not conflate them.
4. **Deep traversal**: `dir = filepath.Join("tmp", "extract")`, `target =
   filepath.Join(dir, "..", "..", "etc", "passwd")` → `want = false`. Build
   this with `filepath.Join` exactly as shown (do not hardcode a
   slash-delimited string) so it resolves correctly and portably on
   Windows too.
5. **Target equals dir**: `dir = filepath.Join("tmp", "extract")`, `target
   = dir` (i.e. the exact same string/value) → `want = true`. Reasoning to
   verify in the test's doc comment or a code comment: `filepath.Rel(dir,
   dir)` returns `"."`, which is neither `".."` nor does it have the
   `".."+separator` prefix, so `isWithinDir` correctly treats "the root
   extraction dir itself" as within itself.
6. Document, in a comment near the test (not as a runnable case), why the
   `filepath.Rel` error branch (`if err != nil { return false }`) is not
   covered by a table case: `filepath.Rel` only errors when the two paths
   can't be made relative to each other (e.g., different volume/drive
   letters on Windows, such as `` `C:\foo` `` vs `` `D:\bar` ``) — not
   reliably or portably triggerable from a single table-driven test that
   must also pass on Linux and macOS. Note in the comment that this branch
   is intentionally left to manual code inspection rather than a forced,
   platform-specific test case.

Model the table/subtest structure directly on `TestExtractJSON` in
`internal/doctor/doctor_test.go` (`[]struct{name string; dir, target
string; want bool}`, iterated with `t.Run(tt.name, func(t *testing.T)
{...})`, comparing `got := isWithinDir(tt.dir, tt.target)` against
`tt.want` and calling `t.Errorf` with both actual and expected values on
mismatch).

**Verify**: `go test -v ./... -run TestIsWithinDir` → `TestIsWithinDir` and
all of its 5 subtests (from cases 1–5 above) print `PASS`, overall exit 0.

### Step 2: Full verification pass

Run every command from the "Commands you will need" table in order:

1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output (if your new file is unformatted, run `gofmt -w
   internal/doctor/install_awscli_test.go` and re-check).
4. `go test -v ./... -run TestIsWithinDir` → all subtests PASS.
5. `go test -v ./...` → full suite passes, exit 0, with zero change to any
   pre-existing test's pass/fail status.

**Verify**: all five commands above complete with the stated expected
result.

## Test plan

- New test: `TestIsWithinDir` in `internal/doctor/install_awscli_test.go`
  (new file), covering: normal nested file (true), sibling escape (false),
  exact-parent traversal via literal `".."` (false), deep multi-level
  traversal via `filepath.Join` (false), and target-equals-dir (true) — 5
  subtests total, per Step 1.
- Structural pattern to follow: `TestExtractJSON` in
  `internal/doctor/doctor_test.go`.
- Verification: `go test -v ./... -run TestIsWithinDir` → all 5 subtests
  pass; then `go test -v ./...` → full suite passes with no regressions.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/doctor/install_awscli_test.go` exists and contains
      `func TestIsWithinDir(t *testing.T)`
- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./... -run TestIsWithinDir` passes with 5 subtests
- [ ] `go test -v ./...` exits 0, all tests pass (existing + new)
- [ ] `grep -c "want" internal/doctor/install_awscli_test.go` shows at
      least 5 table entries with expected values
- [ ] No files outside `internal/doctor/install_awscli_test.go` are
      modified or created (`git status` shows exactly one new file)
- [ ] `plans/README.md` status row for plan 024 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/doctor/install_awscli.go:168-179` doesn't match the
  `isWithinDir` excerpt quoted in "Current state" (the guard has drifted
  since this plan was written — re-derive the branches before writing
  tests).
- Any test case in Step 1 produces an actual result that contradicts its
  documented `want` value — this would mean `isWithinDir` has a real bug.
  Do not "fix" the test to match the buggy behavior, and do not edit
  `install_awscli.go` to fix the function. Stop and report the exact
  input/output that contradicts the documented behavior; this is a
  security-relevant finding for a human to triage.
- `go test -v ./...` fails on a pre-existing test (one not added by this
  plan) — that is a pre-existing or environmental issue, not something this
  plan should fix; report it rather than modifying unrelated test files.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- If `isWithinDir` or `unzipTo` is ever refactored (e.g. to support
  extracting other archive formats, or to change its traversal-detection
  algorithm), `TestIsWithinDir` should be the first thing re-run — a
  regression here means a zip-slip vulnerability, not just a test failure.
- This plan intentionally does not attempt to test `unzipTo` itself (which
  would require constructing a real malicious zip archive and asserting the
  extraction is aborted) — that is a larger, separate testing effort
  (building an in-memory `archive/zip` writer with a crafted `../` entry
  name) that a future plan could take on if end-to-end confidence in the
  full extraction path is desired. This plan only closes the gap on the
  pure guard function itself.
- A reviewer should scrutinize: that all 5 required cases from Step 1 are
  present and use `filepath.Join` (not hardcoded slash strings) for
  cross-platform correctness on the Windows CI runner.
