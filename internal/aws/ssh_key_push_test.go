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

func TestBuildPushKeyScript(t *testing.T) {
	const key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIExampleKeyMaterial user@laptop"

	t.Run("dedup check greps for the raw key, not the marker-tagged line", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyQ := shellSingleQuote(key)
		if !strings.Contains(script, "grep -qF "+keyQ) {
			t.Errorf("script does not grep for the raw key %q; got:\n%s", keyQ, script)
		}
	})

	t.Run("dedup grep target is identical across two different markers for the same key", func(t *testing.T) {
		// This is the regression this plan fixes: the old code grepped for
		// the marker-tagged line, so a previous run's line (different
		// marker, same key) could never be found as "already present".
		scriptA := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		scriptB := buildPushKeyScript("ec2-user", key, "act-push-key-222")
		keyQ := shellSingleQuote(key)
		grepLine := "grep -qF " + keyQ
		if !strings.Contains(scriptA, grepLine) || !strings.Contains(scriptB, grepLine) {
			t.Fatalf("expected both scripts to contain the marker-independent grep line %q", grepLine)
		}
	})

	t.Run("does not regress to matching on the full marker-tagged line", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyLineQ := shellSingleQuote(key + " act-push-key act-push-key-111")
		if strings.Contains(script, "grep -qF "+keyLineQ) || strings.Contains(script, "grep -qxF "+keyLineQ) {
			t.Errorf("dedup check still matches on the marker-tagged line, not just the key; got:\n%s", script)
		}
	})

	t.Run("append branch still tags the new line with the marker for later removal", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		keyLineQ := shellSingleQuote(key + " act-push-key act-push-key-111")
		if !strings.Contains(script, "echo "+keyLineQ+" >> ") {
			t.Errorf("expected the appended line to still carry its marker; got:\n%s", script)
		}
	})

	t.Run("prints an already-present marker line so the caller can detect the skip", func(t *testing.T) {
		script := buildPushKeyScript("ec2-user", key, "act-push-key-111")
		if !strings.Contains(script, `echo "act-push-key: already present"`) {
			t.Errorf("expected an already-present status line; got:\n%s", script)
		}
	})
}
