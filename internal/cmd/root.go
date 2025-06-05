// Package cmd contains the command-line interface for the Guvnor incident management platform.
//
// This package provides CLI commands for managing incidents, configuring the platform,
// and administrative tasks using the Cobra framework.
package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
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

	// Add the run command
	rootCmd.AddCommand(createRunCommand())
}

// createRunCommand creates the run command that starts the Guvnor server.
func createRunCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "run",
		Short: "Start the Guvnor incident management server",
		Long: `Start the Guvnor incident management server with the specified configuration.

The server will load configuration from:
  1. Environment variables (GUVNOR_*)
  2. Configuration file (if specified with --config)
  3. Default values

Example:
  guvnor run                           # Start with default configuration
  guvnor run --config guvnor.yaml     # Start with custom config file
  GUVNOR_SERVER_PORT=9090 guvnor run  # Override port via environment`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile, _ := cmd.Flags().GetString("config")
			return runGuvnor(configFile)
		},
	}
}

// runGuvnor starts the main application with the given configuration.
func runGuvnor(configFile string) error {
	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Initialize structured logger from environment
	logger, err := logging.NewFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	// Add logger to context for propagation
	ctx := logger.WithContext(context.Background())

	// Log application startup with configuration info
	logger.Info("Starting Guvnor incident management platform",
		"version", getVersionString(),
		"commit", getCommitString(),
		"build_date", getBuildDateString(),
		"environment", logger.GetConfig().Environment,
		"config_file", configFile,
		"server_host", cfg.Server.Host,
		"server_port", cfg.Server.Port,
		"storage_type", cfg.Storage.Type,
		"telemetry_enabled", cfg.Telemetry.Enabled,
	)

	// Log configuration details (excluding sensitive information)
	logger.Debug("Configuration loaded",
		"server", fmt.Sprintf("%+v", cfg.Server),
		"storage_type", cfg.Storage.Type,
		"telemetry", fmt.Sprintf("%+v", cfg.Telemetry),
	)

	// Demonstrate structured logging with different levels
	logger.Debug("Debug information", "component", "main")
	logger.Info("Application initialized successfully")

	// Demonstrate operation with configuration context
	if err := demonstrateOperation(ctx, cfg); err != nil {
		logger.WithError(err).Error("Operation failed")
		return err
	}

	logger.Info("Guvnor startup complete",
		"ready_to_serve", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
	)

	return nil
}

// demonstrateOperation shows how to use logger from context with configuration.
func demonstrateOperation(ctx context.Context, cfg *config.Config) error {
	logger := logging.FromContext(ctx)

	// Log operation start
	start := logger.LogOperationStart("demo_operation", "component", "main")

	// Simulate some work using configuration
	logger.Info("Performing demonstration operation",
		"using_storage", cfg.Storage.Type,
		"server_configured_for", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
	)

	// Simulate configuration-specific logic
	if cfg.Telemetry.Enabled {
		logger.Info("Telemetry is enabled",
			"service_name", cfg.Telemetry.ServiceName,
			"sample_rate", cfg.Telemetry.SampleRate,
		)
	} else {
		logger.Debug("Telemetry is disabled")
	}

	// Log operation completion
	logger.LogOperationEnd("demo_operation", start, nil, "component", "main")

	return nil
}

// GetRootCmd returns the root command for testing purposes.
func GetRootCmd() *cobra.Command {
	return rootCmd
}
