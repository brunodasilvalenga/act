//go:build windows

package aws

import (
	"os"
	"os/exec"
)

func StartECSExec(cluster, taskID, containerName, profile, region string) error {
	args := ecsExecArgs(cluster, taskID, containerName, profile, region)

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
