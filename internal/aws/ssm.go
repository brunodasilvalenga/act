package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DocumentForPlatform returns the SSM Run Command document for the given
// EC2 Platform value ("windows" for Windows instances, "" or "linux" otherwise).
func DocumentForPlatform(platform string) string {
	if strings.EqualFold(platform, "windows") {
		return "AWS-RunPowerShellScript"
	}
	return "AWS-RunShellScript"
}

type sendCommandOutput struct {
	Command struct {
		CommandID string `json:"CommandId"`
	} `json:"Command"`
}

// SendCommand submits an SSM Run Command against a single instance and
// returns the resulting command ID.
func SendCommand(instanceID, profile, region, document string, commands []string, timeoutSeconds int, comment string) (string, error) {
	params := map[string][]string{"commands": commands}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to encode command parameters: %w", err)
	}

	args := []string{"ssm", "send-command",
		"--document-name", document,
		"--instance-ids", instanceID,
		"--parameters", string(paramsJSON),
		"--timeout-seconds", fmt.Sprintf("%d", timeoutSeconds),
		"--output", "json",
	}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	cmd := exec.Command("aws", args...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", err
	}

	var result sendCommandOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return "", fmt.Errorf("failed to parse send-command output: %w", err)
	}
	return result.Command.CommandID, nil
}

type commandInvocationOutput struct {
	Status                string `json:"Status"`
	StandardOutputContent string `json:"StandardOutputContent"`
	StandardErrorContent  string `json:"StandardErrorContent"`
	ResponseCode          int    `json:"ResponseCode"`
}

// CommandInvocationResult is the outcome of a completed (or still-running,
// on error) SSM command invocation.
type CommandInvocationResult struct {
	Status   string
	Stdout   string
	Stderr   string
	ExitCode int
}

// WaitForCommandInvocation polls ssm get-command-invocation until the
// command reaches a terminal status (Success, Failed, Cancelled, TimedOut).
func WaitForCommandInvocation(commandID, instanceID, profile, region string, pollInterval time.Duration) (CommandInvocationResult, error) {
	args := []string{"ssm", "get-command-invocation",
		"--command-id", commandID,
		"--instance-id", instanceID,
		"--output", "json",
	}
	if profile != "" {
		args = append(args, "--profile", profile)
	}
	if region != "" {
		args = append(args, "--region", region)
	}

	for {
		cmd := exec.Command("aws", args...)
		out, err := cmd.Output()
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return CommandInvocationResult{}, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
			}
			return CommandInvocationResult{}, err
		}

		var result commandInvocationOutput
		if err := json.Unmarshal(out, &result); err != nil {
			return CommandInvocationResult{}, fmt.Errorf("failed to parse get-command-invocation output: %w", err)
		}

		switch result.Status {
		case "Pending", "InProgress", "Delayed":
			time.Sleep(pollInterval)
			continue
		default:
			return CommandInvocationResult{
				Status:   result.Status,
				Stdout:   result.StandardOutputContent,
				Stderr:   result.StandardErrorContent,
				ExitCode: result.ResponseCode,
			}, nil
		}
	}
}
