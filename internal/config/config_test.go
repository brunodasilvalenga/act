package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveProfile(t *testing.T) {
	// Flag takes priority
	result := ResolveProfile("my-profile", "")
	if result != "my-profile" {
		t.Errorf("expected 'my-profile', got %q", result)
	}

	// Env var when flag empty
	os.Setenv("AWS_PROFILE", "env-profile")
	defer os.Unsetenv("AWS_PROFILE")

	result = ResolveProfile("", "")
	if result != "env-profile" {
		t.Errorf("expected 'env-profile', got %q", result)
	}

	// Flag still wins over env
	result = ResolveProfile("flag-profile", "")
	if result != "flag-profile" {
		t.Errorf("expected 'flag-profile', got %q", result)
	}
}

func TestResolveRegion(t *testing.T) {
	// Flag takes priority
	result := ResolveRegion("us-west-2", "")
	if result != "us-west-2" {
		t.Errorf("expected 'us-west-2', got %q", result)
	}

	// AWS_REGION env var
	os.Setenv("AWS_REGION", "eu-west-1")
	defer os.Unsetenv("AWS_REGION")

	result = ResolveRegion("", "")
	if result != "eu-west-1" {
		t.Errorf("expected 'eu-west-1', got %q", result)
	}
}

func TestResolveWithEnvironment(t *testing.T) {
	// Create temp config with environments
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, ".act.json")

	content := `{
		"default_profile": "default-prof",
		"default_region": "us-east-1",
		"environments": {
			"prod": {"profile": "prod-profile", "region": "ap-southeast-2"},
			"staging": {"profile": "staging-profile", "region": "eu-west-1"}
		}
	}`
	os.WriteFile(tmpFile, []byte(content), 0644)

	// Override HOME for test
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Clear env vars
	os.Unsetenv("AWS_PROFILE")
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	// Environment lookup
	result := ResolveProfile("", "prod")
	if result != "prod-profile" {
		t.Errorf("expected 'prod-profile', got %q", result)
	}

	result = ResolveRegion("", "prod")
	if result != "ap-southeast-2" {
		t.Errorf("expected 'ap-southeast-2', got %q", result)
	}

	// Flag still wins over env name
	result = ResolveProfile("flag-prof", "prod")
	if result != "flag-prof" {
		t.Errorf("expected 'flag-prof', got %q", result)
	}
}

func TestAddRemoveFavorite(t *testing.T) {
	tmpDir := t.TempDir()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	defer os.Setenv("HOME", origHome)

	// Init config
	Init("test", "us-east-1")

	// Add favorite
	err := AddFavorite("i-123")
	if err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}

	cfg := Load()
	if len(cfg.Favorites) != 1 || cfg.Favorites[0] != "i-123" {
		t.Errorf("expected favorites [i-123], got %v", cfg.Favorites)
	}

	// Add duplicate - no error, no duplicate
	AddFavorite("i-123")
	cfg = Load()
	if len(cfg.Favorites) != 1 {
		t.Errorf("expected 1 favorite, got %d", len(cfg.Favorites))
	}

	// Remove
	err = RemoveFavorite("i-123")
	if err != nil {
		t.Fatalf("RemoveFavorite failed: %v", err)
	}
	cfg = Load()
	if len(cfg.Favorites) != 0 {
		t.Errorf("expected 0 favorites, got %d", len(cfg.Favorites))
	}
}
