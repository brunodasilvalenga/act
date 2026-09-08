package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

func runRDS(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("rds", flag.ExitOnError)
	localPort := fs.Int("local-port", 0, "Local port (defaults to RDS port)")
	bastion := fs.String("bastion", "", "Bastion EC2 instance ID")
	noBastion := fs.Bool("no-bastion", false, "Direct connection via VPC endpoint")
	fs.Parse(subArgs)

	// Pick RDS instance
	instances, err := aws.ListRDSInstances(profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing RDS instances: %v\n", err)
		os.Exit(1)
	}
	if len(instances) == 0 {
		fmt.Fprintf(os.Stderr, "No RDS instances found.\n")
		os.Exit(0)
	}

	items := make([]string, len(instances))
	for i, inst := range instances {
		items[i] = inst.DisplayName()
	}

	picked, err := tui.RunPicker("Select RDS Instance", items)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if picked == "" {
		os.Exit(0)
	}

	// Find selected instance
	var rdsInst aws.RDSInstance
	for _, inst := range instances {
		if inst.DisplayName() == picked {
			rdsInst = inst
			break
		}
	}

	rdsPort := rdsInst.Port
	if *localPort == 0 {
		*localPort = rdsPort
	}

	if *noBastion {
		// Direct VPC endpoint connection — no bastion target
		fmt.Printf("Forwarding localhost:%d → %s:%d (direct)\n", *localPort, rdsInst.Endpoint, rdsPort)
		err = aws.StartRemotePortForward("", profile, region, *localPort, rdsPort, rdsInst.Endpoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Pick or use bastion
	bastionID := *bastion
	if bastionID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		bastionID = pickInstance(loadFunc)
	}

	fmt.Printf("Forwarding localhost:%d → %s:%d (via %s)\n", *localPort, rdsInst.Endpoint, rdsPort, bastionID)
	err = aws.StartRemotePortForward(bastionID, profile, region, *localPort, rdsPort, rdsInst.Endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
		os.Exit(1)
	}
}
