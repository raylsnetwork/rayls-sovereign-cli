package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			MarginLeft(2)

	tabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240"))

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("205")).
			Bold(true)

	headerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("255")).
			Padding(0, 1).
			Bold(true)

	footerStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("235")).
			Foreground(lipgloss.Color("248")).
			Padding(0, 1)

	serviceHealthyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("42"))

	serviceStartingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("220"))

	serviceDownStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("240"))

	serviceUnhealthyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("196"))

	commandCategoryStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("39")).
				MarginTop(1)

	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("255")).
			MarginLeft(2)

	commandSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("205")).
				Bold(true).
				MarginLeft(2)

	commandDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("242")).
				MarginLeft(1)

	outputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1).
			MarginTop(1)
)

func (m TUIModel) renderFrame(content string) string {
	header := m.renderHeader()
	footer := m.renderFooter()

	return fmt.Sprintf("%s\n%s\n%s", header, content, footer)
}

func (m TUIModel) renderHeader() string {
	// Tabs
	tabs := []string{}
	views := []ViewMode{ViewServices, ViewCommands, ViewLogs, ViewEndpoints}

	for _, v := range views {
		style := tabStyle
		if v == m.view {
			style = activeTabStyle
		}
		tab := style.Render(fmt.Sprintf("[%s] %s", v.ShortKey(), v.String()))
		tabs = append(tabs, tab)
	}

	tabsRow := lipgloss.JoinHorizontal(lipgloss.Top, tabs...)

	// Status
	status := ""
	if m.errorMsg != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("● " + m.errorMsg)
	} else if m.statusMsg != "" {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render("● " + m.statusMsg)
	} else {
		status = lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("● Ready")
	}

	// Version info
	versionInfo := lipgloss.NewStyle().Foreground(lipgloss.Color("242")).Render("v0.0.1")

	// Combine header components
	headerLeft := tabsRow
	headerRight := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", versionInfo)

	// Calculate spacing
	spacingWidth := m.width - lipgloss.Width(headerLeft) - lipgloss.Width(headerRight) - 4
	if spacingWidth < 0 {
		spacingWidth = 0
	}
	spacing := strings.Repeat(" ", spacingWidth)

	return headerStyle.Render(headerLeft + spacing + headerRight)
}

func (m TUIModel) renderFooter() string {
	helpText := ""
	switch m.view {
	case ViewServices:
		helpText = "[↑↓] navigate  [enter] details  [r] restart  [s] stop  [l] logs  [q] quit  [?] help"
	case ViewCommands:
		if m.commandInput.Focused() {
			helpText = "[enter] execute  [esc] cancel  [q] quit"
		} else {
			helpText = "[↑↓ j/k] navigate  [enter] execute  [space] expand/collapse  [q] quit  [?] help"
		}
	case ViewLogs:
		helpText = "[↑↓] scroll  [f] toggle follow  [c] clear  [q] quit  [?] help"
	case ViewEndpoints:
		helpText = "[↑↓] navigate  [enter] copy  [q] quit  [?] help"
	case ViewHelp:
		helpText = "[any key] back to previous view"
	}

	return footerStyle.Width(m.width - 2).Render(helpText)
}

func (m TUIModel) renderServicesView() string {
	if !m.servicesLoaded {
		return fmt.Sprintf("\n  %s Loading services...\n", m.spinner.View())
	}

	var b strings.Builder

	b.WriteString(titleStyle.Render(fmt.Sprintf("Services (%d)", len(m.services))))
	b.WriteString("\n\n")

	// Table header
	headerRow := fmt.Sprintf("  %-25s %-12s %-12s %-10s %s",
		"NAME", "STATUS", "HEALTH", "UPTIME", "PORTS")
	b.WriteString(headerStyle.Render(headerRow))
	b.WriteString("\n")

	// Service rows
	for _, svc := range m.services {
		statusIcon := getStatusIcon(svc.Status, svc.Health)
		healthStyle := getHealthStyle(svc.Health)

		row := fmt.Sprintf("  %s %-23s %-12s %-12s %-10s %s",
			statusIcon,
			svc.Name,
			svc.Status,
			healthStyle.Render(svc.Health),
			svc.Uptime,
			svc.Ports,
		)
		b.WriteString(row)
		b.WriteString("\n")
	}

	return b.String()
}

func (m TUIModel) renderCommandsView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Commands - Select and Execute"))
	b.WriteString("\n")

	if m.executingCommand {
		b.WriteString(fmt.Sprintf("\n  %s Executing command...\n\n", m.spinner.View()))
	}

	// Render command output if available
	if len(m.commandOutput) > 0 {
		b.WriteString(outputStyle.Width(m.width - 4).Render(strings.Join(m.commandOutput, "\n")))
		b.WriteString("\n")
		return b.String()
	}

	// Render command input if active
	if m.commandInput.Focused() {
		b.WriteString("\n")
		b.WriteString(commandStyle.Render("Enter arguments:"))
		b.WriteString("\n  ")
		b.WriteString(m.commandInput.View())
		b.WriteString("\n")
	}

	cursor := 0
	for catIdx, category := range m.commandCategories {
		// Category header
		categoryHeader := "▸ " + category.Name
		if m.categoryExpanded[catIdx] {
			categoryHeader = "▾ " + category.Name
		}

		style := commandCategoryStyle
		if cursor == m.commandCursor {
			style = style.Background(lipgloss.Color("240"))
		}
		b.WriteString(style.Render(categoryHeader))
		b.WriteString("\n")
		cursor++

		// Commands in category (show if expanded or if no expansion state set)
		if m.categoryExpanded[catIdx] || len(m.categoryExpanded) == 0 {
			for _, cmd := range category.Commands {
				style := commandStyle
				pointer := "  "
				if cursor == m.commandCursor {
					style = commandSelectedStyle
					pointer = "> "
				}

				cmdText := fmt.Sprintf("%s• %s", pointer, cmd.Name)
				if cmd.Args != "" {
					cmdText += " " + cmd.Args
				}

				b.WriteString(style.Render(cmdText))
				b.WriteString(commandDescStyle.Render(" - " + cmd.Description))
				b.WriteString("\n")
				cursor++
			}
		}
	}

	return b.String()
}

func (m TUIModel) renderLogsView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Logs"))
	b.WriteString("\n")

	followStatus := ""
	if m.followLogs {
		followStatus = " [Following]"
	}
	b.WriteString(commandDescStyle.Render("Streaming logs from all services" + followStatus))
	b.WriteString("\n\n")

	// TODO: Render actual logs
	b.WriteString(commandDescStyle.Render("Log output will appear here..."))
	b.WriteString("\n")

	return b.String()
}

func (m TUIModel) renderEndpointsView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Service Endpoints"))
	b.WriteString("\n\n")

	// Shared services
	b.WriteString(commandCategoryStyle.Render("Shared Services"))
	b.WriteString("\n")
	sharedEndpoints := []Endpoint{
		{Name: "Block Explorer", URL: "http://localhost:8181", Type: "Shared"},
		{Name: "Private Network Hub", URL: "http://localhost:3445", Type: "Shared"},
		{Name: "Governance API", URL: "http://localhost:9100", Type: "Shared"},
		{Name: "Grafana", URL: "http://localhost:3300", Type: "Shared"},
	}
	for _, ep := range sharedEndpoints {
		b.WriteString(commandStyle.Render(fmt.Sprintf("• %-25s %s", ep.Name+":", ep.URL)))
		b.WriteString("\n")
	}

	// Participant services
	b.WriteString("\n")
	b.WriteString(commandCategoryStyle.Render("Participant Services"))
	b.WriteString("\n")
	participantEndpoints := []Endpoint{
		{Name: "Privacy Node A", URL: "http://localhost:8545", Type: "Participant"},
		{Name: "Privacy Node B", URL: "http://localhost:8546", Type: "Participant"},
		{Name: "KOS A", URL: "http://localhost:8080", Type: "Participant"},
		{Name: "KOS B", URL: "http://localhost:8081", Type: "Participant"},
		{Name: "Relayer A", URL: "http://localhost:9000", Type: "Participant"},
		{Name: "Relayer B", URL: "http://localhost:9001", Type: "Participant"},
	}
	for _, ep := range participantEndpoints {
		b.WriteString(commandStyle.Render(fmt.Sprintf("• %-25s %s", ep.Name+":", ep.URL)))
		b.WriteString("\n")
	}

	return b.String()
}

func (m TUIModel) renderHelpView() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Keyboard Shortcuts"))
	b.WriteString("\n\n")

	helpSections := []struct {
		title string
		items [][]string
	}{
		{
			title: "Global",
			items: [][]string{
				{"1", "Switch to Services view"},
				{"2", "Switch to Commands view"},
				{"3", "Switch to Logs view"},
				{"4", "Switch to Endpoints view"},
				{"?", "Show this help screen"},
				{"q", "Quit (from any view)"},
			},
		},
		{
			title: "Navigation",
			items: [][]string{
				{"↑↓ or j/k", "Move cursor up/down"},
				{"enter", "Select/Execute"},
				{"space", "Expand/Collapse (where applicable)"},
				{"tab", "Switch panel (where applicable)"},
			},
		},
		{
			title: "Commands View",
			items: [][]string{
				{"enter", "Execute selected command"},
				{"space", "Expand/Collapse category"},
				{"esc", "Cancel input (if entering args)"},
			},
		},
	}

	for _, section := range helpSections {
		b.WriteString(commandCategoryStyle.Render(section.title))
		b.WriteString("\n")
		for _, item := range section.items {
			b.WriteString(commandStyle.Render(fmt.Sprintf("%-15s %s", item[0], item[1])))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return b.String()
}

// Helper functions
func getStatusIcon(status, health string) string {
	switch health {
	case "Healthy":
		return serviceHealthyStyle.Render("●")
	case "Starting":
		return serviceStartingStyle.Render("⚡")
	case "Unhealthy":
		return serviceUnhealthyStyle.Render("✗")
	default:
		if status == "Up" {
			return serviceHealthyStyle.Render("●")
		}
		return serviceDownStyle.Render("○")
	}
}

func getHealthStyle(health string) lipgloss.Style {
	switch health {
	case "Healthy":
		return serviceHealthyStyle
	case "Starting":
		return serviceStartingStyle
	case "Unhealthy":
		return serviceUnhealthyStyle
	default:
		return serviceDownStyle
	}
}
