// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Rayls Core Ltd.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

const logo = "\n" +
	"______            _      \n" +
	"| ___ \\          | |     \n" +
	"| |_/ /__ _ _   _| |___  \n" +
	"|    // _` | | | | / __| \n" +
	"| |\\ \\ (_| | |_| | \\__ \\ \n" +
	"\\_| \\_\\__,_|\\__, |_|___/ \n" +
	"             __/ |       \n" +
	"            |___/        \n"

var logoPrinted bool

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "rayls",
	Short: "The Rayls CLI tool for managing Rayls blockchain environments",
	Long: `The Rayls CLI is a tool designed to streamline the provisioning and management of the Rayls blockchain stack.
Currently, this tool focuses on deploying a local demo environment on a single host, making it ideal for sales demonstrations, proof-of-concept exploration, and local development. It automates the generation of Docker Compose configurations and manages the lifecycle of the Rayls components.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if !logoPrinted {
			fmt.Print(logo)
			logoPrinted = true
		}
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.parfin-rays-installation.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")

	// Override HelpFunc to ensure logo is printed even on help commands
	rootCmd.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if !logoPrinted {
			fmt.Print(logo)
			logoPrinted = true
		}
		// Default help behavior
		cmd.Println(cmd.UsageString())
	})
}
