package cmd

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var checkUpdate bool

// Version information - set from main package
var (
	Version   string
	CommitSHA string
	BuildDate string
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Long:  `Display the current version of Rayls CLI along with build information.`,
	Run: func(cmd *cobra.Command, args []string) {
		cyan := color.New(color.FgCyan).SprintFunc()
		white := color.New(color.FgWhite).SprintFunc()
		gray := color.New(color.FgHiBlack).SprintFunc()

		fmt.Printf("%s %s\n", cyan("Rayls CLI"), white(Version))
		fmt.Printf("%s %s\n", gray("Commit:"), gray(CommitSHA))
		fmt.Printf("%s %s\n", gray("Built:"), gray(BuildDate))

		if checkUpdate {
			fmt.Println() // Empty line
			if err := checkForUpdates(); err != nil {
				color.Red("Failed to check for updates: %v", err)
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	versionCmd.Flags().BoolVar(&checkUpdate, "check", false, "Check for available updates")
}
