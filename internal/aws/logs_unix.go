//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
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

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("aws CLI not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
