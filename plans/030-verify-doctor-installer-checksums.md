# Plan 030: Verify AWS CLI / Session Manager plugin downloads before `doctor --fix` installs them (Linux — confirmed available), document the gap elsewhere (macOS/Windows — confirmed unavailable)

> **Executor instructions**: Follow this plan step by step. Run every
> verification command and confirm the expected result before moving to the
> next step. If anything in the "STOP conditions" section occurs, stop and
> report — do not improvise. When done, update the status row for this plan
> in `plans/README.md` — unless a reviewer dispatched you and told you they
> maintain the index.
>
> **Drift check (run first)**: `git diff --stat aa50614..HEAD -- internal/doctor/`
> If any in-scope file changed since this plan was written, compare the
> "Current state" excerpts against the live code before proceeding; on a
> mismatch, treat it as a STOP condition.

## Status

- **Priority**: P2
- **Effort**: M
- **Risk**: LOW-MED (network-dependent investigation already done and recorded below; the implementation itself is low risk because it is scoped only to the variants confirmed to have real upstream verification material)
- **Depends on**: none
- **Category**: security
- **Planned at**: commit `aa50614`, 2026-07-21

## Why this matters

`act doctor --fix` downloads the AWS CLI v2 installer and the Session
Manager plugin bundle from hardcoded AWS URLs and immediately hands the
downloaded file to `sudo installer`, `sudo dpkg -i`, `sudo rpm -i`, or
`msiexec` — with no integrity check anywhere in that path today. This repo
already has an established, better pattern for exactly this class of risk:
`internal/updater/updater.go`'s `Upgrade` function (see `verifyChecksum`,
lines 163–206) refuses to install `act`'s own upgrade binary if a
`checksums.txt` isn't found and doesn't match — it fails closed. The
`doctor --fix` installers, added later, never adopted that posture. This
plan investigates, with live checks against the real AWS endpoints (not
assumptions), which of the 8 download call sites (4 AWS CLI variants + 4
SSM plugin variants) actually have verification material AWS publishes at a
stable URL, and wires in verification for exactly those — while explicitly
NOT blocking the variants where AWS doesn't publish anything to verify
against. Shipping partial, honest coverage now is strictly better than
shipping nothing while waiting for 100% coverage that AWS's own download
infrastructure doesn't currently support.

## Investigation findings (already done — do not redo this research)

This section records the actual result of hitting AWS's real endpoints and
reading AWS's real documentation, done in preparation for this plan. Treat
these as ground truth, subject only to the drift re-check in Step 1.

### AWS CLI v2 (4 variants: macOS pkg, Linux x86_64, Linux aarch64, Windows msi)

| Variant | Download URL (current code) | Verification available? | Evidence |
|---|---|---|---|
| macOS `.pkg` | `https://awscli.amazonaws.com/AWSCLIV2.pkg` | **NO** | `curl -sI https://awscli.amazonaws.com/AWSCLIV2.pkg.sig` → `404`. Also tried `.sha256`, `.asc` → both `404`. The AWS CLI install docs page (`getting-started-install.html`) documents GPG signature verification only for the Linux `.zip` bundle; the macOS section (GUI installer / command-line installer) contains no verification step at all. |
| Linux `x86_64` `.zip` | `https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip` | **YES** | `curl -sI https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip.sig` → `200 OK`, `Content-Type: binary/octet-stream`. AWS's docs describe this exact `.sig` sidecar and give a GPG public key to verify it (quoted below). |
| Linux `aarch64` `.zip` | `https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip` | **YES** | `curl -sI https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip.sig` → `200 OK`. Same public key as x86_64 (AWS signs both with the same "AWS CLI Team" key). |
| Windows `.msi` | `https://awscli.amazonaws.com/AWSCLIV2.msi` | **NO** | `curl -sI https://awscli.amazonaws.com/AWSCLIV2.msi.sig` → `404`. The Windows section of the install docs has no verification step. |

The verification mechanism AWS documents for the Linux zip is a **detached
PGP/GPG signature**, not a sha256 checksum manifest — it is a different
shape than `updater.go`'s `checksums.txt` parsing and cannot reuse
`verifyChecksum` as-is. AWS's public key for this (from
`https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html`):

```
-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBF2Cr7UBEADJZHcgusOJl7ENSyumXh85z0TRV0xJorM2B/JL0kHOyigQluUG
ZMLhENaG0bYatdrKP+3H91lvK050pXwnO/R7fB/FSTouki4ciIx5OuLlnJZIxSzx
PqGl0mkxImLNbGWoi6Lto0LYxqHN2iQtzlwTVmq9733zd3XfcXrZ3+LblHAgEt5G
TfNxEKJ8soPLyWmwDH6HWCnjZ/aIQRBTIQ05uVeEoYxSh6wOai7ss/KveoSNBbYz
gbdzoqI2Y8cgH2nbfgp3DSasaLZEdCSsIsK1u05CinE7k2qZ7KgKAUIcT/cR/grk
C6VwsnDU0OUCideXcQ8WeHutqvgZH1JgKDbznoIzeQHJD238GEu+eKhRHcz8/jeG
94zkcgJOz3KbZGYMiTh277Fvj9zzvZsbMBCedV1BTg3TqgvdX4bdkhf5cH+7NtWO
lrFj6UwAsGukBTAOxC0l/dnSmZhJ7Z1KmEWilro/gOrjtOxqRQutlIqG22TaqoPG
fYVN+en3Zwbt97kcgZDwqbuykNt64oZWc4XKCa3mprEGC3IbJTBFqglXmZ7l9ywG
EEUJYOlb2XrSuPWml39beWdKM8kzr1OjnlOm6+lpTRCBfo0wa9F8YZRhHPAkwKkX
XDeOGpWRj4ohOx0d2GWkyV5xyN14p2tQOCdOODmz80yUTgRpPVQUtOEhXQARAQAB
tCFBV1MgQ0xJIFRlYW0gPGF3cy1jbGlAYW1hem9uLmNvbT6JAlQEEwEIAD4CGwMF
CwkIBwIGFQoJCAsCBBYCAwECHgECF4AWIQT7Xbd/1cEYuAURraimMQrMRnJHXAUC
akV0ygUJDqP4lQAKCRCmMQrMRnJHXFHjD/9eyZLYcKuQOlLvtqSDtUBiEZf6ZZjM
i3ygYH8rJNtuToUH+HvSpe819urJCquXhDrlK6N+aqW0hCLtNABJG/vsafIgvIYJ
hSGgpgtNnQyMV1jViRWqPjbouw8OkYKBThUfT1i2Y+wn58ifs6ODBCmTexWtXspA
Si+Gt49xDOW0APmbOPnI+a4HJW6tVEo6MWS0WjzpiBayR3d1A4pt4YrPfSdDgpLo
h2SLQqlRqvvVZJaWBjhkErNFpfsBA06sDcPEOb0G8LBUbR4WOcdvhe5LubJbZuxC
AG9kNPCVeQP1ixwjgjXKysaxeQ6rv0VzIQgRp6tLVLWhy6AKDNvLjFSsmXZ1Wl08
Y/RlOHXlzLuQMRE6sR1wOdRxc9TsrNWTGiBK65cvSWOy03JeBkQQ8pesqltiyxI9
U21kkgiXtTSKNGfKK8pO27D81YANhRqPK7iTp6kuFiY2WtOg90KTMNlIT+Ff85Y2
b1rHj6Z0SrCkJujhWk3IBPic/wJgz01LEc/OAdUPlby90RJZcIBhSlWhT7mXnXIO
c0HWlNQrns2s3CTyYwZSiSlYe9ApeLwhjDo8NhbFuCAy61l6O5UsR4AfZxx/rGKv
2wFb1/RN/P4gNe6vmxZAPjR0AQcwD3tc2McimOLr/22kmPz8IH3I0X7WoSFr0Biz
E91G7bb0hOb/cA==
=knv7
-----END PGP PUBLIC KEY BLOCK-----
```

Key ID: `A6310ACC4672475C`. Fingerprint: `FB5D B77F D5C1 18B8 0511  ADA8 A631 0ACC 4672 475C`.

### Session Manager plugin (4 variants: macOS zip bundle, Linux deb, Linux rpm, Windows exe)

| Variant | Download URL (current code) | Verification available? | Evidence |
|---|---|---|---|
| macOS `sessionmanager-bundle.zip` | `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/mac/sessionmanager-bundle.zip` | **NO** | `curl -sI .../mac/sessionmanager-bundle.zip.sig` → `403` (S3 returns 403, not 404, for missing keys in this bucket — confirmed not present). AWS's docs (`install-plugin-macos-overview.html`) explicitly describe *two different* macOS install paths: a "signed installer" (`session-manager-plugin.pkg`, a *different* file/URL) and the "bundled installer" (`sessionmanager-bundle.zip`, what the current code downloads). Only the signed `.pkg` path is described as signed; the bundle `.zip` has no documented verification. |
| Linux `.deb` (`ubuntu_64bit`) | `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/ubuntu_64bit/session-manager-plugin.deb` | **YES** | `curl -sI .../ubuntu_64bit/session-manager-plugin.deb.sig` → `200 OK`. AWS has a dedicated docs page, [Verify the signature of the Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/install-plugin-linux-verify-signature.html), describing exactly this `.sig` sidecar and a public key (quoted below). |
| Linux `.rpm` (`linux_64bit`) | `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/linux_64bit/session-manager-plugin.rpm` | **YES** | `curl -sI .../linux_64bit/session-manager-plugin.rpm.sig` → `200 OK`. Same docs page and public key as `.deb`. |
| Windows `.exe` | `https://s3.amazonaws.com/session-manager-downloads/plugin/latest/windows/SessionManagerPluginSetup.exe` | **NO** | `curl -sI .../windows/SessionManagerPluginSetup.exe.sig` → `403` (not present). The SSM plugin docs' topic list has no "verify signature" page for Windows (unlike Linux). |

Again this is a **detached PGP/GPG signature**, not a sha256 manifest. AWS's
public key for the Linux `.deb`/`.rpm` (from
`https://docs.aws.amazon.com/systems-manager/latest/userguide/install-plugin-linux-verify-signature.html`,
which states it applies to "Session Manager plugin versions 1.2.707.0 or later" —
current `latest` is confirmed well above that):

```
-----BEGIN PGP PUBLIC KEY BLOCK-----

mFIEZ5ERQxMIKoZIzj0DAQcCAwQjuZy+IjFoYg57sLTGhF3aZLBaGpzB+gY6j7Ix
P7NqbpXyjVj8a+dy79gSd64OEaMxUb7vw/jug+CfRXwVGRMNtIBBV1MgU1NNIFNl
c3Npb24gTWFuYWdlciA8c2Vzc2lvbi1tYW5hZ2VyLXBsdWdpbi1zaWduZXJAYW1h
em9uLmNvbT4gKEFXUyBTeXN0ZW1zIE1hbmFnZXIgU2Vzc2lvbiBNYW5hZ2VyIFBs
dWdpbiBMaW51eCBTaWduZXIgS2V5KYkBAAQQEwgAqAUCZ5ERQ4EcQVdTIFNTTSBT
ZXNzaW9uIE1hbmFnZXIgPHNlc3Npb24tbWFuYWdlci1wbHVnaW4tc2lnbmVyQGFt
YXpvbi5jb20+IChBV1MgU3lzdGVtcyBNYW5hZ2VyIFNlc3Npb24gTWFuYWdlciBQ
bHVnaW4gTGludXggU2lnbmVyIEtleSkWIQR5WWNxJM4JOtUB1HosTUr/b2dX7gIe
AwIbAwIVCAAKCRAsTUr/b2dX7rO1AQCa1kig3lQ78W/QHGU76uHx3XAyv0tfpE9U
oQBCIwFLSgEA3PDHt3lZ+s6m9JLGJsy+Cp5ZFzpiF6RgluR/2gA861M=
=2DQm
-----END PGP PUBLIC KEY BLOCK-----
```

Key ID: `2C4D4AFF6F6757EE`. Fingerprint: `7959 6371 24CE 093A D501 D47A 2C4D 4AFF 6F67 57EE`.

### Design decision this plan bakes in (do not re-litigate without reason)

Verify by shelling out to the system `gpg` binary against a hardcoded,
embedded public key — the same mechanism AWS's own docs instruct humans to
use — rather than vendoring a Go OpenPGP library (e.g.
`github.com/ProtonMail/go-crypto`). Reasoning: (a) it matches AWS's
officially documented, supported verification path exactly, so a reviewer
can cross-check this code against AWS's docs line by line; (b) it adds zero
new entries to `go.mod` in a repo that currently has exactly two direct
dependencies; (c) `gpg` is commonly preinstalled on Linux (the only OS this
plan wires verification into). The accepted trade-off: on a Linux box
without `gpg` on `PATH`, `doctor --fix` for AWS CLI / SSM plugin will now
refuse to install (fail closed) where it previously would have installed
unverified — this is an intentional, documented behavior change, not a bug.
The error message must tell the user how to install `gpg` (`sudo apt-get
install gnupg2` / `sudo yum install gnupg2`, per AWS's own docs) so they
aren't stuck.

## Current state

- `internal/doctor/install_awscli.go` — `installAWSCLIDarwin` (line 45),
  `installAWSCLILinux` (line 65), `installAWSCLIWindows` (line 101),
  `describeAWSCLIInstall` (line 14). Also defines `unzipTo` and
  `isWithinDir` (shared zip-slip guard, out of scope here).
- `internal/doctor/install_ssmplugin.go` — `installSSMPluginDarwin` (line
  44), `installSSMPluginLinux` (line 76), `installSSMPluginWindows` (line
  119), `describeSSMPluginInstall` (line 12). Also defines `commandExists`
  (line 139) — reuse this, do not duplicate it.
- `internal/doctor/download.go` — `downloadToTempFile(url, pattern string)
  (string, error)` (the whole file, 37 lines) — the shared download
  primitive both installer files call. Reuse it for downloading `.sig`
  files too.
- `internal/doctor/doctor.go` — wires `describeAWSCLIInstall`/`installAWSCLI`
  and `describeSSMPluginInstall`/`installSSMPlugin` into `fixAction.Describe`
  / `fixAction.Apply` (lines 111–112, 137–138). Not modified by this plan,
  but read it to understand how `Describe()` text reaches the user: it is
  printed by `runFixes` in `internal/doctor/fix.go` before the install
  confirmation prompt, so any "no verification available" text you add to
  `describeAWSCLIInstall`/`describeSSMPluginInstall` is what the user sees
  before they approve the fix.
- Today's `installAWSCLILinux` (relevant excerpt):
  ```go
  func installAWSCLILinux(w io.Writer) error {
      url := "https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip"
      if runtime.GOARCH == "arm64" {
          url = "https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip"
      }
      fmt.Fprintf(w, "Downloading AWS CLI v2 install bundle from %s...\n", url)
      zipPath, err := downloadToTempFile(url, "awscli-exe-linux-*.zip")
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(zipPath)

      tmpDir, err := os.MkdirTemp("", "awscli-install-*")
      // ... unzip zipPath into tmpDir, then sudo ./aws/install
  ```
- Today's `installSSMPluginLinux` (relevant excerpt):
  ```go
  func installSSMPluginLinux(w io.Writer) error {
      // ... choose rpm or deb URL based on which of dpkg/rpm exists ...
      fmt.Fprintf(w, "Downloading Session Manager plugin package from %s...\n", url)
      pkgPath, err := downloadToTempFile(url, pattern)
      if err != nil {
          return fmt.Errorf("download failed: %w", err)
      }
      defer os.Remove(pkgPath)

      var cmd *exec.Cmd
      if useRpm {
          cmd = exec.Command("sudo", "rpm", "-i", pkgPath)
      } else {
          cmd = exec.Command("sudo", "dpkg", "-i", pkgPath)
      }
      // ... cmd.Run() ...
  ```
- `internal/updater/updater.go` — read for context only, **do not modify**.
  It already fails closed on missing checksums (lines 84–89) and its
  `verifyChecksum` (lines 163–206) is the repo's precedent for "fail closed
  by default," but its sha256/`checksums.txt` parsing logic does **not**
  apply here — GPG signature verification is a different mechanism.
- `gpg` was **not found** on the machine this plan was written on
  (`darwin/arm64`, confirmed via `which gpg` → not found). Do not assume
  `gpg` is present anywhere; the new code must detect its absence and fail
  closed with an actionable message, and tests must skip gracefully when
  it's absent (see Test plan).

## Commands you will need

| Purpose | Command | Expected on success |
|---|---|---|
| Build | `go build -v ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0, no output |
| Format check | `gofmt -l .` | no output |
| Test | `go test -v ./...` | all pass (`ok` for every package) |
| Re-verify a sig URL is still live | `curl -sI <url>.sig` | `HTTP/1.1 200` |
| Check for gpg locally | `which gpg` or `command -v gpg` | path if present; nonzero exit if absent — either is fine, just don't assume |

## Scope

**In scope**:
- `internal/doctor/install_awscli.go` — wire verification into
  `installAWSCLILinux`; update `describeAWSCLIInstall` text for all three
  OS cases.
- `internal/doctor/install_ssmplugin.go` — wire verification into
  `installSSMPluginLinux`; update `describeSSMPluginInstall` text for all
  three OS cases.
- `internal/doctor/gpgverify.go` (new file) — the shared GPG-verification
  helper and the two embedded public keys.
- `internal/doctor/gpgverify_test.go` (new file) — tests for the helper.

**Out of scope** (do NOT touch):
- `internal/updater/updater.go` — separate, already-correct system for
  `act`'s own self-upgrade. Its checksum mechanism (sha256 +
  `checksums.txt`) is architecturally different from what AWS publishes for
  these installers; do not try to reuse `verifyChecksum` directly.
- `installAWSCLIDarwin`, `installAWSCLIWindows`, `installSSMPluginDarwin`,
  `installSSMPluginWindows` — no verification material exists upstream for
  these four variants (confirmed in Step 1). Only touch their `Describe`
  text, not their `Apply` logic.
- Switching the macOS SSM plugin install from the bundle `.zip` to the
  signed `.pkg` (which *does* have a documented, different verification
  story) — that's a bigger change (different install mechanism, different
  install paths) genuinely out of scope for this plan. Note it in
  Maintenance notes as a real follow-up opportunity, don't do it here.
- Adding any new entry to `go.mod` — the design decision above is to shell
  out to `gpg`, not vendor a PGP library. If you find yourself wanting to
  add a dependency, STOP and reconsider (see STOP conditions).

## Git workflow

- Branch: `advisor/030-verify-doctor-installer-checksums`
- Conventional Commits style, matching recent history, e.g.:
  `fix: let act doctor run without AWS CLI already installed`,
  `feat: add doctor --fix to auto-remediate missing AWS CLI / Session Manager plugin`
- Suggested commits: one for the new `gpgverify.go` + its tests, one for
  wiring it into `install_awscli.go`, one for wiring it into
  `install_ssmplugin.go` (or combine the last two — reviewer's call).
- Do NOT push or open a PR unless explicitly instructed.

## Steps

### Step 1: Re-confirm the investigation findings are still current

The findings above were captured on 2026-07-21. AWS's "latest" URLs point
at whatever the current release is, so re-run these four checks and confirm
they still return `200`:

```
curl -sI https://awscli.amazonaws.com/awscli-exe-linux-x86_64.zip.sig
curl -sI https://awscli.amazonaws.com/awscli-exe-linux-aarch64.zip.sig
curl -sI https://s3.amazonaws.com/session-manager-downloads/plugin/latest/linux_64bit/session-manager-plugin.rpm.sig
curl -sI https://s3.amazonaws.com/session-manager-downloads/plugin/latest/ubuntu_64bit/session-manager-plugin.deb.sig
```

**Verify**: all four return `HTTP/1.1 200`. If any returns 404/403, STOP —
do not guess a replacement URL; report which one broke and what status it
returned.

### Step 2: Create `internal/doctor/gpgverify.go`

Create the file with:

1. Two exported-within-package constants holding the exact PGP public key
   blocks quoted in "Investigation findings" above, named
   `awsCLILinuxSigningKey` and `ssmPluginLinuxSigningKey`. Copy them
   byte-for-byte from this plan (including the blank line after `BEGIN PGP
   PUBLIC KEY BLOCK-----`) — do not re-fetch or retype them from memory. Add
   a doc comment above each with the source URL, key ID, and fingerprint
   (also quoted above), so a reviewer can cross-check without running gpg.

2. Two pure, no-I/O functions split so the core logic is testable without
   network access:

   ```go
   // verifyGPGSignatureFile verifies that sigPath is a valid detached GPG
   // signature of artifactPath, made by a key in publicKeyBlock. It trusts
   // only publicKeyBlock — never the caller's default GPG keyring — by
   // running gpg with an isolated, temporary GNUPGHOME. It fails closed:
   // any error (gpg not on PATH, key import failure, bad/missing signature)
   // is returned as a non-nil error and the caller must not proceed to
   // install the artifact.
   func verifyGPGSignatureFile(artifactPath, sigPath, publicKeyBlock string) error {
       if !commandExists("gpg") {
           return fmt.Errorf("gpg not found on PATH; install it (e.g. 'sudo apt-get install gnupg2' or 'sudo yum install gnupg2') to allow signature verification, or install manually")
       }

       gnupgHome, err := os.MkdirTemp("", "act-gpg-*")
       if err != nil {
           return fmt.Errorf("failed to create temp GNUPGHOME: %w", err)
       }
       defer os.RemoveAll(gnupgHome)
       if err := os.Chmod(gnupgHome, 0700); err != nil {
           return fmt.Errorf("failed to set GNUPGHOME permissions: %w", err)
       }

       keyFile := filepath.Join(gnupgHome, "key.asc")
       if err := os.WriteFile(keyFile, []byte(publicKeyBlock), 0600); err != nil {
           return fmt.Errorf("failed to write public key: %w", err)
       }

       importCmd := exec.Command("gpg", "--batch", "--homedir", gnupgHome, "--import", keyFile)
       if out, err := importCmd.CombinedOutput(); err != nil {
           return fmt.Errorf("gpg import failed: %w (%s)", err, string(out))
       }

       verifyCmd := exec.Command("gpg", "--batch", "--homedir", gnupgHome, "--verify", sigPath, artifactPath)
       if out, err := verifyCmd.CombinedOutput(); err != nil {
           return fmt.Errorf("gpg signature verification failed: %w\n%s", err, string(out))
       }

       return nil
   }

   // fetchAndVerifyGPGSignature downloads the detached signature at sigURL
   // to a temp file and verifies it against artifactPath using
   // verifyGPGSignatureFile. It writes progress to w.
   func fetchAndVerifyGPGSignature(w io.Writer, artifactPath, sigURL, publicKeyBlock string) error {
       fmt.Fprintf(w, "Downloading signature from %s...\n", sigURL)
       sigPath, err := downloadToTempFile(sigURL, "act-sig-*.sig")
       if err != nil {
           return fmt.Errorf("failed to download signature: %w", err)
       }
       defer os.Remove(sigPath)

       if err := verifyGPGSignatureFile(artifactPath, sigPath, publicKeyBlock); err != nil {
           return err
       }
       fmt.Fprintln(w, "GPG signature verified.")
       return nil
   }
   ```

   `commandExists` is already defined in `install_ssmplugin.go` in the same
   package — do not redefine it.

**Verify**: `go build -v ./...` → exit 0.

### Step 3: Wire verification into `installAWSCLILinux`

In `internal/doctor/install_awscli.go`, in `installAWSCLILinux`, immediately
after the existing `defer os.Remove(zipPath)` line and before `tmpDir, err
:= os.MkdirTemp(...)`, insert:

```go
	if err := fetchAndVerifyGPGSignature(w, zipPath, url+".sig", awsCLILinuxSigningKey); err != nil {
		return fmt.Errorf("signature verification failed, refusing to install: %w", err)
	}
```

This applies to both the x86_64 and aarch64 URLs since `url` already
branches on `runtime.GOARCH` earlier in the function and `.sig` is appended
to whichever it resolved to.

**Verify**: `go build -v ./...` → exit 0. `grep -n "fetchAndVerifyGPGSignature" internal/doctor/install_awscli.go` → one match.

### Step 4: Wire verification into `installSSMPluginLinux`

In `internal/doctor/install_ssmplugin.go`, in `installSSMPluginLinux`,
immediately after the existing `defer os.Remove(pkgPath)` line and before
`var cmd *exec.Cmd`, insert:

```go
	if err := fetchAndVerifyGPGSignature(w, pkgPath, url+".sig", ssmPluginLinuxSigningKey); err != nil {
		return fmt.Errorf("signature verification failed, refusing to install: %w", err)
	}
```

This applies to both the rpm and deb branches since `url` is already
resolved to one or the other earlier in the function.

**Verify**: `go build -v ./...` → exit 0. `grep -n "fetchAndVerifyGPGSignature" internal/doctor/install_ssmplugin.go` → one match.

### Step 5: Document the gap for the 4 unverifiable variants

Update `describeAWSCLIInstall()` in `install_awscli.go`:
- `darwin` case: append to the returned string something like
  `" (no integrity verification is available for this download — AWS does not publish a signature or checksum for the macOS .pkg installer)"`.
- `windows` case: same, for the `.msi`.
- `linux` case: update wording to reflect that a GPG signature is now
  checked, e.g. mention `"...and verifies its GPG signature before installing"`.

Update `describeSSMPluginInstall()` in `install_ssmplugin.go` the same way:
- `darwin` case: note no verification is available for the bundle `.zip`
  download this code uses.
- `windows` case: note no verification is available for the `.exe`.
- `linux` case: mention the GPG signature check now happens.

Keep each string on one logical sentence addition — these are shown to the
user in the fix confirmation prompt (via `fix.go`'s `runFixes`), so keep
them readable, not a wall of text.

**Verify**: `go build -v ./...` → exit 0. Manually read the 6 updated
strings and confirm they're accurate and readable.

### Step 6: Write tests in `internal/doctor/gpgverify_test.go`

See Test plan below for exact cases. All tests that invoke the real `gpg`
binary must open with:

```go
if !commandExists("gpg") {
    t.Skip("gpg not found on PATH; skipping GPG signature verification test")
}
```

**Verify**: `go test -v ./... -run GPG` → all cases pass or skip (none fail).

### Step 7: Full verification pass

Run the full command set from "Commands you will need" and confirm all
pass (build, vet, gofmt, test).

## Test plan

Write `internal/doctor/gpgverify_test.go`. Model the "pure logic separated
from I/O" split on `updater.go`'s `selectReleaseAssets` (pure, tested
directly) vs `downloadToTemp`/`verifyChecksum` (I/O, not needed by name
here, just the separation pattern). All tests below target
`verifyGPGSignatureFile` (no network needed) except where noted; they all
require a locally-generated, throwaway test keypair — never AWS's real key
(you don't have AWS's private key, obviously) — to test the *mechanics* of
verification.

To generate a throwaway test keypair and signature inside a test, in a
`t.TempDir()`-free location (see the flakiness note below) use:

```sh
export GNUPGHOME=<a short, fixed-length temp dir, NOT nested under t.TempDir()>
gpg --batch --passphrase '' --pinentry-mode loopback --quick-generate-key "Test Key <test@example.com>" default default never
gpg --batch --homedir "$GNUPGHOME" --armor --export "Test Key" > pub.asc
gpg --batch --homedir "$GNUPGHOME" --pinentry-mode loopback --passphrase '' --detach-sign artifact.bin   # produces artifact.bin.sig
```

Translate this into `os/exec` calls from the test using `t.TempDir()` for
the artifact/signature files (fine — only the *GNUPGHOME* needs to be
short) but a plain `os.MkdirTemp("", "act-gpgtest-*")` (cleaned up with
`defer os.RemoveAll`) for `GNUPGHOME` itself, to avoid GPG's well-known
`gpg-agent` Unix-socket-path-length limit (~108 chars on some platforms)
that deeply-nested `t.TempDir()` paths (`/tmp/TestFoo/001/...`) can exceed.
If key generation or signing still fails with a socket/path error after
using a short `GNUPGHOME`, treat that specific failure as a known,
acceptable test flakiness risk — skip that test with a clear message rather
than spending more than one troubleshooting pass on it (see STOP
conditions).

Cases:
1. **Happy path** — generate a throwaway keypair, sign a small artifact
   file, export the public key, call `verifyGPGSignatureFile(artifact, sig,
   pubkey)`. Expect `nil` error.
2. **Tampered artifact** — same setup, then append one byte to the artifact
   file after signing, before calling verify. Expect a non-nil error.
3. **Wrong key** — sign with keypair A, call verify passing keypair B's
   exported public key. Expect a non-nil error.
4. **gpg missing** — `t.Setenv("PATH", <empty or a dir with no gpg>)`, call
   `verifyGPGSignatureFile` with any arguments. Expect a non-nil error whose
   message contains `"gpg not found"`. This test does NOT need the
   `commandExists("gpg")` skip guard — it's specifically testing the
   missing-gpg path, so it should run unconditionally.
5. **(Optional, network-dependent, do not block Done Criteria on it)** — a
   test that calls `fetchAndVerifyGPGSignature` against one of the real
   confirmed-live sig URLs from Step 1, skipped both if `gpg` is absent and
   if network access is unavailable in the executor's sandbox. This is a
   nice-to-have integration smoke test, not required for Done Criteria.

Verification: `go test -v ./internal/doctor/... -run GPG` → cases 1–4 pass
or skip cleanly (skips are fine and expected on runners without `gpg`,
e.g. Windows CI runners per `.github/workflows/ci.yml`'s
`windows-latest` matrix entry — verify this yourself rather than trusting
this claim, by checking whether `gpg` is present in that runner image or
just letting the skip guard handle it either way).

## Done criteria

Machine-checkable. ALL must hold:

- [ ] `go build -v ./...` exits 0
- [ ] `go vet ./...` exits 0 with no output
- [ ] `gofmt -l .` produces no output
- [ ] `go test -v ./...` exits 0 (new GPG tests pass or skip, nothing fails)
- [ ] `internal/doctor/gpgverify.go` exists, exporting (package-private)
      `awsCLILinuxSigningKey`, `ssmPluginLinuxSigningKey`,
      `verifyGPGSignatureFile`, `fetchAndVerifyGPGSignature`
- [ ] `grep -c "fetchAndVerifyGPGSignature" internal/doctor/install_awscli.go` → `1`
- [ ] `grep -c "fetchAndVerifyGPGSignature" internal/doctor/install_ssmplugin.go` → `1`
- [ ] `describeAWSCLIInstall` and `describeSSMPluginInstall` each mention,
      in their `darwin` and `windows` cases, that no verification is
      available for that platform (grep for a phrase like "no integrity
      verification" or similar — exact wording is your call, just confirm
      it's present and truthful)
- [ ] `go.mod` is unchanged (`git diff go.mod` → empty) — confirms no new
      dependency was added
- [ ] No files outside the in-scope list are modified (`git status`)
- [ ] `plans/README.md` status row for plan 030 updated

## STOP conditions

Stop and report back (do not improvise) if:

- Any of the four `.sig` URLs re-checked in Step 1 no longer return `200`
  — the investigation findings have drifted; report exactly which URL and
  what status it now returns, don't substitute a guessed replacement URL.
- The code at the locations in "Current state" doesn't match the excerpts
  shown (the codebase has drifted since this plan was written).
- You find yourself wanting to add a new `go.mod` dependency (e.g. a PGP
  library) to work around `gpg` not being available — that contradicts this
  plan's explicit design decision (see "Design decision" above); stop and
  report the specific blocker instead of overriding the decision.
- A GPG-related test fails (not skips) for reasons other than the
  documented socket-path-length flakiness risk, after one focused
  troubleshooting attempt.
- You discover the current macOS SSM plugin install code actually
  downloads the *signed* `.pkg` rather than the bundle `.zip` (i.e. the
  "Current state" excerpt for `installSSMPluginDarwin` doesn't match) — that
  would change the verification-availability finding for that variant and
  needs re-investigation, not silent adaptation.
- Any verification step's actual output doesn't match what's documented
  here after a reasonable single retry.

## Maintenance notes

- The macOS SSM plugin install currently uses the unsigned bundle `.zip`.
  AWS documents a *separate*, signed `.pkg` installer
  (`session-manager-plugin.pkg`, different URL, different install command —
  `sudo installer -pkg ... -target /` instead of the bundle's `./install`
  script) for macOS. Switching to that path would give macOS SSM plugin
  installs the same GPG-verifiable posture as Linux, but it's a bigger
  change (different artifact format, different install invocation) and was
  deliberately left out of this plan's scope. Worth a follow-up plan if
  macOS coverage becomes a priority.
- If AWS ever starts publishing `.sig` sidecars for the macOS `.pkg` or
  Windows `.msi`/`.exe` AWS CLI or SSM plugin downloads, `describeAWSCLIInstall`/
  `describeSSMPluginInstall`'s darwin/windows text and the corresponding
  `installAWSCLIDarwin`/`installAWSCLIWindows`/`installSSMPluginDarwin`/
  `installSSMPluginWindows` functions should be revisited — this plan
  intentionally does not block on that not-yet-existing publication.
  Re-running the `curl -sI ... .sig` checks in Step 1 periodically (or
  before any future work on these files) is the cheapest way to notice if
  that changes.
- This plan's fail-closed behavior on Linux when `gpg` is absent from
  `PATH` is an intentional regression in convenience for a gain in security
  — a reviewer should specifically scrutinize whether the error message
  shown to the user (via `Describe()`/`Apply()` and `fix.go`'s prompt flow)
  clearly explains how to install `gpg` and retry, since a confusing error
  here is a real support-burden risk.
- The embedded PGP public keys are the single most security-critical part
  of this change: if the wrong key ever ends up embedded (e.g. from a typo
  or a bad copy-paste), verification would silently accept a
  signed-by-someone-else artifact as long as it round-trips through `gpg
  --verify` without an exit error tied to *that* key. A reviewer should
  diff the embedded key text against the exact blocks quoted in AWS's docs
  (URLs given above) as part of code review, not just trust this plan's
  transcription.
