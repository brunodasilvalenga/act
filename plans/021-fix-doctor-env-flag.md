# Plan 021: Make `act doctor` honor the global `--env` flag

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go internal/doctor/doctor.go internal/doctor/doctor_test.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P1
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: bug
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

Every other subcommand (`ec2`, `ecs`, `forward`, `ssm run`, `rds`, `fav`) resolves its
AWS profile/region by calling `config.ResolveProfile(profile, env)` /
`config.ResolveRegion(region, env)`, so `act --env staging <cmd>` correctly
picks up the `staging` entry from `~/.act.json`. The `doctor` subcommand is the
one exception: it never receives or passes `env` at all, and its internal
helpers hardcode `""` for the env-name argument. The result: `act --env staging
doctor` silently diagnoses the WRONG account — it falls through to
`AWS_PROFILE`/`AWS_REGION` shell env vars or the config's bare defaults,
with no error or warning that `--env` was ignored. This was reproduced live
during the audit that produced this plan: with a scratch `~/.act.json`
defining `environments.staging = {profile: stagingprof, region: eu-west-1}`,
running `act --env staging doctor` under a shell with `AWS_PROFILE=shellprof
AWS_REGION=shellregion` reported `Profile: shellprof` / `Region: shellregion`
— not `stagingprof`/`eu-west-1` — while `act env list` correctly showed the
`staging` entry as `stagingprof`/`eu-west-1`. A diagnostic command giving
false-confidence output about the wrong account is worse than an obvious
error: someone runs `act --env staging doctor`, sees all green checks, and
concludes staging is healthy when they never actually looked at staging.
After this plan, `doctor` resolves profile/region the same way every other
command does, and its help text documents `--env` like every other command's
help text does.

## Current state

- `main.go` — CLI entry point and subcommand dispatch. The `case "doctor":`
  block (lines 152–176) is the actual bug site; `env` is already in scope as
  a local variable (declared line 24, populated by `parseGlobalFlags` at line
  27) but is never passed into the doctor path:

  ```go
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

  Compare with `case "ec2":` (lines 52–74), which resolves before calling its
  runner:

  ```go
  resolvedProfile := config.ResolveProfile(profile, env)
  resolvedRegion := config.ResolveRegion(region, env)
  ```

  `doctor` does neither — it hands `doctor.Run` the raw, unresolved `profile`
  and `region` flag values and drops `env` on the floor entirely.

- `internal/doctor/doctor.go` (293 lines) — the check implementations.
  - `func Run(profile, region, version string, fix, skipConfirm bool) error`
    (line 49) — signature has no `env` parameter. Body (lines 50–58) calls:
    ```go
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
  - `checkCredentials(profile, region string) result` (lines 149–151):
    ```go
    func checkCredentials(profile, region string) result {
        resolvedProfile := config.ResolveProfile(profile, "")
        resolvedRegion := config.ResolveRegion(region, "")
    ```
    The hardcoded `""` is the bug: it means "no environment name," so
    `ResolveProfile`/`ResolveRegion` never look up `~/.act.json`'s
    `environments` map and fall straight through to `AWS_PROFILE`/
    `AWS_REGION`/`AWS_DEFAULT_REGION` or the config's bare defaults.
  - `checkRegion(flagRegion string) result` (lines 189–190):
    ```go
    func checkRegion(flagRegion string) result {
        resolved := config.ResolveRegion(flagRegion, "")
    ```
    Same hardcoded `""`.
  - `checkProfile(flagProfile string) result` (lines 205–206):
    ```go
    func checkProfile(flagProfile string) result {
        resolved := config.ResolveProfile(flagProfile, "")
    ```
    Same hardcoded `""`.

- `internal/config/config.go` — `ResolveProfile`/`ResolveRegion` (lines
  128–163). Resolution order: (1) CLI flag value if non-empty, (2) if
  `envName` non-empty, look up `cfg.Environments[envName]` in `~/.act.json`
  and use its `Profile`/`Region` if set, (3) `AWS_PROFILE` /
  `AWS_REGION`/`AWS_DEFAULT_REGION` env vars, (4) `cfg.DefaultProfile` /
  `cfg.DefaultRegion`. This is why the fix must be at the call sites in
  `internal/doctor/doctor.go` and `main.go` — `ResolveProfile`/`ResolveRegion`
  themselves are correct and used verbatim by every other command; only
  `doctor`'s callers fail to pass the env name through. Do not modify
  `config.go`.

- `internal/doctor/doctor_test.go` (107 lines) — existing test pattern for
  `checkRegion`/`checkProfile` uses this helper (lines 62–67):
  ```go
  func overrideHomeForDoctorTest(t *testing.T, dir string) {
      t.Helper()
      orig := os.Getenv("HOME")
      os.Setenv("HOME", dir)
      t.Cleanup(func() { os.Setenv("HOME", orig) })
  }
  ```
  and calls it like:
  ```go
  func TestCheckRegion(t *testing.T) {
      tmpDir := t.TempDir()
      overrideHomeForDoctorTest(t, tmpDir)
      os.Unsetenv("AWS_REGION")
      os.Unsetenv("AWS_DEFAULT_REGION")

      r := checkRegion("")
      ...
  }
  ```
  Your new tests must follow this exact pattern: `t.TempDir()`, call
  `overrideHomeForDoctorTest`, write a `.act.json` into that temp dir with
  `os.WriteFile(filepath.Join(tmpDir, ".act.json"), []byte(...), 0600)`,
  unset relevant `AWS_*` env vars, then call the function under test.

- `main.go`'s `printDoctorHelp()` (lines 496–522) — current text (verified
  by reading the file directly):
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
  Its "Global Flags" block lists only `--profile`/`--region`, omitting
  `--env`. Every other help function includes `--env` there, e.g.
  `printEC2Help` (lines 275–295):
  ```go
  Global Flags:
    --profile    AWS profile to use
    --region     AWS region to use
    --env        Environment name
  ```
  Match that exact three-line format (same label column width as the
  surrounding flags list in `printDoctorHelp`, i.e. keep the existing
  `--profile`/`--region` column alignment and add a `--env` line aligned the
  same way other help blocks in this file do it — see `printEC2Help`,
  `printForwardHelp`, `printECSHelp` for the pattern of aligning `--env` under
  the widest label in that specific block).

- README.md — checked with `grep -n -i "doctor" README.md` and by reading
  the "Global Flags" table (lines 94–101) and the Examples section (lines
  193–204). The README has ONE global "Global Flags" table (lines 94–101)
  that already lists `--profile`, `--region`, `--env`, `--version` — it is
  not per-command, so it already covers `doctor` and needs no change. The
  Examples section's `doctor` lines (193–200) show `act doctor`, `act doctor
  --fix`, `act doctor --fix --skip-confirm` with no explicit global-flags
  list for `doctor` specifically. There is no README text that says or
  implies `doctor` doesn't support `--env`, so per CLAUDE.md's doc-sync rule
  (only update docs when there's an actual behavior/flag list to update),
  **no README change is required for this plan**. Do not touch README.md.

## Commands you will need

| Purpose        | Command                                | Expected on success        |
|----------------|-----------------------------------------|-----------------------------|
| Build          | `go build -v ./...`                    | exit 0, no output          |
| Vet            | `go vet ./...`                         | exit 0, no output          |
| Format check   | `gofmt -l .`                           | exit 0, no output (no files listed) |
| Tests          | `go test -v ./...`                     | all pass                   |
| Doctor tests only | `go test -v ./internal/doctor/...`  | all pass                   |

(All verified working in this repo during recon — `go build -v ./...`,
`go vet ./...`, and `gofmt -l .` all currently produce no output.)

## Scope

**In scope** (the only files you should modify):
- `main.go` — the `case "doctor":` dispatch block and `printDoctorHelp()`
- `internal/doctor/doctor.go` — `Run`, `checkCredentials`, `checkRegion`, `checkProfile`
- `internal/doctor/doctor_test.go` — new/updated tests

**Out of scope** (do NOT touch, even though they look related):
- `internal/config/config.go` — `ResolveProfile`/`ResolveRegion` are already
  correct and used by every other command unmodified; this bug is entirely
  in the call sites, not in `config`.
- `README.md` — no behavior/flag-list text needs updating (see "Current
  state" above); do not add speculative doctor-specific flag documentation.
- Any other `case` block in `main.go`'s switch (`ec2`, `forward`, `ecs`,
  `ssm`, `rds`, `fav`, `env`, `init`, `upgrade`) — they already resolve `env`
  correctly; changing them is unrelated to this bug.
- `checkAWSCLI`, `checkSessionManagerPlugin`, `checkConfigFile`,
  `checkVersion` in `doctor.go` — none of these take profile/region and are
  unaffected by this bug.

## Git workflow

- Branch: `advisor/021-fix-doctor-env-flag`
- Commit per logical unit (e.g. one commit for the `doctor.go` + `main.go`
  fix, one for tests — or combine into one commit if you prefer; either is
  fine as long as each commit builds and passes tests).
- Message style: Conventional Commits, matching repo history, e.g.
  `fix: let act doctor run without AWS CLI already installed` (from
  `git log --oneline -10`). Use a `fix:` prefix, e.g.
  `fix: thread --env through act doctor`.
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Create the branch

```
git checkout -b advisor/021-fix-doctor-env-flag
```

**Verify**: `git branch --show-current` → `advisor/021-fix-doctor-env-flag`

### Step 2: Thread `env` through `doctor.Run` and its three helpers

In `internal/doctor/doctor.go`:

1. Change the `Run` signature (line 49) from:
   ```go
   func Run(profile, region, version string, fix, skipConfirm bool) error {
   ```
   to:
   ```go
   func Run(profile, region, env, version string, fix, skipConfirm bool) error {
   ```
   (Insert `env` right after `region` to match the order `main.go` already
   has the variables in scope — `profile, region, env` — minimizing the
   diff at the call site in step 3.)

2. Update the results slice inside `Run` (lines 52–56) from:
   ```go
   checkCredentials(profile, region),
   checkRegion(region),
   checkProfile(profile),
   ```
   to:
   ```go
   checkCredentials(profile, region, env),
   checkRegion(region, env),
   checkProfile(profile, env),
   ```

3. Change `checkCredentials` (lines 149–151) from:
   ```go
   func checkCredentials(profile, region string) result {
       resolvedProfile := config.ResolveProfile(profile, "")
       resolvedRegion := config.ResolveRegion(region, "")
   ```
   to:
   ```go
   func checkCredentials(profile, region, env string) result {
       resolvedProfile := config.ResolveProfile(profile, env)
       resolvedRegion := config.ResolveRegion(region, env)
   ```

4. Change `checkRegion` (lines 189–190) from:
   ```go
   func checkRegion(flagRegion string) result {
       resolved := config.ResolveRegion(flagRegion, "")
   ```
   to:
   ```go
   func checkRegion(flagRegion, env string) result {
       resolved := config.ResolveRegion(flagRegion, env)
   ```

5. Change `checkProfile` (lines 205–206) from:
   ```go
   func checkProfile(flagProfile string) result {
       resolved := config.ResolveProfile(flagProfile, "")
   ```
   to:
   ```go
   func checkProfile(flagProfile, env string) result {
       resolved := config.ResolveProfile(flagProfile, env)
   ```

**Verify**: `go build ./internal/doctor/...` fails right now (expected —
`main.go`'s call to `doctor.Run` hasn't been updated yet, so the whole
package won't build until step 3). Do not treat this failure as a problem;
proceed directly to step 3, then verify both together.

### Step 3: Pass `env` from `main.go`'s doctor dispatch

In `main.go`, change the `case "doctor":` block (lines 152–176). Replace the
single line:
```go
doctor.Run(profile, region, version, fix, skipConfirm)
```
with:
```go
doctor.Run(profile, region, env, version, fix, skipConfirm)
```
Do not change anything else in this block — `env` is already declared and
populated earlier in `main()` (line 24, 27), so no new variable or parsing
logic is needed.

**Verify**: `go build -v ./...` → exit 0, no output.

### Step 4: Add `--env` to `printDoctorHelp()`'s Global Flags block

In `main.go`, in `printDoctorHelp()` (around line 513), change:
```go
Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use

Examples:
```
to:
```go
Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
```
(Matches the exact wording/format used in `printEC2Help`, `printForwardHelp`,
etc.)

**Verify**: `go build -v ./...` → exit 0. Then `./act doctor help 2>&1 |
grep -- '--env'` (after building the binary per Test plan below) → prints
the `--env` line.

### Step 5: Update existing `checkRegion`/`checkProfile` test call sites

`internal/doctor/doctor_test.go`'s `TestCheckRegion` (lines 69–87) and
`TestCheckProfile` (lines 89–106) call `checkRegion("")` / `checkRegion(
"us-west-2")` and `checkProfile("")` / `checkProfile("my-profile")` with only
one argument. After step 2 these functions take two arguments. Update every
call site in this file to pass an empty env name to preserve the existing
"no --env given" test cases:
```go
r := checkRegion("", "")
...
r = checkRegion("us-west-2", "")
```
and
```go
r := checkProfile("", "")
...
r = checkProfile("my-profile", "")
```

**Verify**: `go build ./internal/doctor/...` → exit 0 (package now compiles
standalone). `go vet ./internal/doctor/...` → exit 0.

## Test plan

Add new test functions to `internal/doctor/doctor_test.go`, following the
exact structural pattern of `TestCheckRegion`/`TestCheckProfile` (temp dir +
`overrideHomeForDoctorTest` + unset relevant `AWS_*` vars + write a
`.act.json` + call the function + assert).

1. **`TestCheckRegionWithEnv`** — covers the actual regression: a named
   environment's region must be used when `env` is passed and no `--region`
   flag is given.
   ```go
   func TestCheckRegionWithEnv(t *testing.T) {
       tmpDir := t.TempDir()
       overrideHomeForDoctorTest(t, tmpDir)
       os.Unsetenv("AWS_REGION")
       os.Unsetenv("AWS_DEFAULT_REGION")

       cfgJSON := `{"environments": {"staging": {"profile": "stagingprof", "region": "eu-west-1"}}}`
       if err := os.WriteFile(filepath.Join(tmpDir, ".act.json"), []byte(cfgJSON), 0600); err != nil {
           t.Fatalf("failed to write test config: %v", err)
       }

       r := checkRegion("", "staging")
       if r.Status != statusPass {
           t.Errorf("expected statusPass when env has a region, got %v (%s)", r.Status, r.Detail)
       }
       if r.Detail != "eu-west-1" {
           t.Errorf("expected Detail 'eu-west-1', got %q", r.Detail)
       }
   }
   ```
   (Add `"path/filepath"` to the file's import block if not already present
   — check the current imports at the top of `doctor_test.go` first.)

2. **`TestCheckProfileWithEnv`** — same shape, for profile:
   ```go
   func TestCheckProfileWithEnv(t *testing.T) {
       tmpDir := t.TempDir()
       overrideHomeForDoctorTest(t, tmpDir)
       os.Unsetenv("AWS_PROFILE")

       cfgJSON := `{"environments": {"staging": {"profile": "stagingprof", "region": "eu-west-1"}}}`
       if err := os.WriteFile(filepath.Join(tmpDir, ".act.json"), []byte(cfgJSON), 0600); err != nil {
           t.Fatalf("failed to write test config: %v", err)
       }

       r := checkProfile("", "staging")
       if r.Status != statusPass {
           t.Errorf("expected statusPass when env has a profile, got %v (%s)", r.Status, r.Detail)
       }
       if r.Detail != "stagingprof" {
           t.Errorf("expected Detail 'stagingprof', got %q", r.Detail)
       }
   }
   ```

3. **Explicit `--region`/`--profile` flag still wins over `--env`** — add
   this case to (or alongside) the tests above to lock in the precedence
   documented in `config.ResolveRegion`/`ResolveProfile` (CLI flag beats env
   lookup):
   ```go
   r := checkRegion("us-east-2", "staging")
   if r.Detail != "us-east-2" {
       t.Errorf("expected explicit --region to win over --env, got %q", r.Detail)
   }
   ```
   (Same pattern for `checkProfile("explicit-profile", "staging")` expecting
   `"explicit-profile"`.)

Verification: `go test -v ./internal/doctor/...` → all tests pass, including
the new ones (look for `--- PASS: TestCheckRegionWithEnv`, `--- PASS:
TestCheckProfileWithEnv` in output). Then `go test -v ./...` → all pass
repo-wide.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0
- [ ] `gofmt -l .` prints nothing
- [ ] `go test -v ./...` exits 0; `TestCheckRegionWithEnv` and
      `TestCheckProfileWithEnv` exist in `internal/doctor/doctor_test.go` and pass
- [ ] `grep -n 'ResolveProfile(profile, "")\|ResolveRegion(region, "")\|ResolveRegion(flagRegion, "")\|ResolveProfile(flagProfile, "")' internal/doctor/doctor.go` returns no matches (confirms the hardcoded `""` bug sites are gone)
- [ ] `grep -n 'doctor.Run(profile, region, version' main.go` returns no matches (confirms the call site was updated)
- [ ] Manual repro from the bug report no longer reproduces the bug:
      ```
      go build -o /tmp/act_verify021 .
      TESTHOME=$(mktemp -d)
      cat > "$TESTHOME/.act.json" <<'EOF'
      {"default_profile": "defaultprof", "default_region": "us-east-1", "environments": {"staging": {"profile": "stagingprof", "region": "eu-west-1"}}}
      EOF
      HOME="$TESTHOME" AWS_PROFILE=shellprof AWS_REGION=shellregion /tmp/act_verify021 --env staging doctor 2>&1 | grep -i "profile\|region"
      rm -f /tmp/act_verify021; rm -rf "$TESTHOME"
      ```
      Expected: the `Region:` line shows `eu-west-1` and the `Profile:` line
      shows `stagingprof` — NOT `shellregion`/`shellprof`.
- [ ] `act doctor help` output includes an `--env` line under "Global Flags"
      (verify with `go build -o /tmp/act_verify021_help . && /tmp/act_verify021_help doctor help 2>&1 | grep -- '--env'; rm -f /tmp/act_verify021_help`)
- [ ] No files outside the in-scope list are modified (`git status`):
      only `main.go`, `internal/doctor/doctor.go`,
      `internal/doctor/doctor_test.go` should show as changed.
- [ ] `plans/README.md` status row for plan 021 updated (unless a reviewer
      told you they own the index)

## STOP conditions

Stop and report back (do not improvise) if:

- The code at any of the "Current state" line numbers/excerpts above doesn't
  match what you find in the live files (the codebase drifted since this
  plan was written at commit `aa50614`).
- `doctor.Run`'s call site in `main.go` is not the single line
  `doctor.Run(profile, region, version, fix, skipConfirm)` you expect — e.g.
  if `doctorArgs` (currently unused/dead — collected at line 160/168 but
  never passed to `doctor.Run`) has since been wired up to something, treat
  that as a sign the function's parameter list has changed more than this
  plan assumes, and report back rather than guessing how to merge your `env`
  parameter in.
- Any verification command in "Done criteria" fails twice after a
  reasonable fix attempt.
- Fixing this bug appears to require changing `config.ResolveProfile` or
  `config.ResolveRegion` themselves — it shouldn't; if it does, the codebase
  has drifted from the assumptions in this plan.
- You discover `env` is not actually in scope inside `main()`'s `case
  "doctor":` block (e.g. it was renamed or removed) — report back instead of
  reintroducing global-flag parsing logic.

## Maintenance notes

- If a future `doctor` check is added that needs profile/region (beyond
  `checkCredentials`/`checkRegion`/`checkProfile`), it must also take an
  `env` parameter and call `config.ResolveProfile`/`ResolveRegion` with it —
  don't let a new check reintroduce the same hardcoded-`""` bug this plan
  fixes.
- A reviewer should scrutinize: (1) that `doctor.Run`'s new `env` parameter
  is placed consistently with how `main.go` already has the variable ordered
  in scope, to keep the diff minimal and readable; (2) that the new tests
  actually reset `HOME` and unset `AWS_PROFILE`/`AWS_REGION`/
  `AWS_DEFAULT_REGION` before asserting on resolution — a leaked env var from
  a prior test or the CI shell would make these tests flaky/wrong, which is
  exactly the class of bug this plan is fixing at the product level.
- Out of scope, deferred: this plan does not add a warning/error when
  `--env <name>` is given but `<name>` isn't found in
  `~/.act.json`'s `environments` map (today `ResolveProfile`/`ResolveRegion`
  silently fall through to the next resolution tier in that case, for every
  command, not just `doctor`). That's a separate, repo-wide behavior
  question and was intentionally left out of this plan's scope, which is
  narrowly "`doctor` ignores `--env` entirely."
