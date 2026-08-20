package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"github.com/raylsnetwork/rayls-sovereign-cli/internal/docker"
	"strings"

	"github.com/spf13/cobra"
)

var (
	removeVolumes bool
	removeOrphans bool
	skipConfirm   bool
	downTimeout   int
)

// downCmd represents the down command
var downCmd = &cobra.Command{
	Use:   "down",
	Short: "Stop and remove containers and networks",
	Long: `Stop and remove containers and networks created by 'rayls init'.

By default, this command will:
- Stop all running containers
- Remove containers
- Remove networks
- Keep volumes (preserving data)

Use the -v flag to also remove volumes.

Examples:
  rayls down               # Stop and remove containers (keeps volumes)
  rayls down -v            # Stop and remove containers AND volumes
  rayls down --remove-orphans # Remove containers for services not in compose file`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Check Docker configuration
		version, err := docker.CheckDockerConfig()
		if err != nil {
			return err
		}

		// Confirm before taking down the stack (unless --yes flag is used)
		if !skipConfirm {
			confirmMsg := "\nThis will stop and remove all containers and networks"
			if removeVolumes {
				confirmMsg += " and volumes (all data is wiped)"
			} else {
				confirmMsg += " (volumes and their data are kept — pass -v to remove them)"
			}
			confirmMsg += ". Are you sure? (y/n) [n]: "
			fmt.Print(confirmMsg)
			scanner := bufio.NewScanner(os.Stdin)
			if scanner.Scan() {
				response := strings.ToLower(strings.TrimSpace(scanner.Text()))
				if response != "y" && response != "yes" {
					fmt.Println("Aborted")
					return nil
				}
			}
		}

		// Build docker compose down command. --remove-orphans is ALWAYS on:
		// this directory holds exactly one stack, so containers that are no
		// longer in the generated compose (a previous topology's hub, private
		// relayers, ...) are stale by definition. Without it they survive `down`, and —
		// worse — they keep volume references alive, silently defeating
		// `down -v` (the shared-config volume then carries the previous
		// deployment's manifests and env files into the next init).
		downArgs := []string{"down", "--remove-orphans"}

		if removeVolumes {
			downArgs = append(downArgs, "-v")
		}

		if downTimeout > 0 {
			downArgs = append(downArgs, "-t", fmt.Sprintf("%d", downTimeout))
		}

		var composeCmd *exec.Cmd
		if version == docker.ComposeV2 {
			composeCmd = exec.Command("docker", append([]string{"compose"}, downArgs...)...)
		} else {
			composeCmd = exec.Command("docker-compose", downArgs...)
		}

		statusMsg := "\nStopping and removing containers and networks"
		if removeVolumes {
			statusMsg += " and volumes"
		}
		statusMsg += "..."
		fmt.Println(statusMsg)

		// Execute the command
		output, err := composeCmd.CombinedOutput()
		if err != nil {
			if strings.Contains(string(output), "no configuration file provided") ||
				strings.Contains(string(output), "Can't find") {
				return fmt.Errorf("no docker-compose.yml found")
			}
			return fmt.Errorf("failed to take down stack: %w\nOutput:\n%s", err, string(output))
		}

		fmt.Println(string(output))
		fmt.Println("Stack removed successfully")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(downCmd)
	downCmd.Flags().BoolVarP(&removeVolumes, "volumes", "v", false, "Remove named volumes (destructive)")
	downCmd.Flags().BoolVar(&removeOrphans, "remove-orphans", true, "Remove containers for services not in the compose file (always on — this directory holds a single stack, so such containers are stale topology leftovers)")
	downCmd.Flags().MarkHidden("remove-orphans")
	downCmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompt")
	downCmd.Flags().IntVarP(&downTimeout, "timeout", "t", 10, "Timeout in seconds to wait for shutdown")
}
