# Plan 034: Add `act env use <name>` to set a persistent default named environment

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/config/config.go internal/config/config_test.go main.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: direction (this is a feature addition grounded in a real
  asymmetry in the existing config schema, not a bug — but the scope is small,
  self-contained, and low-risk, so unlike a typical direction/spike plan this
  one specifies a full bounded happy-path implementation rather than stopping
  at "investigate options")
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`act env` already has full CRUD (`list`/`add`/`rm`) for named environments
(profile+region presets), but there is no way to mark one of them as "the one
I'm currently working in." Every single invocation against a named
environment requires repeating `--env staging` explicitly — `act ec2`,
`act ecs`, `act fav`, etc. all need the flag every time. A user managing 2+
named environments (exactly the workflow `env` CRUD was built for) has to
type `--env <name>` on every command in a session, even though the config
schema already has an unnamed-case precedent for this
(`DefaultProfile`/`DefaultRegion` fields, settable via `act init`). This plan
adds `act env use <name>`, which persists a "current default environment" to
`~/.act.json` and wires it into the profile/region resolution fallback chain,
so that omitting `--env` on a later command still resolves to that
environment's profile/region — without changing what an explicit `--env`
flag does.

## Current state

- `internal/config/config.go` (163 lines) — config schema and resolution
  logic. Read the whole file before editing.

  Current `Environment`/`Config` structs (lines 10–20):
  ```go
  type Environment struct {
      Profile string `json:"profile"`
      Region  string `json:"region"`
  }

  type Config struct {
      DefaultProfile string                 `json:"default_profile,omitempty"`
      DefaultRegion  string                 `json:"default_region,omitempty"`
      Favorites      []string               `json:"favorites,omitempty"`
      Environments   map[string]Environment `json:"environments,omitempty"`
  }
  ```

  Current `RemoveEnvironment` (lines 122–126):
  ```go
  func RemoveEnvironment(name string) error {
      cfg := Load()
      delete(cfg.Environments, name)
      return Save(cfg)
  }
  ```

  Current `ResolveProfile` (lines 128–143):
  ```go
  func ResolveProfile(flagValue, envName string) string {
      if flagValue != "" {
          return flagValue
      }
      if envName != "" {
          cfg := Load()
          if env, ok := cfg.Environments[envName]; ok && env.Profile != "" {
              return env.Profile
          }
      }
      if env := os.Getenv("AWS_PROFILE"); env != "" {
          return env
      }
      cfg := Load()
      return cfg.DefaultProfile
  }
  ```

  Current `ResolveRegion` (lines 145–163) — same shape, plus an extra
  `AWS_DEFAULT_REGION` fallback after `AWS_REGION`:
  ```go
  func ResolveRegion(flagValue, envName string) string {
      if flagValue != "" {
          return flagValue
      }
      if envName != "" {
          cfg := Load()
          if env, ok := cfg.Environments[envName]; ok && env.Region != "" {
              return env.Region
          }
      }
      if env := os.Getenv("AWS_REGION"); env != "" {
          return env
      }
      if env := os.Getenv("AWS_DEFAULT_REGION"); env != "" {
          return env
      }
      cfg := Load()
      return cfg.DefaultRegion
  }
  ```

  **Decided priority order** (you must implement exactly this, in both
  functions): CLI flag (`--profile`/`--region`) > explicit `--env <name>`
  lookup > `DefaultEnvironment` lookup (new tier) > `AWS_PROFILE`/
  `AWS_REGION`/`AWS_DEFAULT_REGION` env vars > bare `DefaultProfile`/
  `DefaultRegion`.

  Rationale: an explicit `--env NAME` on the command line is always more
  specific than a persisted "current" environment, so it must be checked
  first (this preserves 100% of existing behavior — it is unaffected by this
  plan). The new `DefaultEnvironment` tier sits *between* the explicit
  `--env` lookup and the shell env vars: `AWS_PROFILE`/`AWS_REGION` are
  ambient/global to the shell and could easily be stale or belong to an
  unrelated tool invocation, whereas `act env use` is an explicit, intentional
  action the user took specifically to configure `act`'s default. A more
  deliberate `act`-specific setting should win over an ambient shell
  variable. `DefaultProfile`/`DefaultRegion` (set via `act init`) remain the
  final fallback, unchanged.

- `main.go` — read `printEnvHelp()` (lines 1169–1184) and `runEnv()`
  (lines 1186–1242) in full.

  Current `printEnvHelp()`:
  ```go
  func printEnvHelp() {
      fmt.Fprintf(os.Stderr, `act env - Manage named environments (profile + region presets)

  Usage: act env [subcommand]

  Subcommands:
    list                                       List configured environments
    add <name>                                 Add or update an environment (use global --profile/--region flags)
    rm <name>                                  Remove an environment

  Examples:
    act env list
    act --profile production --region us-west-2 env add prod
    act env rm prod
  `)
  }
  ```

  Current `runEnv()` switch cases (`list`, `add`, `rm`, `default` at
  lines 1192–1241) — `list` currently prints:
  ```go
  fmt.Printf("%s: profile=%s region=%s\n", name, e.Profile, e.Region)
  ```
  with no indication of which environment (if any) is the default.

  `runEnv` is called from the dispatch switch at `main.go:143–150`:
  ```go
  case "env":
      subArgs := args[1:]
      if hasHelp(subArgs) {
          printEnvHelp()
          os.Exit(0)
      }
      runEnv(subArgs, profile, region)
      os.Exit(0)
  ```
  Note `runEnv` is passed the raw `profile`/`region` flag values (not
  resolved), consistent with `add` needing the literal flag values to store.
  This call site does not need to change.

- `internal/config/config_test.go` (232 lines) — read the whole file. Test
  isolation pattern used throughout: `tmpDir := t.TempDir()` +
  `overrideHome(t, tmpDir)` (helper at lines 12–22, sets `HOME` — and
  `USERPROFILE` on Windows — to `tmpDir` for the duration of the test, with
  `t.Cleanup` restoring it), then `Init(...)` or direct `os.WriteFile` of a
  JSON config, then calling the function under test. `TestEnvironmentCRUD`
  (lines 161–199) is the closest structural match for the new
  `SetDefaultEnvironment` tests. `TestResolveWithEnvironment` (lines 64–103)
  is the closest match for the new resolver-fallback-tier tests.

- `README.md` — "Named environments" example block (lines 185–188):
  ```
  # Named environments
  act env list                                            # list environments
  act --profile production --region us-west-2 env add prod
  act env rm prod
  ```

  "Configuration" section (lines 242–268), JSON example (lines 246–262) and
  resolution-order list (lines 264–268):
  ```json
  {
    "default_profile": "production",
    "default_region": "ap-southeast-2",
    "favorites": ["i-0123456789abcdef0"],
    "environments": {
      "prod": {
        "profile": "production",
        "region": "ap-southeast-2"
      },
      "staging": {
        "profile": "staging",
        "region": "us-west-2"
      }
    }
  }
  ```
  ```
  Resolution order for profile/region:
  1. CLI flag (`--profile`, `--region`)
  2. Environment lookup (`--env` name in config)
  3. Environment variable (`AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`)
  4. Config file defaults (`~/.act.json`)
  ```
  This numbered list must be updated to insert the new tier at position 3
  (renumbering the env-var and config-defaults tiers to 4 and 5).

- Repo convention for new config mutators: single-purpose functions that
  `Load()`, mutate, then `Save(cfg)` — e.g. `AddFavorite`
  (`internal/config/config.go:80–89`) and `AddEnvironment`
  (`internal/config/config.go:113–120`). Match this shape exactly for the new
  `SetDefaultEnvironment` function.

## Commands you will need

| Purpose        | Command                              | Expected on success        |
|----------------|---------------------------------------|-----------------------------|
| Build          | `go build -v ./...`                   | exit 0                      |
| Vet            | `go vet ./...`                        | exit 0, no output           |
| Format check   | `gofmt -l .`                          | exit 0, no file names printed |
| Test           | `go test -v ./...`                    | exit 0, all tests pass, incl. new ones |
| Manual smoke   | `go run . env use <name>` (from a temp `HOME`) | see Step 5 |

## Scope

**In scope** (the only files you should modify):
- `internal/config/config.go`
- `internal/config/config_test.go`
- `main.go` (only `runEnv` and `printEnvHelp`)
- `README.md` (only the "Named environments" example block and the
  "Configuration" section)

**Out of scope** (do NOT touch, even though related):
- `parseGlobalFlags` in `main.go` — this plan adds a new *fallback* used only
  when `--env` is absent; it does not change how the `--env` flag itself is
  parsed.
- Any other subcommand's dispatch code (`ec2`, `ecs`, `forward`, `ssm`,
  `rds`, `fav`, `doctor`, `init`, `upgrade`) — they already call
  `config.ResolveProfile(profile, env)` / `config.ResolveRegion(region, env)`
  unconditionally, so the new fallback tier takes effect for them
  automatically once `ResolveProfile`/`ResolveRegion` are updated. Do not
  add special-casing per subcommand.
- `internal/doctor/`, `internal/aws/`, `internal/tui/`, `internal/updater/` —
  unrelated to this change.
- `plans/README.md` — a separate process owns this index file; only update
  your own plan's status row if a reviewer explicitly asks you to, per the
  banner at the top of this file.

## Git workflow

- Branch: `advisor/034-add-env-use-default-selector`
- Conventional Commits style, matching prior env/fav work — see
  `git log --oneline` for `4aa52f1` ("feat: add fav list and env list/add/rm
  commands") and `434db35` ("feat: add environments, favorites CRUD, and init
  to config package"). Use a `feat:` prefix for this change, e.g.:
  `feat: add act env use to set a persistent default environment`
- Commit per logical step (config layer, then CLI layer, then docs), or as a
  single commit if you prefer — match the granularity of `4aa52f1` (one
  commit spanning config + main.go + README + tests).
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Add `DefaultEnvironment` field to `Config`

In `internal/config/config.go`, add a new field to the `Config` struct
(after `DefaultRegion`, before `Favorites`, to group it with the other
"default" fields):

```go
type Config struct {
    DefaultProfile     string                 `json:"default_profile,omitempty"`
    DefaultRegion      string                 `json:"default_region,omitempty"`
    DefaultEnvironment string                 `json:"default_environment,omitempty"`
    Favorites          []string               `json:"favorites,omitempty"`
    Environments       map[string]Environment `json:"environments,omitempty"`
}
```

**Verify**: `go build -v ./...` → exit 0.

### Step 2: Add `SetDefaultEnvironment` and update `RemoveEnvironment`

In `internal/config/config.go`, add a new exported function right after
`AddEnvironment` (after line 120):

```go
func SetDefaultEnvironment(name string) error {
    cfg := Load()
    if _, ok := cfg.Environments[name]; !ok {
        return fmt.Errorf("environment %q not found; run `act env list` to see configured environments", name)
    }
    cfg.DefaultEnvironment = name
    return Save(cfg)
}
```

This requires adding `"fmt"` to the import block at the top of the file
(currently `"os"`, `"path/filepath"`, `"encoding/json"` — add `"fmt"` in
alphabetical/grouped position consistent with `gofmt`'s expectations; run
`gofmt -w internal/config/config.go` after editing to let it sort imports).

Then update `RemoveEnvironment` (lines 122–126) to clear
`DefaultEnvironment` if it points at the environment being removed — this
avoids a dangling reference where `default_environment` names an environment
that no longer exists:

```go
func RemoveEnvironment(name string) error {
    cfg := Load()
    delete(cfg.Environments, name)
    if cfg.DefaultEnvironment == name {
        cfg.DefaultEnvironment = ""
    }
    return Save(cfg)
}
```

**Verify**: `go build -v ./...` → exit 0. `gofmt -l internal/config/config.go`
→ no output.

### Step 3: Extend `ResolveProfile` and `ResolveRegion` with the new fallback tier

In `internal/config/config.go`, update `ResolveProfile` to insert the new
tier between the explicit-`envName` lookup and the `AWS_PROFILE` check:

```go
func ResolveProfile(flagValue, envName string) string {
    if flagValue != "" {
        return flagValue
    }
    if envName != "" {
        cfg := Load()
        if env, ok := cfg.Environments[envName]; ok && env.Profile != "" {
            return env.Profile
        }
    } else {
        cfg := Load()
        if cfg.DefaultEnvironment != "" {
            if env, ok := cfg.Environments[cfg.DefaultEnvironment]; ok && env.Profile != "" {
                return env.Profile
            }
        }
    }
    if env := os.Getenv("AWS_PROFILE"); env != "" {
        return env
    }
    cfg := Load()
    return cfg.DefaultProfile
}
```

Note the `else`: the new tier only applies when `envName == ""` (no explicit
`--env` was passed) — if `envName != ""` but that name isn't found in
`cfg.Environments` (or its `Profile` is empty), execution must fall through
to `AWS_PROFILE`/`DefaultProfile` exactly as it does today, NOT consult
`DefaultEnvironment`. This is why the new tier is in an `else` branch rather
than a second independent `if`.

Apply the identical structural change to `ResolveRegion` (same `else`
pattern, using `env.Region` and `cfg.DefaultRegion`; the existing
`AWS_REGION`/`AWS_DEFAULT_REGION` checks stay after the new tier, unchanged).

**Verify**: `go build -v ./...` → exit 0.

### Step 4: Add `act env use <name>` subcommand and update help text

In `main.go`, update `printEnvHelp()` to document the new subcommand:

```go
func printEnvHelp() {
	fmt.Fprintf(os.Stderr, `act env - Manage named environments (profile + region presets)

Usage: act env [subcommand]

Subcommands:
  list                                       List configured environments
  add <name>                                 Add or update an environment (use global --profile/--region flags)
  rm <name>                                  Remove an environment
  use <name>                                 Set an environment as the default (used when --env is omitted)

Examples:
  act env list
  act --profile production --region us-west-2 env add prod
  act env rm prod
  act env use prod
`)
}
```

In `runEnv()`, add a new `case "use":` in the switch statement — insert it
after the existing `case "rm":` block (before `default:`), following the
exact validation/error-reporting shape of the `rm` case:

```go
case "use":
    if len(subArgs) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: act env use <name>\n")
        os.Exit(1)
    }
    name := subArgs[1]
    if err := config.SetDefaultEnvironment(name); err != nil {
        fmt.Fprintf(os.Stderr, "Error setting default environment: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("Environment %q is now the default (used when --env is omitted).\n", name)
```

Also update the `list` case's output to mark which environment is currently
the default. Change the print line inside the `list` case from:

```go
fmt.Printf("%s: profile=%s region=%s\n", name, e.Profile, e.Region)
```

to append a `(current)` suffix when the entry matches the loaded config's
`DefaultEnvironment`. Load the config once before the loop and check inside
it:

```go
case "list":
    envs := config.ListEnvironments()
    if len(envs) == 0 {
        fmt.Println("No environments configured.")
        return
    }
    cfg := config.Load()
    names := make([]string, 0, len(envs))
    for name := range envs {
        names = append(names, name)
    }
    sort.Strings(names)
    for _, name := range names {
        e := envs[name]
        marker := ""
        if name == cfg.DefaultEnvironment {
            marker = " (current)"
        }
        fmt.Printf("%s: profile=%s region=%s%s\n", name, e.Profile, e.Region, marker)
    }
```

**Verify**: `go build -v ./...` → exit 0. `gofmt -l main.go` → no output.

### Step 5: Manual smoke test

Run these commands from a scratch `HOME` to confirm end-to-end behavior
without touching your real `~/.act.json`:

```bash
export TESTHOME=$(mktemp -d)
HOME=$TESTHOME go run . init <<< $'test-profile\nus-east-1\n'   # or however `act init` prompts; check printInitHelp/runInit if unsure of prompt order
HOME=$TESTHOME go run . --profile prod-profile --region ap-southeast-2 env add prod
HOME=$TESTHOME go run . env list
# Expected: "prod: profile=prod-profile region=ap-southeast-2" with NO "(current)" suffix yet
HOME=$TESTHOME go run . env use prod
# Expected: 'Environment "prod" is now the default (used when --env is omitted).'
HOME=$TESTHOME go run . env list
# Expected: "prod: profile=prod-profile region=ap-southeast-2 (current)"
HOME=$TESTHOME go run . env use nonexistent
# Expected: exit 1, stderr: Error setting default environment: environment "nonexistent" not found; run `act env list` to see configured environments
HOME=$TESTHOME go run . env rm prod
HOME=$TESTHOME go run . env list
# Expected: "No environments configured." (default_environment was cleared, no dangling reference)
```

If `act init`'s actual prompt sequence differs from the placeholder above,
read `printInitHelp`/`runInit` in `main.go` first to get the real prompts —
do not guess.

**Verify**: each command's output matches the expected text shown above
exactly (aside from the profile/region values you chose).

## Test plan

Add the following to `internal/config/config_test.go`, modeled on
`TestEnvironmentCRUD` (lines 161–199) and `TestResolveWithEnvironment`
(lines 64–103) for setup/style:

1. `TestSetDefaultEnvironment` — using `t.TempDir()` + `overrideHome(t, tmpDir)`
   + `Init(...)`:
   - Call `SetDefaultEnvironment("prod")` before "prod" exists in
     `cfg.Environments` → expect a non-nil error whose message contains
     `"prod"` and `"not found"`.
   - Call `AddEnvironment("prod", "prod-profile", "us-west-2")`, then
     `SetDefaultEnvironment("prod")` → expect `nil` error, then
     `Load().DefaultEnvironment == "prod"`.

2. `TestRemoveEnvironmentClearsDefault` — new test:
   - `AddEnvironment("prod", ...)`, `SetDefaultEnvironment("prod")`, confirm
     `Load().DefaultEnvironment == "prod"`.
   - `RemoveEnvironment("prod")` → confirm `Load().DefaultEnvironment == ""`
     (dangling reference cleared).
   - Separately: add two environments "prod" and "staging", set default to
     "staging", remove "prod" → confirm `Load().DefaultEnvironment` is still
     `"staging"` (removing a *different* environment must not clear an
     unrelated default).

3. Extend `TestResolveWithEnvironment` (or add a new
   `TestResolveWithDefaultEnvironment` test) covering the new fallback tier:
   - Write a config JSON with `"default_environment": "staging"` and both
     `"prod"`/`"staging"` entries (same shape as the existing JSON literal at
     lines 69–76, plus the new field).
   - `ResolveProfile("", "")` (no flag, no `--env`) → expect
     `"staging-profile"` (falls through to the default-environment tier).
   - `ResolveRegion("", "")` → expect `"eu-west-1"` (staging's region in the
     existing fixture).
   - `ResolveProfile("", "prod")` (explicit `--env` given) → expect
     `"prod-profile"`, confirming explicit `--env` still overrides
     `default_environment`.
   - `ResolveProfile("flag-prof", "")` → expect `"flag-prof"`, confirming the
     CLI flag still wins over everything.
   - With `AWS_PROFILE` set in the environment AND `default_environment` set
     to a valid entry, confirm `ResolveProfile("", "")` returns the
     *environment's* profile, not the env var — this is the specific
     ordering decision from "Current state" and must be asserted explicitly,
     not just implied by the other cases.

- Verification: `go test -v ./internal/config/... ` → all pass, including
  the new tests listed above. Then `go test -v ./...` → all pass repo-wide.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` exits 0 with no file names printed
- [ ] `go test -v ./...` exits 0; new tests `TestSetDefaultEnvironment`,
      `TestRemoveEnvironmentClearsDefault`, and the default-environment
      resolver assertions all exist and pass
- [ ] `grep -n "DefaultEnvironment" internal/config/config.go` shows the new
      struct field, its use in `SetDefaultEnvironment`, `RemoveEnvironment`,
      `ResolveProfile`, and `ResolveRegion`
- [ ] `grep -n '"use"' main.go` shows the new `case "use":` inside `runEnv`
- [ ] `act env list` output shows a `(current)` suffix next to the default
      environment (confirmed via the Step 5 manual smoke test)
- [ ] README.md's "Named environments" example block includes an
      `act env use <name>` line, and the "Configuration" section's JSON
      example and resolution-order list both mention `default_environment`
- [ ] No files outside the in-scope list are modified (`git status`)

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/config/config.go` or `main.go`'s `runEnv`/
  `printEnvHelp` doesn't match the excerpts in "Current state" (the codebase
  has drifted since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- The fix appears to require touching `parseGlobalFlags` or any subcommand's
  dispatch code beyond `runEnv`/`printEnvHelp` — that would mean the
  fallback isn't as self-contained as assumed, and the priority-order
  decision in "Current state" needs to be revisited before continuing.
- You discover that `ResolveProfile`/`ResolveRegion` are called anywhere
  with a non-empty `envName` that is expected to fall through to
  `DefaultEnvironment` on a miss (i.e. some caller wants "try this env name,
  then try the default env, then env vars") — this plan's design assumes
  explicit `--env` and `DefaultEnvironment` are mutually exclusive tiers
  (`if envName != "" { ... } else { check DefaultEnvironment }`), not a
  chain. If you find a caller that needs chaining, stop and report rather
  than silently changing the tier structure.

## Maintenance notes

- If a future `act env rename <old> <new>` command is added, it must also
  update `DefaultEnvironment` if it matches `<old>` — the same dangling-
  reference concern that motivated `RemoveEnvironment`'s change in Step 2
  applies there too.
- A reviewer should check that the `else` branch structure in Step 3 (not a
  second independent `if`) is preserved — using two independent `if`
  statements instead of `if/else` would make an explicit `--env <name>` that
  misses the environment map silently fall through to checking
  `DefaultEnvironment`, which is NOT the intended behavior (explicit `--env`
  with an unknown name should behave exactly as it does today: fall through
  to env vars / `DefaultProfile`, not to `DefaultEnvironment`).
- This plan does not add any interactive picker or TUI surface for choosing
  the default environment — `env use <name>` is deliberately a plain
  non-interactive CLI command, consistent with `env add`/`env rm`. A
  follow-up could add an interactive `act env` (no subcommand) picker, but
  that's out of scope here.
