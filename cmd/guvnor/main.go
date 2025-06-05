package main

import (
	"context"
	"fmt"
	"os"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

func main() {
	// Initialize structured logger from environment
	logger, err := logging.NewFromEnvironment()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	// Add logger to context for propagation
	ctx := logger.WithContext(context.Background())

	// Log application startup
	logger.Info("Starting Guvnor incident management platform",
		"version", "0.1.0",
		"environment", logger.GetConfig().Environment,
	)

	// Demonstrate structured logging with different levels
	logger.Debug("Debug information", "component", "main")
	logger.Info("Application initialized successfully")
	logger.Warn("This is a warning message", "reason", "demonstration")

	// Demonstrate error logging with context
	if err := demonstrateOperation(ctx); err != nil {
		logger.WithError(err).Error("Operation failed")
		os.Exit(1)
	}

	logger.Info("Guvnor startup complete")
}

// demonstrateOperation shows how to use logger from context
func demonstrateOperation(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	// Log operation start
	start := logger.LogOperationStart("demo_operation", "component", "main")

	// Simulate some work
	logger.Info("Performing demonstration operation")

	// Log operation completion
	logger.LogOperationEnd("demo_operation", start, nil, "component", "main")

	return nil
}
