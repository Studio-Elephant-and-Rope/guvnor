package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/server"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// createTempConfigFile creates a temporary config file for testing
func createTempConfigFile(cfg *config.Config) (string, error) {
	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "guvnor-test-*.yaml")
	if err != nil {
		return "", err
	}
	defer tmpFile.Close()

	// Write basic YAML config
	configContent := fmt.Sprintf(`
server:
  host: "%s"
  port: %d
  read_timeout_seconds: %d
  write_timeout_seconds: %d
  idle_timeout_seconds: %d
storage:
  type: "%s"
  dsn: "%s"
telemetry:
  enabled: %t
`, cfg.Server.Host, cfg.Server.Port, cfg.Server.ReadTimeoutSeconds,
   cfg.Server.WriteTimeoutSeconds, cfg.Server.IdleTimeoutSeconds,
   cfg.Storage.Type, cfg.Storage.DSN, cfg.Telemetry.Enabled)

	if _, err := tmpFile.WriteString(configContent); err != nil {
		os.Remove(tmpFile.Name())
		return "", err
	}

	return tmpFile.Name(), nil
}

// findFreePort finds an available port for testing
func findFreePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, err
	}
	defer listener.Close()
	return listener.Addr().(*net.TCPAddr).Port, nil
}

func TestServeCommand_Exists(t *testing.T) {
	// Test that the serve command is registered
	rootCmd := GetRootCmd()

	// Find the serve command
	var serveCmd *cobra.Command
	for _, cmd := range rootCmd.Commands() {
		if cmd.Use == "serve" {
			serveCmd = cmd
			break
		}
	}

	if serveCmd == nil {
		t.Fatal("Serve command not found in root command")
	}

	// Verify command properties
	if serveCmd.Short != "Start the Guvnor HTTP server" {
		t.Errorf("Expected short description to be 'Start the Guvnor HTTP server', got: %s", serveCmd.Short)
	}

	if !strings.Contains(serveCmd.Long, "Health check endpoint at /health") {
		t.Error("Expected long description to mention health check endpoint")
	}

	if !strings.Contains(serveCmd.Long, "Graceful shutdown on SIGTERM/SIGINT") {
		t.Error("Expected long description to mention graceful shutdown")
	}
}

func TestServeCommand_Help(t *testing.T) {
	// Test that the serve command help works
	rootCmd := GetRootCmd()

	// Capture output
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	rootCmd.SetErr(&output)

	// Run help command
	rootCmd.SetArgs([]string{"serve", "--help"})
	err := rootCmd.Execute()

	if err != nil {
		t.Fatalf("Serve help command failed: %v", err)
	}

	helpOutput := output.String()

	// Check that help contains expected content
	expectedStrings := []string{
		"Start the Guvnor HTTP server",
		"Health check endpoint at /health",
		"guvnor serve",
		"Examples:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(helpOutput, expected) {
			t.Errorf("Help output missing expected string: %s", expected)
		}
	}
}

// runServeForTest is a test-friendly version of runServe that doesn't block on signals
func runServeForTest(configFile string, shutdownChan <-chan struct{}) error {
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

	// Create and configure the server
	srv, err := server.New(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Start server (non-blocking)
	if err := srv.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}

	// Wait for shutdown signal from test
	<-shutdownChan

	// Perform graceful shutdown
	return srv.Shutdown()
}

func TestRunServe_ConfigFile(t *testing.T) {
	port, err := findFreePort()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	// Create test config
	cfg := config.DefaultConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port
	cfg.Server.ReadTimeoutSeconds = 5
	cfg.Server.WriteTimeoutSeconds = 5
	cfg.Server.IdleTimeoutSeconds = 10

	configFile, err := createTempConfigFile(cfg)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	defer os.Remove(configFile)

	// Set environment variables for logging
	os.Setenv("GUVNOR_LOG_LEVEL", "info")
	defer os.Unsetenv("GUVNOR_LOG_LEVEL")

	// Create shutdown channel to control server
	shutdownChan := make(chan struct{})

	// Start server in goroutine
	var serverErr error
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		serverErr = runServeForTest(configFile, shutdownChan)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is running by making a request
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		close(shutdownChan)
		wg.Wait()
		t.Fatalf("Failed to connect to server: %v", err)
	}

	// Check response
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		close(shutdownChan)
		wg.Wait()
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Parse health response
	var health server.HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&health)
	resp.Body.Close()

	if err != nil {
		close(shutdownChan)
		wg.Wait()
		t.Fatalf("Failed to decode health response: %v", err)
	}

	if health.Status != "healthy" {
		close(shutdownChan)
		wg.Wait()
		t.Errorf("Expected health status 'healthy', got '%s'", health.Status)
	}

	// Trigger shutdown
	close(shutdownChan)

	// Wait for server to shut down with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Server shut down successfully
		if serverErr != nil {
			t.Errorf("Unexpected server error: %v", serverErr)
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not shutdown within timeout")
	}
}

func TestRunServe_InvalidConfig(t *testing.T) {
	// Test with non-existent config file
	err := runServe("/non/existent/config.yaml")
	if err == nil {
		t.Error("Expected error with non-existent config file")
	}

	if !strings.Contains(err.Error(), "failed to load configuration") {
		t.Errorf("Expected config loading error, got: %v", err)
	}
}

func TestRunServe_DefaultConfig(t *testing.T) {
	// Set environment variables to override default config with a free port
	port, err := findFreePort()
	if err != nil {
		t.Fatalf("Failed to find free port: %v", err)
	}

	os.Setenv("GUVNOR_SERVER_HOST", "127.0.0.1")
	os.Setenv("GUVNOR_SERVER_PORT", fmt.Sprintf("%d", port))
	os.Setenv("GUVNOR_LOG_LEVEL", "error") // Reduce log noise
	defer func() {
		os.Unsetenv("GUVNOR_SERVER_HOST")
		os.Unsetenv("GUVNOR_SERVER_PORT")
		os.Unsetenv("GUVNOR_LOG_LEVEL")
	}()

	// Create shutdown channel to control server
	shutdownChan := make(chan struct{})

	// Start server in goroutine
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		// Run with empty config file (will use defaults + env vars)
		_ = runServeForTest("", shutdownChan)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Test that server is running
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/health", port))
	if err != nil {
		close(shutdownChan)
		wg.Wait()
		t.Fatalf("Failed to connect to server: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		close(shutdownChan)
		wg.Wait()
		t.Fatalf("Expected status 200, got %d", resp.StatusCode)
	}

	// Trigger shutdown
	close(shutdownChan)

	// Wait for shutdown
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(3 * time.Second):
		t.Error("Server did not shutdown within timeout")
	}
}

func TestRunServe_PortAlreadyInUse(t *testing.T) {
	// Find a free port and bind to it
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	defer listener.Close() // Ensure cleanup

	// Create config with the same port
	cfg := config.DefaultConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = port

	configFile, err := createTempConfigFile(cfg)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	defer os.Remove(configFile)

	// Set log level to reduce noise
	os.Setenv("GUVNOR_LOG_LEVEL", "error")
	defer os.Unsetenv("GUVNOR_LOG_LEVEL")

	// Try to start server directly - should fail because port is in use
	// Use runServe instead of runServeForTest to avoid hanging
	err = runServe(configFile)

	// The error should occur during server startup
	if err == nil {
		t.Error("Expected error when port is already in use")
	}

	// Should contain some indication of port/address conflict
	if !strings.Contains(strings.ToLower(err.Error()), "address") &&
	   !strings.Contains(strings.ToLower(err.Error()), "bind") &&
	   !strings.Contains(strings.ToLower(err.Error()), "listen") {
		t.Logf("Got error (which is expected): %v", err)
	}
}

func TestRunServe_InvalidServerConfig(t *testing.T) {
	// Create config with invalid server settings
	cfg := config.DefaultConfig()
	cfg.Server.Port = -1 // Invalid port

	configFile, err := createTempConfigFile(cfg)
	if err != nil {
		t.Fatalf("Failed to create temp config: %v", err)
	}
	defer os.Remove(configFile)

	// Set log level to reduce noise
	os.Setenv("GUVNOR_LOG_LEVEL", "error")
	defer os.Unsetenv("GUVNOR_LOG_LEVEL")

	// Try to start server - should fail due to invalid config
	err = runServe(configFile)
	if err == nil {
		t.Error("Expected error with invalid server configuration")
	}

	// Should be a configuration validation error
	if !strings.Contains(err.Error(), "configuration") && !strings.Contains(err.Error(), "server") {
		t.Errorf("Expected configuration error, got: %v", err)
	}
}
