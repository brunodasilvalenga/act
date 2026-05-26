package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/brunodasilvalenga/act/internal/aws"
)

type ecsModel struct {
	tasks        []aws.ECSTask
	filtered     []aws.ECSTask
	cursor       int
	search       string
	selected     *aws.ECSTask
	quitting     bool
	width        int
	height       int
	mode         mode
	spinnerFrame int
	loadFunc     func() ([]aws.ECSTask, error)
	err          error
}

type ecsTasksLoadedMsg struct {
	tasks []aws.ECSTask
	err   error
}

func initialECSModel(loadFunc func() ([]aws.ECSTask, error)) ecsModel {
	return ecsModel{
		mode:     modeLoading,
		loadFunc: loadFunc,
		width:    100,
		height:   24,
	}
}

func (m ecsModel) Init() tea.Cmd {
	return tea.Batch(m.loadTasks(), m.tickSpinner())
}

func (m ecsModel) loadTasks() tea.Cmd {
	return func() tea.Msg {
		tasks, err := m.loadFunc()
		return ecsTasksLoadedMsg{tasks: tasks, err: err}
	}
}

func (m ecsModel) tickSpinner() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(_ time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m ecsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case ecsTasksLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.quitting = true
			return m, tea.Quit
		}
		if len(msg.tasks) == 0 {
			m.err = fmt.Errorf("no running ECS tasks found")
			m.quitting = true
			return m, tea.Quit
		}
		m.tasks = msg.tasks
		m.filtered = msg.tasks
		m.mode = modeList
		return m, nil

	case spinnerTickMsg:
		if m.mode == modeLoading {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, m.tickSpinner()
		}
		return m, nil

	case tea.KeyMsg:
		if m.mode == modeLoading {
			if msg.Type == tea.KeyCtrlC || msg.Type == tea.KeyEsc {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
		return m.handleKeys(msg)
	}
	return m, nil
}

func (m ecsModel) handleKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
	return m, nil
}

func (m *ecsModel) applyFilter() {
	if m.search == "" {
		m.filtered = m.tasks
	} else {
		var filtered []aws.ECSTask
		lower := strings.ToLower(m.search)
		for _, task := range m.tasks {
			if strings.Contains(strings.ToLower(task.ServiceName), lower) ||
				strings.Contains(strings.ToLower(task.ContainerName), lower) ||
				strings.Contains(strings.ToLower(task.TaskID), lower) {
				filtered = append(filtered, task)
			}
		}
		m.filtered = filtered
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = max(0, len(m.filtered)-1)
	}
}

func (m ecsModel) View() string {
	if m.mode == modeLoading {
		var b strings.Builder
		b.WriteString(titleStyle.Render("AWS ECS Execute Command"))
		b.WriteString("\n")
		b.WriteString(dimStyle.Render("──────────────────────────────────────────────"))
		b.WriteString("\n\n")
		frame := spinnerFrames[m.spinnerFrame]
		b.WriteString(fmt.Sprintf("  %s Fetching ECS tasks...\n", frame))
		return b.String()
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render("AWS ECS Execute Command"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("──────────────────────────────────────────────"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(" Search: %s▌\n", searchStyle.Render(m.search)))

	countText := fmt.Sprintf(" %d of %d tasks", len(m.filtered), len(m.tasks))
	b.WriteString(countStyle.Render(countText))
	b.WriteString("\n\n")

	header := fmt.Sprintf("  %-30s %-30s %s", "SERVICE", "CONTAINER", "TASK ID")
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  " + strings.Repeat("─", 80)))
	b.WriteString("\n")

	maxVisible := m.height - 10
	if maxVisible < 5 {
		maxVisible = 5
	}
	if maxVisible > len(m.filtered) {
		maxVisible = len(m.filtered)
	}

	start := 0
	if m.cursor >= maxVisible {
		start = m.cursor - maxVisible + 1
	}
	end := start + maxVisible
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	for i := start; i < end; i++ {
		display := m.filtered[i].DisplayName()
		if i == m.cursor {
			b.WriteString(selectedStyle.Render("▸ " + display))
		} else {
			b.WriteString(normalStyle.Render("  " + display))
		}
		b.WriteString("\n")
	}

	if len(m.filtered) == 0 {
		b.WriteString(dimStyle.Render("  No matches found."))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(dimStyle.Render(" ↑/↓ navigate • type to search • enter to connect • esc to quit"))
	b.WriteString("\n")
	return b.String()
}

func RunECS(loadFunc func() ([]aws.ECSTask, error)) (*aws.ECSTask, error) {
	m := initialECSModel(loadFunc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	final := finalModel.(ecsModel)
	if final.err != nil {
		return nil, final.err
	}
	if final.quitting {
		return nil, nil
	}
	return final.selected, nil
}
