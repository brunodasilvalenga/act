package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/brunodasilvalenga/act/internal/config"
)

func runInit() {
	reader := bufio.NewReader(os.Stdin)

	if config.Exists() {
		cfg := config.Load()
		fmt.Printf("~/.act.json already exists:\n")
		fmt.Printf("  profile: %s\n", cfg.DefaultProfile)
		fmt.Printf("  region: %s\n", cfg.DefaultRegion)
		fmt.Println()
		fmt.Print("Overwrite? [y/N]: ")
		answer, _ := reader.ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Aborted.")
			return
		}
		fmt.Println()
	}

	fmt.Println("Creating ~/.act.json")
	fmt.Println()

	defaultProfile := os.Getenv("AWS_PROFILE")
	defaultRegion := os.Getenv("AWS_REGION")
	if defaultRegion == "" {
		defaultRegion = os.Getenv("AWS_DEFAULT_REGION")
	}

	fmt.Printf("Default AWS profile [%s]: ", defaultProfile)
	profileInput, _ := reader.ReadString('\n')
	profileInput = strings.TrimSpace(profileInput)
	if profileInput == "" {
		profileInput = defaultProfile
	}

	fmt.Printf("Default AWS region [%s]: ", defaultRegion)
	regionInput, _ := reader.ReadString('\n')
	regionInput = strings.TrimSpace(regionInput)
	if regionInput == "" {
		regionInput = defaultRegion
	}

	if err := config.Init(profileInput, regionInput); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✓ Config written to %s\n", config.ConfigPath())
}
