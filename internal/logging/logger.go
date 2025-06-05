// Package logging provides structured logging capabilities for the Guvnor incident management platform.
//
// This package configures and manages structured logging using Go's standard library slog,
// providing environment-aware configuration and context propagation for consistent logging
// across the application.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"time"
)

// Environment represents the deployment environment.
type Environment string

// Environment constants define the valid deployment environments.
const (
	Development Environment = "development"
	Production  Environment = "production"
	Test        Environment = "test"
)

// String returns the string representation of the environment.
func (e Environment) String() string {
	return string(e)
}

// IsValid checks if the environment is one of the defined valid environments.
func (e Environment) IsValid() bool {
	switch e {
	case Development, Production, Test:
		return true
	default:
		return false
	}
}

// Config holds the configuration for the logger.
type Config struct {
	Environment Environment `json:"environment"`
	Level       slog.Level  `json:"level"`
	Output      io.Writer   `json:"-"`
	AddSource   bool        `json:"add_source"`
}

// Logger wraps slog.Logger with additional functionality for structured logging.
type Logger struct {
	*slog.Logger
	config Config
}

// DefaultConfig returns a default configuration based on the environment.
func DefaultConfig(env Environment) Config {
	config := Config{
		Environment: env,
		Level:       slog.LevelInfo,
		Output:      os.Stdout,
		AddSource:   false,
	}

	switch env {
	case Development:
		config.Level = slog.LevelDebug
		config.AddSource = true
	case Production:
		config.Level = slog.LevelInfo
		config.AddSource = false
	case Test:
		config.Level = slog.LevelWarn
		config.AddSource = false
	}

	return config
}

// NewLogger creates a new structured logger with the given configuration.
//
// The logger is configured based on the environment:
//   - Development: Pretty console output with debug level and source information
//   - Production: JSON output with info level for machine parsing
//   - Test: JSON output with warn level to reduce noise
//
// Returns a Logger instance or an error if configuration is invalid.
func NewLogger(config Config) (*Logger, error) {
	if !config.Environment.IsValid() {
		return nil, fmt.Errorf("invalid environment: %s", config.Environment)
	}

	if config.Output == nil {
		config.Output = os.Stdout
	}

	var handler slog.Handler

	handlerOpts := &slog.HandlerOptions{
		Level:     config.Level,
		AddSource: config.AddSource,
	}

	switch config.Environment {
	case Development:
		// Pretty console output for development
		handler = slog.NewTextHandler(config.Output, handlerOpts)
	case Production, Test:
		// JSON output for production and testing (machine parseable)
		handler = slog.NewJSONHandler(config.Output, handlerOpts)
	}

	logger := slog.New(handler)

	return &Logger{
		Logger: logger,
		config: config,
	}, nil
}

// NewFromEnvironment creates a logger using environment variables.
//
// Environment variables:
//   - GUVNOR_ENV: Sets the environment (development, production, test)
//   - GUVNOR_LOG_LEVEL: Sets the log level (debug, info, warn, error)
//   - GUVNOR_LOG_ADD_SOURCE: Enables source information (true, false)
//
// Returns a Logger instance with default configuration if environment variables are not set.
func NewFromEnvironment() (*Logger, error) {
	env := Development
	if envVar := os.Getenv("GUVNOR_ENV"); envVar != "" {
		env = Environment(strings.ToLower(envVar))
	}

	config := DefaultConfig(env)

	// Override log level if specified
	if levelVar := os.Getenv("GUVNOR_LOG_LEVEL"); levelVar != "" {
		level, err := parseLogLevel(levelVar)
		if err != nil {
			return nil, fmt.Errorf("invalid log level: %w", err)
		}
		config.Level = level
	}

	// Override add source if specified
	if sourceVar := os.Getenv("GUVNOR_LOG_ADD_SOURCE"); sourceVar != "" {
		config.AddSource = strings.ToLower(sourceVar) == "true"
	}

	return NewLogger(config)
}

// parseLogLevel converts a string to slog.Level.
func parseLogLevel(level string) (slog.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("unknown log level: %s", level)
	}
}

// WithContext adds logger to context for propagation.
//
// This allows the logger to be retrieved from context throughout the application
// using FromContext. The logger can be augmented with request-specific fields
// before adding to context.
func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

// WithFields returns a new logger with additional structured fields.
//
// This is useful for adding context-specific fields that should be included
// in all log entries from this logger instance.
func (l *Logger) WithFields(fields ...any) *Logger {
	return &Logger{
		Logger: l.Logger.With(fields...),
		config: l.config,
	}
}

// WithRequestID returns a new logger with a request ID field.
//
// This is a convenience method for adding request IDs to log entries,
// which is essential for tracing requests through the system.
func (l *Logger) WithRequestID(requestID string) *Logger {
	return l.WithFields("request_id", requestID)
}

// WithIncidentID returns a new logger with an incident ID field.
//
// This is a convenience method for adding incident IDs to log entries,
// which helps with debugging and auditing incident-related operations.
func (l *Logger) WithIncidentID(incidentID string) *Logger {
	return l.WithFields("incident_id", incidentID)
}

// WithError returns a new logger with an error field.
//
// This is a convenience method for including error information in log entries
// while preserving the error's context and stack trace information.
func (l *Logger) WithError(err error) *Logger {
	return l.WithFields("error", err.Error())
}

// WithDuration returns a new logger with a duration field.
//
// This is useful for logging operation timings and performance metrics.
func (l *Logger) WithDuration(duration time.Duration) *Logger {
	return l.WithFields("duration_ms", duration.Milliseconds())
}

// loggerKey is used as the key for storing logger in context.
type loggerKey struct{}

// FromContext retrieves the logger from context.
//
// If no logger is found in context, returns a default logger configured
// for the current environment. This ensures that logging always works
// even if context propagation fails.
func FromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*Logger); ok {
		return logger
	}

	// Fallback to default logger if not in context
	logger, err := NewFromEnvironment()
	if err != nil {
		// Last resort: create a basic logger
		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		return &Logger{
			Logger: slog.New(handler),
			config: DefaultConfig(Development),
		}
	}

	return logger
}

// GetConfig returns the logger's configuration.
//
// This is useful for debugging logger configuration and ensuring
// the logger is configured as expected.
func (l *Logger) GetConfig() Config {
	return l.config
}

// LogOperationStart logs the start of an operation with timing information.
//
// This should be paired with LogOperationEnd to provide complete timing
// information for operations. Returns the start time for use with LogOperationEnd.
func (l *Logger) LogOperationStart(operation string, fields ...any) time.Time {
	start := time.Now()
	allFields := append([]any{"operation", operation, "phase", "start"}, fields...)
	l.Debug("Operation started", allFields...)
	return start
}

// LogOperationEnd logs the completion of an operation with duration.
//
// This should be called with the start time returned from LogOperationStart
// to provide accurate timing information.
func (l *Logger) LogOperationEnd(operation string, start time.Time, err error, fields ...any) {
	duration := time.Since(start)
	allFields := append([]any{
		"operation", operation,
		"phase", "end",
		"duration_ms", duration.Milliseconds(),
	}, fields...)

	if err != nil {
		allFields = append(allFields, "error", err.Error())
		l.Error("Operation failed", allFields...)
	} else {
		l.Info("Operation completed", allFields...)
	}
}

// GetCaller returns information about the calling function.
//
// This is useful for debugging and can be manually added to log entries
// when automatic source information is not sufficient.
func GetCaller(skip int) (file string, line int, function string) {
	pc, file, line, ok := runtime.Caller(skip + 1)
	if !ok {
		return "unknown", 0, "unknown"
	}

	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return file, line, "unknown"
	}

	return file, line, fn.Name()
}
