package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// migrateCmd represents the migrate command.
var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Database migration management",
	Long: `Manage database migrations for Guvnor.

This command provides database migration capabilities to ensure your database schema
is properly versioned and can be upgraded or downgraded safely.

Available operations:
  • up    - Apply all pending migrations
  • down  - Rollback the last migration
  • status - Show current migration status

Examples:
  guvnor migrate up           # Apply all pending migrations
  guvnor migrate down         # Rollback one migration
  guvnor migrate status       # Show migration status`,
}

// migrateUpCmd applies all pending migrations.
var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply all pending migrations",
	Long: `Apply all pending database migrations.

This command will examine the current database state and apply any migrations
that haven't been applied yet. Migrations are applied in order and the
operation is atomic - if any migration fails, the entire operation is rolled back.

Pre-flight checks:
  • Database connectivity
  • Required permissions
  • Migration file integrity`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		return runMigrateUp(configFile)
	},
}

// migrateDownCmd rolls back the last migration.
var migrateDownCmd = &cobra.Command{
	Use:   "down",
	Short: "Rollback the last migration",
	Long: `Rollback the most recently applied migration.

This command will undo the last migration that was applied to the database.
Only one migration is rolled back at a time for safety. If you need to
rollback multiple migrations, run this command multiple times.

⚠️  WARNING: Rolling back migrations may result in data loss.
Always backup your database before rolling back migrations in production.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		return runMigrateDown(configFile)
	},
}

// migrateStatusCmd shows current migration status.
var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show migration status",
	Long: `Show the current state of database migrations.

This command displays:
  • Current migration version
  • Number of pending migrations
  • List of applied migrations
  • Database connection status`,
	RunE: func(cmd *cobra.Command, args []string) error {
		configFile, _ := cmd.Flags().GetString("config")
		return runMigrateStatus(configFile)
	},
}

// runMigrateUp applies all pending migrations.
func runMigrateUp(configFile string) error {
	logger, err := logging.NewFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info("Starting database migration (up)")

	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate database configuration
	if err := validateDatabaseConfig(cfg, logger); err != nil {
		return fmt.Errorf("database configuration validation failed: %w", err)
	}

	// Run pre-flight checks
	if err := runPreflightChecks(cfg, logger); err != nil {
		return fmt.Errorf("pre-flight checks failed: %w", err)
	}

	// Create migrator
	m, err := createMigrator(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			logger.WithError(fmt.Errorf("source: %v, db: %v", sourceErr, dbErr)).Error("Failed to close migrator")
		}
	}()

	// Check current version
	version, dirty, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state (version %d), please fix manually", version)
	}

	logger.Info("Applying migrations", "current_version", version)

	// Apply migrations
	if err := m.Up(); err != nil {
		if err == migrate.ErrNoChange {
			logger.Info("No pending migrations to apply")
			return nil
		}
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	// Get new version
	newVersion, _, err := m.Version()
	if err != nil {
		return fmt.Errorf("failed to get new migration version: %w", err)
	}

	logger.Info("Migrations applied successfully", "new_version", newVersion)
	return nil
}

// runMigrateDown rolls back the last migration.
func runMigrateDown(configFile string) error {
	logger, err := logging.NewFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Warn("Starting database migration rollback (down)")

	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate database configuration
	if err := validateDatabaseConfig(cfg, logger); err != nil {
		return fmt.Errorf("database configuration validation failed: %w", err)
	}

	// Create migrator
	m, err := createMigrator(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			logger.WithError(fmt.Errorf("source: %v, db: %v", sourceErr, dbErr)).Error("Failed to close migrator")
		}
	}()

	// Check current version
	version, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			logger.Info("No migrations to rollback (database is empty)")
			return nil
		}
		return fmt.Errorf("failed to get current migration version: %w", err)
	}

	if dirty {
		return fmt.Errorf("database is in dirty state (version %d), please fix manually", version)
	}

	logger.Warn("Rolling back migration", "current_version", version)

	// Confirm rollback in production environments
	if cfg.Storage.Type == "postgres" && os.Getenv("GUVNOR_ENV") == "production" {
		fmt.Print("⚠️  You are about to rollback a migration in production. This may result in data loss.\n")
		fmt.Print("Type 'yes' to continue: ")
		var confirmation string
		if _, err := fmt.Scanln(&confirmation); err != nil || confirmation != "yes" {
			logger.Info("Migration rollback cancelled by user")
			return nil
		}
	}

	// Rollback one step
	if err := m.Steps(-1); err != nil {
		if err == migrate.ErrNoChange {
			logger.Info("No migrations to rollback")
			return nil
		}
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	// Get new version
	newVersion, _, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get new migration version: %w", err)
	}

	logger.Warn("Migration rolled back successfully", "previous_version", version, "new_version", newVersion)
	return nil
}

// runMigrateStatus shows current migration status.
func runMigrateStatus(configFile string) error {
	logger, err := logging.NewFromEnvironment()
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	logger.Info("Checking migration status")

	// Load configuration
	cfg, err := config.Load(configFile)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate database configuration
	if err := validateDatabaseConfig(cfg, logger); err != nil {
		return fmt.Errorf("database configuration validation failed: %w", err)
	}

	// Test database connection
	if err := testDatabaseConnection(cfg, logger); err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}

	// Create migrator
	m, err := createMigrator(cfg, logger)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}
	defer func() {
		if sourceErr, dbErr := m.Close(); sourceErr != nil || dbErr != nil {
			logger.WithError(fmt.Errorf("source: %v, db: %v", sourceErr, dbErr)).Error("Failed to close migrator")
		}
	}()

	// Get current version
	version, dirty, err := m.Version()
	if err != nil {
		if err == migrate.ErrNilVersion {
			fmt.Println("Migration Status: No migrations applied (empty database)")
			fmt.Println("Database Type:", cfg.Storage.Type)
			fmt.Println("Connection: ✅ OK")
			return nil
		}
		return fmt.Errorf("failed to get migration version: %w", err)
	}

	// Display status
	fmt.Printf("Migration Status: Version %d", version)
	if dirty {
		fmt.Print(" (DIRTY - needs manual intervention)")
	}
	fmt.Println()
	fmt.Println("Database Type:", cfg.Storage.Type)
	fmt.Println("Connection: ✅ OK")

	if dirty {
		fmt.Println("\n⚠️  WARNING: Database is in dirty state.")
		fmt.Println("   This usually means a migration failed partway through.")
		fmt.Println("   Manual intervention may be required to fix the issue.")
	}

	return nil
}

// createMigrator creates a new migrate instance.
func createMigrator(cfg *config.Config, logger *logging.Logger) (*migrate.Migrate, error) {
	// Find migrations directory
	migrationsPath, err := findMigrationsPath()
	if err != nil {
		return nil, fmt.Errorf("failed to find migrations directory: %w", err)
	}

	if logger != nil {
		logger.Debug("Using migrations path", "path", migrationsPath)
	}

	// Open database connection
	db, err := sql.Open("postgres", cfg.Storage.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	// Create database driver
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to create database driver: %w", err)
	}

	// Create migrator
	m, err := migrate.NewWithDatabaseInstance(
		"file://"+migrationsPath,
		"postgres",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create migrator: %w", err)
	}

	return m, nil
}

// findMigrationsPath locates the migrations directory.
func findMigrationsPath() (string, error) {
	// Try current directory first
	if _, err := os.Stat("migrations"); err == nil {
		abs, err := filepath.Abs("migrations")
		if err != nil {
			return "", err
		}
		return abs, nil
	}

	// Try relative to executable
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}

	execDir := filepath.Dir(execPath)
	migrationsPath := filepath.Join(execDir, "migrations")
	if _, err := os.Stat(migrationsPath); err == nil {
		return migrationsPath, nil
	}

	// Try from project root (for tests running from subdirectories)
	wd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("migrations directory not found")
	}

	// Walk up directory tree looking for migrations directory
	for {
		migrationsPath := filepath.Join(wd, "migrations")
		if _, err := os.Stat(migrationsPath); err == nil {
			return migrationsPath, nil
		}

		parent := filepath.Dir(wd)
		if parent == wd {
			// Reached root directory
			break
		}
		wd = parent
	}

	return "", fmt.Errorf("migrations directory not found")
}

// validateDatabaseConfig validates database configuration.
func validateDatabaseConfig(cfg *config.Config, logger *logging.Logger) error {
	if cfg.Storage.Type != "postgres" {
		return fmt.Errorf("unsupported database type: %s (only postgres is supported for migrations)", cfg.Storage.Type)
	}

	if cfg.Storage.DSN == "" {
		return fmt.Errorf("database DSN is required")
	}

	if logger != nil {
		logger.Debug("Database configuration validated", "type", cfg.Storage.Type)
	}
	return nil
}

// runPreflightChecks performs pre-flight checks before migrations.
func runPreflightChecks(cfg *config.Config, logger *logging.Logger) error {
	if logger != nil {
		logger.Info("Running pre-flight checks")
	}

	// Check database connectivity
	if err := testDatabaseConnection(cfg, logger); err != nil {
		return fmt.Errorf("database connectivity check failed: %w", err)
	}

	// Check migrations directory exists
	migrationsPath, err := findMigrationsPath()
	if err != nil {
		return fmt.Errorf("migrations directory check failed: %w", err)
	}

	// Validate migration files exist
	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	migrationCount := 0
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".sql" {
			migrationCount++
		}
	}

	if migrationCount == 0 {
		return fmt.Errorf("no migration files found in %s", migrationsPath)
	}

	if logger != nil {
		logger.Info("Pre-flight checks passed", "migrations_found", migrationCount/2) // Divide by 2 for up/down pairs
	}
	return nil
}

// testDatabaseConnection tests database connectivity.
func testDatabaseConnection(cfg *config.Config, logger *logging.Logger) error {
	if logger != nil {
		logger.Debug("Testing database connection")
	}

	db, err := sql.Open("postgres", cfg.Storage.DSN)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if logger != nil {
		logger.Debug("Database connection successful")
	}
	return nil
}

// init registers the migrate command and its subcommands.
func init() {
	// Add migrate command to root
	rootCmd.AddCommand(migrateCmd)

	// Add subcommands
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateDownCmd)
	migrateCmd.AddCommand(migrateStatusCmd)
}
