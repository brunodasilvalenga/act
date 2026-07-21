package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerModelUpdate(t *testing.T) {
	t.Run("down arrow moves cursor forward", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b", "c"}}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 := newModel.(pickerModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
	})

	t.Run("down arrow clamps at end", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b", "c"}, cursor: 2}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m2 := newModel.(pickerModel)
		if m2.cursor != 2 {
			t.Errorf("cursor = %d, want 2", m2.cursor)
		}
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
	})

	t.Run("up arrow moves cursor backward", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b", "c"}, cursor: 2}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m2 := newModel.(pickerModel)
		if m2.cursor != 1 {
			t.Errorf("cursor = %d, want 1", m2.cursor)
		}
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
	})

	t.Run("up arrow clamps at zero", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b", "c"}, cursor: 0}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyUp})
		m2 := newModel.(pickerModel)
		if m2.cursor != 0 {
			t.Errorf("cursor = %d, want 0", m2.cursor)
		}
		if cmd != nil {
			t.Errorf("cmd = %v, want nil", cmd)
		}
	})

	t.Run("enter selects current item and quits", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b", "c"}, cursor: 1}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := newModel.(pickerModel)
		if m2.selected != "b" {
			t.Errorf("selected = %q, want %q", m2.selected, "b")
		}
		if cmd == nil {
			t.Fatal("expected a non-nil quit command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected cmd() to produce tea.QuitMsg, got %T", cmd())
		}
	})

	t.Run("enter on empty items does not panic and does not select", func(t *testing.T) {
		m := pickerModel{items: []string{}}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m2 := newModel.(pickerModel)
		if m2.selected != "" {
			t.Errorf("selected = %q, want empty", m2.selected)
		}
		if cmd == nil {
			t.Fatal("expected a non-nil quit command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected cmd() to produce tea.QuitMsg, got %T", cmd())
		}
	})

	t.Run("esc quits without selecting", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b"}, cursor: 0}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		m2 := newModel.(pickerModel)
		if !m2.quitting {
			t.Error("quitting = false, want true")
		}
		if m2.selected != "" {
			t.Errorf("selected = %q, want empty", m2.selected)
		}
		if cmd == nil {
			t.Fatal("expected a non-nil quit command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected cmd() to produce tea.QuitMsg, got %T", cmd())
		}
	})

	t.Run("ctrl+c quits without selecting", func(t *testing.T) {
		m := pickerModel{items: []string{"a", "b"}, cursor: 0}
		newModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		m2 := newModel.(pickerModel)
		if !m2.quitting {
			t.Error("quitting = false, want true")
		}
		if m2.selected != "" {
			t.Errorf("selected = %q, want empty", m2.selected)
		}
		if cmd == nil {
			t.Fatal("expected a non-nil quit command")
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("expected cmd() to produce tea.QuitMsg, got %T", cmd())
		}
	})
}
