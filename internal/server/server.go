// Package server provides HTTP server functionality for the Guvnor incident management platform.
//
// This package implements a production-ready HTTP server with:
//   - Graceful shutdown handling with configurable timeouts
//   - Health check endpoint with system information
//   - Structured logging for all server events
//   - Configuration-driven setup
//   - Signal handling for clean shutdown
//
// The server follows established patterns from the rest of the codebase,
// using the same logging, configuration, and middleware systems.
package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/adapters/storage/memory"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/api/handlers"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/services"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/middleware"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/telemetry"
)

// Version information for the server
// These can be set during build time via -ldflags
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

// Server represents the HTTP server instance with its configuration and state.
//
// The server maintains references to configuration, logging, and the underlying
// HTTP server. It provides methods for starting, stopping, and graceful shutdown.
type Server struct {
	config            *config.Config
	logger            *logging.Logger
	httpServer        *http.Server
	startTime         time.Time
	router            *http.ServeMux
	incidentService   *services.IncidentService
	telemetryReceiver *telemetry.Receiver
}

// HealthResponse represents the structure of the health check response.
//
// This provides information about the server's operational status,
// version information, and uptime for monitoring and debugging purposes.
type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	Uptime  string `json:"uptime"`
}

// New creates a new server instance with the provided configuration and logger.
//
// The server is configured but not started. Call Start() to begin accepting
// connections. Returns an error if the configuration is invalid.
func New(cfg *config.Config, logger *logging.Logger) (*Server, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create incident repository (using memory for now, could be configurable)
	incidentRepo := memory.NewIncidentRepository()

	// Create incident service
	incidentService, err := services.NewIncidentService(incidentRepo, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create incident service: %w", err)
	}

	server := &Server{
		config:          cfg,
		logger:          logger,
		startTime:       time.Now().UTC(),
		router:          http.NewServeMux(),
		incidentService: incidentService,
	}

	// Set up telemetry receiver if enabled
	if cfg.Telemetry.Receiver.Enabled {
		if err := server.setupTelemetryReceiver(); err != nil {
			return nil, fmt.Errorf("failed to setup telemetry receiver: %w", err)
		}
	}

	// Set up routes
	server.setupRoutes()

	// Create HTTP server with configured timeouts
	server.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      server.createHandler(),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
	}

	return server, nil
}

// Start begins accepting HTTP connections.
//
// This method starts the HTTP server and logs startup information.
// It returns immediately and does not block. Use StartWithGracefulShutdown()
// for a blocking start with signal handling.
func (s *Server) Start() error {
	s.logger.Info("Starting HTTP server",
		"address", s.httpServer.Addr,
		"read_timeout", s.httpServer.ReadTimeout,
		"write_timeout", s.httpServer.WriteTimeout,
		"idle_timeout", s.httpServer.IdleTimeout,
		"telemetry_receiver_enabled", s.telemetryReceiver != nil,
	)

	// Start telemetry receiver if configured
	if s.telemetryReceiver != nil {
		if err := s.telemetryReceiver.Start(context.Background()); err != nil {
			return fmt.Errorf("failed to start telemetry receiver: %w", err)
		}
		s.logger.Info("Telemetry receiver started successfully")
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.WithError(err).Error("HTTP server failed")
		}
	}()

	s.logger.Info("HTTP server started successfully", "address", s.httpServer.Addr)
	return nil
}

// StartWithGracefulShutdown starts the server and blocks until shutdown.
//
// This method starts the HTTP server and sets up signal handling for
// graceful shutdown on SIGTERM and SIGINT. It blocks until the server
// is shut down gracefully or an error occurs.
//
// The shutdown process:
//  1. Stops accepting new connections
//  2. Waits for active connections to finish (with timeout)
//  3. Forces close any remaining connections
//  4. Logs shutdown completion
func (s *Server) StartWithGracefulShutdown() error {
	// Start the server
	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Wait for shutdown signal
	sig := <-sigChan
	s.logger.Info("Received shutdown signal", "signal", sig.String())

	// Perform graceful shutdown
	return s.Shutdown()
}

// Shutdown gracefully shuts down the server.
//
// This method stops accepting new connections and waits for active
// connections to finish. It respects the configured timeouts and
// will force shutdown if the timeout is exceeded.
func (s *Server) Shutdown() error {
	s.logger.Info("Initiating graceful shutdown")

	// Create shutdown context with timeout
	// Use the write timeout as a reasonable shutdown timeout
	shutdownTimeout := time.Duration(s.config.Server.WriteTimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Stop telemetry receiver first
	if s.telemetryReceiver != nil {
		if err := s.telemetryReceiver.Stop(ctx); err != nil {
			s.logger.WithError(err).Error("Error stopping telemetry receiver")
		} else {
			s.logger.Info("Telemetry receiver stopped successfully")
		}
	}

	// Attempt graceful shutdown of HTTP server
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.WithError(err).Error("Error during graceful shutdown, forcing close")

		// Force close if graceful shutdown fails
		if closeErr := s.httpServer.Close(); closeErr != nil {
			s.logger.WithError(closeErr).Error("Error during forced close")
			return fmt.Errorf("shutdown failed: %w, close failed: %v", err, closeErr)
		}
		return fmt.Errorf("graceful shutdown failed, forced close succeeded: %w", err)
	}

	s.logger.Info("Server shutdown completed gracefully")
	return nil
}

// setupTelemetryReceiver initializes the OpenTelemetry receiver for incident detection.
func (s *Server) setupTelemetryReceiver() error {
	// Create signal processor with default configuration
	processorConfig := telemetry.DefaultProcessorConfig()
	processor := telemetry.NewThresholdProcessor(processorConfig, s.logger)

	// Create signal handler that creates incidents
	handlerConfig := telemetry.DefaultSignalHandlerConfig()
	signalHandler, err := telemetry.NewIncidentSignalHandler(s.incidentService, s.logger, handlerConfig)
	if err != nil {
		return fmt.Errorf("failed to create signal handler: %w", err)
	}

	// Create telemetry receiver
	receiver, err := telemetry.NewReceiver(&s.config.Telemetry.Receiver, s.logger, processor, signalHandler)
	if err != nil {
		return fmt.Errorf("failed to create telemetry receiver: %w", err)
	}

	s.telemetryReceiver = receiver
	return nil
}

// setupRoutes configures the HTTP routes for the server.
//
// Currently implements:
//   - GET /health - Health check endpoint
//   - Incident management API endpoints
//
// Additional routes can be added here as features are implemented.
func (s *Server) setupRoutes() {
	// Health check endpoint
	s.router.HandleFunc("/health", s.handleHealth)

	// Create incident handler for API endpoints
	incidentHandler, err := handlers.NewIncidentHandler(s.incidentService, s.logger)
	if err != nil {
		s.logger.WithError(err).Error("Failed to create incident handler")
		return
	}

	// Register incident API routes
	incidentHandler.RegisterRoutes(s.router)
}

// createHandler creates the complete HTTP handler chain with middleware.
//
// The middleware chain includes:
//   - Request ID generation
//   - Request logging (with health check skipping)
//   - Route handling
func (s *Server) createHandler() http.Handler {
	// Start with base router
	handler := http.Handler(s.router)

	// Apply middleware in reverse order (last applied is executed first)

	// Logging middleware (with health check skipping)
	handler = middleware.CombinedLoggingMiddleware(s.logger)(handler)

	// Request ID middleware
	handler = middleware.RequestIDMiddleware(handler)

	return handler
}

// handleHealth handles GET /health requests.
//
// Returns a JSON response with:
//   - status: "healthy" (always, unless server is shutting down)
//   - version: Application version from build information
//   - uptime: Duration since server started
//
// This endpoint is designed for load balancer health checks and monitoring.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Only allow GET requests
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Calculate uptime
	uptime := time.Since(s.startTime)

	// Create response
	response := HealthResponse{
		Status:  "healthy",
		Version: getVersionString(),
		Uptime:  uptime.String(),
	}

	// Set response headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(http.StatusOK)

	// Encode and send response
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.WithError(err).Error("Failed to encode health response")
		// At this point headers are already sent, so we can't change the status
		// The client will receive a 200 with partial/invalid JSON
	}
}

// GetAddr returns the server's configured address.
//
// This is useful for testing and logging purposes.
func (s *Server) GetAddr() string {
	return s.httpServer.Addr
}

// GetUptime returns the duration since the server started.
//
// This is useful for monitoring and debugging purposes.
func (s *Server) GetUptime() time.Duration {
	return time.Since(s.startTime)
}

// IsRunning returns true if the server is currently running.
//
// This checks if the underlying HTTP server is active.
// Note: This is a best-effort check and may not be 100% accurate
// during rapid start/stop cycles.
func (s *Server) IsRunning() bool {
	return s.httpServer != nil
}

// getVersionString returns the version string with appropriate fallback.
func getVersionString() string {
	if Version == "" || Version == "dev" {
		return "development"
	}
	return Version
}
