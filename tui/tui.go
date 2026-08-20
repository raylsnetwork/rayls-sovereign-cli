package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// Start launches the TUI
func Start() error {
	m := NewTUIModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return err
	}

	return nil
}
