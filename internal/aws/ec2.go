package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Instance struct {
	InstanceID   string
	Name         string
	PrivateIP    string
	InstanceType string
}

func (i Instance) DisplayName() string {
	name := i.Name
	if name == "" {
		name = "(no name)"
	}
	return fmt.Sprintf("%-40s %-20s %-15s %s", name, i.InstanceID, i.PrivateIP, i.InstanceType)
}

type describeOutput struct {
	Reservations []struct {
		Instances []struct {
			InstanceID       string `json:"InstanceId"`
			PrivateIPAddress string `json:"PrivateIpAddress"`
			InstanceType     string `json:"InstanceType"`
			Tags             []struct {
				Key   string `json:"Key"`
				Value string `json:"Value"`
			} `json:"Tags"`
		} `json:"Instances"`
	} `json:"Reservations"`
}

func ListRunningInstances(profile, region string, tags []string) ([]Instance, error) {
	args := []string{"ec2", "describe-instances",
		"--filters", "Name=instance-state-name,Values=running",
	}

	for _, tag := range tags {
		parts := strings.SplitN(tag, "=", 2)
		if len(parts) == 2 {
			args = append(args, "--filters", fmt.Sprintf("Name=tag:%s,Values=%s", parts[0], parts[1]))
		}
	}

	args = append(args, "--output", "json")

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

	var result describeOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse aws output: %w", err)
	}

	var instances []Instance
	for _, r := range result.Reservations {
		for _, inst := range r.Instances {
			name := ""
			for _, tag := range inst.Tags {
				if tag.Key == "Name" {
					name = tag.Value
					break
				}
			}
			instances = append(instances, Instance{
				InstanceID:   inst.InstanceID,
				Name:         name,
				PrivateIP:    inst.PrivateIPAddress,
				InstanceType: inst.InstanceType,
			})
		}
	}

	return instances, nil
}
