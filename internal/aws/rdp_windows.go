//go:build windows

package aws

import (
	"fmt"
	"os/exec"
)

func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
	cmd := buildRDPSessionCmd(instanceID, profile, region, localPort)
	return runRDPTunnel(cmd, localPort, openClient, launchRDPClientWindows)
}

func launchRDPClientWindows(localPort int) {
	exec.Command("mstsc", fmt.Sprintf("/v:localhost:%d", localPort)).Start()
}
