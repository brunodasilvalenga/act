//go:build windows

package aws

import (
	"os"
	"os/exec"
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

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
