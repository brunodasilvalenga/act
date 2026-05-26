//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartECSExec(cluster, taskID, containerName, profile, region string) error {
	args := []string{"ecs", "execute-command",
		"--cluster", cluster,
		"--task", taskID,
		"--container", containerName,
		"--interactive",
		"--command", "/bin/sh",
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return err
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
