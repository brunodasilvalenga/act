package config

import (
	"os"
	"path/filepath"

	"encoding/json"
)

type Environment struct {
	Profile string `json:"profile"`
	Region  string `json:"region"`
}

type Config struct {
	DefaultProfile string                 `json:"default_profile,omitempty"`
	DefaultRegion  string                 `json:"default_region,omitempty"`
	Favorites      []string               `json:"favorites,omitempty"`
	Environments   map[string]Environment `json:"environments,omitempty"`
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".act.json")
}

func Exists() bool {
	path := ConfigPath()
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

func Init(profile, region string) error {
	path := ConfigPath()
	cfg := Config{
		DefaultProfile: profile,
		DefaultRegion:  region,
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func Load() Config {
	path := ConfigPath()
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

func Save(cfg Config) error {
	path := ConfigPath()
	if path == "" {
		return os.ErrNotExist
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

func AddFavorite(instanceID string) error {
	cfg := Load()
	for _, f := range cfg.Favorites {
		if f == instanceID {
			return nil // already exists
		}
	}
	cfg.Favorites = append(cfg.Favorites, instanceID)
	return Save(cfg)
}

func RemoveFavorite(instanceID string) error {
	cfg := Load()
	var updated []string
	for _, f := range cfg.Favorites {
		if f != instanceID {
			updated = append(updated, f)
		}
	}
	cfg.Favorites = updated
	return Save(cfg)
}

func ResolveProfile(flagValue, envName string) string {
	if flagValue != "" {
		return flagValue
	}
	if envName != "" {
		cfg := Load()
		if env, ok := cfg.Environments[envName]; ok && env.Profile != "" {
			return env.Profile
		}
	}
	if env := os.Getenv("AWS_PROFILE"); env != "" {
		return env
	}
	cfg := Load()
	return cfg.DefaultProfile
}

func ResolveRegion(flagValue, envName string) string {
	if flagValue != "" {
		return flagValue
	}
	if envName != "" {
		cfg := Load()
		if env, ok := cfg.Environments[envName]; ok && env.Region != "" {
			return env.Region
		}
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
