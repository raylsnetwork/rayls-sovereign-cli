package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

var (
	follow     bool
	tail       string
	timestamps bool
)

// logsCmd represents the logs command
var logsCmd = &cobra.Command{
	Use:   "logs [SERVICE...]",
	Short: "View log output from services",
	Long: `View log output from one or more services in the Rayls stack.

Examples:
  rayls logs                    # Show logs from all services
  rayls logs kos-a              # Show logs from kos-a service
  rayls logs kos-a pubrelayer-a    # Show logs from multiple services
  rayls logs -f kos-a           # Follow log output
  rayls logs --tail=100 kos-a   # Show last 100 lines`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Docker configuration
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}

		// Build docker compose logs command
		logsArgs := []string{"logs"}

		if follow {
			logsArgs = append(logsArgs, "-f")
		}

		if tail != "" {
			logsArgs = append(logsArgs, "--tail="+tail)
		}

		if timestamps {
			logsArgs = append(logsArgs, "-t")
		}

		// Add service names if specified
		logsArgs = append(logsArgs, args...)

		var composeCmd *exec.Cmd
		if version == docker.ComposeV2 {
			composeCmd = exec.Command("docker", append([]string{"compose"}, logsArgs...)...)
		} else {
			composeCmd = exec.Command("docker-compose", logsArgs...)
		}

		// Set up command to stream output
		composeCmd.Stdout = os.Stdout
		composeCmd.Stderr = os.Stderr
		composeCmd.Stdin = os.Stdin

		// Handle Ctrl+C gracefully when following logs
		if follow {
			sigChan := make(chan os.Signal, 1)
			signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

			go func() {
				<-sigChan
				if composeCmd.Process != nil {
					composeCmd.Process.Signal(os.Interrupt)
				}
			}()
		}

		// Execute the command
		err = composeCmd.Run()
		if err != nil {
			// Check if it's because docker-compose.yml doesn't exist
			if strings.Contains(err.Error(), "no configuration file provided") ||
				strings.Contains(err.Error(), "Can't find") {
				return fmt.Errorf("no docker-compose.yml found. Run 'rayls init' first")
			}
			return fmt.Errorf("failed to get logs: %w", err)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(logsCmd)
	logsCmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	logsCmd.Flags().StringVar(&tail, "tail", "all", "Number of lines to show from the end of the logs")
	logsCmd.Flags().BoolVarP(&timestamps, "timestamps", "t", false, "Show timestamps")
}
