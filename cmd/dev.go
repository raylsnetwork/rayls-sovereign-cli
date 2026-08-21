package cmd

import (
	"os"

	"github.com/raylsnetwork/rayls-sovereign-cli/internal/stacks"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	devOff    bool
	devStatus bool
	devRepo   string
	devSrc    string
)

var devCmd = &cobra.Command{
	Use:   "dev [component...]",
	Short: "Hack on a component with hot reload (relayer, contracts)",
	Long: `Switch components of a --local stack between the pinned from-source build and
a local checkout you can edit.

Enabling dev mode for a component:
  1. Clones its repo next to the stack directory if you don't have a checkout
     yet (override the location with RAYLS_SRC_DIR or --src).
  2. Records the checkout path as <COMPONENT>_SRC in .env and regenerates
     docker-compose.override.yaml so the service builds from your checkout.
  3. Rebuilds just that component's services; stack state survives.

Then run ` + "`rayls watch`" + `: every file save syncs into the container, where air
rebuilds and restarts the service in a few seconds. Contracts are the
exception — they build from your checkout but redeploys stay explicit.

Examples:
  rayls dev relayer                 # hack on the relayer (kos + pubrelayer)
  rayls dev relayer contracts       # both at once
  rayls dev relayer --repo git@github.com:you/rayls-sovereign-relayer.git
  rayls dev --status                # what's in dev mode?
  rayls dev --off relayer           # back to the pinned build`,
	Run: func(cmd *cobra.Command, args []string) {
		var err error
		switch {
		case devStatus:
			err = stacks.DevStatus()
		case devOff:
			if len(args) == 0 {
				err = cmd.Usage()
				break
			}
			err = stacks.DisableDev(args)
		case len(args) == 0:
			err = cmd.Usage()
		default:
			err = stacks.EnableDev(args, devRepo, devSrc)
			if err == nil {
				cyan := color.New(color.FgCyan).SprintFunc()
				cmd.Printf("\nNext: %s   # sync file saves into the container (hot reload)\n", cyan("rayls watch"))
			}
		}
		if err != nil {
			color.Red("Error: %v", err)
			os.Exit(1)
		}
	},
}

func init() {
	devCmd.Flags().BoolVar(&devOff, "off", false, "Switch the given components back to their pinned from-source builds")
	devCmd.Flags().BoolVar(&devStatus, "status", false, "Show which components are in dev mode")
	devCmd.Flags().StringVar(&devRepo, "repo", "", "Git URL to clone from (e.g. your fork); single component only")
	devCmd.Flags().StringVar(&devSrc, "src", "", "Path to an existing checkout to use; single component only")
	rootCmd.AddCommand(devCmd)
}
