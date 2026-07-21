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

// pickInstance runs the interactive picker for the given loadFunc and
// returns the selected instance ID. It exits the process (0) if the user
// quits the picker without selecting, and exits (1) on error — matching
// the behavior every call site had before this helper was extracted.
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

// parseCommands extracts --command flags from args (can be repeated,
// one shell line per occurrence), returns remaining args and commands.
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
