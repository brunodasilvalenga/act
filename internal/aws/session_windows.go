//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartSession(instanceID, profile, region string) error {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
