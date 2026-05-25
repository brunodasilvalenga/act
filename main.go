package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

func main() {
	profile := flag.String("profile", "", "AWS profile to use")
	region := flag.String("region", "", "AWS region to use")
	flag.Parse()

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
