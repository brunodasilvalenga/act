# Plan 016: Add `act ssm run` — execute commands/scripts on an instance via SSM Run Command

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md`.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- main.go internal/aws/ internal/config/config.go README.md`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts below against the live code before proceeding; on
> a mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: MED (new remote-execution capability; must be careful with JSON
  parameter encoding and exit-code propagation)
- **Depends on**: none
- **Category**: feature
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

Today `act` can open an interactive SSM shell (`act ec2`), SSH over SSM
(`act ec2 ssh`), or exec into an ECS container (`act ecs`) — all *interactive*
sessions. There is no way to run a one-off command or a local script against
an EC2 instance non-interactively (e.g. for ad-hoc ops tasks, quick checks, or
scripting `act` itself from CI). AWS Systems Manager already supports this
via `ssm send-command` with the `AWS-RunShellScript` / `AWS-RunPowerShellScript`
documents — `act` just needs to wrap it with the same instance-picker UX the
other commands already have. This closes that gap with a single new
subcommand, `act ssm run`, following the existing `ec2 ssh` / `ec2 rdp`
nested-subcommand pattern.

## Current state

Relevant files, each with its role:

- `main.go` — subcommand dispatch (`switch subcmd` at line 43), per-command
  `runX`/`printXHelp` functions, and the shared `parseTags` helper for
  repeatable flags.
- `internal/aws/ec2.go` — `Instance` struct (has a `Platform` field, empty
  string for Linux, `"windows"` for Windows instances) and the
  `aws ec2 describe-instances` wrapper `ListRunningInstances`. Every AWS-CLI
  wrapper in this package follows the same shape: build `args []string`,
  append `--profile`/`--region` if set, `exec.Command("aws", args...)`,
  `cmd.Output()`, and on `*exec.ExitError` return
  `fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))`.
- `internal/tui/tui.go` — `func Run(loadFunc func() ([]aws.Instance, error)) (*aws.Instance, error)`,
  the interactive instance picker used by `runConnect`, `runForward`,
  `runSSH`, `runRDP`. Reuse this, not `tui.RunPicker` (that one is for
  picking from a flat `[]string`, used for cluster/service names).
- `internal/aws/rdp_unix.go` — shows the existing pattern for building an SSM
  JSON `--parameters` payload with `fmt.Sprintf`:

  ```go
  // internal/aws/rdp_unix.go:16-17
  document := "AWS-StartPortForwardingSession"
  params := fmt.Sprintf(`{"portNumber":["3389"],"localPortNumber":["%d"]}`, localPort)
  ```

  **Do not copy this `fmt.Sprintf` pattern for this plan.** Those existing
  calls only ever interpolate integers/hostnames with no special JSON
  characters. The new `commands` parameter will contain arbitrary
  user-supplied shell text (quotes, backslashes, newlines) — building that
  JSON with `fmt.Sprintf` would produce invalid JSON or let a crafted command
  string smuggle extra SSM parameters. Build the `--parameters` payload with
  `encoding/json.Marshal` instead (see Step 1).

- `main.go:465-479` — `parseTags`, the existing pattern for a repeatable CLI
  flag that Go's `flag` package doesn't support natively:

  ```go
  // main.go:465-479
  func parseTags(args []string) ([]string, []string) {
      var remaining []string
      var tags []string
      for i := 0; i < len(args); i++ {
          if args[i] == "--tag" || args[i] == "-tag" {
              if i+1 < len(args) {
                  i++
                  tags = append(tags, args[i])
              }
          } else {
              remaining = append(remaining, args[i])
          }
      }
      return remaining, tags
  }
  ```

  The new `--command` flag (repeatable, one shell line per occurrence) needs
  the same treatment — add a sibling `parseCommands` function, do not
  generalize/refactor `parseTags` (out of scope, see below).

- `main.go:82-98` — the nested-subcommand dispatch pattern to copy for `ssm run`
  (this is how `ec2 ssh` / `ecs logs` are wired):

  ```go
  // main.go:82-98
  case "ecs":
      subArgs := args[1:]
      if hasHelp(subArgs) {
          printECSHelp()
          os.Exit(0)
      }
      resolvedProfile := config.ResolveProfile(profile, env)
      resolvedRegion := config.ResolveRegion(region, env)
      if len(subArgs) > 0 && subArgs[0] == "logs" {
          if hasHelp(subArgs[1:]) {
              printECSLogsHelp()
              os.Exit(0)
          }
          runLogs(resolvedProfile, resolvedRegion, subArgs[1:])
      } else {
          runECS(resolvedProfile, resolvedRegion, subArgs)
      }
  ```

- `internal/aws/ec2_test.go` — the only existing test file in `internal/aws`;
  it tests the pure `Instance.DisplayName()` method only (no AWS-CLI calls
  are unit tested anywhere in this repo — that's the established ceiling for
  test coverage in this package). Match that scope: only unit-test pure
  helper functions you add, not `exec.Command` calls.
- `README.md` — per `CLAUDE.md`'s rule ("After adding a new feature, command,
  or flag, always update README.md..."), the Features list (lines 9-26),
  Prerequisites/IAM permissions (line 32), Commands table (lines 75-87),
  and Examples section (lines 98-174) must all be updated.

## Commands you will need

| Purpose | Command | Expected on success |
|---------|---------|---------------------|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output (no unformatted files) |
| Test | `go test -v ./...` | all tests pass, exit 0 |

(Verified against this repo's `.github/workflows/ci.yml`, which runs
`go build -v ./...` and `go test -v ./...` on ubuntu/macos/windows.)

## Scope

**In scope**:
- `internal/aws/ssm.go` (new file)
- `internal/aws/ssm_test.go` (new file)
- `main.go` (add flags/dispatch/help — additive only)
- `README.md` (Features, Prerequisites, Commands table, Examples)
- `plans/README.md` (status row update when done)

**Out of scope** (do NOT touch, even though related):
- Any change to `internal/aws/session_unix.go` / `session_windows.go` /
  `rdp_unix.go` / `rdp_windows.go` — those implement *interactive* SSM
  sessions via `syscall.Exec`/process replacement; `send-command` is a
  fire-and-poll HTTP-style API call and needs none of that machinery.
- Multi-instance fan-out / broadcasting a command to a tag-filtered group of
  instances in one invocation — this plan is single-target only, matching
  `ec2 ssh`/`ec2 rdp`. Note it in Maintenance notes as a natural follow-up.
- Refactoring `parseTags` into a generic "parse repeated flag" helper — add
  a sibling `parseCommands` function instead (see Current state). Don't
  generalize; that's plan 010's territory (extracting shared arg-building)
  and touching it here risks conflicting with that plan.
- Any TUI picker changes in `internal/tui/` — this plan only reuses
  `tui.Run`, it doesn't add a new picker.

## Git workflow

- Branch: no strong convention observed (repo history shows direct commits
  to `main`); if your environment requires a branch, use
  `feat/ssm-run-command`.
- Commit message style (from `git log`): Conventional Commits, e.g.
  `feat: add ec2 rdp command for Windows RDP via SSM`. Use:
  `feat: add ssm run command to execute commands via SSM Run Command`
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Add `internal/aws/ssm.go`

Create `internal/aws/ssm.go` with three functions, following this package's
existing conventions (see `internal/aws/ec2.go`, `internal/aws/rds.go` for
the exact `exec.Command`/error-wrapping shape):

```go
package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DocumentForPlatform returns the SSM Run Command document for the given
// EC2 Platform value ("windows" for Windows instances, "" or "linux" otherwise).
func DocumentForPlatform(platform string) string {
	if strings.EqualFold(platform, "windows") {
		return "AWS-RunPowerShellScript"
	}
	return "AWS-RunShellScript"
}

type sendCommandOutput struct {
	Command struct {
		CommandID string `json:"CommandId"`
	} `json:"Command"`
}

// SendCommand submits an SSM Run Command against a single instance and
// returns the resulting command ID.
func SendCommand(instanceID, profile, region, document string, commands []string, timeoutSeconds int, comment string) (string, error) {
	params := map[string][]string{"commands": commands}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to encode command parameters: %w", err)
	}

	args := []string{"ssm", "send-command",
		"--document-name", document,
		"--instance-ids", instanceID,
		"--parameters", string(paramsJSON),
		"--timeout-seconds", fmt.Sprintf("%d", timeoutSeconds),
		"--output", "json",
	}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}

	var result sendCommandOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse send-command output: %w", err)
	}
	return result.Command.CommandID, nil
}

type commandInvocationOutput struct {
	Status                 string `json:"Status"`
	StandardOutputContent  string `json:"StandardOutputContent"`
	StandardErrorContent   string `json:"StandardErrorContent"`
	ResponseCode           int    `json:"ResponseCode"`
}

// CommandInvocationResult is the outcome of a completed (or still-running,
// on error) SSM command invocation.
type CommandInvocationResult struct {
	Status   string
	Stdout   string
	Stderr   string
	ExitCode int
}

// WaitForCommandInvocation polls ssm get-command-invocation until the
// command reaches a terminal status (Success, Failed, Cancelled, TimedOut).
func WaitForCommandInvocation(commandID, instanceID, profile, region string, pollInterval time.Duration) (CommandInvocationResult, error) {
	args := []string{"ssm", "get-command-invocation",
		"--command-id", commandID,
		"--instance-id", instanceID,
		"--output", "json",
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	for {
		cmd := exec.Command("aws", args...)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return CommandInvocationResult{}, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return CommandInvocationResult{}, err
		}

		var result commandInvocationOutput
		if err := json.Unmarshal(out, &result); err != nil {
			return CommandInvocationResult{}, fmt.Errorf("failed to parse get-command-invocation output: %w", err)
		}

		switch result.Status {
		case "Pending", "InProgress", "Delayed":
			time.Sleep(pollInterval)
			continue
		default:
			return CommandInvocationResult{
				Status:   result.Status,
				Stdout:   result.StandardOutputContent,
				Stderr:   result.StandardErrorContent,
				ExitCode: result.ResponseCode,
			}, nil
		}
	}
}
```

Notes:
- `--instance-ids` (not `--targets`) is used deliberately: this plan is
  single-target only, and `--instance-ids` is the simpler, still-supported
  parameter for that case.
- The `time.Sleep(pollInterval)` busy-poll loop matches this repo's existing
  style of straightforward, unbuffered blocking calls (see
  `internal/aws/rdp_unix.go:41`, `time.Sleep(2 * time.Second)`) — no need
  for context cancellation or backoff.

**Verify**: `go build ./internal/aws/...` → exit 0.

### Step 2: Add `internal/aws/ssm_test.go`

Unit-test only the pure function `DocumentForPlatform` (no AWS-CLI calls),
matching the scope and structure of the existing `internal/aws/ec2_test.go`:

```go
package aws

import "testing"

func TestDocumentForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "windows lowercase", platform: "windows", want: "AWS-RunPowerShellScript"},
		{name: "windows mixed case", platform: "Windows", want: "AWS-RunPowerShellScript"},
		{name: "empty is linux", platform: "", want: "AWS-RunShellScript"},
		{name: "linux explicit", platform: "linux", want: "AWS-RunShellScript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DocumentForPlatform(tt.platform)
			if got != tt.want {
				t.Errorf("DocumentForPlatform(%q) = %q, want %q", tt.platform, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test ./internal/aws/... -run TestDocumentForPlatform -v` →
all subtests pass.

### Step 3: Add `parseCommands` and the `ssm run` help text in `main.go`

Add near `parseTags` (main.go:465-479), a sibling function — do not modify
`parseTags` itself:

```go
// parseCommands extracts --command flags from args (can be repeated,
// one shell line per occurrence), returns remaining args and commands.
func parseCommands(args []string) ([]string, []string) {
	var remaining []string
	var commands []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--command" || args[i] == "-command" {
			if i+1 < len(args) {
				i++
				commands = append(commands, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, commands
}
```

Add a help function following the style of `printEC2SSHHelp` (main.go:339-364):

```go
func printSSMHelp() {
	fmt.Fprintf(os.Stderr, `act ssm - Execute commands via SSM Run Command

Usage: act [global flags] ssm [subcommand|flags]

Subcommands:
  run          Run a command or script on an instance (see 'act ssm run help')

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name
`)
}

func printSSMRunHelp() {
	fmt.Fprintf(os.Stderr, `act ssm run - Run a command or script on an EC2 instance via SSM

Usage: act [global flags] ssm run [flags]

Runs one or more shell commands (or a local script file) on a target
instance via AWS Systems Manager Run Command, waits for completion, and
prints stdout/stderr. Automatically uses AWS-RunPowerShellScript for
Windows instances and AWS-RunShellScript for everything else.

Flags:
  --target       Target instance ID (skip instance picker)
  --command      Command to run (repeatable; each occurrence is one line)
  --script       Path to a local script file to run (mutually exclusive with --command)
  --timeout      Command timeout in seconds (default 300)
  --comment      Optional comment shown in the Systems Manager console
  --no-wait      Submit the command and exit without waiting for it to finish
  --tag          Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act ssm run --command "systemctl status nginx"
  act ssm run --target i-0123456789abcdef0 --command "df -h" --command "uptime"
  act ssm run --script ./deploy.sh --timeout 600
  act ssm run --no-wait --command "sudo reboot"
`)
}
```

**Verify**: `go build ./...` → exit 0 (new functions are unused until Step 4
wires them in, so build this together with Step 4, not in isolation — Go
will not error on unused top-level functions, only unused local vars/imports).

### Step 4: Add `runSSMRun` and wire up dispatch in `main.go`

Add `runSSMRun`, modeled directly on `runSSH` (main.go:779-818) for the
instance-picker/tag-filter flow, and on `runRDP`'s flag-parsing style
(main.go:850-858):

```go
func runSSMRun(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)
	subArgs, commands := parseCommands(subArgs)

	fs := flag.NewFlagSet("ssm run", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	script := fs.String("script", "", "Path to a local script file to run")
	timeout := fs.Int("timeout", 300, "Command timeout in seconds")
	comment := fs.String("comment", "", "Optional comment shown in the Systems Manager console")
	noWait := fs.Bool("no-wait", false, "Submit the command and exit without waiting")
	fs.Parse(subArgs)

	if len(commands) == 0 && *script == "" {
		fmt.Fprintf(os.Stderr, "Error: provide at least one --command or --script\n")
		os.Exit(1)
	}
	if len(commands) > 0 && *script != "" {
		fmt.Fprintf(os.Stderr, "Error: --command and --script are mutually exclusive\n")
		os.Exit(1)
	}

	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading script file: %v\n", err)
			os.Exit(1)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		commands = lines
	}

	instanceID := *target
	var platform string
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		selected, err := tui.Run(loadFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if selected == nil {
			os.Exit(0)
		}
		instanceID = selected.InstanceID
		platform = selected.Platform
	}

	document := aws.DocumentForPlatform(platform)

	commandID, err := aws.SendCommand(instanceID, profile, region, document, commands, *timeout, *comment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Command %s submitted to %s\n", commandID, instanceID)

	if *noWait {
		return
	}

	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error waiting for command: %v\n", err)
		os.Exit(1)
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}

	if result.Status != "Success" {
		fmt.Fprintf(os.Stderr, "Command finished with status %s\n", result.Status)
		os.Exit(1)
	}
}
```

Important: when `instanceID` comes from `--target` (no picker run), `platform`
stays `""`, which `aws.DocumentForPlatform` maps to `AWS-RunShellScript`
(Linux). This means `--target` on a Windows instance requires the caller to
know that limitation. Note it in the help text's Flags description for
`--target` — append `(Linux assumed; use the picker to target Windows
instances)` to that line in `printSSMRunHelp`, and mention it in Maintenance
notes below.

Add the `"time"` import to `main.go`'s import block (it currently imports
`bufio`, `flag`, `fmt`, `os`, `os/exec`, `strings`, plus the four internal
packages — `time` is not yet imported).

Wire the dispatch into the `switch subcmd` block (main.go:43), as a new
`case` alongside `case "ecs":` (main.go:82) — same nested-subcommand shape
as `ecs logs`:

```go
case "ssm":
	subArgs := args[1:]
	if hasHelp(subArgs) {
		printSSMHelp()
		os.Exit(0)
	}
	resolvedProfile := config.ResolveProfile(profile, env)
	resolvedRegion := config.ResolveRegion(region, env)
	if len(subArgs) > 0 && subArgs[0] == "run" {
		if hasHelp(subArgs[1:]) {
			printSSMRunHelp()
			os.Exit(0)
		}
		runSSMRun(resolvedProfile, resolvedRegion, subArgs[1:])
	} else {
		fmt.Fprintf(os.Stderr, "Unknown ssm subcommand. Run 'act ssm help' for usage.\n")
		os.Exit(1)
	}
```

Also add `ssm run` to the `Commands:` list inside `printUsage()`
(main.go:193-204), right after the `rds` line:

```
  rds          Port forward to RDS instance via SSM
  ssm run      Run a command or script on an instance via SSM
```

**Verify**:
1. `gofmt -l main.go` → no output.
2. `go build ./...` → exit 0.
3. `go vet ./...` → no output.
4. `./act ssm run help` (after `go build -o act .`) → prints the help text
   from Step 3 and exits 0.
5. `./act ssm help` → prints the `printSSMHelp` text and exits 0.

### Step 5: Update `README.md`

Per `CLAUDE.md`'s rule, update all four sections:

1. **Features** (README.md:9-26) — add a bullet after the "SSH over SSM" line:
   ```
   - **Run Command** — Execute a command or script on an instance via SSM Run Command (no session needed)
   ```

2. **Prerequisites / IAM permissions** (README.md:32) — append
   `ssm:SendCommand`, `ssm:GetCommandInvocation` to the permissions list.

3. **Commands table** (README.md:75-87) — add a row after the `rds` row:
   ```
   | `ssm run` | Run a command or script on an instance via SSM |
   ```

4. **Examples** (README.md:98-174) — add a new subsection after the
   "Tail ECS service logs" examples (after README.md:153) and before the
   "Exec into an ECS container" examples, following the existing comment +
   command-line style:
   ```bash
   # Run a command on an instance (interactive picker)
   act ssm run --command "systemctl status nginx"

   # Run multiple commands on a specific instance
   act ssm run --target i-0123456789abcdef0 --command "df -h" --command "uptime"

   # Run a local script
   act ssm run --script ./deploy.sh --timeout 600

   # Fire-and-forget (don't wait for completion)
   act ssm run --no-wait --command "sudo reboot"
   ```

**Verify**: `grep -n "ssm run" README.md` → at least 4 matches (Features
bullet is worded differently, so check separately: `grep -n "Run Command" README.md`
→ at least 1 match).

## Test plan

- `internal/aws/ssm_test.go` — `TestDocumentForPlatform` (Step 2), covering:
  windows lowercase, windows mixed-case, empty string, explicit `"linux"`.
  Modeled on `internal/aws/ec2_test.go`'s `TestDisplayName` table-test shape.
- No test is added for `SendCommand`/`WaitForCommandInvocation` — consistent
  with this repo's established ceiling (no `exec.Command`-based AWS-CLI
  wrapper anywhere in `internal/aws` has a test; only pure functions do).
- Verification: `go test ./... -v` → all existing tests still pass, plus the
  new `TestDocumentForPlatform` subtests.

## Done criteria

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test ./... -v` exits 0, includes passing `TestDocumentForPlatform` subtests
- [ ] `./act ssm run help` and `./act ssm help` print the new help text and exit 0
- [ ] `grep -n "func runSSMRun" main.go` finds the new function
- [ ] `grep -n "ssm run" README.md` finds the new Commands-table row and example section
- [ ] Only files listed in Scope are modified (`git status`)
- [ ] `plans/README.md` status row for plan 016 updated to DONE

## STOP conditions

Stop and report back (do not improvise) if:
- `main.go`'s `switch subcmd` block, `parseTags`, or the `runSSH`/`runRDP`
  functions look materially different from the excerpts above (the codebase
  has drifted since this plan was written at commit `a3d40be`).
- `aws ssm send-command`/`aws ssm get-command-invocation` are not available
  in the AWS CLI version used to test (they require a reasonably current
  AWS CLI v2; if `aws ssm send-command help` fails, stop and report rather
  than downgrading the feature).
- You find `internal/aws/ssm.go` or a `DocumentForPlatform`/`SendCommand`
  symbol already exists (possible partial prior implementation) — reconcile
  with what's there rather than overwriting blindly.
- A step's verification fails twice after a reasonable fix attempt.

## Maintenance notes

- **Windows target via `--target`**: as noted in Step 4, when a caller
  supplies `--target` directly (bypassing the picker), `act` has no way to
  know the instance's platform and defaults to the Linux document. A future
  enhancement could call `aws ec2 describe-instances --instance-ids <id>` to
  look up the platform when `--target` is used, but that adds a network call
  to what is otherwise a synchronous flag-only path — deliberately deferred.
- **Multi-instance fan-out**: this plan is single-target only. Extending to
  a tag-filtered group (SSM's `--targets Key=tag:Name,Values=...`) and
  aggregating per-instance results is a natural follow-up but a materially
  bigger UX/output-formatting problem (multiple instances' stdout/stderr
  need to be attributed and interleaved sensibly) — left for a separate plan.
- **Command output size**: `StandardOutputContent`/`StandardErrorContent`
  from `get-command-invocation` are truncated by AWS at 24KB. If a future
  need arises for larger output, `aws ssm get-command-invocation` output
  would need to be supplemented with S3/CloudWatch output redirection
  (`--output-s3-bucket-name` / CloudWatch logging on the document) — out of
  scope here, note only.
- A reviewer should scrutinize: the `encoding/json.Marshal` use for the
  `--parameters` payload (this is the one place in the codebase choosing
  `json.Marshal` over `fmt.Sprintf` for SSM parameters — intentional, see
  Current state) and the exit-code propagation logic (`result.Status !=
  "Success"` → `os.Exit(1)`).
