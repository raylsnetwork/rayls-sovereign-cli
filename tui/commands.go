package tui

// CommandCategory represents a group of related commands
type CommandCategory struct {
	Name     string
	Commands []Command
}

// Command represents an executable command
type Command struct {
	Name        string
	Args        string // Placeholder for arguments
	Description string
	NeedsInput  bool   // Whether command needs user input for args
}

// GetAllCommands returns all available commands organized by category
func GetAllCommands() []CommandCategory {
	return []CommandCategory{
		{
			Name: "Stack Management",
			Commands: []Command{
				{Name: "init", Args: "[--members N] [--monitoring]", Description: "Initialize new stack", NeedsInput: true},
				{Name: "start", Args: "[service...]", Description: "Start all or specific services", NeedsInput: false},
				{Name: "stop", Args: "[service...]", Description: "Stop all or specific services", NeedsInput: false},
				{Name: "down", Args: "[-v]", Description: "Remove stack (preserves volumes)", NeedsInput: false},
				{Name: "down -v", Args: "", Description: "Remove stack including volumes", NeedsInput: false},
				{Name: "ps", Args: "", Description: "List running services", NeedsInput: false},
				{Name: "ps -a", Args: "", Description: "List all services", NeedsInput: false},
			},
		},
		{
			Name: "Verification & Testing",
			Commands: []Command{
				{Name: "verify contracts", Args: "", Description: "Run E2E test suite", NeedsInput: false},
				{Name: "verify time", Args: "<participant>", Description: "Check blockchain time (a-f)", NeedsInput: true},
				{Name: "verify ls", Args: "-la", Description: "List contracts directory", NeedsInput: false},
				{Name: "verify custom", Args: "<command>", Description: "Run custom command in contracts", NeedsInput: true},
			},
		},
		{
			Name: "Monitoring & Logs",
			Commands: []Command{
				{Name: "logs", Args: "", Description: "View logs from all services", NeedsInput: false},
				{Name: "logs -f", Args: "[service...]", Description: "Follow logs (tail -f)", NeedsInput: true},
				{Name: "logs --tail", Args: "N [service]", Description: "Show last N lines", NeedsInput: true},
				{Name: "stats", Args: "", Description: "Show Docker daemon statistics", NeedsInput: false},
				{Name: "info", Args: "", Description: "Show system information", NeedsInput: false},
			},
		},
		{
			Name: "Updates",
			Commands: []Command{
				{Name: "version", Args: "", Description: "Show CLI version", NeedsInput: false},
				{Name: "version --check", Args: "", Description: "Check for updates", NeedsInput: false},
				{Name: "update check", Args: "", Description: "Check for updates", NeedsInput: false},
			},
		},
	}
}

// GetVerifyCommands returns just the verify commands for quick access
func GetVerifyCommands() []Command {
	for _, category := range GetAllCommands() {
		if category.Name == "Verification & Testing" {
			return category.Commands
		}
	}
	return []Command{}
}
