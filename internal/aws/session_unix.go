//go:build !windows

package aws

import (
	"os"
	"os/exec"
	"syscall"
)

func StartSession(instanceID, profile, region string) error {
	args := sessionArgs(instanceID, profile, region)

	awsBin, err := exec.LookPath("aws")
	if err != nil {
		return err
	}

	// Replace current process with aws ssm session
	env := os.Environ()
	return syscall.Exec(awsBin, append([]string{"aws"}, args...), env)
}
