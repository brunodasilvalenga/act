//go:build windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
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
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", proxyCmd),
		fmt.Sprintf("%s@%s", user, instanceID),
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
