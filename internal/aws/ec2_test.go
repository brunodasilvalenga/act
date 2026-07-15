package aws

import "testing"

func TestDisplayName(t *testing.T) {
	tests := []struct {
		name     string
		instance Instance
		contains string
	}{
		{
			name:     "with name",
			instance: Instance{Name: "web-server", InstanceID: "i-123", PrivateIP: "10.0.0.1", InstanceType: "t3.micro"},
			contains: "web-server",
		},
		{
			name:     "without name",
			instance: Instance{Name: "", InstanceID: "i-456", PrivateIP: "10.0.0.2", InstanceType: "t3.small"},
			contains: "(no name)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.instance.DisplayName()
			if len(result) == 0 {
				t.Fatal("DisplayName returned empty string")
			}
			if !contains(result, tt.contains) {
				t.Errorf("DisplayName() = %q, want it to contain %q", result, tt.contains)
			}
			if !contains(result, tt.instance.InstanceID) {
				t.Errorf("DisplayName() = %q, want it to contain instance ID %q", result, tt.instance.InstanceID)
			}
		})
	}
}

func TestNormalizePasswordOutput(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"real password", "P@ssw0rd123!\n", "P@ssw0rd123!"},
		{"literal None", "None\n", ""},
		{"literal None no newline", "None", ""},
		{"empty string", "", ""},
		{"whitespace only", "   \n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizePasswordOutput(tt.raw); got != tt.want {
				t.Errorf("normalizePasswordOutput(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestEscapeTagFilterValue(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no comma", "production", "production"},
		{"single comma", "prod,us-east", "prod\\,us-east"},
		{"multiple commas", "a,b,c", "a\\,b\\,c"},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeTagFilterValue(tt.in); got != tt.want {
				t.Errorf("escapeTagFilterValue(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
