//go:build windows

package aws

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
)

func StartRDP(instanceID, profile, region string, localPort int, openClient bool) error {
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

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start port forward: %w", err)
	}

	// Wait for tunnel to be ready
	time.Sleep(2 * time.Second)

	fmt.Printf("\nRDP available at localhost:%d\n", localPort)

	if openClient {
		exec.Command("mstsc", fmt.Sprintf("/v:localhost:%d", localPort)).Start()
	}

	// Wait for ctrl+c or process exit
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-sigCh:
		cmd.Process.Signal(syscall.SIGTERM)
		return nil
	case err := <-done:
		return err
	}
}
