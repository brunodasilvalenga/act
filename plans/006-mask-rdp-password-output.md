# Plan 006: Stop printing the decrypted Windows Administrator password to stdout

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- main.go`
> If the file changed since this plan was written, compare the
> "Current state" excerpt against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

`act ec2 rdp --key <path>` decrypts and prints the target Windows instance's
Administrator password directly to stdout with no masking:
`fmt.Printf("Administrator password: %s\n", password)`
(`main.go:881`). This plaintext credential then persists in terminal
scrollback, any terminal-recording/session-logging tool, shell history if
the command's output is redirected, and screen-sharing during support
sessions — a durable copy of a high-value credential outside any credential
store, for the entire lifetime of that terminal session or log.

The fix: require an explicit opt-in flag (`--show-password`) to print the
password at all; by default, mask it and tell the user how to reveal it. This
keeps the feature available (some users genuinely want to paste the password
somewhere) while making accidental plaintext exposure require a deliberate
choice rather than being the default.

## Current state

`main.go:850-893` (full `runRDP` function):

```go
func runRDP(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("rdp", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	localPort := fs.Int("local-port", 3389, "Local port")
	key := fs.String("key", "", "Path to private key for password decryption")
	noOpen := fs.Bool("no-open", false, "Don't auto-open RDP client")
	fs.Parse(subArgs)

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListWindowsInstances(profile, region, tags)
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
	}

	if *key != "" {
		password, err := aws.GetPasswordData(instanceID, profile, region, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not retrieve password: %v\n", err)
		} else if password != "" {
			fmt.Printf("Administrator password: %s\n", password)
		} else {
			fmt.Println("No password data available (instance may use domain auth or password not yet generated).")
		}
	}

	fmt.Printf("Starting RDP port forward to %s (localhost:%d → 3389)\n", instanceID, *localPort)
	err := aws.StartRDP(instanceID, profile, region, *localPort, !*noOpen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

The password-printing block is lines 876-885. Note: plan `013` separately
fixes a bug where AWS CLI's literal `"None"` text output is misinterpreted
as a real password by the `password != ""` check on line 880 — that fix is
independent of this plan's masking behavior and is tracked separately; if
plan `013` has already executed, the `password != ""` check may have
changed to also exclude `"None"` — if so, preserve that check exactly and
only change what happens in the "print it" branch, not the condition that
selects that branch.

The corresponding help text is at `main.go:820-848` (`printEC2RDPHelp`):

```go
func printEC2RDPHelp() {
	fmt.Fprintf(os.Stderr, `act ec2 rdp - RDP to Windows EC2 instance via SSM

Usage: act [global flags] ec2 rdp [flags]

Starts a port forwarding session to port 3389 on a Windows EC2 instance
and optionally opens your RDP client.

Only Windows instances are shown in the picker.

Flags:
  --target       Target instance ID (skip instance picker)
  --local-port   Local port (default: 3389)
  --key          Path to private key for password decryption
  --no-open      Don't auto-open RDP client
  --tag          Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act ec2 rdp
  act ec2 rdp --key ~/.ssh/my-key.pem
  act ec2 rdp --no-open --local-port 13389
  act ec2 rdp --target i-0123456789abcdef0
`)
}
```

This needs a new `--show-password` flag documented alongside `--key`.

Per `CLAUDE.md`: "After adding a new feature, command, or flag, always
update README.md" — this plan's Step 3 covers the required README update.

`README.md`'s `ec2 rdp` section is in the Commands/Examples area
(search for `act ec2 rdp` — it appears in the Examples block under `RDP to
a Windows instance via SSM`).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test | `go test ./...` | exit 0, all pass |
| Confirm help text | `./act ec2 rdp --help` (after `go build`) | shows new `--show-password` flag |

## Scope

**In scope** (the only files you should modify):
- `main.go` (`runRDP` function and `printEC2RDPHelp` function only)
- `README.md` (the `ec2 rdp` examples block only)

**Out of scope** (do NOT touch, even though they look related):
- `internal/aws/ec2.go` (`GetPasswordData`) — no change to how the password
  is retrieved, only to what `main.go` does with it after retrieval. (Plan
  `013` changes `GetPasswordData`'s literal-`"None"` handling — a different,
  independent concern; do not conflate the two.)
- Clipboard integration (e.g. auto-copying the password to the OS clipboard)
  — that would require a new dependency and is a larger feature than this
  plan's scope; this plan only adds a masked-by-default / explicit-opt-in
  print behavior.
- Any other `run*` function in `main.go`.

## Git workflow

- Branch: `advisor/006-mask-rdp-password-output`
- Single commit; message style example: `feat: add ec2 rdp command for Windows RDP via SSM` (commit `a3d40be`, for
  feature-shaped precedent) — for this change:
  `fix: require --show-password to print RDP admin password in plaintext`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Add the `--show-password` flag

In `runRDP` (`main.go:850-858`), add a new flag alongside the existing ones:

```go
	fs := flag.NewFlagSet("rdp", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	localPort := fs.Int("local-port", 3389, "Local port")
	key := fs.String("key", "", "Path to private key for password decryption")
	noOpen := fs.Bool("no-open", false, "Don't auto-open RDP client")
	showPassword := fs.Bool("show-password", false, "Print the decrypted password to stdout (default: masked)")
	fs.Parse(subArgs)
```

**Verify**: `go build ./...` → exit 0.

### Step 2: Change the password-printing branch to mask by default

Replace the block at `main.go:876-885`:

```go
	if *key != "" {
		password, err := aws.GetPasswordData(instanceID, profile, region, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not retrieve password: %v\n", err)
		} else if password != "" {
			fmt.Printf("Administrator password: %s\n", password)
		} else {
			fmt.Println("No password data available (instance may use domain auth or password not yet generated).")
		}
	}
```

with:

```go
	if *key != "" {
		password, err := aws.GetPasswordData(instanceID, profile, region, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not retrieve password: %v\n", err)
		} else if password != "" {
			if *showPassword {
				fmt.Printf("Administrator password: %s\n", password)
			} else {
				fmt.Println("Administrator password retrieved (use --show-password to display it in plaintext).")
			}
		} else {
			fmt.Println("No password data available (instance may use domain auth or password not yet generated).")
		}
	}
```

If plan `013` has already executed and changed the `password != ""`
condition (e.g. to `password != "" && password != "None"`), preserve that
exact condition and only wrap the "print it" branch as shown above — do not
revert plan `013`'s fix.

**Verify**: `go build ./...` → exit 0.

### Step 3: Update the help text

In `printEC2RDPHelp` (`main.go:820-848`), add the new flag to the `Flags:`
block, immediately after `--key`:

```
  --target       Target instance ID (skip instance picker)
  --local-port   Local port (default: 3389)
  --key          Path to private key for password decryption
  --show-password  Print the decrypted password in plaintext (default: masked)
  --no-open      Don't auto-open RDP client
  --tag          Filter instances by tag (key=value, can be repeated)
```

**Verify**: `go build -o act . && ./act ec2 rdp --help 2>&1 | grep -- '--show-password'` →
returns a match. (Remove the built `act` binary afterward if it's not
already gitignored — check `.gitignore`; confirmed it already contains a
bare `act` entry, so this is safe to leave for local testing but should not
be committed.)

### Step 4: Update README

Find the `ec2 rdp` example block in `README.md` (search for `act ec2 rdp`
under the Examples section). It currently reads:

```
# RDP to a Windows instance via SSM (auto-opens RDP client on macOS/Windows)
act ec2 rdp
act ec2 rdp --key ~/.ssh/my-key.pem
act ec2 rdp --no-open --local-port 13389
act ec2 rdp --target i-0123456789abcdef0
```

Add a line documenting the new flag's interaction with `--key`:

```
# RDP to a Windows instance via SSM (auto-opens RDP client on macOS/Windows)
act ec2 rdp
act ec2 rdp --key ~/.ssh/my-key.pem                  # retrieves password (masked by default)
act ec2 rdp --key ~/.ssh/my-key.pem --show-password  # prints password in plaintext
act ec2 rdp --no-open --local-port 13389
act ec2 rdp --target i-0123456789abcdef0
```

**Verify**: `grep -n "show-password" README.md` → returns a match.

### Step 5: Run the full test suite

**Verify**: `go test ./...` → exit 0, all packages pass (this change has no
new unit-testable logic beyond a conditional print, so no new test file is
required — see Test plan below for why).

## Test plan

This change is a `main` package CLI-behavior change with no pure/isolable
logic beyond a boolean flag gate (`if *showPassword { ... } else { ... }`),
and `main.go` currently has zero test coverage with no established pattern
for testing `run*` functions' stdout output (confirmed: no `main_test.go`
exists). Adding a full CLI-output test harness for this one flag is out of
proportion to the fix and is not required by this plan — the existing
`go test ./...` full-suite run (Step 5) is the verification gate, combined
with the manual `--help` grep check in Step 3 confirming the flag is wired
up. If a future plan adds `main_test.go` infrastructure (see plan `011`'s
maintenance notes), a golden-output test for `runRDP`'s masked-vs-shown
branches would be a good addition then.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `runRDP` defines and parses a `--show-password` bool flag
- [ ] The password-printing branch only prints the plaintext password when
      `*showPassword` is true; otherwise it prints a message referencing
      `--show-password` without revealing the value
- [ ] `printEC2RDPHelp`'s output includes `--show-password`
- [ ] `README.md` documents `--show-password` in the `ec2 rdp` examples
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `git status` shows only `main.go` and `README.md` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- `main.go:850-893` (`runRDP`) doesn't match the excerpt above (drift since
  this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- Plan `013` has executed and its change to the `password != ""` condition
  is structured in a way that doesn't compose cleanly with this plan's
  `if *showPassword` wrapper — if so, STOP and report the actual current
  code rather than guessing how to merge the two.

## Maintenance notes

- If clipboard integration is added later (a natural follow-up, not in this
  plan's scope), it should default to copying instead of the current masked
  message, while `--show-password` should still exist as an explicit
  plaintext-to-stdout escape hatch for users who want to pipe/redirect it.
- A reviewer should scrutinize: that the default (no `--show-password`)
  behavior never leaks the password into any output stream, including
  stderr warnings.
- This plan intentionally leaves `GetPasswordData` itself unchanged; see
  plan `013` for the separate `"None"`-string bug in that function.
