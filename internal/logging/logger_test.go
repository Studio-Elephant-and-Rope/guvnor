package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestEnvironment_String(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want string
	}{
		{
			name: "development environment",
			env:  Development,
			want: "development",
		},
		{
			name: "production environment",
			env:  Production,
			want: "production",
		},
		{
			name: "test environment",
			env:  Test,
			want: "test",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.String(); got != tt.want {
				t.Errorf("Environment.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnvironment_IsValid(t *testing.T) {
	tests := []struct {
		name string
		env  Environment
		want bool
	}{
		{
			name: "development is valid",
			env:  Development,
			want: true,
		},
		{
			name: "production is valid",
			env:  Production,
			want: true,
		},
		{
			name: "test is valid",
			env:  Test,
			want: true,
		},
		{
			name: "invalid environment",
			env:  Environment("invalid"),
			want: false,
		},
		{
			name: "empty environment",
			env:  Environment(""),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.env.IsValid(); got != tt.want {
				t.Errorf("Environment.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	tests := []struct {
		name        string
		env         Environment
		wantLevel   slog.Level
		wantSource  bool
	}{
		{
			name:       "development config",
			env:        Development,
			wantLevel:  slog.LevelDebug,
			wantSource: true,
		},
		{
			name:       "production config",
			env:        Production,
			wantLevel:  slog.LevelInfo,
			wantSource: false,
		},
		{
			name:       "test config",
			env:        Test,
			wantLevel:  slog.LevelWarn,
			wantSource: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultConfig(tt.env)

			if config.Environment != tt.env {
				t.Errorf("DefaultConfig().Environment = %v, want %v", config.Environment, tt.env)
			}
			if config.Level != tt.wantLevel {
				t.Errorf("DefaultConfig().Level = %v, want %v", config.Level, tt.wantLevel)
			}
			if config.AddSource != tt.wantSource {
				t.Errorf("DefaultConfig().AddSource = %v, want %v", config.AddSource, tt.wantSource)
			}
			if config.Output == nil {
				t.Error("DefaultConfig().Output should not be nil")
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
	}{
		{
			name: "valid development config",
			config: Config{
				Environment: Development,
				Level:       slog.LevelDebug,
				Output:      &bytes.Buffer{},
				AddSource:   true,
			},
			wantErr: false,
		},
		{
			name: "valid production config",
			config: Config{
				Environment: Production,
				Level:       slog.LevelInfo,
				Output:      &bytes.Buffer{},
				AddSource:   false,
			},
			wantErr: false,
		},
		{
			name: "invalid environment",
			config: Config{
				Environment: Environment("invalid"),
				Level:       slog.LevelInfo,
				Output:      &bytes.Buffer{},
			},
			wantErr: true,
		},
		{
			name: "nil output uses default",
			config: Config{
				Environment: Development,
				Level:       slog.LevelInfo,
				Output:      nil,
				AddSource:   false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger(tt.config)

			if tt.wantErr && err == nil {
				t.Error("NewLogger() expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("NewLogger() unexpected error: %v", err)
			}
			if !tt.wantErr && logger == nil {
				t.Error("NewLogger() returned nil logger without error")
			}
			if !tt.wantErr && logger != nil {
				if logger.config.Environment != tt.config.Environment {
					t.Errorf("Logger environment = %v, want %v", logger.config.Environment, tt.config.Environment)
				}
			}
		})
	}
}

func TestLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// Log a message
	logger.Info("test message", "key", "value", "number", 42)

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Verify JSON structure
	if logEntry["level"] != "INFO" {
		t.Errorf("Expected level INFO, got %v", logEntry["level"])
	}
	if logEntry["msg"] != "test message" {
		t.Errorf("Expected msg 'test message', got %v", logEntry["msg"])
	}
	if logEntry["key"] != "value" {
		t.Errorf("Expected key 'value', got %v", logEntry["key"])
	}
	if logEntry["number"] != float64(42) {
		t.Errorf("Expected number 42, got %v", logEntry["number"])
	}
	if _, exists := logEntry["time"]; !exists {
		t.Error("Expected time field in JSON output")
	}
}

func TestLogger_LogLevelFiltering(t *testing.T) {
	tests := []struct {
		name       string
		level      slog.Level
		logFunc    func(*Logger)
		shouldLog  bool
	}{
		{
			name:  "debug level allows debug logs",
			level: slog.LevelDebug,
			logFunc: func(l *Logger) {
				l.Debug("debug message")
			},
			shouldLog: true,
		},
		{
			name:  "info level filters debug logs",
			level: slog.LevelInfo,
			logFunc: func(l *Logger) {
				l.Debug("debug message")
			},
			shouldLog: false,
		},
		{
			name:  "info level allows info logs",
			level: slog.LevelInfo,
			logFunc: func(l *Logger) {
				l.Info("info message")
			},
			shouldLog: true,
		},
		{
			name:  "warn level filters info logs",
			level: slog.LevelWarn,
			logFunc: func(l *Logger) {
				l.Info("info message")
			},
			shouldLog: false,
		},
		{
			name:  "error level allows error logs",
			level: slog.LevelError,
			logFunc: func(l *Logger) {
				l.Error("error message")
			},
			shouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			config := Config{
				Environment: Production,
				Level:       tt.level,
				Output:      &buf,
				AddSource:   false,
			}

			logger, err := NewLogger(config)
			if err != nil {
				t.Fatalf("NewLogger() error: %v", err)
			}

			tt.logFunc(logger)

			if tt.shouldLog && buf.Len() == 0 {
				t.Error("Expected log output but got none")
			}
			if !tt.shouldLog && buf.Len() > 0 {
				t.Errorf("Expected no log output but got: %s", buf.String())
			}
		})
	}
}

func TestLogger_WithFields(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// Create logger with fields
	enrichedLogger := logger.WithFields("service", "guvnor", "version", "1.0.0")
	enrichedLogger.Info("test message")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	// Verify fields are included
	if logEntry["service"] != "guvnor" {
		t.Errorf("Expected service 'guvnor', got %v", logEntry["service"])
	}
	if logEntry["version"] != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got %v", logEntry["version"])
	}
}

func TestLogger_WithRequestID(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	requestID := "req-123456"
	logger.WithRequestID(requestID).Info("handling request")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["request_id"] != requestID {
		t.Errorf("Expected request_id '%s', got %v", requestID, logEntry["request_id"])
	}
}

func TestLogger_WithIncidentID(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	incidentID := "inc-789012"
	logger.WithIncidentID(incidentID).Info("processing incident")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["incident_id"] != incidentID {
		t.Errorf("Expected incident_id '%s', got %v", incidentID, logEntry["incident_id"])
	}
}

func TestLogger_WithError(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	testErr := errors.New("test error")
	logger.WithError(testErr).Error("operation failed")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["error"] != testErr.Error() {
		t.Errorf("Expected error '%s', got %v", testErr.Error(), logEntry["error"])
	}
}

func TestLogger_WithDuration(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	duration := 250 * time.Millisecond
	logger.WithDuration(duration).Info("operation completed")

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["duration_ms"] != float64(250) {
		t.Errorf("Expected duration_ms 250, got %v", logEntry["duration_ms"])
	}
}

func TestLogger_ContextPropagation(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// Add logger to context
	ctx := logger.WithContext(context.Background())

	// Retrieve logger from context
	retrievedLogger := FromContext(ctx)

	// Log with retrieved logger
	retrievedLogger.Info("context test")

	if buf.Len() == 0 {
		t.Error("Expected log output from context logger")
	}

	// Parse the JSON output to ensure it's valid
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["msg"] != "context test" {
		t.Errorf("Expected msg 'context test', got %v", logEntry["msg"])
	}
}

func TestFromContext_Fallback(t *testing.T) {
	// Test with context that doesn't contain a logger
	ctx := context.Background()
	logger := FromContext(ctx)

	if logger == nil {
		t.Error("FromContext should return a fallback logger when none exists in context")
	}

	// Ensure the fallback logger works
	// We can't easily test the fallback logger's output since it uses a global default,
	// but we can ensure it doesn't panic
	logger.Info("fallback test")
}

func TestLogger_LogOperationStartEnd(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelDebug,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// Test successful operation
	start := logger.LogOperationStart("test_operation", "component", "test")
	time.Sleep(10 * time.Millisecond) // Simulate some work
	logger.LogOperationEnd("test_operation", start, nil, "component", "test")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 2 {
		t.Errorf("Expected 2 log lines, got %d", len(lines))
	}

	// Verify start log
	var startEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &startEntry); err != nil {
		t.Fatalf("Failed to parse start log JSON: %v", err)
	}

	if startEntry["operation"] != "test_operation" {
		t.Errorf("Expected operation 'test_operation', got %v", startEntry["operation"])
	}
	if startEntry["phase"] != "start" {
		t.Errorf("Expected phase 'start', got %v", startEntry["phase"])
	}

	// Verify end log
	var endEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lines[1]), &endEntry); err != nil {
		t.Fatalf("Failed to parse end log JSON: %v", err)
	}

	if endEntry["operation"] != "test_operation" {
		t.Errorf("Expected operation 'test_operation', got %v", endEntry["operation"])
	}
	if endEntry["phase"] != "end" {
		t.Errorf("Expected phase 'end', got %v", endEntry["phase"])
	}
	if _, exists := endEntry["duration_ms"]; !exists {
		t.Error("Expected duration_ms field in end log")
	}
}

func TestLogger_LogOperationEndWithError(t *testing.T) {
	var buf bytes.Buffer
	config := Config{
		Environment: Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("NewLogger() error: %v", err)
	}

	// Test failed operation
	start := time.Now()
	testErr := errors.New("operation failed")
	logger.LogOperationEnd("test_operation", start, testErr)

	// Parse the JSON output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if logEntry["level"] != "ERROR" {
		t.Errorf("Expected level ERROR for failed operation, got %v", logEntry["level"])
	}
	if logEntry["error"] != testErr.Error() {
		t.Errorf("Expected error '%s', got %v", testErr.Error(), logEntry["error"])
	}
}

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      slog.Level
		wantError bool
	}{
		{
			name:      "debug level",
			input:     "debug",
			want:      slog.LevelDebug,
			wantError: false,
		},
		{
			name:      "info level",
			input:     "info",
			want:      slog.LevelInfo,
			wantError: false,
		},
		{
			name:      "warn level",
			input:     "warn",
			want:      slog.LevelWarn,
			wantError: false,
		},
		{
			name:      "warning level",
			input:     "warning",
			want:      slog.LevelWarn,
			wantError: false,
		},
		{
			name:      "error level",
			input:     "error",
			want:      slog.LevelError,
			wantError: false,
		},
		{
			name:      "uppercase input",
			input:     "INFO",
			want:      slog.LevelInfo,
			wantError: false,
		},
		{
			name:      "invalid level",
			input:     "invalid",
			want:      slog.LevelInfo,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseLogLevel(tt.input)

			if tt.wantError && err == nil {
				t.Error("parseLogLevel() expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("parseLogLevel() unexpected error: %v", err)
			}
			if !tt.wantError && got != tt.want {
				t.Errorf("parseLogLevel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewFromEnvironment(t *testing.T) {
	// Save original environment variables
	originalEnv := os.Getenv("GUVNOR_ENV")
	originalLevel := os.Getenv("GUVNOR_LOG_LEVEL")
	originalSource := os.Getenv("GUVNOR_LOG_ADD_SOURCE")

	// Cleanup function
	defer func() {
		os.Setenv("GUVNOR_ENV", originalEnv)
		os.Setenv("GUVNOR_LOG_LEVEL", originalLevel)
		os.Setenv("GUVNOR_LOG_ADD_SOURCE", originalSource)
	}()

	tests := []struct {
		name        string
		envVars     map[string]string
		wantEnv     Environment
		wantLevel   slog.Level
		wantSource  bool
		wantError   bool
	}{
		{
			name:        "default configuration",
			envVars:     map[string]string{},
			wantEnv:     Development,
			wantLevel:   slog.LevelDebug,
			wantSource:  true,
			wantError:   false,
		},
		{
			name: "production environment",
			envVars: map[string]string{
				"GUVNOR_ENV": "production",
			},
			wantEnv:     Production,
			wantLevel:   slog.LevelInfo,
			wantSource:  false,
			wantError:   false,
		},
		{
			name: "custom log level",
			envVars: map[string]string{
				"GUVNOR_ENV":       "development",
				"GUVNOR_LOG_LEVEL": "warn",
			},
			wantEnv:     Development,
			wantLevel:   slog.LevelWarn,
			wantSource:  true,
			wantError:   false,
		},
		{
			name: "custom add source",
			envVars: map[string]string{
				"GUVNOR_ENV":            "production",
				"GUVNOR_LOG_ADD_SOURCE": "true",
			},
			wantEnv:     Production,
			wantLevel:   slog.LevelInfo,
			wantSource:  true,
			wantError:   false,
		},
		{
			name: "invalid log level",
			envVars: map[string]string{
				"GUVNOR_LOG_LEVEL": "invalid",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment variables
			os.Unsetenv("GUVNOR_ENV")
			os.Unsetenv("GUVNOR_LOG_LEVEL")
			os.Unsetenv("GUVNOR_LOG_ADD_SOURCE")

			// Set test environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
			}

			logger, err := NewFromEnvironment()

			if tt.wantError && err == nil {
				t.Error("NewFromEnvironment() expected error but got none")
				return
			}
			if !tt.wantError && err != nil {
				t.Errorf("NewFromEnvironment() unexpected error: %v", err)
				return
			}
			if tt.wantError {
				return
			}

			config := logger.GetConfig()
			if config.Environment != tt.wantEnv {
				t.Errorf("Environment = %v, want %v", config.Environment, tt.wantEnv)
			}
			if config.Level != tt.wantLevel {
				t.Errorf("Level = %v, want %v", config.Level, tt.wantLevel)
			}
			if config.AddSource != tt.wantSource {
				t.Errorf("AddSource = %v, want %v", config.AddSource, tt.wantSource)
			}
		})
	}
}

func TestGetCaller(t *testing.T) {
	file, line, function := GetCaller(0)

	if file == "unknown" || line == 0 || function == "unknown" {
		t.Errorf("GetCaller() returned unknown values: file=%s, line=%d, function=%s", file, line, function)
	}

	if !strings.Contains(function, "TestGetCaller") {
		t.Errorf("GetCaller() function should contain 'TestGetCaller', got %s", function)
	}
}
