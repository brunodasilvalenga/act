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

// sshProxyArgs builds the ssh CLI argument list using AWS SSM as a
// ProxyCommand.
func sshProxyArgs(instanceID, profile, region, user string) []string {
	return []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", fmt.Sprintf("ProxyCommand=%s", ssmProxyCommand(profile, region)),
		fmt.Sprintf("%s@%s", user, instanceID),
	}
}
