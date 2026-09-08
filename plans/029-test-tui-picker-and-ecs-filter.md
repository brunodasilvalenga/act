# Plan 029: Add test coverage for `pickerModel.Update` and `ecsModel.applyFilter`

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/tui/picker.go internal/tui/ecs.go internal/tui/tui_test.go`
> If any of these three files changed since this plan was written, compare
> the "Current state" excerpts below against the live code before
> proceeding; on a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P3
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: tests
- **Planned at**: commit `aa50614`, 2026-07-20

## Why this matters

`internal/tui/picker.go`'s `pickerModel.Update` is the shared picker driving
`act fav`, `act ecs`/`act ecs logs` (cluster/service selection), and `act
rds` — cursor movement, Enter-to-select, and Esc/Ctrl+C-to-quit all have zero
test coverage today. `internal/tui/ecs.go`'s `ecsModel.applyFilter` (the ECS
task picker's search filter) is structurally identical to `model.applyFilter`
in `internal/tui/tui.go` (the EC2 instance picker's filter), which already
has two tests (`TestApplyFilter`, `TestApplyFilterCursorReset` in
`internal/tui/tui_test.go`) — but `ecsModel.applyFilter` has no equivalent
test anywhere. Both pieces of logic are pure/deterministic and cheap to test
without spinning up the full bubbletea runtime. Landing this plan closes the
gap so a future regression in cursor clamping, Enter-selection, or ECS
search filtering fails a test instead of shipping silently.

## Current state

- `internal/tui/picker.go` (80 lines) — generic single-column picker. Struct
  and `Update` as of `aa50614`:

  ```go
  type pickerModel struct {
      title    string
      items    []string
      cursor   int
      selected string
      quitting bool
  }

  func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
      switch msg := msg.(type) {
      case tea.KeyMsg:
          switch msg.Type {
          case tea.KeyCtrlC, tea.KeyEsc:
              m.quitting = true
              return m, tea.Quit
          case tea.KeyEnter:
              if len(m.items) > 0 {
                  m.selected = m.items[m.cursor]
              }
              return m, tea.Quit
          case tea.KeyUp:
              if m.cursor > 0 {
                  m.cursor--
              }
          case tea.KeyDown:
              if m.cursor < len(m.items)-1 {
                  m.cursor++
              }
          }
      }
      return m, nil
  }
  ```

  `Update` has value receiver `pickerModel` (not pointer) and returns a new
  `tea.Model` — in a test, capture the returned model via a type assertion:
  `newModel, cmd := m.Update(msg); m2 := newModel.(pickerModel)`.

- `internal/tui/ecs.go` (238 lines) — ECS task picker. `applyFilter` as of
  `aa50614` (lines 134-152), pointer receiver:

  ```go
  func (m *ecsModel) applyFilter() {
      if m.search == "" {
          m.filtered = m.tasks
      } else {
          var filtered []aws.ECSTask
          lower := strings.ToLower(m.search)
          for _, task := range m.tasks {
              if strings.Contains(strings.ToLower(task.ServiceName), lower) ||
                  strings.Contains(strings.ToLower(task.ContainerName), lower) ||
                  strings.Contains(strings.ToLower(task.TaskID), lower) {
                  filtered = append(filtered, task)
              }
          }
          m.filtered = filtered
      }
      if m.cursor >= len(m.filtered) {
          m.cursor = max(0, len(m.filtered)-1)
      }
  }
  ```

  Filters by substring match (case-insensitive) on `ServiceName`,
  `ContainerName`, OR `TaskID`; clamps `m.cursor` when the new filtered list
  is shorter.

- `internal/aws/ecs.go` (lines 10-17) defines the fixture type:

  ```go
  type ECSTask struct {
      TaskARN       string
      TaskID        string
      ContainerName string
      ClusterARN    string
      ClusterName   string
      ServiceName   string
  }
  ```

- `internal/tui/tui_test.go` (64 lines, full file) — the exact structural
  pattern to mirror for the ECS filter tests. It tests `model.applyFilter`
  (EC2 instance picker, in `internal/tui/tui.go`) with a table-driven test
  plus a dedicated cursor-reset test:

  ```go
  func TestApplyFilter(t *testing.T) {
      instances := []aws.Instance{
          {Name: "web-server-prod", InstanceID: "i-abc123", PrivateIP: "10.0.1.1", InstanceType: "t3.micro"},
          {Name: "api-server-prod", InstanceID: "i-def456", PrivateIP: "10.0.1.2", InstanceType: "t3.small"},
          {Name: "db-server-staging", InstanceID: "i-ghi789", PrivateIP: "10.0.2.1", InstanceType: "r5.large"},
      }
      tests := []struct {
          name     string
          search   string
          expected int
      }{
          {"empty filter returns all", "", 3},
          {"filter by name", "web", 1},
          {"filter by instance ID", "def456", 1},
          {"filter by IP", "10.0.2", 1},
          {"filter case insensitive", "WEB", 1},
          {"filter no match", "nonexistent", 0},
          {"filter partial name", "prod", 2},
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              m := &model{instances: instances, filtered: instances, search: tt.search}
              m.applyFilter()
              if len(m.filtered) != tt.expected {
                  t.Errorf("applyFilter(%q) returned %d results, want %d", tt.search, len(m.filtered), tt.expected)
              }
          })
      }
  }

  func TestApplyFilterCursorReset(t *testing.T) {
      instances := []aws.Instance{
          {Name: "a", InstanceID: "i-1", PrivateIP: "10.0.0.1", InstanceType: "t3.micro"},
          {Name: "b", InstanceID: "i-2", PrivateIP: "10.0.0.2", InstanceType: "t3.micro"},
          {Name: "c", InstanceID: "i-3", PrivateIP: "10.0.0.3", InstanceType: "t3.micro"},
      }
      m := &model{instances: instances, filtered: instances, cursor: 2, search: "a"}
      m.applyFilter()
      if m.cursor != 0 {
          t.Errorf("cursor should reset to 0 when filtered list is smaller, got %d", m.cursor)
      }
  }
  ```

  Match this table-driven style, naming, and assertion style exactly for the
  new ECS tests (swap `aws.Instance{Name, InstanceID, PrivateIP,
  InstanceType}` for `aws.ECSTask{ServiceName, ContainerName, TaskID}`).

- Bubbletea version pinned in `go.mod`: `github.com/charmbracelet/bubbletea
  v1.3.10`. Relevant types in that package (confirmed via `go doc
  github.com/charmbracelet/bubbletea.<Type>` — re-run these yourself if in
  doubt):
  - `type KeyMsg Key` where `type Key struct { Type KeyType; Runes []rune;
    Alt bool; Paste bool }`. Construct synthetic key presses as
    `tea.KeyMsg{Type: tea.KeyDown}`, `tea.KeyMsg{Type: tea.KeyUp}`,
    `tea.KeyMsg{Type: tea.KeyEnter}`, `tea.KeyMsg{Type: tea.KeyEsc}`,
    `tea.KeyMsg{Type: tea.KeyCtrlC}`.
  - `func Quit() Msg` — the `tea.Cmd` value referenced as `tea.Quit` in
    `picker.go` is this function itself (a `tea.Cmd` is `func() tea.Msg`, and
    `tea.Quit` is passed as a bare function value, not called). Calling the
    returned `tea.Cmd` (i.e. `cmd()`) executes `tea.Quit`, which returns a
    `tea.QuitMsg{}` value (`type QuitMsg struct{}`).
  - Precise assertion for "Update returned a quit command": call the
    returned `cmd` and check its result type, e.g.:
    ```go
    _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
    if cmd == nil {
        t.Fatal("expected a non-nil quit command")
    }
    if _, ok := cmd().(tea.QuitMsg); !ok {
        t.Fatalf("expected cmd() to produce tea.QuitMsg, got %T", cmd())
    }
    ```
    Use this exact pattern (nil-check the cmd, then call it and assert the
    resulting message type is `tea.QuitMsg`) for every case that should quit
    (Enter, Esc, Ctrl+C). For cases that should NOT quit (Up, Down), assert
    `cmd == nil` — `Update`'s `KeyUp`/`KeyDown` branches fall through to the
    final `return m, nil`, so a non-nil cmd there would itself indicate a
    behavior change worth flagging (see STOP conditions).

- `internal/tui/` currently contains only one test file: `tui_test.go`
  (confirmed via `ls internal/tui/*_test.go`). No `picker_test.go` or
  `ecs_test.go` exists yet.

- Module path: `github.com/brunodasilvalenga/act` (import ECS/instance
  fixtures via `github.com/brunodasilvalenga/act/internal/aws`, same as
  `tui_test.go` does).

## Commands you will need

| Purpose        | Command                                                              | Expected on success        |
|-----------------|-----------------------------------------------------------------------|-----------------------------|
| Build           | `go build -v ./...`                                                   | exit 0                     |
| Vet             | `go vet ./...`                                                        | exit 0, no output           |
| Format check    | `gofmt -l .`                                                           | no output (no files listed) |
| Confirm bubbletea types | `go doc github.com/charmbracelet/bubbletea.KeyMsg` / `.Key` / `.Quit` / `.QuitMsg` | matches excerpts above |
| Run new tests   | `go test -v ./... -run 'TestPickerModelUpdate|TestECSApplyFilter'`     | all new subtests PASS      |
| Full test suite | `go test ./...`                                                        | all packages ok, no regressions |

## Scope

**In scope** (the only files you should create/modify):
- `internal/tui/picker_test.go` (new file)
- `internal/tui/ecs_test.go` (new file)

**Out of scope** (do NOT touch, even though they look related):
- `internal/tui/picker.go` — production code. If writing a test reveals a
  real bug (e.g. cursor going out of bounds, a missing guard), do NOT patch
  it — STOP and report per "STOP conditions" below.
- `internal/tui/ecs.go` — same rule; test-only, no production changes.
- `internal/tui/tui.go`, `internal/tui/tui_test.go` — the existing EC2
  picker and its tests are the reference pattern to mirror, not something to
  edit.
- `plans/README.md` — a separate process owns this file; do not edit it
  unless you are told a reviewer is not tracking it for you.

## Git workflow

- Branch: `advisor/029-test-tui-picker-and-ecs-filter`
- Commit message style: Conventional Commits, matching this repo's history
  (e.g. `test: add characterization tests for main.go flag parsing and help
  text` from commit `26de8d1`, `test: add unit tests for updater.buildAssetName
  and doctor.extractJSON` from commit `ef570e7`). Use a `test:` prefix, e.g.
  `test: add coverage for pickerModel.Update and ecsModel.applyFilter`.
- One commit for both new test files is fine (they're one logical unit: test
  coverage for this plan's finding). Do not push or open a PR unless told.

## Steps

### Step 1: Confirm the ground truth hasn't drifted

Run the drift check from the top of this file. Then open
`internal/tui/picker.go` and `internal/tui/ecs.go` and confirm the code
matches the "Current state" excerpts above exactly (same field names, same
guard conditions, same line-level behavior). If it doesn't match, STOP per
the "STOP conditions" section — do not adapt the plan to different code
without reporting first.

**Verify**: the excerpts in this plan match the live file contents by eye.

### Step 2: Create `internal/tui/picker_test.go`

Add a new test file in package `tui` with a single `TestPickerModelUpdate`
function using `t.Run` subtests (mirroring the table-driven style, but since
each case needs a distinct assertion shape — cursor position vs. quit
command vs. panic-safety — structure it as named subtests rather than one
table). Cover exactly these cases:

1. **`"down arrow moves cursor forward"`**: `m := pickerModel{items: []string{"a", "b", "c"}}` (cursor starts at 0). Call `m.Update(tea.KeyMsg{Type: tea.KeyDown})`, assert the returned model's `cursor == 1`, and assert `cmd == nil`.
2. **`"down arrow clamps at end"`**: `m := pickerModel{items: []string{"a", "b", "c"}, cursor: 2}` (already at last index). Call `Update` with `KeyDown`, assert `cursor` is still `2` (does not go past `len(items)-1`).
3. **`"up arrow moves cursor backward"`**: `m := pickerModel{items: []string{"a", "b", "c"}, cursor: 2}`. Call `Update` with `KeyUp`, assert `cursor == 1`.
4. **`"up arrow clamps at zero"`**: `m := pickerModel{items: []string{"a", "b", "c"}, cursor: 0}`. Call `Update` with `KeyUp`, assert `cursor` is still `0` (does not go negative).
5. **`"enter selects current item and quits"`**: `m := pickerModel{items: []string{"a", "b", "c"}, cursor: 1}`. Call `Update` with `tea.KeyMsg{Type: tea.KeyEnter}`. Assert the returned model's `selected == "b"`. Assert `cmd != nil` and `cmd()` produces a `tea.QuitMsg` (use the exact pattern from "Current state" above).
6. **`"enter on empty items does not panic and does not select"`**: `m := pickerModel{items: []string{}}` (cursor defaults to 0). Call `Update` with `KeyEnter` inside the test function body directly (no need for `recover()` — if the `if len(m.items) > 0` guard were ever removed, `go test` would fail the whole test run with an unrecovered panic, which is an acceptable and correct failure signal). Assert the returned model's `selected == ""` (unchanged from zero value) and `cmd != nil` producing `tea.QuitMsg` (Enter always quits regardless of items).
7. **`"esc quits without selecting"`**: `m := pickerModel{items: []string{"a", "b"}, cursor: 0}`. Call `Update` with `tea.KeyMsg{Type: tea.KeyEsc}`. Assert returned model's `quitting == true`, `selected == ""`, and `cmd() == tea.QuitMsg{}` per the pattern above.
8. **`"ctrl+c quits without selecting"`**: same as case 7 but with `tea.KeyMsg{Type: tea.KeyCtrlC}`.

Remember: `pickerModel.Update` has a value receiver, so capture the result
via `newModel, cmd := m.Update(msg)` then `m2 := newModel.(pickerModel)` to
inspect fields — the original `m` is never mutated.

**Verify**: `go test -v ./internal/tui/... -run TestPickerModelUpdate` → all
8 subtests PASS, 0 failures.

### Step 3: Create `internal/tui/ecs_test.go`

Add a new test file in package `tui`, mirroring `tui_test.go`'s
`TestApplyFilter`/`TestApplyFilterCursorReset` structure exactly, but named
`TestECSApplyFilter` and `TestECSApplyFilterCursorReset`, using
`aws.ECSTask{ServiceName, ContainerName, TaskID}` fixtures:

```go
func TestECSApplyFilter(t *testing.T) {
    tasks := []aws.ECSTask{
        {ServiceName: "web-service-prod", ContainerName: "web", TaskID: "abc123"},
        {ServiceName: "api-service-prod", ContainerName: "api", TaskID: "def456"},
        {ServiceName: "db-service-staging", ContainerName: "db", TaskID: "ghi789"},
    }

    tests := []struct {
        name     string
        search   string
        expected int
    }{
        {"empty filter returns all", "", 3},
        {"filter by service name", "web", 1},
        {"filter by container name", "api", 1},
        {"filter by task ID", "ghi789", 1},
        {"filter case insensitive", "WEB", 1},
        {"filter no match", "nonexistent", 0},
        {"filter partial service name", "prod", 2},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m := &ecsModel{tasks: tasks, filtered: tasks, search: tt.search}
            m.applyFilter()
            if len(m.filtered) != tt.expected {
                t.Errorf("applyFilter(%q) returned %d results, want %d", tt.search, len(m.filtered), tt.expected)
            }
        })
    }
}

func TestECSApplyFilterCursorReset(t *testing.T) {
    tasks := []aws.ECSTask{
        {ServiceName: "a", ContainerName: "a", TaskID: "1"},
        {ServiceName: "b", ContainerName: "b", TaskID: "2"},
        {ServiceName: "c", ContainerName: "c", TaskID: "3"},
    }

    m := &ecsModel{tasks: tasks, filtered: tasks, cursor: 2, search: "a"}
    m.applyFilter()

    if m.cursor != 0 {
        t.Errorf("cursor should reset to 0 when filtered list is smaller, got %d", m.cursor)
    }
}
```

Note `applyFilter` has a pointer receiver, so `m` must be `&ecsModel{...}`
(matches `tui_test.go`'s `&model{...}` pattern exactly).

**Verify**: `go test -v ./internal/tui/... -run TestECSApplyFilter` → both
tests and all 7 `TestECSApplyFilter` subtests PASS.

### Step 4: Full verification pass

Run the full command set from "Commands you will need" in order: build,
vet, gofmt check, then the combined new-test filter, then the full suite.

**Verify**:
- `go build -v ./...` → exit 0
- `go vet ./...` → exit 0, no output
- `gofmt -l .` → no output
- `go test -v ./... -run 'TestPickerModelUpdate|TestECSApplyFilter'` → all
  new subtests PASS
- `go test ./...` → all packages `ok`, no failures anywhere (confirms no
  regression in the rest of the suite)

## Test plan

- New tests, `internal/tui/picker_test.go`: `TestPickerModelUpdate` with 8
  subtests covering cursor up/down movement and clamping at both bounds,
  Enter-selects-and-quits, Enter-on-empty-list-is-safe, and Esc/Ctrl+C-quits.
- New tests, `internal/tui/ecs_test.go`: `TestECSApplyFilter` (7 subtests:
  empty filter, filter by each of the three matched fields, case
  insensitivity, no-match, partial match) and `TestECSApplyFilterCursorReset`
  (1 test: cursor clamps to 0 when the filtered list shrinks below the
  current cursor position).
- Structural pattern to follow: `internal/tui/tui_test.go`'s
  `TestApplyFilter`/`TestApplyFilterCursorReset` (existing, EC2-instance
  version of the same filter logic).
- Verification: `go test -v ./... -run 'TestPickerModelUpdate|TestECSApplyFilter'`
  → all new subtests pass; `go test ./...` → full suite green, no
  regressions.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/tui/picker_test.go` exists with `TestPickerModelUpdate`
      (8 subtests per Step 2)
- [ ] `internal/tui/ecs_test.go` exists with `TestECSApplyFilter` (7
      subtests) and `TestECSApplyFilterCursorReset`
- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./... -run 'TestPickerModelUpdate|TestECSApplyFilter'`
      shows every new subtest as PASS
- [ ] `go test ./...` shows every package `ok` (no regressions)
- [ ] `git status` shows only `internal/tui/picker_test.go` and
      `internal/tui/ecs_test.go` as new/changed — no production `.go` files
      touched
- [ ] `plans/README.md` status row for plan 029 updated (unless a reviewer
      told you they own the index)

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/tui/picker.go` or `internal/tui/ecs.go` doesn't
  match the excerpts in "Current state" (the codebase drifted since this
  plan was written) — report the actual current code instead of silently
  adapting tests to it.
- Writing the empty-items Enter test (Step 2, case 6) actually panics —
  this would mean the `if len(m.items) > 0` guard shown in "Current state"
  is no longer present in `picker.go`. Do not add the guard yourself; report
  the panic and the current code as a real bug for a separate fix plan.
- Any assertion about `tea.KeyMsg`, `tea.Key`, `tea.Quit`, or `tea.QuitMsg`
  in this plan does not match what `go doc github.com/charmbracelet/bubbletea.<Type>`
  reports for the pinned `v1.3.10` — re-verify with `go doc` and if the
  actual API differs from what's written here (e.g. field names, whether
  `Quit` is a func or a var), stop and report the discrepancy rather than
  guessing at a fix.
- A verification command in Step 4 fails twice after a reasonable fix
  attempt confined to the two new test files.
- Fixing a failing test appears to require touching `picker.go`, `ecs.go`,
  or any other production file — that is out of scope for this plan.

## Maintenance notes

- If `pickerModel` or `ecsModel.applyFilter` gain new fields or branches in
  future work (e.g. a page-size/scroll-window feature, or filtering by an
  additional field), extend the corresponding test file's cases rather than
  rewriting it — keep the one-subtest-per-behavior structure.
- A reviewer of the resulting PR should check: no production `.go` files
  changed, the two new test files use `t.Run` subtests consistently with
  `tui_test.go`'s existing style, and the `tea.QuitMsg` assertion pattern
  (call the returned `cmd()` and check its result type) is used consistently
  rather than just checking `cmd != nil` (which would be weaker and could
  pass even if `Update` returned the wrong kind of command).
- Deferred, not in this plan's scope: `ecsModel.handleKeys`'s search-typing
  path (`KeyBackspace`/`KeyRunes` cases in `internal/tui/ecs.go` lines
  120-129) and the loading-state key handling in `ecsModel.Update` (lines
  89-97) are also untested, but they involve more setup (constructing a full
  `ecsModel` with `mode`/`loadFunc`) and were judged lower-value than the
  pure `applyFilter` logic covered here — a candidate for a follow-up plan
  if broader `ecsModel` coverage is wanted later.
