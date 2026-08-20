package cmd

import (
	"fmt"
	"os/exec"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"strings"

	"github.com/spf13/cobra"
)

var (
	stopTimeout int
)

// stopCmd represents the stop command
var stopCmd = &cobra.Command{
	Use:   "stop [SERVICE...]",
	Short: "Stop services",
	Long: `Stop running containers without removing them.

Examples:
  rayls stop                  # Stop all services
  rayls stop kos-a            # Stop specific service
  rayls stop kos-a pubrelayer-a  # Stop multiple services
  rayls stop -t 30            # Stop with custom timeout`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Docker configuration
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}

		// Build docker compose stop command
		stopArgs := []string{"stop"}

		if stopTimeout > 0 {
			stopArgs = append(stopArgs, "-t", fmt.Sprintf("%d", stopTimeout))
		}

		// Add service names if specified
		stopArgs = append(stopArgs, args...)

		var composeCmd *exec.Cmd
		if version == docker.ComposeV2 {
			composeCmd = exec.Command("docker", append([]string{"compose"}, stopArgs...)...)
		} else {
			composeCmd = exec.Command("docker-compose", stopArgs...)
		}

		fmt.Println("Stopping services...")

		// Execute the command
		output, err := composeCmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "no configuration file provided") ||
				strings.Contains(string(output), "Can't find") {
				return fmt.Errorf("no docker-compose.yml found. Run 'rayls init' first")
			}
			return fmt.Errorf("failed to stop services: %w\nOutput:\n%s", err, string(output))
		}

		fmt.Println(string(output))
		fmt.Println("Services stopped successfully")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
	stopCmd.Flags().IntVarP(&stopTimeout, "timeout", "t", 10, "Timeout in seconds to wait for stop")
}
