package cmd

import (
	"fmt"
	"os/exec"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var (
	psAll bool
)

// psCmd represents the ps command
var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List containers for the Rayls stack",
	Long:  `List all containers that are part of the Rayls stack, showing their status.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Docker configuration
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}

		// Build docker compose ps command
		var composeCmd *exec.Cmd
		psArgs := []string{"ps"}
		if psAll {
			psArgs = append(psArgs, "-a")
		}

		if version == docker.ComposeV2 {
			composeCmd = exec.Command("docker", append([]string{"compose"}, psArgs...)...)
		} else {
			composeCmd = exec.Command("docker-compose", psArgs...)
		}

		// Execute the command
		output, err := composeCmd.CombinedOutput()
		if err != nil {
			// If docker-compose.yml doesn't exist, provide helpful message
			if strings.Contains(string(output), "no configuration file provided") ||
				strings.Contains(string(output), "Can't find") {
				return fmt.Errorf("no docker-compose.yml found. Run 'rayls init' first")
			}
			return fmt.Errorf("failed to list containers: %w\nOutput:\n%s", err, string(output))
		}

		// Parse and format output
		outputStr := string(output)
		lines := strings.Split(outputStr, "\n")

		if len(lines) <= 1 {
			fmt.Println("No containers found")
			return nil
		}

		// Use tabwriter for formatted output
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 3, ' ', 0)
		defer w.Flush()

		// Print the output
		fmt.Fprintln(w, outputStr)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(psCmd)
	psCmd.Flags().BoolVarP(&psAll, "all", "a", false, "Show all containers (including stopped)")
}
