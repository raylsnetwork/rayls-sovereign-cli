package tui

import (
	"fmt"
	"os"
	"os/exec"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type model struct {
	choices    []string
	cursor     int
	status     string
	spinner    spinner.Model
	err        error // New field to store error
	dockerInfo string
}

type commandFinishedMsg struct{ err error }

type dockerInfoMsg struct {
	version string
	err     error
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return model{
		choices: []string{"Demo", "Dev", "Info"},
		status:  "choosing",
		spinner: s,
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "up", "k":
			if m.status == "choosing" && m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.status == "choosing" && m.cursor < len(m.choices)-1 {
				m.cursor++
			}
		case "enter":
			if m.status == "choosing" {
				choice := m.choices[m.cursor]
				if choice == "Demo" {
					m.status = "running"
					return m, runDemoCmd()
				} else if choice == "Dev" {
					// for now, just quit
					return m, tea.Quit
				} else if choice == "Info" {
					m.status = "running" // Set status to running before getting info
					return m, getDockerInfoCmd()
				}
			} else if m.status == "info-result" || m.err != nil {
				m.status = "choosing"
				m.dockerInfo = ""
				m.err = nil
				return m, nil
			}
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		if m.status == "running" {
			m.spinner, cmd = m.spinner.Update(msg)
		}
		return m, cmd

	case commandFinishedMsg:
		m.status = "done"
		m.err = msg.err // Store the error
		return m, tea.Quit
	case dockerInfoMsg:
		m.status = "info-result"
		m.err = msg.err
		m.dockerInfo = msg.version
		return m, nil
	}
	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		// Clear dockerInfo on error to not show stale info
		m.dockerInfo = ""
		return fmt.Sprintf("Error: %v\n\nPress enter to return to the menu.", m.err)
	}
	if m.status == "running" {
		return fmt.Sprintf("%s ...", m.spinner.View())
	}
	if m.status == "done" {
		return "Done!\n"
	}
	if m.status == "info-result" {
		if m.dockerInfo != "" {
			return fmt.Sprintf("Docker Compose version detected: %s\n\nPress enter to return to the menu.", m.dockerInfo)
		}
		// This case should ideally not be reached if there is no error and no info, but as a fallback:
		return "Could not retrieve Docker info.\n\nPress enter to return to the menu."
	}
	s := "What type of environment do you want to set up?\n\n"
	for i, choice := range m.choices {
		cursor := " "
		if m.cursor == i {
			cursor = ">"
		}
		s += fmt.Sprintf("%s %s\n", cursor, choice)
	}
	s += "\nPress q to quit.\n"
	return s
}

func StartUI() error {
	p := tea.NewProgram(initialModel())
	return p.Start()
}

func generateDockerCompose() error {
	content := `version: '3'
services:
  web:
    image: nginx
    ports:
      - "8080:80"
`
	return os.WriteFile("docker-compose.yml", []byte(content), 0644)
}

func runDemoCmd() tea.Cmd {
	return func() tea.Msg {
		err := generateDockerCompose()
		if err != nil {
			return commandFinishedMsg{err}
		}
		cmd := exec.Command("docker-compose", "up", "-d")
		err = cmd.Run()
		return commandFinishedMsg{err}
	}
}

func getDockerInfoCmd() tea.Cmd {
	return func() tea.Msg {
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return dockerInfoMsg{err: err}
		}
		return dockerInfoMsg{version: version.String()}
	}
}
