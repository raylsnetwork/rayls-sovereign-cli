package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ServiceInfo represents a Docker service
type ServiceInfo struct {
	Name    string
	Status  string // Up, Down, Exited, etc.
	Health  string // Healthy, Unhealthy, Starting, etc.
	Uptime  string
	Ports   string
}

// TUIModel is the main model for the TUI
type TUIModel struct {
	// Current view
	view ViewMode

	// Services view
	services       []ServiceInfo
	servicesList   list.Model
	servicesLoaded bool

	// Commands view
	commandCategories []CommandCategory
	commandCursor     int
	categoryExpanded  map[int]bool
	commandInput      textinput.Model

	// Command execution
	executingCommand bool
	commandOutput    []string
	outputViewport   viewport.Model

	// Logs view
	logsViewport viewport.Model
	logLines     []string
	followLogs   bool

	// Endpoints view
	endpoints []Endpoint

	// UI state
	spinner      spinner.Model
	width        int
	height       int
	showingHelp  bool
	statusMsg    string
	errorMsg     string
	lastUpdate   time.Time

	// Flags
	ready bool
}

// Endpoint represents a service endpoint
type Endpoint struct {
	Name string
	URL  string
	Type string // Shared, Participant
}

// NewTUIModel creates a new TUI model
func NewTUIModel() TUIModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	ti := textinput.New()
	ti.Placeholder = "Enter arguments..."
	ti.CharLimit = 100
	ti.Width = 50

	return TUIModel{
		view:              ViewServices,
		spinner:           s,
		commandCategories: GetAllCommands(),
		categoryExpanded:  make(map[int]bool),
		commandInput:      ti,
		lastUpdate:        time.Now(),
	}
}

// Init initializes the TUI
func (m TUIModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		fetchServicesCmd(),
	)
}

// Messages for async operations
type servicesMsg struct {
	services []ServiceInfo
	err      error
}

type commandOutputMsg struct {
	output []string
	err    error
}

type tickMsg time.Time

// Commands for async operations
func fetchServicesCmd() tea.Cmd {
	return func() tea.Msg {
		// TODO: Actually fetch from docker compose ps
		services := []ServiceInfo{
			{Name: "privacy-node-a", Status: "Up", Health: "Healthy", Uptime: "5m", Ports: "8545:8545"},
			{Name: "privacy-node-b", Status: "Up", Health: "Healthy", Uptime: "5m", Ports: "8546:8545"},
			{Name: "kos-a", Status: "Up", Health: "Healthy", Uptime: "4m", Ports: "8080:8080"},
			{Name: "kos-b", Status: "Up", Health: "Starting", Uptime: "2m", Ports: "8081:8080"},
			{Name: "relayer-a", Status: "Up", Health: "Healthy", Uptime: "3m", Ports: "9000:9000"},
			{Name: "nats", Status: "Up", Health: "Healthy", Uptime: "7m", Ports: "4222:4222"},
			{Name: "postgres", Status: "Up", Health: "Healthy", Uptime: "7m", Ports: "5432:5432"},
			{Name: "contracts", Status: "Exited", Health: "Complete", Uptime: "-", Ports: "-"},
		}
		return servicesMsg{services: services, err: nil}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(time.Second*2, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func executeCommandCmd(command string) tea.Cmd {
	return func() tea.Msg {
		// TODO: Actually execute the command
		output := []string{
			fmt.Sprintf("$ rayls %s", command),
			"",
			"Executing command...",
			"Output will appear here",
		}
		return commandOutputMsg{output: output, err: nil}
	}
}

// Update handles messages
func (m TUIModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true

		// Initialize viewport for output
		if !m.outputViewport.HighPerformanceRendering {
			m.outputViewport = viewport.New(msg.Width-4, msg.Height-10)
			m.outputViewport.HighPerformanceRendering = true
		}

		return m, nil

	case tea.KeyMsg:
		// Global keys
		switch msg.String() {
		case "ctrl+c", "q":
			if !m.commandInput.Focused() {
				return m, tea.Quit
			}
		case "?":
			m.view = ViewHelp
			return m, nil
		case "1":
			m.view = ViewServices
			return m, fetchServicesCmd()
		case "2":
			m.view = ViewCommands
			return m, nil
		case "3":
			m.view = ViewLogs
			return m, nil
		case "4":
			m.view = ViewEndpoints
			return m, nil
		}

		// View-specific keys
		switch m.view {
		case ViewCommands:
			return m.updateCommandsView(msg)
		case ViewServices:
			return m.updateServicesView(msg)
		}

	case servicesMsg:
		m.services = msg.services
		m.servicesLoaded = true
		m.statusMsg = fmt.Sprintf("Loaded %d services", len(msg.services))
		return m, tickCmd()

	case commandOutputMsg:
		m.commandOutput = msg.output
		m.outputViewport.SetContent(strings.Join(msg.output, "\n"))
		m.executingCommand = false
		return m, nil

	case tickMsg:
		m.lastUpdate = time.Time(msg)
		return m, tea.Batch(fetchServicesCmd(), tickCmd())

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, tea.Batch(cmds...)
}

func (m TUIModel) updateCommandsView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.commandInput.Focused() {
		switch msg.String() {
		case "enter":
			// Execute command with input
			m.commandInput.Blur()
			cmd := m.getCurrentCommand()
			if cmd != nil {
				fullCommand := cmd.Name + " " + m.commandInput.Value()
				m.executingCommand = true
				m.commandInput.SetValue("")
				return m, executeCommandCmd(fullCommand)
			}
		case "esc":
			m.commandInput.Blur()
			m.commandInput.SetValue("")
			return m, nil
		default:
			var cmd tea.Cmd
			m.commandInput, cmd = m.commandInput.Update(msg)
			return m, cmd
		}
	}

	switch msg.String() {
	case "up", "k":
		if m.commandCursor > 0 {
			m.commandCursor--
		}
	case "down", "j":
		maxCursor := m.getTotalCommandCount() - 1
		if m.commandCursor < maxCursor {
			m.commandCursor++
		}
	case "enter":
		cmd := m.getCurrentCommand()
		if cmd != nil {
			if cmd.NeedsInput {
				m.commandInput.Focus()
				return m, textinput.Blink
			} else {
				m.executingCommand = true
				return m, executeCommandCmd(cmd.Name)
			}
		}
	case " ":
		// Toggle category expansion (if on category header)
		categoryIdx := m.getCategoryAtCursor()
		if categoryIdx >= 0 {
			m.categoryExpanded[categoryIdx] = !m.categoryExpanded[categoryIdx]
		}
	}

	return m, nil
}

func (m TUIModel) updateServicesView(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// TODO: Handle service selection and actions
	return m, nil
}

func (m TUIModel) getTotalCommandCount() int {
	count := 0
	for range m.commandCategories {
		count++ // Category header
		count += len(m.commandCategories[0].Commands)
	}
	return count
}

func (m TUIModel) getCurrentCommand() *Command {
	cursor := 0
	for catIdx, category := range m.commandCategories {
		if cursor == m.commandCursor {
			return nil // On category header
		}
		cursor++

		if m.categoryExpanded[catIdx] || len(m.categoryExpanded) == 0 {
			for cmdIdx := range category.Commands {
				if cursor == m.commandCursor {
					return &category.Commands[cmdIdx]
				}
				cursor++
			}
		}
	}
	return nil
}

func (m TUIModel) getCategoryAtCursor() int {
	cursor := 0
	for catIdx := range m.commandCategories {
		if cursor == m.commandCursor {
			return catIdx
		}
		cursor++
		cursor += len(m.commandCategories[catIdx].Commands)
	}
	return -1
}

// View renders the TUI
func (m TUIModel) View() string {
	if !m.ready {
		return "\n  Initializing..."
	}

	var content string

	switch m.view {
	case ViewServices:
		content = m.renderServicesView()
	case ViewCommands:
		content = m.renderCommandsView()
	case ViewLogs:
		content = m.renderLogsView()
	case ViewEndpoints:
		content = m.renderEndpointsView()
	case ViewHelp:
		content = m.renderHelpView()
	}

	// Render with header and footer
	return m.renderFrame(content)
}
