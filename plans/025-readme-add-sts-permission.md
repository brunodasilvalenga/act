# Plan 025: Add missing `sts:GetCallerIdentity` to README's IAM permissions list

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- README.md`
> If `README.md` changed since this plan was written, compare the "Current
> state" excerpt below against the live file before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`act doctor`'s credentials check (`internal/doctor/doctor.go`, function
`checkCredentials`) runs `aws sts get-caller-identity --output json` to verify
the user is authenticated. README.md's "Prerequisites" section (line 35)
lists the IAM permissions `act` needs, but omits `sts:GetCallerIdentity`. A
user who scopes an IAM policy exactly to the README's list will have `act
doctor`'s credentials check permanently fail with `AccessDenied` even though
every other listed permission is correctly granted — because the one
permission that check actually needs isn't in the list they built. This is
the same class of bug this repo has fixed before: commit `9f9e5a6` ("docs: add
missing ecs:ListServices/DescribeServices/DescribeTaskDefinition to IAM
list") added three ECS permissions that `ecs logs` auto-detection needed but
the README didn't list. This plan is the same fix, for `sts:GetCallerIdentity`.

CLAUDE.md's rule "Keep README examples consistent with actual CLI help output
in main.go" is about command examples/flags specifically, not IAM
permissions — but this plan serves that rule's underlying spirit: the README
must describe what the code actually does, not what it did when last edited.

**Audit confirming this is the only gap**: every distinct AWS CLI subcommand
invoked anywhere in the codebase was enumerated and cross-checked against the
README's permission list. Command: `grep -rn 'args := \[\]string{"' --include="*.go" internal/aws internal/doctor` plus `grep -rln 'exec.Command("aws"' --include="*.go" .` (excluding `.claude/worktrees/`). Result — every subcommand found and its README status:

| AWS CLI subcommand | Found in | IAM permission | In README today? |
|---|---|---|---|
| `ec2 describe-instances` | `internal/aws/ec2.go:42,100` | `ec2:DescribeInstances` | yes |
| `ec2 get-password-data` | `internal/aws/ec2.go:177` | `ec2:GetPasswordData` | yes |
| `ssm start-session` | `internal/aws/session_args.go:5`, `rdp_windows.go:18`, `rdp_unix.go:19`, `forward_args.go:11,35` | `ssm:StartSession` | yes |
| `ssm send-command` | `internal/aws/ssm.go:35` | `ssm:SendCommand` | yes |
| `ssm get-command-invocation` | `internal/aws/ssm.go:87` | `ssm:GetCommandInvocation` | yes |
| `ecs list-clusters` | `internal/aws/ecs.go:43` | `ecs:ListClusters` | yes |
| `ecs list-tasks` | `internal/aws/ecs.go:75` | `ecs:ListTasks` | yes |
| `ecs describe-tasks` | `internal/aws/ecs.go:105` | `ecs:DescribeTasks` | yes |
| `ecs execute-command` | `internal/aws/ecs_exec_args.go:5` | `ecs:ExecuteCommand` | yes |
| `ecs list-services` | `internal/aws/logs.go:33` | `ecs:ListServices` | yes |
| `ecs describe-services` | `internal/aws/logs.go:65` | `ecs:DescribeServices` | yes |
| `ecs describe-task-definition` | `internal/aws/logs.go:93` | `ecs:DescribeTaskDefinition` | yes |
| `rds describe-db-instances` | `internal/aws/rds.go:33` | `rds:DescribeDBInstances` | yes |
| `logs tail` | `internal/aws/logs_args.go:5` | `logs:GetLogEvents`, `logs:FilterLogEvents` | yes |
| `sts get-caller-identity` | `internal/doctor/doctor.go:153` | `sts:GetCallerIdentity` | **NO — this is the gap** |

`sts get-caller-identity` is confirmed the ONLY AWS CLI call in the entire
codebase not backed by a README permission entry (verified via
`grep -rn '\bsts\b' --include="*.go" .` excluding `.claude/worktrees/`, which
returns exactly one real call site: `internal/doctor/doctor.go:153`). No
other gaps were found — this plan's scope is complete as written; do not
expand it.

## Current state

- `README.md` — line 35, the IAM permissions bullet, reads today exactly:
  ```
  - IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ssm:SendCommand`, `ssm:GetCommandInvocation`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `ecs:ListServices`, `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`
  ```
  If the live line differs from this quote at all, treat it as drift — see
  STOP conditions.
- `internal/doctor/doctor.go:149-187` — `checkCredentials`, the function that
  needs the missing permission:
  ```go
  func checkCredentials(profile, region string) result {
      resolvedProfile := config.ResolveProfile(profile, "")
      resolvedRegion := config.ResolveRegion(region, "")

      args := []string{"sts", "get-caller-identity", "--output", "json"}
      if resolvedProfile != "" {
          args = append(args, "--profile", resolvedProfile)
      }
      if resolvedRegion != "" {
          args = append(args, "--region", resolvedRegion)
      }

      out, err := exec.Command("aws", args...).Output()
      ...
  ```
- Convention for this exact kind of fix: commit `9f9e5a6` (`git show 9f9e5a6 --
  README.md`), which added `ecs:ListServices`, `ecs:DescribeServices`,
  `ecs:DescribeTaskDefinition` to the same bullet, as a single-line diff, with
  commit message style `docs: <what was added> to IAM list` plus a body
  explaining which feature/code path needs it. Match this style.
- The permission list is not strictly alphabetical by full string (e.g.
  `ecs:ListClusters` comes before `ecs:ListTasks` before `ecs:DescribeTasks` —
  not alphabetical within `ecs:*` either), but it IS grouped by service
  prefix in a stable, non-alphabetical-by-service order: `ec2:*`, `ssm:*`,
  `ecs:*`, `rds:*`, `logs:*`. This is the order permissions were historically
  appended in as features were added (SSM run-command permissions were
  appended after the original `ec2`/`ssm:StartSession` entries; ECS
  logs-auto-detect permissions were appended after the original ECS-exec
  entries). Given that pattern — new permissions get appended to the end of
  their logical group, or to the end of the whole list when introducing a new
  service prefix — append `sts:GetCallerIdentity` as a new trailing group at
  the very end of the list (after `logs:FilterLogEvents`), since `sts` is a
  new service prefix not yet represented and doesn't logically belong grouped
  inside any existing service's permissions.
- There is no automated README-vs-code consistency test. Confirmed: `grep -n
  "README" main_test.go` returns no matches, and `grep -rln "README"
  --include="*_test.go" .` (excluding `.claude/worktrees/`) returns no files.
  This means this fix cannot be regression-tested automatically — the
  verification is manual grep-based (see Steps below).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Confirm the only sts call site | `grep -rn '\bsts\b' --include="*.go" . \| grep -v '.claude/worktrees'` | exactly one match: `internal/doctor/doctor.go:153` |
| Confirm current README line | `grep -n "IAM permissions" README.md` | shows line 35, missing `sts:GetCallerIdentity` |
| Sanity diff after edit | `git diff README.md` | one line changed, adding `` `sts:GetCallerIdentity` `` |
| Build/test (should be unaffected) | `go build ./...` | exit 0 |
| Test (should be unaffected) | `go test ./...` | exit 0, all pass |

(This is a docs-only change; `go build`/`go test` are run only to confirm the
docs edit did not accidentally touch any `.go` file.)

## Scope

**In scope** (the only file you should modify):
- `README.md`

**Out of scope** (do NOT touch, even though they look related):
- `internal/doctor/doctor.go` — no code change needed; the code is already
  correct, only the docs are wrong.
- Any other permission in the README's list — the audit in "Why this
  matters" confirmed every other AWS CLI call already has a corresponding
  README entry. Do not reorder, rewrite, or "clean up" the rest of the list.
- `plans/README.md` — a separate process owns this index file; only update
  your own plan's status row within it if you are told you own the index,
  otherwise leave it alone per the top-of-file executor instructions.

## Git workflow

- Branch: `advisor/025-readme-add-sts-permission`
- Single commit for this change. Message style, matching precedent commit
  `9f9e5a6` ("docs: add missing ecs:ListServices/DescribeServices/
  DescribeTaskDefinition to IAM list"):
  ```
  docs: add missing sts:GetCallerIdentity to IAM permissions list

  act doctor's credentials check calls `aws sts get-caller-identity` but this
  permission was missing from the README's IAM permissions list, causing the
  check to fail with AccessDenied for users who scope their policy to exactly
  what the README lists.
  ```
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Confirm the codebase has not drifted

Run the drift check and the sts grep from "Commands you will need". Confirm:
- `git diff --stat aa50614..HEAD -- README.md` shows no changes (or, if it
  does, that line 35 still reads exactly as quoted in "Current state" before
  proceeding).
- `grep -rn '\bsts\b' --include="*.go" . | grep -v '.claude/worktrees'`
  returns exactly one match, `internal/doctor/doctor.go:153`, containing
  `args := []string{"sts", "get-caller-identity", "--output", "json"}`.

**Verify**: both commands' output matches the expectations above. If either
does not, stop — see STOP conditions.

### Step 2: Edit the IAM permissions line in README.md

In `README.md`, find the line (currently line 35):
```
- IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ssm:SendCommand`, `ssm:GetCommandInvocation`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `ecs:ListServices`, `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`
```
Change it to append `` `sts:GetCallerIdentity` `` at the end of the list
(after `` `logs:FilterLogEvents` ``):
```
- IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ssm:SendCommand`, `ssm:GetCommandInvocation`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `ecs:ListServices`, `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`, `sts:GetCallerIdentity`
```
Do not change anything else on this line or elsewhere in the file.

**Verify**: `git diff README.md` shows exactly one changed line, and that
line's only difference is the addition of `` , `sts:GetCallerIdentity` `` at
the end.

### Step 3: Confirm nothing else was touched and the build is unaffected

**Verify**:
- `git status` shows only `README.md` as modified.
- `go build ./...` exits 0.
- `go test ./...` exits 0, all tests pass (this confirms the docs edit had no
  side effect on code — no test should reference this README line, per the
  "no automated README-vs-code consistency test" finding above).

## Test plan

- No new tests are needed or possible: this is a one-line prose edit to a
  Markdown file, and the repo has no automated README-content test (confirmed
  in "Current state": no `_test.go` file references `README`).
- Verification is the grep/diff checks in Steps 1–3 above, plus a final human
  read of the changed line in context (`sed -n '31,38p' README.md` or
  equivalent) to confirm it reads naturally and the Markdown backticks are
  balanced.
- Regression check: `go build ./...` and `go test ./...` both exit 0 (proves
  the docs-only change didn't accidentally modify a `.go` file).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n "sts:GetCallerIdentity" README.md` returns exactly one match,
  on the IAM permissions line.
- [ ] `git diff README.md` shows exactly one line changed, adding only
  `` , `sts:GetCallerIdentity` `` at the end of the existing list.
- [ ] `git status` shows no file other than `README.md` modified.
- [ ] `go build ./...` exits 0.
- [ ] `go test ./...` exits 0.
- [ ] Commit created on branch `advisor/025-readme-add-sts-permission` with a
  `docs:` prefixed message.
- [ ] `plans/README.md` status row for plan 025 updated (unless a reviewer
  told you they own the index).

## STOP conditions

Stop and report back (do not improvise) if:

- The IAM permissions line in `README.md` does not match the "Current state"
  quote (the file has drifted since this plan was written — e.g. a different
  plan already added `sts:GetCallerIdentity`, or the line was restructured).
- `grep -rn '\bsts\b' --include="*.go" .` (excluding `.claude/worktrees/`)
  finds any `sts` call site other than `internal/doctor/doctor.go:153`,
  meaning the codebase now calls STS somewhere new and this plan's "only one
  gap" assumption no longer holds.
- `go build ./...` or `go test ./...` fails after the edit — a docs-only
  change to `README.md` should never affect these; a failure means something
  else is wrong in the working tree and should be investigated before
  committing.
- The fix appears to require touching any file other than `README.md`.

## Maintenance notes

- This fix is narrowly scoped to the one confirmed gap
  (`sts:GetCallerIdentity`). The audit in "Why this matters" checked every
  other AWS CLI subcommand call site against the README's permission list and
  found no other gaps as of commit `aa50614` — there is no deferred follow-up
  work from this plan.
- Future risk: every time a new AWS CLI call is added to `internal/aws/` or
  `internal/doctor/`, the README's IAM permissions bullet must be updated in
  the same change. There is no automated check for this (no test asserts the
  README lists a permission for every `aws` subcommand called in the code) —
  a reviewer of any PR that adds a new `exec.Command("aws", ...)` call site
  should manually check whether `README.md`'s Prerequisites section needs a
  matching permission added, the same way this plan and its precedent
  (`9f9e5a6`) had to catch the gap manually after the fact.
- A reviewer of this specific PR should scrutinize only that: (a) exactly one
  line of `README.md` changed, (b) the added permission is
  `sts:GetCallerIdentity` appended at the end of the existing comma-separated
  list, and (c) no other file changed.
