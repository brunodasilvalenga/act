package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/brunodasilvalenga/act/internal/config"
)

func runEnv(subArgs []string, profile, region string) {
	if len(subArgs) == 0 {
		printEnvHelp()
		os.Exit(1)
	}

	switch subArgs[0] {
	case "list":
		envs := config.ListEnvironments()
		if len(envs) == 0 {
			fmt.Println("No environments configured.")
			return
		}
		names := make([]string, 0, len(envs))
		for name := range envs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			e := envs[name]
			fmt.Printf("%s: profile=%s region=%s\n", name, e.Profile, e.Region)
		}

	case "add":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act --profile <profile> --region <region> env add <name>\n")
			os.Exit(1)
		}
		name := subArgs[1]
		if profile == "" && region == "" {
			fmt.Fprintf(os.Stderr, "Error: at least one of --profile or --region is required (pass them as global flags before the subcommand, e.g. act --profile prod --region us-west-2 env add prod)\n")
			os.Exit(1)
		}
		if err := config.AddEnvironment(name, profile, region); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding environment: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added environment %q (profile=%s, region=%s).\n", name, profile, region)

	case "rm":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act env rm <name>\n")
			os.Exit(1)
		}
		name := subArgs[1]
		if err := config.RemoveEnvironment(name); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing environment: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed environment %q.\n", name)

	default:
		fmt.Fprintf(os.Stderr, "Unknown env subcommand: %s\n", subArgs[0])
		printEnvHelp()
		os.Exit(1)
	}
}
