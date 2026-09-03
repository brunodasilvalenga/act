package aws

import (
	"fmt"
	"os"
	"os/exec"
)

// scpEndpoints computes the local and remote-spec scp arguments for a
// given copy direction.
func scpEndpoints(instanceID, user, source, dest string, download bool) (localArg, remoteSpecArg string, remoteFirst bool) {
	remotePath, localPath := dest, source
	if download {
		remotePath, localPath = source, dest
	}
	remoteSpec := fmt.Sprintf("%s@%s:%s", user, instanceID, remotePath)
	return localPath, remoteSpec, download
}

// CopyFile runs scp over an SSM-proxied SSH tunnel to copy a file (or,
// with recursive, a directory) between the local machine and an EC2
// instance. By default source is a local path and dest is the remote
// path (upload); with download=true the direction is reversed (dest is
// the local path, source is the remote path).
func CopyFile(instanceID, profile, region, user, source, dest string, download, recursive bool) error {
	if err := validateSSHProxyToken(profile, "profile"); err != nil {
		return err
	}
	if err := validateSSHProxyToken(region, "region"); err != nil {
		return err
	}

	args := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
	}
	if recursive {
		args = append(args, "-r")
	}

	localPath, remoteSpec, remoteFirst := scpEndpoints(instanceID, user, source, dest, download)
	if remoteFirst {
		args = append(args, remoteSpec, localPath)
	} else {
		args = append(args, localPath, remoteSpec)
	}

	scpBin, err := exec.LookPath("scp")
	if err != nil {
		return fmt.Errorf("scp not found in PATH (install an OpenSSH client): %w", err)
	}

	cmd := exec.Command(scpBin, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
