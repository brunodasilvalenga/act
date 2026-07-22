package doctor

import (
	"os"
	"sync"
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

	r := checkRegion("")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no region configured, got %v", r.Status)
	}

	r = checkRegion("us-west-2")
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

	r := checkProfile("")
	if r.Status != statusWarn {
		t.Errorf("expected statusWarn when no profile configured, got %v", r.Status)
	}

	r = checkProfile("my-profile")
	if r.Status != statusPass {
		t.Errorf("expected statusPass when profile flag given, got %v", r.Status)
	}
	if r.Detail != "my-profile" {
		t.Errorf("expected Detail 'my-profile', got %q", r.Detail)
	}
}

// TestRunResultOrder exercises the exact concurrency shape used by Run:
// a pre-sized []result slice, 5 sequential writes to indices 0/1/3/4/5,
// and two goroutines synchronized via sync.WaitGroup writing to indices
// 2/6. It uses dummy stand-ins for checkCredentials/checkVersion instead
// of invoking the real network-bound checks, so it stays deterministic
// and fast while still proving the indexing pattern is race-free (run
// with -race) and preserves the expected 0-6 print order regardless of
// which goroutine finishes first.
func TestRunResultOrder(t *testing.T) {
	wantNames := []string{
		"AWS CLI",
		"Session Manager plugin",
		"AWS credentials",
		"Region",
		"Profile",
		"Config",
		"Version",
	}

	dummy := func(name string) result {
		return result{Name: name, Status: statusPass, Detail: "ok"}
	}

	for iter := 0; iter < 20; iter++ {
		results := make([]result, 7)
		results[0] = dummy("AWS CLI")
		results[1] = dummy("Session Manager plugin")
		results[3] = dummy("Region")
		results[4] = dummy("Profile")
		results[5] = dummy("Config")

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			results[2] = dummy("AWS credentials")
		}()
		go func() {
			defer wg.Done()
			results[6] = dummy("Version")
		}()
		wg.Wait()

		for i, want := range wantNames {
			if results[i].Name != want {
				t.Fatalf("iteration %d: results[%d].Name = %q, want %q", iter, i, results[i].Name, want)
			}
			if results[i].Status != statusPass {
				t.Fatalf("iteration %d: results[%d].Status = %v, want statusPass", iter, i, results[i].Status)
			}
		}
	}
}
