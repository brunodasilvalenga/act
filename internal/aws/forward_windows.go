//go:build windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
)

func StartRemotePortForward(instanceID, profile, region string, localPort, remotePort int, remoteHost string) error {
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

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func StartPortForward(instanceID, profile, region string, localPort, remotePort int) error {
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

	cmd := exec.Command("aws", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
