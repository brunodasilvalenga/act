package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type listServicesOutput struct {
	ServiceArns []string `json:"serviceArns"`
}

type describeServicesOutput struct {
	Services []struct {
		TaskDefinition string `json:"taskDefinition"`
	} `json:"services"`
}

type describeTaskDefOutput struct {
	TaskDefinition struct {
		ContainerDefinitions []struct {
			Name             string `json:"name"`
			LogConfiguration *struct {
				LogDriver string            `json:"logDriver"`
				Options   map[string]string `json:"options"`
			} `json:"logConfiguration"`
		} `json:"containerDefinitions"`
	} `json:"taskDefinition"`
}

func ListECSServices(cluster, profile, region string) ([]string, error) {
	args := []string{"ecs", "list-services", "--cluster", cluster, "--output", "json"}
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
			return nil, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var result listServicesOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse services output: %w", err)
	}

	services := make([]string, len(result.ServiceArns))
	for i, arn := range result.ServiceArns {
		parts := strings.Split(arn, "/")
		services[i] = parts[len(parts)-1]
	}
	return services, nil
}

func GetLogGroupsFromService(cluster, service, profile, region string) ([]string, error) {
	// Get task definition ARN from service
	descArgs := []string{"ecs", "describe-services", "--cluster", cluster, "--services", service, "--output", "json"}
	if profile != "" {
		descArgs = append(descArgs, "--profile", profile)
	}
	if region != "" {
		descArgs = append(descArgs, "--region", region)
	}

	cmd := exec.Command("aws", descArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var svcResult describeServicesOutput
	if err := json.Unmarshal(out, &svcResult); err != nil {
		return nil, fmt.Errorf("failed to parse services description: %w", err)
	}
	if len(svcResult.Services) == 0 {
		return nil, fmt.Errorf("service %s not found", service)
	}

	taskDefARN := svcResult.Services[0].TaskDefinition

	// Describe task definition to get log groups
	tdArgs := []string{"ecs", "describe-task-definition", "--task-definition", taskDefARN, "--output", "json"}
	if profile != "" {
		tdArgs = append(tdArgs, "--profile", profile)
	}
	if region != "" {
		tdArgs = append(tdArgs, "--region", region)
	}

	cmd = exec.Command("aws", tdArgs...)
	out, err = cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var tdResult describeTaskDefOutput
	if err := json.Unmarshal(out, &tdResult); err != nil {
		return nil, fmt.Errorf("failed to parse task definition: %w", err)
	}

	seen := make(map[string]bool)
	var groups []string
	for _, container := range tdResult.TaskDefinition.ContainerDefinitions {
		if container.LogConfiguration != nil && container.LogConfiguration.LogDriver == "awslogs" {
			if group, ok := container.LogConfiguration.Options["awslogs-group"]; ok && !seen[group] {
				seen[group] = true
				groups = append(groups, group)
			}
		}
	}

	return groups, nil
}
