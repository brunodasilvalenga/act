# Plan 015: Add test coverage for internal/updater and internal/doctor

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/updater/ internal/doctor/`
> If any file under either directory changed since this plan was written,
> compare the "Current state" excerpts against the live code before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: M
- **Risk**: LOW
- **Depends on**: plans 003 and 004 (both modify
  `internal/updater/updater.go` — this plan's tests should be written
  against whichever version of that file exists after those two plans land,
  to avoid writing tests against soon-to-be-replaced code; if plans 003/004
  have not run, this plan's Step 2/3 tests still apply to the current code
  as excerpted below, but note some described behavior — e.g. the fail-open
  checksum skip — will change if plan 003 later runs, so re-read the
  function before writing tests either way)
- **Category**: tests
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`go test ./... -cover` reports `internal/updater` and `internal/doctor` at
0.0% coverage, with no test file in either package
(confirmed via `find . -name '*_test.go'` returning no matches under either
directory). `internal/updater` is the self-upgrade code path — it downloads
a release binary, verifies a checksum, extracts an archive, and overwrites
the running executable; a bug here could brick an installation (see plans
003 and 004, which fix two specific bugs in this package but don't add
general test coverage). `internal/doctor` contains `extractJSON`
(`internal/doctor/doctor.go:243-256`), a hand-rolled substring-search JSON
value extractor (not using `encoding/json`) that is fragile against
whitespace/formatting variance in `aws sts get-caller-identity` output, and
is entirely untested. This plan adds targeted unit tests for the pure,
already-testable logic in both packages: `buildAssetName` and
`extractJSON`, plus (if plans 003/004 have landed) their new pure helpers
`selectReleaseAssets` and the atomic-replace logic. It deliberately does NOT
attempt to test the network-calling or process-launching parts of either
package, since that would require mocking `http.Get`/`exec.Command`, which
this codebase has no existing pattern for and is a larger effort than this
plan's scope.

## Current state

`internal/updater/updater.go:114-124` (`buildAssetName`, full function —
unaffected by plans 003/004, which touch different functions in this same
file):

```go
func buildAssetName(version string) string {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("act_%s_%s_%s.%s", version, goos, goarch, ext)
}
```

`internal/doctor/doctor.go:243-256` (`extractJSON`, full function):

```go
func extractJSON(jsonStr, key string) string {
	// Simple extraction without importing encoding/json for a struct
	search := fmt.Sprintf(`"%s": "`, key)
	idx := strings.Index(jsonStr, search)
	if idx == -1 {
		return ""
	}
	start := idx + len(search)
	end := strings.Index(jsonStr[start:], `"`)
	if end == -1 {
		return ""
	}
	return jsonStr[start : start+end]
}
```

Called from `checkCredentials` (`internal/doctor/doctor.go:112-150`) against
the real JSON output of `aws sts get-caller-identity --output json`, to pull
out `"Arn"` and `"Account"` values for display. Note the search pattern
`"%s": "` (with a space after the colon) is specific to the JSON
formatting AWS CLI happens to produce — a key insight to preserve when
writing tests: this function is brittle by design (it's explicitly a
"simple extraction" avoiding a proper struct+`encoding/json` unmarshal), so
tests should characterize its actual (possibly surprising) behavior rather
than an idealized one.

`internal/doctor/doctor.go:152-182` (`checkRegion`, `checkProfile` — pure
functions delegating to `config.ResolveRegion`/`ResolveProfile`):

```go
func checkRegion(flagRegion string) result {
	resolved := config.ResolveRegion(flagRegion, "")
	if resolved == "" {
		return result{
			Name:   "Region",
			Status: statusWarn,
			Detail: "not configured (use --region, AWS_REGION, or ~/.act.json)",
		}
	}
	return result{
		Name:   "Region",
		Status: statusPass,
		Detail: resolved,
	}
}

func checkProfile(flagProfile string) result {
	resolved := config.ResolveProfile(flagProfile, "")
	if resolved == "" {
		return result{
			Name:   "Profile",
			Status: statusWarn,
			Detail: "using default (no explicit profile set)",
		}
	}
	return result{
		Name:   "Profile",
		Status: statusPass,
		Detail: resolved,
	}
}
```

These call `config.ResolveRegion`/`config.ResolveProfile`
(`internal/config/config.go:103-138`), which read environment variables and
`~/.act.json` — testing them requires the same `overrideHome`-style
isolation already used in `internal/config/config_test.go`. `result`,
`status`, `statusPass`, `statusWarn`, `statusFail` are defined at
`internal/doctor/doctor.go:14-26`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (updater) | `go test ./internal/updater/... -v` | exit 0, all pass |
| Test (doctor) | `go test ./internal/doctor/... -v` | exit 0, all pass |
| Full test suite | `go test ./...` | exit 0, all pass |
| Coverage check | `go test ./... -cover` | `internal/updater` and `internal/doctor` show coverage > 0.0% |

## Scope

**In scope** (the only files you should modify):
- `internal/updater/updater_test.go` (create, or extend if plans 003/004
  already created it — merge into the existing file rather than duplicating)
- `internal/doctor/doctor_test.go` (create — new file)

**Out of scope** (do NOT touch, even though they look required):
- Any production code in `internal/updater/updater.go`,
  `internal/updater/extract.go`, or `internal/doctor/doctor.go` — this plan
  is test-only (unless plans 003/004 already made production changes to
  `updater.go`, which is expected and fine — this plan just adds tests
  around whatever state that file is in).
- Mocking `http.Get` to test `Upgrade`, `CheckLatestVersion`, or
  `downloadToTemp` end-to-end — out of scope (see plan 003's Step 2 for the
  same reasoning: no HTTP client injection exists in this codebase, and
  adding one is a larger refactor).
- Mocking `exec.Command` to test `checkAWSCLI`, `checkSessionManagerPlugin`,
  `checkCredentials`, or `checkVersion` end-to-end — out of scope; these
  call real external binaries (`aws`) and are not unit-testable without a
  mocking layer this codebase doesn't have.
- Archive extraction tests for `extract.go` (`extractFromTarGz`,
  `extractFromZip`) — while genuinely valuable (per the original audit
  finding about untested path-traversal safety in archive extraction), they
  require building fixture `.tar.gz`/`.zip` files with crafted entries,
  which is a larger, separate effort with its own design decisions about
  what malicious inputs to construct. This plan intentionally limits itself
  to the two files listed in "In scope" to stay a clean, mergeable unit; a
  follow-up plan should cover `extract.go` specifically.

## Git workflow

- Branch: `advisor/015-add-updater-doctor-test-coverage`
- Single commit (or one per package); message style example:
  `test: add characterization tests for main.go flag parsing and help text` (plan 011, for precedent on this kind
  of change) — for this plan: `test: add unit tests for updater.buildAssetName and doctor.extractJSON`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add tests for `buildAssetName`

Create or extend `internal/updater/updater_test.go` (if plans 003/004 ran
first, this file already exists with `TestSelectReleaseAssets`,
`TestSelectReleaseAssetsMissingChecksums`, `TestReplaceBinary`,
`TestReplaceBinaryMissingSource` — append to it, keeping the existing
`package updater` declaration; do not create a duplicate file):

```go
func TestBuildAssetName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		goos    string
		goarch  string
		want    string
	}{
		{"linux amd64", "1.2.3", "linux", "amd64", "act_1.2.3_linux_amd64.tar.gz"},
		{"darwin arm64", "1.2.3", "darwin", "arm64", "act_1.2.3_darwin_arm64.tar.gz"},
		{"windows amd64 uses zip", "1.2.3", "windows", "amd64", "act_1.2.3_windows_amd64.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAssetNameFor(tt.version, tt.goos, tt.goarch)
			if got != tt.want {
				t.Errorf("buildAssetNameFor(%q, %q, %q) = %q, want %q", tt.version, tt.goos, tt.goarch, got, tt.want)
			}
		})
	}
}
```

This test calls a new `buildAssetNameFor` helper (not the existing
`buildAssetName`) because `buildAssetName` hardcodes `runtime.GOOS`/
`runtime.GOARCH` internally, making it impossible to test the Windows-vs-
other branch from a non-Windows test runner (and vice versa) without this
small refactor. Add this to `internal/updater/updater.go`, replacing the
body of the existing `buildAssetName`:

```go
func buildAssetName(version string) string {
	return buildAssetNameFor(version, runtime.GOOS, runtime.GOARCH)
}

func buildAssetNameFor(version, goos, goarch string) string {
	ext := "tar.gz"
	if goos == "windows" {
		ext = "zip"
	}

	return fmt.Sprintf("act_%s_%s_%s.%s", version, goos, goarch, ext)
}
```

This is a mechanical extraction — `buildAssetName`'s existing callers
(`Upgrade` at `internal/updater/updater.go:66`) are unaffected since
`buildAssetName`'s signature and behavior are unchanged; it just delegates
to the new, more directly testable function.

**Verify**: `go build ./internal/updater/...` → exit 0.

**Verify**: `go test ./internal/updater/... -run TestBuildAssetName -v` →
all 3 subtests pass, including the Windows case run from a non-Windows
machine (this is the whole point of the refactor — you can now test all
3 branches regardless of which OS you're running the test suite on).

### Step 2: Add tests for `extractJSON`

Create `internal/doctor/doctor_test.go`:

```go
package doctor

import "testing"

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		key     string
		want    string
	}{
		{
			name:    "arn present",
			jsonStr: `{"UserId": "AIDA...", "Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/alice"}`,
			key:     "Arn",
			want:    "arn:aws:iam::123456789012:user/alice",
		},
		{
			name:    "account present",
			jsonStr: `{"UserId": "AIDA...", "Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/alice"}`,
			key:     "Account",
			want:    "123456789012",
		},
		{
			name:    "key not found",
			jsonStr: `{"UserId": "AIDA..."}`,
			key:     "Arn",
			want:    "",
		},
		{
			name:    "empty input",
			jsonStr: "",
			key:     "Arn",
			want:    "",
		},
		{
			name:    "malformed json still extracts if pattern matches",
			jsonStr: `not real json but "Arn": "arn:aws:iam::123:user/x" happens to match`,
			key:     "Arn",
			want:    "arn:aws:iam::123:user/x",
		},
		{
			name:    "no space after colon does not match (documents brittle behavior)",
			jsonStr: `{"Arn":"arn:aws:iam::123:user/x"}`,
			key:     "Arn",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.jsonStr, tt.key)
			if got != tt.want {
				t.Errorf("extractJSON(%q, %q) = %q, want %q", tt.jsonStr, tt.key, got, tt.want)
			}
		})
	}
}
```

The last test case ("no space after colon does not match") is deliberate:
it documents `extractJSON`'s actual brittle behavior (it searches for the
literal pattern `"key": "` with a space after the colon, which happens to
match AWS CLI's default JSON pretty-printing but would NOT match compact
JSON with no space) as a *characterization* test, not a bug fix — per this
plan's scope, changing `extractJSON`'s parsing approach (e.g. to use real
`encoding/json`) is out of scope; this plan only adds tests for the
existing, admittedly fragile, behavior.

**Verify**: `go test ./internal/doctor/... -run TestExtractJSON -v` → all 6
subtests pass.

### Step 3: Add tests for `checkRegion` and `checkProfile`

Append to `internal/doctor/doctor_test.go`. These two functions delegate to
`config.ResolveRegion`/`config.ResolveProfile`, which read `~/.act.json` and
environment variables — isolate the test environment the same way
`internal/config/config_test.go` does. Since `internal/doctor` doesn't
currently import a test-helper equivalent to `config_test.go`'s
`overrideHome`, define a local copy (test helpers are not exported across
packages in Go, so duplicating this small helper is the correct, idiomatic
approach — not a violation of DRY, since `internal/config`'s version is
`_test.go`-only and not importable):

```go
func overrideHomeForDoctorTest(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", orig) })
}

func TestCheckRegion(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	r := checkRegion("")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no region configured, got %v", r.Status)
	}

	r = checkRegion("us-west-2")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when region flag given, got %v", r.Status)
	}
	if r.Detail != "us-west-2" {
		t.Errorf("expected Detail 'us-west-2', got %q", r.Detail)
	}
}

func TestCheckProfile(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_PROFILE")

	r := checkProfile("")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no profile configured, got %v", r.Status)
	}

	r = checkProfile("my-profile")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when profile flag given, got %v", r.Status)
	}
	if r.Detail != "my-profile" {
		t.Errorf("expected Detail 'my-profile', got %q", r.Detail)
	}
}
```

Add `"os"` and `"testing"` to `internal/doctor/doctor_test.go`'s import
block (both needed by the code above).

This test does not use `runtime.GOOS` Windows-specific `USERPROFILE`
handling like `internal/config/config_test.go`'s `overrideHome` does,
because `config.ResolveRegion`/`ResolveProfile`'s own `Load()` call
(`internal/config/config.go:52-66`) uses `os.UserHomeDir()`, which reads
`HOME` on Unix and `USERPROFILE` on Windows — if you're running this test
suite on Windows, add the same `USERPROFILE` handling
`internal/config/config_test.go:15-19` uses, mirrored here. Check
`runtime.GOOS` before finalizing this step if your test environment is
Windows; if it's Unix/macOS, the simpler version above is sufficient.

**Verify**: `go test ./internal/doctor/... -run 'TestCheckRegion|TestCheckProfile' -v` →
all pass.

### Step 4: Run full package and repo test suites

**Verify**: `go test ./internal/updater/... -v` → all pass.

**Verify**: `go test ./internal/doctor/... -v` → all pass.

**Verify**: `go test ./...` → exit 0, all packages pass.

**Verify**: `go test ./... -cover` → `internal/updater` and
`internal/doctor` both show coverage percentages greater than `0.0%`.

## Test plan

- `internal/updater/updater_test.go`: `TestBuildAssetName` (3 cases: linux,
  darwin, windows — the windows/zip branch is now testable regardless of
  the host OS thanks to the `buildAssetNameFor` extraction).
- `internal/doctor/doctor_test.go` (new file): `TestExtractJSON` (6 cases,
  including 2 that document brittle-but-real existing behavior rather than
  idealized behavior), `TestCheckRegion` and `TestCheckProfile` (2 cases
  each: unconfigured → warn, flag-provided → pass), using a small
  locally-defined `overrideHomeForDoctorTest` helper mirroring
  `internal/config/config_test.go`'s pattern.
- Verification: `go test ./internal/updater/... -v` and
  `go test ./internal/doctor/... -v` → all pass; `go test ./...` → all
  packages pass; `go test ./... -cover` → both packages show non-zero
  coverage.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/updater/updater.go` has `buildAssetNameFor(version, goos,
      goarch string) string`, and `buildAssetName` delegates to it
- [ ] `internal/updater/updater_test.go` has `TestBuildAssetName` with 3
      passing subtests
- [ ] `internal/doctor/doctor_test.go` exists with `TestExtractJSON` (6
      subtests), `TestCheckRegion` (2 cases), `TestCheckProfile` (2 cases),
      all passing
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `go test ./... -cover` shows `internal/updater` and `internal/doctor`
      both above `0.0%`
- [ ] `git status` shows only `internal/updater/updater.go`,
      `internal/updater/updater_test.go`, `internal/doctor/doctor_test.go`
      modified/added
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `buildAssetName` or `extractJSON` don't match the excerpts above (drift
  since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- Plans 003/004 have run and `internal/updater/updater.go`'s structure
  around `buildAssetName` has changed in a way that makes the
  `buildAssetNameFor` extraction in Step 1 awkward to apply — if so, STOP
  and report the actual current state of the function rather than forcing
  the extraction to fit.
- You're running on Windows and `TestCheckRegion`/`TestCheckProfile` need
  the `USERPROFILE` handling mentioned in Step 3's note — apply it following
  `internal/config/config_test.go:15-19`'s exact pattern rather than
  improvising a different approach.

## Maintenance notes

- A follow-up plan should add tests for `internal/updater/extract.go`
  (`extractFromTarGz`, `extractFromZip`), specifically including a
  path-traversal test case (an archive entry with a `../` path component) —
  this was flagged in the original audit as untested but was deliberately
  excluded from this plan's scope to keep it small and mergeable; see "Out
  of scope" above.
- `extractJSON`'s brittleness (documented by the "no space after colon"
  test case in Step 2) is a known limitation, not a regression risk this
  plan introduces — if `internal/doctor` is ever changed to properly
  unmarshal the `aws sts get-caller-identity` JSON output into a struct
  (removing `extractJSON` entirely), that test case should be removed along
  with the function, not preserved as a "should still be brittle" contract.
- A reviewer should scrutinize: that the new `buildAssetNameFor` helper's
  extraction didn't change `buildAssetName`'s actual runtime behavior for
  real `runtime.GOOS`/`runtime.GOARCH` values — it shouldn't, since it's a
  pure delegation, but confirm by reading both functions together.
