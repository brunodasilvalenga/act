//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartECSExec(cluster, taskID, containerName, profile, region string) error {
	args := ecsExecArgs(cluster, taskID, containerName, profile, region)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return err
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
