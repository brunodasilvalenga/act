package aws

import "testing"

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
