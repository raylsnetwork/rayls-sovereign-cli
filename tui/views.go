package tui

// ViewMode represents different TUI views
type ViewMode int

const (
	ViewServices ViewMode = iota
	ViewCommands
	ViewLogs
	ViewEndpoints
	ViewHelp
)

func (v ViewMode) String() string {
	switch v {
	case ViewServices:
		return "Services"
	case ViewCommands:
		return "Commands"
	case ViewLogs:
		return "Logs"
	case ViewEndpoints:
		return "Endpoints"
	case ViewHelp:
		return "Help"
	default:
		return "Unknown"
	}
}

func (v ViewMode) ShortKey() string {
	switch v {
	case ViewServices:
		return "1"
	case ViewCommands:
		return "2"
	case ViewLogs:
		return "3"
	case ViewEndpoints:
		return "4"
	case ViewHelp:
		return "?"
	default:
		return ""
	}
}
