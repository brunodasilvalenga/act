//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartSession(instanceID, profile, region string) error {
	args := sessionArgs(instanceID, profile, region)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
