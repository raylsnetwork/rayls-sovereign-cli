package cmd

import (
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"

	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Get docker stats",
	//ValidArgsFunction: listStacks,
	Long: `Get docker stats.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ctx := log.WithVerbosity(context.Background(), verbose)
		// ctx = context.WithValue(ctx, docker.CtxIsLogCmdKey{}, true)
		// ctx = log.WithLogger(ctx, logger)
		return docker.DockerStats()
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
