//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func StartSSHSession(instanceID, profile, region, user string) error {
	args := append([]string{"ssh"}, sshProxyArgs(instanceID, profile, region, user)...)

	sshBin, err := exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("ssh not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(sshBin, args, env)
}
