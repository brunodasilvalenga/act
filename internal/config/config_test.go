package config

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// overrideHome sets HOME (Unix) and USERPROFILE (Windows) to dir for tests.
func overrideHome(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	if runtime.GOOS == "windows" {
		origUP := os.Getenv("USERPROFILE")
		os.Setenv("USERPROFILE", dir)
		t.Cleanup(func() { os.Setenv("USERPROFILE", origUP) })
	}
	t.Cleanup(func() { os.Setenv("HOME", orig) })
}

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
	overrideHome(t, tmpDir)

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
	overrideHome(t, tmpDir)

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

func TestListFavorites(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	Init("test", "us-east-1")

	if got := ListFavorites(); len(got) != 0 {
		t.Errorf("expected no favorites initially, got %v", got)
	}

	AddFavorite("i-abc")
	AddFavorite("i-def")

	got := ListFavorites()
	want := []string{"i-abc", "i-def"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListFavorites() = %v, want %v", got, want)
	}
}

func TestEnvironmentCRUD(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	Init("test", "us-east-1")

	if got := ListEnvironments(); len(got) != 0 {
		t.Errorf("expected no environments initially, got %v", got)
	}

	if err := AddEnvironment("prod", "prod-profile", "us-west-2"); err != nil {
		t.Fatalf("AddEnvironment failed: %v", err)
	}

	envs := ListEnvironments()
	if len(envs) != 1 {
		t.Fatalf("expected 1 environment, got %d", len(envs))
	}
	if envs["prod"].Profile != "prod-profile" || envs["prod"].Region != "us-west-2" {
		t.Errorf("unexpected environment value: %+v", envs["prod"])
	}

	// Upsert: adding again with different values overwrites.
	if err := AddEnvironment("prod", "prod-profile-v2", "eu-west-1"); err != nil {
		t.Fatalf("AddEnvironment (update) failed: %v", err)
	}
	envs = ListEnvironments()
	if envs["prod"].Profile != "prod-profile-v2" || envs["prod"].Region != "eu-west-1" {
		t.Errorf("expected AddEnvironment to overwrite existing entry, got %+v", envs["prod"])
	}

	if err := RemoveEnvironment("prod"); err != nil {
		t.Fatalf("RemoveEnvironment failed: %v", err)
	}
	envs = ListEnvironments()
	if len(envs) != 0 {
		t.Errorf("expected 0 environments after remove, got %d", len(envs))
	}
}

func TestConfigFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix file permission bits are not meaningful on Windows")
	}

	tmpDir := t.TempDir()
	overrideHome(t, tmpDir)

	if err := Init("test-profile", "us-east-1"); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	info, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatalf("failed to stat config file: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected Init to write config with mode 0600, got %v", perm)
	}

	// Save (exercised indirectly via AddFavorite) must also use 0600.
	if err := AddFavorite("i-999"); err != nil {
		t.Fatalf("AddFavorite failed: %v", err)
	}
	info, err = os.Stat(ConfigPath())
	if err != nil {
		t.Fatalf("failed to stat config file after Save: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected Save to write config with mode 0600, got %v", perm)
	}
}
