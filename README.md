# act - AWS Connect TUI

A terminal UI for connecting to AWS resources via Session Manager — EC2 instances, ECS containers, and port forwarding.

No extra dependencies — just the AWS CLI and the Session Manager plugin.

![demo](https://github.com/brunodasilvalenga/act/raw/main/demo.gif)

## Features

- **EC2 Connect** — List running instances, filter, and start SSM sessions
- **ECS Exec** — Pick a cluster, service, and task to exec into a container
- **Port Forwarding** — Forward local ports to remote instances via SSM
- **Remote Host Forwarding** — Forward ports to remote hosts (e.g., RDS) through a bastion
- **RDS Forwarding** — Interactive RDS picker with bastion-based port forwarding
- **ECS Logs** — Auto-detect and tail CloudWatch logs from ECS services
- **RDP over SSM** — RDP to Windows instances via SSM port forwarding with auto-client launch
- **SSH over SSM** — SSH to instances without open inbound ports
- **Run Command** — Execute a command or script on an instance via SSM Run Command (no session needed)
- **Favorites** — Save and quickly connect to frequently used instances
- **Tag Filtering** — Filter EC2 instances by tags
- **Multi-Environment Config** — Switch between environments (prod, staging, etc.)
- **Config File** — Set defaults in `~/.act.json` (profile, region, favorites, environments)
- **Environment Variables** — Falls back to `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`
- **Self-Upgrade** — Update to latest version with `act upgrade`
- **Doctor** — Verify dependencies and configuration with `act doctor`
- **Cross-Platform** — Works on macOS, Linux, and Windows

## Prerequisites

- [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) installed and configured
- [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed
- IAM permissions for `ec2:DescribeInstances`, `ec2:GetPasswordData`, `ssm:StartSession`, `ssm:SendCommand`, `ssm:GetCommandInvocation`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`, `rds:DescribeDBInstances`, `logs:GetLogEvents`, `logs:FilterLogEvents`
- EC2 instances must have the SSM Agent running and proper IAM role attached
- For `ec2 ssh`: OpenSSH client (`ssh`) and an SSH key configured on the target instance
- For `ec2 rdp`: An RDP client (macOS: Microsoft Remote Desktop or built-in; Windows: mstsc)

## Installation

### Homebrew (macOS)

```bash
brew install brunodasilvalenga/tap/act
```

### From source

```bash
go install github.com/brunodasilvalenga/act@latest
```

### Quick install (curl)

```bash
curl -sSfL https://raw.githubusercontent.com/brunodasilvalenga/act/main/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/brunodasilvalenga/act/main/install.ps1 | iex
```

### From releases

Download the binary for your platform from the [Releases](https://github.com/brunodasilvalenga/act/releases) page.

## Usage

```
act [global flags] <command> [command flags]
```

### Commands

| Command | Description |
|---------|-------------|
| `ec2` | Connect to EC2 instance via SSM session |
| `ec2 ssh` | SSH to EC2 instance via SSM |
| `ec2 rdp` | RDP to Windows EC2 instance via SSM |
| `forward` | Port forwarding via SSM |
| `ecs` | Connect to ECS container via execute-command |
| `ecs logs` | Tail ECS service logs |
| `rds` | Port forward to RDS instance via SSM |
| `ssm run` | Run a command or script on an instance via SSM |
| `fav` | Connect to a favorite instance |
| `init` | Create `~/.act.json` configuration file interactively |
| `doctor` | Check system dependencies and configuration |
| `upgrade` | Upgrade act to the latest version |

### Global Flags

| Flag | Description |
|------|-------------|
| `--profile` | AWS profile to use |
| `--region` | AWS region to use |
| `--env` | Environment name (from ~/.act.json environments) |
| `--version` | Show version information |

### Examples

```bash
# Connect to an EC2 instance (interactive picker)
act ec2

# Connect with a specific profile and region
act --profile production --region us-west-2 ec2

# Filter instances by tag
act ec2 --tag Environment=production
act ec2 --tag Name=bastion --tag Team=platform

# Use a named environment
act --env prod ec2

# Port forward local port 5432 to a selected instance
act forward --local-port 5432

# Port forward with explicit target
act forward --local-port 5432 --remote-port 5432 --target i-0123456789abcdef0

# Port forward to a remote host through a bastion
act forward --local-port 5432 --remote-port 5432 --remote-host mydb.internal.com --target i-bastion123

# Forward to an RDS instance (interactive picker)
act rds

# RDS with explicit bastion
act rds --bastion i-0123456789abcdef0

# RDS with custom local port
act rds --local-port 5433

# RDS direct connection (VPC endpoint, no bastion)
act rds --no-bastion

# RDP to a Windows instance via SSM (auto-opens RDP client on macOS/Windows)
act ec2 rdp
act ec2 rdp --key ~/.ssh/my-key.pem
act ec2 rdp --no-open --local-port 13389
act ec2 rdp --target i-0123456789abcdef0

# SSH to an instance via SSM (real SSH with ProxyCommand)
act ec2 ssh
act ec2 ssh --user ubuntu
act ec2 ssh --user ec2-user --target i-0123456789abcdef0

# Tail ECS service logs (auto-detects log group)
act ecs logs

# Tail with explicit cluster and service
act ecs logs --cluster my-cluster --service my-service

# Tail with custom time range
act ecs logs --log-group /ecs/my-service --since 1h

# Run a command on an instance (interactive picker)
act ssm run --command "systemctl status nginx"

# Run multiple commands on a specific instance
act ssm run --target i-0123456789abcdef0 --command "df -h" --command "uptime"

# Run a local script
act ssm run --script ./deploy.sh --timeout 600

# Fire-and-forget (don't wait for completion)
act ssm run --no-wait --command "sudo reboot"

# Exec into an ECS container (interactive cluster/task picker)
act ecs

# Exec into a specific cluster, filtered by service
act ecs --cluster my-cluster --service my-service

# Favorites
act fav                              # picker + connect
act fav add i-0123456789abcdef0      # add to favorites
act fav rm i-0123456789abcdef0       # remove from favorites

# Create config file interactively
act init

# Check dependencies
act doctor

# Upgrade to latest version
act upgrade
```

### `ec2` vs `ec2 ssh`

| | `act ec2` | `act ec2 ssh` |
|---|---|---|
| Protocol | SSM session (shell) | SSH over SSM tunnel |
| SSH keys needed | No | Yes (key on instance) |
| SCP/rsync | No | Yes |
| Port forwarding | Use `act forward` | SSH -L/-R flags |
| Agent forwarding | No | Yes (-A) |

To use SCP/rsync directly (without `act`), add this to `~/.ssh/config`:

```
Host i-*
    User ec2-user
    StrictHostKeyChecking no
    UserKnownHostsFile /dev/null
    ProxyCommand aws ssm start-session --target %h --document-name AWS-StartSSHSession --parameters portNumber=%p
```

Then:
```bash
scp file.txt i-0123456789abcdef0:/tmp/
rsync -avz ./dir i-0123456789abcdef0:/opt/app/
```

### Controls

| Key | Action |
|-----|--------|
| Type | Filter instances |
| Up/Down | Navigate list |
| Enter | Connect to selected instance |
| Backspace | Clear search character |
| Esc / Ctrl+C | Quit |

## Configuration

Run `act init` to create `~/.act.json` interactively, or create it manually:

```json
{
  "default_profile": "production",
  "default_region": "ap-southeast-2",
  "favorites": ["i-0123456789abcdef0"],
  "environments": {
    "prod": {
      "profile": "production",
      "region": "ap-southeast-2"
    },
    "staging": {
      "profile": "staging",
      "region": "us-west-2"
    }
  }
}
```

Resolution order for profile/region:
1. CLI flag (`--profile`, `--region`)
2. Environment lookup (`--env` name in config)
3. Environment variable (`AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`)
4. Config file defaults (`~/.act.json`)

## How it works

1. Runs AWS API calls to fetch running instances/clusters/tasks/RDS instances
2. Displays an interactive TUI for selection with real-time filtering
3. Executes the appropriate AWS CLI command (`ssm start-session`, port forwarding, `ecs execute-command`, or `logs tail`)

No custom SSH keys, no bastion hosts, no open inbound ports required.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

MIT License - see [LICENSE](LICENSE) for details.
