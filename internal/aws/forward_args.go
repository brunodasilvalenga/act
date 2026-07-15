package aws

import "fmt"

// remotePortForwardArgs builds the aws CLI argument list for
// AWS-StartPortForwardingSessionToRemoteHost.
func remotePortForwardArgs(instanceID, profile, region string, localPort, remotePort int, remoteHost string) []string {
	document := "AWS-StartPortForwardingSessionToRemoteHost"
	params := fmt.Sprintf(`{"host":["%s"],"portNumber":["%d"],"localPortNumber":["%d"]}`, remoteHost, remotePort, localPort)

	args := []string{"ssm", "start-session",
		"--document-name", document,
		"--parameters", params,
	}

	if instanceID != "" {
		args = append(args, "--target", instanceID)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}

// portForwardArgs builds the aws CLI argument list for
// AWS-StartPortForwardingSession.
func portForwardArgs(instanceID, profile, region string, localPort, remotePort int) []string {
	document := "AWS-StartPortForwardingSession"
	params := fmt.Sprintf(`{"portNumber":["%d"],"localPortNumber":["%d"]}`, remotePort, localPort)

	args := []string{"ssm", "start-session",
		"--target", instanceID,
		"--document-name", document,
		"--parameters", params,
	}

	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	return args
}
