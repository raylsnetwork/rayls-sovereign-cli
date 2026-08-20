package cmd

import (
	"os"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/stacks"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Hot reload: sync saves into dev-mode components (docker compose watch)",
	Long: `Runs ` + "`docker compose watch`" + ` in the foreground. File saves in the local
checkouts of components enabled via ` + "`rayls dev`" + ` are synced into their
containers, where air rebuilds and restarts the service — typically a few
seconds per change. Stack state (chain data, Postgres, NATS) survives restarts.

Press Ctrl-C to stop watching; the stack keeps running.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := stacks.Watch(); err != nil {
			color.Red("Error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}
