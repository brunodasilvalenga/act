package aws

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type RDSInstance struct {
	Identifier string
	Engine     string
	Endpoint   string
	Port       int
}

func (r RDSInstance) DisplayName() string {
	return fmt.Sprintf("%-40s %-12s %s:%d", r.Identifier, r.Engine, r.Endpoint, r.Port)
}

type describeDBOutput struct {
	DBInstances []struct {
		DBInstanceIdentifier string `json:"DBInstanceIdentifier"`
		Engine               string `json:"Engine"`
		Endpoint             *struct {
			Address string `json:"Address"`
			Port    int    `json:"Port"`
		} `json:"Endpoint"`
	} `json:"DBInstances"`
}

func ListRDSInstances(profile, region string) ([]RDSInstance, error) {
	args := []string{"rds", "describe-db-instances", "--output", "json"}

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

	var result describeDBOutput
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("failed to parse RDS output: %w", err)
	}

	var instances []RDSInstance
	for _, db := range result.DBInstances {
		if db.Endpoint == nil {
			continue
		}
		instances = append(instances, RDSInstance{
			Identifier: db.DBInstanceIdentifier,
			Engine:     db.Engine,
			Endpoint:   db.Endpoint.Address,
			Port:       db.Endpoint.Port,
		})
	}

	return instances, nil
}
