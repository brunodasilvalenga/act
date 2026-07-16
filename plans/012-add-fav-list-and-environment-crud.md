# Plan 012: Add `act fav list` and `act env` CRUD subcommands

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- main.go internal/config/config.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: direction
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`act fav` supports `add`/`rm` but has no way to list favorites as plain text
— the only way to see them is the interactive TUI picker triggered by
running `act fav` with no arguments, which requires a real TTY and isn't
scriptable. Separately, `~/.act.json`'s `environments` map (named
profile+region presets used via `--env`) has NO command-line management at
all — `internal/config/config.go` has `AddFavorite`/`RemoveFavorite` but no
equivalent for environments; the only way to add or remove a named
environment today is to hand-edit the JSON file directly. This is a real gap
for a tool whose core pitch is fast switching between named AWS
profiles/regions: onboarding a teammate to a new environment, or scripting
against the favorites list (e.g. `act fav list | fzf`), currently has no
supported non-interactive path.

This plan adds: `act fav list` (prints favorites, one per line, to stdout)
and `act env list` / `act env add <name> --profile <p> --region <r>` /
`act env rm <name>` (mirroring the existing `fav add`/`fav rm` UX pattern
exactly).

## Current state

`internal/config/config.go` (full file relevant sections):

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

```go
func AddFavorite(instanceID string) error {
	cfg := Load()
	for _, f := range cfg.Favorites {
		if f == instanceID {
			return nil // already exists
		}
	}
	cfg.Favorites = append(cfg.Favorites, instanceID)
	return Save(cfg)
}

func RemoveFavorite(instanceID string) error {
	cfg := Load()
	var updated []string
	for _, f := range cfg.Favorites {
		if f != instanceID {
			updated = append(updated, f)
		}
	}
	cfg.Favorites = updated
	return Save(cfg)
}
```

There is no `ListFavorites`, `AddEnvironment`, `RemoveEnvironment`, or
`ListEnvironments` function anywhere in this file (confirmed via
`grep -n "ListEnvironments\|AddEnvironment\|RemoveEnvironment\|ListFavorites" internal/config/config.go main.go`,
which returns no matches).

`main.go:895-955` (full `runFav` function):

```go
func runFav(profile, region string, subArgs []string) {
	if len(subArgs) == 0 {
		// Show picker from favorites
		cfg := config.Load()
		if len(cfg.Favorites) == 0 {
			fmt.Fprintf(os.Stderr, "No favorites configured. Use 'act fav add <instance-id>' to add one.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select Favorite Instance", cfg.Favorites)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}

		err = aws.StartSession(picked, profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch subArgs[0] {
	case "add":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav add <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if !strings.HasPrefix(id, "i-") {
			fmt.Fprintf(os.Stderr, "Error: instance ID must start with 'i-'\n")
			os.Exit(1)
		}
		if err := config.AddFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added %s to favorites.\n", id)

	case "rm":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav rm <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if err := config.RemoveFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed %s from favorites.\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown fav subcommand: %s\n", subArgs[0])
		printFavHelp()
		os.Exit(1)
	}
}
```

`main.go:366-386` (`printFavHelp`, full function):

```go
func printFavHelp() {
	fmt.Fprintf(os.Stderr, `act fav - Manage and connect to favorite instances

Usage: act [global flags] fav [subcommand]

Subcommands:
  (none)       Show favorites picker and connect
  add <id>     Add instance to favorites
  rm <id>      Remove instance from favorites

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act fav
  act fav add i-0123456789abcdef0
  act fav rm i-0123456789abcdef0
`)
}
```

The `main.go` command dispatch switch (`main.go:43-149`) has a `case "fav":`
block (lines 110-118) that calls `runFav`. A new `case "env":` block needs
to be added alongside it — note `env` is NOT currently a reserved subcommand
name; confirm via `grep -n 'case "' main.go` that no existing case uses
`"env"` before adding it (the global `--env` *flag* is unrelated to and
does not conflict with an `env` *subcommand*, since flag parsing happens
separately in `parseGlobalFlags` before subcommand dispatch).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all pass |
| Manual check | `go build -o act . && ./act fav list` (in a temp `~/.act.json` scenario) | prints favorites or "No favorites configured." |
| Manual check | `./act env list` | prints environments or a "no environments configured" message |

## Scope

**In scope** (the only files you should modify):
- `internal/config/config.go` (add `ListFavorites`, `AddEnvironment`,
  `RemoveEnvironment`, `ListEnvironments` functions)
- `internal/config/config_test.go` (add tests for the 4 new functions)
- `main.go` (extend `runFav` with a `list` subcommand; add a new `runEnv`
  function and `case "env":` dispatch entry; add `printEnvHelp`; update
  `printFavHelp` and `printUsage` to document the additions)
- `README.md` (document `act fav list` and the new `act env` command, per
  `CLAUDE.md`'s rule to keep README in sync with `main.go`)

**Out of scope** (do NOT touch, even though they look related):
- Do not change the existing `fav add`/`fav rm` behavior or validation
  (the `i-` prefix check on `fav add` stays exactly as-is).
- Do not add environment *validation* beyond what's structurally necessary
  (e.g. don't validate that a profile name in `env add` actually exists in
  `~/.aws/config` — that's a larger feature, out of scope here).
- Do not change `ResolveProfile`/`ResolveRegion` — this plan only adds
  CRUD/listing for environments, not new resolution logic.
- `internal/doctor/doctor.go` — no changes needed.

## Git workflow

- Branch: `advisor/012-add-fav-list-and-environment-crud`
- Single commit; message style example: `feat: add environments, favorites CRUD, and init to config package` (commit `434db35`, direct precedent for
  this exact kind of change) — for this change:
  `feat: add fav list and env list/add/rm commands`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add `ListFavorites` to `internal/config/config.go`

Add after `RemoveFavorite` (after line 101):

```go
func ListFavorites() []string {
	cfg := Load()
	return cfg.Favorites
}
```

**Verify**: `go build ./internal/config/...` → exit 0.

### Step 2: Add environment CRUD functions to `internal/config/config.go`

Add after `ListFavorites`:

```go
func ListEnvironments() map[string]Environment {
	cfg := Load()
	return cfg.Environments
}

func AddEnvironment(name, profile, region string) error {
	cfg := Load()
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]Environment)
	}
	cfg.Environments[name] = Environment{Profile: profile, Region: region}
	return Save(cfg)
}

func RemoveEnvironment(name string) error {
	cfg := Load()
	delete(cfg.Environments, name)
	return Save(cfg)
}
```

Note `AddEnvironment` overwrites an existing environment of the same name —
this matches the natural "upsert" semantics a user would expect from
`env add prod --profile x --region y` run twice with different values, and
is consistent with `Save`'s existing all-or-nothing config write pattern.
This differs slightly from `AddFavorite`'s "no-op if exists" semantics,
which is correct: favorites are a set of IDs (no attributes to update),
while environments have a profile/region payload a user would legitimately
want to update in place.

**Verify**: `go build ./internal/config/...` → exit 0.

### Step 3: Add tests for the 4 new config functions

Append to `internal/config/config_test.go`:

```go
func TestListFavorites(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	Init("test", "us-east-1")

	if got := ListFavorites(); len(got) != 0 {
		t.Errorf("expected no favorites initially, got %v", got)
	}

	AddFavorite("i-abc")
	AddFavorite("i-def")

	got := ListFavorites()
	want := []string{"i-abc", "i-def"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListFavorites() = %v, want %v", got, want)
	}
}

func TestEnvironmentCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	Init("test", "us-east-1")

	if got := ListEnvironments(); len(got) != 0 {
		t.Errorf("expected no environments initially, got %v", got)
	}

	if err := AddEnvironment("prod", "prod-profile", "us-west-2"); err != nil {
		t.Fatalf("AddEnvironment failed: %v", err)
	}

	envs := ListEnvironments()
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs["prod"].Profile != "prod-profile" || envs["prod"].Region != "us-west-2" {
		t.Errorf("unexpected environment value: %+v", envs["prod"])
	}

	// Upsert: adding again with different values overwrites.
	if err := AddEnvironment("prod", "prod-profile-v2", "eu-west-1"); err != nil {
		t.Fatalf("AddEnvironment (update) failed: %v", err)
	}
	envs = ListEnvironments()
	if envs["prod"].Profile != "prod-profile-v2" || envs["prod"].Region != "eu-west-1" {
		t.Errorf("expected AddEnvironment to overwrite existing entry, got %+v", envs["prod"])
	}

	if err := RemoveEnvironment("prod"); err != nil {
		t.Fatalf("RemoveEnvironment failed: %v", err)
	}
	envs = ListEnvironments()
	if len(envs) != 0 {
		t.Errorf("expected 0 environments after remove, got %d", len(envs))
	}
}
```

Add `"reflect"` to the test file's import block if not already present
(check the existing imports at the top of `config_test.go` — currently
`"os"`, `"path/filepath"`, `"runtime"`, `"testing"`; add `"reflect"`
alongside them).

**Verify**: `go test ./internal/config/... -run 'TestListFavorites|TestEnvironmentCRUD' -v` →
both pass.

### Step 4: Add `fav list` subcommand to `main.go`

In `runFav` (`main.go:895-955`), add a `case "list":` branch to the switch
statement, right before `case "add":`:

```go
	switch subArgs[0] {
	case "list":
		favorites := config.ListFavorites()
		if len(favorites) == 0 {
			fmt.Println("No favorites configured.")
			return
		}
		for _, f := range favorites {
			fmt.Println(f)
		}

	case "add":
		...
```

(Keep the existing `add`/`rm`/`default` cases exactly as they are — only
insert the new `case "list":` block.)

**Verify**: `go build ./...` → exit 0.

### Step 5: Add `runEnv` function and `printEnvHelp` to `main.go`

Add a new function after `runFav` (after line 955), following the exact
structural pattern of `runFav`:

```go
func printEnvHelp() {
	fmt.Fprintf(os.Stderr, `act env - Manage named environments (profile + region presets)

Usage: act env [subcommand]

Subcommands:
  list                            List configured environments
  add <name> --profile P --region R   Add or update an environment
  rm <name>                       Remove an environment

Examples:
  act env list
  act env add prod --profile production --region us-west-2
  act env rm prod
`)
}

func runEnv(subArgs []string) {
	if len(subArgs) == 0 {
		printEnvHelp()
		os.Exit(1)
	}

	switch subArgs[0] {
	case "list":
		envs := config.ListEnvironments()
		if len(envs) == 0 {
			fmt.Println("No environments configured.")
			return
		}
		names := make([]string, 0, len(envs))
		for name := range envs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := envs[name]
			fmt.Printf("%s: profile=%s region=%s\n", name, e.Profile, e.Region)
		}

	case "add":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act env add <name> --profile <profile> --region <region>\n")
			os.Exit(1)
		}
		name := subArgs[1]
		fs := flag.NewFlagSet("env add", flag.ExitOnError)
		envProfile := fs.String("profile", "", "AWS profile for this environment")
		envRegion := fs.String("region", "", "AWS region for this environment")
		fs.Parse(subArgs[2:])

		if *envProfile == "" && *envRegion == "" {
			fmt.Fprintf(os.Stderr, "Error: at least one of --profile or --region is required\n")
			os.Exit(1)
		}
		if err := config.AddEnvironment(name, *envProfile, *envRegion); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding environment: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added environment %q (profile=%s, region=%s).\n", name, *envProfile, *envRegion)

	case "rm":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act env rm <name>\n")
			os.Exit(1)
		}
		name := subArgs[1]
		if err := config.RemoveEnvironment(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing environment: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed environment %q.\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown env subcommand: %s\n", subArgs[0])
		printEnvHelp()
		os.Exit(1)
	}
}
```

This requires adding `"sort"` to `main.go`'s import block (currently
`"bufio"`, `"flag"`, `"fmt"`, `"os"`, `"os/exec"`, `"strings"`, plus the
4 internal packages) — add `"sort"` alongside the standard-library imports,
keeping the existing grouping (standard library imports first, blank line,
then this repo's internal packages).

Note: `AddEnvironment("prod", "prof", "")` (region omitted) is allowed by
this design — a user might want a profile-only or region-only environment
entry, matching how `ResolveProfile`/`ResolveRegion` already independently
fall through per-field (`internal/config/config.go:103-138`) if one of the
two is empty in the environment map.

**Verify**: `go build ./...` → exit 0.

### Step 6: Wire `case "env":` into the dispatch switch

In `main.go`'s main dispatch `switch subcmd { ... }` (lines 43-149), add a
new case after the existing `case "fav":` block (after line 118, before
`case "doctor":`):

```go
	case "env":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printEnvHelp()
			os.Exit(0)
		}
		runEnv(subArgs)
		os.Exit(0)
```

**Verify**: `go build ./...` → exit 0.

### Step 7: Update `printFavHelp` and `printUsage`

In `printFavHelp` (`main.go:366-386`), add `list` to the Subcommands block
and an example:

```go
func printFavHelp() {
	fmt.Fprintf(os.Stderr, `act fav - Manage and connect to favorite instances

Usage: act [global flags] fav [subcommand]

Subcommands:
  (none)       Show favorites picker and connect
  list         List favorites (non-interactive)
  add <id>     Add instance to favorites
  rm <id>      Remove instance from favorites

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act fav
  act fav list
  act fav add i-0123456789abcdef0
  act fav rm i-0123456789abcdef0
`)
}
```

In `printUsage` (`main.go:188-214`), add `env` to the Commands table, after
the `fav` line:

```
Commands:
  ec2          Connect to EC2 instance via SSM session
  ec2 ssh      SSH to EC2 instance via SSM
  ec2 rdp      RDP to Windows EC2 instance via SSM
  forward      Port forwarding via SSM
  ecs          Connect to ECS container via execute-command
  ecs logs     Tail ECS service logs
  rds          Port forward to RDS instance via SSM
  fav          Connect to a favorite instance
  env          Manage named environments
  init         Create ~/.act.json configuration file
  doctor       Check system dependencies and configuration
  upgrade      Upgrade act to the latest version
```

**Verify**: `go build -o act . && ./act help 2>&1 | grep 'env '` → returns a
match. `./act fav help 2>&1 | grep 'list'` → returns a match.

### Step 8: Update README

Add `env` to the Commands table in `README.md` (find the existing table
with rows like `| \`fav\` | ... |`), and add examples for `act fav list` and
`act env` to the Examples section, following the existing style (e.g. near
the existing `# Favorites` example block):

```
# Favorites
act fav                              # picker + connect
act fav list                         # list favorites (non-interactive)
act fav add i-0123456789abcdef0      # add to favorites
act fav rm i-0123456789abcdef0       # remove from favorites

# Named environments
act env list                                            # list environments
act env add prod --profile production --region us-west-2
act env rm prod
```

**Verify**: `grep -n "act env" README.md` → returns matches.

### Step 9: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass.

## Test plan

- `internal/config/config_test.go`: `TestListFavorites` (empty case, then
  after 2 adds) and `TestEnvironmentCRUD` (empty, add, upsert-overwrite,
  remove) — both modeled after the existing `TestAddRemoveFavorite`
  (`internal/config/config_test.go:104-138`) for the `t.TempDir()` +
  `overrideHome` pattern.
- `main.go` itself gets no new automated tests in this plan (consistent with
  plan 011's scoping rationale: `run*`/CLI-dispatch functions that call
  `os.Exit` and touch the config file are not covered by the existing
  `main_test.go` pattern); the Step 4/6/7 verifications are manual `go
  build` + `grep`/`./act ... | grep` checks instead. If plan 011 has already
  landed, consider (as an optional, not required, follow-up outside this
  plan) adding `printEnvHelp` to `TestHelpFunctionsContainDocumentedFlags`'s
  table.
- Verification: `go test ./...` → all pass, including the 2 new config
  tests.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/config/config.go` exports `ListFavorites`,
      `ListEnvironments`, `AddEnvironment`, `RemoveEnvironment`
- [ ] `internal/config/config_test.go` has `TestListFavorites` and
      `TestEnvironmentCRUD`, both passing
- [ ] `main.go` has a `case "list":` branch in `runFav`
- [ ] `main.go` has a new `runEnv` function, `printEnvHelp` function, and
      `case "env":` dispatch entry
- [ ] `printUsage` and `printFavHelp` document the additions
- [ ] `README.md` documents `act fav list` and `act env {list,add,rm}`
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `git status` shows only `main.go`, `internal/config/config.go`,
      `internal/config/config_test.go`, `README.md` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at any cited location doesn't match the excerpts above (drift
  since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- You discover `"env"` is already used as a subcommand name elsewhere in
  `main.go`'s dispatch switch (re-check via
  `grep -n 'case "' main.go` yourself before Step 6) — if so, STOP and
  report the conflict rather than picking a different name unilaterally.
- You're unsure whether `AddEnvironment`'s upsert-overwrite semantics (vs.
  `AddFavorite`'s no-op-if-exists semantics) is the right call — this plan
  has made that decision explicitly (see Step 2's note) and it should not be
  revisited without flagging it in your final report.

## Maintenance notes

- If a future plan adds `act profile list` (surfacing `~/.aws/config`
  profiles, a related direction item not covered here), it should follow
  the same `list`/`add`/`rm` subcommand convention established by this plan
  for consistency.
- A reviewer should scrutinize: that `env add`'s upsert behavior is
  documented clearly enough in the help text and README that a user
  re-running `env add prod` with new values isn't surprised it overwrote the
  old entry rather than erroring.
- This plan does not add environment name validation (e.g. rejecting names
  that collide with reserved words) — if `--env` resolution
  (`internal/config/config.go:103-138`) ever needs stricter validation,
  that's a separate concern from this plan's CRUD additions.
