# Plan 002: Quote profile/region before splicing into OpenSSH ProxyCommand

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/aws/ssh_unix.go internal/aws/ssh_windows.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`act ec2 ssh` builds an OpenSSH `ProxyCommand` string by directly
concatenating the resolved `profile` and `region` values into a shell command
string, with no escaping or quoting. OpenSSH's `ProxyCommand` value is
executed via the user's shell (`$SHELL -c "..."`) when establishing the
connection. `profile` and `region` are not literal ssh-side tokens like
`%h`/`%p` — they are ordinary Go strings that flow in from `--profile`/
`--region` CLI flags, a `~/.act.json` `environments` entry (see
`internal/config/config.go` `ResolveProfile`/`ResolveRegion`), or the
`AWS_PROFILE`/`AWS_REGION` environment variables. Any of these containing
shell metacharacters (e.g. an env var inherited from a compromised parent
process, a synced/shared config file, or an environment name typo that
collides with unexpected input) would be interpreted by the shell OpenSSH
spawns, allowing arbitrary command execution in the context of the user
running `act ec2 ssh`.

The fix is narrow: validate/reject profile and region values that contain
shell metacharacters before they're spliced into the `ProxyCommand` string,
rather than trying to escape arbitrary shell syntax (escaping is error-prone;
rejecting invalid characters is simpler and sufficient since AWS profile
names and region names have a well-known safe character set).

## Current state

- `internal/aws/ssh_unix.go` (full file, 37 lines):
  ```go
  //go:build !windows

  package aws

  import (
  	"fmt"
  	"os"
  	"os/exec"
  	"syscall"
  )

  func StartSSHSession(instanceID, profile, region, user string) error {
  	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
  	if profile != "" {
  		proxyCmd += " --profile " + profile
  	}
  	if region != "" {
  		proxyCmd += " --region " + region
  	}

  	args := []string{
  		"ssh",
  		"-o", "StrictHostKeyChecking=no",
  		"-o", "UserKnownHostsFile=/dev/null",
  		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
  		fmt.Sprintf("%s@%s", user, instanceID),
  	}

  	sshBin, err := exec.LookPath("ssh")
  	if err != nil {
  		return fmt.Errorf("ssh not found in PATH: %w", err)
  	}

  	env := os.Environ()
  	return syscall.Exec(sshBin, args, env)
  }
  ```
- `internal/aws/ssh_windows.go` (full file, 33 lines) — identical
  `proxyCmd` construction (lines 11-18), differing only in process launch
  (`exec.Command(...).Run()` instead of `syscall.Exec`):
  ```go
  //go:build windows

  package aws

  import (
  	"fmt"
  	"os"
  	"os/exec"
  )

  func StartSSHSession(instanceID, profile, region, user string) error {
  	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
  	if profile != "" {
  		proxyCmd += " --profile " + profile
  	}
  	if region != "" {
  		proxyCmd += " --region " + region
  	}

  	args := []string{
  		"-o", "StrictHostKeyChecking=no",
  		"-o", "UserKnownHostsFile=/dev/null",
  		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
  		fmt.Sprintf("%s@%s", user, instanceID),
  	}

  	cmd := exec.Command("ssh", args...)
  	cmd.Stdin = os.Stdin
  	cmd.Stdout = os.Stdout
  	cmd.Stderr = os.Stderr
  	return cmd.Run()
  }
  ```
- Repo convention: both files export the same function signature
  (`StartSSHSession(instanceID, profile, region, user string) error`) via Go
  build tags (`//go:build !windows` / `//go:build windows`) — this pattern is
  used identically across 5 file pairs in `internal/aws/` (see plan `010` for
  the broader duplication issue; do not attempt to deduplicate the two files
  in this plan — stay narrowly scoped to the validation fix, applied
  identically to both).
- Caller: `main.go:779-818` (`runSSH`) resolves `profile`/`region` via
  `config.ResolveProfile`/`config.ResolveRegion` (see
  `internal/config/config.go:103-138`) before calling `aws.StartSSHSession`.
  No validation currently happens anywhere in that chain.
- AWS CLI profile names and region names follow a known-safe character set:
  profile names are conventionally alphanumeric plus `-`, `_`, `.`; region
  names look like `us-west-2`, `ap-southeast-2` (lowercase letters, digits,
  hyphens). Neither legitimately contains spaces, quotes, `;`, `|`, `&`,
  backticks, or `$`.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (this package) | `go test ./internal/aws/...` | exit 0, all pass, including new tests |
| Full test suite | `go test ./...` | exit 0, all pass |
| Vet | `go vet ./...` | exit 0, no output |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ssh_unix.go`
- `internal/aws/ssh_windows.go`
- `internal/aws/ssh_test.go` (create — new file, no `_unix`/`_windows` suffix
  needed since the validation logic you're adding is platform-independent;
  see Step 2 for where to put it)

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/rdp_unix.go` / `rdp_windows.go`, `forward_unix.go` /
  `forward_windows.go`, `session_unix.go` / `session_windows.go`,
  `ecs_exec_unix.go` / `ecs_exec_windows.go`, `logs_unix.go` /
  `logs_windows.go` — these pass `profile`/`region` as separate `exec.Command`
  argv elements (not spliced into a shell-executed string), so they are not
  vulnerable to this class of issue and are out of scope for this plan.
- `main.go` — the caller doesn't need changes; validation belongs at the
  point where the string is constructed (`ssh_unix.go`/`ssh_windows.go`), not
  at the call site, so that any future caller of `StartSSHSession` is
  protected automatically.
- Do NOT attempt to merge `ssh_unix.go` and `ssh_windows.go` into one file —
  that's a separate, larger refactor tracked in plan `010`. Keep this plan's
  diff minimal: add a shared validation helper and call it from both files.

## Git workflow

- Branch: `advisor/002-fix-ssh-proxycommand-injection`
- Single commit; message style example from `git log`: `fix: bump CI go-version to 1.26 to match go.mod` (see commit `ffbc9cf`)
  — follow the same convention, e.g.
  `fix: validate profile/region before use in ssh ProxyCommand`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add a shared validation function

Add a new function to `internal/aws/ssh_unix.go` (it will be compiled only
on non-Windows due to the file's `//go:build !windows` tag — so you must add
the *same* function, or an equivalent, reachable from both platform files;
the simplest approach is a small new file with no build tag):

Create `internal/aws/ssh_validate.go` (no build tag — compiles on all
platforms):

```go
package aws

import (
	"fmt"
	"regexp"
)

var safeProfileRegionPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// validateSSHProxyToken rejects values that would be unsafe to splice
// unescaped into an OpenSSH ProxyCommand string, which is executed by the
// user's shell. AWS profile and region names never legitimately contain
// characters outside this set.
func validateSSHProxyToken(value, kind string) error {
	if value == "" {
		return nil
	}
	if !safeProfileRegionPattern.MatchString(value) {
		return fmt.Errorf("%s %q contains characters not allowed in an SSH ProxyCommand (allowed: letters, digits, '.', '_', '-')", kind, value)
	}
	return nil
}
```

**Verify**: `go build ./internal/aws/...` → exit 0.

### Step 2: Call the validator from `StartSSHSession` in both platform files

In `internal/aws/ssh_unix.go`, at the top of `StartSSHSession` (before the
`proxyCmd :=` line), add:

```go
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}
```

Make the identical change in `internal/aws/ssh_windows.go` at the same
position in its `StartSSHSession`.

**Verify**: `go build ./...` → exit 0 (this builds only the current OS's
variant; see Step 4 for cross-compile verification of the other platform).

### Step 3: Write unit tests for the validator

Create `internal/aws/ssh_validate_test.go`:

```go
package aws

import "testing"

func TestValidateSSHProxyToken(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty is allowed", "", false},
		{"plain profile", "production", false},
		{"profile with dashes and dots", "my-profile_v1.2", false},
		{"plain region", "us-west-2", false},
		{"semicolon injection attempt", "prod; rm -rf ~", true},
		{"backtick injection attempt", "prod`whoami`", true},
		{"dollar injection attempt", "prod$(whoami)", true},
		{"space", "prod eu", true},
		{"pipe", "prod|cat", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSSHProxyToken(tt.value, "profile")
			if (err != nil) != tt.wantErr {
				t.Errorf("validateSSHProxyToken(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
```

This follows the table-driven test pattern already used in
`internal/aws/ec2_test.go:6-21` and `internal/config/config_test.go`.

**Verify**: `go test ./internal/aws/... -run TestValidateSSHProxyToken -v` →
all subtests pass.

### Step 4: Cross-compile the other platform to confirm it still builds

If you're on macOS/Linux, cross-compile the Windows variant (and vice versa)
to confirm both platform files build correctly with the new validation call:

```
GOOS=windows GOARCH=amd64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=darwin GOARCH=arm64 go build ./...
```

**Verify**: all three commands exit 0.

### Step 5: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass, including the new
`TestValidateSSHProxyToken` subtests from Step 3.

## Test plan

- New test file: `internal/aws/ssh_validate_test.go`, table-driven, modeled
  after `internal/aws/ec2_test.go:6-21` (table of cases with a `name`/`want`
  shape) and `internal/config/config_test.go` (multiple assertions per
  scenario).
- Cases to cover (see Step 3's table): empty string (allowed — matches
  existing `if profile != ""` guard behavior), a normal profile name, a
  normal profile name with dots/underscores/dashes, a normal region name,
  and several injection-shaped strings (semicolon, backtick, `$()`, space,
  pipe) — all of which must be rejected.
- Verification: `go test ./internal/aws/... -v` → all pass, including the
  new tests.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/aws/ssh_validate.go` exists and exports/uses
      `validateSSHProxyToken`
- [ ] `internal/aws/ssh_unix.go` and `internal/aws/ssh_windows.go` both call
      `validateSSHProxyToken` on `profile` and `region` before constructing
      `proxyCmd`
- [ ] `go build ./...` exits 0 on the current platform
- [ ] `GOOS=windows go build ./...` and `GOOS=linux go build ./...` and
      `GOOS=darwin go build ./...` all exit 0
- [ ] `go test ./...` exits 0, all pass, including new
      `TestValidateSSHProxyToken` subtests
- [ ] A profile/region value like `prod` still works (no regression to the
      happy path) — confirmed by the "plain profile"/"plain region" test
      cases passing
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ssh_unix.go` or `internal/aws/ssh_windows.go`
  doesn't match the excerpts above (the codebase has drifted since this plan
  was written) — re-read both files fully before making any change.
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching `main.go` or any file outside the
  in-scope list.
- You discover that valid, real-world AWS profile or region names exist that
  would be rejected by the `^[A-Za-z0-9._-]+$` pattern (e.g. AWS SSO session
  profiles with unusual naming) — if so, STOP and report the specific example
  rather than loosening the pattern unilaterally, since loosening it
  incorrectly could reopen the vulnerability.

## Maintenance notes

- If a future command adds a new OpenSSH `ProxyCommand`-based flow (unlikely
  given the current architecture, but possible), it must reuse
  `validateSSHProxyToken` rather than reintroducing raw string concatenation.
- A reviewer should scrutinize: that the regex is anchored (`^...$`, not
  partial match) and that both platform files call the validator identically
  — a mismatch between the two would leave one platform vulnerable.
- This plan does not address plan `010`'s broader `*_unix.go`/`*_windows.go`
  duplication; if plan `010` executes after this one, its executor must
  preserve the validation calls added here when consolidating the shared
  logic.
