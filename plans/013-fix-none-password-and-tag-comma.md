# Plan 013: Fix "None" password bug and comma-in-tag-value bug

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat a3d40be..HEAD -- internal/aws/ec2.go main.go`
> If either file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: S
- **Risk**: LOW
- **Depends on**: none (if plan 006 also touches `runRDP`'s password branch,
  see the note in plan 006's "STOP conditions" about composing the two — do
  either plan first, then re-read the current state of that block before
  applying the other)
- **Category**: bug
- **Planned at**: commit `a3d40be`, 2026-07-15

## Why this matters

Two independent, small correctness bugs in `internal/aws/ec2.go`:

1. **`GetPasswordData` treats AWS CLI's literal `"None"` text output as a
   real password.** When `aws ec2 get-password-data --output text --query
   PasswordData` selects a null field (the common case when a Windows
   instance's password hasn't been generated yet, or domain auth is used),
   AWS CLI's `--output text` mode prints the literal string `None` — not an
   empty string. `GetPasswordData` (`internal/aws/ec2.go:158-183`) returns
   this verbatim, and `runRDP` (`main.go:876-885`) checks `password != ""`
   to decide whether to show the "password retrieved" branch vs. the "no
   password data available" branch. Since `"None"` is non-empty, users see
   `Administrator password: None` — a nonsense value presented as if it were
   a real credential, and the intended fallback message becomes practically
   unreachable for this common case.

2. **Tag values containing a comma are silently misinterpreted by the AWS
   CLI filter syntax.** `ListRunningInstances` and `ListWindowsInstances`
   (`internal/aws/ec2.go:41-97`, `:99-156`) build EC2 filter strings via
   `fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], parts[1])`. AWS CLI's
   shorthand filter syntax treats a bare `,` inside `Values=...` as a
   value-list separator. A legitimate EC2 tag value containing a comma
   (e.g. `"prod, us-east"` — commas are valid in AWS tag values) gets
   reinterpreted as two separate filter values (`Values=prod, us-east` →
   matches instances tagged either `prod` OR `us-east`), silently widening
   the `--tag` filter beyond what the user intended. This is a data-
   integrity bug (wrong instances returned), not a shell-injection risk
   (the argv is passed via `exec.Command`'s slice, never through a shell).

Both fixes are small, independent, and localized to `internal/aws/ec2.go`.

## Current state

`internal/aws/ec2.go:158-183` (`GetPasswordData`, full function):

```go
func GetPasswordData(instanceID, profile, region, keyPath string) (string, error) {
	args := []string{"ec2", "get-password-data",
		"--instance-id", instanceID,
		"--priv-launch-key", keyPath,
		"--output", "text",
		"--query", "PasswordData",
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

	return strings.TrimSpace(string(out)), nil
}
```

`main.go:876-885` (the caller, inside `runRDP`):

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

`internal/aws/ec2.go:41-51` (`ListRunningInstances`, the tag-filter
construction — `ListWindowsInstances` at lines 100-110 has the identical
pattern):

```go
func ListRunningInstances(profile, region string, tags []string) ([]Instance, error) {
	args := []string{"ec2", "describe-instances",
		"--filters", "Name=instance-state-name,Values=running",
	}

	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], parts[1]))
		}
	}

	args = append(args, "--output", "json")
	...
```

AWS CLI's documented shorthand-syntax escaping rule: a literal comma inside
a shorthand value is escaped by preceding it with a backslash (`\,`). This
is the standard, documented AWS CLI shorthand escaping mechanism (not a
project-specific workaround) — see AWS CLI's "Quoting Strings" /
"escaping commas" documentation for `--filters`/similar shorthand
parameters.

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build ./...` | exit 0 |
| Test (this package) | `go test ./internal/aws/... -v` | exit 0, all pass, including new tests |
| Full test suite | `go test ./...` | exit 0, all pass |

## Scope

**In scope** (the only files you should modify):
- `internal/aws/ec2.go` (`GetPasswordData`, `ListRunningInstances`,
  `ListWindowsInstances`)
- `internal/aws/ec2_test.go` (add tests for both fixes)

**Out of scope** (do NOT touch, even though they look related):
- `main.go`'s `runRDP` — the `password != ""` check there does not need to
  change if you fix `GetPasswordData` to return `""` instead of `"None"` at
  the source (see Step 1 — fixing it at the source is simpler and correct;
  do not also add a `"None"` check in `main.go`, that would be redundant and
  is explicitly not needed once Step 1 lands). If plan 006 has already
  changed this block to add a `--show-password` flag, do not revert that
  change — only verify the `password != ""` condition still behaves
  correctly given `GetPasswordData`'s fix.
- Switching `--output text` to `--output json` for `GetPasswordData` — that
  would be a larger change to the parsing approach; the literal-string
  comparison fix in Step 1 is narrower and sufficient.
- Switching the tag-filter construction from AWS CLI shorthand syntax to
  JSON-based `--filters file://...` syntax — that would be a larger
  structural change; escaping the comma per AWS CLI's documented shorthand
  rule is narrower and sufficient.
- `internal/aws/rds.go`, `internal/aws/ecs.go`, `internal/aws/logs.go` — no
  tag-filtering or password-parsing logic exists in these files; out of
  scope.

## Git workflow

- Branch: `advisor/013-fix-none-password-and-tag-comma`
- Single commit (both fixes are small and in the same file; splitting into
  2 commits is also acceptable); message style example:
  `fix: treat AWS CLI "None" text output as no password data, escape commas in tag filters`
- Do NOT push or open a PR unless the operator instructed it.

## Steps

### Step 1: Fix the "None" password bug

In `internal/aws/ec2.go`, change the return statement of `GetPasswordData`
(line 182) from:

```go
	return strings.TrimSpace(string(out)), nil
```

to:

```go
	result := strings.TrimSpace(string(out))
	if result == "None" {
		return "", nil
	}
	return result, nil
}
```

Wait — since this changes the shape of the function body, here is the full
corrected function for clarity:

```go
func GetPasswordData(instanceID, profile, region, keyPath string) (string, error) {
	args := []string{"ec2", "get-password-data",
		"--instance-id", instanceID,
		"--priv-launch-key", keyPath,
		"--output", "text",
		"--query", "PasswordData",
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

	result := strings.TrimSpace(string(out))
	if result == "None" {
		return "", nil
	}
	return result, nil
}
```

This makes the fix at the source: `GetPasswordData` now returns `""` (not
`"None"`) when AWS has no password data yet, so `main.go`'s existing
`password != ""` check (`main.go:880`, or wherever it lives after plan 006's
changes, if that plan ran first) correctly falls through to the "No password
data available" branch without any change needed in `main.go`.

**Verify**: `go build ./internal/aws/...` → exit 0.

### Step 2: Fix the comma-in-tag-value bug

In `internal/aws/ec2.go`, both `ListRunningInstances` (line 49) and
`ListWindowsInstances` (line 108) contain:

```go
			args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], parts[1]))
```

Change both occurrences to escape literal commas in the tag value (AWS CLI
shorthand syntax escapes a literal comma with a backslash):

```go
			escapedValue := strings.ReplaceAll(parts[1], ",", "\\,")
			args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], escapedValue))
```

Apply this identically in both functions (`ListRunningInstances` and
`ListWindowsInstances`) — they currently have byte-identical tag-filter
loops, so the same 2-line change applies to both.

**Verify**: `go build ./internal/aws/...` → exit 0.

### Step 3: Add tests for the "None" password fix

Add to `internal/aws/ec2_test.go`. Since `GetPasswordData` shells out to the
real `aws` CLI via `exec.Command`, you cannot unit-test the full function
without mocking the subprocess — which this codebase doesn't currently do
anywhere (confirmed: no test in this repo mocks `exec.Command`). Testing the
literal-string-comparison logic in isolation requires extracting it into a
small pure helper. Add this helper to `internal/aws/ec2.go`, and use it from
`GetPasswordData`:

```go
// normalizePasswordOutput converts AWS CLI's literal "None" text output
// (produced when --query selects a null field under --output text) into an
// empty string, so callers can use a simple non-empty check.
func normalizePasswordOutput(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "None" {
		return ""
	}
	return trimmed
}
```

And simplify `GetPasswordData`'s return to:

```go
	return normalizePasswordOutput(string(out)), nil
```

(replacing the inline `if result == "None"` block from Step 1 with a call to
this new helper — this is the same fix, just structured to be testable).

Then add the test:

```go
func TestNormalizePasswordOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"real password", "P@ssw0rd123!\n", "P@ssw0rd123!"},
		{"literal None", "None\n", ""},
		{"literal None no newline", "None", ""},
		{"empty string", "", ""},
		{"whitespace only", "   \n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePasswordOutput(tt.raw); got != tt.want {
				t.Errorf("normalizePasswordOutput(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test ./internal/aws/... -run TestNormalizePasswordOutput -v` →
all subtests pass.

### Step 4: Add tests for the comma-escaping fix

Similarly, extract the escaping logic into a small pure helper so it's
testable without mocking `exec.Command`. Add to `internal/aws/ec2.go`:

```go
// escapeTagFilterValue escapes characters that are significant in AWS CLI
// shorthand filter syntax (commas separate multiple filter values) so a
// literal comma in a tag value is not misinterpreted as a value separator.
func escapeTagFilterValue(value string) string {
	return strings.ReplaceAll(value, ",", "\\,")
}
```

Update both `ListRunningInstances` and `ListWindowsInstances` to call
`escapeTagFilterValue(parts[1])` instead of the inline `strings.ReplaceAll`
call from Step 2 (same fix, now via a named, testable helper):

```go
			args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], escapeTagFilterValue(parts[1])))
```

Add the test:

```go
func TestEscapeTagFilterValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no comma", "production", "production"},
		{"single comma", "prod,us-east", "prod\\,us-east"},
		{"multiple commas", "a,b,c", "a\\,b\\,c"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeTagFilterValue(tt.in); got != tt.want {
				t.Errorf("escapeTagFilterValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
```

**Verify**: `go test ./internal/aws/... -run TestEscapeTagFilterValue -v` →
all subtests pass.

### Step 5: Run the full package and repo test suites

**Verify**: `go test ./internal/aws/... -v` → all pass, including the
existing `TestDisplayName` and the two new tests from Steps 3-4.

**Verify**: `go test ./...` → exit 0, all packages pass.

## Test plan

- New tests in `internal/aws/ec2_test.go`: `TestNormalizePasswordOutput`
  (table-driven: real password, literal `"None"` with/without trailing
  newline, empty string, whitespace-only) and `TestEscapeTagFilterValue`
  (table-driven: no comma, single comma, multiple commas, empty string).
- Both follow the existing table-driven pattern in this same file
  (`TestDisplayName`, `internal/aws/ec2_test.go:5-21`).
- Neither test requires mocking `exec.Command` — the fix is deliberately
  structured (via the two new pure helper functions) so the actual bug
  logic is unit-testable without a real AWS CLI call.
- Verification: `go test ./internal/aws/... -v` → all pass.

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `internal/aws/ec2.go` has `normalizePasswordOutput` and
      `escapeTagFilterValue` helper functions
- [ ] `GetPasswordData` calls `normalizePasswordOutput` on its result
- [ ] `ListRunningInstances` and `ListWindowsInstances` both call
      `escapeTagFilterValue` on tag values before building the filter string
- [ ] `go test ./internal/aws/... -v` exits 0, all pass, including
      `TestNormalizePasswordOutput` and `TestEscapeTagFilterValue`
- [ ] `go build ./...` exits 0
- [ ] `go test ./...` exits 0, all pass
- [ ] `git status` shows only `internal/aws/ec2.go` and
      `internal/aws/ec2_test.go` modified
- [ ] `plans/README.md` status row updated

## STOP conditions

Stop and report back (do not improvise) if:

- The code at `internal/aws/ec2.go:158-183`, `:41-51`, or `:99-110` doesn't
  match the excerpts above (drift since this plan was written).
- A step's verification fails twice after a reasonable fix attempt.
- Plan 006 has already modified `main.go`'s `runRDP` password-printing
  block in a way that assumes `GetPasswordData` still returns literal
  `"None"` (unlikely, since plan 006 explicitly says to preserve whatever
  `password != ""` condition exists — but verify this before finishing) — if
  you find an inconsistency, STOP and report the actual current state of
  both functions rather than guessing how to reconcile them.

## Maintenance notes

- If `GetPasswordData` is ever changed to use `--output json` instead of
  `--output text` (a larger, separate change explicitly out of scope here),
  `normalizePasswordOutput`'s `"None"`-string check would no longer be
  needed — a JSON `null` would parse to a real empty/nil value instead. That
  refactor should remove this workaround if it happens.
- A reviewer should scrutinize: that `escapeTagFilterValue` is applied
  consistently to both `ListRunningInstances` and `ListWindowsInstances` —
  a future third function with the same tag-filter pattern should also call
  this helper rather than reintroducing the inline `fmt.Sprintf` without
  escaping.
