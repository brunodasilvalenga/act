package main

import (
	"fmt"
	"os"
)

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
