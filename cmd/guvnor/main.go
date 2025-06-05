package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// Version information (should be set at build time)
var (
	Version = "development"
	Commit  = "unknown"
	Date    = "unknown"
)

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// rootCmd creates the root command for the Guvnor application
func rootCmd() *cobra.Command {
	var configFile string

	cmd := &cobra.Command{
		Use:   "guvnor",
		Short: "Guvnor incident management platform",
		Long: `Guvnor is an open-source incident management platform built for reliability.
It provides incident tracking, alerting, and response coordination for SRE teams.`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", Version, Commit, Date),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGuvnor(configFile)
		},
	}

	// Add configuration file flag
	cmd.Flags().StringVarP(&configFile, "config", "c", "",
		"path to configuration file (default: uses environment variables and defaults)")

	return cmd
}

// runGuvnor starts the main application with the given configuration
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
		"version", Version,
		"commit", Commit,
		"build_date", Date,
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

// demonstrateOperation shows how to use logger from context with configuration
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
