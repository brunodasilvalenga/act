package tui

import (
	"testing"

	"github.com/brunodasilvalenga/act/internal/aws"
)

func TestApplyFilter(t *testing.T) {
	instances := []aws.Instance{
		{Name: "web-server-prod", InstanceID: "i-abc123", PrivateIP: "10.0.1.1", InstanceType: "t3.micro"},
		{Name: "api-server-prod", InstanceID: "i-def456", PrivateIP: "10.0.1.2", InstanceType: "t3.small"},
		{Name: "db-server-staging", InstanceID: "i-ghi789", PrivateIP: "10.0.2.1", InstanceType: "r5.large"},
	}

	tests := []struct {
		name     string
		search   string
		expected int
	}{
		{"empty filter returns all", "", 3},
		{"filter by name", "web", 1},
		{"filter by instance ID", "def456", 1},
		{"filter by IP", "10.0.2", 1},
		{"filter case insensitive", "WEB", 1},
		{"filter no match", "nonexistent", 0},
		{"filter partial name", "prod", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &model{
				instances: instances,
				filtered:  instances,
				search:    tt.search,
			}
			m.applyFilter()

			if len(m.filtered) != tt.expected {
				t.Errorf("applyFilter(%q) returned %d results, want %d", tt.search, len(m.filtered), tt.expected)
			}
		})
	}
}

func TestApplyFilterCursorReset(t *testing.T) {
	instances := []aws.Instance{
		{Name: "a", InstanceID: "i-1", PrivateIP: "10.0.0.1", InstanceType: "t3.micro"},
		{Name: "b", InstanceID: "i-2", PrivateIP: "10.0.0.2", InstanceType: "t3.micro"},
		{Name: "c", InstanceID: "i-3", PrivateIP: "10.0.0.3", InstanceType: "t3.micro"},
	}

	m := &model{
		instances: instances,
		filtered:  instances,
		cursor:    2,
		search:    "a",
	}
	m.applyFilter()

	if m.cursor != 0 {
		t.Errorf("cursor should reset to 0 when filtered list is smaller, got %d", m.cursor)
	}
}
