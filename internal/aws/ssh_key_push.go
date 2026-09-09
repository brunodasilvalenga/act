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

// rejectIfWindows returns an error if platform indicates a Windows
// instance, using the same case-insensitive convention DocumentForPlatform
// uses in ssm.go. --push-key's shell script is POSIX-specific (getent,
// chown, .ssh/authorized_keys) and has no Windows equivalent, so
// PushSSHKeyViaSSM must refuse before sending any SSM command rather than
// letting AWS-RunShellScript fail confusingly against a Windows instance.
func rejectIfWindows(platform string) error {
	if strings.EqualFold(platform, "windows") {
		return fmt.Errorf("--push-key is not supported for Windows targets (target platform: %s); Windows instances don't use authorized_keys the same way — see 'act ec2 rdp' for Windows access", platform)
	}
	return nil
}

// PushSSHKeyViaSSM appends the public key at publicKeyPath to osUser's
// authorized_keys on the target instance, via an SSM Run Command rather than
// EC2 Instance Connect — this only depends on the SSM Agent, which every
// other command in this tool already requires, instead of a separate
// on-instance agent that isn't installed on every AMI. The appended line is
// tagged with a unique marker so it can be found and removed later; the
// returned string is the exact "act ssm run" invocation that removes it.
// platform is the target's EC2 Platform value (as in Instance.Platform /
// DocumentForPlatform); Windows targets are rejected immediately since this
// function's script is POSIX-only.
func PushSSHKeyViaSSM(instanceID, profile, region, osUser, publicKeyPath, platform string) (string, error) {
	if err := rejectIfWindows(platform); err != nil {
		return "", err
	}
	keyBytes, err := os.ReadFile(publicKeyPath)
	if err != nil {
		return "", fmt.Errorf("reading public key: %w", err)
	}
	key := strings.TrimSpace(string(keyBytes))
	if key == "" {
		return "", fmt.Errorf("public key file %s is empty", publicKeyPath)
	}
	if strings.ContainsAny(key, "\n\r") {
		return "", fmt.Errorf("public key file %s contains more than one line; expected a single public key", publicKeyPath)
	}

	marker := fmt.Sprintf("act-push-key-%d", time.Now().UnixNano())
	keyLine := fmt.Sprintf("%s act-push-key %s", key, marker)

	osUserQ := shellSingleQuote(osUser)
	ownerQ := shellSingleQuote(osUser + ":" + osUser)
	keyLineQ := shellSingleQuote(keyLine)

	script := fmt.Sprintf(`set -e
homedir=$(getent passwd %s | cut -d: -f6)
if [ -z "$homedir" ]; then echo "act-push-key: no such user %s" >&2; exit 1; fi
mkdir -p "$homedir/.ssh"
touch "$homedir/.ssh/authorized_keys"
chmod 700 "$homedir/.ssh"
chmod 600 "$homedir/.ssh/authorized_keys"
chown %s "$homedir/.ssh" "$homedir/.ssh/authorized_keys"
grep -qxF %s "$homedir/.ssh/authorized_keys" || echo %s >> "$homedir/.ssh/authorized_keys"
echo "$homedir/.ssh/authorized_keys"`, osUserQ, osUserQ, ownerQ, keyLineQ, keyLineQ)

	commandID, err := SendCommand(instanceID, profile, region, "AWS-RunShellScript", strings.Split(script, "\n"), 60, "act ec2 ssh --push-key")
	if err != nil {
		return "", fmt.Errorf("sending push-key command: %w", err)
	}

	result, err := WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
	if err != nil {
		return "", fmt.Errorf("waiting for push-key command: %w", err)
	}
	if result.Status != "Success" {
		return "", fmt.Errorf("push-key command finished with status %s: %s", result.Status, strings.TrimSpace(result.Stderr))
	}

	authKeysPath := strings.TrimSpace(result.Stdout)
	removeCmd := fmt.Sprintf(`act ssm run --target %s --command "sed -i '/%s/d' %s"`, instanceID, marker, authKeysPath)
	return removeCmd, nil
}
