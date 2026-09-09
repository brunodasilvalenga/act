package aws

import (
	"strings"
	"testing"
	"time"
)

func TestDocumentForPlatform(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		want     string
	}{
		{name: "windows lowercase", platform: "windows", want: "AWS-RunPowerShellScript"},
		{name: "windows mixed case", platform: "Windows", want: "AWS-RunPowerShellScript"},
		{name: "empty is linux", platform: "", want: "AWS-RunShellScript"},
		{name: "linux explicit", platform: "linux", want: "AWS-RunShellScript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DocumentForPlatform(tt.platform)
			if got != tt.want {
				t.Errorf("DocumentForPlatform(%q) = %q, want %q", tt.platform, got, tt.want)
			}
		})
	}
}

func TestMaxWaitFromTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name           string
		timeoutSeconds int
		want           time.Duration
	}{
		{name: "small timeout floored to 1 minute", timeoutSeconds: 10, want: time.Minute},
		{name: "normal timeout doubled", timeoutSeconds: 300, want: 10 * time.Minute},
		{name: "large timeout capped at 30 minutes", timeoutSeconds: 5000, want: 30 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MaxWaitFromTimeoutSeconds(tt.timeoutSeconds)
			if got != tt.want {
				t.Errorf("MaxWaitFromTimeoutSeconds(%d) = %s, want %s", tt.timeoutSeconds, got, tt.want)
			}
		})
	}
}

func TestWaitForCommandInvocation_TimesOutWhenDeadlineAlreadyPassed(t *testing.T) {
	start := time.Now()
	result, err := WaitForCommandInvocation("cmd-1", "i-1", "", "", time.Millisecond, -1*time.Second)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out after") {
		t.Errorf("expected error to contain %q, got: %v", "timed out after", err)
	}
	if !strings.Contains(err.Error(), "cmd-1") {
		t.Errorf("expected error to reference the command ID, got: %v", err)
	}
	if result != (CommandInvocationResult{}) {
		t.Errorf("expected zero-value result on timeout, got: %+v", result)
	}
	if elapsed > time.Second {
		t.Errorf("expected the already-past-deadline case to return almost immediately without shelling out to aws, took %s", elapsed)
	}
}
