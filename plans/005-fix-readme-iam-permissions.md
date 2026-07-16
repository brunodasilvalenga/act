# Plan 005: Fix README IAM permissions list for `ecs logs`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- README.md internal/aws/logs.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: docs
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`CLAUDE.md` states: "Keep README examples consistent with actual CLI help
output in main.go" and "After adding a new feature, command, or flag, always
update README.md." The README's IAM permissions list
(`README.md:32`) has drifted from what `act ecs logs`'s auto-detection flow
actually calls. A user who scopes a least-privilege IAM policy using exactly
the README's documented list will hit `AccessDenied` running `act ecs logs`
with no `--cluster`/`--service` flags, because three ECS API calls used
during auto-detection aren't listed. This is "stale docs that are actively
wrong" — the highest-priority docs case per this project's own standards.

## Current state

`README.md:32` (the full permissions bullet, one line):

```
- IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`
```

The `act ecs logs` auto-detect flow calls (all shelling out to `aws` via
`exec.Command` in `internal/aws/logs.go`):

- `internal/aws/logs.go:32-61` (`ListECSServices`) — runs `aws ecs
  list-services --cluster ... --output json` (line 33) — requires
  `ecs:ListServices`. Called from `main.go:724` (`runLogs`) when
  `--service` isn't given.
- `internal/aws/logs.go:63-127` (`GetLogGroupsFromService`) — runs two more
  AWS CLI calls:
  - line 65: `aws ecs describe-services --cluster ... --services ...
    --output json` — requires `ecs:DescribeServices`.
  - line 93: `aws ecs describe-task-definition --task-definition ...
    --output json` — requires `ecs:DescribeTaskDefinition`.
  Called from `main.go:746` (`runLogs`) whenever `--log-group` isn't given
  (i.e. the default/documented usage in `README.md`'s own examples: `act
  ecs logs` and `act ecs logs --cluster my-cluster --service my-service`
  both hit this path since neither passes `--log-group`).

None of `ecs:ListServices`, `ecs:DescribeServices`, or
`ecs:DescribeTaskDefinition` appear in the current `README.md:32` list.

Separately: the actual log-tailing call is `aws logs tail ...` (see
`internal/aws/logs_unix.go:13` / `logs_windows.go:11`), which the AWS CLI
implements using the CloudWatch Logs `FilterLogEvents` API — matching the
README's existing `logs:FilterLogEvents` entry. The README also lists
`logs:GetLogEvents`, which is not obviously required by anything in
`internal/aws/logs.go`, `logs_unix.go`, or `logs_windows.go` — leave it in
place for this plan (removing a permission you're not 100% sure is unused
risks under-scoping a real user's policy; adding the missing ones is the
clear, low-risk fix). Do not attempt to verify `GetLogEvents` further; that
determination is out of scope here.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Confirm current text | `grep -n "IAM permissions" README.md` | shows line 32 with old text |
| Confirm fix | `grep -n "ecs:ListServices" README.md` | shows line 32 with new text |

This is a documentation-only change; no build/test commands apply, but run
`go build ./...` anyway as a sanity check that nothing else in the repo was
touched inadvertently.

## Scope

**In scope** (the only files you should modify):
- `README.md` (the single bullet line at line 32)

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/logs.go`, `logs_unix.go`, `logs_windows.go` — no code
  changes; this plan is documentation-only.
- The `logs:GetLogEvents` entry — do not remove it (see rationale above).
- Any other bullet in the "Prerequisites" section
  (`README.md:29-35`) — leave `ec2:DescribeInstances`, `ec2:GetPasswordData`,
  `ssm:StartSession`, `ecs:ListClusters`, `ecs:ListTasks`,
  `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `rds:DescribeDBInstances`,
  `logs:FilterLogEvents` exactly as they are — only add the three missing
  permissions.

## Git workflow

- Branch: `advisor/005-fix-readme-iam-permissions`
- Single commit; message style example from `git log`: `docs: update README with all new commands, flags, and examples` (commit `12569db`) —
  follow the same convention, e.g.
  `docs: add missing ecs:ListServices/DescribeServices/DescribeTaskDefinition to IAM list`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Update the IAM permissions bullet

In `README.md`, find line 32 (search for `IAM permissions for`). Replace the
bullet with (inserting the three missing permissions in the `ecs:` group,
immediately after `ecs:ExecuteCommand` for readability, keeping every other
entry unchanged and in its original order):

```
- IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `ecs:ListServices`, `ecs:DescribeServices`, `ecs:DescribeTaskDefinition`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`
```

**Verify**: `grep -n "ecs:ListServices" README.md` → returns a match on line
32 (or wherever the line now sits if surrounding content shifted — the line
number is not load-bearing, only the content).

### Step 2: Confirm nothing else changed

**Verify**: `git diff README.md` shows only this one line changed (the
permissions bullet), no other content in the file touched.

**Verify**: `go build ./...` → exit 0 (sanity check — this change should
have zero effect on the build since it's a Markdown-only edit).

## Test plan

No automated tests apply to a README prose change. The verification is the
`grep` check in Step 1 confirming the three new permission strings are
present, plus the `git diff` check in Step 2 confirming the edit is scoped
to exactly the intended line.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `grep -n "ecs:ListServices" README.md` returns a match
- [ ] `grep -n "ecs:DescribeServices" README.md` returns a match
- [ ] `grep -n "ecs:DescribeTaskDefinition" README.md` returns a match
- [ ] `git diff --stat` shows only `README.md` changed, with 1 line
      modified (not added/removed as separate lines — it's a single bullet
      edited in place)
- [ ] `go build ./...` exits 0
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- Line 32 of `README.md` doesn't contain the text shown in "Current state"
  (the README has drifted since this plan was written — re-read the
  Prerequisites section in full before editing).
- You find evidence that `logs:GetLogEvents` actually *is* required
  somewhere (e.g. a code path you find while double-checking) — if so, leave
  it in per this plan's scope, and just note the finding in your final
  report; do not expand this plan to investigate further.

## Maintenance notes

- This kind of drift (README permissions list vs. actual `aws` CLI calls in
  `internal/aws/`) has no automated check today (see plan `009`'s related
  DX finding about missing `go vet`/lint gates — a stronger fix would be a
  test that cross-references `printXHelp()`/README flags, but that's a
  separate, larger effort not covered by this plan).
- Any future command that adds a new `aws ecs`/`aws logs`/`aws rds`/`aws
  ec2` subcommand call should be cross-checked against this same
  Prerequisites bullet in `README.md:32` before merging, per the existing
  `CLAUDE.md` rule.
