package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/doctor"
	"github.com/brunodasilvalenga/act/internal/tui"
	"github.com/brunodasilvalenga/act/internal/updater"
)

var version = "dev"

func main() {
	if _, err := exec.LookPath("aws"); err != nil {
		fmt.Fprintf(os.Stderr, "Error: 'aws' CLI not found in PATH.\nInstall it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html\n")
		os.Exit(1)
	}

	// Parse global flags manually from os.Args
	var profile, region string
	var showVersion bool
	args := os.Args[1:]
	args = parseGlobalFlags(args, &profile, &region, &showVersion)

	if showVersion {
		printVersion()
		os.Exit(0)
	}

	// Determine subcommand
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}

	switch subcmd {
	case "", "help", "--help", "-h":
		printUsage()
		os.Exit(0)

	case "ec2":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printEC2Help()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile)
		resolvedRegion := config.ResolveRegion(region)
		runConnect(resolvedProfile, resolvedRegion)

	case "forward":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printForwardHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile)
		resolvedRegion := config.ResolveRegion(region)
		runForward(resolvedProfile, resolvedRegion, subArgs)

	case "ecs":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printECSHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile)
		resolvedRegion := config.ResolveRegion(region)
		runECS(resolvedProfile, resolvedRegion, subArgs)

	case "doctor":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printDoctorHelp()
			os.Exit(0)
		}
		doctor.Run(profile, region, version)
		os.Exit(0)

	case "upgrade":
		if err := updater.Upgrade(version); err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", subcmd)
		printUsage()
		os.Exit(1)
	}
}

// parseGlobalFlags extracts --profile, --region, --version from args and returns remaining args.
func parseGlobalFlags(args []string, profile, region *string, showVersion *bool) []string {
	var remaining []string
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--version" || args[i] == "-version":
			*showVersion = true
		case args[i] == "--profile" || args[i] == "-profile":
			if i+1 < len(args) {
				i++
				*profile = args[i]
			}
		case args[i] == "--region" || args[i] == "-region":
			if i+1 < len(args) {
				i++
				*region = args[i]
			}
		default:
			remaining = append(remaining, args[i])
		}
	}
	return remaining
}

func hasHelp(args []string) bool {
	if len(args) > 0 && (args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		return true
	}
	return false
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `act - AWS Connect TUI

Usage: act [global flags] <command> [command flags]

Commands:
  ec2          Connect to EC2 instance via SSM session
  forward      Port forwarding via SSM
  ecs          Connect to ECS container via execute-command
  doctor       Check system dependencies and configuration
  upgrade      Upgrade act to the latest version

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --version    Show version information

Run 'act <command> help' for command-specific help.
`)
}

func printEC2Help() {
	fmt.Fprintf(os.Stderr, `act ec2 - Connect to EC2 instance via SSM session

Usage: act [global flags] ec2

Launches an interactive instance picker and starts an SSM session
to the selected instance.

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
`)
}

func printForwardHelp() {
	fmt.Fprintf(os.Stderr, `act forward - Port forwarding via SSM

Usage: act [global flags] forward [flags]

Flags:
  --local-port   Local port for forwarding (required)
  --remote-port  Remote port for forwarding (defaults to local-port)
  --target       Target instance ID (skip instance picker)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use

Examples:
  act forward --local-port 5432
  act forward --local-port 5432 --remote-port 5432 --target i-0123456789abcdef0
`)
}

func printECSHelp() {
	fmt.Fprintf(os.Stderr, `act ecs - Connect to ECS container via execute-command

Usage: act [global flags] ecs [flags]

Flags:
  --cluster    ECS cluster name (skip cluster picker)

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use

Examples:
  act ecs
  act ecs --cluster my-cluster
`)
}

func printDoctorHelp() {
	fmt.Fprintf(os.Stderr, `act doctor - Check system dependencies and configuration

Usage: act [global flags] doctor

Checks that all required tools are installed, credentials are valid,
and configuration is correct.

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
`)
}

func runConnect(profile, region string) {
	loadFunc := func() ([]aws.Instance, error) {
		return aws.ListRunningInstances(profile, region)
	}

	selected, err := tui.Run(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if selected == nil {
		os.Exit(0)
	}

	err = aws.StartSession(selected.InstanceID, profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
		os.Exit(1)
	}
}

func runForward(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("forward", flag.ExitOnError)
	localPort := fs.Int("local-port", 0, "Local port for forwarding")
	remotePort := fs.Int("remote-port", 0, "Remote port for forwarding")
	target := fs.String("target", "", "Target instance ID (skip instance picker)")
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
			return aws.ListRunningInstances(profile, region)
		}

		selected, err := tui.Run(loadFunc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if selected == nil {
			os.Exit(0)
		}
		instanceID = selected.InstanceID
	}

	fmt.Printf("Forwarding localhost:%d → %s:%d\n", *localPort, instanceID, *remotePort)
	err := aws.StartPortForward(instanceID, profile, region, *localPort, *remotePort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
		os.Exit(1)
	}
}

func runECS(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("ecs", flag.ExitOnError)
	cluster := fs.String("cluster", "", "ECS cluster name")
	fs.Parse(subArgs)

	clusterName := *cluster
	if clusterName == "" {
		clusters, err := aws.ListECSClusters(profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing clusters: %v\n", err)
			os.Exit(1)
		}
		if len(clusters) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS clusters found.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select ECS Cluster", clusters)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		clusterName = picked
	}

	loadFunc := func() ([]aws.ECSTask, error) {
		return aws.ListECSTasks(clusterName, profile, region)
	}

	selected, err := tui.RunECS(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}

	err = aws.StartECSExec(selected.ClusterName, selected.TaskID, selected.ContainerName, profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting ECS exec: %v\n", err)
		os.Exit(1)
	}
}

func printVersion() {
	fmt.Printf("act version %s\n", version)

	latest, err := updater.CheckLatestVersion()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not check for updates: %v\n", err)
		return
	}

	if latest != version && version != "dev" {
		fmt.Printf("A new version (v%s) is available! Run `act upgrade` to update.\n", latest)
	} else if version != "dev" {
		fmt.Println("You are up to date.")
	}
}
