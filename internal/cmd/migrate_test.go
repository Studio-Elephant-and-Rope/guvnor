package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
)

// PostgreSQLContainer wraps a testcontainers PostgreSQL container.
type PostgreSQLContainer struct {
	testcontainers.Container
	DSN string
}

// setupPostgreSQLContainer creates a PostgreSQL test container.
func setupPostgreSQLContainer(ctx context.Context) (*PostgreSQLContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "postgres:15-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "guvnor_test",
			"POSTGRES_USER":     "guvnor_test",
			"POSTGRES_PASSWORD": "test_password",
		},
		WaitingFor: wait.ForAll(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2),
			wait.ForListeningPort("5432/tcp"),
		).WithDeadline(2 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		return nil, fmt.Errorf("failed to get container port: %w", err)
	}

	dsn := fmt.Sprintf("postgres://guvnor_test:test_password@%s:%s/guvnor_test?sslmode=disable", host, port.Port())

	return &PostgreSQLContainer{
		Container: container,
		DSN:       dsn,
	}, nil
}

// createTestConfig creates a test configuration with the given DSN.
func createTestConfig(dsn string) *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Storage: config.StorageConfig{
			Type: "postgres",
			DSN:  dsn,
		},
		Telemetry: config.TelemetryConfig{
			Enabled:        false,
			ServiceName:    "guvnor-test",
			ServiceVersion: "test",
		},
	}
}

// validateTableExists checks if a table exists in the database.
func validateTableExists(dsn, tableName string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)`

	var exists bool
	err = db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	return nil
}

// validateTableNotExists checks if a table does not exist in the database.
func validateTableNotExists(dsn, tableName string) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_schema = 'public'
			AND table_name = $1
		)`

	var exists bool
	err = db.QueryRow(query, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if exists {
		return fmt.Errorf("table %s should not exist but does", tableName)
	}

	return nil
}

// validateMigrationVersion checks the current migration version.
func validateMigrationVersion(dsn string, expectedVersion uint) error {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	query := `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`

	var version uint
	err = db.QueryRow(query).Scan(&version)
	if err != nil {
		if err == sql.ErrNoRows && expectedVersion == 0 {
			return nil // No migrations is expected
		}
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	if version != expectedVersion {
		return fmt.Errorf("expected migration version %d, got %d", expectedVersion, version)
	}

	return nil
}

// TestMigrationSystem tests the complete migration system.
func TestMigrationSystem(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup PostgreSQL container
	container, err := setupPostgreSQLContainer(ctx)
	if err != nil {
		t.Fatalf("Failed to setup PostgreSQL container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	// Create test configuration
	cfg := createTestConfig(container.DSN)

	// Validate database configuration
	if err := validateDatabaseConfig(cfg, nil); err != nil {
		t.Fatalf("Database configuration validation failed: %v", err)
	}

	// Test database connection
	if err := testDatabaseConnection(cfg, nil); err != nil {
		t.Fatalf("Database connection test failed: %v", err)
	}

	t.Run("Initial Migration Up", func(t *testing.T) {
		// Test migrating up from empty database
		if err := runMigrationTest(cfg, "up"); err != nil {
			t.Fatalf("Migration up failed: %v", err)
		}

		// Validate core tables exist
		coreTables := []string{
			"incidents", "events", "signals", "incident_signals",
			"schedules", "schedule_shifts", "notification_rules",
		}

		for _, table := range coreTables {
			if err := validateTableExists(container.DSN, table); err != nil {
				t.Errorf("Core table validation failed: %v", err)
			}
		}

		// Validate migration version (should be 2 as migrate up applies all pending migrations)
		if err := validateMigrationVersion(container.DSN, 2); err != nil {
			t.Errorf("Migration version validation failed: %v", err)
		}
	})

	t.Run("Second Migration Up", func(t *testing.T) {
		// Apply second migration
		if err := runMigrationTest(cfg, "up"); err != nil {
			t.Fatalf("Second migration up failed: %v", err)
		}

		// Validate additional tables exist
		additionalTables := []string{
			"schedule_rotations", "schedule_overrides", "escalation_policies",
			"escalation_steps", "audit_logs", "team_preferences",
		}

		for _, table := range additionalTables {
			if err := validateTableExists(container.DSN, table); err != nil {
				t.Errorf("Additional table validation failed: %v", err)
			}
		}

		// Validate migration version
		if err := validateMigrationVersion(container.DSN, 2); err != nil {
			t.Errorf("Migration version validation failed: %v", err)
		}
	})

	t.Run("Migration Status", func(t *testing.T) {
		// Test status command (we can't easily test the output, but we can test it doesn't error)
		if err := runMigrationTest(cfg, "status"); err != nil {
			t.Fatalf("Migration status failed: %v", err)
		}
	})

	t.Run("Migration Down", func(t *testing.T) {
		// Test rolling back one migration
		if err := runMigrationTest(cfg, "down"); err != nil {
			t.Fatalf("Migration down failed: %v", err)
		}

		// Validate additional tables are removed
		additionalTables := []string{
			"schedule_rotations", "schedule_overrides", "escalation_policies",
			"escalation_steps", "audit_logs", "team_preferences",
		}

		for _, table := range additionalTables {
			if err := validateTableNotExists(container.DSN, table); err != nil {
				t.Errorf("Table should be removed: %v", err)
			}
		}

		// Validate core tables still exist
		coreTables := []string{
			"incidents", "events", "signals", "incident_signals",
			"schedules", "schedule_shifts", "notification_rules",
		}

		for _, table := range coreTables {
			if err := validateTableExists(container.DSN, table); err != nil {
				t.Errorf("Core table should still exist: %v", err)
			}
		}

		// Validate migration version
		if err := validateMigrationVersion(container.DSN, 1); err != nil {
			t.Errorf("Migration version validation failed: %v", err)
		}
	})

	t.Run("Full Rollback", func(t *testing.T) {
		// Test rolling back the initial migration
		if err := runMigrationTest(cfg, "down"); err != nil {
			t.Fatalf("Full rollback failed: %v", err)
		}

		// Validate all tables are removed
		allTables := []string{
			"incidents", "events", "signals", "incident_signals",
			"schedules", "schedule_shifts", "notification_rules",
			"schedule_rotations", "schedule_overrides", "escalation_policies",
			"escalation_steps", "audit_logs", "team_preferences",
		}

		for _, table := range allTables {
			if err := validateTableNotExists(container.DSN, table); err != nil {
				t.Errorf("Table should be removed: %v", err)
			}
		}
	})
}

// TestPreflightChecks tests the pre-flight validation.
func TestPreflightChecks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup PostgreSQL container
	container, err := setupPostgreSQLContainer(ctx)
	if err != nil {
		t.Fatalf("Failed to setup PostgreSQL container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	cfg := createTestConfig(container.DSN)

	t.Run("Valid Configuration", func(t *testing.T) {
		if err := runPreflightChecks(cfg, nil); err != nil {
			t.Errorf("Pre-flight checks should pass with valid configuration: %v", err)
		}
	})

	t.Run("Invalid Database Type", func(t *testing.T) {
		invalidCfg := *cfg
		invalidCfg.Storage.Type = "mysql"

		if err := validateDatabaseConfig(&invalidCfg, nil); err == nil {
			t.Error("Should reject non-postgres database type")
		}
	})

	t.Run("Invalid DSN", func(t *testing.T) {
		invalidCfg := *cfg
		invalidCfg.Storage.DSN = "invalid-dsn"

		if err := testDatabaseConnection(&invalidCfg, nil); err == nil {
			t.Error("Should reject invalid DSN")
		}
	})
}

// TestMigrationPaths tests migration path discovery.
func TestMigrationPaths(t *testing.T) {
	t.Run("Current Directory", func(t *testing.T) {
		// Should find migrations in current directory
		originalWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(originalWd)

		// Change to project root where migrations directory exists
		projectRoot := filepath.Join(originalWd, "..", "..")
		if err := os.Chdir(projectRoot); err != nil {
			t.Fatalf("Failed to change to project root: %v", err)
		}

		path, err := findMigrationsPath()
		if err != nil {
			t.Errorf("Should find migrations directory: %v", err)
		}

		if !filepath.IsAbs(path) {
			t.Error("Should return absolute path")
		}
	})

	t.Run("Missing Directory", func(t *testing.T) {
		// Create temporary directory without migrations
		tempDir := t.TempDir()
		originalWd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Failed to get working directory: %v", err)
		}
		defer os.Chdir(originalWd)

		if err := os.Chdir(tempDir); err != nil {
			t.Fatalf("Failed to change to temp directory: %v", err)
		}

		_, err = findMigrationsPath()
		if err == nil {
			t.Error("Should fail when migrations directory doesn't exist")
		}
	})
}

// runMigrationTest is a helper to run migration commands for testing.
func runMigrationTest(cfg *config.Config, command string) error {
	// Save original config to temporary file
	tempFile, err := createTempConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create temp config: %w", err)
	}
	defer os.Remove(tempFile)

	switch command {
	case "up":
		return runMigrateUp(tempFile)
	case "down":
		// Set environment to non-production to skip confirmation
		os.Setenv("GUVNOR_ENV", "test")
		defer os.Unsetenv("GUVNOR_ENV")
		return runMigrateDown(tempFile)
	case "status":
		return runMigrateStatus(tempFile)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

// createTempConfig creates a temporary configuration file for testing.
func createTempConfig(cfg *config.Config) (string, error) {
	tempFile, err := os.CreateTemp("", "guvnor-test-config-*.yaml")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	// Write minimal config
	content := fmt.Sprintf(`
storage:
  type: %s
  dsn: %s
server:
  host: %s
  port: %d
telemetry:
  enabled: %t
  service_name: %s
  service_version: %s
`, cfg.Storage.Type, cfg.Storage.DSN, cfg.Server.Host, cfg.Server.Port,
		cfg.Telemetry.Enabled, cfg.Telemetry.ServiceName, cfg.Telemetry.ServiceVersion)

	if _, err := tempFile.WriteString(content); err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}

// TestMigrationIdempotency tests that migrations can be applied multiple times safely.
func TestMigrationIdempotency(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Setup PostgreSQL container
	container, err := setupPostgreSQLContainer(ctx)
	if err != nil {
		t.Fatalf("Failed to setup PostgreSQL container: %v", err)
	}
	defer func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}()

	cfg := createTestConfig(container.DSN)

	// Apply migrations first time
	if err := runMigrationTest(cfg, "up"); err != nil {
		t.Fatalf("First migration up failed: %v", err)
	}

	// Apply migrations second time - should be no-op
	if err := runMigrationTest(cfg, "up"); err != nil {
		t.Errorf("Second migration up should not fail: %v", err)
	}

	// Validate tables still exist and version is correct
	if err := validateTableExists(container.DSN, "incidents"); err != nil {
		t.Errorf("Tables should still exist after idempotent migration: %v", err)
	}

	if err := validateMigrationVersion(container.DSN, 2); err != nil {
		t.Errorf("Migration version should be correct after idempotent migration: %v", err)
	}
}
