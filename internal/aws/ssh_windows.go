//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartSSHSession(instanceID, profile, region, user string) error {
	args := sshProxyArgs(instanceID, profile, region, user)

	cmd := exec.Command("ssh", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
