package main

import (
	"fmt"
	"os"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/tui"
)

// parseTags extracts --tag key=value flags from args, returns remaining args and tags.

func parseTags(args []string) ([]string, []string) {
	var remaining []string
	var tags []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--tag" || args[i] == "-tag" {
			if i+1 < len(args) {
				i++
				tags = append(tags, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, tags
}

func pickInstance(loadFunc func() ([]aws.Instance, error)) string {
	selected, err := tui.Run(loadFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if selected == nil {
		os.Exit(0)
	}
	return selected.InstanceID
}

func parseCommands(args []string) ([]string, []string) {
	var remaining []string
	var commands []string
	for i := 0; i < len(args); i++ {
		if args[i] == "--command" || args[i] == "-command" {
			if i+1 < len(args) {
				i++
				commands = append(commands, args[i])
			}
		} else {
			remaining = append(remaining, args[i])
		}
	}
	return remaining, commands
}
