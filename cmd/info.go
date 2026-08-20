package cmd

import (
	"fmt"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info",
	Short: "Display system and environment information",
	//ValidArgsFunction: listStacks,
	Long: `Display information about the current environment configuration and Docker setup.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ctx := log.WithVerbosity(context.Background(), verbose)
		// ctx = context.WithValue(ctx, docker.CtxIsLogCmdKey{}, true)
		// ctx = log.WithLogger(ctx, logger)
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}
		fmt.Printf("Docker Compose version detected: %s\n", version)
		// ctx = context.WithValue(ctx, docker.CtxComposeVersionKey{}, version)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(infoCmd)
}

// initCmd represents the init command
// var initCmd = &cobra.Command{
// 	Use:   "init",
// 	Short: "Initialize a new environment",
// 	Long:  `Initialize a new environment, either for Demo or Dev.`,
// 	Run: func(cmd *cobra.Command, args []string) {
// 		if err := tui.StartUI(); err != nil {
// 			fmt.Printf("Error: %v\n", err)
// 			os.Exit(1)
// 		}
// 	},
// }
