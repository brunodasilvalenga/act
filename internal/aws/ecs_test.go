package aws

import "testing"

func TestArnSuffix(t *testing.T) {
	tests := []struct {
		name string
		arn  string
		want string
	}{
		{"multi-segment arn", "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster", "my-cluster"},
		{"no slash", "my-cluster", "my-cluster"},
		{"empty string", "", ""},
		{"trailing slash", "arn:aws:ecs:us-east-1:123456789012:cluster/", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arnSuffix(tt.arn); got != tt.want {
				t.Errorf("arnSuffix(%q) = %q, want %q", tt.arn, got, tt.want)
			}
		})
	}
}

func TestServiceNameFromGroup(t *testing.T) {
	tests := []struct {
		name  string
		group string
		want  string
	}{
		{"service prefix", "service:my-service", "my-service"},
		{"standalone task, no service prefix", "family:my-task", "family:my-task"},
		{"empty group", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serviceNameFromGroup(tt.group); got != tt.want {
				t.Errorf("serviceNameFromGroup(%q) = %q, want %q", tt.group, got, tt.want)
			}
		})
	}
}
