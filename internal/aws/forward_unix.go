//go:build !windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

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

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return fmt.Errorf("aws CLI not found in PATH: %w", err)
	}

	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
