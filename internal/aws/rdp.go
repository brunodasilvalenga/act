package aws

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

// buildRDPSessionCmd builds the "aws ssm start-session" command that opens
// an SSM port-forwarding tunnel from localPort on this machine to port 3389
// (RDP) on instanceID.
func buildRDPSessionCmd(instanceID, profile, region string, localPort int) *exec.Cmd {
	document := "AWS-StartPortForwardingSession"
	params := fmt.Sprintf(`{"portNumber":["3389"],"localPortNumber":["%d"]}`, localPort)

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
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// runRDPTunnel starts cmd (an already-configured "aws ssm start-session"
// port-forwarding command) and waits up to 2 seconds for it to either come
// up or exit early.
//
// If cmd exits within that 2-second window, that is treated as a failure:
// a successful port-forwarding session runs until manually stopped
// (ctrl+c) or until the remote end closes it, so exiting within 2 seconds
// means "aws ssm start-session" itself failed (bad instance ID, SSM agent
// unreachable, session-manager-plugin missing, etc). In that case
// runRDPTunnel returns an error without printing that RDP is available and
// without invoking launchClient.
//
// If the 2-second timer elapses first, the tunnel is presumed up:
// runRDPTunnel prints the "RDP available" message, invokes launchClient
// (unless openClient is false), and then blocks until either an
// interrupt/SIGTERM (in which case it signals cmd to stop and returns nil)
// or cmd exits on its own (in which case cmd's exit error, if any, is
// returned).
func runRDPTunnel(cmd *exec.Cmd, localPort int, openClient bool, launchClient func(localPort int)) error {
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err == nil {
			return fmt.Errorf("aws ssm start-session exited immediately (before the tunnel came up); check the output above for the cause")
		}
		return fmt.Errorf("aws ssm start-session failed: %w", err)
	case <-time.After(2 * time.Second):
		// Tunnel is presumed up; fall through.
	}

	fmt.Printf("\nRDP available at localhost:%d\n", localPort)

	if openClient && launchClient != nil {
		launchClient(localPort)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		cmd.Process.Signal(syscall.SIGTERM)
		return nil
	case err := <-done:
		return err
	}
}
