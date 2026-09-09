package aws

import (
	"os"
	"path/filepath"
	"strings"
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

func TestRejectIfWindows(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		wantErr  bool
	}{
		{name: "windows lowercase", platform: "windows", wantErr: true},
		{name: "windows mixed case", platform: "Windows", wantErr: true},
		{name: "empty is linux", platform: "", wantErr: false},
		{name: "linux explicit", platform: "linux", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectIfWindows(tt.platform)
			if tt.wantErr && err == nil {
				t.Errorf("rejectIfWindows(%q) = nil, want an error", tt.platform)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("rejectIfWindows(%q) = %v, want nil", tt.platform, err)
			}
		})
	}
}

func TestRejectIfWindowsErrorMentionsRDP(t *testing.T) {
	err := rejectIfWindows("windows")
	if err == nil {
		t.Fatal("expected an error for windows platform")
	}
	if !strings.Contains(err.Error(), "ec2 rdp") {
		t.Errorf("rejectIfWindows error = %q, want it to mention 'ec2 rdp'", err.Error())
	}
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
