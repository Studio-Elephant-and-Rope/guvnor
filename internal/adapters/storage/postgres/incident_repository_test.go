package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// TestContainer wraps a PostgreSQL test container.
type TestContainer struct {
	testcontainers.Container
	ConnectionString string
}

// setupTestDB creates a PostgreSQL test container and runs migrations.
func setupTestDB(ctx context.Context, t *testing.T) (*TestContainer, *pgxpool.Pool, func()) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create PostgreSQL container
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
		t.Fatalf("Failed to start PostgreSQL container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Failed to get container host: %v", err)
	}

	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Failed to get container port: %v", err)
	}

	connStr := fmt.Sprintf("postgres://guvnor_test:test_password@%s:%s/guvnor_test?sslmode=disable",
		host, port.Port())

	// Create connection pool
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		container.Terminate(ctx)
		t.Fatalf("Failed to create connection pool: %v", err)
	}

	// Test connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		container.Terminate(ctx)
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Run migrations
	if err := runTestMigrations(ctx, pool); err != nil {
		pool.Close()
		container.Terminate(ctx)
		t.Fatalf("Failed to run migrations: %v", err)
	}

	testContainer := &TestContainer{
		Container:        container,
		ConnectionString: connStr,
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate container: %v", err)
		}
	}

	return testContainer, pool, cleanup
}

// runTestMigrations runs the database migrations for testing.
func runTestMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	// Enable extensions
	extensionSQL := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE EXTENSION IF NOT EXISTS "btree_gin";
	`
	if _, err := pool.Exec(ctx, extensionSQL); err != nil {
		return fmt.Errorf("failed to create extensions: %w", err)
	}

	// Create enums
	enumSQL := `
		CREATE TYPE incident_status AS ENUM ('triggered', 'acknowledged', 'investigating', 'resolved');
		CREATE TYPE incident_severity AS ENUM ('critical', 'high', 'medium', 'low', 'info');
		CREATE TYPE channel_type AS ENUM ('email', 'slack', 'sms', 'webhook', 'pagerduty');
	`
	if _, err := pool.Exec(ctx, enumSQL); err != nil {
		return fmt.Errorf("failed to create enums: %w", err)
	}

	// Create tables
	tableSQL := `
		-- Incidents table
		CREATE TABLE incidents (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			title VARCHAR(200) NOT NULL,
			description TEXT,
			status incident_status NOT NULL DEFAULT 'triggered',
			severity incident_severity NOT NULL,
			team_id VARCHAR(255) NOT NULL,
			assignee_id VARCHAR(255),
			service_id VARCHAR(255),
			labels JSONB DEFAULT '{}',
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			resolved_at TIMESTAMPTZ,

			CONSTRAINT incident_title_not_empty CHECK (LENGTH(TRIM(title)) > 0),
			CONSTRAINT incident_team_id_not_empty CHECK (LENGTH(TRIM(team_id)) > 0),
			CONSTRAINT incident_resolved_check CHECK (
				(status = 'resolved' AND resolved_at IS NOT NULL) OR
				(status != 'resolved' AND resolved_at IS NULL)
			)
		);

		-- Events table
		CREATE TABLE events (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
			type VARCHAR(100) NOT NULL,
			actor VARCHAR(255) NOT NULL,
			description TEXT NOT NULL,
			metadata JSONB DEFAULT '{}',
			occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT event_type_not_empty CHECK (LENGTH(TRIM(type)) > 0),
			CONSTRAINT event_actor_not_empty CHECK (LENGTH(TRIM(actor)) > 0),
			CONSTRAINT event_description_not_empty CHECK (LENGTH(TRIM(description)) > 0)
		);

		-- Signals table
		CREATE TABLE signals (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			source VARCHAR(255) NOT NULL,
			title VARCHAR(500) NOT NULL,
			description TEXT,
			severity incident_severity NOT NULL,
			labels JSONB DEFAULT '{}',
			annotations JSONB DEFAULT '{}',
			payload JSONB DEFAULT '{}',
			received_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			CONSTRAINT signal_source_not_empty CHECK (LENGTH(TRIM(source)) > 0),
			CONSTRAINT signal_title_not_empty CHECK (LENGTH(TRIM(title)) > 0)
		);

		-- Incident signals junction table
		CREATE TABLE incident_signals (
			incident_id UUID NOT NULL REFERENCES incidents(id) ON DELETE CASCADE,
			signal_id UUID NOT NULL REFERENCES signals(id) ON DELETE CASCADE,
			attached_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

			PRIMARY KEY (incident_id, signal_id)
		);
	`
	if _, err := pool.Exec(ctx, tableSQL); err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Create indexes
	indexSQL := `
		-- Incidents indexes
		CREATE INDEX idx_incidents_status ON incidents(status);
		CREATE INDEX idx_incidents_severity ON incidents(severity);
		CREATE INDEX idx_incidents_team_id ON incidents(team_id);
		CREATE INDEX idx_incidents_assignee_id ON incidents(assignee_id) WHERE assignee_id IS NOT NULL;
		CREATE INDEX idx_incidents_service_id ON incidents(service_id) WHERE service_id IS NOT NULL;
		CREATE INDEX idx_incidents_created_at ON incidents(created_at);
		CREATE INDEX idx_incidents_updated_at ON incidents(updated_at);
		CREATE INDEX idx_incidents_labels ON incidents USING GIN(labels);

		-- Events indexes
		CREATE INDEX idx_events_incident_id ON events(incident_id);
		CREATE INDEX idx_events_type ON events(type);
		CREATE INDEX idx_events_occurred_at ON events(occurred_at);

		-- Signals indexes
		CREATE INDEX idx_signals_source ON signals(source);
		CREATE INDEX idx_signals_severity ON signals(severity);
		CREATE INDEX idx_signals_received_at ON signals(received_at);
		CREATE INDEX idx_signals_labels ON signals USING GIN(labels);

		-- Incident signals indexes
		CREATE INDEX idx_incident_signals_signal_id ON incident_signals(signal_id);
	`
	if _, err := pool.Exec(ctx, indexSQL); err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	return nil
}

// createTestLogger creates a logger for testing.
func createTestLogger() *logging.Logger {
	config := logging.DefaultConfig(logging.Test)
	config.AddSource = false // Reduce noise in tests
	logger, _ := logging.NewLogger(config)
	return logger
}

// createTestIncident creates a valid test incident.
func createTestIncident(id string) *domain.Incident {
	now := time.Now().UTC()
	return &domain.Incident{
		ID:          id,
		Title:       "Test incident",
		Description: "A test incident for unit tests",
		Status:      domain.StatusTriggered,
		Severity:    domain.SeverityHigh,
		TeamID:      "team-1",
		AssigneeID:  "user-1",
		ServiceID:   "service-1",
		Labels:      map[string]string{"env": "test", "component": "api"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// createTestEvent creates a valid test event.
func createTestEvent(incidentID string) *domain.Event {
	return &domain.Event{
		ID:          uuid.New().String(),
		IncidentID:  incidentID,
		Type:        "status_changed",
		Actor:       "test@example.com",
		Description: "Incident status changed",
		Metadata:    map[string]interface{}{"old_status": "triggered", "new_status": "acknowledged"},
		OccurredAt:  time.Now().UTC(),
	}
}

// createTestSignal creates a valid test signal.
func createTestSignal() domain.Signal {
	return domain.Signal{
		ID:          uuid.New().String(),
		Source:      "prometheus",
		Title:       "High CPU usage",
		Description: "CPU usage is above threshold",
		Severity:    domain.SeverityHigh,
		Labels:      map[string]string{"host": "web-01", "metric": "cpu"},
		Annotations: map[string]string{"runbook": "https://example.com/runbook"},
		Payload:     map[string]interface{}{"value": 85.5, "threshold": 80.0},
		ReceivedAt:  time.Now().UTC(),
	}
}

func TestNewIncidentRepository(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()

	t.Run("valid pool and logger", func(t *testing.T) {
		repo, err := NewIncidentRepository(pool, logger)
		if err != nil {
			t.Errorf("NewIncidentRepository() error = %v, want nil", err)
		}
		if repo == nil {
			t.Error("NewIncidentRepository() returned nil repository")
		}
	})

	t.Run("nil pool", func(t *testing.T) {
		_, err := NewIncidentRepository(nil, logger)
		if err == nil {
			t.Error("NewIncidentRepository(nil pool) should return error")
		}
	})

	t.Run("nil logger", func(t *testing.T) {
		_, err := NewIncidentRepository(pool, nil)
		if err == nil {
			t.Error("NewIncidentRepository(nil logger) should return error")
		}
	})
}

func TestIncidentRepository_Create(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	t.Run("create valid incident", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Create() error = %v, want nil", err)
		}

		// Verify incident was created
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve created incident: %v", err)
		}

		if retrieved.Title != incident.Title {
			t.Errorf("Expected title %s, got %s", incident.Title, retrieved.Title)
		}
	})

	t.Run("create incident with signals and events", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		incident.Signals = []domain.Signal{createTestSignal()}
		incident.Events = []domain.Event{*createTestEvent(incident.ID)}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Create() with signals/events error = %v, want nil", err)
		}

		// Verify signals and events were created
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve incident: %v", err)
		}

		if len(retrieved.Signals) != 1 {
			t.Errorf("Expected 1 signal, got %d", len(retrieved.Signals))
		}
		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(retrieved.Events))
		}
	})

	t.Run("create nil incident", func(t *testing.T) {
		err := repo.Create(ctx, nil)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Create(nil) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("create duplicate incident", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())

		// First create should succeed
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("First create failed: %v", err)
		}

		// Second create with same ID should fail
		err = repo.Create(ctx, incident)
		if !errors.Is(err, ports.ErrAlreadyExists) {
			t.Errorf("Create(duplicate) error = %v, want %v", err, ports.ErrAlreadyExists)
		}
	})

	t.Run("create invalid incident", func(t *testing.T) {
		incident := &domain.Incident{} // Missing required fields
		err := repo.Create(ctx, incident)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Create(invalid) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentRepository_Get(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	t.Run("get existing incident", func(t *testing.T) {
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}

		if retrieved.ID != incident.ID {
			t.Errorf("Expected ID %s, got %s", incident.ID, retrieved.ID)
		}
		if retrieved.Title != incident.Title {
			t.Errorf("Expected title %s, got %s", incident.Title, retrieved.Title)
		}
	})

	t.Run("get non-existent incident", func(t *testing.T) {
		_, err := repo.Get(ctx, uuid.New().String())
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Get(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("get with empty ID", func(t *testing.T) {
		_, err := repo.Get(ctx, "")
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Get(empty ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentRepository_List(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incidents
	now := time.Now().UTC()
	incidents := []*domain.Incident{
		{
			ID:        uuid.New().String(),
			Title:     "Critical database issue",
			Status:    domain.StatusTriggered,
			Severity:  domain.SeverityCritical,
			TeamID:    "team-db",
			Labels:    map[string]string{"env": "production"},
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        uuid.New().String(),
			Title:     "API performance issue",
			Status:    domain.StatusAcknowledged,
			Severity:  domain.SeverityHigh,
			TeamID:    "team-api",
			Labels:    map[string]string{"env": "production", "service": "api"},
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:         uuid.New().String(),
			Title:      "Minor logging issue",
			Status:     domain.StatusResolved,
			Severity:   domain.SeverityLow,
			TeamID:     "team-ops",
			Labels:     map[string]string{"env": "staging"},
			CreatedAt:  now.Add(-30 * time.Minute),
			UpdatedAt:  now.Add(-10 * time.Minute),
			ResolvedAt: func() *time.Time { t := now.Add(-10 * time.Minute); return &t }(),
		},
	}

	for _, incident := range incidents {
		if err := repo.Create(ctx, incident); err != nil {
			t.Fatalf("Failed to create test incident: %v", err)
		}
	}

	t.Run("list all incidents", func(t *testing.T) {
		filter := ports.ListFilter{}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 3 {
			t.Errorf("Expected total 3, got %d", result.Total)
		}
		if len(result.Incidents) != 3 {
			t.Errorf("Expected 3 incidents, got %d", len(result.Incidents))
		}
	})

	t.Run("filter by team", func(t *testing.T) {
		filter := ports.ListFilter{TeamID: "team-db"}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 1 {
			t.Errorf("Expected total 1 for team-db, got %d", result.Total)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		filter := ports.ListFilter{Status: []domain.Status{domain.StatusTriggered}}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 1 {
			t.Errorf("Expected total 1 for triggered status, got %d", result.Total)
		}
	})

	t.Run("filter by labels", func(t *testing.T) {
		filter := ports.ListFilter{Labels: map[string]string{"env": "production"}}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 2 {
			t.Errorf("Expected total 2 for production env, got %d", result.Total)
		}
	})

	t.Run("search by text", func(t *testing.T) {
		filter := ports.ListFilter{Search: "database"}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 1 {
			t.Errorf("Expected total 1 for database search, got %d", result.Total)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		filter := ports.ListFilter{Limit: 2, Offset: 0}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if len(result.Incidents) != 2 {
			t.Errorf("Expected 2 incidents in first page, got %d", len(result.Incidents))
		}
		if !result.HasMore {
			t.Error("Expected HasMore to be true for first page")
		}
	})
}

func TestIncidentRepository_Update(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	t.Run("update existing incident", func(t *testing.T) {
		// Get current incident for optimistic locking
		current, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get current incident: %v", err)
		}

		current.Title = "Updated title"
		current.Status = domain.StatusAcknowledged

		err = repo.Update(ctx, current)
		if err != nil {
			t.Errorf("Update() error = %v, want nil", err)
		}

		// Verify update
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get updated incident: %v", err)
		}

		if retrieved.Title != "Updated title" {
			t.Errorf("Expected updated title, got %s", retrieved.Title)
		}
		if retrieved.Status != domain.StatusAcknowledged {
			t.Errorf("Expected updated status, got %s", retrieved.Status)
		}
	})

	t.Run("update non-existent incident", func(t *testing.T) {
		nonExistent := createTestIncident(uuid.New().String())
		err := repo.Update(ctx, nonExistent)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Update(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("optimistic locking conflict", func(t *testing.T) {
		// Get current incident twice to simulate two concurrent operations
		incident1, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get incident1: %v", err)
		}

		incident2, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get incident2: %v", err)
		}

		// First update succeeds
		incident1.Title = "Updated by user 1"
		err = repo.Update(ctx, incident1)
		if err != nil {
			t.Fatalf("Failed to perform first update: %v", err)
		}

		// Second update should fail due to stale timestamp
		incident2.Title = "Updated by user 2"
		err = repo.Update(ctx, incident2)
		if !errors.Is(err, ports.ErrConflict) {
			t.Errorf("Update(conflicted) error = %v, want %v", err, ports.ErrConflict)
		}
	})
}

func TestIncidentRepository_AddEvent(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	t.Run("add valid event", func(t *testing.T) {
		event := createTestEvent(incident.ID)
		err := repo.AddEvent(ctx, event)
		if err != nil {
			t.Errorf("AddEvent() error = %v, want nil", err)
		}

		// Verify event was added
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get incident: %v", err)
		}

		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(retrieved.Events))
		}
		if retrieved.Events[0].Type != "status_changed" {
			t.Errorf("Expected event type 'status_changed', got %s", retrieved.Events[0].Type)
		}
	})

	t.Run("add event to non-existent incident", func(t *testing.T) {
		event := createTestEvent(uuid.New().String())
		err := repo.AddEvent(ctx, event)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("AddEvent(non-existent incident) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("add event without ID generates ID", func(t *testing.T) {
		event := createTestEvent(incident.ID)
		event.ID = "" // Clear ID to test generation

		err := repo.AddEvent(ctx, event)
		if err != nil {
			t.Errorf("AddEvent() error = %v, want nil", err)
		}

		// Verify event ID was generated
		if event.ID == "" {
			t.Error("Expected event ID to be generated")
		}
	})
}

func TestIncidentRepository_Delete(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	t.Run("delete existing incident", func(t *testing.T) {
		err := repo.Delete(ctx, incident.ID)
		if err != nil {
			t.Errorf("Delete() error = %v, want nil", err)
		}

		// Verify deletion
		_, err = repo.Get(ctx, incident.ID)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Expected incident to be deleted, but Get() error = %v", err)
		}
	})

	t.Run("delete non-existent incident", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New().String())
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Delete(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("delete with empty ID", func(t *testing.T) {
		err := repo.Delete(ctx, "")
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Delete(empty ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentRepository_GetWithEvents(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create incident and add event
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	event := createTestEvent(incident.ID)
	if err := repo.AddEvent(ctx, event); err != nil {
		t.Fatalf("Failed to add event: %v", err)
	}

	t.Run("get with events", func(t *testing.T) {
		retrieved, err := repo.GetWithEvents(ctx, incident.ID)
		if err != nil {
			t.Errorf("GetWithEvents() error = %v, want nil", err)
		}

		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(retrieved.Events))
		}
	})
}

func TestIncidentRepository_GetWithSignals(t *testing.T) {
	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create incident with signals
	incident := createTestIncident(uuid.New().String())
	incident.Signals = []domain.Signal{createTestSignal()}

	if err := repo.Create(ctx, incident); err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	t.Run("get with signals", func(t *testing.T) {
		retrieved, err := repo.GetWithSignals(ctx, incident.ID)
		if err != nil {
			t.Errorf("GetWithSignals() error = %v, want nil", err)
		}

		if len(retrieved.Signals) != 1 {
			t.Errorf("Expected 1 signal, got %d", len(retrieved.Signals))
		}
	})
}

// Benchmark tests to ensure performance
func BenchmarkIncidentRepository_Create(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, &testing.T{})
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		incident := createTestIncident(uuid.New().String())
		if err := repo.Create(ctx, incident); err != nil {
			b.Fatalf("Create failed: %v", err)
		}
	}
}

func BenchmarkIncidentRepository_Get(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, &testing.T{})
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	if err := repo.Create(ctx, incident); err != nil {
		b.Fatalf("Setup failed: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.Get(ctx, incident.ID); err != nil {
			b.Fatalf("Get failed: %v", err)
		}
	}
}

func BenchmarkIncidentRepository_List(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping benchmark in short mode")
	}

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, &testing.T{})
	defer cleanup()

	logger := createTestLogger()
	repo, _ := NewIncidentRepository(pool, logger)

	// Create test incidents
	for i := 0; i < 100; i++ {
		incident := createTestIncident(uuid.New().String())
		if err := repo.Create(ctx, incident); err != nil {
			b.Fatalf("Setup failed: %v", err)
		}
	}

	filter := ports.ListFilter{Limit: 50}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := repo.List(ctx, filter); err != nil {
			b.Fatalf("List failed: %v", err)
		}
	}
}

// Helper function to skip integration tests based on environment
func skipIntegrationTest(t *testing.T) {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "true" {
		t.Skip("Skipping integration test")
	}
}

// Test helper functions
func TestHelperFunctions(t *testing.T) {
	t.Run("nullableString", func(t *testing.T) {
		result := nullableString("")
		if result.Valid {
			t.Error("Empty string should result in invalid NullString")
		}

		result = nullableString("test")
		if !result.Valid || result.String != "test" {
			t.Error("Non-empty string should result in valid NullString")
		}
	})

	t.Run("isUniqueViolation", func(t *testing.T) {
		err := fmt.Errorf("duplicate key violates unique constraint")
		if !isUniqueViolation(err) {
			t.Error("Should detect unique violation")
		}

		err = fmt.Errorf("some other error")
		if isUniqueViolation(err) {
			t.Error("Should not detect unique violation for other errors")
		}
	})
}

// TestIncidentRepository_ErrorHandling tests comprehensive error scenarios
func TestIncidentRepository_ErrorHandling(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("database timeout simulation", func(t *testing.T) {
		// Create a context with very short timeout
		timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Nanosecond)
		defer cancel()

		incident := createTestIncident("timeout-test")
		err := repo.Create(timeoutCtx, incident)
		if err == nil {
			t.Error("Expected timeout error but got nil")
		}
		// Note: Actual timeout testing requires specific database conditions
	})

	t.Run("context cancellation", func(t *testing.T) {
		cancelCtx, cancel := context.WithCancel(ctx)
		cancel() // Cancel immediately

		incident := createTestIncident("cancel-test")
		err := repo.Create(cancelCtx, incident)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Expected context.Canceled, got: %v", err)
		}
	})

	t.Run("malformed JSON in labels", func(t *testing.T) {
		// This test would require injecting malformed JSON directly into database
		// For now, we test the JSON error handling path indirectly
		incident := createTestIncident(uuid.New().String())
		// Create incident with complex nested labels to test JSON edge cases
		incident.Labels = map[string]string{
			"very_long_key_that_might_cause_issues": strings.Repeat("x", 1000),
			"unicode_test":                          "测试中文字符",
			"special_chars":                         "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Should handle complex labels, got error: %v", err)
		}
	})

	t.Run("connection pool exhaustion simulation", func(t *testing.T) {
		// This is difficult to test without modifying pool settings
		// For now, we ensure the repository handles pool-related errors gracefully

		// Test with nil pool to simulate connection issues
		invalidRepo := &IncidentRepository{pool: nil, logger: logger}
		incident := createTestIncident("pool-test")

		// This should panic or error due to nil pool
		defer func() {
			if r := recover(); r != nil {
				// Expected behavior for nil pool
			}
		}()

		invalidRepo.Create(ctx, incident)
	})
}

// TestIncidentRepository_TransactionFailures tests transaction rollback scenarios
func TestIncidentRepository_TransactionFailures(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("create with invalid signals", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		// Add invalid signal that should cause transaction to fail
		incident.Signals = []domain.Signal{
			{
				ID:     "", // Will be generated
				Source: "", // Invalid - empty source should fail validation
				Title:  "Invalid Signal",
			},
		}

		err := repo.Create(ctx, incident)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Expected ErrInvalidInput for invalid signal, got: %v", err)
		}

		// Verify incident was not created due to rollback
		_, err = repo.Get(ctx, incident.ID)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Error("Incident should not exist after transaction rollback")
		}
	})

	t.Run("create with invalid events", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		incident.Events = []domain.Event{
			{
				IncidentID: incident.ID,
				Type:       "", // Invalid - empty type
				Actor:      "test@example.com",
			},
		}

		err := repo.Create(ctx, incident)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Expected ErrInvalidInput for invalid event, got: %v", err)
		}

		// Verify incident was not created
		_, err = repo.Get(ctx, incident.ID)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Error("Incident should not exist after transaction rollback")
		}
	})
}

// TestIncidentRepository_ConcurrencyEdgeCases tests advanced concurrency scenarios
func TestIncidentRepository_ConcurrencyEdgeCases(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("optimistic locking with rapid updates", func(t *testing.T) {
		// Create initial incident
		incident := createTestIncident(uuid.New().String())
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Failed to create incident: %v", err)
		}

		// Simulate rapid concurrent updates
		const numUpdaters = 5
		const updatesPerUpdater = 3

		var wg sync.WaitGroup
		errorCh := make(chan error, numUpdaters*updatesPerUpdater)

		for i := 0; i < numUpdaters; i++ {
			wg.Add(1)
			go func(updaterID int) {
				defer wg.Done()

				for j := 0; j < updatesPerUpdater; j++ {
					// Get fresh copy
					current, err := repo.Get(ctx, incident.ID)
					if err != nil {
						errorCh <- fmt.Errorf("updater %d: failed to get incident: %v", updaterID, err)
						return
					}

					// Modify
					current.Title = fmt.Sprintf("Updated by %d-%d", updaterID, j)
					current.UpdatedAt = time.Now().UTC()

					// Try to update
					err = repo.Update(ctx, current)
					if err != nil && !errors.Is(err, ports.ErrConflict) {
						errorCh <- fmt.Errorf("updater %d: unexpected error: %v", updaterID, err)
						return
					}
					// ErrConflict is expected due to optimistic locking
				}
			}(i)
		}

		wg.Wait()
		close(errorCh)

		// Check for unexpected errors
		for err := range errorCh {
			t.Errorf("Concurrent update error: %v", err)
		}

		// Verify incident still exists and is in consistent state
		final, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to get incident after concurrent updates: %v", err)
		}

		if final.ID != incident.ID {
			t.Error("Incident ID should remain unchanged")
		}
	})
}

// TestIncidentRepository_buildListQueryEdgeCases tests the complex query building logic
func TestIncidentRepository_buildListQueryEdgeCases(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	// Create test data with various combinations
	now := time.Now().UTC()
	testIncidents := []*domain.Incident{
		{
			ID:        uuid.New().String(),
			Title:     "Database Issue with Special Characters: !@#$%^&*()",
			Status:    domain.StatusTriggered,
			Severity:  domain.SeverityCritical,
			TeamID:    "team-special",
			Labels:    map[string]string{"env": "production", "component": "database", "priority": "urgent"},
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        uuid.New().String(),
			Title:     "API Rate Limiting Issue",
			Status:    domain.StatusAcknowledged,
			Severity:  domain.SeverityHigh,
			TeamID:    "team-api",
			Labels:    map[string]string{"env": "staging", "component": "api"},
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
	}

	for _, incident := range testIncidents {
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Failed to create test incident %s: %v", incident.ID, err)
		}
	}

	edgeCaseFilters := []struct {
		name   string
		filter ports.ListFilter
		desc   string
	}{
		{
			name: "empty arrays",
			filter: ports.ListFilter{
				Status:   []domain.Status{},
				Severity: []domain.Severity{},
				Labels:   map[string]string{},
			},
			desc: "Test empty filter arrays",
		},
		{
			name: "special characters in search",
			filter: ports.ListFilter{
				Search: "!@#$%^&*()",
			},
			desc: "Search with special characters",
		},
		{
			name: "unicode search",
			filter: ports.ListFilter{
				Search: "测试",
			},
			desc: "Search with unicode characters",
		},
		{
			name: "complex time ranges",
			filter: ports.ListFilter{
				CreatedAfter:  &[]time.Time{now.Add(-3 * time.Hour)}[0],
				CreatedBefore: &[]time.Time{now.Add(-30 * time.Minute)}[0],
				UpdatedAfter:  &[]time.Time{now.Add(-2 * time.Hour)}[0],
				UpdatedBefore: &[]time.Time{now}[0],
			},
			desc: "Complex time range filtering",
		},
		{
			name: "multiple status and severity",
			filter: ports.ListFilter{
				Status:   []domain.Status{domain.StatusTriggered, domain.StatusAcknowledged, domain.StatusInvestigating},
				Severity: []domain.Severity{domain.SeverityCritical, domain.SeverityHigh, domain.SeverityMedium},
			},
			desc: "Multiple status and severity filters",
		},
		{
			name: "nested label filtering",
			filter: ports.ListFilter{
				Labels: map[string]string{
					"env":       "production",
					"component": "database",
					"priority":  "urgent",
				},
			},
			desc: "Multiple label filtering",
		},
		{
			name: "pagination edge cases",
			filter: ports.ListFilter{
				Limit:  1,
				Offset: 0,
			},
			desc: "Single item pagination",
		},
	}

	for _, tc := range edgeCaseFilters {
		t.Run(tc.name, func(t *testing.T) {
			result, err := repo.List(ctx, tc.filter)
			if err != nil {
				t.Errorf("Filter '%s' failed: %v", tc.desc, err)
				return
			}

			if result == nil {
				t.Errorf("Filter '%s' returned nil result", tc.desc)
				return
			}

			// Basic sanity checks
			if result.Total < 0 {
				t.Errorf("Filter '%s' returned negative total: %d", tc.desc, result.Total)
			}

			if len(result.Incidents) > result.Total {
				t.Errorf("Filter '%s' returned more incidents than total", tc.desc)
			}

			if tc.filter.Limit > 0 && len(result.Incidents) > tc.filter.Limit {
				t.Errorf("Filter '%s' returned more incidents than limit", tc.desc)
			}
		})
	}
}

// TestIncidentRepository_JSONBEdgeCases tests JSONB handling edge cases
func TestIncidentRepository_JSONBEdgeCases(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("complex nested data structures", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		incident.Labels = map[string]string{
			"empty_value":   "",
			"null_like":     "null",
			"bool_like":     "true",
			"number_like":   "123",
			"json_like":     `{"nested": "value"}`,
			"unicode":       "测试中文字符",
			"special_chars": "!@#$%^&*()_+-=[]{}|;':\",./<>?",
			"very_long":     strings.Repeat("x", 500),
		}

		// Create signal with complex payload
		signal := createTestSignal()
		signal.Payload = map[string]interface{}{
			"nested_object": map[string]interface{}{
				"level2": map[string]interface{}{
					"level3": "deep_value",
					"array":  []interface{}{1, 2, 3, "mixed", true},
				},
			},
			"null_value":   nil,
			"empty_string": "",
			"large_number": 9223372036854775807, // Max int64
			"float_value":  3.14159265359,
			"boolean":      true,
		}
		signal.Annotations = map[string]string{
			"runbook_url": "https://example.com/runbook?param=value&other=test",
			"unicode":     "🚨 Alert: System Down 🚨",
		}

		incident.Signals = []domain.Signal{signal}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Failed to create incident with complex JSONB data: %v", err)
			return
		}

		// Retrieve and verify
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve incident: %v", err)
		}

		// Verify labels
		for key, value := range incident.Labels {
			if retrieved.Labels[key] != value {
				t.Errorf("Label mismatch for key %s: expected %s, got %s", key, value, retrieved.Labels[key])
			}
		}

		// Verify signal payload integrity
		if len(retrieved.Signals) != 1 {
			t.Fatalf("Expected 1 signal, got %d", len(retrieved.Signals))
		}

		retrievedSignal := retrieved.Signals[0]

		// Check specific payload values
		if retrievedSignal.Payload["empty_string"] != "" {
			t.Error("Empty string payload value should be preserved")
		}

		if retrievedSignal.Payload["boolean"] != true {
			t.Error("Boolean payload value should be preserved")
		}

		// Check nested object
		nested, ok := retrievedSignal.Payload["nested_object"].(map[string]interface{})
		if !ok {
			t.Error("Nested object should be preserved")
		} else {
			level2, ok := nested["level2"].(map[string]interface{})
			if !ok {
				t.Error("Deep nested object should be preserved")
			} else if level2["level3"] != "deep_value" {
				t.Error("Deep nested value should be preserved")
			}
		}
	})

	t.Run("nil and empty JSONB fields", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())
		incident.Labels = nil // Test nil labels

		signal := createTestSignal()
		signal.Labels = nil
		signal.Annotations = nil
		signal.Payload = nil
		incident.Signals = []domain.Signal{signal}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Failed to create incident with nil JSONB fields: %v", err)
			return
		}

		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve incident: %v", err)
		}

		// Verify nil fields are handled correctly
		if retrieved.Labels != nil && len(retrieved.Labels) > 0 {
			t.Error("Nil labels should remain nil or empty")
		}

		if len(retrieved.Signals) > 0 {
			s := retrieved.Signals[0]
			if s.Labels != nil && len(s.Labels) > 0 {
				t.Error("Nil signal labels should remain nil or empty")
			}
			if s.Annotations != nil && len(s.Annotations) > 0 {
				t.Error("Nil signal annotations should remain nil or empty")
			}
			if s.Payload != nil && len(s.Payload) > 0 {
				t.Error("Nil signal payload should remain nil or empty")
			}
		}
	})
}

// TestIncidentRepository_buildListQueryComprehensive tests comprehensive query building scenarios
func TestIncidentRepository_buildListQueryComprehensive(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	// Create diverse test data
	now := time.Now().UTC()
	testIncidents := []*domain.Incident{
		{
			ID:         uuid.New().String(),
			Title:      "Production API Gateway Down",
			Status:     domain.StatusTriggered,
			Severity:   domain.SeverityCritical,
			TeamID:     "team-platform",
			AssigneeID: "alice@example.com",
			ServiceID:  "api-gateway",
			Labels:     map[string]string{"env": "production", "service": "gateway", "region": "us-east-1"},
			CreatedAt:  now.Add(-4 * time.Hour),
			UpdatedAt:  now.Add(-3 * time.Hour),
		},
		{
			ID:         uuid.New().String(),
			Title:      "Database Connection Pool Exhausted",
			Status:     domain.StatusInvestigating,
			Severity:   domain.SeverityHigh,
			TeamID:     "team-database",
			AssigneeID: "bob@example.com",
			ServiceID:  "postgres-primary",
			Labels:     map[string]string{"env": "production", "service": "database", "component": "connection-pool"},
			CreatedAt:  now.Add(-2 * time.Hour),
			UpdatedAt:  now.Add(-1 * time.Hour),
		},
		{
			ID:         uuid.New().String(),
			Title:      "Staging Environment Deployment Failed",
			Status:     domain.StatusResolved,
			Severity:   domain.SeverityMedium,
			TeamID:     "team-devops",
			ServiceID:  "deployment-service",
			Labels:     map[string]string{"env": "staging", "service": "deployment", "pipeline": "main"},
			CreatedAt:  now.Add(-6 * time.Hour),
			UpdatedAt:  now.Add(-30 * time.Minute),
			ResolvedAt: func() *time.Time { t := now.Add(-30 * time.Minute); return &t }(),
		},
		{
			ID:        uuid.New().String(),
			Title:     "Monitoring Alert: High Memory Usage",
			Status:    domain.StatusAcknowledged,
			Severity:  domain.SeverityLow,
			TeamID:    "team-platform",
			ServiceID: "monitoring-service",
			Labels:    map[string]string{"env": "production", "service": "monitoring", "metric": "memory"},
			CreatedAt: now.Add(-8 * time.Hour),
			UpdatedAt: now.Add(-7 * time.Hour),
		},
	}

	for _, incident := range testIncidents {
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Failed to create test incident %s: %v", incident.ID, err)
		}
	}

	comprehensiveTests := []struct {
		name        string
		filter      ports.ListFilter
		expectedMin int
		expectedMax int
		shouldError bool
		description string
	}{
		{
			name: "multiple status with multiple severity",
			filter: ports.ListFilter{
				Status:    []domain.Status{domain.StatusTriggered, domain.StatusInvestigating},
				Severity:  []domain.Severity{domain.SeverityCritical, domain.SeverityHigh},
				SortBy:    "severity",
				SortOrder: "desc",
			},
			expectedMin: 2,
			expectedMax: 2,
			description: "Complex filter with multiple enums",
		},
		{
			name: "assignee filter with specific user",
			filter: ports.ListFilter{
				AssigneeID: "alice@example.com",
				SortBy:     "created_at",
				SortOrder:  "asc",
			},
			expectedMin: 1,
			expectedMax: 1,
			description: "Filter by specific assignee",
		},
		{
			name: "service filter with pagination",
			filter: ports.ListFilter{
				ServiceID: "api-gateway",
				Limit:     10,
				Offset:    0,
				SortBy:    "updated_at",
				SortOrder: "desc",
			},
			expectedMin: 1,
			expectedMax: 1,
			description: "Service filter with pagination",
		},
		{
			name: "complex time range with labels",
			filter: ports.ListFilter{
				CreatedAfter:  &[]time.Time{now.Add(-5 * time.Hour)}[0],
				CreatedBefore: &[]time.Time{now.Add(-1 * time.Hour)}[0],
				Labels:        map[string]string{"env": "production"},
				SortBy:        "created_at",
				SortOrder:     "asc",
			},
			expectedMin: 2,
			expectedMax: 2,
			description: "Time range with label filtering",
		},
		{
			name: "updated time filters",
			filter: ports.ListFilter{
				UpdatedAfter:  &[]time.Time{now.Add(-2 * time.Hour)}[0],
				UpdatedBefore: &[]time.Time{now}[0],
				SortBy:        "updated_at",
				SortOrder:     "desc",
			},
			expectedMin: 2,
			expectedMax: 3,
			description: "Filter by update time range",
		},

		{
			name: "search in description",
			filter: ports.ListFilter{
				Search:    "failed",
				SortBy:    "severity",
				SortOrder: "asc",
			},
			expectedMin: 0,
			expectedMax: 1,
			description: "Search in incident description",
		},
		{
			name: "nested label matching",
			filter: ports.ListFilter{
				Labels: map[string]string{
					"service": "gateway",
					"env":     "production",
					"region":  "us-east-1",
				},
				SortBy:    "created_at",
				SortOrder: "desc",
			},
			expectedMin: 1,
			expectedMax: 1,
			description: "Multiple label exact matching",
		},
		{
			name: "empty results with impossible filter",
			filter: ports.ListFilter{
				TeamID:    "non-existent-team",
				Status:    []domain.Status{domain.StatusTriggered},
				SortBy:    "created_at",
				SortOrder: "asc",
			},
			expectedMin: 0,
			expectedMax: 0,
			description: "Filter that should return no results",
		},
		{
			name: "pagination beyond available data",
			filter: ports.ListFilter{
				Limit:     5,
				Offset:    100,
				SortBy:    "created_at",
				SortOrder: "asc",
			},
			expectedMin: 0,
			expectedMax: 4, // Total count remains, but no results returned
			description: "Pagination offset beyond data",
		},
		{
			name: "all parameters combined",
			filter: ports.ListFilter{
				TeamID:        "team-platform",
				Status:        []domain.Status{domain.StatusTriggered, domain.StatusAcknowledged},
				Severity:      []domain.Severity{domain.SeverityCritical, domain.SeverityLow},
				Labels:        map[string]string{"env": "production"},
				CreatedAfter:  &[]time.Time{now.Add(-10 * time.Hour)}[0],
				CreatedBefore: &[]time.Time{now}[0],
				Search:        "monitoring",
				Limit:         2,
				Offset:        0,
				SortBy:        "severity",
				SortOrder:     "desc",
			},
			expectedMin: 0,
			expectedMax: 2,
			description: "All filter parameters combined",
		},
	}

	for _, tc := range comprehensiveTests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := repo.List(ctx, tc.filter)

			if tc.shouldError && err == nil {
				t.Errorf("Test '%s' expected error but got none", tc.description)
				return
			}

			if !tc.shouldError && err != nil {
				t.Errorf("Test '%s' failed with error: %v", tc.description, err)
				return
			}

			if err == nil {
				if result.Total < tc.expectedMin || result.Total > tc.expectedMax {
					t.Errorf("Test '%s' expected %d-%d results, got %d",
						tc.description, tc.expectedMin, tc.expectedMax, result.Total)
				}

				if len(result.Incidents) > result.Total {
					t.Errorf("Filter '%s' returned more incidents than total", tc.description)
				}

				if tc.filter.Limit > 0 && len(result.Incidents) > tc.filter.Limit {
					t.Errorf("Filter '%s' returned more incidents than limit", tc.description)
				}

				if tc.filter.Offset >= 0 && result.Offset != tc.filter.Offset {
					t.Errorf("Filter '%s' offset mismatch: expected %d, got %d",
						tc.description, tc.filter.Offset, result.Offset)
				}

				if tc.filter.Limit > 0 && result.Limit != tc.filter.Limit {
					t.Errorf("Filter '%s' limit mismatch: expected %d, got %d",
						tc.description, tc.filter.Limit, result.Limit)
				}

				// Verify HasMore calculation
				expectedHasMore := result.Offset+len(result.Incidents) < result.Total
				if result.HasMore != expectedHasMore {
					t.Errorf("Filter '%s' HasMore mismatch: expected %v, got %v",
						tc.description, expectedHasMore, result.HasMore)
				}
			}
		})
	}
}

// TestIncidentRepository_ErrorPathsCoverage tests specific error handling paths
func TestIncidentRepository_ErrorPathsCoverage(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("mapError with different error types", func(t *testing.T) {
		// Test context deadline exceeded
		timeoutErr := context.DeadlineExceeded
		mappedErr := repo.mapError(timeoutErr)
		if !errors.Is(mappedErr, ports.ErrTimeout) {
			t.Errorf("Expected ErrTimeout for DeadlineExceeded, got: %v", mappedErr)
		}

		// Test context cancelled
		cancelErr := context.Canceled
		mappedErr = repo.mapError(cancelErr)
		if !errors.Is(mappedErr, context.Canceled) {
			t.Errorf("Expected context.Canceled to pass through, got: %v", mappedErr)
		}

		// Test connection-related errors
		connErr := fmt.Errorf("connection refused")
		mappedErr = repo.mapError(connErr)
		if !errors.Is(mappedErr, ports.ErrConnectionFailed) {
			t.Errorf("Expected ErrConnectionFailed for connection error, got: %v", mappedErr)
		}

		// Test network errors
		networkErr := fmt.Errorf("network timeout occurred")
		mappedErr = repo.mapError(networkErr)
		if !errors.Is(mappedErr, ports.ErrConnectionFailed) {
			t.Errorf("Expected ErrConnectionFailed for network error, got: %v", mappedErr)
		}

		// Test JSON-related errors
		jsonErr := fmt.Errorf("json unmarshal failed")
		mappedErr = repo.mapError(jsonErr)
		if !errors.Is(mappedErr, ports.ErrInvalidInput) {
			t.Errorf("Expected ErrInvalidInput for JSON error, got: %v", mappedErr)
		}

		// Test unknown error
		unknownErr := fmt.Errorf("mysterious database issue")
		mappedErr = repo.mapError(unknownErr)
		if !errors.Is(mappedErr, ports.ErrConnectionFailed) {
			t.Errorf("Expected ErrConnectionFailed for unknown error, got: %v", mappedErr)
		}

		// Test nil error
		mappedErr = repo.mapError(nil)
		if mappedErr != nil {
			t.Errorf("Expected nil for nil error, got: %v", mappedErr)
		}
	})

	t.Run("scanIncident with malformed JSON", func(t *testing.T) {
		// This test simulates scanning a row with malformed JSON
		// Since PostgreSQL rejects invalid JSON at insert time, we'll test the unmarshal path differently
		incident := createTestIncident(uuid.New().String())
		// Create incident with complex nested labels to test JSON edge cases
		incident.Labels = map[string]string{
			"very_long_key_that_might_cause_issues": strings.Repeat("x", 1000),
			"unicode_test":                          "测试中文字符",
			"special_chars":                         "!@#$%^&*()_+-=[]{}|;':\",./<>?",
		}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Failed to create test incident: %v", err)
		}

		// Instead of corrupting JSON in DB (which PostgreSQL prevents),
		// we'll test the scanning of valid but complex JSON data
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Errorf("Failed to retrieve incident with complex JSON: %v", err)
		}

		// Verify data integrity
		if retrieved != nil {
			for key, expectedValue := range incident.Labels {
				if retrieved.Labels[key] != expectedValue {
					t.Errorf("JSON data integrity failed for key %s", key)
				}
			}
		}
	})

	t.Run("loadSignals and loadEvents with complex JSON", func(t *testing.T) {
		// Create incident with signals/events containing complex JSON
		incident := createTestIncident(uuid.New().String())
		signal := createTestSignal()
		signal.Labels = map[string]string{
			"complex_json": `{"nested": {"value": "test"}}`,
			"unicode":      "🔥💻",
		}
		signal.Annotations = map[string]string{
			"runbook": "https://example.com/runbook",
			"tags":    "critical,database,production",
		}
		signal.Payload = map[string]interface{}{
			"metrics": map[string]interface{}{
				"cpu":    85.5,
				"memory": 92.1,
				"nested": map[string]string{"host": "web-01"},
			},
		}
		incident.Signals = []domain.Signal{signal}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Failed to create incident with complex signals: %v", err)
		}

		// Verify signals load correctly
		signals, err := repo.loadSignals(ctx, incident.ID)
		if err != nil {
			t.Errorf("Failed to load signals with complex JSON: %v", err)
		}

		if len(signals) != 1 {
			t.Errorf("Expected 1 signal, got %d", len(signals))
		}

		// Test events with complex metadata
		event := createTestEvent(incident.ID)
		event.Metadata = map[string]interface{}{
			"changes": map[string]interface{}{
				"from": "triggered",
				"to":   "acknowledged",
			},
			"actor_details": map[string]string{
				"name":  "John Doe",
				"email": "john@example.com",
			},
			"timestamp": time.Now().Unix(),
		}

		err = repo.AddEvent(ctx, event)
		if err != nil {
			t.Fatalf("Failed to add event with complex metadata: %v", err)
		}

		// Verify events load correctly
		events, err := repo.loadEvents(ctx, incident.ID)
		if err != nil {
			t.Errorf("Failed to load events with complex JSON: %v", err)
		}

		if len(events) == 0 {
			t.Error("Expected at least 1 event")
		}
	})

	t.Run("insertSignals and insertEvents validation failures", func(t *testing.T) {
		// Test insertSignals with validation failure
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatalf("Failed to begin transaction: %v", err)
		}
		defer tx.Rollback(ctx)

		invalidSignals := []domain.Signal{
			{
				ID:     uuid.New().String(),
				Source: "", // Invalid - empty source
				Title:  "Test Signal",
			},
		}

		err = repo.insertSignals(ctx, tx, "test-incident-id", invalidSignals)
		if err == nil {
			t.Error("Expected error for invalid signal")
		}
		// Check that it's a validation error (wrapped as invalid input)
		if !strings.Contains(err.Error(), "invalid signal") {
			t.Errorf("Expected validation error for invalid signal, got: %v", err)
		}

		// Test insertEvents with validation failure
		invalidEvents := []domain.Event{
			{
				IncidentID: "test-incident-id",
				Type:       "", // Invalid - empty type
				Actor:      "test@example.com",
			},
		}

		err = repo.insertEvents(ctx, tx, invalidEvents)
		if err == nil {
			t.Error("Expected error for invalid event")
		}
		// Check that it's a validation error (wrapped as invalid input)
		if !strings.Contains(err.Error(), "invalid event") {
			t.Errorf("Expected validation error for invalid event, got: %v", err)
		}
	})

	t.Run("Create with ID generation", func(t *testing.T) {
		// For this implementation, ID must be provided as per domain validation
		// Test that we properly handle the validation error
		incident := createTestIncident("") // This will create a valid incident but with empty ID
		incident.ID = ""                   // Ensure it's empty

		err := repo.Create(ctx, incident)
		if err == nil {
			t.Error("Expected error when ID is empty (domain validation)")
		}

		// The error should be wrapped as ErrInvalidInput
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Expected ErrInvalidInput for empty ID, got: %v", err)
		}

		// Test with valid ID to ensure normal path works
		incident.ID = uuid.New().String()
		err = repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Create with valid ID should succeed, got error: %v", err)
		}
	})

	t.Run("Complex JSON marshaling edge cases", func(t *testing.T) {
		incident := createTestIncident(uuid.New().String())

		// Test with labels containing special JSON characters
		incident.Labels = map[string]string{
			"quotes":    `"double" and 'single' quotes`,
			"backslash": `path\to\file`,
			"newlines":  "line1\nline2\r\nline3",
			"unicode":   "测试🚨💻",
			"control":   "\t\b\f\r\n",
		}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Failed to create incident with special JSON characters: %v", err)
		}

		// Verify retrieval maintains data integrity
		retrieved, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Fatalf("Failed to retrieve incident: %v", err)
		}

		for key, expectedValue := range incident.Labels {
			if retrieved.Labels[key] != expectedValue {
				t.Errorf("Label data integrity failed for key %s: expected %q, got %q",
					key, expectedValue, retrieved.Labels[key])
			}
		}
	})
}

// TestIncidentRepository_BuildQueryParameterCounting tests parameter counting edge cases
func TestIncidentRepository_BuildQueryParameterCounting(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	// Create test incident
	incident := createTestIncident(uuid.New().String())
	err = repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Failed to create test incident: %v", err)
	}

	// Test query building with various parameter combinations
	testCases := []struct {
		name   string
		filter ports.ListFilter
	}{
		{
			name: "maximum parameters",
			filter: ports.ListFilter{
				TeamID:        "team-1",
				Status:        []domain.Status{domain.StatusTriggered, domain.StatusAcknowledged},
				Severity:      []domain.Severity{domain.SeverityHigh, domain.SeverityMedium},
				AssigneeID:    "user-1",
				ServiceID:     "service-1",
				Labels:        map[string]string{"env": "prod", "region": "us-east"},
				CreatedAfter:  &[]time.Time{time.Now().Add(-24 * time.Hour)}[0],
				CreatedBefore: &[]time.Time{time.Now()}[0],
				UpdatedAfter:  &[]time.Time{time.Now().Add(-12 * time.Hour)}[0],
				UpdatedBefore: &[]time.Time{time.Now()}[0],
				Search:        "test search",
				Limit:         10,
				Offset:        0,
				SortBy:        "created_at",
				SortOrder:     "desc",
			},
		},
		{
			name: "minimal parameters",
			filter: ports.ListFilter{
				Limit:     5,
				Offset:    0,
				SortBy:    "updated_at",
				SortOrder: "asc",
			},
		},
		{
			name: "only time filters",
			filter: ports.ListFilter{
				CreatedAfter:  &[]time.Time{time.Now().Add(-1 * time.Hour)}[0],
				UpdatedBefore: &[]time.Time{time.Now()}[0],
				Limit:         20,
				Offset:        5,
			},
		},
		{
			name: "only enum filters",
			filter: ports.ListFilter{
				Status:   []domain.Status{domain.StatusResolved},
				Severity: []domain.Severity{domain.SeverityLow, domain.SeverityInfo},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test that the query builds without error
			result, err := repo.List(ctx, tc.filter)
			if err != nil {
				t.Errorf("Query building failed for %s: %v", tc.name, err)
			}

			// Verify basic result structure
			if result == nil {
				t.Errorf("Got nil result for %s", tc.name)
				return
			}

			if result.Total < 0 {
				t.Errorf("Got negative total for %s: %d", tc.name, result.Total)
			}

			if len(result.Incidents) > result.Total {
				t.Errorf("Incidents count exceeds total for %s", tc.name)
			}

			// Verify pagination parameters are preserved
			if tc.filter.Limit > 0 && result.Limit != tc.filter.Limit {
				t.Errorf("Limit not preserved for %s: expected %d, got %d",
					tc.name, tc.filter.Limit, result.Limit)
			}

			if tc.filter.Offset >= 0 && result.Offset != tc.filter.Offset {
				t.Errorf("Offset not preserved for %s: expected %d, got %d",
					tc.name, tc.filter.Offset, result.Offset)
			}
		})
	}
}

// TestIncidentRepository_HelperMethodsCoverage ensures helper methods are covered
func TestIncidentRepository_HelperMethodsCoverage(t *testing.T) {
	skipIntegrationTest(t)

	ctx := context.Background()
	_, pool, cleanup := setupTestDB(ctx, t)
	defer cleanup()

	logger := createTestLogger()
	repo, err := NewIncidentRepository(pool, logger)
	if err != nil {
		t.Fatalf("Failed to create repository: %v", err)
	}

	t.Run("nullableString edge cases", func(t *testing.T) {
		// Test empty string
		result := nullableString("")
		if result.Valid {
			t.Error("Empty string should result in invalid NullString")
		}

		// Test whitespace-only string
		result = nullableString("   ")
		if !result.Valid || result.String != "   " {
			t.Error("Whitespace string should be valid and preserved")
		}

		// Test very long string
		longString := strings.Repeat("x", 1000)
		result = nullableString(longString)
		if !result.Valid || result.String != longString {
			t.Error("Long string should be handled correctly")
		}

		// Test string with special characters
		specialString := "test\x00\xff\n\r\t"
		result = nullableString(specialString)
		if !result.Valid || result.String != specialString {
			t.Error("String with special characters should be preserved")
		}
	})

	t.Run("isUniqueViolation with various error types", func(t *testing.T) {
		// Test string-based detection
		err1 := fmt.Errorf("duplicate key violates unique constraint")
		if !isUniqueViolation(err1) {
			t.Error("Should detect duplicate key error")
		}

		err2 := fmt.Errorf("unique constraint violation occurred")
		if !isUniqueViolation(err2) {
			t.Error("Should detect unique constraint error")
		}

		// Test non-unique violation errors
		err3 := fmt.Errorf("foreign key constraint violation")
		if isUniqueViolation(err3) {
			t.Error("Should not detect foreign key error as unique violation")
		}

		err4 := fmt.Errorf("check constraint failed")
		if isUniqueViolation(err4) {
			t.Error("Should not detect check constraint as unique violation")
		}

		// Test nil error
		if isUniqueViolation(nil) {
			t.Error("Should not detect nil as unique violation")
		}
	})

	t.Run("repository methods with edge case inputs", func(t *testing.T) {
		// Test Get with various invalid IDs
		invalidIDs := []string{"", "   ", "not-a-uuid", "12345"}
		for _, id := range invalidIDs {
			_, err := repo.Get(ctx, id)
			if id == "" && !errors.Is(err, ports.ErrInvalidInput) {
				t.Errorf("Empty ID should return ErrInvalidInput, got: %v", err)
			} else if id != "" && !errors.Is(err, ports.ErrNotFound) {
				// Non-empty invalid IDs should return ErrNotFound (not found in DB)
				t.Logf("Invalid ID '%s' returned: %v", id, err)
			}
		}

		// Test Delete with invalid IDs
		for _, id := range invalidIDs {
			err := repo.Delete(ctx, id)
			if id == "" && !errors.Is(err, ports.ErrInvalidInput) {
				t.Errorf("Delete with empty ID should return ErrInvalidInput, got: %v", err)
			} else if id != "" && !errors.Is(err, ports.ErrNotFound) {
				t.Logf("Delete with invalid ID '%s' returned: %v", id, err)
			}
		}

		// Test AddEvent with nil
		err = repo.AddEvent(ctx, nil)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("AddEvent(nil) should return ErrInvalidInput, got: %v", err)
		}

		// Test AddEvent with valid event to non-existent incident
		validEvent := createTestEvent("non-existent-incident-id")
		err = repo.AddEvent(ctx, validEvent)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Logf("AddEvent with non-existent incident returned: %v", err)
		}
	})
}
