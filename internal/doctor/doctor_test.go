package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name    string
		jsonStr string
		key     string
		want    string
	}{
		{
			name:    "arn present",
			jsonStr: `{"UserId": "AIDA...", "Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/alice"}`,
			key:     "Arn",
			want:    "arn:aws:iam::123456789012:user/alice",
		},
		{
			name:    "account present",
			jsonStr: `{"UserId": "AIDA...", "Account": "123456789012", "Arn": "arn:aws:iam::123456789012:user/alice"}`,
			key:     "Account",
			want:    "123456789012",
		},
		{
			name:    "key not found",
			jsonStr: `{"UserId": "AIDA..."}`,
			key:     "Arn",
			want:    "",
		},
		{
			name:    "empty input",
			jsonStr: "",
			key:     "Arn",
			want:    "",
		},
		{
			name:    "malformed json still extracts if pattern matches",
			jsonStr: `not real json but "Arn": "arn:aws:iam::123:user/x" happens to match`,
			key:     "Arn",
			want:    "arn:aws:iam::123:user/x",
		},
		{
			name:    "no space after colon does not match (documents brittle behavior)",
			jsonStr: `{"Arn":"arn:aws:iam::123:user/x"}`,
			key:     "Arn",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSON(tt.jsonStr, tt.key)
			if got != tt.want {
				t.Errorf("extractJSON(%q, %q) = %q, want %q", tt.jsonStr, tt.key, got, tt.want)
			}
		})
	}
}

func overrideHomeForDoctorTest(t *testing.T, dir string) {
	t.Helper()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() { os.Setenv("HOME", orig) })
}

func TestCheckRegion(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	r := checkRegion("", "")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no region configured, got %v", r.Status)
	}

	r = checkRegion("us-west-2", "")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when region flag given, got %v", r.Status)
	}
	if r.Detail != "us-west-2" {
		t.Errorf("expected Detail 'us-west-2', got %q", r.Detail)
	}
}

func TestCheckProfile(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_PROFILE")

	r := checkProfile("", "")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no profile configured, got %v", r.Status)
	}

	r = checkProfile("my-profile", "")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when profile flag given, got %v", r.Status)
	}
	if r.Detail != "my-profile" {
		t.Errorf("expected Detail 'my-profile', got %q", r.Detail)
	}
}

func TestCheckRegionWithEnv(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_REGION")
	os.Unsetenv("AWS_DEFAULT_REGION")

	cfgJSON := `{"environments": {"staging": {"profile": "stagingprof", "region": "eu-west-1"}}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".act.json"), []byte(cfgJSON), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	r := checkRegion("", "staging")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when env has a region, got %v (%s)", r.Status, r.Detail)
	}
	if r.Detail != "eu-west-1" {
		t.Errorf("expected Detail 'eu-west-1', got %q", r.Detail)
	}

	r = checkRegion("us-east-2", "staging")
	if r.Detail != "us-east-2" {
		t.Errorf("expected explicit --region to win over --env, got %q", r.Detail)
	}
}

func TestCheckProfileWithEnv(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)
	os.Unsetenv("AWS_PROFILE")

	cfgJSON := `{"environments": {"staging": {"profile": "stagingprof", "region": "eu-west-1"}}}`
	if err := os.WriteFile(filepath.Join(tmpDir, ".act.json"), []byte(cfgJSON), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	r := checkProfile("", "staging")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when env has a profile, got %v (%s)", r.Status, r.Detail)
	}
	if r.Detail != "stagingprof" {
		t.Errorf("expected Detail 'stagingprof', got %q", r.Detail)
	}

	r = checkProfile("explicit-profile", "staging")
	if r.Detail != "explicit-profile" {
		t.Errorf("expected explicit --profile to win over --env, got %q", r.Detail)
	}
}
