package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/brunodasilvalenga/act/internal/aws"
	"github.com/brunodasilvalenga/act/internal/config"
	"github.com/brunodasilvalenga/act/internal/tui"
)

func runFav(profile, region string, subArgs []string) {
	if len(subArgs) == 0 {
		// Show picker from favorites
		cfg := config.Load()
		if len(cfg.Favorites) == 0 {
			fmt.Fprintf(os.Stderr, "No favorites configured. Use 'act fav add <instance-id>' to add one.\n")
			os.Exit(0)
		}

		picked, err := tui.RunPicker("Select Favorite Instance", cfg.Favorites)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if picked == "" {
			os.Exit(0)
		}

		err = aws.StartSession(picked, profile, region)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting session: %v\n", err)
			os.Exit(1)
		}
		return
	}

	switch subArgs[0] {
	case "list":
		favorites := config.ListFavorites()
		if len(favorites) == 0 {
			fmt.Println("No favorites configured.")
			return
		}
		for _, f := range favorites {
			fmt.Println(f)
		}

	case "add":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav add <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if !strings.HasPrefix(id, "i-") {
			fmt.Fprintf(os.Stderr, "Error: instance ID must start with 'i-'\n")
			os.Exit(1)
		}
		if err := config.AddFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error adding favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Added %s to favorites.\n", id)

	case "rm":
		if len(subArgs) < 2 {
			fmt.Fprintf(os.Stderr, "Usage: act fav rm <instance-id>\n")
			os.Exit(1)
		}
		id := subArgs[1]
		if err := config.RemoveFavorite(id); err != nil {
			fmt.Fprintf(os.Stderr, "Error removing favorite: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Removed %s from favorites.\n", id)

	default:
		fmt.Fprintf(os.Stderr, "Unknown fav subcommand: %s\n", subArgs[0])
		printFavHelp()
		os.Exit(1)
	}
}
