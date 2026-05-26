# act - AWS Connect TUI

A terminal UI for connecting to AWS resources via Session Manager — EC2 instances, ECS containers, and port forwarding.

No extra dependencies — just the AWS CLI and the Session Manager plugin.

![demo](https://github.com/brunodasilvalenga/act/raw/main/demo.gif)

## Features

- **EC2 Connect** — List running instances, filter, and start SSM sessions
- **ECS Exec** — Pick a cluster, service, and task to exec into a container
- **Port Forwarding** — Forward local ports to remote instances via SSM
- **Config File** — Set defaults in `~/.act.json` (profile, region, favorites)
- **Environment Variables** — Falls back to `AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`
- **Self-Upgrade** — Update to latest version with `act upgrade`
- **Doctor** — Verify dependencies and configuration with `act doctor`
- **Cross-Platform** — Works on macOS, Linux, and Windows

## Prerequisites

- [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) installed and configured
- [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed
- IAM permissions for `ec2:DescribeInstances`, `ssm:StartSession`, `ecs:ListClusters`, `ecs:ListTasks`, `ecs:DescribeTasks`, `ecs:ExecuteCommand`
- EC2 instances must have the SSM Agent running and proper IAM role attached

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
| `forward` | Port forwarding via SSM |
| `ecs` | Connect to ECS container via execute-command |
| `doctor` | Check system dependencies and configuration |
| `upgrade` | Upgrade act to the latest version |

### Global Flags

| Flag | Description |
|------|-------------|
| `--profile` | AWS profile to use |
| `--region` | AWS region to use |
| `--version` | Show version information |

### Examples

```bash
# Connect to an EC2 instance (interactive picker)
act ec2

# Connect with a specific profile and region
act --profile production --region us-west-2 ec2

# Port forward local port 5432 to a selected instance
act forward --local-port 5432

# Port forward with explicit target
act forward --local-port 5432 --remote-port 5432 --target i-0123456789abcdef0

# Exec into an ECS container (interactive cluster/task picker)
act ecs

# Exec into a specific cluster
act ecs --cluster my-cluster

# Check dependencies
act doctor

# Upgrade to latest version
act upgrade
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

Create `~/.act.json` to set defaults:

```json
{
  "default_profile": "production",
  "default_region": "ap-southeast-2",
  "favorites": ["i-0123456789abcdef0"]
}
```

Resolution order for profile/region:
1. CLI flag (`--profile`, `--region`)
2. Environment variable (`AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`)
3. Config file (`~/.act.json`)

## How it works

1. Runs AWS API calls to fetch running instances/clusters/tasks
2. Displays an interactive TUI for selection with real-time filtering
3. Executes the appropriate AWS CLI command (`ssm start-session`, `ssm start-session` with port forwarding, or `ecs execute-command`)

No custom SSH keys, no bastion hosts, no open inbound ports required.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

MIT License - see [LICENSE](LICENSE) for details.
