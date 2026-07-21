package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/doctor"
	"github.com/brunodasilvalenga/act/internal/tui"
	"github.com/brunodasilvalenga/act/internal/updater"
)

var version = "dev"

func main() {
	// Parse global flags manually from os.Args
	var profile, region, env string
	var showVersion bool
	args := os.Args[1:]
	args = parseGlobalFlags(args, &profile, &region, &env, &showVersion)

	// Determine subcommand
	subcmd := ""
	if len(args) > 0 {
		subcmd = args[0]
	}

	if subcommandNeedsAWSCLI(subcmd) {
		if _, err := exec.LookPath("aws"); err != nil {
			fmt.Fprintf(os.Stderr, "Error: 'aws' CLI not found in PATH.\nInstall it from https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html\n")
			os.Exit(1)
		}
	}

	if showVersion {
		printVersion()
		os.Exit(0)
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
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		if len(subArgs) > 0 && subArgs[0] == "ssh" {
			if hasHelp(subArgs[1:]) {
				printEC2SSHHelp()
				os.Exit(0)
			}
			runSSH(resolvedProfile, resolvedRegion, subArgs[1:])
		} else if len(subArgs) > 0 && subArgs[0] == "rdp" {
			if hasHelp(subArgs[1:]) {
				printEC2RDPHelp()
				os.Exit(0)
			}
			runRDP(resolvedProfile, resolvedRegion, subArgs[1:])
		} else {
			runConnect(resolvedProfile, resolvedRegion, subArgs)
		}

	case "forward":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printForwardHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		runForward(resolvedProfile, resolvedRegion, subArgs)

	case "ecs":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printECSHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		if len(subArgs) > 0 && subArgs[0] == "logs" {
			if hasHelp(subArgs[1:]) {
				printECSLogsHelp()
				os.Exit(0)
			}
			runLogs(resolvedProfile, resolvedRegion, subArgs[1:])
		} else {
			runECS(resolvedProfile, resolvedRegion, subArgs)
		}

	case "ssm":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printSSMHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		if len(subArgs) > 0 && subArgs[0] == "run" {
			if hasHelp(subArgs[1:]) {
				printSSMRunHelp()
				os.Exit(0)
			}
			runSSMRun(resolvedProfile, resolvedRegion, subArgs[1:])
		} else {
			fmt.Fprintf(os.Stderr, "Unknown ssm subcommand. Run 'act ssm help' for usage.\n")
			os.Exit(1)
		}

	case "rds":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printRDSHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		runRDS(resolvedProfile, resolvedRegion, subArgs)

	case "fav":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printFavHelp()
			os.Exit(0)
		}
		resolvedProfile := config.ResolveProfile(profile, env)
		resolvedRegion := config.ResolveRegion(region, env)
		runFav(resolvedProfile, resolvedRegion, subArgs)

	case "env":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printEnvHelp()
			os.Exit(0)
		}
		runEnv(subArgs, profile, region)
		os.Exit(0)

	case "doctor":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printDoctorHelp()
			os.Exit(0)
		}
		fix := false
		skipConfirm := false
		var doctorArgs []string
		for _, a := range subArgs {
			switch a {
			case "--fix":
				fix = true
			case "--skip-confirm":
				skipConfirm = true
			default:
				doctorArgs = append(doctorArgs, a)
			}
		}
		if skipConfirm && !fix {
			fmt.Fprintln(os.Stderr, "Error: --skip-confirm has no effect without --fix")
			os.Exit(1)
		}
		doctor.Run(profile, region, version, fix, skipConfirm)
		os.Exit(0)

	case "init":
		subArgs := args[1:]
		if hasHelp(subArgs) {
			printInitHelp()
			os.Exit(0)
		}
		runInit()
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

// parseGlobalFlags extracts --profile, --region, --env, --version from args and returns remaining args.
func parseGlobalFlags(args []string, profile, region, env *string, showVersion *bool) []string {
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
		case args[i] == "--env" || args[i] == "-env":
			if i+1 < len(args) {
				i++
				*env = args[i]
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

// subcommandNeedsAWSCLI reports whether subcmd requires the AWS CLI to be
// installed before it can do anything useful. "doctor" is the one
// exception: it diagnoses (and, with --fix, can install) a missing AWS
// CLI itself, so it must be reachable even when aws isn't on PATH yet.
func subcommandNeedsAWSCLI(subcmd string) bool {
	return subcmd != "doctor"
}

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

func runFav(profile, region string, subArgs []string) {
	if len(subArgs) == 0 {
		// Show picker from favorites
		cfg := config.Load()
		if len(cfg.Favorites) == 0 {
			fmt.Fprintf(os.Stderr, "No favorites configured. Use 'act fav add <instance-id>' to add one.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select Favorite Instance", cfg.Favorites)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}

		err = aws.StartSession(picked, profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch subArgs[0] {
	case "list":
		favorites := config.ListFavorites()
		if len(favorites) == 0 {
			fmt.Println("No favorites configured.")
			return
		}
		for _, f := range favorites {
			fmt.Println(f)
		}

	case "add":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav add <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if !strings.HasPrefix(id, "i-") {
			fmt.Fprintf(os.Stderr, "Error: instance ID must start with 'i-'\n")
			os.Exit(1)
		}
		if err := config.AddFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added %s to favorites.\n", id)

	case "rm":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav rm <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if err := config.RemoveFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed %s from favorites.\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown fav subcommand: %s\n", subArgs[0])
		printFavHelp()
		os.Exit(1)
	}
}

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
