//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func TailLogs(logGroup, profile, region, since string, follow bool) error {
	args := []string{"logs", "tail", logGroup, "--since", since}
	if follow {
		args = append(args, "--follow")
	}
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
