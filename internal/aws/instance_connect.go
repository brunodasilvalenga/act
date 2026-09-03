package aws

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SendSSHPublicKey pushes a local SSH public key to the given OS user on the
// given EC2 instance via EC2 Instance Connect. The pushed key is valid for
// only 60 seconds, so the caller must start the SSH session immediately
// afterward.
func SendSSHPublicKey(instanceID, profile, region, osUser, publicKeyPath string) error {
	absPath, err := filepath.Abs(publicKeyPath)
	if err != nil {
		return fmt.Errorf("resolving public key path: %w", err)
	}

	args := []string{"ec2-instance-connect", "send-ssh-public-key",
		"--instance-id", instanceID,
		"--instance-os-user", osUser,
		"--ssh-public-key", "file://" + absPath,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	_, err = cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return err
	}

	return nil
}

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
