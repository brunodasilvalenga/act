package aws

// tailLogsArgs builds the aws CLI argument list for `aws logs tail`.
func tailLogsArgs(logGroup, profile, region, since string, follow bool) []string {
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
	return args
}
