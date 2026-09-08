# Plan 035: Add `act ec2 cp` — copy files to/from an EC2 instance via SSM

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go main_test.go internal/aws/ssh_args.go README.md`
> If any of these files changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: direction (new feature, requested by user)
- **Planned at**: commit `aa50614`, 2026-09-03

## Why this matters

`act ec2 ssh` already gives every instance a real SSH session over an SSM
tunnel, and the README even documents (README.md:216-230) that `scp`/`rsync`
work fine today if the user hand-writes an `~/.ssh/config` `Host i-*` block
with the right `ProxyCommand`. But there is no `act` command that does this
for a one-off file copy: today a user has to know the instance ID already
(no picker), remember the exact `ProxyCommand` string, and get the
`user@instance-id:path` syntax right by hand. This plan adds `act ec2 cp`,
which reuses the exact same SSM-proxied-SSH mechanism `act ec2 ssh` already
relies on, but wraps `scp` instead of `ssh`, with the same interactive
instance picker `ec2 ssh`/`ec2 rdp`/`ssm run` already have. No new AWS API
surface, no new IAM permissions, no new binary dependency beyond the `scp`
that ships with the OpenSSH client `ec2 ssh` already requires.

## Current state

Relevant files:

- `main.go` — subcommand dispatch (`case "ec2":` block), `run*`/`print*Help`
  functions, shared helpers `parseTags`, `pickInstance`.
- `internal/aws/ssh_args.go` — builds the `ssh` `ProxyCommand` argument list
  used by `act ec2 ssh`.
- `internal/aws/ssh_unix.go` / `internal/aws/ssh_windows.go` — platform-
  specific process launch for `act ec2 ssh` (`StartSSHSession`).
- `internal/aws/ssh_validate.go` — `validateSSHProxyToken`, the injection
  guard for `profile`/`region` before they're spliced into the
  `ProxyCommand` string.
- `main_test.go` — has a table-driven test asserting every `print*Help`
  function's output contains certain flag/subcommand substrings.
- `README.md` — feature list, prerequisites, commands table, examples,
  the `ec2` vs `ec2 ssh` comparison section.

**`main.go`'s `ec2` dispatch today** (main.go:52-74):

```go
case "ec2":
	subArgs := args[1:]
	if hasHelp(subArgs) {
		printEC2Help()
		os.Exit(0)
	}
	resolvedProfile := config.ResolveProfile(profile, env)
	resolvedRegion := config.ResolveRegion(region, env)
	if len(subArgs) > 0 && subArgs[0] == "ssh" {
		if hasHelp(subArgs[1:]) {
			printEC2SSHHelp()
			os.Exit(0)
		}
		runSSH(resolvedProfile, resolvedRegion, subArgs[1:])
	} else if len(subArgs) > 0 && subArgs[0] == "rdp" {
		if hasHelp(subArgs[1:]) {
			printEC2RDPHelp()
			os.Exit(0)
		}
		runRDP(resolvedProfile, resolvedRegion, subArgs[1:])
	} else {
		runConnect(resolvedProfile, resolvedRegion, subArgs)
	}
```

**`runSSH`, the pattern this plan's `runCP` must match** (main.go:991-1022):

```go
func runSSH(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	fs.Parse(subArgs)

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	sshUser := *user
	if sshUser == "" {
		picked, err := tui.RunPicker("Select SSH user", []string{"ec2-user", "ubuntu", "root", "ssm-user"})
		if err != nil || picked == "" {
			sshUser = "ec2-user"
		} else {
			sshUser = picked
		}
	}

	err := aws.StartSSHSession(instanceID, profile, region, sshUser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting SSH session: %v\n", err)
		os.Exit(1)
	}
}
```

**`internal/aws/ssh_args.go` in full today** — this is the `ProxyCommand`
string this plan's `scp` wrapper must reuse the same shape of:

```go
package aws

import "fmt"

// sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
// ProxyCommand.
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

**`validateSSHProxyToken`** (`internal/aws/ssh_validate.go`, unexported,
same package) — rejects `profile`/`region` values containing characters
unsafe to splice unescaped into the `ProxyCommand` string. `ec2 ssh` calls
it on both `profile` and `region` before building `sshProxyArgs`; this
plan's `scp` path must do the same.

**Important Go `flag` package gotcha** — `flag.FlagSet.Parse` stops parsing
at the first non-flag argument; everything after that (including further
`--flags`) lands unparsed in `fs.Args()`. Every other subcommand in this
CLI (`ec2 ssh --user ubuntu`, `ssm run --target i-xxx --command "..."`)
therefore documents flags *before* any positional argument, and this plan's
help text and README examples must do the same:
`act ec2 cp [flags] <source> <destination>` — flags first, then the two
positional paths.

**`main_test.go`'s help-text assertion table** (main_test.go:210-227) is a
flat Go slice literal — adding a new command means adding one new entry to
this table, described exactly in Step 5 below. It is a plain substring
check, not reflection — do not overthink it.

**`README.md`'s relevant sections today**:
- Prerequisites (README.md:37): `- For `ec2 ssh`: OpenSSH client (`ssh`) and an SSH key configured on the target instance`
- Commands table (README.md:80-81):
  ```
  | `ec2` | Connect to EC2 instance via SSM session |
  | `ec2 ssh` | SSH to EC2 instance via SSM |
  ```
- `ec2` help subcommand list (README.md — see `printEC2Help` mirror, main.go:283-285):
  ```
  Subcommands:
    ssh          SSH to EC2 instance via SSM (see 'act ec2 ssh help')
    rdp          RDP to Windows EC2 instance via SSM (see 'act ec2 rdp help')
  ```
- Examples section (README.md:147-150):
  ```
  # SSH to an instance via SSM (real SSH with ProxyCommand)
  act ec2 ssh
  act ec2 ssh --user ubuntu
  act ec2 ssh --user ec2-user --target i-0123456789abcdef0
  ```

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|----------------------|
| Build | `go build ./...` | exit 0 |
| Format check | `gofmt -l .` | empty output |
| Vet | `go vet ./...` | exit 0, no output |
| Test | `go test ./...` | `ok` for every package, exit 0 |
| Full local gate (mirrors CI) | `make test` | exit 0 (runs fmt, vet, then test) |

## Scope

**In scope**:
- `internal/aws/ssh_args.go` — extract the shared `ProxyCommand` string
  builder so both `ssh` and the new `scp` path use it (see Step 1).
- `internal/aws/scp.go` (new) — `CopyFile` function wrapping `scp`.
- `internal/aws/scp_test.go` (new) — unit test(s) for the new pure argument-
  building logic.
- `main.go` — new `runCP`, `printEC2CPHelp`, one new dispatch branch inside
  `case "ec2":`, and the `cp` line added to `printEC2Help`'s subcommand list.
- `main_test.go` — one new entry in the `TestHelpFunctionsContainDocumentedFlags`
  table, plus (optionally) `cp` added to the existing `printEC2Help` entry's
  `wantContains`.
- `README.md` — prerequisites line, commands table row, `ec2` subcommand
  list, and a new examples block, per CLAUDE.md's rule to keep README in
  sync with new commands/flags.

**Out of scope** (do NOT touch):
- `internal/aws/ssh_unix.go` / `ssh_windows.go` — `act ec2 ssh` still needs
  its own platform-specific `syscall.Exec`/`exec.Command` split because it
  hands over an interactive terminal. `scp` is a plain non-interactive
  subprocess call and does **not** need that split — see Step 2's design
  note. Do not modify these files.
- `internal/doctor/*` — doctor does not currently check for the `ssh`
  binary either (only AWS CLI + Session Manager plugin), so adding a doctor
  check for `scp` would be new scope beyond what `ec2 ssh` already has. Skip
  it; note it as a possible follow-up in Maintenance notes instead.
- Any change to `act forward`, `act rds`, `act ssm run`, or any other
  existing subcommand.
- Any change to `ec2 ssh`'s or `ec2 rdp`'s existing flags or behavior.

## Git workflow

- Branch: `advisor/035-add-ec2-cp-command`
- Commit per logical step (e.g. one commit for the `aws` package changes,
  one for `main.go`, one for docs), or a single commit if you prefer —
  match the repo's observed style (`git log --oneline`): short imperative
  subject, e.g. `Add act ec2 cp command using SSM-proxied scp`.
- Do NOT push or open a PR. Do NOT merge to `main`.

## Steps

### Step 1: Extract a shared SSM `ProxyCommand` string builder

In `internal/aws/ssh_args.go`, factor the `proxyCmd` construction out of
`sshProxyArgs` into its own function so `scp.go` (Step 2) can reuse it
without duplicating the AWS SSM document/parameter string:

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

Do not change `sshProxyArgs`'s signature or behavior — its return value
must be byte-for-byte identical to before this refactor for the same
inputs.

**Verify**: `go build ./... && go test ./internal/aws/...` → exit 0, all
existing tests (including `TestValidateSSHProxyToken`) still pass.

### Step 2: Add `internal/aws/scp.go` with `CopyFile`

Create `internal/aws/scp.go`:

```go
package aws

import (
	"fmt"
	"os"
	"os/exec"
)

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

	remotePath, localPath := dest, source
	if download {
		remotePath, localPath = source, dest
	}
	remoteSpec := fmt.Sprintf("%s@%s:%s", user, instanceID, remotePath)

	if download {
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

**Design note (do not deviate without reporting back)**: unlike
`StartSSHSession`, this does **not** need a `syscall.Exec`-based unix
variant or a build-tag split. `act ec2 ssh` replaces the process because it
hands over an interactive TTY session. `scp` is a single non-interactive
subprocess: it prints its own progress meter to stderr and exits when
done, and `exec.Command` with inherited stdio works identically on
Linux/macOS/Windows (Windows ships `scp.exe` alongside `ssh.exe` in the
same OpenSSH client `ec2 ssh` already requires per README.md:37). One file,
no `_unix`/`_windows` split, no new build tags.

**Verify**: `go build ./...` → exit 0.

### Step 3: Add `internal/aws/scp_test.go`

There is no existing test file for `ssh_args.go`'s pure argument-building
logic to model after directly, but `internal/aws/ssh_validate_test.go`
shows the repo's plain table-driven style for this package. Since
`CopyFile` shells out to a real `scp` binary, do not attempt to test the
full `CopyFile` function directly (no exec-mocking scaffolding exists in
this repo — do not introduce one). Instead, test the pure logic:

1. Refactor the direction-selection and remote-spec construction in
   `CopyFile` into a small pure helper so it's testable without invoking
   `exec.Command`:

```go
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
```

   Then have `CopyFile` call it:

```go
	localPath, remoteSpec, remoteFirst := scpEndpoints(instanceID, user, source, dest, download)
	if remoteFirst {
		args = append(args, remoteSpec, localPath)
	} else {
		args = append(args, localPath, remoteSpec)
	}
```

2. In `internal/aws/scp_test.go`:

```go
package aws

import "testing"

func TestScpEndpoints(t *testing.T) {
	tests := []struct {
		name           string
		download       bool
		wantLocal      string
		wantRemote     string
		wantRemoteFirst bool
	}{
		{"upload", false, "local.txt", "ec2-user@i-abc123:/remote/path", false},
		{"download", true, "local.txt", "ec2-user@i-abc123:/remote/path", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local, remote, remoteFirst := scpEndpoints("i-abc123", "ec2-user", "local.txt", "/remote/path", tt.download)
			if local != tt.wantLocal {
				t.Errorf("local = %q, want %q", local, tt.wantLocal)
			}
			if remote != tt.wantRemote {
				t.Errorf("remote = %q, want %q", remote, tt.wantRemote)
			}
			if remoteFirst != tt.wantRemoteFirst {
				t.Errorf("remoteFirst = %v, want %v", remoteFirst, tt.wantRemoteFirst)
			}
		})
	}
}
```

Note: for the `"upload"` case `source="local.txt"`, `dest="/remote/path"`;
for `"download"` case `source="/remote/path"`, `dest="local.txt"` — reread
the table above carefully, the test cases pass `source`/`dest` positionally
matching `CopyFile`'s signature, not by intent — get this right or the test
will fail immediately, which is fine, it means you transposed an argument.

**Verify**: `go test ./internal/aws/... -run TestScpEndpoints -v` → both
subtests pass.

### Step 4: Wire up `main.go` — `printEC2CPHelp` and `runCP`

Add `printEC2CPHelp` near `printEC2SSHHelp` (main.go:445-470):

```go
func printEC2CPHelp() {
	fmt.Fprintf(os.Stderr, `act ec2 cp - Copy a file or directory to/from an EC2 instance via SSM

Usage: act [global flags] ec2 cp [flags] <source> <destination>

Copies a file (or, with --recursive, a directory) between the local
machine and an EC2 instance using scp over an SSM-proxied SSH tunnel —
the same mechanism 'act ec2 ssh' uses. By default source is a local path
and destination is a remote path (upload); use --download to reverse the
direction.

Requires an SSH key configured on the target instance (same requirement
as 'act ec2 ssh').

Flags:
  --target      Target instance ID (skip instance picker)
  --user        SSH user (default: prompt interactively)
  --download    Copy from the instance to local instead of local to instance
  --recursive   Copy directories recursively
  --tag         Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile     AWS profile to use
  --region      AWS region to use
  --env         Environment name

Examples:
  act ec2 cp ./deploy.tar.gz /opt/app/deploy.tar.gz
  act ec2 cp --user ubuntu ./deploy.tar.gz /home/ubuntu/deploy.tar.gz
  act ec2 cp --target i-0123456789abcdef0 ./config.yml /etc/app/config.yml
  act ec2 cp --download /var/log/app.log ./app.log
  act ec2 cp --recursive ./dist /opt/app/dist
`)
}
```

Add `runCP` near `runSSH` (after main.go:1022):

```go
func runCP(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ec2 cp", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	download := fs.Bool("download", false, "Copy from the instance to local instead of local to instance")
	recursive := fs.Bool("recursive", false, "Copy directories recursively")
	fs.Parse(subArgs)

	positional := fs.Args()
	if len(positional) != 2 {
		fmt.Fprintf(os.Stderr, "Error: 'ec2 cp' requires exactly 2 positional arguments (source and destination), got %d.\nRun 'act ec2 cp help' for usage.\n", len(positional))
		os.Exit(1)
	}
	source, dest := positional[0], positional[1]

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	sshUser := *user
	if sshUser == "" {
		picked, err := tui.RunPicker("Select SSH user", []string{"ec2-user", "ubuntu", "root", "ssm-user"})
		if err != nil || picked == "" {
			sshUser = "ec2-user"
		} else {
			sshUser = picked
		}
	}

	err := aws.CopyFile(instanceID, profile, region, sshUser, source, dest, *download, *recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying file: %v\n", err)
		os.Exit(1)
	}
}
```

Wire the dispatch branch into the existing `case "ec2":` block
(main.go:52-74), adding a third `else if` alongside `ssh`/`rdp`:

```go
	} else if len(subArgs) > 0 && subArgs[0] == "rdp" {
		if hasHelp(subArgs[1:]) {
			printEC2RDPHelp()
			os.Exit(0)
		}
		runRDP(resolvedProfile, resolvedRegion, subArgs[1:])
	} else if len(subArgs) > 0 && subArgs[0] == "cp" {
		if hasHelp(subArgs[1:]) {
			printEC2CPHelp()
			os.Exit(0)
		}
		runCP(resolvedProfile, resolvedRegion, subArgs[1:])
	} else {
		runConnect(resolvedProfile, resolvedRegion, subArgs)
	}
```

Update `printEC2Help`'s subcommand list (main.go:283-285) to mention `cp`:

```go
Subcommands:
  ssh          SSH to EC2 instance via SSM (see 'act ec2 ssh help')
  rdp          RDP to Windows EC2 instance via SSM (see 'act ec2 rdp help')
  cp           Copy a file to/from EC2 instance via SSM (see 'act ec2 cp help')
```

**Verify**: `go build ./...` → exit 0. Then `./act ec2 cp help` (build
first with `go build -o act .`) → prints the new help text and exits 0.

### Step 5: Update `main_test.go`

Add one entry to the `TestHelpFunctionsContainDocumentedFlags` table
(main_test.go:210-227), matching the existing style exactly:

```go
{"printEC2CPHelp", printEC2CPHelp, []string{"--target", "--user", "--download", "--recursive", "--tag"}},
```

Also update the existing `printEC2Help` entry on the same table (main.go:217)
to include `"cp"`:

```go
{"printEC2Help", printEC2Help, []string{"ssh", "rdp", "cp", "--tag"}},
```

**Verify**: `go test ./... -run TestHelpFunctionsContainDocumentedFlags -v`
→ all subtests pass, including the two you just touched.

### Step 6: Update `README.md`

1. Prerequisites (README.md:37) — broaden the existing `ec2 ssh` line to
   cover `cp` too, since both need the same OpenSSH client and target-side
   SSH key:

```markdown
- For `ec2 ssh`/`ec2 cp`: OpenSSH client (`ssh`/`scp`) and an SSH key configured on the target instance
```

2. Commands table (README.md:80-92) — add a row directly after the
   `ec2 ssh` row:

```markdown
| `ec2 ssh` | SSH to EC2 instance via SSM |
| `ec2 cp` | Copy a file to/from EC2 instance via SSM |
```

3. Feature list (README.md:20) — the existing `SSH over SSM` bullet already
   covers the underlying mechanism; add one new bullet directly after it:

```markdown
- **SSH over SSM** — SSH to instances without open inbound ports
- **File Copy over SSM** — Copy files to/from instances via `act ec2 cp` (scp over the same SSM tunnel)
```

4. Examples section (README.md:147-150) — add a new block directly after
   the existing `ec2 ssh` examples:

```markdown
# SSH to an instance via SSM (real SSH with ProxyCommand)
act ec2 ssh
act ec2 ssh --user ubuntu
act ec2 ssh --user ec2-user --target i-0123456789abcdef0

# Copy a file to an instance via SSM (scp over the same SSH ProxyCommand)
act ec2 cp ./deploy.tar.gz /opt/app/deploy.tar.gz
act ec2 cp --user ubuntu --target i-0123456789abcdef0 ./config.yml /etc/app/config.yml

# Copy a file *from* an instance
act ec2 cp --download /var/log/app.log ./app.log

# Copy a directory recursively
act ec2 cp --recursive ./dist /opt/app/dist
```

**Verify**: manual read-through — no command that runs automated checks
against README prose exists in this repo, so confirm by eye that every
flag/example you added matches `printEC2CPHelp`'s actual text from Step 4.

## Test plan

- `internal/aws/scp_test.go` — new `TestScpEndpoints`, covering the upload
  and download direction cases (Step 3). This is the only new automated
  test; `CopyFile` itself is a thin `exec.Command` wrapper with no branching
  logic left to unit test once `scpEndpoints` is extracted.
- `main_test.go` — extend `TestHelpFunctionsContainDocumentedFlags` (Step 5)
  rather than writing a new test function, matching the existing pattern.
- Manual smoke test (no AWS credentials required for this part): after
  `go build -o act .`, run `./act ec2 cp help` and `./act ec2 cp` (with no
  args) and confirm the latter prints the "requires exactly 2 positional
  arguments... got 0" error and exits 1, rather than hanging or panicking.
- Full suite: `go test ./...` → all packages `ok`, including the new/changed
  tests above.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...` exits 0
- [ ] `go test ./...` exits 0, all packages `ok`
- [ ] `./act ec2 cp help` prints help text containing `--target`, `--user`,
      `--download`, `--recursive`, `--tag`
- [ ] `./act ec2 cp` (no args, no AWS calls needed) exits 1 with a "requires
      exactly 2 positional arguments" error
- [ ] `grep -n "cp" main.go` shows the new dispatch branch, `printEC2CPHelp`,
      and `runCP`
- [ ] `README.md` contains a `` `ec2 cp` `` row in the commands table and at
      least one `act ec2 cp` example
- [ ] Only the files listed in "Scope → In scope" are modified
      (`git status` / `git diff --stat`)
- [ ] `plans/README.md` status row for plan 035 updated

## STOP conditions

Stop and report back (do not improvise) if:

- `sshProxyArgs` or `ssh_args.go` no longer matches the "Current state"
  excerpt above (the file has drifted) — do not guess at how to integrate
  `ssmProxyCommand` into an unfamiliar version of the file.
- `validateSSHProxyToken` has been renamed, removed, or made exported —
  reread `internal/aws/ssh_validate.go` before assuming the call signature
  in Step 2 is still correct.
- The `TestHelpFunctionsContainDocumentedFlags` table in `main_test.go` has
  a materially different shape than shown in Step 5 (e.g. it now uses
  reflection or a different assertion style) — adapt the new entry to match
  what's actually there rather than forcing the old shape in.
- You find that `scp` is not available on any CI/test platform this repo
  targets in a way that would make a *build-time* (not just runtime) check
  fail — this plan assumes `scp` is a runtime dependency only, resolved via
  `exec.LookPath` at call time, never required at compile time.

## Maintenance notes

- `act doctor` does not check for `ssh`/`scp`/OpenSSH client at all today —
  only AWS CLI and the Session Manager plugin. If the maintainer wants
  `act doctor` to catch a missing OpenSSH client before a user hits it at
  `ec2 ssh`/`ec2 cp` runtime, that's a separate, small follow-up plan
  (add a `checkOpenSSHClient` check alongside `checkAWSCLI`/
  `checkSessionManagerPlugin` in `internal/doctor/doctor.go`) — deliberately
  out of scope here since neither existing SSH-based command has it either.
- If `act ec2 ssh` ever grows key-based auth (e.g. an `--identity`/`-i`
  flag to pass a specific private key), `act ec2 cp` should very likely
  grow the matching flag at the same time, since both wrap the same
  underlying SSM-proxied SSH mechanism — check `ssh_args.go` and
  `scp.go` together when that lands.
- A reviewer should scrutinize: (1) that `validateSSHProxyToken` is called
  on `profile`/`region` in `CopyFile` exactly as it is in `StartSSHSession`
  — this is the injection guard, don't let it get dropped in the new path;
  (2) that the flags-before-positionals ordering constraint (from the Go
  `flag` package gotcha noted in "Current state") is reflected correctly
  in both the help text and README examples, since a flag placed after
  `<source> <destination>` will silently be ignored rather than erroring.
