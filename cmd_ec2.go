package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

func runConnect(profile, region string, subArgs []string) {
	_, tags := parseTags(subArgs)

	loadFunc := func() ([]aws.Instance, error) {
		return aws.ListRunningInstances(profile, region, tags)
	}

	instanceID := pickInstance(loadFunc)

	err := aws.StartSession(instanceID, profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
		os.Exit(1)
	}
}

func runSSH(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ssh", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	fs.Parse(subArgs)

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	sshUser := *user
	if sshUser == "" {
		picked, err := tui.RunPicker("Select SSH user", []string{"ec2-user", "ubuntu", "root", "ssm-user"})
		if err != nil || picked == "" {
			sshUser = "ec2-user"
		} else {
			sshUser = picked
		}
	}

	err := aws.StartSSHSession(instanceID, profile, region, sshUser)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting SSH session: %v\n", err)
		os.Exit(1)
	}
}

func runRDP(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("rdp", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	localPort := fs.Int("local-port", 3389, "Local port")
	key := fs.String("key", "", "Path to private key for password decryption")
	noOpen := fs.Bool("no-open", false, "Don't auto-open RDP client")
	showPassword := fs.Bool("show-password", false, "Print the decrypted password to stdout (default: masked)")
	fs.Parse(subArgs)

	instanceID := *target
	if instanceID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListWindowsInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	if *key != "" {
		password, err := aws.GetPasswordData(instanceID, profile, region, *key)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not retrieve password: %v\n", err)
		} else if password != "" {
			if *showPassword {
				fmt.Printf("Administrator password: %s\n", password)
			} else {
				fmt.Println("Administrator password retrieved (use --show-password to display it in plaintext).")
			}
		} else {
			fmt.Println("No password data available (instance may use domain auth or password not yet generated).")
		}
	}

	fmt.Printf("Starting RDP port forward to %s (localhost:%d → 3389)\n", instanceID, *localPort)
	err := aws.StartRDP(instanceID, profile, region, *localPort, !*noOpen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
