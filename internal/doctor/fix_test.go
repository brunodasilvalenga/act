package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestFixLogPath(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	got := fixLogPath()
	want := filepath.Join(tmpDir, ".act-doctor-fix.log")
	if got != want {
		t.Errorf("fixLogPath() = %q, want %q", got, want)
	}
}

func TestRunFixesSkipsPassingAndNoFixChecks(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	applyCalled := false
	results := []result{
		{Name: "Region", Status: statusWarn, Detail: "not configured"},
		{Name: "AWS CLI", Status: statusFail, Detail: "not found", Fix: &fixAction{
			Describe: func() string { return "test fix" },
			Apply: func(w io.Writer) error {
				applyCalled = true
				fmt.Fprintln(w, "applying")
				return nil
			},
		}},
	}

	// Simulate declining the prompt by feeding "n\n" on stdin.
	r, w, _ := os.Pipe()
	origStdin := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = origStdin }()
	w.WriteString("n\n")
	w.Close()

	_, _ = runFixes(results, false, nil)

	if applyCalled {
		t.Error("Apply should not run when the user declines the prompt")
	}
}

func TestRunFixesSkipConfirmRunsApply(t *testing.T) {
	tmpDir := t.TempDir()
	overrideHomeForDoctorTest(t, tmpDir)

	applyCalled := false
	results := []result{
		{Name: "AWS CLI", Status: statusFail, Detail: "not found", Fix: &fixAction{
			Describe: func() string { return "test fix" },
			Apply: func(w io.Writer) error {
				applyCalled = true
				return nil
			},
		}},
	}

	_, allOK := runFixes(results, true, func(name string) result {
		return result{Name: name, Status: statusPass, Detail: "fixed"}
	})

	if !applyCalled {
		t.Error("Apply should run when skipConfirm is true")
	}
	if !allOK {
		t.Error("expected allOK true when Apply succeeds")
	}

	logPath := fixLogPath()
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("expected fix log to exist at %s: %v", logPath, err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty fix log")
	}
}
