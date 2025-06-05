// Package cmd contains the command-line interface for the Guvnor incident management platform.
//
// This package provides CLI commands for managing incidents, configuring the platform,
// and administrative tasks using the Cobra framework.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "guvnor",
	Short: "Guvnor is an open-source incident management platform",
	Long: `Guvnor is a reliable, self-hostable incident management platform
built for SREs who trust git commits over marketing promises.

Key features:
  • OpenTelemetry-native monitoring and alerting
  • Self-hostable with minimal dependencies
  • Simple, boring technology that works at 3 AM
  • Built-in escalation and on-call management

For more information, visit https://github.com/Studio-Elephant-and-Rope/guvnor`,
	// Run function is optional for the root command
	Run: func(cmd *cobra.Command, args []string) {
		// If no subcommand is provided, show help
		cmd.Help()
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// init initialises the CLI configuration and adds global flags.
func init() {
	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	// Add global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().String("log-level", "info", "Set log level (debug, info, warn, error)")
	rootCmd.PersistentFlags().String("config", "", "Config file (default is $HOME/.guvnor.yaml)")

	// Cobra also supports local flags, which will only run
	// when this action is called directly.
	rootCmd.Flags().BoolP("version", "V", false, "Show version information")
}

// GetRootCmd returns the root command for testing purposes.
func GetRootCmd() *cobra.Command {
	return rootCmd
}
