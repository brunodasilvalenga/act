//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartRemotePortForward(instanceID, profile, region string, localPort, remotePort int, remoteHost string) error {
	args := remotePortForwardArgs(instanceID, profile, region, localPort, remotePort, remoteHost)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func StartPortForward(instanceID, profile, region string, localPort, remotePort int) error {
	args := portForwardArgs(instanceID, profile, region, localPort, remotePort)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
