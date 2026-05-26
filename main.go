package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
	"github.com/brunodasilvalenga/act/internal/updater"
)

var version = "dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: act [flags] [command]\n\nCommands:\n  upgrade    Upgrade act to the latest version\n\nFlags:\n")
		flag.PrintDefaults()
	}

	showVersion := flag.Bool("version", false, "Show version information")
	profile := flag.String("profile", "", "AWS profile to use")
	region := flag.String("region", "", "AWS region to use")
	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	if len(flag.Args()) > 0 && flag.Args()[0] == "upgrade" {
		if err := updater.Upgrade(version); err != nil {
			fmt.Fprintf(os.Stderr, "Error upgrading: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	instances, err := aws.ListRunningInstances(*profile, *region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing instances: %v\n", err)
		os.Exit(1)
	}

	if len(instances) == 0 {
		fmt.Println("No running instances found.")
		os.Exit(0)
	}

	selected, err := tui.Run(instances)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if selected == nil {
		os.Exit(0)
	}

	err = aws.StartSession(selected.InstanceID, *profile, *region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
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
