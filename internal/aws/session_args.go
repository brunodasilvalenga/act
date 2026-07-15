package aws

// sessionArgs builds the aws CLI argument list for `aws ssm start-session`.
func sessionArgs(instanceID, profile, region string) []string {
	args := []string{"ssm", "start-session", "--target", instanceID}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}
