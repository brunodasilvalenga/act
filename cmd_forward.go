package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
)

func runForward(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("forward", flag.ExitOnError)
	localPort := fs.Int("local-port", 0, "Local port for forwarding")
	remotePort := fs.Int("remote-port", 0, "Remote port for forwarding")
	target := fs.String("target", "", "Target instance ID (skip instance picker)")
	remoteHost := fs.String("remote-host", "", "Remote host for forwarding")
	fs.Parse(subArgs)

	if *localPort == 0 {
		fmt.Fprintf(os.Stderr, "Error: --local-port is required for forwarding\n")
		os.Exit(1)
	}
	if *remotePort == 0 {
		*remotePort = *localPort
	}

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	if *remoteHost != "" {
		fmt.Printf("Forwarding localhost:%d → %s:%d (via %s)\n", *localPort, *remoteHost, *remotePort, instanceID)
		err := aws.StartRemotePortForward(instanceID, profile, region, *localPort, *remotePort, *remoteHost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting remote port forward: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Forwarding localhost:%d → %s:%d\n", *localPort, instanceID, *remotePort)
		err := aws.StartPortForward(instanceID, profile, region, *localPort, *remotePort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
			os.Exit(1)
		}
	}
}
