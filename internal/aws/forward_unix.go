//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func StartRemotePortForward(instanceID, profile, region string, localPort, remotePort int, remoteHost string) error {
	args := remotePortForwardArgs(instanceID, profile, region, localPort, remotePort, remoteHost)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("aws CLI not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}

func StartPortForward(instanceID, profile, region string, localPort, remotePort int) error {
	args := portForwardArgs(instanceID, profile, region, localPort, remotePort)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("aws CLI not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
