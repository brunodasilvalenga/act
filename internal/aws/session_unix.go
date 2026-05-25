//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartSession(instanceID, profile, region string) error {
	args := []string{"ssm", "start-session", "--target", instanceID}

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

	// Replace current process with aws ssm session
	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
