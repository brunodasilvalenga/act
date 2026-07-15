//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func TailLogs(logGroup, profile, region, since string, follow bool) error {
	args := tailLogsArgs(logGroup, profile, region, since, follow)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("aws CLI not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
