// Package config provides configuration management for the Guvnor incident management platform.
//
// This package handles loading configuration from multiple sources with proper precedence:
// 1. Environment variables (GUVNOR_*)
// 2. Configuration file (YAML)
// 3. Default values
//
// The configuration system uses Viper for flexible configuration management,
// supporting various formats and sources while maintaining backwards compatibility.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config represents the complete application configuration.
//
// This is the root configuration structure that contains all subsystem
// configurations. Each subsystem should have its own configuration struct
// to maintain separation of concerns.
type Config struct {
	Server        ServerConfig       `mapstructure:"server" yaml:"server"`
	Storage       StorageConfig      `mapstructure:"storage" yaml:"storage"`
	Telemetry     TelemetryConfig    `mapstructure:"telemetry" yaml:"telemetry"`
	Notifications NotificationConfig `mapstructure:"notifications" yaml:"notifications"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	// Host is the interface to bind the server to
	Host string `mapstructure:"host" yaml:"host"`
	// Port is the port number to listen on
	Port int `mapstructure:"port" yaml:"port"`
	// ReadTimeout is the maximum duration for reading the entire request
	ReadTimeoutSeconds int `mapstructure:"read_timeout_seconds" yaml:"read_timeout_seconds"`
	// WriteTimeout is the maximum duration before timing out writes
	WriteTimeoutSeconds int `mapstructure:"write_timeout_seconds" yaml:"write_timeout_seconds"`
	// IdleTimeout is the maximum time to wait for the next request when keep-alives are enabled
	IdleTimeoutSeconds int `mapstructure:"idle_timeout_seconds" yaml:"idle_timeout_seconds"`
}

// StorageConfig contains database and storage configuration.
type StorageConfig struct {
	// Type specifies the storage backend (postgres, sqlite, etc.)
	Type string `mapstructure:"type" yaml:"type"`
	// DSN is the data source name / connection string
	DSN string `mapstructure:"dsn" yaml:"dsn"`
	// MaxOpenConnections is the maximum number of open connections to the database
	MaxOpenConnections int `mapstructure:"max_open_connections" yaml:"max_open_connections"`
	// MaxIdleConnections is the maximum number of connections in the idle connection pool
	MaxIdleConnections int `mapstructure:"max_idle_connections" yaml:"max_idle_connections"`
	// ConnectionMaxLifetimeMinutes is the maximum time a connection may be reused
	ConnectionMaxLifetimeMinutes int `mapstructure:"connection_max_lifetime_minutes" yaml:"connection_max_lifetime_minutes"`
}

// TelemetryConfig contains observability configuration.
type TelemetryConfig struct {
	// Enabled determines whether telemetry is active
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// ServiceName is the name used to identify this service in traces
	ServiceName string `mapstructure:"service_name" yaml:"service_name"`
	// ServiceVersion is the version of this service for telemetry
	ServiceVersion string `mapstructure:"service_version" yaml:"service_version"`
	// Endpoint is the OpenTelemetry collector endpoint
	Endpoint string `mapstructure:"endpoint" yaml:"endpoint"`
	// SampleRate is the fraction of traces to sample (0.0 to 1.0)
	SampleRate float64 `mapstructure:"sample_rate" yaml:"sample_rate"`

	// Receiver configuration for ingesting signals
	Receiver ReceiverConfig `mapstructure:"receiver" yaml:"receiver"`
}

// ReceiverConfig configures the OpenTelemetry receiver endpoints.
type ReceiverConfig struct {
	// Enabled determines whether the OTLP receiver is active
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
	// GRPCPort is the port for OTLP gRPC receiver (default: 4317)
	GRPCPort int `mapstructure:"grpc_port" yaml:"grpc_port"`
	// HTTPPort is the port for OTLP HTTP receiver (default: 4318)
	HTTPPort int `mapstructure:"http_port" yaml:"http_port"`
	// GRPCHost is the host for OTLP gRPC receiver
	GRPCHost string `mapstructure:"grpc_host" yaml:"grpc_host"`
	// HTTPHost is the host for OTLP HTTP receiver
	HTTPHost string `mapstructure:"http_host" yaml:"http_host"`
}

// NotificationConfig contains notification configuration.
type NotificationConfig struct {
	// Webhooks is a list of webhook endpoints to send notifications to
	Webhooks []WebhookConfig `mapstructure:"webhooks" yaml:"webhooks"`
}

// WebhookConfig contains configuration for a single webhook endpoint.
type WebhookConfig struct {
	// URL is the webhook endpoint URL
	URL string `mapstructure:"url" yaml:"url"`
	// Secret is the shared secret for HMAC signature verification
	Secret string `mapstructure:"secret" yaml:"secret"`
	// Headers are additional HTTP headers to include in requests
	Headers map[string]string `mapstructure:"headers" yaml:"headers"`
	// Timeout is the request timeout duration (default: 30s)
	Timeout string `mapstructure:"timeout" yaml:"timeout"`
	// MaxRetries is the maximum number of retry attempts (default: 3)
	MaxRetries int `mapstructure:"max_retries" yaml:"max_retries"`
	// Enabled determines if this webhook is active
	Enabled bool `mapstructure:"enabled" yaml:"enabled"`
}

// DefaultConfig returns a configuration with sensible defaults.
//
// These defaults are suitable for development and testing environments.
// Production deployments should override these values through configuration
// files or environment variables.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Host:                "0.0.0.0",
			Port:                8080,
			ReadTimeoutSeconds:  30,
			WriteTimeoutSeconds: 30,
			IdleTimeoutSeconds:  120,
		},
		Storage: StorageConfig{
			Type:                         "sqlite",
			DSN:                          "file:guvnor.db?cache=shared&mode=rwc",
			MaxOpenConnections:           25,
			MaxIdleConnections:           5,
			ConnectionMaxLifetimeMinutes: 5,
		},
		Telemetry: TelemetryConfig{
			Enabled:        false,
			ServiceName:    "guvnor",
			ServiceVersion: "development",
			Endpoint:       "",
			SampleRate:     0.1,
			Receiver: ReceiverConfig{
				Enabled:  false,
				GRPCPort: 4317,
				HTTPPort: 4318,
				GRPCHost: "0.0.0.0",
				HTTPHost: "0.0.0.0",
			},
		},
		Notifications: NotificationConfig{
			Webhooks: []WebhookConfig{},
		},
	}
}

// Load loads configuration from multiple sources with proper precedence.
//
// Configuration is loaded in this order of precedence:
//  1. Environment variables (GUVNOR_*)
//  2. Configuration file (if specified)
//  3. Default values
//
// Returns a fully populated Config struct or an error if loading fails.
func Load(configFile string) (*Config, error) {
	// Start with default configuration
	config := DefaultConfig()

	// Create a new viper instance
	v := viper.New()

	// Set up environment variable handling
	v.SetEnvPrefix("GUVNOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Configure viper with our defaults
	if err := setDefaults(v, config); err != nil {
		return nil, fmt.Errorf("failed to set default configuration: %w", err)
	}

	// Load configuration file if specified
	if configFile != "" {
		if err := loadConfigFile(v, configFile); err != nil {
			return nil, fmt.Errorf("failed to load config file %s: %w", configFile, err)
		}
	}

	// Unmarshal into our config struct
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Validate the configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// setDefaults configures viper with default values from our config struct.
func setDefaults(v *viper.Viper, config *Config) error {
	// Server defaults
	v.SetDefault("server.host", config.Server.Host)
	v.SetDefault("server.port", config.Server.Port)
	v.SetDefault("server.read_timeout_seconds", config.Server.ReadTimeoutSeconds)
	v.SetDefault("server.write_timeout_seconds", config.Server.WriteTimeoutSeconds)
	v.SetDefault("server.idle_timeout_seconds", config.Server.IdleTimeoutSeconds)

	// Storage defaults
	v.SetDefault("storage.type", config.Storage.Type)
	v.SetDefault("storage.dsn", config.Storage.DSN)
	v.SetDefault("storage.max_open_connections", config.Storage.MaxOpenConnections)
	v.SetDefault("storage.max_idle_connections", config.Storage.MaxIdleConnections)
	v.SetDefault("storage.connection_max_lifetime_minutes", config.Storage.ConnectionMaxLifetimeMinutes)

	// Telemetry defaults
	v.SetDefault("telemetry.enabled", config.Telemetry.Enabled)
	v.SetDefault("telemetry.service_name", config.Telemetry.ServiceName)
	v.SetDefault("telemetry.service_version", config.Telemetry.ServiceVersion)
	v.SetDefault("telemetry.endpoint", config.Telemetry.Endpoint)
	v.SetDefault("telemetry.sample_rate", config.Telemetry.SampleRate)

	// Telemetry receiver defaults
	v.SetDefault("telemetry.receiver.enabled", config.Telemetry.Receiver.Enabled)
	v.SetDefault("telemetry.receiver.grpc_port", config.Telemetry.Receiver.GRPCPort)
	v.SetDefault("telemetry.receiver.http_port", config.Telemetry.Receiver.HTTPPort)
	v.SetDefault("telemetry.receiver.grpc_host", config.Telemetry.Receiver.GRPCHost)
	v.SetDefault("telemetry.receiver.http_host", config.Telemetry.Receiver.HTTPHost)

	// Notification defaults
	v.SetDefault("notifications.webhooks", config.Notifications.Webhooks)

	return nil
}

// loadConfigFile loads configuration from a YAML file.
func loadConfigFile(v *viper.Viper, configFile string) error {
	// Check if file exists
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return fmt.Errorf("configuration file does not exist: %s", configFile)
	}

	// Set config file path
	v.SetConfigFile(configFile)

	// Read configuration
	if err := v.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read configuration file: %w", err)
	}

	return nil
}

// Validate checks that the configuration is valid and complete.
//
// This method performs comprehensive validation of all configuration values,
// ensuring they are within acceptable ranges and that required fields are set.
//
// Returns an error describing the first validation failure encountered.
func (c *Config) Validate() error {
	// Validate server configuration
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server configuration invalid: %w", err)
	}

	// Validate storage configuration
	if err := c.Storage.Validate(); err != nil {
		return fmt.Errorf("storage configuration invalid: %w", err)
	}

	// Validate telemetry configuration
	if err := c.Telemetry.Validate(); err != nil {
		return fmt.Errorf("telemetry configuration invalid: %w", err)
	}

	// Validate notification configuration
	if err := c.Notifications.Validate(); err != nil {
		return fmt.Errorf("notification configuration invalid: %w", err)
	}

	return nil
}

// Validate checks server configuration for validity.
func (s *ServerConfig) Validate() error {
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", s.Port)
	}

	if s.ReadTimeoutSeconds < 1 {
		return fmt.Errorf("read timeout must be positive, got %d", s.ReadTimeoutSeconds)
	}

	if s.WriteTimeoutSeconds < 1 {
		return fmt.Errorf("write timeout must be positive, got %d", s.WriteTimeoutSeconds)
	}

	if s.IdleTimeoutSeconds < 1 {
		return fmt.Errorf("idle timeout must be positive, got %d", s.IdleTimeoutSeconds)
	}

	return nil
}

// Validate checks storage configuration for validity.
func (s *StorageConfig) Validate() error {
	if s.Type == "" {
		return fmt.Errorf("storage type is required")
	}

	if s.DSN == "" {
		return fmt.Errorf("storage DSN is required")
	}

	if s.MaxOpenConnections < 1 {
		return fmt.Errorf("max open connections must be positive, got %d", s.MaxOpenConnections)
	}

	if s.MaxIdleConnections < 0 {
		return fmt.Errorf("max idle connections cannot be negative, got %d", s.MaxIdleConnections)
	}

	if s.MaxIdleConnections > s.MaxOpenConnections {
		return fmt.Errorf("max idle connections (%d) cannot exceed max open connections (%d)",
			s.MaxIdleConnections, s.MaxOpenConnections)
	}

	if s.ConnectionMaxLifetimeMinutes < 0 {
		return fmt.Errorf("connection max lifetime cannot be negative, got %d", s.ConnectionMaxLifetimeMinutes)
	}

	return nil
}

// Validate checks telemetry configuration for validity.
func (t *TelemetryConfig) Validate() error {
	if t.ServiceName == "" {
		return fmt.Errorf("service name is required")
	}

	if t.ServiceVersion == "" {
		return fmt.Errorf("service version is required")
	}

	if t.SampleRate < 0.0 || t.SampleRate > 1.0 {
		return fmt.Errorf("sample rate must be between 0.0 and 1.0, got %f", t.SampleRate)
	}

	// If telemetry is enabled, endpoint should be specified
	if t.Enabled && t.Endpoint == "" {
		return fmt.Errorf("telemetry endpoint is required when telemetry is enabled")
	}

	// Validate receiver configuration
	if err := t.Receiver.Validate(); err != nil {
		return fmt.Errorf("receiver configuration invalid: %w", err)
	}

	return nil
}

// Validate checks receiver configuration for validity.
func (r *ReceiverConfig) Validate() error {
	// Only validate ports if receiver is enabled
	if !r.Enabled {
		return nil
	}

	if r.GRPCPort < 1 || r.GRPCPort > 65535 {
		return fmt.Errorf("gRPC port must be between 1 and 65535, got %d", r.GRPCPort)
	}

	if r.HTTPPort < 1 || r.HTTPPort > 65535 {
		return fmt.Errorf("HTTP port must be between 1 and 65535, got %d", r.HTTPPort)
	}

	// Ports cannot be the same (avoid conflicts)
	if r.GRPCPort == r.HTTPPort {
		return fmt.Errorf("gRPC port (%d) and HTTP port (%d) cannot be the same", r.GRPCPort, r.HTTPPort)
	}

	// Host addresses are optional but must not be empty strings
	if r.GRPCHost == "" {
		return fmt.Errorf("gRPC host cannot be empty when receiver is enabled")
	}

	if r.HTTPHost == "" {
		return fmt.Errorf("HTTP host cannot be empty when receiver is enabled")
	}

	return nil
}

// Validate checks notification configuration for validity.
func (n *NotificationConfig) Validate() error {
	// Validate webhook configurations
	for i, webhook := range n.Webhooks {
		if err := webhook.Validate(); err != nil {
			return fmt.Errorf("webhook configuration at index %d invalid: %w", i, err)
		}
	}

	return nil
}

// Validate checks webhook configuration for validity.
func (w *WebhookConfig) Validate() error {
	if w.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	if w.MaxRetries < 0 {
		return fmt.Errorf("webhook max_retries cannot be negative")
	}

	if w.Timeout != "" {
		// Validate timeout is a valid duration string
		if _, err := time.ParseDuration(w.Timeout); err != nil {
			return fmt.Errorf("invalid webhook timeout duration: %w", err)
		}
	}

	return nil
}
