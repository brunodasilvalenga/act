package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbletea"
	"github.com/brunodasilvalenga/act/internal/aws"
)

type model struct {
	instances []aws.Instance
	filtered  []aws.Instance
	cursor    int
	search    string
	selected  *aws.Instance
	quitting  bool
}

func initialModel(instances []aws.Instance) model {
	return model{
		instances: instances,
		filtered:  instances,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.selected = &m.filtered[m.cursor]
			}
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
			}
		case tea.KeyBackspace:
			if len(m.search) > 0 {
				m.search = m.search[:len(m.search)-1]
				m.applyFilter()
			}
		default:
			if msg.Type == tea.KeyRunes {
				m.search += string(msg.Runes)
				m.applyFilter()
			}
		}
	}
	return m, nil
}

func (m *model) applyFilter() {
	if m.search == "" {
		m.filtered = m.instances
	} else {
		var filtered []aws.Instance
		lower := strings.ToLower(m.search)
		for _, inst := range m.instances {
			if strings.Contains(strings.ToLower(inst.Name), lower) ||
				strings.Contains(strings.ToLower(inst.InstanceID), lower) ||
				strings.Contains(inst.PrivateIP, lower) {
				filtered = append(filtered, inst)
			}
		}
		m.filtered = filtered
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m model) View() string {
	var b strings.Builder

	b.WriteString("AWS EC2 Instance Connect (Session Manager)\n")
	b.WriteString("──────────────────────────────────────────────\n")
	b.WriteString(fmt.Sprintf(" Search: %s▌\n\n", m.search))

	header := fmt.Sprintf("  %-40s %-20s %-15s %s", "NAME", "INSTANCE ID", "PRIVATE IP", "TYPE")
	b.WriteString(header + "\n")
	b.WriteString("  " + strings.Repeat("─", 90) + "\n")

	visible := m.filtered
	maxVisible := 20
	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(visible) {
		end = len(visible)
	}

	for i := start; i < end; i++ {
		cursor := "  "
		if i == m.cursor {
			cursor = "▸ "
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, visible[i].DisplayName()))
	}

	if len(m.filtered) == 0 {
		b.WriteString("  No matches found.\n")
	}

	b.WriteString("\n ↑/↓ navigate • type to search • enter to connect • esc to quit\n")
	return b.String()
}

func Run(instances []aws.Instance) (*aws.Instance, error) {
	m := initialModel(instances)
	p := tea.NewProgram(m)
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := finalModel.(model)
	if final.quitting {
		return nil, nil
	}
	return final.selected, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
