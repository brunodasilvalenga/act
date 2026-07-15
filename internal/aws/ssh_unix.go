//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func StartSSHSession(instanceID, profile, region, user string) error {
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}
	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	if profile != "" {
		proxyCmd += " --profile " + profile
	}
	if region != "" {
		proxyCmd += " --region " + region
	}

	args := []string{
		"ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
		fmt.Sprintf("%s@%s", user, instanceID),
	}

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(sshBin, args, env)
}
