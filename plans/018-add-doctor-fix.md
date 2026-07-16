# Plan 018: Add `act doctor --fix` to auto-remediate failing checks

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat e30ffe4..HEAD -- main.go internal/doctor/ internal/config/config.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (downloads and installs third-party binaries onto the user's
  system; must never do so without either an explicit interactive
  confirmation or the explicit `--skip-confirm` opt-in)
- **Depends on**: none
- **Category**: feature
- **Planned at**: commit `e30ffe4`, 2026-07-16

## Why this matters

`act doctor` today (`internal/doctor/doctor.go`) only *reports* problems —
missing AWS CLI, missing Session Manager plugin, bad credentials, no region
configured, no `~/.act.json`. For the two checks that are actually fixable by
installing something (AWS CLI, Session Manager plugin), the user still has to
leave `act`, read the printed URL, and manually install — on three different
OSes with three different install mechanisms. This plan adds `act doctor
--fix`: for each failing check that has a known remediation, it prints what
it is about to do, asks for confirmation (unless `--skip-confirm` is passed),
performs the fix, and logs the outcome to a file the user can review later.
Checks with no automated fix (credentials, region/profile config) keep
reporting only, unchanged.

## Current state

- `internal/doctor/doctor.go` — the entire doctor package (257 lines). Key
  pieces the fix feature must integrate with:

  ```go
  // internal/doctor/doctor.go:34-43
  func Run(profile, region, version string) error {
      results := []result{
          checkAWSCLI(),
          checkSessionManagerPlugin(),
          checkCredentials(profile, region),
          checkRegion(region),
          checkProfile(profile),
          checkConfigFile(),
          checkVersion(version),
      }
  ```

  ```go
  // internal/doctor/doctor.go:22-26
  type result struct {
      Name   string
      Status status
      Detail string
  }
  ```

  ```go
  // internal/doctor/doctor.go:74-94
  func checkAWSCLI() result {
      path, err := exec.LookPath("aws")
      if err != nil {
          return result{
              Name:   "AWS CLI",
              Status: statusFail,
              Detail: "not found. Install: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
          }
      }
      out, err := exec.Command("aws", "--version").Output()
      ver := strings.TrimSpace(string(out))
      if err != nil || ver == "" {
          ver = "unknown version"
      }
      return result{
          Name:   "AWS CLI",
          Status: statusPass,
          Detail: fmt.Sprintf("%s (%s)", path, ver),
      }
  }
  ```

  ```go
  // internal/doctor/doctor.go:96-110
  func checkSessionManagerPlugin() result {
      path, err := exec.LookPath("session-manager-plugin")
      if err != nil {
          return result{
              Name:   "Session Manager plugin",
              Status: statusFail,
              Detail: "not found. Install: https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html",
          }
      }
      return result{
          Name:   "Session Manager plugin",
          Status: statusPass,
          Detail: path,
      }
  }
  ```

  The other four checks (`checkCredentials`, `checkRegion`, `checkProfile`,
  `checkConfigFile`, `checkVersion`) have **no automated fix** in scope — see
  "Out of scope" below.

- `main.go:150-157` — current `doctor` dispatch, no flags parsed at all:

  ```go
  // main.go:150-157
  case "doctor":
      subArgs := args[1:]
      if hasHelp(subArgs) {
          printDoctorHelp()
          os.Exit(0)
      }
      doctor.Run(profile, region, version)
      os.Exit(0)
  ```

- `main.go:469-481` — current help text to extend:

  ```go
  // main.go:469-481
  func printDoctorHelp() {
      fmt.Fprintf(os.Stderr, `act doctor - Check system dependencies and configuration

  Usage: act [global flags] doctor

  Checks that all required tools are installed, credentials are valid,
  and configuration is correct.

  Global Flags:
    --profile    AWS profile to use
    --region     AWS region to use
  `)
  }
  ```

- `main.go:495-511` — the repo's one existing "ask user y/N" pattern, in
  `runInit`. Match this exact style (no third-party prompt library is used
  anywhere in this repo):

  ```go
  // main.go:495-511
  func runInit() {
      reader := bufio.NewReader(os.Stdin)

      if config.Exists() {
          cfg := config.Load()
          fmt.Printf("~/.act.json already exists:\n")
          fmt.Printf("  profile: %s\n", cfg.DefaultProfile)
          fmt.Printf("  region: %s\n", cfg.DefaultRegion)
          fmt.Println()
          fmt.Print("Overwrite? [y/N]: ")
          answer, _ := reader.ReadString('\n')
          answer = strings.TrimSpace(strings.ToLower(answer))
          if answer != "y" && answer != "yes" {
              fmt.Println("Aborted.")
              return
          }
          fmt.Println()
      }
  ```

- `internal/updater/updater.go` — the repo's only existing "download +
  verify + install a binary" flow (`Upgrade`, lines 43-104). Reuse its
  shape for fix actions that download something: fetch to a temp file,
  verify (checksum where the upstream vendor publishes one), move into
  place, clean up temp files with `defer os.Remove(...)`. Do **not** import
  or call into `internal/updater` directly — it upgrades the `act` binary
  itself, a different concern. Follow its *pattern*, not its code.

- `internal/config/config.go` — `ConfigPath()` (line 22) returns
  `~/.act.json`'s path and is the established way to build a path under the
  user's home directory. The fix log file should live alongside it: use
  `filepath.Join(filepath.Dir(config.ConfigPath()), ".act-doctor-fix.log")`
  so it lands in `~` without hardcoding home-dir resolution a second time.

- `go.mod` — module is `github.com/brunodasilvalenga/act`, Go `1.26.5`, only
  external deps are `github.com/charmbracelet/{bubbletea,lipgloss}` and
  their transitives (check `go.mod` directly if in doubt: `cat go.mod`). No
  HTTP client library beyond stdlib `net/http` (already used in
  `internal/updater/updater.go`) — use stdlib for all new HTTP/download code,
  do not add a dependency.

- No `internal/doctor/*_unix.go` / `*_windows.go` split exists today — this
  package has no OS-specific files yet. `internal/aws/` has the established
  pattern for splitting OS-specific code via build tags (see
  `internal/aws/ssh_unix.go` / `ssh_windows.go`, each starting with
  `//go:build !windows` / `//go:build windows`). This plan does **not**
  need a full unix/windows split — see Step 2's approach (branch on
  `runtime.GOOS` inside one file, not separate build-tagged files) — because
  the install commands per-OS are short enough that a single file with a
  switch reads more clearly than three tiny files, and there's no
  syscall-level behavior (unlike `ssh_unix.go`'s `syscall.Exec`) forcing a
  build-tag split.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output (no unformatted files) |
| Test | `go test -v ./...` | all tests pass, exit 0 |

(Verified against `.github/workflows/ci.yml`, which runs
`go build -v ./...` and `go test -v ./...` on ubuntu-latest, macos-latest,
and windows-latest — this feature's tests must pass on all three, since CI
runs the full matrix. Any test that shells out to a real installer (`brew`,
`apt`, `msiexec`, etc.) must not run in `go test` — see Step 3's testing
approach.)

## Scope

**In scope**:
- `internal/doctor/doctor.go` (add fix plumbing, extend `checkAWSCLI` /
  `checkSessionManagerPlugin` results with a `Fixable` capability — additive)
- `internal/doctor/fix.go` (new file — fix orchestration: confirm prompt,
  logging, dispatch to per-tool installers)
- `internal/doctor/fix_test.go` (new file)
- `internal/doctor/install_awscli.go` (new file — AWS CLI installer, all OSes)
- `internal/doctor/install_ssmplugin.go` (new file — Session Manager plugin
  installer, all OSes)
- `main.go` (add `--fix` and `--skip-confirm` flags to the `doctor` dispatch,
  update `printDoctorHelp` — additive only)
- `README.md` (Features list, Commands/Examples sections, per `CLAUDE.md`'s
  rule)
- `plans/README.md` (status row update when done)

**Out of scope** (do NOT touch, even though related):
- `checkCredentials`, `checkRegion`, `checkProfile`, `checkConfigFile`,
  `checkVersion` — none of these have a safe, unambiguous automated fix
  (credentials require the user's own secrets; region/profile are user
  choices, not defects; config file already has `act init` for that; version
  already has `act upgrade`). Leave all five exactly as they are. If you're
  tempted to make `--fix` call `act init` or `act upgrade` automatically for
  these, don't — that's a materially different, riskier scope (running
  interactive `init` or replacing the running binary as a *side effect* of
  `doctor`) and is not part of this plan.
- `internal/updater/` — do not modify. Only imitate its download/verify
  pattern (see Current state); this plan's installers are self-contained in
  `internal/doctor/`.
- Any new third-party dependency (prompt libraries, HTTP clients, installer
  SDKs) — stdlib only, matching the rest of the repo.
- Windows registry changes, PATH mutation beyond what the vendor's official
  installer already does — if the official AWS/Session Manager installer
  for a platform updates PATH itself (e.g. the AWS CLI v2 MSI on Windows),
  rely on that; do not add extra PATH-editing logic on top.
- A `--yes`/`--force`-style flag that skips confirmation for *destructive*
  future doctor fixes — this plan's `--skip-confirm` only ever gates
  *installs*, never deletions or overwrites; if a future fixable check needs
  to overwrite/delete something, that's a new decision point, not covered
  here.

## Git workflow

- Branch: no strong convention observed (repo history shows both direct
  commits and `advisor/NNN-*` branches merged via `merge:` commits); use
  `advisor/018-add-doctor-fix` to match the majority of recent history.
- Commit message style (from `git log --oneline -10`): Conventional Commits
  for the subject line, e.g. `feat: add ssm run command to execute commands
  via SSM Run Command`. Use: `feat: add doctor --fix to auto-remediate
  missing AWS CLI / Session Manager plugin`
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Define the fix data model in `internal/doctor/doctor.go`

Extend the existing `result` struct with an optional fix hook, without
changing any existing field or check's behavior when no fix runs:

```go
// internal/doctor/doctor.go — extend the result struct
type result struct {
	Name   string
	Status status
	Detail string
	Fix    *fixAction // nil when this check has no automated fix
}

// fixAction describes an automated remediation for a failing check.
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

Wire `checkAWSCLI` and `checkSessionManagerPlugin` to populate `Fix` only in
their failing branch, calling into the new installer files (Steps 2-3):

```go
// checkAWSCLI's failing branch becomes:
if err != nil {
	return result{
		Name:   "AWS CLI",
		Status: statusFail,
		Detail: "not found. Install: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html",
		Fix: &fixAction{
			Describe: describeAWSCLIInstall,
			Apply:    installAWSCLI,
		},
	}
}
```

(Same pattern for `checkSessionManagerPlugin`, calling
`describeSSMPluginInstall` / `installSSMPlugin` from Step 3.) The passing
branches of both functions are unchanged — `Fix` stays nil.

**Verify**: `go build ./internal/doctor/...` → fails at this step (the
`describeAWSCLIInstall` etc. symbols don't exist yet) — that's expected;
proceed to Step 2 before verifying a clean build. Add `"io"` to the file's
import block.

### Step 2: Add `internal/doctor/install_awscli.go`

One file, branching on `runtime.GOOS` — no build-tag split (see Current
state's rationale). Structure:

```go
package doctor

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

func describeAWSCLIInstall() string {
	switch runtime.GOOS {
	case "darwin":
		return "Download the official AWS CLI v2 .pkg installer from " +
			"awscli.amazonaws.com and install it with 'sudo installer' " +
			"(requires sudo password)"
	case "linux":
		return "Download the official AWS CLI v2 install bundle from " +
			"awscli.amazonaws.com, unzip it, and run its bundled " +
			"'./aws/install' script (requires sudo)"
	case "windows":
		return "Download the official AWS CLI v2 MSI installer from " +
			"awscli.amazonaws.com and run it silently via msiexec"
	default:
		return fmt.Sprintf("no automated installer available for GOOS=%s", runtime.GOOS)
	}
}

func installAWSCLI(w io.Writer) error {
	switch runtime.GOOS {
	case "darwin":
		return installAWSCLIDarwin(w)
	case "linux":
		return installAWSCLILinux(w)
	case "windows":
		return installAWSCLIWindows(w)
	default:
		return fmt.Errorf("no automated AWS CLI installer for GOOS=%s; install manually: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html", runtime.GOOS)
	}
}
```

Implement each OS function using `os/exec` + stdlib `net/http` downloads,
following `internal/updater/updater.go`'s download-to-temp-file pattern
(`downloadToTemp`, lines 141-162) — do not import `internal/updater`, just
mirror its shape (`http.Get`, check `resp.StatusCode == http.StatusOK`,
`os.CreateTemp`, `io.Copy`, `defer os.Remove(...)`). Every step of every
`installXxx` function must write a line to `w` describing what it's doing
before doing it (this is the "verbose" requirement — see Why this matters),
e.g.:

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

For `linux`: download
`https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip` (or
`awscli-exe-linux-aarch64.zip` on `runtime.GOARCH == "arm64"`), unzip to a
temp dir with `archive/zip` (stdlib — same package
`internal/updater/extract.go` already uses for `.zip`, follow its
`extractFromZip` pattern for reading entries), then run
`<tmpdir>/aws/install` via `exec.Command("sudo", filepath.Join(tmpDir, "aws", "install"))`.

For `windows`: download
`https://awscli.amazonaws.com/AWSCLIV2.msi` to a temp file, then run
`exec.Command("msiexec.exe", "/i", msiPath, "/qn")` (`/qn` = silent, no UI —
still gated behind the same confirmation prompt from Step 4, so silence here
is about the installer's own UI, not about skipping user consent).

Add a small shared helper `downloadToTempFile(url, pattern string) (string, error)`
at the bottom of this file (or a new `internal/doctor/download.go` if you
prefer to share it with Step 3 — pick one, keep it DRY across the two
installer files since both need it).

**Verify**: `go build ./internal/doctor/...` → exit 0. Do not run the
installer functions yet (no test invokes real installers — see Step 5).

### Step 3: Add `internal/doctor/install_ssmplugin.go`

Same shape as Step 2, for the Session Manager plugin. Official download
URLs (from
https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html,
verify these are still current before hardcoding — if the plan's URLs are
stale, use the current ones from that page and note the correction in your
final report):

- macOS: `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac/sessionmanager-bundle.zip`
  → unzip, run `sudo ./sessionmanager-bundle/install -i /usr/local/sessionmanagerplugin -b /usr/local/bin/session-manager-plugin`
- Linux (x86_64): `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/ubuntu_64bit/session-manager-plugin.deb`
  (use `.rpm` variant + `sudo yum install -y` / `.deb` + `sudo dpkg -i` —
  detect via `exec.LookPath("dpkg")` vs `exec.LookPath("rpm")`, defaulting to
  the `.deb` path if neither is found and reporting the ambiguity in the
  error) → `sudo dpkg -i session-manager-plugin.deb` (or `sudo rpm -i
  session-manager-plugin.rpm`)
- Windows: `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe`
  → run silently: `exec.Command(exePath, "/S")`

Structure mirrors Step 2 exactly: `describeSSMPluginInstall() string`,
`installSSMPlugin(w io.Writer) error` switching on `runtime.GOOS`, one
per-OS function each, verbose `fmt.Fprintf(w, ...)` before every download
and every command execution.

**Verify**: `go build ./internal/doctor/...` → exit 0.

### Step 4: Add `internal/doctor/fix.go` — orchestration, confirmation, logging

This is the piece that ties fixes to `act doctor --fix`. Add:

```go
package doctor

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brunodasilvalenga/act/internal/config"
)

// fixLogPath returns the path to the doctor fix log, alongside ~/.act.json.
func fixLogPath() string {
	cfgPath := config.ConfigPath()
	if cfgPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(cfgPath), ".act-doctor-fix.log")
}

// runFixes walks results in order, and for every statusFail result with a
// non-nil Fix, prompts for confirmation (unless skipConfirm is true), runs
// the fix, and appends a timestamped record to the fix log. It returns the
// updated results slice (re-running the originating check after a
// successful fix, so the final report reflects the post-fix state) and
// whether every attempted fix succeeded.
func runFixes(results []result, skipConfirm bool, recheck func(name string) result) ([]result, bool) {
	logPath := fixLogPath()
	var logFile *os.File
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err == nil {
			logFile = f
			defer logFile.Close()
		}
	}

	reader := bufio.NewReader(os.Stdin)
	allOK := true

	for i, r := range results {
		if r.Status != statusFail || r.Fix == nil {
			continue
		}

		desc := r.Fix.Describe()
		fmt.Printf("\n%s: %s\n", r.Name, r.Detail)
		fmt.Printf("Proposed fix: %s\n", desc)

		if !skipConfirm {
			fmt.Print("Proceed? [y/N]: ")
			answer, _ := reader.ReadString('\n')
			answer = strings.TrimSpace(strings.ToLower(answer))
			if answer != "y" && answer != "yes" {
				fmt.Println("Skipped.")
				logLine(logFile, r.Name, "SKIPPED (user declined)")
				continue
			}
		}

		var writers []io.Writer = []io.Writer{os.Stdout}
		if logFile != nil {
			writers = append(writers, logFile)
		}
		w := io.MultiWriter(writers...)

		fmt.Fprintf(w, "[%s] Fixing %s...\n", timestamp(), r.Name)
		err := r.Fix.Apply(w)
		if err != nil {
			fmt.Fprintf(w, "[%s] FAILED: %v\n", timestamp(), err)
			allOK = false
			continue
		}
		fmt.Fprintf(w, "[%s] Done.\n", timestamp())

		if recheck != nil {
			results[i] = recheck(r.Name)
		}
	}

	if logPath != "" {
		fmt.Printf("\nFix log written to %s\n", logPath)
	}
	return results, allOK
}

func logLine(f *os.File, name, msg string) {
	if f == nil {
		return
	}
	fmt.Fprintf(f, "[%s] %s: %s\n", timestamp(), name, msg)
}

func timestamp() string {
	return time.Now().Format(time.RFC3339)
}
```

Then rewrite `Run` to accept the two new flags and dispatch to `runFixes`,
keeping today's no-fix behavior 100% unchanged when `fix` is false:

```go
// internal/doctor/doctor.go — Run's new signature
func Run(profile, region, version string, fix, skipConfirm bool) error {
	results := []result{
		checkAWSCLI(),
		checkSessionManagerPlugin(),
		checkCredentials(profile, region),
		checkRegion(region),
		checkProfile(profile),
		checkConfigFile(),
		checkVersion(version),
	}

	if fix {
		recheck := func(name string) result {
			switch name {
			case "AWS CLI":
				return checkAWSCLI()
			case "Session Manager plugin":
				return checkSessionManagerPlugin()
			default:
				return result{Name: name, Status: statusFail, Detail: "unknown check"}
			}
		}
		results, _ = runFixes(results, skipConfirm, recheck)
	}

	// ... existing print loop and hasFailure logic below, unchanged ...
}
```

Everything from `fmt.Println()` at doctor.go:45 onward stays exactly as it
is today — the fix pass runs *before* the report is printed, so the final
report reflects post-fix state (a fixed check now prints ✓, an unfixed or
failed-to-fix one still prints ✗).

**Verify**: `go build ./internal/doctor/...` → exit 0 (this will also
require updating `Run`'s only caller in `main.go`, done in Step 6 — if the
build fails only because of that caller mismatch, that's expected until
Step 6; confirm by temporarily checking `go build ./internal/doctor/...`
in isolation, which does not touch `main.go`).

### Step 5: Add `internal/doctor/fix_test.go`

Test only the pure, side-effect-free pieces — matching this repo's existing
ceiling for test coverage in `internal/doctor` (`doctor_test.go` today tests
`extractJSON`, `checkRegion`, `checkProfile`: pure functions and functions
whose only "impurity" is reading env vars/temp dirs, never functions that
shell out to real installers or touch the network). Do **not** write a test
that actually invokes `installAWSCLI`/`installSSMPlugin`/msiexec/apt/etc.

```go
package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestFixLogPath(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	got := fixLogPath()
	want := filepath.Join(tmpDir, ".act-doctor-fix.log")
	if got != want {
		t.Errorf("fixLogPath() = %q, want %q", got, want)
	}
}

func TestRunFixesSkipsPassingAndNoFixChecks(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	applyCalled := false
	results := []result{
		{Name: "Region", Status: statusWarn, Detail: "not configured"},
		{Name: "AWS CLI", Status: statusFail, Detail: "not found", Fix: &fixAction{
			Describe: func() string { return "test fix" },
			Apply: func(w io.Writer) error {
				applyCalled = true
				fmt.Fprintln(w, "applying")
				return nil
			},
		}},
	}

	// Simulate declining the prompt by feeding "n\n" on stdin.
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	w.WriteString("n\n")
	w.Close()

	_, _ = runFixes(results, false, nil)

	if applyCalled {
		t.Error("Apply should not run when the user declines the prompt")
	}
}

func TestRunFixesSkipConfirmRunsApply(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	applyCalled := false
	results := []result{
		{Name: "AWS CLI", Status: statusFail, Detail: "not found", Fix: &fixAction{
			Describe: func() string { return "test fix" },
			Apply: func(w io.Writer) error {
				applyCalled = true
				return nil
			},
		}},
	}

	_, allOK := runFixes(results, true, func(name string) result {
		return result{Name: name, Status: statusPass, Detail: "fixed"}
	})

	if !applyCalled {
		t.Error("Apply should run when skipConfirm is true")
	}
	if !allOK {
		t.Error("expected allOK true when Apply succeeds")
	}

	logPath := fixLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected fix log to exist at %s: %v", logPath, err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty fix log")
	}
}
```

Adjust imports (`io`, `fmt` needed in the test file for the closures above)
and reuse the existing `overrideHomeForDoctorTest` helper already defined in
`doctor_test.go:62-67` (same package, no need to redefine it).

**Verify**: `go test ./internal/doctor/... -v` → all tests pass, including
the three new ones and all pre-existing ones (`TestExtractJSON`,
`TestCheckRegion`, `TestCheckProfile`, plus anything plan 015 added — run
`go test ./internal/doctor/... -v` and confirm zero failures, not just the
new subset).

### Step 6: Wire `--fix` and `--skip-confirm` into `main.go`

Update the `doctor` case to parse the two new flags before calling
`doctor.Run`:

```go
// main.go — replace the doctor case
case "doctor":
	subArgs := args[1:]
	if hasHelp(subArgs) {
		printDoctorHelp()
		os.Exit(0)
	}
	fix := false
	skipConfirm := false
	var doctorArgs []string
	for _, a := range subArgs {
		switch a {
		case "--fix":
			fix = true
		case "--skip-confirm":
			skipConfirm = true
		default:
			doctorArgs = append(doctorArgs, a)
		}
	}
	if skipConfirm && !fix {
		fmt.Fprintln(os.Stderr, "Error: --skip-confirm has no effect without --fix")
		os.Exit(1)
	}
	doctor.Run(profile, region, version, fix, skipConfirm)
	os.Exit(0)
```

(`doctorArgs` is parsed but intentionally unused beyond flag-stripping —
`doctor.Run` doesn't take positional args today and this plan doesn't add
any; keep the loop simple rather than wiring up a `flag.FlagSet` for two
boolean switches, consistent with how `hasHelp`/`parseGlobalFlags` already
hand-roll flag scanning in this file.)

Update `printDoctorHelp` (main.go:469-481):

```go
func printDoctorHelp() {
	fmt.Fprintf(os.Stderr, `act doctor - Check system dependencies and configuration

Usage: act [global flags] doctor [--fix] [--skip-confirm]

Checks that all required tools are installed, credentials are valid,
and configuration is correct.

Flags:
  --fix            Attempt to automatically fix failing checks (currently:
                    installing a missing AWS CLI or Session Manager plugin).
                    Prompts for confirmation before each install unless
                    --skip-confirm is also given. Writes a log of every fix
                    attempt to ~/.act-doctor-fix.log.
  --skip-confirm   With --fix, run every available fix without prompting.
                    Has no effect without --fix.

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use

Examples:
  act doctor
  act doctor --fix
  act doctor --fix --skip-confirm
`)
}
```

**Verify**:
1. `go build -v ./...` → exit 0.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l .` → no output.
4. `./act doctor --fix help` is not a valid invocation (help must still be
   caught by the existing `hasHelp(subArgs)` check *before* the new flag
   loop runs, since `--fix`/`--skip-confirm` are stripped only from
   `subArgs`, and `hasHelp` runs on unstripped `subArgs` first) — confirm
   `act doctor help` and `act doctor --help` still print help and exit 0,
   unchanged from before this plan.
5. Manually run `act doctor` (no flags) on your own machine with AWS CLI
   already installed — confirm output is byte-for-byte identical in
   structure to before this plan (still prints ✓ AWS CLI, no fix-related
   output at all, since `fix` defaults to `false`).

### Step 7: Update `README.md`

Per `CLAUDE.md`'s rule ("After adding a new feature, command, or flag,
always update README.md..."):

- Features list (README.md, the `- **Doctor** — ...` bullet): extend to
  mention the fix capability, e.g. `- **Doctor** — Verify dependencies and
  configuration with \`act doctor\`, or auto-fix missing tools with \`act
  doctor --fix\``.
- Commands table (`| doctor | Check system dependencies and configuration |`):
  leave as-is (flags aren't listed in that table for any other command
  either — e.g. `ec2 rdp`'s many flags aren't in the Commands table, they're
  in the Examples section only. Match that convention).
- Examples section: add after the existing `# Check dependencies` /
  `act doctor` example:

  ```
  # Auto-fix missing dependencies (prompts before each install)
  act doctor --fix

  # Auto-fix without prompting (e.g. in a provisioning script)
  act doctor --fix --skip-confirm
  ```

**Verify**: `grep -n "doctor --fix" README.md` → at least 3 matches (the
Features bullet and the two example lines).

## Test plan

- `internal/doctor/fix_test.go` (new, Step 5): `TestFixLogPath`,
  `TestRunFixesSkipsPassingAndNoFixChecks`,
  `TestRunFixesSkipConfirmRunsApply` — pattern-match `doctor_test.go`'s
  existing `overrideHomeForDoctorTest` + `t.TempDir()` approach.
- No test exercises `installAWSCLI*`/`installSSMPlugin*` end-to-end (they
  invoke real network downloads and system installers — out of scope for
  `go test`, matching this repo's established practice of never unit-testing
  `exec.Command`-wrapping AWS-CLI calls either).
- Verification: `go test -v ./...` → all pass, including the 3 new tests,
  with zero changes to any existing test's pass/fail status.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0, no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./...` exits 0, all tests pass (existing + 3 new in
      `internal/doctor/fix_test.go`)
- [ ] `act doctor` (no flags) produces output identical in structure to
      before this plan — verified manually, not just by diffing test output
- [ ] `act doctor --help` and `act doctor help` both still print help text
      and exit 0
- [ ] `grep -n "doctor --fix" README.md` returns ≥3 matches
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 018 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at any "Current state" location doesn't match what's excerpted
  above (drift since this plan was written).
- The official AWS CLI or Session Manager plugin download URLs in Steps 2-3
  return 404 or have moved — verify with `curl -sI <url>` before wiring them
  in; if stale, use the current URLs from the official AWS docs (linked in
  Step 3) and note the correction when you report back, but do not silently
  guess at unverified URLs.
- You find yourself wanting to make `--fix` also handle `checkCredentials`,
  `checkRegion`, `checkProfile`, `checkConfigFile`, or `checkVersion` — this
  is explicitly out of scope (see "Out of scope"); stop and flag it as a
  possible follow-up plan instead of expanding this one.
- A step's verification fails twice after a reasonable fix attempt.
- You discover `internal/doctor` already has fix-related code (e.g. from a
  differently-numbered plan run in parallel) — stop and reconcile with
  `plans/README.md` rather than duplicating.

## Maintenance notes

- If a future plan adds more fixable checks (e.g. detecting an expired SSO
  session and running `aws sso login`), follow this plan's `fixAction`
  pattern: populate `Fix` only on the failing branch of the relevant
  `checkX` function, and add its name to `runFixes`'s `recheck` switch in
  `doctor.go`.
- The two installer files (`install_awscli.go`, `install_ssmplugin.go`) hit
  real AWS-owned download URLs. If AWS changes these URLs or ships a new
  major version with a different install flow, both files need updating —
  there's no dynamic discovery, they're hardcoded (matching this repo's
  existing style: `internal/updater/updater.go`'s `repoAPI` constant is
  likewise hardcoded, not discovered).
- Multi-target / fleet-wide fixing (e.g. "fix this on every instance in the
  ASG") is explicitly out of scope — this only ever fixes the local machine
  running `act doctor`.
- `~/.act-doctor-fix.log` grows unbounded (append-only, never rotated or
  truncated). If that becomes a real problem, add rotation in a follow-up
  plan — not worth the complexity here given expected usage (a handful of
  fix attempts per machine, ever).
- A reviewer should scrutinize: (1) that no fix path can execute without
  either an explicit `y`/`yes` answer or the explicit `--skip-confirm` flag —
  there must be no code path that installs anything silently by default;
  (2) that every `exec.Command` call in the new installer files uses a
  fixed argv (no string concatenation building a shell command from
  variable input) to avoid command injection, matching the fix already
  applied to `internal/aws/ssh_unix.go`/`ssh_windows.go` in plan 002.
