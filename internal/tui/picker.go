package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type pickerModel struct {
	title    string
	items    []string
	cursor   int
	selected string
	quitting bool
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			m.quitting = true
			return m, tea.Quit
		case tea.KeyEnter:
			if len(m.items) > 0 {
				m.selected = m.items[m.cursor]
			}
			return m, tea.Quit
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		}
	}
	return m, nil
}

func (m pickerModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.title))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("──────────────────────────────────────────────"))
	b.WriteString("\n\n")

	for i, item := range m.items {
		if i == m.cursor {
			b.WriteString(selectedStyle.Render(fmt.Sprintf("▸ %s", item)))
		} else {
			b.WriteString(normalStyle.Render(fmt.Sprintf("  %s", item)))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" ↑/↓ navigate • enter to select • esc to quit"))
	b.WriteString("\n")
	return b.String()
}

func RunPicker(title string, items []string) (string, error) {
	m := pickerModel{title: title, items: items}
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	final := finalModel.(pickerModel)
	if final.quitting {
		return "", nil
	}
	return final.selected, nil
}
