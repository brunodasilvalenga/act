package aws

import "fmt"

// ssmProxyCommand builds the "aws ssm start-session ..." ProxyCommand
// string shared by both the ssh and scp code paths.
func ssmProxyCommand(profile, region string) string {
	proxyCmd := "aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p"
	if profile != "" {
		proxyCmd += " --profile " + profile
	}
	if region != "" {
		proxyCmd += " --region " + region
	}
	return proxyCmd
}

// sshProxyOptionArgs builds the "-o" flag pairs shared by both the ssh
// and scp code paths: SSH host-key hardening plus the SSM ProxyCommand.
func sshProxyOptionArgs(profile, region string) []string {
	return []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
	}
}

// sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
// ProxyCommand.
func sshProxyArgs(instanceID, profile, region, user string) []string {
	args := sshProxyOptionArgs(profile, region)
	args = append(args, "--")
	return append(args, fmt.Sprintf("%s@%s", user, instanceID))
}
