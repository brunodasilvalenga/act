# Plan 010: Extract shared AWS-CLI arg-building out of *_unix.go/*_windows.go pairs

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/aws/`
> If any file under `internal/aws/` changed since this plan was written,
> compare the "Current state" excerpts against the live code before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: M
- **Risk**: MED
- **Depends on**: plan 002 (fix the SSH ProxyCommand validation first — this
  plan touches the same `ssh_unix.go`/`ssh_windows.go` files and should
  preserve plan 002's validation calls when consolidating; executing this
  plan before plan 002 would mean re-applying the validation fix to a
  refactored file structure, which is more error-prone than doing it once,
  cleanly, in the original files first)
- **Category**: tech-debt
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`internal/aws/` contains 5 file pairs split by Go build tag
(`//go:build !windows` / `//go:build windows`) where the AWS-CLI
argument-building logic — which subcommand, which flags, which SSM document
name, which JSON parameters string — is copy-pasted verbatim between the two
files, differing only in how the final process is launched
(`syscall.Exec` replacing the current process on Unix, vs.
`exec.Command(...).Run()` with `Stdin`/`Stdout`/`Stderr` wired up on
Windows). The 5 pairs are: `ssh_unix.go`/`ssh_windows.go`,
`forward_unix.go`/`forward_windows.go`,
`session_unix.go`/`session_windows.go`,
`ecs_exec_unix.go`/`ecs_exec_windows.go`, and
`logs_unix.go`/`logs_windows.go`. Because each file only compiles under its
own build tag, there is no compiler check enforcing that a future change to
one file's argument-building logic (e.g. adding a new SSM parameter or
fixing a flag name) is mirrored in its sibling — the two platforms can
silently diverge. This plan extracts each pair's shared argument-building
into a small, no-build-tag helper function that both platform files call,
leaving only the actual process launch as platform-specific code.

## Current state

All 10 files in full (confirmed already read in full during audit):

**`internal/aws/session_unix.go`** (29 lines):
```go
//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartSession(instanceID, profile, region string) error {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return err
	}

	// Replace current process with aws ssm session
	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
```

**`internal/aws/session_windows.go`** (25 lines):
```go
//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartSession(instanceID, profile, region string) error {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

**`internal/aws/ecs_exec_unix.go`** (34 lines) / **`ecs_exec_windows.go`**
(31 lines) — identical `args` construction for `StartECSExec` (cluster,
task, container, `--interactive`, `--command /bin/sh`, optional
profile/region), differing only in launch mechanism.

**`internal/aws/logs_unix.go`** (31 lines) / **`logs_windows.go`** (27
lines) — identical `args` construction for `TailLogs` (`logs tail`,
log group, `--since`, optional `--follow`, optional profile/region).

**`internal/aws/forward_unix.go`** (65 lines) / **`forward_windows.go`** (60
lines) — identical `document`/`params` JSON-string construction and `args`
building for both `StartRemotePortForward` and `StartPortForward` (two
functions per file).

**`internal/aws/ssh_unix.go`** (37 lines, post-plan-002) /
**`ssh_windows.go`** (33 lines, post-plan-002) — identical `proxyCmd`
construction for `StartSSHSession`, now including plan 002's
`validateSSHProxyToken` calls if that plan has executed first (see
"Depends on" above).

Common shape across every pair: the Unix variant does
`awsBin, err := exec.LookPath("aws")` (or, for `ssh_unix.go`,
`exec.LookPath("ssh")`) then `syscall.Exec(bin, argv, env)`; the Windows
variant does `cmd := exec.Command(name, args...)` then wires
`Stdin`/`Stdout`/`Stderr` and `cmd.Run()`. This launch-mechanism difference
is genuinely platform-specific and must stay split by build tag — only the
argument-building above it is safe to share.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build (current OS) | `go build ./...` | exit 0 |
| Build (Windows cross-compile) | `GOOS=windows GOARCH=amd64 go build ./...` | exit 0 |
| Build (Linux cross-compile) | `GOOS=linux GOARCH=amd64 go build ./...` | exit 0 |
| Build (macOS cross-compile) | `GOOS=darwin GOARCH=arm64 go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/session_unix.go`, `internal/aws/session_windows.go`
- `internal/aws/ecs_exec_unix.go`, `internal/aws/ecs_exec_windows.go`
- `internal/aws/logs_unix.go`, `internal/aws/logs_windows.go`
- `internal/aws/forward_unix.go`, `internal/aws/forward_windows.go`
- `internal/aws/ssh_unix.go`, `internal/aws/ssh_windows.go`
- New shared (no build tag) files you create, one per pair (see Step 1's
  naming convention): `internal/aws/session_args.go`,
  `internal/aws/ecs_exec_args.go`, `internal/aws/logs_args.go`,
  `internal/aws/forward_args.go`, `internal/aws/ssh_args.go`
- `internal/aws/ssh_validate.go` / `internal/aws/ssh_validate_test.go` — only
  if you need to move `validateSSHProxyToken` alongside the new
  `ssh_args.go` for logical grouping; if plan 002 already created it as a
  standalone no-build-tag file, you may leave it exactly where it is and
  just call it from `ssh_args.go` — do not needlessly relocate working code.

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ec2.go`, `internal/aws/ecs.go`, `internal/aws/rds.go`,
  `internal/aws/logs.go`, `internal/aws/rdp_unix.go`,
  `internal/aws/rdp_windows.go` — these either have no unix/windows split
  (`ec2.go`, `ecs.go`, `rds.go`, `logs.go` are single files with no build
  tag, doing AWS API *listing*, not process launching) or (for
  `rdp_unix.go`/`rdp_windows.go`) are a 6th pair not included in this plan's
  scope — the RDP pair has more platform-specific logic beyond simple
  arg-building (Windows opens `mstsc`, Unix opens via `open` on macOS — see
  `rdp_unix.go:49` and `rdp_windows.go:45`) and deduplicating it cleanly is a
  larger, separate effort not bundled here.
- `main.go` — no caller-facing function signatures change; every exported
  function (`StartSession`, `StartECSExec`, `TailLogs`,
  `StartRemotePortForward`, `StartPortForward`, `StartSSHSession`) keeps its
  exact existing signature.
- Any behavior change to the actual AWS CLI arguments produced — this is a
  pure refactor; the argv slice built for a given set of inputs must be
  byte-identical before and after.

## Git workflow

- Branch: `advisor/010-dedupe-unix-windows-arg-building`
- Commit per file pair (5 commits) or one commit for the whole plan — either
  is acceptable given this is a single logical refactor; if using multiple
  commits, message style example: `refactor: extract shared instance-picker helper in main.go` (matching plan 007's expected commit, for precedent) — e.g.
  `refactor: extract shared arg-building from session_unix/windows.go`
  repeated per pair, or one commit:
  `refactor: extract shared AWS-CLI arg-building from unix/windows pairs`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

Repeat this same pattern for each of the 5 pairs. Full detail is given for
the first (simplest) pair; apply the identical technique to the remaining
4, adapting names.

### Step 1: `session_unix.go` / `session_windows.go` → extract `sessionArgs`

Create `internal/aws/session_args.go` (no build tag):

```go
package aws

// sessionArgs builds the aws CLI argument list for `aws ssm start-session`.
func sessionArgs(instanceID, profile, region string) []string {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}
```

Update `internal/aws/session_unix.go` to:

```go
//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartSession(instanceID, profile, region string) error {
	args := sessionArgs(instanceID, profile, region)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return err
	}

	// Replace current process with aws ssm session
	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
```

Update `internal/aws/session_windows.go` to:

```go
//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartSession(instanceID, profile, region string) error {
	args := sessionArgs(instanceID, profile, region)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

**Verify**: `go build ./...` → exit 0 on your current platform.
**Verify**: `GOOS=windows GOARCH=amd64 go build ./...` → exit 0 (confirms
the Windows variant compiles even though you can't run it here).
**Verify**: `GOOS=linux GOARCH=amd64 go build ./...` and
`GOOS=darwin GOARCH=arm64 go build ./...` → exit 0.

### Step 2: `ecs_exec_unix.go` / `ecs_exec_windows.go` → extract `ecsExecArgs`

Create `internal/aws/ecs_exec_args.go` (no build tag):

```go
package aws

// ecsExecArgs builds the aws CLI argument list for `aws ecs execute-command`.
func ecsExecArgs(cluster, taskID, containerName, profile, region string) []string {
	args := []string{"ecs", "execute-command",
		"--cluster", cluster,
		"--task", taskID,
		"--container", containerName,
		"--interactive",
		"--command", "/bin/sh",
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}
```

Update `internal/aws/ecs_exec_unix.go`'s `StartECSExec` body to
`args := ecsExecArgs(cluster, taskID, containerName, profile, region)`
followed by the existing `exec.LookPath("aws")` / `syscall.Exec` lines
unchanged. Update `internal/aws/ecs_exec_windows.go`'s `StartECSExec`
identically, followed by its existing `exec.Command`/`cmd.Run()` lines
unchanged.

**Verify**: same 4-platform build check as Step 1.

### Step 3: `logs_unix.go` / `logs_windows.go` → extract `tailLogsArgs`

Create `internal/aws/logs_args.go` (no build tag):

```go
package aws

// tailLogsArgs builds the aws CLI argument list for `aws logs tail`.
func tailLogsArgs(logGroup, profile, region, since string, follow bool) []string {
	args := []string{"logs", "tail", logGroup, "--since", since}
	if follow {
		args = append(args, "--follow")
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}
	return args
}
```

Update both `logs_unix.go` and `logs_windows.go`'s `TailLogs` bodies to call
`tailLogsArgs(logGroup, profile, region, since, follow)`, keeping each
file's launch mechanism unchanged.

**Verify**: same 4-platform build check as Step 1.

### Step 4: `forward_unix.go` / `forward_windows.go` → extract two helpers

This pair has two functions (`StartRemotePortForward`,
`StartPortForward`). Create `internal/aws/forward_args.go` (no build tag):

```go
package aws

import "fmt"

// remotePortForwardArgs builds the aws CLI argument list for
// AWS-StartPortForwardingSessionToRemoteHost.
func remotePortForwardArgs(instanceID, profile, region string, localPort, remotePort int, remoteHost string) []string {
	document := "AWS-StartPortForwardingSessionToRemoteHost"
	params := fmt.Sprintf(`{"host":["%s"],"portNumber":["%d"],"localPortNumber":["%d"]}`, remoteHost, remotePort, localPort)

	args := []string{"ssm", "start-session",
		"--document-name", document,
		"--parameters", params,
	}

	if instanceID != "" {
		args = append(args, "--target", instanceID)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}

// portForwardArgs builds the aws CLI argument list for
// AWS-StartPortForwardingSession.
func portForwardArgs(instanceID, profile, region string, localPort, remotePort int) []string {
	document := "AWS-StartPortForwardingSession"
	params := fmt.Sprintf(`{"portNumber":["%d"],"localPortNumber":["%d"]}`, remotePort, localPort)

	args := []string{"ssm", "start-session",
		"--target", instanceID,
		"--document-name", document,
		"--parameters", params,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}
```

Update both `forward_unix.go` and `forward_windows.go`: each function
(`StartRemotePortForward`, `StartPortForward`) now starts with
`args := remotePortForwardArgs(...)` or `args := portForwardArgs(...)`
respectively, followed by the existing per-platform launch code unchanged.

**Verify**: same 4-platform build check as Step 1.

### Step 5: `ssh_unix.go` / `ssh_windows.go` → extract `sshProxyArgs`

If plan 002 has already run, `ssh_unix.go`/`ssh_windows.go` will each
contain a `validateSSHProxyToken` call at the top of `StartSSHSession`
before the `proxyCmd :=` line, and a standalone `ssh_validate.go` file
exists with `validateSSHProxyToken`'s definition. Preserve those validation
calls exactly — move the argument-building that comes *after* validation
into the new shared helper, not the validation itself (validation should
stay a guard at the top of `StartSSHSession` in each platform file, or be
folded into the new helper — either is acceptable as long as it still runs
before the ProxyCommand string is used).

Create `internal/aws/ssh_args.go` (no build tag):

```go
package aws

import "fmt"

// sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
// ProxyCommand. Callers must validate profile/region before calling this
// (see validateSSHProxyToken) since these values are spliced into a
// shell-executed ProxyCommand string.
func sshProxyArgs(instanceID, profile, region, user string) []string {
	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	if profile != "" {
		proxyCmd += " --profile " + profile
	}
	if region != "" {
		proxyCmd += " --region " + region
	}

	return []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
		fmt.Sprintf("%s@%s", user, instanceID),
	}
}
```

Note this returns args *without* the leading `"ssh"` binary name — the Unix
variant's `syscall.Exec` needs `argv[0]` to be `"ssh"` prepended, while the
Windows variant's `exec.Command("ssh", args...)` takes the binary name
separately. Update `ssh_unix.go`'s `StartSSHSession`:

```go
func StartSSHSession(instanceID, profile, region, user string) error {
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}

	args := append([]string{"ssh"}, sshProxyArgs(instanceID, profile, region, user)...)

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(sshBin, args, env)
}
```

Update `ssh_windows.go`'s `StartSSHSession`:

```go
func StartSSHSession(instanceID, profile, region, user string) error {
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}

	args := sshProxyArgs(instanceID, profile, region, user)

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
```

If plan 002 has NOT run yet, omit the two `validateSSHProxyToken` calls in
both snippets above — do not invent validation logic in this plan; that's
plan 002's job. Note this discrepancy in your final report either way, so
the reviewer knows which order the plans executed in.

**Verify**: same 4-platform build check as Step 1. Also run
`go test ./internal/aws/... -run TestValidateSSHProxyToken -v` if plan 002's
test exists — it must still pass unchanged.

### Step 6: Full verification pass

**Verify**: `go build ./...` → exit 0.
**Verify**: `GOOS=windows GOARCH=amd64 go build ./...`,
`GOOS=linux GOARCH=amd64 go build ./...`,
`GOOS=darwin GOARCH=arm64 go build ./...` → all exit 0.
**Verify**: `go test ./...` → exit 0, all pass (same test count as baseline
— this plan adds no new test cases of its own, since it's a pure extraction
of already-covered-or-uncovered logic; if plan 002's SSH validation tests
exist, they must still pass).

## Test plan

This plan does not add new tests — it's a pure extraction refactor with no
behavior change, and the argument-building logic being extracted has no
existing test coverage of its own to preserve (confirmed: `internal/aws`
package sits at 1.3% coverage per `go test ./... -cover`, with only
`ec2_test.go`'s `TestDisplayName` exercising anything). The verification
gate is: (1) all 4 platform builds succeed (Step 6), (2) the full test suite
passes unchanged, and (3) a manual read-through per step confirming the
extracted helper produces byte-identical argv slices to the original inline
code for the same inputs. If plan `015` (add test coverage to
`internal/aws`) executes after this plan, the newly-extracted pure functions
(`sessionArgs`, `ecsExecArgs`, `tailLogsArgs`, `remotePortForwardArgs`,
`portForwardArgs`, `sshProxyArgs`) become much easier to unit-test than the
original inline code was, since they're now free functions with no
`exec`/`syscall` side effects — that's a natural follow-up, not required by
this plan.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] 5 new no-build-tag files exist: `session_args.go`, `ecs_exec_args.go`,
      `logs_args.go`, `forward_args.go`, `ssh_args.go`
- [ ] Each of the 10 original `*_unix.go`/`*_windows.go` files' exported
      function bodies call the corresponding shared helper for
      argument-building, with only the process-launch code remaining
      platform-specific
- [ ] `go build ./...` exits 0
- [ ] `GOOS=windows GOARCH=amd64 go build ./...` exits 0
- [ ] `GOOS=linux GOARCH=amd64 go build ./...` exits 0
- [ ] `GOOS=darwin GOARCH=arm64 go build ./...` exits 0
- [ ] `go test ./...` exits 0, same or greater pass count than baseline (16
      tests, 6 packages, plus plan 002's tests if it ran first)
- [ ] `git status` shows only the 10 original files plus the 5 new
      `*_args.go` files (and possibly `ssh_validate.go` if relocated —
      avoid relocating it unless necessary) modified/added
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any of the 10 files don't match the excerpts above (drift since this plan
  was written) — re-read the specific pair fully before touching it.
- A step's verification fails twice after a reasonable fix attempt.
- Cross-compiling for a platform you're not currently on reveals a build
  tag or import issue you can't resolve without running code on that actual
  platform — report the specific compiler error rather than guessing at a
  fix blind.
- You find that plan 002 has run but its validation calls don't match what
  this plan's Step 5 expects (e.g. different function name, different call
  site) — STOP and report the actual current state of
  `ssh_unix.go`/`ssh_windows.go` rather than forcing this plan's Step 5 to
  fit.

## Maintenance notes

- Any future change to AWS CLI argument construction for these 6 commands
  (session, ecs-exec, logs-tail, port-forward ×2, ssh-proxy) should now be
  made once in the shared `*_args.go` file, and both platform files will
  pick it up automatically — this is the whole point of this refactor.
- A reviewer should scrutinize: that no platform-specific behavior
  accidentally moved into a shared file (e.g. if a future edit needs
  Windows-specific quoting for the SSH ProxyCommand argument, that quoting
  logic must NOT go in `ssh_args.go` unless it's genuinely identical on both
  platforms).
- This plan deliberately excludes the `rdp_unix.go`/`rdp_windows.go` pair
  (see "Out of scope") — a future plan could extend this same pattern there,
  but it needs to account for the extra platform-specific "open RDP client"
  logic (`mstsc` vs. `open`) that has no shared equivalent.
