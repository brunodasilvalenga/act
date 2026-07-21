package main

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/doctor"
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
