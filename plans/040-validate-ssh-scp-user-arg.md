# Plan 040: Validate `--user` and guard positional argv against leading-dash reinterpretation in `ec2 ssh`/`ec2 cp`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/ssh_validate.go internal/aws/ssh_args.go internal/aws/ssh_unix.go internal/aws/ssh_windows.go internal/aws/scp.go`
> If any of those files changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding —
> in particular check whether `plans/042-dedupe-scp-ssh-proxy-flags.md` has
> already landed and restructured `sshProxyArgs`/`CopyFile` (see the
> "Interaction with plan 042" note under Scope). On any mismatch beyond that
> known possibility, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (soft-recommended ordering: let `plans/042-dedupe-scp-ssh-proxy-flags.md` land first if it's available — see "Interaction with plan 042" under Scope — but this plan is written to succeed either way)
- **Category**: security
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

`internal/aws/ssh_unix.go`, `internal/aws/ssh_windows.go`, and
`internal/aws/scp.go` already validate `profile` and `region` (via
`validateSSHProxyToken`, added by plan 002 to fix an OpenSSH `ProxyCommand`
injection bug) before splicing them into the argv passed to the real
`ssh`/`scp` binaries. The `user` parameter — populated from the `--user` CLI
flag on `act ec2 ssh` and `act ec2 cp` — flows into the exact same
unsanitized-argv-splicing pattern (`internal/aws/ssh_args.go`'s
`sshProxyArgs` builds `fmt.Sprintf("%s@%s", user, instanceID)`, and
`internal/aws/scp.go`'s `scpEndpoints` builds `fmt.Sprintf("%s@%s:%s", user,
instanceID, remotePath)`) but is never validated at all.

Because these values become literal argv elements (not shell text), the
risk is not shell-string injection — it's that OpenSSH's own getopt-style
argument parser can reinterpret a token starting with `-` as another
option, even when it is not the literal first argv element, and even when
it is glued to other text (e.g. `-Fmyconfig@i-0123` is parsed by `ssh` as
`-F myconfig@i-0123`, i.e. "use this alternate SSH config file"). A crafted
`--user` value, or an EC2 instance ID / scp source-dest path that happens to
start with `-`, could therefore get reinterpreted as an SSH/SCP option
positioned *after* the `-o ProxyCommand=...` option this tool already sets —
potentially letting it override that `ProxyCommand` (e.g. via an alternate
`-F` config file containing its own `ProxyCommand`) and run an
attacker-or-mistake-chosen command instead of the intended SSM tunnel.
`validateSSHProxyToken`'s existing character allow-list (letters, digits,
`.`, `_`, `-`) blocks the most direct form of this (`-o key=value`, which
needs `=`, and any path-like value, which needs `/`), but it does **not**
reject a bare leading-dash token like `-Fmyconfig` (no `=`, no `/`) — so
character-allow-listing `user` alone would give a false sense of complete
protection. The root cause is that OpenSSH's argv parser, not string
content, decides what counts as an option; the reliable fix for that is a
literal `--` "end of options" argv separator before the final positional
argument, which this repo's `ssh`/`scp` invocations do not currently have
anywhere.

This plan closes both gaps: (1) apply the existing, already-proven
`validateSSHProxyToken` check to `user` at the same 3 call sites that
already check `profile`/`region`, for consistency and defense-in-depth
against embedded `=`/space/shell-metacharacters; and (2) insert a `--` argv
separator immediately before the final positional argument(s) in
`sshProxyArgs` (`ssh_args.go`) and in `CopyFile`'s scp argument-building
(`scp.go`), which is the mechanism that actually prevents OpenSSH from ever
reinterpreting `user`, `instanceID` (from `--target`), or scp's `source`/
`dest` path arguments as options — those last three legitimately need a
much wider character set than an allow-list can safely support (instance
IDs are fixed by AWS, but paths need `/`, spaces, etc.), so `--` is the only
approach that protects them without breaking legitimate input.

## Current state

- `internal/aws/ssh_validate.go` (unchanged by this plan) — the existing
  validator, reused as-is:

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

  Note the doc comment says "AWS profile and region names never legitimately
  contain characters outside this set" — POSIX usernames are a superset
  concern (they also never legitimately contain `=`, spaces, or shell
  metacharacters, and in practice are restricted to
  `[a-zA-Z_][a-zA-Z0-9_-]*` by most Linux `useradd` implementations), so
  reusing this function unchanged for `user` is valid and does not need a
  wider allowed-character set. **Do not edit this file** — the fix for the
  leading-dash gap this function does not cover is the `--` separator added
  in Steps 2–3 below, not a change to this regex.

- `internal/aws/ssh_args.go` (current, full file, 27 lines):

  ```go
  package aws

  import "fmt"

  // ssmProxyCommand builds the "aws ssm start-session ..." ProxyCommand
  // string shared by both the ssh and scp code paths.
  func ssmProxyCommand(profile, region string) string {
  	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
  	if profile != "" {
  		proxyCmd += " --profile " + profile
  	}
  	if region != "" {
  		proxyCmd += " --region " + region
  	}
  	return proxyCmd
  }

  // sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
  // ProxyCommand.
  func sshProxyArgs(instanceID, profile, region, user string) []string {
  	return []string{
  		"-o", "StrictHostKeyChecking=no",
  		"-o", "UserKnownHostsFile=/dev/null",
  		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
  		fmt.Sprintf("%s@%s", user, instanceID),
  	}
  }
  ```

- `internal/aws/ssh_unix.go` (current, full file, 29 lines, build-tagged
  `!windows`):

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

- `internal/aws/ssh_windows.go` (current, full file, 25 lines, build-tagged
  `windows`) — a near-duplicate of the unix version:

  ```go
  //go:build windows

  package aws

  import (
  	"os"
  	"os/exec"
  )

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

- `internal/aws/scp.go` (current, full file, 59 lines):

  ```go
  package aws

  import (
  	"fmt"
  	"os"
  	"os/exec"
  )

  // scpEndpoints computes the local and remote-spec scp arguments for a
  // given copy direction.
  func scpEndpoints(instanceID, user, source, dest string, download bool) (localArg, remoteSpecArg string, remoteFirst bool) {
  	remotePath, localPath := dest, source
  	if download {
  		remotePath, localPath = source, dest
  	}
  	remoteSpec := fmt.Sprintf("%s@%s:%s", user, instanceID, remotePath)
  	return localPath, remoteSpec, download
  }

  // CopyFile runs scp over an SSM-proxied SSH tunnel to copy a file (or,
  // with recursive, a directory) between the local machine and an EC2
  // instance. By default source is a local path and dest is the remote
  // path (upload); with download=true the direction is reversed (dest is
  // the local path, source is the remote path).
  func CopyFile(instanceID, profile, region, user, source, dest string, download, recursive bool) error {
  	if err := validateSSHProxyToken(profile, "profile"); err != nil {
  		return err
  	}
  	if err := validateSSHProxyToken(region, "region"); err != nil {
  		return err
  	}

  	args := []string{
  		"-o", "StrictHostKeyChecking=no",
  		"-o", "UserKnownHostsFile=/dev/null",
  		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
  	}
  	if recursive {
  		args = append(args, "-r")
  	}

  	localPath, remoteSpec, remoteFirst := scpEndpoints(instanceID, user, source, dest, download)
  	if remoteFirst {
  		args = append(args, remoteSpec, localPath)
  	} else {
  		args = append(args, localPath, remoteSpec)
  	}

  	scpBin, err := exec.LookPath("scp")
  	if err != nil {
  		return fmt.Errorf("scp not found in PATH (install an OpenSSH client): %w", err)
  	}

  	cmd := exec.Command(scpBin, args...)
  	cmd.Stdin = os.Stdin
  	cmd.Stdout = os.Stdout
  	cmd.Stderr = os.Stderr
  	return cmd.Run()
  }
  ```

- Callers, for context only (do not modify — the `user` value they pass
  through already comes straight from the `--user` CLI flag, unvalidated,
  or from an interactive picker limited to `ec2-user`/`ubuntu`/`root`/
  `ssm-user`, but the flag path bypasses that picker entirely):
  - `cmd_ec2.go:33` (`runSSH`): `user := fs.String("user", "", "SSH user (default: prompt)")`, passed to `aws.StartSSHSession(instanceID, profile, region, sshUser)` at `cmd_ec2.go:75`.
  - `cmd_ec2.go:129` (`runCP`): same flag pattern, passed to `aws.CopyFile(instanceID, profile, region, sshUser, source, dest, *download, *recursive)` at `cmd_ec2.go:159`.
  - `cmd_ec2.go:32`/`cmd_ec2.go:128`: `--target` (`instanceID`) is *also* a raw, unvalidated CLI flag value when supplied directly (it only goes through the AWS-API-backed interactive picker when `--target` is omitted) — this is why Step 3 below also protects `instanceID`, not just `user`.
  - `internal/aws/ssh_key_push.go:47`'s `PushSSHKeyViaSSM` also takes an
    `osUser` parameter, but it is a **different, already-safe** mechanism:
    it single-quote-escapes `osUser` via `shellSingleQuote` (see
    `ssh_key_push.go:63`) before embedding it in an `AWS-RunShellScript` SSM
    document body — not an OpenSSH argv element. It is out of scope for
    this plan (not part of the finding, and not vulnerable to the same
    issue); do not touch it.

- Local verification that `--` is accepted by both `ssh` and `scp` as an
  "end of options" separator and does not change behavior for normal
  arguments (checked against `OpenSSH_9.9p2` locally; OpenSSH has supported
  `--` in both binaries' argv parsing for a very long time, well before any
  version this project could plausibly be run against):
  ```
  $ ssh -o BatchMode=yes -- -oProxyCommand=id nonexistentzzz
  hostname contains invalid characters        # rejected as a hostname, NOT reinterpreted as -o
  $ ssh -o BatchMode=yes -o ConnectTimeout=1 -- ec2-user@127.0.0.1.invalid
  ssh: Could not resolve hostname ...          # identical to the same command without --
  $ scp -o BatchMode=yes -o ConnectTimeout=1 -- local.txt ec2-user@127.0.0.1.invalid:/tmp/x
  scp: stat local "local.txt": No such file    # identical behavior to without --, just a missing local file
  ```

## Commands you will need

| Purpose       | Command             | Expected on success        |
|---------------|----------------------|-----------------------------|
| Build         | `go build ./...`     | exit 0                      |
| Test          | `go test ./...`      | all pass                    |
| Format check  | `gofmt -l .`          | no output (no files listed) |
| Vet           | `go vet ./...`        | exit 0                      |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ssh_args.go` — add `--` separator in `sshProxyArgs`.
- `internal/aws/ssh_unix.go` — add `validateSSHProxyToken(user, "user")` call in `StartSSHSession`.
- `internal/aws/ssh_windows.go` — add `validateSSHProxyToken(user, "user")` call in `StartSSHSession`.
- `internal/aws/scp.go` — add `validateSSHProxyToken(user, "user")` call and `--` separator in `CopyFile`.
- `internal/aws/ssh_args_test.go` (create) — unit tests for `sshProxyArgs`'s new `--` placement.
- `internal/aws/ssh_validate_test.go` — add `user`-flavored test cases (see Test plan); do not change the function under test.
- `plans/README.md` — status row update only, at the end.

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ssh_validate.go` — reused unchanged; see "Current state" for why no character-set change is needed.
- `internal/aws/ssh_key_push.go` (`PushSSHKeyViaSSM`, `shellSingleQuote`) — different, already-safe mechanism (shell-quoted, not argv-spliced); not part of this finding.
- `internal/aws/scp.go`'s `scpEndpoints` function signature/return values — only the *caller* (`CopyFile`'s args-building) changes; `scpEndpoints` itself keeps returning `(localArg, remoteSpecArg string, remoteFirst bool)` unchanged, so `internal/aws/scp_test.go`'s existing `TestScpEndpoints` must keep passing without modification.
- `cmd_ec2.go` — the CLI flag definitions and call sites are correct already; this is a library-layer fix.
- README.md / help.go — no user-facing behavior, flags, or help text changes; existing valid usernames (`ec2-user`, `ubuntu`, `root`, `ssm-user`, and any other POSIX-style username) continue to work identically. This is a pure hardening fix with no interface change, so CLAUDE.md's "update README after adding a feature/flag" rule does not apply here (no feature or flag is being added).

**Interaction with plan 042** (`plans/042-dedupe-scp-ssh-proxy-flags.md`, if
present): that plan is expected to extract the shared `-o` ProxyCommand-
option-building logic currently duplicated between `sshProxyArgs` and
`CopyFile` into a new shared helper in `ssh_args.go`. If `plans/042-*.md`
exists and its status in `plans/README.md` is `DONE`/merged before you
start this plan, **re-read the actual current contents of
`internal/aws/ssh_args.go` and `internal/aws/scp.go` before starting** —
the function bodies shown in "Current state" above may have moved into a
shared helper. In that case: apply the same two changes (the `--` separator
immediately before the final positional argument(s), and the
`validateSSHProxyToken(user, "user")` call in `StartSSHSession`/`CopyFile`)
to whatever the equivalent current call sites/helper are, preserving the
intent described in "Why this matters," rather than assuming the exact line
numbers or function bodies quoted above are still current. This is also
why "Depends on" above is "none" rather than a hard dependency — this plan
must succeed whether 042 has landed or not.

## Git workflow

- Branch: `advisor/040-validate-ssh-scp-user-arg`
- Commit per logical step (Step 1, then Steps 2–3 together since they're
  the same argv-hardening change across two files, then Step 4 for tests)
  is fine, or a single commit — match this repo's convention of small,
  focused commits. Message style follows this repo's observed convention
  (short imperative, e.g. `fix: validate profile/region before use in ssh
  ProxyCommand` from `git log`). Suggested message for this change:
  `fix: validate --user and guard positional ssh/scp args against leading-dash options`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Validate `user` at all 3 existing `profile`/`region` call sites

In `internal/aws/ssh_unix.go`'s `StartSSHSession`, add a third validation
call right after the existing `region` check, following the exact same
pattern:

```go
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(user, "user"); err != nil {
		return err
	}
```

Apply the identical addition to `internal/aws/ssh_windows.go`'s
`StartSSHSession` (same 3-line block, same position — right after the
`region` check, before the `args :=` line).

Apply the identical addition to `internal/aws/scp.go`'s `CopyFile` (right
after its own `region` check, before the `args := []string{...}` block).

**Verify**: `go build ./...` → exit 0.
**Verify**: `grep -rn 'validateSSHProxyToken(user, "user")' internal/aws/ssh_unix.go internal/aws/ssh_windows.go internal/aws/scp.go` → 3 lines, one per file.

### Step 2: Add a `--` "end of options" separator in `sshProxyArgs`

In `internal/aws/ssh_args.go`, change `sshProxyArgs` so the final element
(`user@instanceID`) is preceded by a literal `"--"` element, guaranteeing
OpenSSH can never reinterpret it as an option no matter what `user` or
`instanceID` contain:

```go
func sshProxyArgs(instanceID, profile, region, user string) []string {
	return []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
		"--",
		fmt.Sprintf("%s@%s", user, instanceID),
	}
}
```

**Verify**: `go build ./...` → exit 0.
**Verify**: `grep -n '"--"' internal/aws/ssh_args.go` → 1 match, on the line immediately before the `fmt.Sprintf("%s@%s", user, instanceID)` line.

### Step 3: Add the same `--` separator in `CopyFile`'s scp argument building

In `internal/aws/scp.go`'s `CopyFile`, insert `args = append(args, "--")`
right after the `if recursive { ... }` block and before the
`scpEndpoints(...)` call, so `--` always comes immediately before the two
positional path arguments (`localPath`/`remoteSpec`), regardless of which
order they're appended in:

```go
	if recursive {
		args = append(args, "-r")
	}
	args = append(args, "--")

	localPath, remoteSpec, remoteFirst := scpEndpoints(instanceID, user, source, dest, download)
	if remoteFirst {
		args = append(args, remoteSpec, localPath)
	} else {
		args = append(args, localPath, remoteSpec)
	}
```

(If plan 042 has already refactored this into a shared helper, apply the
equivalent insertion — `--` immediately before whatever now appends the
final positional path argument(s) — inside that helper or its caller,
whichever actually builds the final `[]string` passed to
`exec.Command`/`syscall.Exec`.)

**Verify**: `go build ./...` → exit 0.
**Verify**: `grep -n '"--"' internal/aws/scp.go` → 1 match, appearing after the `-r`/`recursive` handling and before the `scpEndpoints` call in the source order of `CopyFile`.

### Step 4: Add unit tests

Create `internal/aws/ssh_args_test.go` (no such file exists yet — `scp.go`'s
pure helper `scpEndpoints` already has `internal/aws/scp_test.go` as a
structural model; `sshProxyArgs` has no test file yet). Model the new file
on `scp_test.go`'s plain table-test style:

```go
package aws

import (
	"reflect"
	"testing"
)

func TestSSHProxyArgs(t *testing.T) {
	args := sshProxyArgs("i-0123456789abcdef0", "myprofile", "us-west-2", "ec2-user")

	want := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ProxyCommand=aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p --profile myprofile --region us-west-2",
		"--",
		"ec2-user@i-0123456789abcdef0",
	}
	if !reflect.DeepEqual(args, want) {
		t.Errorf("sshProxyArgs() = %#v, want %#v", args, want)
	}
}

func TestSSHProxyArgsEndsWithSeparatorThenPositional(t *testing.T) {
	args := sshProxyArgs("i-abc", "", "", "-Fmyconfig")

	if len(args) < 2 {
		t.Fatalf("sshProxyArgs() returned too few elements: %#v", args)
	}
	last, secondLast := args[len(args)-1], args[len(args)-2]
	if secondLast != "--" {
		t.Errorf("expected \"--\" immediately before the final positional arg, got %q", secondLast)
	}
	if last != "-Fmyconfig@i-abc" {
		t.Errorf("final positional arg = %q, want %q", last, "-Fmyconfig@i-abc")
	}
}
```

(The second test documents *why* `--` matters: even with a hostile-looking
`user` value that would otherwise be reinterpreted as an option, it now
sits after `--` as a single opaque positional argument.)

In `internal/aws/ssh_validate_test.go`, add cases to the existing
`tests` table in `TestValidateSSHProxyToken` (do not change the function
being tested, only extend the table — follow the exact existing style:
`{"name", "value", wantErr}`):

```go
		{"plain username", "ec2-user", false},
		{"ubuntu username", "ubuntu", false},
		{"root username", "root", false},
		{"user with equals sign", "user=evil", true},
		{"user with space", "ec2 user", true},
```

Do **not** add a case asserting that a bare leading-dash value like
`-oSomething` (no `=`, no `/`, no space) is rejected by
`validateSSHProxyToken` — it is not, by design of the existing character
allow-list (letters/digits/`.`/`_`/`-` are all individually legal, and a
leading `-` alone doesn't violate that pattern). That specific case is
guarded by the `--` separator added in Steps 2–3, not by this function;
`TestSSHProxyArgsEndsWithSeparatorThenPositional` above is the test that
covers it. Asserting that `validateSSHProxyToken("-oSomething", "user")`
returns an error would fail against the current (correct, unchanged)
implementation.

**Verify**: `go test ./internal/aws/... -run 'TestSSHProxyArgs|TestValidateSSHProxyToken' -v` → all listed subtests `PASS`.

## Test plan

- New tests, `internal/aws/ssh_args_test.go` (new file):
  - `TestSSHProxyArgs` — happy path, asserts the full expected argv
    including `--` in the correct position.
  - `TestSSHProxyArgsEndsWithSeparatorThenPositional` — regression test for
    this finding: a hostile-looking `user` (`-Fmyconfig`) still ends up as
    an inert positional argument after `--`.
- Extended table in `internal/aws/ssh_validate_test.go`'s existing
  `TestValidateSSHProxyToken` — realistic usernames pass, `=`/space are
  rejected (matching the existing injection-attempt cases already in that
  table for `profile`).
- `internal/aws/scp_test.go`'s existing `TestScpEndpoints` must keep
  passing unmodified (its function signature/behavior is untouched by this
  plan).
- Full verification: `go test ./...` → all tests pass (no count regression
  from before this plan; 2+ new test functions added).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `gofmt -l .` prints no output
- [ ] `go vet ./...` exits 0
- [ ] `grep -rn 'validateSSHProxyToken(user, "user")' internal/aws/ssh_unix.go internal/aws/ssh_windows.go internal/aws/scp.go` → exactly 3 matches (one per file)
- [ ] `grep -n '"--"' internal/aws/ssh_args.go` → at least 1 match, immediately preceding the `%s@%s` positional-arg line in `sshProxyArgs`
- [ ] `grep -n '"--"' internal/aws/scp.go` → at least 1 match, positioned after the `recursive`/`-r` handling and before `scpEndpoints(...)` is called, inside `CopyFile`
- [ ] `go test ./internal/aws/... -run TestSSHProxyArgs -v` → `PASS` for both new test functions
- [ ] `go test ./internal/aws/... -run TestValidateSSHProxyToken -v` → `PASS`, including the new username/`=`/space cases
- [ ] `go test ./internal/aws/... -run TestScpEndpoints -v` → still `PASS`, unmodified
- [ ] `git status` shows only the files listed in "Scope" (In scope) modified/created — no other file changed
- [ ] `plans/README.md` status row for plan 040 updated to `DONE`

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match what you find
  (e.g. plan 042 has landed and restructured `sshProxyArgs`/`CopyFile`) —
  re-read the actual current file contents as instructed in "Interaction
  with plan 042" above and adapt the *insertion points* for `--` and the
  `validateSSHProxyToken(user, ...)` call accordingly; only treat it as a
  true STOP if you cannot locate an equivalent call site that builds the
  final argv passed to `exec.Command`/`syscall.Exec`/`cmd.Run()` for ssh or
  scp.
- Adding `--` changes observed `ssh`/`scp` behavior for a normal, valid
  `user`/`instanceID`/path combination during manual smoke-testing (it
  should not — verified locally against OpenSSH_9.9p2 in "Current state" —
  but if the executor's environment has a materially different OpenSSH
  version that behaves differently, report the version and behavior rather
  than removing the separator).
- A verification command's expected output doesn't match twice in a row
  after a reasonable fix attempt.
- The fix appears to require touching a file outside the Scope section
  (e.g. `ssh_validate.go`'s regex, or `ssh_key_push.go`).

## Maintenance notes

- If a future change adds another OpenSSH-argv-consuming code path (e.g. an
  `sftp` subcommand, or a new flag that becomes another positional
  argument), it must get the same two protections this plan establishes:
  (1) run any freeform string through `validateSSHProxyToken` if it's
  meant to be a restricted-charset AWS-style token (profile/region/user),
  and (2) always place a literal `"--"` immediately before the final
  positional argument(s) in the built argv, regardless of (1) — (1) alone
  is not sufficient, as explained in "Why this matters."
- `internal/aws/ssh_key_push.go`'s `PushSSHKeyViaSSM` takes its own
  `osUser` parameter and handles it via shell single-quoting
  (`shellSingleQuote`) rather than argv splicing — a different, already-
  safe mechanism, deliberately left untouched by this plan. If that
  function is ever refactored to shell out to `ssh`/`scp` directly instead
  of going through SSM Run Command, it would need the same `--`-separator
  treatment as this plan applies here.
- This plan intentionally does not add a leading-dash-specific rejection
  inside `validateSSHProxyToken` itself (e.g. "reject any value starting
  with `-`") because the `--` separator addresses the actual mechanism
  (argv reinterpretation) more robustly and without narrowing what
  `validateSSHProxyToken` is allowed to accept for any of its three
  current callers (`profile`, `region`, `user`). A reviewer should confirm
  they agree this is preferable to a character-level fix before merging.
- No follow-up work is deferred out of this plan for `user`/`instanceID`;
  scp's `source`/`dest` are now also protected by the `--` separator added
  in Step 3, so no further hardening of those is needed either.
