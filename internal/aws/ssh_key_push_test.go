package aws

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindSSHPublicKey(t *testing.T) {
	t.Run("prefers ed25519 over rsa", func(t *testing.T) {
		home := t.TempDir()
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"id_ed25519.pub", "id_rsa.pub"} {
			if err := os.WriteFile(filepath.Join(sshDir, name), []byte("key"), 0644); err != nil {
				t.Fatal(err)
			}
		}
		got, err := findSSHPublicKey(home)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(sshDir, "id_ed25519.pub")
		if got != want {
			t.Errorf("findSSHPublicKey() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to rsa", func(t *testing.T) {
		home := t.TempDir()
		sshDir := filepath.Join(home, ".ssh")
		if err := os.MkdirAll(sshDir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sshDir, "id_rsa.pub"), []byte("key"), 0644); err != nil {
			t.Fatal(err)
		}
		got, err := findSSHPublicKey(home)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(sshDir, "id_rsa.pub")
		if got != want {
			t.Errorf("findSSHPublicKey() = %q, want %q", got, want)
		}
	})

	t.Run("errors when neither exists", func(t *testing.T) {
		home := t.TempDir()
		if _, err := findSSHPublicKey(home); err == nil {
			t.Error("expected an error when no default key exists, got nil")
		}
	})
}

func TestShellSingleQuote(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain word", "ec2-user", "'ec2-user'"},
		{"empty string", "", "''"},
		{"embedded single quote", "o'brien", `'o'"'"'brien'`},
		{"ssh-ed25519 key with comment", "ssh-ed25519 AAAAC3 me@host", "'ssh-ed25519 AAAAC3 me@host'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellSingleQuote(tt.in); got != tt.want {
				t.Errorf("shellSingleQuote(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
