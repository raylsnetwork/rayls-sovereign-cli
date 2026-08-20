package cmd

import (
	"fmt"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/stacks"
	"strings"

	"github.com/spf13/cobra"
)

var (
	noDeps bool
)

// startCmd represents the start command
var startCmd = &cobra.Command{
	Use:   "start [SERVICE...]",
	Short: "Start services",
	Long: `Start services defined in docker-compose.yml.

Runs 'docker compose up -d', which (re)creates and starts containers while
honoring service dependency and health-check ordering. Unlike 'docker compose
start', this brings up containers that were removed and starts dependencies in
the correct order, so services don't crash waiting on an unhealthy dependency.
Use 'rayls init' to generate a new stack from scratch.

Examples:
  rayls start                # Start all services
  rayls start kos-a          # Start specific service
  rayls start kos-a pubrelayer-a # Start multiple services`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Docker configuration
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}

		// Use `up -d` rather than `start`: `docker compose start` only starts
		// pre-existing containers and ignores depends_on/health ordering, so
		// services boot before their dependencies are healthy and exit. `up -d`
		// honors ordering, starts stopped containers, and recreates any that
		// were removed (e.g. after `rayls down`). Any image missing locally is
		// fetched here, so this goes through the rate-limit-aware helper.
		output, err := stacks.StartServices(version, false, args...)
		if err != nil {
			if strings.Contains(output, "no configuration file provided") ||
				strings.Contains(output, "Can't find") {
				return fmt.Errorf("no docker-compose.yml found. Run 'rayls init' first")
			}
			return fmt.Errorf("failed to start services: %w", err)
		}

		fmt.Println("Services started successfully")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
