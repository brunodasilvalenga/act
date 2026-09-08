package tui

import (
	"testing"

	"github.com/brunodasilvalenga/act/internal/aws"
)

func TestECSApplyFilter(t *testing.T) {
	tasks := []aws.ECSTask{
		{ServiceName: "web-service-prod", ContainerName: "web", TaskID: "abc123"},
		{ServiceName: "api-service-prod", ContainerName: "api", TaskID: "def456"},
		{ServiceName: "db-service-staging", ContainerName: "db", TaskID: "ghi789"},
	}

	tests := []struct {
		name     string
		search   string
		expected int
	}{
		{"empty filter returns all", "", 3},
		{"filter by service name", "web", 1},
		{"filter by container name", "api", 1},
		{"filter by task ID", "ghi789", 1},
		{"filter case insensitive", "WEB", 1},
		{"filter no match", "nonexistent", 0},
		{"filter partial service name", "prod", 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &ecsModel{tasks: tasks, filtered: tasks, search: tt.search}
			m.applyFilter()
			if len(m.filtered) != tt.expected {
				t.Errorf("applyFilter(%q) returned %d results, want %d", tt.search, len(m.filtered), tt.expected)
			}
		})
	}
}

func TestECSApplyFilterCursorReset(t *testing.T) {
	tasks := []aws.ECSTask{
		{ServiceName: "a", ContainerName: "a", TaskID: "1"},
		{ServiceName: "b", ContainerName: "b", TaskID: "2"},
		{ServiceName: "c", ContainerName: "c", TaskID: "3"},
	}

	m := &ecsModel{tasks: tasks, filtered: tasks, cursor: 2, search: "a"}
	m.applyFilter()

	if m.cursor != 0 {
		t.Errorf("cursor should reset to 0 when filtered list is smaller, got %d", m.cursor)
	}
}
