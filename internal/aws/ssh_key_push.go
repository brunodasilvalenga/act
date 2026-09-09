package aws

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultSSHPublicKeyPath returns the first of the user's standard SSH
// public key files that exists, preferring id_ed25519.pub over id_rsa.pub.
func DefaultSSHPublicKeyPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}
	return findSSHPublicKey(home)
}

func findSSHPublicKey(homeDir string) (string, error) {
	candidates := []string{
		filepath.Join(homeDir, ".ssh", "id_ed25519.pub"),
		filepath.Join(homeDir, ".ssh", "id_rsa.pub"),
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("no default SSH public key found (checked %s); use --push-key-path to specify one", strings.Join(candidates, ", "))
}

// shellSingleQuote escapes s for safe inclusion inside single quotes in a
// POSIX shell command (the standard '\” escape for an embedded quote).
func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// buildPushKeyScript returns the shell script that appends key to osUser's
// authorized_keys, tagging the appended line with marker so it can be
// removed later. The dedup check greps for the raw key text (stable across
// runs) rather than the marker-tagged line, so a line left by a previous
// run — which carries a different marker but the same key — is correctly
// recognized as "already present" and no duplicate is appended. It prints
// "act-push-key: already present" as its first line of output when it
// skips the append, so the caller can tell the two outcomes apart; the
// authorized_keys path is always the last line of output.
func buildPushKeyScript(osUser, key, marker string) string {
	keyLine := fmt.Sprintf("%s act-push-key %s", key, marker)

	osUserQ := shellSingleQuote(osUser)
	ownerQ := shellSingleQuote(osUser + ":" + osUser)
	keyQ := shellSingleQuote(key)
	keyLineQ := shellSingleQuote(keyLine)

	return fmt.Sprintf(`set -e
homedir=$(getent passwd %s | cut -d: -f6)
if [ -z "$homedir" ]; then echo "act-push-key: no such user %s" >&2; exit 1; fi
mkdir -p "$homedir/.ssh"
touch "$homedir/.ssh/authorized_keys"
chmod 700 "$homedir/.ssh"
chmod 600 "$homedir/.ssh/authorized_keys"
chown %s "$homedir/.ssh" "$homedir/.ssh/authorized_keys"
if grep -qF %s "$homedir/.ssh/authorized_keys"; then
  echo "act-push-key: already present"
else
  echo %s >> "$homedir/.ssh/authorized_keys"
fi
echo "$homedir/.ssh/authorized_keys"`, osUserQ, osUserQ, ownerQ, keyQ, keyLineQ)
}

// PushSSHKeyViaSSM appends the public key at publicKeyPath to osUser's
// authorized_keys on the target instance, via an SSM Run Command rather than
// EC2 Instance Connect — this only depends on the SSM Agent, which every
// other command in this tool already requires, instead of a separate
// on-instance agent that isn't installed on every AMI. The appended line is
// tagged with a unique marker so it can be found and removed later; the
// returned removeCmd is the exact "act ssm run" invocation that removes it.
// If the key is already present (e.g. from a previous run), alreadyPresent
// is true, removeCmd is "", and no duplicate line is appended.
func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath string) (removeCmd string, alreadyPresent bool, err error) {
	keyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", false, fmt.Errorf("reading public key: %w", err)
	}
	key := strings.TrimSpace(string(keyBytes))
	if key == "" {
		return "", false, fmt.Errorf("public key file %s is empty", publicKeyPath)
	}
	if strings.ContainsAny(key, "\n\r") {
		return "", false, fmt.Errorf("public key file %s contains more than one line; expected a single public key", publicKeyPath)
	}

	marker := fmt.Sprintf("act-push-key-%d", time.Now().UnixNano())
	script := buildPushKeyScript(osUser, key, marker)

	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
	if err != nil {
		return "", false, fmt.Errorf("sending push-key command: %w", err)
	}

	result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
	if err != nil {
		return "", false, fmt.Errorf("waiting for push-key command: %w", err)
	}
	if result.Status != "Success" {
		return "", false, fmt.Errorf("push-key command finished with status %s: %s", result.Status, strings.TrimSpace(result.Stderr))
	}

	outLines := strings.Split(strings.TrimSpace(result.Stdout), "\n")
	authKeysPath := outLines[len(outLines)-1]
	alreadyPresent = len(outLines) > 1 && strings.Contains(outLines[0], "act-push-key: already present")
	if alreadyPresent {
		return "", true, nil
	}
	removeCmd = fmt.Sprintf(`act ssm run --target %s --command "sed -i '/%s/d' %s"`, instanceID, marker, authKeysPath)
	return removeCmd, false, nil
}
