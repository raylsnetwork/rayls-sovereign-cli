package cmd

import (
	"fmt"
	"os"
	"github.com/raylsnetwork/rayls-sovereign-cli/tui"

	"github.com/spf13/cobra"
)

// tuiCmd represents the tui command
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Launch interactive Terminal UI",
	Long: `Launch an interactive Terminal User Interface (TUI) for managing the Rayls stack.

The TUI provides:
  • Real-time service monitoring
  • Easy command discovery and execution
  • Log streaming
  • Endpoint management

Navigate with arrow keys or vim bindings (j/k). Press ? for help.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := tui.Start(); err != nil {
			fmt.Printf("Error starting TUI: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	// Temporarily hidden - TUI not ready for users yet
	// rootCmd.AddCommand(tuiCmd)
}
