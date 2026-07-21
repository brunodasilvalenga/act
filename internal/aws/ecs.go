package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type ECSTask struct {
	TaskARN       string
	TaskID        string
	ContainerName string
	ClusterARN    string
	ClusterName   string
	ServiceName   string
}

func (t ECSTask) DisplayName() string {
	return fmt.Sprintf("%-30s %-30s %s", t.ServiceName, t.ContainerName, t.TaskID)
}

type listTasksOutput struct {
	TaskArns []string `json:"taskArns"`
}

type describeTasksOutput struct {
	Tasks []struct {
		TaskArn    string `json:"taskArn"`
		ClusterArn string `json:"clusterArn"`
		Group      string `json:"group"`
		Containers []struct {
			Name string `json:"name"`
		} `json:"containers"`
	} `json:"tasks"`
}

type listClustersOutput struct {
	ClusterArns []string `json:"clusterArns"`
}

// arnSuffix returns the last "/"-delimited segment of an ARN — the
// resource name/ID that AWS CLI list-* commands return as a full ARN but
// that this tool displays and passes to other commands as a bare name.
// logs.go's ListECSServices also uses this helper.
func arnSuffix(arn string) string {
	parts := strings.Split(arn, "/")
	return parts[len(parts)-1]
}

// serviceNameFromGroup extracts the service name from an ECS task's Group
// field. AWS formats Group as "service:<name>" for tasks launched by a
// service, and leaves it as some other value (e.g. "family:<name>") for
// standalone tasks not launched by a service; TrimPrefix returns the input
// unchanged when the prefix isn't present, so this already handles both
// cases correctly.
func serviceNameFromGroup(group string) string {
	return strings.TrimPrefix(group, "service:")
}

func ListECSClusters(profile, region string) ([]string, error) {
	args := []string{"ecs", "list-clusters", "--output", "json"}
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

	var result listClustersOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse clusters output: %w", err)
	}

	clusters := make([]string, len(result.ClusterArns))
	for i, arn := range result.ClusterArns {
		clusters[i] = arnSuffix(arn)
	}
	return clusters, nil
}

func ListECSTasks(cluster, profile, region, serviceName string) ([]ECSTask, error) {
	// List task ARNs
	listArgs := []string{"ecs", "list-tasks", "--cluster", cluster, "--desired-status", "RUNNING", "--output", "json"}
	if serviceName != "" {
		listArgs = append(listArgs, "--service-name", serviceName)
	}
	if profile != "" {
		listArgs = append(listArgs, "--profile", profile)
	}
	if region != "" {
		listArgs = append(listArgs, "--region", region)
	}

	cmd := exec.Command("aws", listArgs...)
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var listResult listTasksOutput
	if err := json.Unmarshal(out, &listResult); err != nil {
		return nil, fmt.Errorf("failed to parse tasks list: %w", err)
	}

	if len(listResult.TaskArns) == 0 {
		return nil, nil
	}

	// Describe tasks to get container info
	descArgs := []string{"ecs", "describe-tasks", "--cluster", cluster, "--tasks"}
	descArgs = append(descArgs, listResult.TaskArns...)
	descArgs = append(descArgs, "--output", "json")
	if profile != "" {
		descArgs = append(descArgs, "--profile", profile)
	}
	if region != "" {
		descArgs = append(descArgs, "--region", region)
	}

	cmd = exec.Command("aws", descArgs...)
	out, err = cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("aws cli error: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var descResult describeTasksOutput
	if err := json.Unmarshal(out, &descResult); err != nil {
		return nil, fmt.Errorf("failed to parse tasks description: %w", err)
	}

	var tasks []ECSTask
	for _, task := range descResult.Tasks {
		taskID := arnSuffix(task.TaskArn)
		serviceName := serviceNameFromGroup(task.Group)

		for _, container := range task.Containers {
			tasks = append(tasks, ECSTask{
				TaskARN:       task.TaskArn,
				TaskID:        taskID,
				ContainerName: container.Name,
				ClusterARN:    task.ClusterArn,
				ClusterName:   cluster,
				ServiceName:   serviceName,
			})
		}
	}

	return tasks, nil
}
