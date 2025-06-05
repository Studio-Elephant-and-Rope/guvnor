package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	// Test server defaults
	if config.Server.Host != "0.0.0.0" {
		t.Errorf("Expected default server host '0.0.0.0', got '%s'", config.Server.Host)
	}
	if config.Server.Port != 8080 {
		t.Errorf("Expected default server port 8080, got %d", config.Server.Port)
	}
	if config.Server.ReadTimeoutSeconds != 30 {
		t.Errorf("Expected default read timeout 30, got %d", config.Server.ReadTimeoutSeconds)
	}

	// Test storage defaults
	if config.Storage.Type != "sqlite" {
		t.Errorf("Expected default storage type 'sqlite', got '%s'", config.Storage.Type)
	}
	if config.Storage.MaxOpenConnections != 25 {
		t.Errorf("Expected default max open connections 25, got %d", config.Storage.MaxOpenConnections)
	}

	// Test telemetry defaults
	if config.Telemetry.Enabled != false {
		t.Errorf("Expected default telemetry enabled false, got %t", config.Telemetry.Enabled)
	}
	if config.Telemetry.ServiceName != "guvnor" {
		t.Errorf("Expected default service name 'guvnor', got '%s'", config.Telemetry.ServiceName)
	}

	// Validate default config is valid
	if err := config.Validate(); err != nil {
		t.Errorf("Default config should be valid, got error: %v", err)
	}
}

func TestLoad_DefaultsOnly(t *testing.T) {
	// Clear any environment variables that might interfere
	clearGuvnorEnvVars()

	config, err := Load("")
	if err != nil {
		t.Fatalf("Load() with no config file should not error, got: %v", err)
	}

	// Should match default config
	defaultConfig := DefaultConfig()
	if config.Server.Port != defaultConfig.Server.Port {
		t.Errorf("Expected port %d, got %d", defaultConfig.Server.Port, config.Server.Port)
	}
	if config.Storage.Type != defaultConfig.Storage.Type {
		t.Errorf("Expected storage type %s, got %s", defaultConfig.Storage.Type, config.Storage.Type)
	}
}

func TestLoad_FromYAMLFile(t *testing.T) {
	// Clear any environment variables that might interfere
	clearGuvnorEnvVars()

	// Create temporary config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "test-config.yaml")

	yamlContent := `
server:
  host: "127.0.0.1"
  port: 9090
  read_timeout_seconds: 60
  write_timeout_seconds: 60
  idle_timeout_seconds: 300

storage:
  type: "postgres"
  dsn: "postgres://user:pass@localhost/testdb"
  max_open_connections: 50
  max_idle_connections: 10
  connection_max_lifetime_minutes: 10

telemetry:
  enabled: true
  service_name: "guvnor-test"
  service_version: "1.0.0"
  endpoint: "http://localhost:14268/api/traces"
  sample_rate: 0.5
`

	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() should not error with valid config file, got: %v", err)
	}

	// Verify values from YAML were loaded
	if config.Server.Host != "127.0.0.1" {
		t.Errorf("Expected host '127.0.0.1', got '%s'", config.Server.Host)
	}
	if config.Server.Port != 9090 {
		t.Errorf("Expected port 9090, got %d", config.Server.Port)
	}
	if config.Storage.Type != "postgres" {
		t.Errorf("Expected storage type 'postgres', got '%s'", config.Storage.Type)
	}
	if config.Storage.MaxOpenConnections != 50 {
		t.Errorf("Expected max open connections 50, got %d", config.Storage.MaxOpenConnections)
	}
	if config.Telemetry.Enabled != true {
		t.Errorf("Expected telemetry enabled true, got %t", config.Telemetry.Enabled)
	}
	if config.Telemetry.SampleRate != 0.5 {
		t.Errorf("Expected sample rate 0.5, got %f", config.Telemetry.SampleRate)
	}
}

func TestLoad_EnvironmentVariableOverrides(t *testing.T) {
	// Clear any existing environment variables
	clearGuvnorEnvVars()

	// Set environment variables
	envVars := map[string]string{
		"GUVNOR_SERVER_HOST":                "192.168.1.100",
		"GUVNOR_SERVER_PORT":                "3000",
		"GUVNOR_STORAGE_TYPE":               "mysql",
		"GUVNOR_STORAGE_MAX_OPEN_CONNECTIONS": "100",
		"GUVNOR_TELEMETRY_ENABLED":          "true",
		"GUVNOR_TELEMETRY_SERVICE_NAME":     "custom-guvnor",
		"GUVNOR_TELEMETRY_ENDPOINT":         "http://localhost:14268/api/traces",
		"GUVNOR_TELEMETRY_SAMPLE_RATE":      "0.8",
	}

	// Set environment variables
	for key, value := range envVars {
		os.Setenv(key, value)
	}

	// Clean up after test
	defer func() {
		for key := range envVars {
			os.Unsetenv(key)
		}
	}()

	config, err := Load("")
	if err != nil {
		t.Fatalf("Load() should not error with environment variables, got: %v", err)
	}

	// Verify environment variables override defaults
	if config.Server.Host != "192.168.1.100" {
		t.Errorf("Expected host '192.168.1.100', got '%s'", config.Server.Host)
	}
	if config.Server.Port != 3000 {
		t.Errorf("Expected port 3000, got %d", config.Server.Port)
	}
	if config.Storage.Type != "mysql" {
		t.Errorf("Expected storage type 'mysql', got '%s'", config.Storage.Type)
	}
	if config.Storage.MaxOpenConnections != 100 {
		t.Errorf("Expected max open connections 100, got %d", config.Storage.MaxOpenConnections)
	}
	if config.Telemetry.Enabled != true {
		t.Errorf("Expected telemetry enabled true, got %t", config.Telemetry.Enabled)
	}
	if config.Telemetry.ServiceName != "custom-guvnor" {
		t.Errorf("Expected service name 'custom-guvnor', got '%s'", config.Telemetry.ServiceName)
	}
	if config.Telemetry.SampleRate != 0.8 {
		t.Errorf("Expected sample rate 0.8, got %f", config.Telemetry.SampleRate)
	}
}

func TestLoad_ConfigFileTakesPrecedenceOverDefaults(t *testing.T) {
	// Clear any environment variables that might interfere
	clearGuvnorEnvVars()

	// Create temporary config file with different values
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "precedence-test.yaml")

	yamlContent := `
server:
  port: 7777
storage:
  type: "custom"
telemetry:
  service_name: "file-guvnor"
`

	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	config, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() should not error, got: %v", err)
	}

	// Values from file should override defaults
	if config.Server.Port != 7777 {
		t.Errorf("Expected port from file 7777, got %d", config.Server.Port)
	}
	if config.Storage.Type != "custom" {
		t.Errorf("Expected storage type from file 'custom', got '%s'", config.Storage.Type)
	}
	if config.Telemetry.ServiceName != "file-guvnor" {
		t.Errorf("Expected service name from file 'file-guvnor', got '%s'", config.Telemetry.ServiceName)
	}

	// Unspecified values should still be defaults
	if config.Server.Host != "0.0.0.0" {
		t.Errorf("Expected default host '0.0.0.0', got '%s'", config.Server.Host)
	}
}

func TestLoad_EnvironmentTakesPrecedenceOverFile(t *testing.T) {
	// Clear any existing environment variables
	clearGuvnorEnvVars()

	// Create config file
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "env-precedence-test.yaml")

	yamlContent := `
server:
  port: 5555
telemetry:
  service_name: "file-service"
`

	err := os.WriteFile(configFile, []byte(yamlContent), 0644)
	if err != nil {
		t.Fatalf("Failed to write test config file: %v", err)
	}

	// Set environment variable that conflicts with file
	os.Setenv("GUVNOR_SERVER_PORT", "6666")
	os.Setenv("GUVNOR_TELEMETRY_SERVICE_NAME", "env-service")
	defer func() {
		os.Unsetenv("GUVNOR_SERVER_PORT")
		os.Unsetenv("GUVNOR_TELEMETRY_SERVICE_NAME")
	}()

	config, err := Load(configFile)
	if err != nil {
		t.Fatalf("Load() should not error, got: %v", err)
	}

	// Environment should override file
	if config.Server.Port != 6666 {
		t.Errorf("Expected port from env 6666, got %d", config.Server.Port)
	}
	if config.Telemetry.ServiceName != "env-service" {
		t.Errorf("Expected service name from env 'env-service', got '%s'", config.Telemetry.ServiceName)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("/path/that/does/not/exist.yaml")
	if err == nil {
		t.Error("Load() should error when config file does not exist")
	}
	if !contains(err.Error(), "does not exist") {
		t.Errorf("Error should mention file doesn't exist, got: %v", err)
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	// Create temporary file with invalid YAML
	tempDir := t.TempDir()
	configFile := filepath.Join(tempDir, "invalid.yaml")

	invalidYAML := `
server:
  port: not-a-number
  invalid-yaml: [unclosed bracket
`

	err := os.WriteFile(configFile, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid config file: %v", err)
	}

	_, err = Load(configFile)
	if err == nil {
		t.Error("Load() should error with invalid YAML")
	}
}

func TestServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  ServerConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: ServerConfig{
				Host:                "localhost",
				Port:                8080,
				ReadTimeoutSeconds:  30,
				WriteTimeoutSeconds: 30,
				IdleTimeoutSeconds:  120,
			},
			wantErr: false,
		},
		{
			name: "port too low",
			config: ServerConfig{
				Port:                0,
				ReadTimeoutSeconds:  30,
				WriteTimeoutSeconds: 30,
				IdleTimeoutSeconds:  120,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "port too high",
			config: ServerConfig{
				Port:                70000,
				ReadTimeoutSeconds:  30,
				WriteTimeoutSeconds: 30,
				IdleTimeoutSeconds:  120,
			},
			wantErr: true,
			errMsg:  "port must be between 1 and 65535",
		},
		{
			name: "negative read timeout",
			config: ServerConfig{
				Port:                8080,
				ReadTimeoutSeconds:  -1,
				WriteTimeoutSeconds: 30,
				IdleTimeoutSeconds:  120,
			},
			wantErr: true,
			errMsg:  "read timeout must be positive",
		},
		{
			name: "zero write timeout",
			config: ServerConfig{
				Port:                8080,
				ReadTimeoutSeconds:  30,
				WriteTimeoutSeconds: 0,
				IdleTimeoutSeconds:  120,
			},
			wantErr: true,
			errMsg:  "write timeout must be positive",
		},
		{
			name: "negative idle timeout",
			config: ServerConfig{
				Port:                8080,
				ReadTimeoutSeconds:  30,
				WriteTimeoutSeconds: 30,
				IdleTimeoutSeconds:  -5,
			},
			wantErr: true,
			errMsg:  "idle timeout must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("ServerConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("ServerConfig.Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestStorageConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  StorageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid config",
			config: StorageConfig{
				Type:                         "postgres",
				DSN:                          "postgres://user:pass@localhost/db",
				MaxOpenConnections:           25,
				MaxIdleConnections:           5,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: false,
		},
		{
			name: "empty type",
			config: StorageConfig{
				DSN:                          "some-dsn",
				MaxOpenConnections:           25,
				MaxIdleConnections:           5,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: true,
			errMsg:  "storage type is required",
		},
		{
			name: "empty DSN",
			config: StorageConfig{
				Type:                         "postgres",
				MaxOpenConnections:           25,
				MaxIdleConnections:           5,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: true,
			errMsg:  "storage DSN is required",
		},
		{
			name: "zero max open connections",
			config: StorageConfig{
				Type:                         "postgres",
				DSN:                          "some-dsn",
				MaxOpenConnections:           0,
				MaxIdleConnections:           5,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: true,
			errMsg:  "max open connections must be positive",
		},
		{
			name: "negative max idle connections",
			config: StorageConfig{
				Type:                         "postgres",
				DSN:                          "some-dsn",
				MaxOpenConnections:           25,
				MaxIdleConnections:           -1,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: true,
			errMsg:  "max idle connections cannot be negative",
		},
		{
			name: "idle connections exceed open connections",
			config: StorageConfig{
				Type:                         "postgres",
				DSN:                          "some-dsn",
				MaxOpenConnections:           10,
				MaxIdleConnections:           15,
				ConnectionMaxLifetimeMinutes: 5,
			},
			wantErr: true,
			errMsg:  "max idle connections (15) cannot exceed max open connections (10)",
		},
		{
			name: "negative connection lifetime",
			config: StorageConfig{
				Type:                         "postgres",
				DSN:                          "some-dsn",
				MaxOpenConnections:           25,
				MaxIdleConnections:           5,
				ConnectionMaxLifetimeMinutes: -1,
			},
			wantErr: true,
			errMsg:  "connection max lifetime cannot be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("StorageConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("StorageConfig.Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestTelemetryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  TelemetryConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid disabled telemetry",
			config: TelemetryConfig{
				Enabled:        false,
				ServiceName:    "guvnor",
				ServiceVersion: "1.0.0",
				SampleRate:     0.1,
			},
			wantErr: false,
		},
		{
			name: "valid enabled telemetry",
			config: TelemetryConfig{
				Enabled:        true,
				ServiceName:    "guvnor",
				ServiceVersion: "1.0.0",
				Endpoint:       "http://localhost:14268",
				SampleRate:     0.5,
			},
			wantErr: false,
		},
		{
			name: "empty service name",
			config: TelemetryConfig{
				ServiceVersion: "1.0.0",
				SampleRate:     0.1,
			},
			wantErr: true,
			errMsg:  "service name is required",
		},
		{
			name: "empty service version",
			config: TelemetryConfig{
				ServiceName: "guvnor",
				SampleRate:  0.1,
			},
			wantErr: true,
			errMsg:  "service version is required",
		},
		{
			name: "sample rate too low",
			config: TelemetryConfig{
				ServiceName:    "guvnor",
				ServiceVersion: "1.0.0",
				SampleRate:     -0.1,
			},
			wantErr: true,
			errMsg:  "sample rate must be between 0.0 and 1.0",
		},
		{
			name: "sample rate too high",
			config: TelemetryConfig{
				ServiceName:    "guvnor",
				ServiceVersion: "1.0.0",
				SampleRate:     1.5,
			},
			wantErr: true,
			errMsg:  "sample rate must be between 0.0 and 1.0",
		},
		{
			name: "enabled without endpoint",
			config: TelemetryConfig{
				Enabled:        true,
				ServiceName:    "guvnor",
				ServiceVersion: "1.0.0",
				SampleRate:     0.1,
			},
			wantErr: true,
			errMsg:  "telemetry endpoint is required when telemetry is enabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("TelemetryConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("TelemetryConfig.Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "invalid server config",
			config: &Config{
				Server: ServerConfig{
					Port: -1, // Invalid
				},
				Storage:   DefaultConfig().Storage,
				Telemetry: DefaultConfig().Telemetry,
			},
			wantErr: true,
			errMsg:  "server configuration invalid",
		},
		{
			name: "invalid storage config",
			config: &Config{
				Server: DefaultConfig().Server,
				Storage: StorageConfig{
					Type: "", // Invalid
				},
				Telemetry: DefaultConfig().Telemetry,
			},
			wantErr: true,
			errMsg:  "storage configuration invalid",
		},
		{
			name: "invalid telemetry config",
			config: &Config{
				Server:  DefaultConfig().Server,
				Storage: DefaultConfig().Storage,
				Telemetry: TelemetryConfig{
					ServiceName: "", // Invalid
				},
			},
			wantErr: true,
			errMsg:  "telemetry configuration invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Config.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !contains(err.Error(), tt.errMsg) {
				t.Errorf("Config.Validate() error = %v, should contain %v", err.Error(), tt.errMsg)
			}
		})
	}
}

// Helper function to clear GUVNOR environment variables
func clearGuvnorEnvVars() {
	for _, env := range os.Environ() {
		if len(env) > 7 && env[:7] == "GUVNOR_" {
			parts := strings.SplitN(env, "=", 2)
			if len(parts) > 0 {
				os.Unsetenv(parts[0])
			}
		}
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
