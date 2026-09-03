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
