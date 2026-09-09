# Plan 039: Fix stale "EC2 Instance Connect" comment in README's `--push-key` example

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat 0802c51..HEAD -- README.md`
> If README.md changed since this plan was written, compare the "Current
> state" excerpt below against the live file before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `0802c51`, 2026-09-08

## Why this matters

CLAUDE.md has a hard rule: "Keep README examples consistent with actual CLI
help output in main.go." README.md's Examples section still describes
`act ec2 ssh --push-key` as pushing the key "via EC2 Instance Connect" — that
was the old implementation. The feature was deliberately reimplemented to
push the key via an SSM Run Command instead (see
`internal/aws/ssh_key_push.go`'s `PushSSHKeyViaSSM`), and every other
description of this flag in the repo (the CLI's own `--help` text, and
README's own Prerequisites section) already says "SSM Run Command." Leaving
the stale comment in place directly violates the project's own doc-consistency
rule and will mislead anyone reading the README about how `--push-key`
actually works (it makes them think EC2 Instance Connect must be
enabled/configured, which it does not).

## Current state

- `README.md:153` (Examples section, under the `## Usage` heading, in the
  "SSH to an instance via SSM (real SSH with ProxyCommand)" example block)
  currently reads exactly:

  ```
  act ec2 ssh --push-key                                       # push your local pubkey via EC2 Instance Connect first
  ```

- `README.md:38` (Prerequisites section) already uses the correct, current
  wording and must NOT be changed — it reads:

  ```
  - For `ec2 ssh`: OpenSSH client (`ssh`), and either an SSH key already configured on the target instance, or `--push-key` to add your local public key to the target's `authorized_keys` via SSM Run Command (only needs the SSM Agent — no extra on-instance agent; the key persists until removed, and `--push-key` prints the exact command to remove it); for `ec2 cp`: OpenSSH client (`scp`) and an SSH key configured on the target instance
  ```

- `help.go:209-243` defines `printEC2SSHHelp()`, the function that prints
  `act ec2 ssh help`'s output (called from `main.go:59`). This is the
  authoritative "actual CLI help output" that CLAUDE.md says the README must
  stay consistent with. Its description of `--push-key` (lines 217-219)
  reads exactly:

  ```
  Requires an SSH key configured on the target instance, or use --push-key
  to add your local public key to the target's authorized_keys via SSM
  Run Command first.
  ```

  Note: main.go was recently split into multiple files as part of a
  refactor (help.go, cmd_ec2.go, etc.) — `printEC2SSHHelp` now lives in
  `help.go`, not `main.go`. `main.go:59` just calls it.

- The phrase common to both the correct sources above is **"via SSM Run
  Command"**, and `printEC2SSHHelp` specifically uses "via SSM\nRun Command
  first" (i.e., ends in "... Run Command first."), which is the closest match
  to the stale comment's sentence structure ("... first").

## Commands you will need

| Purpose        | Command                                                         | Expected on success            |
|----------------|------------------------------------------------------------------|---------------------------------|
| Build          | `go build ./...`                                                 | exit 0                          |
| Test           | `go test ./...`                                                  | all pass                        |
| Format check   | `gofmt -l .`                                                      | no output (no files listed)     |
| Vet            | `go vet ./...`                                                    | exit 0                          |

(This is a docs-only change — no Go code is touched — but the plan template
requires listing the repo's standard verification commands; running them
confirms nothing was accidentally broken.)

## Scope

**In scope** (the only file you should modify):
- `README.md` (specifically line 153, the Examples section)

**Out of scope** (do NOT touch, even though they look related):
- `README.md:38` (Prerequisites section) — already correct, do not edit.
- `help.go` (`printEC2SSHHelp`) — already correct, do not edit.
- `main.go` — only routes to `printEC2SSHHelp`; nothing to change.
- `internal/aws/ssh_key_push.go` — implementation is already correct
  (SSM Run Command); this plan only fixes stale documentation text.
- Any other line in README.md's Examples or Prerequisites sections.

## Git workflow

- Branch: `advisor/039-fix-readme-stale-push-key-comment`
- Single commit; message style follows this repo's observed convention
  (short imperative, e.g. `fix: push --push-key via SSM Run Command instead
  of EC2 Instance Connect` from `git log`). Suggested message for this
  change: `docs: fix stale EC2 Instance Connect comment in README push-key example`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Replace the stale comment on README.md's `--push-key` example line

In `README.md`, find the line (currently line 153, inside the `## Usage` →
Examples code block for "SSH to an instance via SSM"):

Before:
```
act ec2 ssh --push-key                                       # push your local pubkey via EC2 Instance Connect first
```

After:
```
act ec2 ssh --push-key                                       # push your local pubkey via SSM Run Command first
```

This is the only change: replace `EC2 Instance Connect` with `SSM Run
Command` in the trailing comment. Do not change spacing/alignment of the
`#` column relative to the line below it (`act ec2 ssh --push-key
--push-key-path ...`) — keep the existing whitespace run before `#` as-is,
only substituting the words after "via".

**Verify**: `grep -n "push your local pubkey" /Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/README.md`
→ outputs exactly:
```
153:act ec2 ssh --push-key                                       # push your local pubkey via SSM Run Command first
```

## Test plan

No new automated tests are needed — this is a documentation string fix with
no executable behavior. Verification is via grep checks only (see Steps and
Done criteria).

- Run `go build ./...` and `go test ./...` anyway to confirm the doc-only
  change didn't accidentally touch any `.go` file.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -c "EC2 Instance Connect" /Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/README.md` → `0`
- [ ] `grep -c "SSM Run Command" /Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/README.md` → `2` (one at the Prerequisites line ~38, one at the fixed Examples line ~153)
- [ ] `grep -n "push your local pubkey" /Users/brunodasilvavalenga/dnx/dnx/aws-connect-tui/README.md` → contains "via SSM Run Command first"
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0
- [ ] `gofmt -l .` prints no output
- [ ] `git status` shows only `README.md` modified — no other file changed
- [ ] `plans/README.md` status row for plan 039 updated to `DONE`

## STOP conditions

Stop and report back (do not improvise) if:

- `README.md:153` (or wherever the `--push-key` example line now lives) does
  not contain the exact stale text `EC2 Instance Connect` shown in "Current
  state" — the file may have already been fixed or reworded differently;
  report what you found instead of guessing a replacement.
- `printEC2SSHHelp` in `help.go` no longer says "SSM Run Command" (i.e., the
  implementation's own help text changed) — in that case the "correct"
  wording this plan targets may itself be stale; report the current wording
  instead of copying the (possibly outdated) text from this plan.
- Any command in "Commands you will need" fails for reasons unrelated to
  this change (e.g., pre-existing build/test failures) — report the failure
  rather than attempting unrelated fixes.
- The fix appears to require touching any file outside the Scope section.

## Maintenance notes

- This is a leaf, one-line docs fix — nothing depends on it and it unblocks
  nothing else.
- If `--push-key`'s underlying mechanism changes again in the future (e.g.,
  a different transport than SSM Run Command), update all three locations
  together: `README.md` Prerequisites (~line 38), `README.md` Examples
  (~line 153), and `help.go`'s `printEC2SSHHelp` (~lines 217-219) — per
  CLAUDE.md's rule that README must stay consistent with the CLI's own help
  output.
- No follow-up work is deferred out of this plan.
