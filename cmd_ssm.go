package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

func runSSMRun(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)
	subArgs, commands := parseCommands(subArgs)

	fs := flag.NewFlagSet("ssm run", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	script := fs.String("script", "", "Path to a local script file to run")
	timeout := fs.Int("timeout", 300, "Command timeout in seconds")
	comment := fs.String("comment", "", "Optional comment shown in the Systems Manager console")
	noWait := fs.Bool("no-wait", false, "Submit the command and exit without waiting")
	fs.Parse(subArgs)

	if len(commands) == 0 && *script == "" {
		fmt.Fprintf(os.Stderr, "Error: provide at least one --command or --script\n")
		os.Exit(1)
	}
	if len(commands) > 0 && *script != "" {
		fmt.Fprintf(os.Stderr, "Error: --command and --script are mutually exclusive\n")
		os.Exit(1)
	}

	if *script != "" {
		data, err := os.ReadFile(*script)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading script file: %v\n", err)
			os.Exit(1)
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
		commands = lines
	}

	instanceID := *target
	var platform string
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
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
		platform = selected.Platform
	}

	document := aws.DocumentForPlatform(platform)

	commandID, err := aws.SendCommand(instanceID, profile, region, document, commands, *timeout, *comment)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending command: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Command %s submitted to %s\n", commandID, instanceID)

	if *noWait {
		return
	}

	maxWait := aws.MaxWaitFromTimeoutSeconds(*timeout)
	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second, maxWait)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error waiting for command: %v\n", err)
		os.Exit(1)
	}

	if result.Stdout != "" {
		fmt.Print(result.Stdout)
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}

	if result.Status != "Success" {
		fmt.Fprintf(os.Stderr, "Command finished with status %s\n", result.Status)
		os.Exit(1)
	}
}
