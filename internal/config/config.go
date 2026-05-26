package config

import (
	"os"
	"path/filepath"

	"encoding/json"
)

type Config struct {
	DefaultProfile string   `json:"default_profile,omitempty"`
	DefaultRegion  string   `json:"default_region,omitempty"`
	Favorites      []string `json:"favorites,omitempty"`
}

func configPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".act.json")
}

func Load() Config {
	path := configPath()
	if path == "" {
		return Config{}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}
	}

	var cfg Config
	json.Unmarshal(data, &cfg)
	return cfg
}

func ResolveProfile(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("AWS_PROFILE"); env != "" {
		return env
	}
	cfg := Load()
	return cfg.DefaultProfile
}

func ResolveRegion(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("AWS_REGION"); env != "" {
		return env
	}
	if env := os.Getenv("AWS_DEFAULT_REGION"); env != "" {
		return env
	}
	cfg := Load()
	return cfg.DefaultRegion
}
