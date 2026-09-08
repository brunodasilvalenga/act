# Plan 033: Split main.go into per-concern files within the same `package main`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- main.go main_test.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts and exact function list below against the live
> code before proceeding; on a mismatch (a function renamed, removed, or a
> new one added that isn't accounted for in this plan's mapping), treat it
> as a STOP condition — do not improvise a mapping for it.

## Status

- **Priority**: P3
- **Effort**: L
- **Risk**: MED
- **Depends on**: none (but see "Maintenance notes" — real merge-order
  conflict with any other in-flight plan that edits `main.go`)
- **Category**: tech-debt
- **Planned at**: commit `aa50614`, 2026-07-21

## Why this matters

`main.go` is 1258 lines — roughly 10x the repo's per-file median
(`internal/doctor/doctor.go` at 293 lines is the next-largest non-main
file). It mixes five unrelated concerns in one file: global-flag parsing,
the full subcommand dispatch switch, ~15 `print*Help` functions that are
pure string literals (~290 lines of the file), three small shared helpers,
and the complete business logic for 10 subcommands. The file has grown by
roughly 140 lines since the prior audit pass (011/012/016/018 added `ssm
run`, `fav`, `env` CRUD, `doctor --fix` — all landing in this one file),
and every future subcommand will keep growing it unless the pattern
changes now. This is a WATCH-level finding, not urgent — no bug or stuck
feature is tied to the file's size itself — but splitting it now, while
the mapping is still simple, is far cheaper than doing it after 5 more
subcommands land.

**Design decision — same-package split, not a move into `internal/`.**
This plan splits `main.go` into multiple files that ALL remain
`package main` in the repo root (e.g. `help.go`, `cmd_ec2.go`). It
deliberately does NOT move any code into a new `internal/cmd/` package.
Two reasons, both verified during recon:

1. `main_test.go` (`package main`, same directory) calls package-level
   symbols directly and unqualified — `parseGlobalFlags(...)`,
   `hasHelp(...)`, `subcommandNeedsAWSCLI(...)`, `parseTags(...)`,
   `printUsage`, `printEC2Help`, `printForwardHelp`, `printECSHelp`,
   `printRDSHelp`, `printECSLogsHelp`, `printEC2SSHHelp`, `printFavHelp`,
   `printDoctorHelp`, `printInitHelp`, `printEC2RDPHelp` (see exact test
   names and line numbers in "Current state" below). A same-package,
   multi-file split requires **zero changes** to `main_test.go` — Go
   makes every symbol in every file of a package visible to every other
   file in that package, including test files, regardless of which
   `.go` file declares it. Moving any of these functions into
   `internal/cmd/` would break every one of these references unless
   `main_test.go` were rewritten with new import paths and the functions
   were exported (capitalized) — out of scope and unnecessary risk for a
   pure reorganization.
2. Moving `run*` functions into `internal/cmd/` was considered and
   rejected on import-cycle grounds: those functions call `pickInstance`,
   `parseTags`, and `parseCommands`, which live in `main.go` today. If
   `run*` moved to `internal/cmd` but the helpers stayed in `package main`,
   `internal/cmd` would need to import `main` to call them — which is
   impossible in Go (nothing can import package `main`). Moving the
   helpers too just relocates the same problem one level deeper. Staying
   in `package main`, split across files in the same directory, avoids
   this entirely: Go resolves same-package symbols with no import
   statement needed, no matter how many files declare them.

## Current state

Confirmed by reading the entirety of both files at commit `aa50614`.

- `main.go` (1258 lines, `package main`, confirmed line 1) — imports at
  lines 1–18:
  ```go
  import (
  	"bufio"
  	"flag"
  	"fmt"
  	"os"
  	"os/exec"
  	"sort"
  	"strings"
  	"time"

  	"github.com/brunodasilvalenga/act/internal/aws"
  	"github.com/brunodasilvalenga/act/internal/config"
  	"github.com/brunodasilvalenga/act/internal/doctor"
  	"github.com/brunodasilvalenga/act/internal/tui"
  	"github.com/brunodasilvalenga/act/internal/updater"
  )
  ```

- Every top-level declaration in `main.go`, in file order, with its exact
  current line range (re-verify with `grep -n "^func \|^var version" main.go`
  before starting — line numbers shift if any other change lands first):

  | Symbol | Lines | Kind |
  |---|---|---|
  | `var version` | 20 | var |
  | `main` | 22–199 | dispatch switch |
  | `parseGlobalFlags` | 202–228 | helper |
  | `hasHelp` | 230–235 | helper |
  | `subcommandNeedsAWSCLI` | 241–243 | helper |
  | `printUsage` | 245–273 | help text |
  | `printEC2Help` | 275–295 | help text |
  | `printForwardHelp` | 297–319 | help text |
  | `printECSHelp` | 321–343 | help text |
  | `printRDSHelp` | 345–369 | help text |
  | `printSSMHelp` | 371–384 | help text |
  | `printSSMRunHelp` | 386–416 | help text |
  | `printECSLogsHelp` | 418–443 | help text |
  | `printEC2SSHHelp` | 445–470 | help text |
  | `printFavHelp` | 472–494 | help text |
  | `printDoctorHelp` | 496–522 | help text |
  | `printInitHelp` | 524–534 | help text |
  | `runInit` | 536–584 | business logic |
  | `parseTags` | 587–601 | helper |
  | `pickInstance` | 607–617 | helper |
  | `parseCommands` | 621–635 | helper |
  | `runConnect` | 637–651 | business logic (ec2) |
  | `runForward` | 653–694 | business logic (forward) |
  | `runECS` | 696–744 | business logic (ecs) |
  | `runRDS` | 746–820 | business logic (rds) |
  | `runSSMRun` | 822–901 | business logic (ssm) |
  | `runLogs` | 903–989 | business logic (ecs logs) |
  | `runSSH` | 991–1022 | business logic (ec2 ssh) |
  | `printEC2RDPHelp` | 1024–1053 | help text |
  | `runRDP` | 1055–1095 | business logic (ec2 rdp) |
  | `runFav` | 1097–1167 | business logic (fav) |
  | `printEnvHelp` | 1169–1184 | help text |
  | `runEnv` | 1186–1242 | business logic (env) |
  | `printVersion` | 1244–1258 | version/updater |

  That is **15 `print*Help` functions** total: `printUsage`,
  `printEC2Help`, `printForwardHelp`, `printECSHelp`, `printRDSHelp`,
  `printSSMHelp`, `printSSMRunHelp`, `printECSLogsHelp`, `printEC2SSHHelp`,
  `printFavHelp`, `printDoctorHelp`, `printInitHelp`, `printEC2RDPHelp`,
  `printEnvHelp` — that's 14; `printVersion` is NOT a help function (it
  prints `act version X` and checks for updates via `updater`, called only
  from the `--version` flag path) and stays with `main`/version logic, not
  in `help.go`. Re-count: `printUsage, printEC2Help, printForwardHelp,
  printECSHelp, printRDSHelp, printSSMHelp, printSSMRunHelp,
  printECSLogsHelp, printEC2SSHHelp, printFavHelp, printDoctorHelp,
  printInitHelp, printEC2RDPHelp, printEnvHelp` = **14 print*Help
  functions** — move all 14 into `help.go`. `printVersion` stays in
  `main.go` (see Scope below).

  And **10 `run*` functions**: `runInit`, `runConnect`, `runForward`,
  `runECS`, `runRDS`, `runSSMRun`, `runLogs`, `runSSH`, `runRDP`, `runFav`,
  `runEnv` — wait, that's 11. Confirmed count by the grep above: `runInit,
  runConnect, runForward, runECS, runRDS, runSSMRun, runLogs, runSSH,
  runRDP, runFav, runEnv` = **11 `run*` functions**. (The audit finding
  said "10" — the executor should trust this plan's grep-verified list of
  11, not the rounded number in the finding summary.)

- `main_test.go` (211 lines, `package main`, confirmed line 1) has exactly
  5 test functions (`grep -n "^func Test" main_test.go`):
  - `TestParseGlobalFlags` (line 12) → calls `parseGlobalFlags`
  - `TestHasHelp` (line 96) → calls `hasHelp`
  - `TestSubcommandNeedsAWSCLI` (line 118) → calls `subcommandNeedsAWSCLI`
  - `TestParseTags` (line 146) → calls `parseTags`
  - `TestHelpFunctionsContainDocumentedFlags` (line 210) → calls
    `printUsage`, `printEC2Help`, `printForwardHelp`, `printECSHelp`,
    `printRDSHelp`, `printECSLogsHelp`, `printEC2SSHHelp`, `printFavHelp`,
    `printDoctorHelp`, `printInitHelp`, `printEC2RDPHelp` as bare
    function-value references (e.g. `{"printUsage", printUsage, ...}` at
    line 216) — these are passed as `func()` values, so their *names* as
    package-level identifiers matter, not their file location.

  None of these 5 tests reference any `run*` function or `pickInstance`/
  `parseCommands`. This confirms the split's blast radius on
  `main_test.go` is zero as long as every symbol keeps its exact current
  name and stays in `package main` — which this plan guarantees.

- Repo conventions confirmed from `Makefile`:
  ```
  fmt:
  	@test -z "$$(gofmt -l .)" || (echo "..."; gofmt -l .; exit 1)
  vet:
  	go vet ./...
  test: fmt vet
  	go test ./...
  ```
  `make test` runs `gofmt` check, then `go vet`, then `go test ./...` — use
  this as your primary gate, supplemented by `go build ./...` before it
  (build catches missing-import/unused-import errors fastest).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build -v ./...` | exit 0, no compile errors |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | exit 0, **no file paths printed** (a printed path means that file isn't gofmt'd) |
| Full test suite | `go test -v ./...` | exit 0, all tests pass (112 tests passed at the time of writing, per `go test ./...` — see Current state) |
| Combined gate | `make test` | exit 0 (runs fmt check, vet, then test) |
| Smoke test — help wiring | `go run . ec2 help`, `go run . forward help`, `go run . ecs help`, `go run . ecs logs help`, `go run . ssm help`, `go run . ssm run help`, `go run . rds help`, `go run . fav help`, `go run . env help`, `go run . doctor help`, `go run . init help`, `go run . ec2 ssh help`, `go run . ec2 rdp help`, `go run . help` | each prints its corresponding help text to stderr and exits 0 — confirms dispatch in `main.go` still finds the moved `print*Help` functions |

Run the build+vet+fmt+test gate after **every single step** below, not just
at the end — this refactor's entire safety strategy is "verify constantly,
never move more than one logical group before checking."

## Scope

**In scope** (the only files you should create or modify):
- `main.go` — shrinks to: package decl, trimmed imports, `var version`,
  `main()`, `parseGlobalFlags`, `hasHelp`, `subcommandNeedsAWSCLI`,
  `printVersion`. Expected final size: roughly 230–260 lines (the current
  `main()` dispatch switch alone is ~178 lines and does not move).
- `help.go` (new file) — all 14 `print*Help` functions listed above.
- `helpers.go` (new file) — `parseTags`, `pickInstance`, `parseCommands`.
- `cmd_ec2.go` (new file) — `runConnect`, `runSSH`, `runRDP`.
- `cmd_forward.go` (new file) — `runForward`.
- `cmd_ecs.go` (new file) — `runECS`, `runLogs`.
- `cmd_rds.go` (new file) — `runRDS`.
- `cmd_ssm.go` (new file) — `runSSMRun`.
- `cmd_fav.go` (new file) — `runFav`.
- `cmd_env.go` (new file) — `runEnv`.
- `cmd_init.go` (new file) — `runInit`.

That is 9 new files (`help.go`, `helpers.go`, 7 `cmd_*.go` files), all in
the repo root, all `package main`. (Note: the audit finding's phrasing
"~8 cmd_*.go files" rounds down slightly — the verified exact count from
this mapping is 7 `cmd_*.go` files plus `help.go` and `helpers.go`, 9 new
files total.)

**Out of scope** (do NOT touch, even though related):
- `main_test.go` — must need **zero** changes. If any step in this plan
  seems to require editing `main_test.go` to make tests pass again, that
  means a symbol was accidentally renamed, made unexported when it wasn't
  before, or dropped — STOP and report immediately (see STOP conditions).
  Do not "fix" it by editing `main_test.go`.
- Any file under `internal/` — no package boundaries change, no function
  moves into `internal/cmd/` or anywhere else outside the repo root.
- Any behavior change of any kind. This is a pure file reorganization: no
  renamed functions, no changed signatures, no changed flag names, no
  changed help text wording, no changed dispatch logic in `main()`. If you
  notice something in the code that looks like a bug while moving it,
  leave it exactly as-is and do not fix it in this plan — note it in your
  final report instead.
- `README.md` — this plan changes no user-visible behavior, flags, or
  commands, so the "update README after adding a feature" repo rule does
  not apply here. Do not edit README.md for this plan.

## Git workflow

- Branch: `advisor/033-split-main-go-by-subcommand`
- Conventional Commits style, matching repo history (e.g. `refactor: ...`,
  `fix: ...`, `test: ...` — see `git log --oneline -10`).
- **One commit per step below** (not one giant commit for the whole
  refactor) — this lets a reviewer or future `git bisect` pinpoint exactly
  which single file-move introduced a problem, if one slips through
  despite the per-step verification gates.
- Do NOT push or open a PR unless the operator instructs it.

## Steps

Work through these in order. Do not skip ahead or batch multiple steps'
file moves into one commit — the entire point of this ordering is that the
codebase builds and passes tests after every single step.

### Step 0: Baseline check

Confirm you are starting from a clean, working tree:

```
git status                # expect: clean, on branch advisor/033-split-main-go-by-subcommand
go build -v ./...         # expect: exit 0
go vet ./...               # expect: exit 0, no output
gofmt -l .                  # expect: exit 0, no output
go test -v ./...            # expect: exit 0, all pass
```

If any of these fail before you've made any change, STOP — the repo is
already broken and this plan's drift check should have caught it; report
this instead of proceeding.

### Step 1: Create `help.go` with all 14 `print*Help` functions

Create `help.go` in the repo root:
```go
package main

import (
	"fmt"
	"os"
)
```
Cut (not copy) these 14 functions from `main.go` verbatim, in their current
order, and paste them into `help.go`: `printUsage`, `printEC2Help`,
`printForwardHelp`, `printECSHelp`, `printRDSHelp`, `printSSMHelp`,
`printSSMRunHelp`, `printECSLogsHelp`, `printEC2SSHHelp`, `printFavHelp`,
`printDoctorHelp`, `printInitHelp`, `printEC2RDPHelp`, `printEnvHelp`.
Every one of these functions uses only `fmt.Fprintf(os.Stderr, ...)` with
a raw string literal — no other imports are needed in `help.go`. Do not
change any string content, formatting, or whitespace inside the
functions — cut/paste verbatim.

After removing these functions from `main.go`, `main.go` no longer uses
`fmt`/`os` for these specific calls, but it still uses both packages
elsewhere (e.g. `main()`, `printVersion`) — do NOT remove `fmt` or `os`
from `main.go`'s import block; they are still needed there.

**Verify**:
```
go build -v ./...   # expect: exit 0
go vet ./...          # expect: exit 0
gofmt -l .             # expect: exit 0, no output — if help.go or main.go
                       # is listed, run `gofmt -w help.go main.go` and re-check
go test -v ./...      # expect: exit 0, all pass (TestHelpFunctionsContainDocumentedFlags
                       # must still pass — it references these functions by name,
                       # and same-package visibility means it doesn't care they
                       # moved files)
```
Commit: `refactor: extract print*Help functions from main.go into help.go`

### Step 2: Create `helpers.go` with `parseTags`, `pickInstance`, `parseCommands`

Create `helpers.go` in the repo root:
```go
package main

import (
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `parseTags` (lines ~587–601 before Step 1's removal shifted numbers —
re-grep to find current lines), `pickInstance`, and `parseCommands` from
`main.go` verbatim and paste into `helpers.go`, in that order. `pickInstance`
uses `aws.Instance` and `tui.Run` — confirm both imports are needed in
`helpers.go`; `parseTags`/`parseCommands` need only the implicit string
handling already covered by no extra import (they only use slice/string
built-ins — verify with `go build` that `fmt`/`os` are actually used in
this file, since none of these three functions appear to call `fmt` or
`os` directly; if `go vet`/`go build` says `fmt` or `os` is unused in
`helpers.go`, remove that import from `helpers.go`'s block rather than
force it in).

After this move, check whether `main.go` still uses `aws` or `tui`
anywhere else (it does not currently call either directly outside of
`run*` functions which haven't moved yet — those still live in `main.go`
at this point, so `main.go` still needs `aws`/`tui` imports until Step 3+
removes the `run*` functions that use them). Do not remove imports from
`main.go` speculatively — only remove an import when `go build` reports it
unused.

**Verify**:
```
go build -v ./...   # expect: exit 0
go vet ./...          # expect: exit 0
gofmt -l .             # expect: exit 0, no output
go test -v ./...      # expect: exit 0, all pass (TestParseTags must still pass)
```
Commit: `refactor: extract parseTags/pickInstance/parseCommands into helpers.go`

### Step 3: Create `cmd_ec2.go` with `runConnect`, `runSSH`, `runRDP`

Create `cmd_ec2.go`:
```go
package main

import (
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `runConnect`, `runSSH`, `runRDP` from `main.go` verbatim (in that
order) and paste into `cmd_ec2.go`. These three call `parseTags`,
`pickInstance` (now in `helpers.go` — no import needed, same package),
`aws.ListRunningInstances`, `aws.ListWindowsInstances`, `aws.StartSession`,
`aws.StartSSHSession`, `aws.GetPasswordData`, `aws.StartRDP`,
`tui.RunPicker`. Determine the exact import set for `cmd_ec2.go` by
running `go build` and adding/removing imports until it's clean — do not
blanket-copy every import from the original `main.go`.

**Verify**:
```
go build -v ./...   # expect: exit 0
go vet ./...          # expect: exit 0
gofmt -l .             # expect: exit 0, no output
go test -v ./...      # expect: exit 0, all pass
```
Commit: `refactor: extract runConnect/runSSH/runRDP into cmd_ec2.go`

### Step 4: Create `cmd_forward.go` with `runForward`

Create `cmd_forward.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
)
```
Cut `runForward` verbatim into `cmd_forward.go`. It calls `parseTags`,
`pickInstance`, `flag.NewFlagSet`, `aws.StartRemotePortForward`,
`aws.StartPortForward`. Determine exact imports via `go build`.

**Verify**: same 4 commands as Step 3.
Commit: `refactor: extract runForward into cmd_forward.go`

### Step 5: Create `cmd_ecs.go` with `runECS`, `runLogs`

Create `cmd_ecs.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `runECS` and `runLogs` verbatim into `cmd_ecs.go`, in that order. They
call `aws.ListECSClusters`, `aws.ListECSTasks`, `aws.StartECSExec`,
`aws.ListECSServices`, `aws.GetLogGroupsFromService`, `aws.TailLogs`,
`tui.RunPicker`, `tui.RunECS`. Determine exact imports via `go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runECS/runLogs into cmd_ecs.go`

### Step 6: Create `cmd_rds.go` with `runRDS`

Create `cmd_rds.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `runRDS` verbatim into `cmd_rds.go`. Calls `parseTags`, `pickInstance`,
`aws.ListRDSInstances`, `aws.RDSInstance`, `aws.StartRemotePortForward`,
`tui.RunPicker`. Determine exact imports via `go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runRDS into cmd_rds.go`

### Step 7: Create `cmd_ssm.go` with `runSSMRun`

Create `cmd_ssm.go`:
```go
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `runSSMRun` verbatim into `cmd_ssm.go`. Calls `parseTags`,
`parseCommands`, `os.ReadFile`, `strings.Split`/`strings.TrimRight`,
`tui.Run`, `aws.DocumentForPlatform`, `aws.SendCommand`,
`aws.WaitForCommandInvocation`, `time.Second`. Determine exact imports via
`go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runSSMRun into cmd_ssm.go`

### Step 8: Create `cmd_fav.go` with `runFav`

Create `cmd_fav.go`:
```go
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/tui"
)
```
Cut `runFav` verbatim into `cmd_fav.go`. Calls `config.Load`,
`config.ListFavorites`, `config.AddFavorite`, `config.RemoveFavorite`,
`tui.RunPicker`, `aws.StartSession`, `strings.HasPrefix`, and
`printFavHelp` (now in `help.go` — same package, no import needed).
Determine exact imports via `go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runFav into cmd_fav.go`

### Step 9: Create `cmd_env.go` with `runEnv`

Create `cmd_env.go`:
```go
package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/brunodasilvalenga/act/internal/config"
)
```
Cut `runEnv` verbatim into `cmd_env.go`. Calls `config.ListEnvironments`,
`config.AddEnvironment`, `config.RemoveEnvironment`, `sort.Strings`, and
`printEnvHelp` (now in `help.go` — same package). Determine exact imports
via `go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runEnv into cmd_env.go`

### Step 10: Create `cmd_init.go` with `runInit`

Create `cmd_init.go`:
```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/brunodasilvalenga/act/internal/config"
)
```
Cut `runInit` verbatim into `cmd_init.go`. Calls `bufio.NewReader`,
`config.Exists`, `config.Load`, `config.Init`, `config.ConfigPath`,
`strings.TrimSpace`/`strings.ToLower`. Determine exact imports via
`go build`.

**Verify**: same 4 commands.
Commit: `refactor: extract runInit into cmd_init.go`

### Step 11: Trim `main.go`'s imports and confirm final shape

At this point `main.go` should contain only: package decl, imports,
`var version`, `main()`, `parseGlobalFlags`, `hasHelp`,
`subcommandNeedsAWSCLI`, `printVersion`. Run `go build ./...` and remove
any import from `main.go`'s block that the compiler reports as unused
(likely candidates to drop: `bufio`, `flag`, `sort`, `strings`, `time` —
likely candidates to keep: `fmt`, `os`, `os/exec` for the `aws` CLI
`exec.LookPath` check in `main()`, plus `internal/config`, `internal/doctor`,
`internal/updater` which are still called directly from the dispatch
switch in `main()`). Do not guess — let the compiler tell you via build
errors (`imported and not used`) which imports to drop.

Check the final line count of `main.go`:
```
wc -l main.go
```
Expected: roughly 230–260 lines. If it's wildly outside this range (e.g.
still over 500, or under 100), re-check that you moved the right set of
functions and didn't accidentally leave something behind or delete
something you shouldn't have.

**Verify**:
```
go build -v ./...   # expect: exit 0
go vet ./...          # expect: exit 0
gofmt -l .             # expect: exit 0, no output across ALL files
go test -v ./...      # expect: exit 0, all pass, same test count as baseline (112)
wc -l main.go         # expect: ~230-260
```
Commit: `refactor: trim main.go imports after subcommand split`

### Step 12: Full verification suite + manual smoke test

Run the complete gate one final time:
```
go build -v ./...
go vet ./...
gofmt -l .
go test -v ./...
make test
```
All must pass exactly as at baseline (Step 0) — same test count, no new
failures, no formatting issues.

Then manually smoke-test that dispatch still reaches every moved help
function (this is the one thing `go build`/`go test` cannot fully prove —
that `main()`'s switch statement in `main.go` still correctly calls into
functions now defined in other files):
```
go run . help
go run . ec2 help
go run . ec2 ssh help
go run . ec2 rdp help
go run . forward help
go run . ecs help
go run . ecs logs help
go run . ssm help
go run . ssm run help
go run . rds help
go run . fav help
go run . env help
go run . doctor help
go run . init help
```
Each command must print its corresponding help text (matching the content
you moved into `help.go` in Step 1) to stderr and exit 0. If any of these
prints nothing, prints the wrong help text, or errors, STOP — this means
`main()`'s dispatch switch (which did not move) is somehow not resolving
to the correct now-relocated function, which should be impossible in a
correct same-package split; report the exact command and output.

Also confirm `git status` shows only the 9 new files added and `main.go`
modified — nothing else:
```
git status
```

Commit (if any cleanup was needed) or simply confirm the branch is clean
and ready for review.

## Test plan

This plan adds no new test files and changes no test behavior — the
existing 5 test functions in `main_test.go` (`TestParseGlobalFlags`,
`TestHasHelp`, `TestSubcommandNeedsAWSCLI`, `TestParseTags`,
`TestHelpFunctionsContainDocumentedFlags`) are the full regression
coverage for this refactor, and they must pass unchanged and un-edited
after every step. There is no new functionality to test — this is a pure
reorganization plan, verified by:
- `go test -v ./...` passing with the same test count before and after
  (112 tests passed at baseline — confirm the same count at the end).
- The manual smoke test in Step 12, which exercises the dispatch paths
  that unit tests don't cover (the `main()` switch statement itself).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` exits 0 with no output (no file listed)
- [ ] `go test -v ./...` exits 0, same test count as baseline, no failures
- [ ] `make test` exits 0
- [ ] `main_test.go` has zero diff from its state at commit `aa50614`
      (`git diff aa50614..HEAD -- main_test.go` produces no output)
- [ ] `main.go` is between roughly 230 and 260 lines (`wc -l main.go`)
- [ ] `help.go`, `helpers.go`, `cmd_ec2.go`, `cmd_forward.go`, `cmd_ecs.go`,
      `cmd_rds.go`, `cmd_ssm.go`, `cmd_fav.go`, `cmd_env.go`, `cmd_init.go`
      all exist in the repo root and all declare `package main`
      (`head -1 help.go helpers.go cmd_*.go` prints `package main` for
      each)
- [ ] Every manual smoke-test command in Step 12 prints the expected help
      text and exits 0
- [ ] `git status` shows exactly: 9 new files, `main.go` modified, nothing
      else
- [ ] `plans/README.md` status row for plan 033 updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at the locations in "Current state" doesn't match what's on
  disk (function names, line ranges, or the 14/11 function-count tallies
  are wrong) — the codebase has drifted since this plan was written.
- Any step's `go build`/`go vet`/`gofmt`/`go test` verification fails
  twice in a row after a reasonable fix attempt (e.g. adjusting one
  file's import list). Do not keep moving code around trying to "make it
  work" — revert that step's file creation, re-examine the current-state
  mapping, and report the discrepancy.
- Making this split appears to require ANY change to `main_test.go`. This
  would mean a symbol was renamed, unexported, or dropped somewhere along
  the way — a violation of this plan's core "same-package, zero test
  changes" design constraint. Stop immediately and report which symbol
  and which step caused it; do not "fix" it by editing the test file.
- You find a genuine behavior bug while moving code (e.g. a help function
  documents a flag that doesn't exist, or a `run*` function has dead
  code). Leave it as-is, move it verbatim, and note the bug in your final
  report — fixing it is out of scope for this plan.
- `git status` at the end shows any file touched outside the Scope list
  (e.g. anything under `internal/`, `README.md`, `go.mod`).

## Maintenance notes

- **Real merge-order conflict**: `plans/026-dedupe-ecs-cluster-picker.md`
  is also planned at base commit `aa50614` and its Scope directly edits
  `runECS` and `runLogs` inside `main.go` (extracting a shared "pick ECS
  cluster" helper). If plan 026 and this plan (033) are both executed
  from branches created off the same base commit, they will conflict
  textually wherever `runECS`/`runLogs` are touched — 026 edits them in
  place inside `main.go`, this plan relocates them whole into
  `cmd_ecs.go`. This is a sequencing decision for the human operator, not
  something either plan's executor can resolve unilaterally: either (a)
  execute 026 first, merge it, then execute this plan against the
  post-026 `main.go` (033's Step 5 mapping for `runECS`/`runLogs` should
  still hold structurally, just cut/paste whatever the deduped versions
  look like), or (b) execute this plan first, merge it, then execute 026
  against the relocated `cmd_ecs.go` instead of `main.go`. Do NOT execute
  both concurrently from the same base commit and attempt to merge both
  branches — that guarantees a conflicting or silently-wrong merge.
  Flag this to whoever is sequencing plan execution.
- Any future plan that adds a new subcommand should add its `run*`
  function and `print*Help` function directly to the relevant existing
  `cmd_*.go`/`help.go` file (or create a new `cmd_<name>.go` file for a
  genuinely new command group) rather than appending to `main.go` — this
  is the whole point of the split, and it should be called out in this
  repo's `CLAUDE.md`/`MEMORY.md` "Project Structure" notes as a follow-up
  if not already reflected there after this plan merges.
- A reviewer should scrutinize: (1) that every cut/paste was truly
  verbatim (no accidental reformatting or logic changes), (2) that each
  new file's import list is minimal (no leftover unused imports that
  happened to also be used by something else in the same file coincidentally),
  and (3) that the manual smoke test in Step 12 was actually run and its
  output pasted/confirmed, not just assumed to pass because `go build`
  succeeded (a successful build only proves the code compiles, not that
  `main()`'s switch statement dispatches to the right function for every
  subcommand).
- No follow-up work is deferred out of this plan — it is a complete,
  self-contained reorganization. The next natural step (not part of this
  plan) would be updating `~/.claude/projects/.../memory/MEMORY.md`'s
  "Project Structure" section to mention the new `help.go`/`helpers.go`/
  `cmd_*.go` files, but that file lives outside this repo and is
  maintained by the user's own tooling, not by this plan.
