package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

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
