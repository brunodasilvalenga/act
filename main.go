package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

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
	var profile, region, env string
	var showVersion bool
	args := os.Args[1:]
	args = parseGlobalFlags(args, &profile, &region, &env, &showVersion)

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

func printUsage() {
	fmt.Fprintf(os.Stderr, `act - AWS Connect TUI

Usage: act [global flags] <command> [command flags]

Commands:
  ec2          Connect to EC2 instance via SSM session
  ec2 ssh      SSH to EC2 instance via SSM
  ec2 rdp      RDP to Windows EC2 instance via SSM
  forward      Port forwarding via SSM
  ecs          Connect to ECS container via execute-command
  ecs logs     Tail ECS service logs
  rds          Port forward to RDS instance via SSM
  ssm run      Run a command or script on an instance via SSM
  fav          Connect to a favorite instance
  env          Manage named environments
  init         Create ~/.act.json configuration file
  doctor       Check system dependencies and configuration
  upgrade      Upgrade act to the latest version

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name (from ~/.act.json environments)
  --version    Show version information

Run 'act <command> help' for command-specific help.
`)
}

func printEC2Help() {
	fmt.Fprintf(os.Stderr, `act ec2 - Connect to EC2 instance via SSM session

Usage: act [global flags] ec2 [subcommand|flags]

Launches an interactive instance picker and starts an SSM session
to the selected instance.

Subcommands:
  ssh          SSH to EC2 instance via SSM (see 'act ec2 ssh help')
  rdp          RDP to Windows EC2 instance via SSM (see 'act ec2 rdp help')

Flags:
  --tag        Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name
`)
}

func printForwardHelp() {
	fmt.Fprintf(os.Stderr, `act forward - Port forwarding via SSM

Usage: act [global flags] forward [flags]

Flags:
  --local-port    Local port for forwarding (required)
  --remote-port   Remote port for forwarding (defaults to local-port)
  --target        Target instance ID (skip instance picker)
  --remote-host   Remote host for forwarding (uses remote host document)
  --tag           Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile       AWS profile to use
  --region        AWS region to use
  --env           Environment name

Examples:
  act forward --local-port 5432
  act forward --local-port 5432 --remote-port 5432 --target i-0123456789abcdef0
  act forward --local-port 5432 --remote-port 5432 --remote-host mydb.internal.com --target i-bastion123
`)
}

func printECSHelp() {
	fmt.Fprintf(os.Stderr, `act ecs - Connect to ECS container via execute-command

Usage: act [global flags] ecs [subcommand|flags]

Subcommands:
  logs         Tail ECS service logs (see 'act ecs logs help')

Flags:
  --cluster    ECS cluster name (skip cluster picker)
  --service    Filter tasks by service name

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act ecs
  act ecs --cluster my-cluster
  act ecs --cluster my-cluster --service my-service
`)
}

func printRDSHelp() {
	fmt.Fprintf(os.Stderr, `act rds - Port forward to RDS instance via SSM

Usage: act [global flags] rds [flags]

Forwards a local port to an RDS instance through an EC2 bastion via SSM.

Flags:
  --local-port   Local port (defaults to RDS instance port)
  --bastion      Bastion EC2 instance ID (skip picker)
  --no-bastion   Direct connection via VPC endpoint (no bastion needed)
  --tag          Filter bastion instances by tag (key=value, can be repeated)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act rds
  act rds --bastion i-0123456789abcdef0
  act rds --local-port 5433
  act rds --no-bastion
`)
}

func printSSMHelp() {
	fmt.Fprintf(os.Stderr, `act ssm - Execute commands via SSM Run Command

Usage: act [global flags] ssm [subcommand|flags]

Subcommands:
  run          Run a command or script on an instance (see 'act ssm run help')

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name
`)
}

func printSSMRunHelp() {
	fmt.Fprintf(os.Stderr, `act ssm run - Run a command or script on an EC2 instance via SSM

Usage: act [global flags] ssm run [flags]

Runs one or more shell commands (or a local script file) on a target
instance via AWS Systems Manager Run Command, waits for completion, and
prints stdout/stderr. Automatically uses AWS-RunPowerShellScript for
Windows instances and AWS-RunShellScript for everything else.

Flags:
  --target       Target instance ID (skip instance picker; Linux assumed, use the picker to target Windows instances)
  --command      Command to run (repeatable; each occurrence is one line)
  --script       Path to a local script file to run (mutually exclusive with --command)
  --timeout      Command timeout in seconds (default 300)
  --comment      Optional comment shown in the Systems Manager console
  --no-wait      Submit the command and exit without waiting for it to finish
  --tag          Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act ssm run --command "systemctl status nginx"
  act ssm run --target i-0123456789abcdef0 --command "df -h" --command "uptime"
  act ssm run --script ./deploy.sh --timeout 600
  act ssm run --no-wait --command "sudo reboot"
`)
}

func printECSLogsHelp() {
	fmt.Fprintf(os.Stderr, `act ecs logs - Tail ECS service logs

Usage: act [global flags] ecs logs [flags]

Auto-detects the CloudWatch log group from the ECS task definition
and tails the logs.

Flags:
  --cluster      ECS cluster name (skip cluster picker)
  --service      ECS service name (skip service picker)
  --log-group    Override auto-detected log group
  --since        How far back to start (default "5m")
  --no-follow    Disable follow mode (default: follows)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act ecs logs
  act ecs logs --cluster my-cluster --service my-service
  act ecs logs --log-group /ecs/my-service --since 1h
`)
}

func printEC2SSHHelp() {
	fmt.Fprintf(os.Stderr, `act ec2 ssh - SSH to EC2 instance via SSM

Usage: act [global flags] ec2 ssh [flags]

Starts a real SSH session using SSM as a ProxyCommand. This enables
SCP, rsync, agent forwarding (-A), and port forwarding (-L/-R).

Requires an SSH key configured on the target instance.

Flags:
  --target     Target instance ID (skip instance picker)
  --user       SSH user (default: prompt interactively)
  --tag        Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act ec2 ssh
  act ec2 ssh --user ubuntu
  act ec2 ssh --user ec2-user --target i-0123456789abcdef0
`)
}

func printFavHelp() {
	fmt.Fprintf(os.Stderr, `act fav - Manage and connect to favorite instances

Usage: act [global flags] fav [subcommand]

Subcommands:
  (none)       Show favorites picker and connect
  list         List favorites (non-interactive)
  add <id>     Add instance to favorites
  rm <id>      Remove instance from favorites

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use
  --env        Environment name

Examples:
  act fav
  act fav list
  act fav add i-0123456789abcdef0
  act fav rm i-0123456789abcdef0
`)
}

func printDoctorHelp() {
	fmt.Fprintf(os.Stderr, `act doctor - Check system dependencies and configuration

Usage: act [global flags] doctor [--fix] [--skip-confirm]

Checks that all required tools are installed, credentials are valid,
and configuration is correct.

Flags:
  --fix            Attempt to automatically fix failing checks (currently:
                    installing a missing AWS CLI or Session Manager plugin).
                    Prompts for confirmation before each install unless
                    --skip-confirm is also given. Writes a log of every fix
                    attempt to ~/.act-doctor-fix.log.
  --skip-confirm   With --fix, run every available fix without prompting.
                    Has no effect without --fix.

Global Flags:
  --profile    AWS profile to use
  --region     AWS region to use

Examples:
  act doctor
  act doctor --fix
  act doctor --fix --skip-confirm
`)
}

func printInitHelp() {
	fmt.Fprintf(os.Stderr, `act init - Create ~/.act.json configuration file

Usage: act init

Interactively creates a configuration file at ~/.act.json with
default AWS profile and region settings.

If the file already exists, shows current values and asks to overwrite.
`)
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

// parseTags extracts --tag key=value flags from args, returns remaining args and tags.
func parseTags(args []string) ([]string, []string) {
	var remaining []string
	var tags []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--tag" || args[i] == "-tag" {
			if i+1 < len(args) {
				i++
				tags = append(tags, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, tags
}

// pickInstance runs the interactive picker for the given loadFunc and
// returns the selected instance ID. It exits the process (0) if the user
// quits the picker without selecting, and exits (1) on error — matching
// the behavior every call site had before this helper was extracted.
func pickInstance(loadFunc func() ([]aws.Instance, error)) string {
	selected, err := tui.Run(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}
	return selected.InstanceID
}

// parseCommands extracts --command flags from args (can be repeated,
// one shell line per occurrence), returns remaining args and commands.
func parseCommands(args []string) ([]string, []string) {
	var remaining []string
	var commands []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--command" || args[i] == "-command" {
			if i+1 < len(args) {
				i++
				commands = append(commands, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, commands
}

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

func runForward(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("forward", flag.ExitOnError)
	localPort := fs.Int("local-port", 0, "Local port for forwarding")
	remotePort := fs.Int("remote-port", 0, "Remote port for forwarding")
	target := fs.String("target", "", "Target instance ID (skip instance picker)")
	remoteHost := fs.String("remote-host", "", "Remote host for forwarding")
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
			return aws.ListRunningInstances(profile, region, tags)
		}
		instanceID = pickInstance(loadFunc)
	}

	if *remoteHost != "" {
		fmt.Printf("Forwarding localhost:%d → %s:%d (via %s)\n", *localPort, *remoteHost, *remotePort, instanceID)
		err := aws.StartRemotePortForward(instanceID, profile, region, *localPort, *remotePort, *remoteHost)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting remote port forward: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("Forwarding localhost:%d → %s:%d\n", *localPort, instanceID, *remotePort)
		err := aws.StartPortForward(instanceID, profile, region, *localPort, *remotePort)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
			os.Exit(1)
		}
	}
}

func runECS(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("ecs", flag.ExitOnError)
	cluster := fs.String("cluster", "", "ECS cluster name")
	service := fs.String("service", "", "Filter tasks by service name")
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

	serviceName := *service
	loadFunc := func() ([]aws.ECSTask, error) {
		return aws.ListECSTasks(clusterName, profile, region, serviceName)
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

func runRDS(profile, region string, subArgs []string) {
	subArgs, tags := parseTags(subArgs)

	fs := flag.NewFlagSet("rds", flag.ExitOnError)
	localPort := fs.Int("local-port", 0, "Local port (defaults to RDS port)")
	bastion := fs.String("bastion", "", "Bastion EC2 instance ID")
	noBastion := fs.Bool("no-bastion", false, "Direct connection via VPC endpoint")
	fs.Parse(subArgs)

	// Pick RDS instance
	instances, err := aws.ListRDSInstances(profile, region)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing RDS instances: %v\n", err)
		os.Exit(1)
	}
	if len(instances) == 0 {
		fmt.Fprintf(os.Stderr, "No RDS instances found.\n")
		os.Exit(0)
	}

	items := make([]string, len(instances))
	for i, inst := range instances {
		items[i] = inst.DisplayName()
	}

	picked, err := tui.RunPicker("Select RDS Instance", items)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if picked == "" {
		os.Exit(0)
	}

	// Find selected instance
	var rdsInst aws.RDSInstance
	for _, inst := range instances {
		if inst.DisplayName() == picked {
			rdsInst = inst
			break
		}
	}

	rdsPort := rdsInst.Port
	if *localPort == 0 {
		*localPort = rdsPort
	}

	if *noBastion {
		// Direct VPC endpoint connection — no bastion target
		fmt.Printf("Forwarding localhost:%d → %s:%d (direct)\n", *localPort, rdsInst.Endpoint, rdsPort)
		err = aws.StartRemotePortForward("", profile, region, *localPort, rdsPort, rdsInst.Endpoint)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Pick or use bastion
	bastionID := *bastion
	if bastionID == "" {
		loadFunc := func() ([]aws.Instance, error) {
			return aws.ListRunningInstances(profile, region, tags)
		}
		bastionID = pickInstance(loadFunc)
	}

	fmt.Printf("Forwarding localhost:%d → %s:%d (via %s)\n", *localPort, rdsInst.Endpoint, rdsPort, bastionID)
	err = aws.StartRemotePortForward(bastionID, profile, region, *localPort, rdsPort, rdsInst.Endpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting port forward: %v\n", err)
		os.Exit(1)
	}
}

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

	result, err := aws.WaitForCommandInvocation(commandID, instanceID, profile, region, 2*time.Second)
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

func runLogs(profile, region string, subArgs []string) {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	cluster := fs.String("cluster", "", "ECS cluster name")
	service := fs.String("service", "", "ECS service name")
	logGroup := fs.String("log-group", "", "Override log group")
	since := fs.String("since", "5m", "How far back to start")
	noFollow := fs.Bool("no-follow", false, "Disable follow mode")
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

	serviceName := *service
	if serviceName == "" {
		services, err := aws.ListECSServices(clusterName, profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error listing services: %v\n", err)
			os.Exit(1)
		}
		if len(services) == 0 {
			fmt.Fprintf(os.Stderr, "No ECS services found.\n")
			os.Exit(0)
		}
		picked, err := tui.RunPicker("Select ECS Service", services)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}
		serviceName = picked
	}

	logGroupName := *logGroup
	if logGroupName == "" {
		groups, err := aws.GetLogGroupsFromService(clusterName, serviceName, profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error detecting log group: %v\n", err)
			os.Exit(1)
		}
		if len(groups) == 0 {
			fmt.Fprintf(os.Stderr, "No log groups found in task definition.\n")
			os.Exit(1)
		}
		if len(groups) == 1 {
			logGroupName = groups[0]
		} else {
			picked, err := tui.RunPicker("Select Log Group", groups)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			if picked == "" {
				os.Exit(0)
			}
			logGroupName = picked
		}
	}

	follow := !*noFollow
	fmt.Printf("Tailing %s (since %s)\n", logGroupName, *since)
	err := aws.TailLogs(logGroupName, profile, region, *since, follow)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error tailing logs: %v\n", err)
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

func printEC2RDPHelp() {
	fmt.Fprintf(os.Stderr, `act ec2 rdp - RDP to Windows EC2 instance via SSM

Usage: act [global flags] ec2 rdp [flags]

Starts a port forwarding session to port 3389 on a Windows EC2 instance
and optionally opens your RDP client.

Only Windows instances are shown in the picker.

Flags:
  --target       Target instance ID (skip instance picker)
  --local-port   Local port (default: 3389)
  --key          Path to private key for password decryption
  --show-password  Print the decrypted password in plaintext (default: masked)
  --no-open      Don't auto-open RDP client
  --tag          Filter instances by tag (key=value, can be repeated)

Global Flags:
  --profile      AWS profile to use
  --region       AWS region to use
  --env          Environment name

Examples:
  act ec2 rdp
  act ec2 rdp --key ~/.ssh/my-key.pem
  act ec2 rdp --no-open --local-port 13389
  act ec2 rdp --target i-0123456789abcdef0
`)
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

func printEnvHelp() {
	fmt.Fprintf(os.Stderr, `act env - Manage named environments (profile + region presets)

Usage: act env [subcommand]

Subcommands:
  list                                       List configured environments
  add <name>                                 Add or update an environment (use global --profile/--region flags)
  rm <name>                                  Remove an environment

Examples:
  act env list
  act --profile production --region us-west-2 env add prod
  act env rm prod
`)
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
