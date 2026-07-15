package aws

// ecsExecArgs builds the aws CLI argument list for `aws ecs execute-command`.
func ecsExecArgs(cluster, taskID, containerName, profile, region string) []string {
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

	return args
}
