# Plan 031: Extract a shared download-extract-run helper for the six OS installer functions

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/doctor/install_awscli.go internal/doctor/install_ssmplugin.go`
> If either file changed since this plan was written, compare the "Current
> state" excerpts below against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: M
- **Risk**: LOW (refactor of `act doctor --fix` installer plumbing; behavior-preserving, backed by new tests; the six functions this touches shell out to `sudo`/`msiexec`/`installer` which are not exercised by CI, so the real regression risk is confined to a manual smoke test, not automated coverage)
- **Depends on**: none (soft note — see "Maintenance notes" re: plan 030, which does not exist as a file in `plans/` as of this writing but was referenced in the audit as a planned checksum-verification change to these same two files)
- **Category**: tech-debt
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`internal/doctor/install_awscli.go` and `internal/doctor/install_ssmplugin.go`
together define six OS-specific installer functions
(`installAWSCLIDarwin`/`Linux`/`Windows` and
`installSSMPluginDarwin`/`Linux`/`Windows`) that each hand-roll the same
15–25 line shape: print a "Downloading … from URL" line, call
`downloadToTempFile`, optionally `os.MkdirTemp` + `unzipTo` to extract an
archive, build an `exec.Command` (sometimes prefixed with `sudo`), wire
`cmd.Stdout`/`cmd.Stderr` to the caller's `io.Writer`, run it, and print a
fixed success line. This pattern has already drifted once — the Linux SSM
plugin installer branches on `dpkg` vs `rpm` while the macOS/Windows
variants don't need that branching — which is exactly the kind of drift
that happens when a shape is copy-pasted instead of shared. Any future
installer target, or any future cross-cutting change to all six (a timeout,
a progress indicator, or a checksum-verification step), currently requires
touching six near-identical bodies by hand instead of one. Extracting the
shared "download → optionally extract → run" orchestration into a single
helper removes that duplication risk with zero behavior change, and gives
any future checksum/timeout work a single choke point to modify instead of
six.

## Current state

- `internal/doctor/install_awscli.go` (180 lines total) — the three
  OS-specific functions, quoted in full as they exist today:

  ```go
  func installAWSCLIDarwin(w io.Writer) error {
      const url = "https://awscli.amazonaws.com/AWSCLIV2.pkg"
      fmt.Fprintf(w, "Downloading AWS CLI v2 installer from %s...\n", url)
      pkgPath, err := downloadToTempFile(url, "awscliv2-*.pkg")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(pkgPath)

      fmt.Fprintf(w, "Running: sudo installer -pkg %s -target /\n", pkgPath)
      cmd := exec.Command("sudo", "installer", "-pkg", pkgPath, "-target", "/")
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "AWS CLI installed successfully.")
      return nil
  }
  ```

  ```go
  func installAWSCLILinux(w io.Writer) error {
      url := "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip"
      if runtime.GOARCH == "arm64" {
          url = "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip"
      }

      fmt.Fprintf(w, "Downloading AWS CLI v2 install bundle from %s...\n", url)
      zipPath, err := downloadToTempFile(url, "awscli-exe-linux-*.zip")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(zipPath)

      tmpDir, err := os.MkdirTemp("", "awscli-install-*")
      if err != nil {
          return fmt.Errorf("failed to create temp dir: %w", err)
      }
      defer os.RemoveAll(tmpDir)

      fmt.Fprintf(w, "Unzipping %s to %s...\n", zipPath, tmpDir)
      if err := unzipTo(zipPath, tmpDir); err != nil {
          return fmt.Errorf("unzip failed: %w", err)
      }

      installScript := filepath.Join(tmpDir, "aws", "install")
      fmt.Fprintf(w, "Running: sudo %s\n", installScript)
      cmd := exec.Command("sudo", installScript)
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "AWS CLI installed successfully.")
      return nil
  }
  ```

  ```go
  func installAWSCLIWindows(w io.Writer) error {
      const url = "https://awscli.amazonaws.com/AWSCLIV2.msi"
      fmt.Fprintf(w, "Downloading AWS CLI v2 MSI installer from %s...\n", url)
      msiPath, err := downloadToTempFile(url, "awscliv2-*.msi")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(msiPath)

      fmt.Fprintf(w, "Running: msiexec.exe /i %s /qn\n", msiPath)
      cmd := exec.Command("msiexec.exe", "/i", msiPath, "/qn")
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "AWS CLI installed successfully.")
      return nil
  }
  ```

  Note the shape differences across these three: Darwin has NO extraction
  step (the `.pkg` is installed directly by `installer`); Linux HAS
  extraction (`unzipTo` into a temp dir, then runs the extracted
  `aws/install` script); Windows has NO extraction (the `.msi` is run
  directly via `msiexec`). "Download → optionally extract → run" is the
  right generalization, where "optionally extract" must be a no-op for 2 of
  the 3 AWS CLI variants.

  Also in this file, `unzipTo` (lines 123–166) and `isWithinDir` (lines
  168–179) are the shared extraction primitive — already correctly shared,
  out of scope for this plan, do not touch.

- `internal/doctor/install_ssmplugin.go` (143 lines total) — the three
  OS-specific functions, quoted in full as they exist today:

  ```go
  func installSSMPluginDarwin(w io.Writer) error {
      const url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac/sessionmanager-bundle.zip"
      fmt.Fprintf(w, "Downloading Session Manager plugin bundle from %s...\n", url)
      zipPath, err := downloadToTempFile(url, "sessionmanager-bundle-*.zip")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(zipPath)

      tmpDir, err := os.MkdirTemp("", "ssm-plugin-install-*")
      if err != nil {
          return fmt.Errorf("failed to create temp dir: %w", err)
      }
      defer os.RemoveAll(tmpDir)

      fmt.Fprintf(w, "Unzipping %s to %s...\n", zipPath, tmpDir)
      if err := unzipTo(zipPath, tmpDir); err != nil {
          return fmt.Errorf("unzip failed: %w", err)
      }

      installScript := filepath.Join(tmpDir, "sessionmanager-bundle", "install")
      fmt.Fprintf(w, "Running: sudo %s -i /usr/local/sessionmanagerplugin -b /usr/local/bin/session-manager-plugin\n", installScript)
      cmd := exec.Command("sudo", installScript, "-i", "/usr/local/sessionmanagerplugin", "-b", "/usr/local/bin/session-manager-plugin")
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "Session Manager plugin installed successfully.")
      return nil
  }
  ```

  ```go
  func installSSMPluginLinux(w io.Writer) error {
      hasDpkg := commandExists("dpkg")
      hasRpm := commandExists("rpm")

      if !hasDpkg && !hasRpm {
          return fmt.Errorf("neither dpkg nor rpm found on this system; cannot determine package format. Install manually: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html")
      }

      useRpm := hasRpm && !hasDpkg

      var url, pattern string
      if useRpm {
          url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/linux_64bit/session-manager-plugin.rpm"
          pattern = "session-manager-plugin-*.rpm"
      } else {
          url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/ubuntu_64bit/session-manager-plugin.deb"
          pattern = "session-manager-plugin-*.deb"
      }

      fmt.Fprintf(w, "Downloading Session Manager plugin package from %s...\n", url)
      pkgPath, err := downloadToTempFile(url, pattern)
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(pkgPath)

      var cmd *exec.Cmd
      if useRpm {
          fmt.Fprintf(w, "Running: sudo rpm -i %s\n", pkgPath)
          cmd = exec.Command("sudo", "rpm", "-i", pkgPath)
      } else {
          fmt.Fprintf(w, "Running: sudo dpkg -i %s\n", pkgPath)
          cmd = exec.Command("sudo", "dpkg", "-i", pkgPath)
      }
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "Session Manager plugin installed successfully.")
      return nil
  }
  ```

  Note: `installSSMPluginLinux` has NO extraction step (the `.deb`/`.rpm`
  package is installed directly), but it makes a dpkg-vs-rpm decision
  (`hasDpkg`/`hasRpm`/`useRpm`) BEFORE the download that picks both the
  download URL/pattern AND the install command. That decision logic must
  stay outside the shared helper — it decides *what to pass in*, it is not
  part of the "download → extract → run" shape itself.

  ```go
  func installSSMPluginWindows(w io.Writer) error {
      const url = "https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe"
      fmt.Fprintf(w, "Downloading Session Manager plugin installer from %s...\n", url)
      exePath, err := downloadToTempFile(url, "SessionManagerPluginSetup-*.exe")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(exePath)

      fmt.Fprintf(w, "Running: %s /S\n", exePath)
      cmd := exec.Command(exePath, "/S")
      cmd.Stdout = w
      cmd.Stderr = w
      if err := cmd.Run(); err != nil {
          return fmt.Errorf("installer failed: %w", err)
      }
      fmt.Fprintln(w, "Session Manager plugin installed successfully.")
      return nil
  }
  ```

  Also in this file: `commandExists` (lines 139–142, used by the Linux
  dpkg/rpm decision) — out of scope, do not touch.

- `internal/doctor/download.go` (36 lines, full file) — `downloadToTempFile`,
  the already-shared download primitive this plan builds on top of, NOT
  re-implements:

  ```go
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

  Its signature (`func downloadToTempFile(url, pattern string) (string,
  error)`) takes a plain string URL, so an `httptest.NewServer`'s `.URL`
  field can be passed directly in tests with zero production-code changes
  — no dependency injection needed.

- `internal/doctor/doctor.go` — the `fixAction` struct (lines 30–41) that
  every installer plugs into:

  ```go
  type fixAction struct {
      // Describe returns a one-line, human-readable summary of what Apply
      // will do (e.g. "Download and install AWS CLI v2 for macOS via the
      // official .pkg installer"). Shown before the confirmation prompt.
      Describe func() string
      // Apply performs the fix. It must write progress to the provided
      // io.Writer (both for live stdout echo and for the fix log) and
      // return an error if the fix failed. It must be safe to call at most
      // once per fixAction.
      Apply func(w io.Writer) error
  }
  ```

  The call chain is: `checkAWSCLI()` (doctor.go line ~103) sets
  `Fix.Apply: installAWSCLI` (doctor.go line 112) — NOT one of the per-OS
  functions directly. `installAWSCLI` (install_awscli.go lines 32–43) is a
  `runtime.GOOS` switch wrapper that dispatches to
  `installAWSCLIDarwin`/`Linux`/`Windows`. Symmetrically,
  `checkSessionManagerPlugin()` (doctor.go line ~129) sets `Fix.Apply:
  installSSMPlugin` (doctor.go line 138), and `installSSMPlugin`
  (install_ssmplugin.go lines 31–42) dispatches to
  `installSSMPluginDarwin`/`Linux`/`Windows`. **This plan must NOT change
  `installAWSCLI`, `installSSMPlugin`, or either `checkX` function** — only
  the six per-OS function bodies get rewritten, and each rewritten function
  must keep the exact signature `func(w io.Writer) error` so these existing
  call sites keep compiling unchanged.

- `internal/doctor/fix_test.go` (89 lines, full file already read) —
  existing tests construct a fake `Apply` closure directly (e.g. `Apply:
  func(w io.Writer) error { ...; return nil }`) rather than exercising any
  real installer function. This confirms the refactor in this plan requires
  **zero changes** to any existing test file — only new test file(s) are
  added.

- `internal/doctor/doctor_test.go` — defines `overrideHomeForDoctorTest`,
  used elsewhere in the package for HOME-dependent tests; not needed for
  this plan's new tests (the shared helper touches neither HOME nor
  `~/.act.json`).

- Confirmed via reading both files end-to-end: no existing test file
  (`install_awscli_test.go` or `install_ssmplugin_test.go`) exists in
  `internal/doctor/` as of this writing.

## Commands you will need

| Purpose      | Command                                                    | Expected on success       |
|--------------|-------------------------------------------------------------|----------------------------|
| Build        | `go build -v ./...`                                         | exit 0                     |
| Vet          | `go vet ./...`                                              | exit 0, no output          |
| Format check | `gofmt -l .`                                                 | no output                  |
| Run new tests| `go test -v ./... -run TestRunDownloadAndInstall`            | all subtests PASS          |
| Full test    | `go test -v ./...`                                           | all tests pass, exit 0     |

## Scope

**In scope** (the only files you should create or modify):
- `internal/doctor/install_awscli.go` — rewrite the three per-OS bodies to
  call the new shared helper; add the helper itself here (see Step 1 for
  why it belongs in this file, not a new one).
- `internal/doctor/install_ssmplugin.go` — rewrite the three per-OS bodies
  to call the shared helper.
- `internal/doctor/install_test.go` (new file) — tests for the shared
  helper.

**Out of scope** (do NOT touch, even though related):
- `internal/doctor/download.go`'s `downloadToTempFile` — already correctly
  shared; this plan builds on top of it, does not modify it.
- `internal/doctor/doctor.go`'s `fixAction` struct, `checkAWSCLI`,
  `checkSessionManagerPlugin` — their call sites into the `installAWSCLI`/
  `installSSMPlugin` OS-dispatch wrapper functions must stay unchanged.
- `installAWSCLI` and `installSSMPlugin` themselves (the `runtime.GOOS`
  switch wrappers in each file, lines 32–43 and 31–42 respectively) — these
  dispatch to the per-OS functions; their signatures and bodies do not need
  to change since the per-OS functions they call keep the same
  `func(w io.Writer) error` signature.
- `unzipTo`, `isWithinDir` (install_awscli.go) and `commandExists`
  (install_ssmplugin.go) — already-correct shared primitives this plan's
  new helper calls into; do not modify their internals.
- `describeAWSCLIInstall`, `describeSSMPluginInstall` — the human-readable
  `Describe` strings shown before the confirm prompt; unrelated to the
  `Apply` code path this plan touches.
- `README.md` — this is an internal refactor with no user-facing CLI
  behavior, flag, or output change (aside from the accepted wording note in
  Step 4), so the "update README after adding a feature/flag" rule in
  `CLAUDE.md` does not apply. Do not touch `README.md`.
- `plans/030-*.md` or any file related to checksum verification — a
  separate, independent effort; see "Maintenance notes" for how the two
  interact.

## Git workflow

- Branch: `advisor/031-dedupe-installer-pattern`
- Commit style: Conventional Commits, matching this repo's refactor
  precedent from `git log --oneline`, e.g.
  `cf60ade refactor: extract shared AWS-CLI arg-building from unix/windows pairs`.
  Use: `refactor: extract shared download-extract-run helper for OS installers`
- Commit once, after all steps pass verification (Step 5).
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Add the shared helper to `internal/doctor/install_awscli.go`

Add this function to `internal/doctor/install_awscli.go` (placed after
`installAWSCLIWindows` and before `unzipTo`, so both `install_awscli.go` and
`install_ssmplugin.go` — same package, `package doctor` — can call it
unqualified). It belongs in `install_awscli.go` rather than a new file
because `unzipTo`, its sibling extraction primitive, already lives there;
keeping the orchestration helper next to it avoids a third file for closely
related code. (If you prefer a dedicated file, that is an acceptable
deviation — note it in your final report — but do not create both a new
file AND leave duplicate logic behind.)

Target shape (adapt as needed — this is a design sketch to verify against
the six real call sites, not exact code to paste verbatim):

```go
// runDownloadAndInstall downloads url to a temp file (named per pattern,
// see downloadToTempFile), optionally extracts it (when extract is
// non-nil) into a fresh temp directory, then runs the *exec.Cmd built by
// buildCmd against the resulting artifact path, streaming its output to w.
// It returns an error if any stage fails. The downloaded file and any
// extraction temp dir are always cleaned up before returning.
func runDownloadAndInstall(w io.Writer, downloadMsg, url, tempPattern string, extract func(archivePath, destDir string) (artifactPath string, err error), buildCmd func(artifactPath string) *exec.Cmd) error {
	fmt.Fprintf(w, "%s from %s...\n", downloadMsg, url)
	downloaded, err := downloadToTempFile(url, tempPattern)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(downloaded)

	artifactPath := downloaded
	if extract != nil {
		tmpDir, err := os.MkdirTemp("", "act-install-*")
		if err != nil {
			return fmt.Errorf("failed to create temp dir: %w", err)
		}
		defer os.RemoveAll(tmpDir)

		artifactPath, err = extract(downloaded, tmpDir)
		if err != nil {
			return err
		}
	}

	cmd := buildCmd(artifactPath)
	fmt.Fprintf(w, "Running: %s\n", strings.Join(cmd.Args, " "))
	cmd.Stdout = w
	cmd.Stderr = w
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("installer failed: %w", err)
	}
	return nil
}
```

Design notes to resolve while writing this:

- The `downloadMsg` parameter exists because the six functions currently
  use five distinct download-message wordings ("Downloading AWS CLI v2
  installer", "Downloading AWS CLI v2 install bundle", "Downloading AWS CLI
  v2 MSI installer", "Downloading Session Manager plugin bundle",
  "Downloading Session Manager plugin package", "Downloading Session
  Manager plugin installer"). Preserve each caller's exact original wording
  by passing it in — do not collapse them to one generic string, since that
  would be an unnecessary, avoidable message-wording change. If you find a
  cleaner way to thread this through (e.g. the caller prints its own
  "Downloading ..." line before calling the helper, and the helper does not
  print a download message itself), that is an acceptable equivalent — just
  make sure every original wording is preserved exactly, character for
  character.
- The helper deliberately does NOT print the fixed success line (e.g. "AWS
  CLI installed successfully.") — leave that in each caller, since the
  message differs by product ("AWS CLI" vs "Session Manager plugin") and
  is one line, not worth threading through another parameter.
- `unzipTo`'s existing error wrapping is `fmt.Errorf("unzip failed: %w",
  err)` in the current per-OS bodies. Since `extract` closures will call
  `unzipTo` themselves (see Step 2), have each `extract` closure do that
  wrapping itself and return the wrapped error — the helper should NOT
  double-wrap or swallow the message. In the sketch above, `extract`'s
  error is returned unwrapped by the helper (`return err`, not
  `fmt.Errorf("extract failed: %w", err)`) specifically so the closure's
  own `"unzip failed: %w"` wrapping (or whatever wrapping you write into
  the closure) is what surfaces — confirm this in Step 5's message-diff
  check.
- Required new imports for this function in `install_awscli.go`: `strings`
  is already imported (used by `isWithinDir`); `os/exec` is already
  imported. No new imports needed.

**Verify**: `go build ./...` → fails only because the six per-OS callers
haven't been rewritten yet if you left them calling old code paths that no
longer compile together — actually, since this step only ADDS a function
and changes nothing else, `go build ./...` must still succeed after this
step alone (an unused-function error does not occur in Go for package-level
funcs). Run `go build -v ./...` → exit 0.

### Step 2: Rewrite the three `installAWSCLI*` functions in `install_awscli.go` to call the helper

Rewrite each of `installAWSCLIDarwin`, `installAWSCLILinux`,
`installAWSCLIWindows` to be thin callers of `runDownloadAndInstall`,
preserving:
- the exact same URLs and `runtime.GOARCH` branching (Linux only) as today,
- the exact same temp-file patterns (`"awscliv2-*.pkg"`,
  `"awscli-exe-linux-*.zip"`, `"awscliv2-*.msi"`),
- the exact same download message text quoted in "Current state" above,
- the exact same `sudo`/`msiexec` invocation shape,
- the exact same final success line (`"AWS CLI installed successfully.\n"`
  via `fmt.Fprintln`).

For `installAWSCLIDarwin` (no extraction), pass `extract: nil` and
`buildCmd: func(artifactPath string) *exec.Cmd { return exec.Command("sudo",
"installer", "-pkg", artifactPath, "-target", "/") }`.

For `installAWSCLILinux` (has extraction), the `extract` closure must:
call `unzipTo(archivePath, destDir)`, wrapping any error as
`fmt.Errorf("unzip failed: %w", err)` (matching current behavior exactly),
and on success return `filepath.Join(destDir, "aws", "install")` as the
artifact path (this is the path to the install script inside the extracted
tree, matching today's `installScript := filepath.Join(tmpDir, "aws",
"install")`). `buildCmd` becomes `func(artifactPath string) *exec.Cmd {
return exec.Command("sudo", artifactPath) }`.

For `installAWSCLIWindows` (no extraction), pass `extract: nil` and
`buildCmd: func(artifactPath string) *exec.Cmd { return
exec.Command("msiexec.exe", "/i", artifactPath, "/qn") }`.

Each rewritten function keeps the signature `func installAWSCLIDarwin(w
io.Writer) error { ... }` (etc.) — do not change any function name or
signature, only the body.

**Verify**: `go build -v ./...` → exit 0. `go vet ./...` → exit 0, no
output.

### Step 3: Rewrite the three `installSSMPlugin*` functions in `install_ssmplugin.go` to call the helper

Rewrite `installSSMPluginDarwin`, `installSSMPluginLinux`,
`installSSMPluginWindows` the same way, preserving all URLs, temp-file
patterns, download messages, command shapes, and success lines exactly.

For `installSSMPluginDarwin` (has extraction): `extract` closure calls
`unzipTo`, wraps errors as `fmt.Errorf("unzip failed: %w", err)`, returns
`filepath.Join(destDir, "sessionmanager-bundle", "install")` as the
artifact path (matching today's `installScript`). `buildCmd` returns
`exec.Command("sudo", artifactPath, "-i", "/usr/local/sessionmanagerplugin",
"-b", "/usr/local/bin/session-manager-plugin")`.

For `installSSMPluginLinux`: the `hasDpkg`/`hasRpm`/`useRpm` decision and
the "neither found" error return MUST stay as plain code BEFORE the call to
`runDownloadAndInstall` — this decision picks the URL, temp-file pattern,
and which command to build; it is caller-side logic, not part of the shared
helper. No extraction (`extract: nil`). `buildCmd` branches on `useRpm` the
same way the current code does (`exec.Command("sudo", "rpm", "-i",
artifactPath)` vs `exec.Command("sudo", "dpkg", "-i", artifactPath)`), and
the download message/URL/pattern selection (rpm vs deb) also stays
caller-side, exactly as today.

For `installSSMPluginWindows` (no extraction): `buildCmd` returns
`exec.Command(artifactPath, "/S")`.

`commandExists` (lines 139–142) is untouched — it's still called from
`installSSMPluginLinux`'s caller-side decision logic.

**Verify**: `go build -v ./...` → exit 0. `go vet ./...` → exit 0, no
output. `gofmt -l .` → no output (run `gofmt -w internal/doctor/install_awscli.go internal/doctor/install_ssmplugin.go` if it lists either file, then re-check).

### Step 4: Manual message-diff check (no code change)

Since `sudo`/`msiexec`/`installer` cannot run in CI, you cannot get
automated coverage that the six functions still print the exact same
sequence of lines. Instead, manually diff the intended output of each
rewritten function against its original, function by function, by tracing
through the code you just wrote line by line (do not run the actual
installers — they require `sudo`/network/an actual missing dependency).
For each of the six functions, write down (in your own notes, not into any
file) the sequence of `fmt.Fprint*` calls that would fire on a hypothetical
success, and confirm each line matches the original verbatim, in the same
order, with the same trailing newline behavior (`Fprintf` with `\n` vs
`Fprintln`). If you find you had to change any wording to make the
refactor clean (e.g. because the "Running: ..." line's exact command
rendering changed shape when centralized), that is acceptable **only if
you explicitly list the exact before/after diff** in your final report —
do not silently change user-visible output.

**Verify**: no command — this is a manual trace. Confirm by stating, for
each of the six functions, either "unchanged" or the exact line(s) that
changed and why.

### Step 5: Add tests for `runDownloadAndInstall` in `internal/doctor/install_test.go`

Create `internal/doctor/install_test.go` with `package doctor` and a
`TestRunDownloadAndInstall` parent test using `t.Run` subtests. Required
imports: `"errors"`, `"io"`, `"net/http"`, `"net/http/httptest"`, `"os"`,
`"os/exec"`, `"strings"`, `"testing"`.

Model the httptest usage on `internal/doctor/download_test.go`'s
`TestDownloadToTempFile` if that file exists (added by a separate,
already-completed plan for `downloadToTempFile`/`unzipTo` coverage) — check
with `ls internal/doctor/*_test.go` first. If it does not exist, write these
tests self-sufficiently without relying on it; do not assume it is present.

Write these subtests, each starting its own `httptest.NewServer`:

1. **`t.Run("no extraction, success", ...)`**: server returns 200 + a small
   body. Call `runDownloadAndInstall(&bytes.Buffer{}, "Downloading",
   srv.URL, "install-test-*.bin", nil, func(artifactPath string) *exec.Cmd
   { return exec.Command("true") })` — use `exec.Command("true")` on
   Unix-like test runners; since CI runs on `ubuntu-latest`, `macos-latest`,
   AND `windows-latest` (per `.github/workflows/ci.yml`), `"true"` does not
   exist on Windows. Use a portable no-op instead:
   `exec.Command(os.Args[0], "-test.run=NONE")` runs the current test
   binary with a flag that matches no tests and exits 0 immediately on all
   three platforms — use this pattern for every subtest below that needs a
   "command that succeeds trivially." Assert `err == nil`.
2. **`t.Run("with extraction, success", ...)`**: server returns 200 + body.
   Pass a non-nil `extract` closure that returns a fixed artifact path
   string (e.g. `filepath.Join(destDir, "fake-artifact")`, ignoring its
   `archivePath` argument) and `nil` error — assert the `buildCmd` closure
   you pass received exactly that artifact path (capture it via a closure
   variable) and that `err == nil`. Also assert the temp dir passed to
   `extract` as `destDir` actually exists at the time `extract` is called
   (i.e. `os.Stat(destDir)` succeeds inside the closure) — proves
   `os.MkdirTemp` ran before `extract`.
3. **`t.Run("download failure propagates", ...)`**: server returns 404.
   Call with any `extract`/`buildCmd` (they must not be invoked). Assert
   `err != nil` and `strings.Contains(err.Error(), "download failed")`.
   Assert (via a bool flag set in the closures) that neither `extract` nor
   `buildCmd` was called.
4. **`t.Run("extract failure propagates", ...)`**: server returns 200 + a
   body. `extract` closure returns `"", errors.New("boom")`. Assert `err !=
   nil` and `strings.Contains(err.Error(), "boom")`. Assert `buildCmd` was
   NOT called (use a bool flag).
5. **`t.Run("command failure propagates", ...)`**: server returns 200 + a
   body. `extract: nil`. `buildCmd` returns a command guaranteed to fail
   portably on all three CI platforms — use
   `exec.Command(os.Args[0], "-test.run=NONE", "-nonexistent-flag-xyz")`
   (an unrecognized flag makes the test binary exit non-zero on all
   platforms), or simpler: `exec.Command(os.Args[0],
   "-test.run=TestRunDownloadAndInstallDeliberatelyMissing")` is fine too
   (test binaries exit 0 even for a no-match run filter, so prefer the
   unrecognized-flag form, which reliably exits non-zero). Verify your
   choice actually exits non-zero locally before relying on it. Assert `err
   != nil` and `strings.Contains(err.Error(), "installer failed")`.

Each subtest should use `&bytes.Buffer{}` (add `"bytes"` import) as the `w`
argument rather than a real file, since output content isn't the focus of
this test (Step 4 already covers exact message wording by manual trace);
only assert on `err` and on which closures were/weren't invoked.

**Verify**: `go test -v ./... -run TestRunDownloadAndInstall` → all 5
subtests PASS, exit 0.

### Step 6: Full verification pass

Run, in order:

1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output.
4. `go test -v ./... -run TestRunDownloadAndInstall` → 5 subtests PASS.
5. `go test -v ./...` → full suite passes, exit 0, with zero change to any
   pre-existing test's pass/fail status.
6. `git status` → confirms only `internal/doctor/install_awscli.go`,
   `internal/doctor/install_ssmplugin.go`, and (new file)
   `internal/doctor/install_test.go` are changed/added.

**Verify**: all six checks above complete with the stated expected result.

## Test plan

- New test `TestRunDownloadAndInstall` in `internal/doctor/install_test.go`:
  5 subtests — no-extraction success, with-extraction success, download
  failure propagates, extract failure propagates, command failure
  propagates (full list and exact assertions in Step 5).
- Structural pattern to follow: `httptest.NewServer` + `t.Run` subtests, the
  same technique used by the (possibly already-landed) `downloadToTempFile`
  tests — self-sufficient here regardless of whether that file exists.
- The six real `installXxxYyy` functions themselves remain untested by
  automation (they shell out to `sudo`/`msiexec`/`installer`, not
  CI-testable) — this was already true before this plan and is unchanged
  by it. This plan's new coverage is on the shared helper only.
- Verification: `go test -v ./... -run TestRunDownloadAndInstall` → 5
  subtests pass; then `go test -v ./...` → full suite passes with no
  regressions.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `runDownloadAndInstall` (or equivalently named helper — note any name
      change in your report) exists in `internal/doctor/install_awscli.go`
      and is called by all six `installAWSCLIDarwin/Linux/Windows` and
      `installSSMPluginDarwin/Linux/Windows` functions
- [ ] `grep -c "cmd.Stdout = w" internal/doctor/install_awscli.go internal/doctor/install_ssmplugin.go` sums to 1 total (only inside the shared helper), not 6 — confirms the duplicated wiring was actually removed from the six callers
- [ ] `internal/doctor/install_test.go` exists and contains
      `func TestRunDownloadAndInstall(t *testing.T)` with 5 subtests
- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./... -run TestRunDownloadAndInstall` passes, 5 subtests
- [ ] `go test -v ./...` exits 0, all tests pass (existing + new)
- [ ] All six `installXxxYyy` functions keep their exact original names and
      `func(w io.Writer) error` signatures (`grep -n "^func install" internal/doctor/install_awscli.go internal/doctor/install_ssmplugin.go` shows the same 8 function names as today: 3 `installAWSCLI*` + `installAWSCLI` wrapper in the first file, 3 `installSSMPlugin*` + `installSSMPlugin` wrapper in the second)
- [ ] `doctor.go` is unmodified (`git diff --stat -- internal/doctor/doctor.go` shows no changes)
- [ ] No files outside `internal/doctor/install_awscli.go`,
      `internal/doctor/install_ssmplugin.go`, and
      `internal/doctor/install_test.go` are modified or created
- [ ] `plans/README.md` status row for plan 031 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at either `internal/doctor/install_awscli.go` or
  `internal/doctor/install_ssmplugin.go` doesn't match the excerpts quoted
  in "Current state" (the codebase has drifted since this plan was
  written — re-derive the expected behavior before proceeding).
- A step's verification fails twice after a reasonable fix attempt.
- The shared-helper shape sketched in Step 1 turns out not to fit one of
  the six real functions after you try it (e.g. a caller needs to run
  cleanup between extraction and the command that the helper's structure
  can't express) — stop and report which function doesn't fit and why,
  rather than forcing an awkward shape or leaving that one function
  un-refactored without saying so.
- You find that making this change would require touching `doctor.go`'s
  `fixAction`, `checkAWSCLI`, or `checkSessionManagerPlugin` — that would
  mean the call-chain assumption in "Current state" is wrong; stop and
  report rather than editing those out-of-scope functions.
- Any of the six rewritten functions would need to change its printed
  output wording beyond what you can list exactly in Step 4's trace (i.e.
  you can't produce a clean before/after diff of every line) — stop and
  report rather than shipping an unaccounted-for user-visible message
  change.

## Maintenance notes

- **Interaction with checksum verification work (plan 030-equivalent, if/when
  it exists in `plans/`)**: as of this writing there is no
  `plans/030-*.md` file in the repo, but the audit that produced this plan
  anticipated a separate, independent plan adding checksum verification to
  these same two files' download step. If such a plan exists or is written
  later, and it hasn't landed yet, this plan (031, a pure refactor with no
  new behavior) should land FIRST — that lets the checksum work add its
  verification step once, inside `runDownloadAndInstall`, instead of into
  six separate functions. If the checksum plan lands FIRST instead, this
  plan's executor should rebase the extraction on top of the checksum
  change's structure rather than reverting it — the two changes touch
  overlapping code (the same six functions) for different purposes
  (dedup vs. new verification behavior) and both should survive. This is a
  soft recommendation, not a hard blocking dependency; either order is
  technically executable.
- If a 7th installer target is ever added (e.g. a different Linux package
  format, or a future Windows ARM64 variant), it should be written as a
  caller of `runDownloadAndInstall`, not as a new copy-pasted 20-line
  function — that is the entire point of this refactor. A reviewer seeing a
  new `installXxxYyy` function that hand-rolls `downloadToTempFile` +
  `exec.Command` + `cmd.Stdout = w` wiring without going through the shared
  helper should flag it.
- A reviewer of this plan's PR should specifically check: (1) that no
  download URL, temp-file pattern, or exact printed message changed
  (Step 4's trace should show "unchanged" for all six, or an explicitly
  listed diff); (2) that `installSSMPluginLinux`'s dpkg/rpm decision logic
  is still caller-side, not pushed into the shared helper; (3) that
  `doctor.go` has zero diff.
- This plan does not attempt to make the `sudo`/`msiexec`/`installer`
  invocation itself mockable/testable end-to-end — that would be a larger
  refactor (e.g. injecting a command-runner interface) and is out of scope
  here. The new test coverage is limited to the shared helper's control
  flow using no-op/failing stand-in commands, not the real installers.
