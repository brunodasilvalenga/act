//go:build !windows

package aws

import (
	"fmt"
	"os/exec"
	"runtime"
)

func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
	cmd := buildRDPSessionCmd(instanceID, profile, region, localPort)
	return runRDPTunnel(cmd, localPort, openClient, launchRDPClientUnix)
}

func launchRDPClientUnix(localPort int) {
	switch runtime.GOOS {
	case "darwin":
		url := fmt.Sprintf("rdp://full%%20address=s:localhost:%d", localPort)
		exec.Command("open", url).Start()
	default:
		fmt.Println("Connect with your RDP client to localhost:" + fmt.Sprint(localPort))
	}
}
