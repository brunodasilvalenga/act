package config

import (
	"os"
	"testing"
)

func TestResolveProfile(t *testing.T) {
	// Flag takes priority
	result := ResolveProfile("my-profile")
	if result != "my-profile" {
		t.Errorf("expected 'my-profile', got %q", result)
	}

	// Env var when flag empty
	os.Setenv("AWS_PROFILE", "env-profile")
	defer os.Unsetenv("AWS_PROFILE")

	result = ResolveProfile("")
	if result != "env-profile" {
		t.Errorf("expected 'env-profile', got %q", result)
	}

	// Flag still wins over env
	result = ResolveProfile("flag-profile")
	if result != "flag-profile" {
		t.Errorf("expected 'flag-profile', got %q", result)
	}
}

func TestResolveRegion(t *testing.T) {
	// Flag takes priority
	result := ResolveRegion("us-west-2")
	if result != "us-west-2" {
		t.Errorf("expected 'us-west-2', got %q", result)
	}

	// AWS_REGION env var
	os.Setenv("AWS_REGION", "eu-west-1")
	defer os.Unsetenv("AWS_REGION")

	result = ResolveRegion("")
	if result != "eu-west-1" {
		t.Errorf("expected 'eu-west-1', got %q", result)
	}
}
