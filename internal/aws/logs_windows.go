//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func TailLogs(logGroup, profile, region, since string, follow bool) error {
	args := tailLogsArgs(logGroup, profile, region, since, follow)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
