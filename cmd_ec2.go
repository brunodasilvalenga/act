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
	pushKey := fs.Bool("push-key", false, "Add local SSH public key to the target's authorized_keys via SSM before connecting")
	pushKeyPath := fs.String("push-key-path", "", "Path to public key to push (default: ~/.ssh/id_ed25519.pub or ~/.ssh/id_rsa.pub)")
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

	if *pushKey {
		keyPath := *pushKeyPath
		if keyPath == "" {
			var err error
			keyPath, err = aws.DefaultSSHPublicKeyPath()
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		}
		removeCmd, alreadyPresent, err := aws.PushSSHKeyViaSSM(instanceID, profile, region, sshUser, keyPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error pushing SSH public key: %v\n", err)
			os.Exit(1)
		}
		if alreadyPresent {
			fmt.Fprintf(os.Stderr, "%s is already in %s@%s's authorized_keys; not adding a duplicate.\n", keyPath, sshUser, instanceID)
		} else {
			fmt.Fprintf(os.Stderr, "Pushed %s to %s@%s via SSM (added to authorized_keys).\n", keyPath, sshUser, instanceID)
			fmt.Fprintf(os.Stderr, "To remove it later: %s\n", removeCmd)
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

func runCP(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("ec2 cp", flag.ExitOnError)
	target := fs.String("target", "", "Target instance ID")
	user := fs.String("user", "", "SSH user (default: prompt)")
	download := fs.Bool("download", false, "Copy from the instance to local instead of local to instance")
	recursive := fs.Bool("recursive", false, "Copy directories recursively")
	fs.Parse(subArgs)

	positional := fs.Args()
	if len(positional) != 2 {
		fmt.Fprintf(os.Stderr, "Error: 'ec2 cp' requires exactly 2 positional arguments (source and destination), got %d.\nRun 'act ec2 cp help' for usage.\n", len(positional))
		os.Exit(1)
	}
	source, dest := positional[0], positional[1]

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

	err := aws.CopyFile(instanceID, profile, region, sshUser, source, dest, *download, *recursive)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error copying file: %v\n", err)
		os.Exit(1)
	}
}
