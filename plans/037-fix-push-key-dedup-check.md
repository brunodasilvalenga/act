# Plan 037: Fix `act ec2 ssh --push-key` so re-running it never appends a duplicate `authorized_keys` line

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- internal/aws/ssh_key_push.go internal/aws/ssh_key_push_test.go cmd_ec2.go`
> If any of these three files changed since this plan was written, compare
> the "Current state" excerpts below against the live code before proceeding;
> on a mismatch, treat it as a STOP condition (see also the cross-plan note
> under "Maintenance notes" — a different, unrelated plan touches the same
> file and may have landed first).

## Status

- **Priority**: P1
- **Effort**: M
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

`act ec2 ssh --push-key` is supposed to be idempotent: if you already pushed
your key to an instance, running it again should notice the key is already
there and do nothing. Instead, every single invocation appends a brand-new
line to the target's `~/.ssh/authorized_keys`, because the dedup check
searches for a line containing *this run's own freshly-generated marker*,
which by construction can never already exist. A user who runs `--push-key`
more than once against the same instance (e.g. from a wrapper script, or
because they forgot they already ran it) silently accumulates duplicate
grant lines in `authorized_keys` that they have no record of and no easy way
to find — the tool only ever prints the removal command for the single most
recent line, never for the earlier ones it left behind. This is a real
access-control hygiene bug (stale, forgotten SSH grants pile up on
instances) with a small, well-scoped fix: make the dedup check search for
the *key itself*, which is stable across runs, instead of the per-run
marker.

## Current state

- `internal/aws/ssh_key_push.go` — implements the push. The bug and the fix
  both live in `PushSSHKeyViaSSM` (currently lines 47–94).
- `internal/aws/ssh_key_push_test.go` — currently tests only the two pure
  helpers `findSSHPublicKey` (lines 9–56) and `shellSingleQuote` (lines
  58–76). It does **not** test anything inside `PushSSHKeyViaSSM` — that
  function is untestable as-is because it does real file I/O
  (`os.ReadFile`) and makes real AWS calls (`SendCommand`,
  `WaitForCommandInvocation`).
- `cmd_ec2.go` — the only caller of `PushSSHKeyViaSSM`, at line 66, inside
  the `if *pushKey {}` block (lines 56–73):

  ```go
  56		if *pushKey {
  57			keyPath := *pushKeyPath
  58			if keyPath == "" {
  59				var err error
  60				keyPath, err = aws.DefaultSSHPublicKeyPath()
  61				if err != nil {
  62					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
  63					os.Exit(1)
  64				}
  65			}
  66			removeCmd, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
  67			if err != nil {
  68				fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
  69				os.Exit(1)
  70			}
  71			fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via SSM (added to authorized_keys).\n", keyPath, sshUser, instanceID)
  72			fmt.Fprintf(os.Stderr, "To remove it later: %s\n", removeCmd)
  73		}
  ```

  Note lines 71–72 currently print an unconditional "Pushed ... (added)"
  message plus a remove command, even though (after this fix) the key may
  not have been newly added at all.

- The exact current body of `PushSSHKeyViaSSM` (`internal/aws/ssh_key_push.go:47-94`):

  ```go
  47	func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath string) (string, error) {
  48		keyBytes, err := os.ReadFile(publicKeyPath)
  49		if err != nil {
  50			return "", fmt.Errorf("reading public key: %w", err)
  51		}
  52		key := strings.TrimSpace(string(keyBytes))
  53		if key == "" {
  54			return "", fmt.Errorf("public key file %s is empty", publicKeyPath)
  55		}
  56		if strings.ContainsAny(key, "\n\r") {
  57			return "", fmt.Errorf("public key file %s contains more than one line; expected a single public key", publicKeyPath)
  58		}
  59
  60		marker := fmt.Sprintf("act-push-key-%d", time.Now().UnixNano())
  61		keyLine := fmt.Sprintf("%s act-push-key %s", key, marker)
  62
  63		osUserQ := shellSingleQuote(osUser)
  64		ownerQ := shellSingleQuote(osUser + ":" + osUser)
  65		keyLineQ := shellSingleQuote(keyLine)
  66
  67		script := fmt.Sprintf(`set -e
  68	homedir=$(getent passwd %s | cut -d: -f6)
  69	if [ -z "$homedir" ]; then echo "act-push-key: no such user %s" >&2; exit 1; fi
  70	mkdir -p "$homedir/.ssh"
  71	touch "$homedir/.ssh/authorized_keys"
  72	chmod 700 "$homedir/.ssh"
  73	chmod 600 "$homedir/.ssh/authorized_keys"
  74	chown %s "$homedir/.ssh" "$homedir/.ssh/authorized_keys"
  75	grep -qxF %s "$homedir/.ssh/authorized_keys" || echo %s >> "$homedir/.ssh/authorized_keys"
  76	echo "$homedir/.ssh/authorized_keys"`, osUserQ, osUserQ, ownerQ, keyLineQ, keyLineQ)
  77
  78		commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
  79		if err != nil {
  80			return "", fmt.Errorf("sending push-key command: %w", err)
  81		}
  82
  83		result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
  84		if err != nil {
  85			return "", fmt.Errorf("waiting for push-key command: %w", err)
  86		}
  87		if result.Status != "Success" {
  88			return "", fmt.Errorf("push-key command finished with status %s: %s", result.Status, strings.TrimSpace(result.Stderr))
  89		}
  90
  91		authKeysPath := strings.TrimSpace(result.Stdout)
  92		removeCmd := fmt.Sprintf(`act ssm run --target %s --command "sed -i '/%s/d' %s"`, instanceID, marker, authKeysPath)
  93		return removeCmd, nil
  94	}
  ```

  Line 75 is the bug: `grep -qxF %s` is given `keyLineQ` (the *marker-tagged*
  line), which is unique to this run and therefore can never match a line
  written by a previous run — so the `|| echo ... >>` always fires.

- **The fix**: change the `grep` target on line 75 from the marker-tagged
  `keyLineQ` to a new `keyQ := shellSingleQuote(key)` (just the raw public
  key text, stable across runs), and switch from `-qxF` (exact whole-line
  match) to `-qF` (fixed-string *substring* match, since the on-disk line is
  `"<key> act-push-key <marker>"`, not exactly `<key>`). SSH public keys are
  base64 and never contain the literal string `act-push-key`, so this
  substring match cannot falsely trigger on the marker text itself, and a
  prior run's differently-marked line will now correctly match because it
  still contains the same key text.

- **Test environment constraint you must respect**: `getent` (used on line
  68 of the script) is Linux-only and is not available on macOS. Do **not**
  write a test that executes the *whole* generated script via `bash`/`sh` —
  it will fail on any macOS dev machine or CI runner regardless of
  correctness. Test the script's *text* (the dedup `grep` line it contains),
  not its execution.

- **Repo's existing extraction-for-testability convention** — follow this
  exactly. `internal/aws/scp.go:9-18` extracts a pure function out of an
  I/O-performing one so it's unit-testable:

  ```go
  // scp.go:9-18
  func scpEndpoints(instanceID, user, source, dest string, download bool) (localArg, remoteSpecArg string, remoteFirst bool) {
  	remotePath, localPath := dest, source
  	if download {
  		remotePath, localPath = source, dest
  	}
  	remoteSpec := fmt.Sprintf("%s@%s:%s", user, instanceID, remotePath)
  	return localPath, remoteSpec, download
  }
  ```

  and `internal/aws/scp_test.go` tests it directly with a table-driven test
  (no I/O, no mocking):

  ```go
  // scp_test.go:5-36
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
  			// ... assertions ...
  		})
  	}
  }
  ```

  This plan applies the same pattern: extract the shell-script text into a
  pure `buildPushKeyScript(osUser, key, marker string) string` function, and
  table-test *that*.

## Scope decision: what happens when the key is already present

The finding this plan fixes leaves open a design choice: when the key is
already present, should the tool (a) silently skip the append but still
give the user *some* remove command (which would require extracting the
existing line's marker back out of the remote script's output), or (b) skip
the append and tell the user plainly that the key is already there, without
attempting to reconstruct a remove command for whatever earlier run added
it?

**This plan chooses (b), the smaller-scoped option.** Reconstructing option
(a) would require the shell script to echo back the *existing* matched
line's marker, then parsing that out in Go, which is more moving parts for
a fix whose core bug is just "the grep target is wrong." Choice (b) still
fully fixes the reported bug (no more duplicate lines on repeat runs) and
gives the user an accurate status message. It is an acceptable, deliberately
smaller scope — noted here so it isn't re-litigated as a missed requirement
in review.

Concretely, `PushSSHKeyViaSSM`'s signature changes from
`(instanceID, profile, region, osUser, publicKeyPath string) (string, error)`
to `(instanceID, profile, region, osUser, publicKeyPath string) (removeCmd string, alreadyPresent bool, err error)`.
When `alreadyPresent` is `true`, `removeCmd` is `""` and the caller must not
print a remove-command line.

## Commands you will need

| Purpose         | Command                              | Expected on success              |
|------------------|---------------------------------------|-----------------------------------|
| Build            | `go build ./...`                     | exit 0, no output                 |
| Vet              | `go vet ./...`                       | exit 0, no output                 |
| Format check     | `gofmt -l .`                         | exit 0, no output (no files listed) |
| Tests (package)  | `go test ./internal/aws/... -run PushKey -v` | all matched tests pass |
| Tests (full)     | `go test ./...`                      | `ok` for every package, exit 0    |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ssh_key_push.go`
- `internal/aws/ssh_key_push_test.go`
- `cmd_ec2.go`
- `plans/README.md` (status row update only, at the end)

**Out of scope** (do NOT touch, even though they look related):
- Any platform/OS check on the target instance (e.g. guarding
  `AWS-RunShellScript` against non-Linux targets) — that is a separate,
  already-planned concern; see "Maintenance notes" below for the cross-plan
  note.
- `internal/aws/ssh_key_push.go`'s `findSSHPublicKey` / `shellSingleQuote`
  helpers and their existing tests — unrelated to this bug, leave as-is.
- README.md — this is a pure bug fix with no new flag, no changed flag
  semantics, and no changed prerequisites; the existing description at
  README.md:38 ("the key persists until removed, and `--push-key` prints
  the exact command to remove it") remains accurate for the "newly added"
  case, which is still the common case for a first run. If you find during
  implementation that the user-visible behavior described there no longer
  matches (e.g. you decide to change the printed message wording in a way
  that contradicts README.md:38), update that one sentence — but do not do
  a broader README pass.

## Git workflow

- Branch: `advisor/037-fix-push-key-dedup-check` (matches this repo's
  observed convention, e.g. `advisor/036-ec2-ssh-push-key-instance-connect`
  from `git log`).
- Commit per logical unit (e.g. one commit for the `internal/aws` change +
  its test, one for the `cmd_ec2.go` caller update, if you want to split
  them; a single commit for the whole fix is also fine given the size).
  Match the repo's commit message style from `git log` — short imperative
  subject line, e.g. `fix: dedup push-key against the key itself, not the per-run marker`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Extract the script-building logic into a pure, testable function

In `internal/aws/ssh_key_push.go`, add a new function above
`PushSSHKeyViaSSM`:

```go
// buildPushKeyScript returns the shell script that appends key to osUser's
// authorized_keys, tagging the appended line with marker so it can be
// removed later. The dedup check greps for the raw key text (stable across
// runs) rather than the marker-tagged line, so a line left by a previous
// run — which carries a different marker but the same key — is correctly
// recognized as "already present" and no duplicate is appended. It prints
// "act-push-key: already present" as its first line of output when it
// skips the append, so the caller can tell the two outcomes apart; the
// authorized_keys path is always the last line of output.
func buildPushKeyScript(osUser, key, marker string) string {
	keyLine := fmt.Sprintf("%s act-push-key %s", key, marker)

	osUserQ := shellSingleQuote(osUser)
	ownerQ := shellSingleQuote(osUser + ":" + osUser)
	keyQ := shellSingleQuote(key)
	keyLineQ := shellSingleQuote(keyLine)

	return fmt.Sprintf(`set -e
homedir=$(getent passwd %s | cut -d: -f6)
if [ -z "$homedir" ]; then echo "act-push-key: no such user %s" >&2; exit 1; fi
mkdir -p "$homedir/.ssh"
touch "$homedir/.ssh/authorized_keys"
chmod 700 "$homedir/.ssh"
chmod 600 "$homedir/.ssh/authorized_keys"
chown %s "$homedir/.ssh" "$homedir/.ssh/authorized_keys"
if grep -qF %s "$homedir/.ssh/authorized_keys"; then
  echo "act-push-key: already present"
else
  echo %s >> "$homedir/.ssh/authorized_keys"
fi
echo "$homedir/.ssh/authorized_keys"`, osUserQ, osUserQ, ownerQ, keyQ, keyLineQ)
}
```

Key points to get exactly right:
- The `grep -qF %s` target is `keyQ` (raw key), **not** `keyLineQ`. This is
  the actual bug fix.
- `-qF` not `-qxF`: fixed-string substring match, because the on-disk line
  is `"<key> act-push-key <marker>"` and will never equal `<key>` exactly.
- The `echo %s >>` inside the `else` branch still uses `keyLineQ` (the
  marker-tagged line) — newly appended lines must keep carrying a marker so
  `removeCmd` (built in Step 2) can still target them precisely with `sed`.
- Every line of the script must remain valid when later passed through
  `strings.Split(script, "\n")` to `SendCommand` (same as today) — do not
  introduce a literal newline inside any of the `%s`-substituted values.

**Verify**: `go build ./...` → exit 0, no output. (This alone won't catch
logic errors — Step 3's tests do that — it just confirms the file still
compiles.)

### Step 2: Rewire `PushSSHKeyViaSSM` to use the new function and report `alreadyPresent`

Replace the body of `PushSSHKeyViaSSM` (currently
`internal/aws/ssh_key_push.go:47-94`) so that:

1. It calls `buildPushKeyScript(osUser, key, marker)` instead of building
   `script` inline (delete the old lines 60–76's `keyLine`/`osUserQ`/
   `ownerQ`/`keyLineQ`/`script :=` — `marker` itself is still generated here
   and passed in).
2. Its signature becomes:
   ```go
   func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath string) (removeCmd string, alreadyPresent bool, err error)
   ```
   Every existing `return "", fmt.Errorf(...)` in the function must become
   `return "", false, fmt.Errorf(...)` (there are 5 such early-return sites
   in the current body: empty-key, multi-line-key, `SendCommand` error,
   `WaitForCommandInvocation` error, non-`Success` status).
3. After the command succeeds, parse `result.Stdout` to detect which branch
   the script took and to find `authKeysPath` (now the *last* line of
   output, since the script may print the "already present" line first):
   ```go
   outLines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
   authKeysPath := outLines[len(outLines)-1]
   alreadyPresent = len(outLines) > 1 && strings.Contains(outLines[0], "act-push-key: already present")
   if alreadyPresent {
   	return "", true, nil
   }
   removeCmd = fmt.Sprintf(`act ssm run --target %s --command "sed -i '/%s/d' %s"`, instanceID, marker, authKeysPath)
   return removeCmd, false, nil
   ```

**Verify**: `go build ./...` → fails (expected at this point, because
`cmd_ec2.go`'s call site hasn't been updated yet — confirm the *only*
compile error is about `PushSSHKeyViaSSM`'s call in `cmd_ec2.go` having the
wrong number of return values, e.g. `assignment mismatch: 2 variables but
aws.PushSSHKeyViaSSM returns 3 values`). If you see any other compile
error, STOP and report it before continuing.

### Step 3: Update the caller in `cmd_ec2.go`

Replace `cmd_ec2.go:66-72`:

```go
		removeCmd, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via SSM (added to authorized_keys).\n", keyPath, sshUser, instanceID)
		fmt.Fprintf(os.Stderr, "To remove it later: %s\n", removeCmd)
```

with:

```go
		removeCmd, alreadyPresent, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
			os.Exit(1)
		}
		if alreadyPresent {
			fmt.Fprintf(os.Stderr, "%s is already in %s@%s's authorized_keys; not adding a duplicate.\n", keyPath, sshUser, instanceID)
		} else {
			fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via SSM (added to authorized_keys).\n", keyPath, sshUser, instanceID)
			fmt.Fprintf(os.Stderr, "To remove it later: %s\n", removeCmd)
		}
```

**Verify**: `go build ./...` → exit 0, no output. `go vet ./...` → exit 0,
no output. `gofmt -l .` → exit 0, no output.

### Step 4: Add table-driven tests for `buildPushKeyScript`

Append to `internal/aws/ssh_key_push_test.go` (model the table-driven style
after `TestShellSingleQuote` in the same file, and the pure-function
extraction-testing approach of `internal/aws/scp_test.go`'s
`TestScpEndpoints`):

```go
func TestBuildPushKeyScript(t *testing.T) {
	const key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyMaterial user@laptop"

	t.Run("dedup check greps for the raw key, not the marker-tagged line", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyQ := shellSingleQuote(key)
		if !strings.Contains(script, "grep -qF "+keyQ) {
			t.Errorf("script does not grep for the raw key %q; got:\n%s", keyQ, script)
		}
	})

	t.Run("dedup grep target is identical across two different markers for the same key", func(t *testing.T) {
		// This is the regression this plan fixes: the old code grepped for
		// the marker-tagged line, so a previous run's line (different
		// marker, same key) could never be found as "already present".
		scriptA := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		scriptB := buildPushKeyScript("ec2-user", key, "act-push-key-222")
		keyQ := shellSingleQuote(key)
		grepLine := "grep -qF " + keyQ
		if !strings.Contains(scriptA, grepLine) || !strings.Contains(scriptB, grepLine) {
			t.Fatalf("expected both scripts to contain the marker-independent grep line %q", grepLine)
		}
	})

	t.Run("does not regress to matching on the full marker-tagged line", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyLineQ := shellSingleQuote(key + " act-push-key act-push-key-111")
		if strings.Contains(script, "grep -qF "+keyLineQ) || strings.Contains(script, "grep -qxF "+keyLineQ) {
			t.Errorf("dedup check still matches on the marker-tagged line, not just the key; got:\n%s", script)
		}
	})

	t.Run("append branch still tags the new line with the marker for later removal", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyLineQ := shellSingleQuote(key + " act-push-key act-push-key-111")
		if !strings.Contains(script, "echo "+keyLineQ+" >> ") {
			t.Errorf("expected the appended line to still carry its marker; got:\n%s", script)
		}
	})

	t.Run("prints an already-present marker line so the caller can detect the skip", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		if !strings.Contains(script, `echo "act-push-key: already present"`) {
			t.Errorf("expected an already-present status line; got:\n%s", script)
		}
	})
}
```

Do **not** attempt to execute `buildPushKeyScript`'s output with `bash`/`sh`
in this test — the script calls `getent`, which does not exist on macOS, and
would make this test fail on that platform for reasons unrelated to the fix.

**Verify**: `go test ./internal/aws/... -run TestBuildPushKeyScript -v` →
all subtests print `--- PASS`, overall `ok`.

### Step 5: Full verification pass

**Verify** (run all four; all must pass):
- `go build ./...` → exit 0, no output.
- `go vet ./...` → exit 0, no output.
- `gofmt -l .` → exit 0, no output.
- `go test ./...` → `ok` for every listed package, exit 0.

## Test plan

- New tests: `TestBuildPushKeyScript` in `internal/aws/ssh_key_push_test.go`
  (added in Step 4), covering:
  - the dedup grep targets the raw key, not the marker-tagged line (the
    core regression check);
  - the grep target is identical regardless of which marker is passed in
    (proves marker-independence — the actual bug this plan fixes);
  - the old buggy pattern (`grep -qF`/`-qxF` against the full marker-tagged
    line) does not reappear;
  - the append branch still tags new lines with the marker (removal must
    keep working for newly-added lines);
  - the script emits a machine-detectable "already present" status line.
- Structural pattern to follow: `internal/aws/scp_test.go`'s
  `TestScpEndpoints` (pure function, table/subtests, no I/O, no mocks) and
  this file's own existing `TestShellSingleQuote`.
- Existing tests (`TestFindSSHPublicKey`, `TestShellSingleQuote`) must
  continue to pass unmodified — this plan does not touch those helpers.
- No test exercises `PushSSHKeyViaSSM` itself end-to-end (it still requires
  live AWS calls); this plan intentionally limits new coverage to the
  extracted pure function, consistent with the repo's existing pattern for
  I/O-heavy functions.
- Verification: `go test ./...` → all packages `ok`, including the new
  subtests under `TestBuildPushKeyScript`.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `gofmt -l .` prints nothing
- [ ] `go test ./...` exits 0, and `go test ./internal/aws/... -run TestBuildPushKeyScript -v` shows all subtests passing
- [ ] `grep -n "grep -qxF" internal/aws/ssh_key_push.go` returns no matches (the old exact-line dedup pattern is gone)
- [ ] `grep -n "func PushSSHKeyViaSSM" internal/aws/ssh_key_push.go` shows the 3-return-value signature `(removeCmd string, alreadyPresent bool, err error)`
- [ ] `grep -n "func buildPushKeyScript" internal/aws/ssh_key_push.go` exists
- [ ] `git status --porcelain` shows changes only in `internal/aws/ssh_key_push.go`, `internal/aws/ssh_key_push_test.go`, `cmd_ec2.go`, and `plans/README.md`
- [ ] `plans/README.md` status row for plan 037 updated to DONE (or the repo's current in-progress convention)

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ssh_key_push.go:47-94` or `cmd_ec2.go:56-73`
  does not match the excerpts in "Current state" (the file has drifted —
  possibly because the parallel plan `plans/043-guard-push-key-non-linux-target.md`
  already landed its own change to this file; if so, re-read the live file,
  locate the equivalent grep/marker/dedup logic and caller code by name
  rather than by line number, and re-apply this plan's intent there instead
  of blindly pattern-matching line numbers).
- `go build ./...` fails after Step 3 for any reason other than the
  expected transient mismatch described in Step 2's verification.
- Any step's verification command fails twice in a row after a reasonable
  fix attempt.
- You find that `grep -F` (fixed-string, non-regex) does not behave as
  described here in the target instances' shell (e.g. a non-POSIX `grep`
  that doesn't support `-F`) — this plan assumes a standard GNU/BSD-style
  `grep` such as ships on Amazon Linux, Ubuntu, and other common EC2 AMIs.
- You determine that a public key value could ever legitimately contain the
  literal substring `act-push-key` (it cannot — SSH public keys are
  whitespace-plus-base64 — but if you find a counterexample, the `-qF`
  substring match in Step 1 could misfire and needs the whole-line vs.
  substring tradeoff reconsidered).

## Maintenance notes

- **Cross-plan overlap**: `plans/043-guard-push-key-non-linux-target.md` (a
  separate plan, written in parallel by a different agent) also modifies
  `internal/aws/ssh_key_push.go` and `internal/aws/ssh_key_push_test.go` —
  it adds a platform/OS check near the `SendCommand(instanceID, profile,
  region, "AWS-RunShellScript", ...)` call (`internal/aws/ssh_key_push.go:78`
  in this plan's numbering), which is a different region of the function
  from the grep/marker/dedup logic this plan changes (lines 60-76 in this
  plan's numbering). There is no hard ordering dependency between the two
  plans, but whichever plan executes second should re-read the file first,
  since the other one will have already changed line numbers and possibly
  the function signature area. If both plans are applied, `PushSSHKeyViaSSM`
  will end up with both the 3-return-value signature from this plan and
  whatever platform-guard early-return plan 043 adds — make sure that
  guard's `return` statement also gets updated to match the 3-value
  signature if it lands after this plan, or is written to match it if it
  lands after and follows the older 2-value signature.
- If a future change wants to give the user a *working* remove command even
  when the key was already present (the (a) option this plan explicitly
  deferred, see "Scope decision" above), it will need `buildPushKeyScript`
  to also echo back the *existing* line's marker (e.g. via `grep -oF` combined
  with a marker-extraction pattern) so `PushSSHKeyViaSSM` can build a
  `removeCmd` targeting that specific line — this plan deliberately does
  not do that.
- If `authorized_keys` already accumulated duplicate lines from the bug
  *before* this fix lands (i.e., on instances already pushed-to multiple
  times), this fix does not clean those up — it only stops new duplicates
  from being added going forward. That cleanup, if wanted, is a separate,
  unplanned effort (would need a script to de-duplicate lines by key text
  on affected instances) and is explicitly out of scope here.
- A reviewer should scrutinize: (1) that `-qF` (substring) rather than
  `-qxF` (exact line) is intentional and correctly justified by the
  marker-tagged on-disk format, not an accidental loosening; (2) that the
  `outLines[len(outLines)-1]` parsing in Step 2 is robust to the "already
  present" line being present or absent; (3) that the 2-value-to-3-value
  signature change in `PushSSHKeyViaSSM` is applied consistently at every
  return site.
