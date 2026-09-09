# Plan 042: Extract shared `sshProxyOptionArgs` helper for the SSH-proxy `-o` flags duplicated in `ssh_args.go` and `scp.go`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/ssh_args.go internal/aws/scp.go`
> If either in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition (see also the note about plan 040
> under "Maintenance notes" — it may add unrelated validation calls to
> `scp.go` around the same time; that alone is not a mismatch requiring a
> stop, but re-read the live file and adapt line numbers accordingly).

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tech-debt
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

`internal/aws/scp.go`'s `CopyFile` currently re-inlines the exact same three
`-o` SSH-hardening flags (`StrictHostKeyChecking=no`,
`UserKnownHostsFile=/dev/null`, and the `ProxyCommand=...` flag) that
`internal/aws/ssh_args.go`'s `sshProxyArgs` already builds. Both functions
already call the shared `ssmProxyCommand(profile, region string) string`
helper for the `ProxyCommand=...` string itself — only the three-pair `-o`
list around it is duplicated, not yet extracted. This is now the second copy
of this exact block; a third SSM-proxied-SSH command (there is already
`act ec2 cp` using `scp.go` and `act ec2 ssh` using `ssh_args.go` — a
plausible third candidate is a future `act ec2 sftp` or similar) would very
likely copy it a third time. More importantly, any future change to these
hardening flags (e.g. adding `-o ConnectTimeout=10`) has to be made in two
places today, and it is easy to update one call site and silently miss the
other, producing a security-relevant behavioral split between `act ec2 ssh`
and `act ec2 cp`. Extracting one shared helper removes that risk with a
mechanical, behavior-preserving refactor.

## Current state

- `internal/aws/ssh_args.go` — builds the `ssh` CLI argv for the SSM-proxied
  session. Full current file (27 lines):

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

  (`ssmProxyCommand` is lines 7-16, `sshProxyArgs` is lines 20-27, as of
  commit `0802c51`.)

- `internal/aws/scp.go` — builds the `scp` CLI argv for the same tunnel. Full
  current file (59 lines):

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

  (The duplicated `-o` block is lines 33-37, as of commit `0802c51`.)

- `internal/aws/ssh_validate.go` — defines `validateSSHProxyToken(value,
  kind string) error` (lines 10-22), already called by `CopyFile` (scp.go
  lines 26-31) and by both `StartSSHSession` implementations
  (`ssh_unix.go` lines 12-17, `ssh_windows.go` lines 10-15) for `profile`
  and `region`. This plan does not touch `validateSSHProxyToken` or its call
  sites — it is out of scope (see "Scope" below and the cross-plan note in
  "Maintenance notes").

- `internal/aws/scp_test.go` — the existing test file for `scp.go`. It
  currently has one table-driven test, `TestScpEndpoints`, covering the pure
  `scpEndpoints` helper only (upload and download cases). Full current file
  (36 lines):

  ```go
  package aws

  import "testing"

  func TestScpEndpoints(t *testing.T) {
  	tests := []struct {
  		name            string
  		download        bool
  		wantLocal       string
  		wantRemote      string
  		wantRemoteFirst bool
  	}{
  		{"upload", false, "local.txt", "ec2-user@i-abc123:/remote/path", false},
  		{"download", true, "local.txt", "ec2-user@i-abc123:/remote/path", true},
  	}
  	for _, tt := range tests {
  		t.Run(tt.name, func(t *testing.T) {
  			var source, dest string
  			if tt.download {
  				source, dest = "/remote/path", "local.txt"
  			} else {
  				source, dest = "local.txt", "/remote/path"
  			}
  			local, remote, remoteFirst := scpEndpoints("i-abc123", "ec2-user", source, dest, tt.download)
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

  There is currently **no test file for `ssh_args.go`** (`ls
  internal/aws/` has no `ssh_args_test.go`). This plan creates one.

- Repo convention to match: table-driven tests using an anonymous struct
  slice, `t.Run(tt.name, ...)` subtests, and plain `if got != want {
  t.Errorf(...) }` checks — see `TestScpEndpoints` above and
  `internal/aws/ssh_validate_test.go`. New tests in this plan must follow
  this same shape, not `testify` or table-driven-with-reflection helpers
  (the repo uses neither).

- Module path: `github.com/brunodasilvalenga/act` (from `go.mod`). All
  in-scope code lives in `internal/aws`, package `aws` — no import path
  changes needed since the new helper stays in the same package.

## Exact before/after argv (must be byte-for-byte identical)

For `sshProxyArgs("i-abc123", "myprofile", "us-west-2", "ec2-user")`, both
before and after this refactor, the returned `[]string` must be exactly:

```
["-o", "StrictHostKeyChecking=no",
 "-o", "UserKnownHostsFile=/dev/null",
 "-o", "ProxyCommand=aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p --profile myprofile --region us-west-2",
 "ec2-user@i-abc123"]
```

For `CopyFile`'s internal `args` slice immediately after the `-o` block is
built (before the `-r`/path arguments are appended), with the same
`profile`/`region`, both before and after this refactor it must be exactly:

```
["-o", "StrictHostKeyChecking=no",
 "-o", "UserKnownHostsFile=/dev/null",
 "-o", "ProxyCommand=aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p --profile myprofile --region us-west-2"]
```

With `profile=""` and `region=""`, the third pair's value in both cases
must be exactly (no trailing space, no `--profile`/`--region` at all —
this is `ssmProxyCommand`'s existing conditional-append behavior at
`ssh_args.go:9-14`, unchanged by this plan):

```
"ProxyCommand=aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
```

This is the acceptance bar for Step 1 and Step 2 below: the diff must not
change what argv either function produces for any input, only how it is
assembled internally.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build ./...` | exit 0, no output |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | exit 0, empty output (no files listed) |
| Test (whole repo) | `go test ./...` | exit 0, all packages `ok` |
| Test (this package only) | `go test ./internal/aws/... -v` | exit 0, all tests `PASS` |
| Makefile equivalent | `make test` | runs fmt+vet+test, exit 0 |

(Verified against this repo during planning: `go build ./...`, `go vet
./...`, and `gofmt -l .` all currently pass clean with no output;
`go test ./internal/aws/... -run 'TestScpEndpoints|TestSSH' -v` currently
shows 3 tests passing in the `internal/aws` package.)

## Scope

**In scope** (the only files you should modify or create):
- `internal/aws/ssh_args.go` (modify — add the new helper, update
  `sshProxyArgs` to call it)
- `internal/aws/scp.go` (modify — update `CopyFile` to call the new helper)
- `internal/aws/ssh_args_test.go` (create — new test file for the new
  helper)

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ssh_validate.go` and `validateSSHProxyToken` — unrelated to
  this refactor; already wired up correctly at all current call sites.
  Another plan (040) may add a new call site for it inside `scp.go` around
  the same time — see "Maintenance notes". Do not add, remove, or move any
  `validateSSHProxyToken` call as part of this plan.
- `internal/aws/ssh_unix.go` / `internal/aws/ssh_windows.go` — these call
  `sshProxyArgs` but do not contain any inlined `-o` flags themselves; no
  change needed.
- `internal/aws/scp_test.go` — leave `TestScpEndpoints` exactly as is; do
  not add scp-specific proxy-flag assertions there — put the new coverage
  in `ssh_args_test.go` next to the helper it tests (per the existing
  convention of one test file per production file in this package: compare
  `ecs_exec_args.go` with no dedicated test vs. `scp.go`/`scp_test.go` and
  `ssh_validate.go`/`ssh_validate_test.go` — helpers with meaningful pure
  logic get their own `_test.go`).
- `main.go`, `README.md`, or any other file — this is a pure internal
  refactor with no user-visible behavior change, no new flag, and no new
  command, so per the CLAUDE.md rule ("after adding a new feature, command,
  or flag, update README.md"), README does not need updating here — nothing
  user-facing changed.

## Git workflow

- Branch: `advisor/042-dedupe-scp-ssh-proxy-flags` (matches this repo's
  observed convention, e.g. `advisor/036-ec2-ssh-push-key-instance-connect`,
  `advisor/035-add-ec2-cp-command`).
- Single commit is fine given the small size; if you prefer to split, one
  commit for Steps 1-3 (the extraction) and one for Step 4 (the new test)
  is also acceptable.
- Commit message style: this repo uses short Conventional-Commits-style
  subjects, e.g. `fix: push --push-key via SSM Run Command instead of EC2
  Instance Connect` and `feat: add --push-key to act ec2 ssh via EC2
  Instance Connect` (see `git log --oneline -5`). Use a `refactor:` prefix,
  e.g.: `refactor: extract shared sshProxyOptionArgs helper`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add `sshProxyOptionArgs` to `ssh_args.go`

In `internal/aws/ssh_args.go`, add a new exported-to-package function
directly above `sshProxyArgs`:

```go
// sshProxyOptionArgs builds the "-o" flag pairs shared by both the ssh
// and scp code paths: SSH host-key hardening plus the SSM ProxyCommand.
func sshProxyOptionArgs(profile, region string) []string {
	return []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
	}
}
```

**Verify**: `go build ./...` → exit 0, no output (the function is unused by
anything yet but Go does not error on unused package-level functions, only
unused local variables/imports, so this compiles fine even before Step 2).

### Step 2: Update `sshProxyArgs` to call the new helper

In `internal/aws/ssh_args.go`, replace the body of `sshProxyArgs` so it
delegates to `sshProxyOptionArgs` instead of inlining the three pairs:

```go
// sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
// ProxyCommand.
func sshProxyArgs(instanceID, profile, region, user string) []string {
	args := sshProxyOptionArgs(profile, region)
	return append(args, fmt.Sprintf("%s@%s", user, instanceID))
}
```

Do not change `ssmProxyCommand` at all.

**Verify**:
1. `go build ./...` → exit 0, no output.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l internal/aws/ssh_args.go` → empty output.
4. `go test ./internal/aws/... -run TestSSH -v` → will not yet find the new
   test (added in Step 4); at this point just confirm no existing test in
   the package regressed: `go test ./internal/aws/... -v` → all existing
   tests still `PASS`, exit 0.

### Step 3: Update `CopyFile` in `scp.go` to call the new helper

In `internal/aws/scp.go`, replace the inlined `args := []string{...}` block
(currently lines 33-37) with a call to the new helper:

```go
	args := sshProxyOptionArgs(profile, region)
```

The rest of `CopyFile` (the `if recursive { ... }` block, the
`scpEndpoints` call, and everything after) stays exactly as it is — do not
reformat or reorder anything else in the function. After this edit, `fmt`
is still used elsewhere in `scp.go` (in `scpEndpoints` and the
`exec.LookPath` error), so the `"fmt"` import stays; do not remove it.

**Verify**:
1. `go build ./...` → exit 0, no output.
2. `go vet ./...` → exit 0, no output.
3. `gofmt -l internal/aws/scp.go` → empty output.
4. `go test ./internal/aws/... -run TestScpEndpoints -v` → `PASS`, exit 0
   (confirms `scp.go` still compiles and its existing test is unaffected).

### Step 4: Add `internal/aws/ssh_args_test.go`

Create a new file `internal/aws/ssh_args_test.go` with a table-driven test
for `sshProxyOptionArgs`, modeled on `TestScpEndpoints`'s style in
`scp_test.go` (anonymous struct slice, `t.Run` subtests, plain
`t.Errorf`/`t.Fatalf` checks — no external test libraries). Cover:

1. Both `profile` and `region` non-empty — the third `-o` pair's value must
   contain both ` --profile <profile>` and ` --region <region>` appended in
   that order (matching `ssmProxyCommand`'s `ssh_args.go:9-14` logic: profile
   is appended before region when both are present).
2. Both `profile` and `region` empty — the third `-o` pair's value must be
   exactly the base command string with **no** `--profile`/`--region`
   suffix at all (per `ssmProxyCommand`'s early-return-free but
   conditional-append logic — empty string skips the `if` block entirely).
3. (Recommended, not required) One mixed case — e.g. `profile` set,
   `region` empty — to confirm the two conditionals in `ssmProxyCommand` are
   independent, not both-or-neither.

Use this exact shape (fill in/adjust only the `wantProxyCmd` values per the
case, computed directly from `ssmProxyCommand`'s logic in `ssh_args.go:7-16`
— do not hardcode a value without deriving it from that function's actual
string concatenation):

```go
package aws

import "testing"

func TestSSHProxyOptionArgs(t *testing.T) {
	const base = "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	tests := []struct {
		name         string
		profile      string
		region       string
		wantProxyCmd string
	}{
		{
			name:         "profile and region set",
			profile:      "myprofile",
			region:       "us-west-2",
			wantProxyCmd: "ProxyCommand=" + base + " --profile myprofile --region us-west-2",
		},
		{
			name:         "empty profile and region",
			profile:      "",
			region:       "",
			wantProxyCmd: "ProxyCommand=" + base,
		},
		{
			name:         "profile set, region empty",
			profile:      "myprofile",
			region:       "",
			wantProxyCmd: "ProxyCommand=" + base + " --profile myprofile",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sshProxyOptionArgs(tt.profile, tt.region)
			want := []string{
				"-o", "StrictHostKeyChecking=no",
				"-o", "UserKnownHostsFile=/dev/null",
				"-o", tt.wantProxyCmd,
			}
			if len(got) != len(want) {
				t.Fatalf("sshProxyOptionArgs(%q, %q) = %v, want %v", tt.profile, tt.region, got, want)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Errorf("sshProxyOptionArgs(%q, %q)[%d] = %q, want %q", tt.profile, tt.region, i, got[i], want[i])
				}
			}
		})
	}
}
```

**Verify**:
1. `gofmt -l internal/aws/ssh_args_test.go` → empty output.
2. `go test ./internal/aws/... -run TestSSHProxyOptionArgs -v` → 3 subtests
   `PASS`, exit 0.

## Test plan

- New test: `TestSSHProxyOptionArgs` in `internal/aws/ssh_args_test.go`
  (created in Step 4), covering: both profile+region set, both empty,
  profile-only. Modeled on `TestScpEndpoints` in `internal/aws/scp_test.go`.
- Regression: re-run the existing `TestScpEndpoints` (in `scp_test.go`) and
  any existing `ssh`-prefixed tests (`ssh_validate_test.go`'s tests, and
  `ssh_key_push_test.go` if it references `sshProxyArgs`/`ssmProxyCommand`
  indirectly — check with `grep -rn "sshProxyArgs\|ssmProxyCommand\|sshProxyOptionArgs" internal/aws/*_test.go` before finishing, and if any other
  test file references these symbols, re-run it too) to confirm no
  regression.
- Final verification: `go test ./... -v 2>&1 | tail -60` → exit 0, every
  package `ok`, and `TestSSHProxyOptionArgs`, `TestScpEndpoints` both show
  `PASS` in the output.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `gofmt -l .` produces empty output
- [ ] `go test ./...` exits 0, all packages report `ok`
- [ ] `go test ./internal/aws/... -run TestSSHProxyOptionArgs -v` shows all
      subtests `PASS`
- [ ] `go test ./internal/aws/... -run TestScpEndpoints -v` shows `PASS`
      (no regression)
- [ ] `grep -n '"-o", "StrictHostKeyChecking=no"' internal/aws/scp.go`
      returns **no matches** (the inline duplicate is gone from `scp.go`)
- [ ] `grep -n 'func sshProxyOptionArgs' internal/aws/ssh_args.go` returns
      exactly one match
- [ ] `grep -c '"-o", "StrictHostKeyChecking=no"' internal/aws/ssh_args.go`
      returns `1` (the flag pair now exists exactly once in the file, inside
      `sshProxyOptionArgs`, not duplicated again inside `sshProxyArgs`)
- [ ] `git status --porcelain` shows changes only in
      `internal/aws/ssh_args.go`, `internal/aws/scp.go`,
      `internal/aws/ssh_args_test.go` (plus `plans/README.md` if you update
      the status row, and the plan file itself is not modified)
- [ ] `plans/README.md` status row for plan 042 updated to `DONE` (or the
      appropriate status) — unless a reviewer told you they own the index

## STOP conditions

Stop and report back (do not improvise) if:

- The live content of `internal/aws/ssh_args.go` or `internal/aws/scp.go`
  does not match the "Current state" excerpts above (e.g. `sshProxyArgs` or
  `CopyFile` already looks different, has different line numbers, or
  already contains a call to a `sshProxyOptionArgs`-like helper). Re-read
  the live file in full before making any change, and treat any
  discrepancy — including finding `validateSSHProxyToken` calls in
  `scp.go` that don't match this plan's excerpt (plan 040 may have added a
  third call for `user` there) — as informational, not blocking: this
  plan's edits only touch the `-o` flag list, not the validation calls, so
  as long as the three `-o` pairs still appear in `CopyFile` in the shape
  shown above, proceed; only stop if the `-o` flag values themselves
  (not just surrounding code) differ from what's quoted in "Current state".
- After Step 2 or Step 3, the exact argv comparison in "Exact before/after
  argv (must be byte-for-byte identical)" fails for any input — i.e. the
  refactored function returns something different from what the original
  inlined code would have returned. This is a hard behavior-preservation
  requirement; do not "fix" a perceived improvement here, revert and report.
- A verification command fails twice in a row after a reasonable fix
  attempt.
- Making this change would require touching any file outside the in-scope
  list (it should not — this is a two-function, same-package extraction).

## Maintenance notes

- **Cross-plan note (non-blocking)**: `plans/040-validate-ssh-scp-user-arg.md`
  (written separately) is expected to add a `validateSSHProxyToken(user,
  "user")` call at three call sites — `ssh_unix.go`'s `StartSSHSession`,
  `ssh_windows.go`'s `StartSSHSession`, and `scp.go`'s `CopyFile` — around
  the same time as this plan. That plan adds validation calls at the call
  sites; it does not depend on this plan's `sshProxyOptionArgs` helper's
  signature or internals, so there is no hard coupling either direction.
  Recommended (soft, non-blocking) order: land this plan (042, a pure
  refactor) first, so plan 040's validation-call additions apply cleanly to
  the final structure rather than to code that's about to be restructured
  underneath it. If 040 lands first instead, this plan's Step 3 edit (in
  `CopyFile`) still applies cleanly — it only touches the `args := []string{...}`
  block, not the `validateSSHProxyToken` calls above it — just re-read the
  live `scp.go` first per the STOP-condition note above.
- Future changes to the SSH hardening/proxy flags (e.g. adding `-o
  ConnectTimeout=10`, changing `StrictHostKeyChecking`) should now be made
  in exactly one place: `sshProxyOptionArgs` in `internal/aws/ssh_args.go`.
  A reviewer should scrutinize the PR for exactly this: confirm no other
  file still inlines these three `-o` pairs after this change lands (the
  `grep` done-criteria above cover this mechanically).
- If a third SSM-proxied-SSH command is added later (e.g. `act ec2 sftp`),
  it should call `sshProxyOptionArgs` directly rather than inlining the
  flags a third time — this plan exists specifically to make that the
  obvious, easy path.
