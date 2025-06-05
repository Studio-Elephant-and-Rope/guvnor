package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/server"
)

// serveCmd represents the serve command
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the Guvnor HTTP server",
	Long: `Start the Guvnor HTTP server to handle incident management requests.

The server provides:
  • REST API for incident management
  • Health check endpoint at /health
  • Graceful shutdown on SIGTERM/SIGINT
  • Structured logging for all requests
  • Configurable timeouts and limits

The server will load configuration from:
  1. Environment variables (GUVNOR_*)
  2. Configuration file (if specified with --config)
  3. Default values

Examples:
  guvnor serve                           # Start with default configuration
  guvnor serve --config guvnor.yaml     # Start with custom config file
  GUVNOR_SERVER_PORT=9090 guvnor serve  # Override port via environment`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		return runServe(configFile)
	},
}

// runServe starts the HTTP server with the given configuration.
func runServe(configFile string) error {
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

	// Log startup information
	logger.Info("Starting Guvnor HTTP server",
		"version", getVersionString(),
		"commit", getCommitString(),
		"build_date", getBuildDateString(),
		"environment", logger.GetConfig().Environment,
		"config_file", configFile,
		"server_address", fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		"storage_type", cfg.Storage.Type,
		"telemetry_enabled", cfg.Telemetry.Enabled,
	)

	// Create and configure the server
	srv, err := server.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server with graceful shutdown handling
	logger.Info("Server ready to accept connections", "address", srv.GetAddr())

	if err := srv.StartWithGracefulShutdown(); err != nil {
		logger.WithError(err).Error("Server shutdown with error")
		return fmt.Errorf("server error: %w", err)
	}

	logger.Info("Server shutdown completed")
	return nil
}

// init registers the serve command with the root command.
func init() {
	rootCmd.AddCommand(serveCmd)
}
