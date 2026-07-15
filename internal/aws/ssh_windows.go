//go:build windows

package aws

import (
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

	args := sshProxyArgs(instanceID, profile, region, user)

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
