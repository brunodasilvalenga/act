# Plan 003: Make self-upgrade checksum verification fail-closed

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

- **Priority**: P1
- **Effort**: M
- **Risk**: MED
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`act upgrade` downloads a new release binary and, separately, an optional
`checksums.txt` asset to verify it. Today, checksum verification only runs
`if checksumsURL != ""` (`internal/updater/updater.go:89-93`) — if a GitHub
release is missing the `checksums.txt` asset (a renamed asset, a partial
release, a GoReleaser config change, or GitHub API flakiness that drops one
asset from the listing), `Upgrade` silently skips verification entirely and
proceeds straight to extracting and installing the downloaded binary via
`replaceBinary`, which runs with the user's OS-level privileges to overwrite
the currently-running executable.

This is a fail-open design: the absence of a security control is treated as
"verification passed" rather than "verification could not be performed."
The fix makes a missing checksums asset a hard error instead of a silent
skip, so `act upgrade` never installs an unverified binary. (Note: even with
this fix, the checksums file itself is fetched from the same unauthenticated
GitHub release as the binary, so this does not add cryptographic signature
verification — it only ensures the existing SHA-256 comparison isn't
silently bypassed. Signature verification would be a larger, separate
change and is not part of this plan's scope.)

## Current state

`internal/updater/updater.go:44-112` (full `Upgrade` function):

```go
func Upgrade(currentVersion string) error {
	resp, err := http.Get(repoAPI)
	if err != nil {
		return fmt.Errorf("failed to fetch release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("failed to parse release info: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	if latestVersion == currentVersion {
		fmt.Println("Already up to date.")
		return nil
	}

	assetName := buildAssetName(latestVersion)
	var downloadURL, checksumsURL string
	for _, asset := range release.Assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}

	if downloadURL == "" {
		return fmt.Errorf("no release asset found for %s/%s (expected %s)", runtime.GOOS, runtime.GOARCH, assetName)
	}

	fmt.Printf("Downloading %s...\n", assetName)

	tmpFile, err := downloadToTemp(downloadURL)
	if err != nil {
		return err
	}
	defer os.Remove(tmpFile)

	if checksumsURL != "" {
		if err := verifyChecksum(tmpFile, assetName, checksumsURL); err != nil {
			return err
		}
	}

	binary, err := extractBinary(tmpFile, assetName)
	if err != nil {
		return err
	}
	defer os.Remove(binary)

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	if err := replaceBinary(binary, execPath); err != nil {
		return err
	}

	fmt.Printf("Successfully upgraded to v%s\n", latestVersion)
	return nil
}
```

The bug is at lines 89-93: `if checksumsURL != "" { ... }` — when
`checksumsURL` is empty (no `checksums.txt` asset was found in the loop at
lines 68-75), verification is skipped entirely and execution falls through to
`extractBinary`/`replaceBinary`.

Confirmed via `.goreleaser.yml:26-27`, this project's release process always
generates a `checksums.txt` asset:
```yaml
checksum:
  name_template: "checksums.txt"
```
So under normal operation `checksumsURL` should always be populated — the
fail-open branch only matters in the abnormal case (asset missing/renamed/API
hiccup), which is exactly the case this plan protects against.

`verifyChecksum` itself (`internal/updater/updater.go:151-194`) is unchanged
by this plan — it already returns an error on checksum mismatch or on
failure to fetch/read the checksums file; it just isn't always called.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (this package) | `go test ./internal/updater/...` | exit 0, all pass, including new tests |
| Full test suite | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `internal/updater/updater.go` (the `Upgrade` function, lines 89-93 only)
- `internal/updater/updater_test.go` (create — new file)

**Out of scope** (do NOT touch, even though they look related):
- `internal/updater/extract.go` — archive extraction logic; unrelated to
  this plan (a separate finding, tracked in plan `015`, covers adding tests
  for extraction, but no behavior change to extraction itself is in scope
  here).
- `.goreleaser.yml` — already correctly configured to always produce
  `checksums.txt`; no change needed.
- Adding GPG/Sigstore signature verification — that is a materially larger
  change (key management, signing in the release pipeline, verification
  logic) and is explicitly out of scope for this plan. This plan only closes
  the fail-open gap in the existing SHA-256 comparison.
- `doctor.go`'s `checkVersion` (`internal/doctor/doctor.go:211-241`) — it
  calls `updater.CheckLatestVersion`, not `Upgrade`, and is unaffected by
  this change.

## Git workflow

- Branch: `advisor/003-fail-closed-checksum-verification`
- Single commit; message style example from `git log`: `feat: add ec2 rdp command for Windows RDP via SSM` for feature-shaped commits, or for this fix-shaped change:
  `fix: fail closed when release checksums are missing during upgrade`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Change the fail-open branch to fail-closed

In `internal/updater/updater.go`, replace lines 89-93:

```go
	if checksumsURL != "" {
		if err := verifyChecksum(tmpFile, assetName, checksumsURL); err != nil {
			return err
		}
	}
```

with:

```go
	if checksumsURL == "" {
		return fmt.Errorf("checksums.txt asset not found in release %s; refusing to install an unverified binary", release.TagName)
	}
	if err := verifyChecksum(tmpFile, assetName, checksumsURL); err != nil {
		return err
	}
```

**Verify**: `go build ./internal/updater/...` → exit 0.

### Step 2: Write a test proving the fail-closed behavior

The existing code has no test file for this package. Structure the new test
around the two collaborators that are already pure/testable without a real
network call: `buildAssetName` (pure, no network) and the asset-selection
loop's outcome. Since `Upgrade` itself does a real `http.Get` to
`repoAPI` (a hardcoded constant, not injectable), you cannot unit-test the
full `Upgrade` function's fail-closed branch without a network call or a
refactor. Do **not** refactor `Upgrade` to accept an injectable HTTP client —
that's out of scope for this plan (it would be a larger structural change;
if desired later, it should be its own plan). Instead, test the logic in
isolation by extracting the asset-selection decision into a small, pure,
already-testable helper.

Add this pure helper to `internal/updater/updater.go`, right after
`buildAssetName` (after line 124):

```go
// selectReleaseAssets finds the download URL for the current platform's
// asset and the checksums.txt URL among a release's assets. It does not
// perform any I/O.
func selectReleaseAssets(assets []struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}, assetName string) (downloadURL, checksumsURL string) {
	for _, asset := range assets {
		if asset.Name == assetName {
			downloadURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumsURL = asset.BrowserDownloadURL
		}
	}
	return downloadURL, checksumsURL
}
```

Then replace the inline loop in `Upgrade` (originally lines 68-75) with a
call to this helper:

```go
	assetName := buildAssetName(latestVersion)
	downloadURL, checksumsURL := selectReleaseAssets(release.Assets, assetName)
```

Note: `release.Assets` is typed as an anonymous struct slice matching the
`githubRelease.Assets` field (`internal/updater/updater.go:17-23`) — confirm
the helper's parameter type matches that anonymous struct exactly, or Go
will reject the call with a type mismatch. If the anonymous struct type
causes friction, it is acceptable to instead give the `Assets` field's
element type a name (e.g. `type releaseAsset struct { Name string
\`json:"name"\`; BrowserDownloadURL string \`json:"browser_download_url"\` }`)
and use `[]releaseAsset` in both `githubRelease` and the new helper's
signature — this is a mechanical rename, not a behavior change.

**Verify**: `go build ./internal/updater/...` → exit 0.

### Step 3: Add the test file

Create `internal/updater/updater_test.go`:

```go
package updater

import "testing"

func TestSelectReleaseAssets(t *testing.T) {
	assets := []releaseAsset{
		{Name: "act_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/binary"},
		{Name: "checksums.txt", BrowserDownloadURL: "https://example.com/checksums"},
		{Name: "act_1.2.3_darwin_amd64.tar.gz", BrowserDownloadURL: "https://example.com/other-binary"},
	}

	downloadURL, checksumsURL := selectReleaseAssets(assets, "act_1.2.3_linux_amd64.tar.gz")
	if downloadURL != "https://example.com/binary" {
		t.Errorf("expected binary download URL, got %q", downloadURL)
	}
	if checksumsURL != "https://example.com/checksums" {
		t.Errorf("expected checksums URL, got %q", checksumsURL)
	}
}

func TestSelectReleaseAssetsMissingChecksums(t *testing.T) {
	assets := []releaseAsset{
		{Name: "act_1.2.3_linux_amd64.tar.gz", BrowserDownloadURL: "https://example.com/binary"},
	}

	downloadURL, checksumsURL := selectReleaseAssets(assets, "act_1.2.3_linux_amd64.tar.gz")
	if downloadURL != "https://example.com/binary" {
		t.Errorf("expected binary download URL, got %q", downloadURL)
	}
	if checksumsURL != "" {
		t.Errorf("expected empty checksums URL when asset is missing, got %q", checksumsURL)
	}
}
```

If you used the named-type approach in Step 2 (`releaseAsset`), this test
compiles as-is. If you kept the anonymous struct, adjust the test's slice
literal type to match exactly what `selectReleaseAssets` expects.

This proves the *input condition* that triggers the fail-closed branch
(`checksumsURL == ""`) is correctly detected; the fail-closed branch itself
(the `return fmt.Errorf(...)` in `Upgrade`) is a one-line `if` that doesn't
need its own test beyond a manual code read, since testing `Upgrade` end-to-
end would require mocking `http.Get`, which is out of scope per this step's
guidance above.

**Verify**: `go test ./internal/updater/... -v` → both new tests pass.

### Step 4: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass.

## Test plan

- New test file: `internal/updater/updater_test.go`.
- Cases: asset selection when checksums.txt is present (happy path) and when
  it's absent (the case that must now trigger the fail-closed error in
  `Upgrade`) — both via the pure `selectReleaseAssets` helper.
- No test exercises the real `http.Get`-based `Upgrade` flow end-to-end;
  that would require injecting an HTTP client, which this plan explicitly
  defers (see Step 2's note).
- Verification: `go test ./internal/updater/... -v` → all pass.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/updater/updater.go`'s `Upgrade` function returns an error
      when `checksumsURL == ""` instead of skipping verification
- [ ] `grep -n 'if checksumsURL != ""' internal/updater/updater.go` returns
      no matches (the fail-open branch is gone)
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, including new tests in
      `internal/updater/updater_test.go`
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/updater/updater.go:44-112` doesn't match the excerpt
  above (drift since this plan was written) — re-read the whole function
  before changing anything.
- A step's verification fails twice after a reasonable fix attempt.
- Extracting `selectReleaseAssets` reveals that `githubRelease.Assets`'s
  anonymous struct type can't be cleanly referenced from a standalone
  function signature in your Go version — if so, use the named-type
  (`releaseAsset`) approach described in Step 2 rather than fighting the
  anonymous type; do not skip writing the test.
- You're tempted to add real HTTP-client mocking to test `Upgrade` end-to-end
  — that's explicitly out of scope; stick to testing the pure helper.

## Maintenance notes

- If a future change adds signature verification (GPG/Sigstore) on top of
  this checksum check, it should be added as an additional fail-closed gate
  alongside this one, not a replacement for it.
- A reviewer should scrutinize: that the new error message is clear enough
  for a user to understand ("why did my upgrade fail?") and that it doesn't
  leave the temp download file uncleaned (it doesn't — `defer os.Remove(tmpFile)`
  at line 87 already covers all early returns).
- This plan intentionally does not touch `internal/updater/extract.go`; if
  plan `015` (test coverage for updater/doctor) executes after this one, its
  executor should test the *new* fail-closed branch too, not just the
  pre-existing `verifyChecksum`/extraction logic.
