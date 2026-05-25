# act - AWS Connect TUI

A minimal terminal UI for connecting to EC2 instances via AWS Session Manager.

No extra dependencies — just the AWS CLI and the Session Manager plugin.

![demo](https://github.com/brunodasilvalenga/act/raw/main/demo.gif)

## Features

- Lists all running EC2 instances (name, ID, private IP, type)
- Real-time search/filter as you type
- Keyboard navigation
- Connects via `aws ssm start-session` on selection
- Supports `--profile` and `--region` flags

## Prerequisites

- [AWS CLI v2](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) installed and configured
- [Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) installed
- IAM permissions for `ec2:DescribeInstances` and `ssm:StartSession`
- EC2 instances must have the SSM Agent running and proper IAM role attached

## Installation

### From source

```bash
go install github.com/brunodasilvalenga/act@latest
```

### From releases

Download the binary for your platform from the [Releases](https://github.com/brunodasilvalenga/act/releases) page.

### macOS (Homebrew)

```bash
brew install brunodasilvalenga/tap/act
```

### Manual build

```bash
git clone https://github.com/brunodasilvalenga/act.git
cd act
make install
```

## Usage

```bash
# Default profile and region
act

# Specific AWS profile
act --profile production

# Specific region
act --region us-west-2

# Both
act --profile production --region eu-west-1
```

### Controls

| Key | Action |
|-----|--------|
| Type | Filter instances |
| Up/Down | Navigate list |
| Enter | Connect to selected instance |
| Backspace | Clear search character |
| Esc / Ctrl+C | Quit |

## How it works

1. Runs `aws ec2 describe-instances` to fetch running instances
2. Displays an interactive TUI for selection
3. Execs `aws ssm start-session --target <instance-id>` (replaces the process)

No custom SSH keys, no bastion hosts, no open inbound ports required.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

MIT License - see [LICENSE](LICENSE) for details.
