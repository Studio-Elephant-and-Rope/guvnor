package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// MockIncidentRepository is a mock implementation of ports.IncidentRepository for testing.
type MockIncidentRepository struct {
	incidents   map[string]*domain.Incident
	events      map[string][]*domain.Event
	createErr   error
	getErr      error
	listErr     error
	updateErr   error
	addEventErr error
	deleteErr   error
}

// NewMockIncidentRepository creates a new mock repository.
func NewMockIncidentRepository() *MockIncidentRepository {
	return &MockIncidentRepository{
		incidents: make(map[string]*domain.Incident),
		events:    make(map[string][]*domain.Event),
	}
}

// SetCreateError sets the error to return from Create calls.
func (m *MockIncidentRepository) SetCreateError(err error) {
	m.createErr = err
}

// SetGetError sets the error to return from Get calls.
func (m *MockIncidentRepository) SetGetError(err error) {
	m.getErr = err
}

// SetListError sets the error to return from List calls.
func (m *MockIncidentRepository) SetListError(err error) {
	m.listErr = err
}

// SetUpdateError sets the error to return from Update calls.
func (m *MockIncidentRepository) SetUpdateError(err error) {
	m.updateErr = err
}

// SetAddEventError sets the error to return from AddEvent calls.
func (m *MockIncidentRepository) SetAddEventError(err error) {
	m.addEventErr = err
}

// SetDeleteError sets the error to return from Delete calls.
func (m *MockIncidentRepository) SetDeleteError(err error) {
	m.deleteErr = err
}

// Create implements ports.IncidentRepository.
func (m *MockIncidentRepository) Create(ctx context.Context, incident *domain.Incident) error {
	if m.createErr != nil {
		return m.createErr
	}

	if _, exists := m.incidents[incident.ID]; exists {
		return ports.ErrAlreadyExists
	}

	// Create a copy to avoid external modifications
	copy := *incident
	m.incidents[incident.ID] = &copy
	return nil
}

// Get implements ports.IncidentRepository.
func (m *MockIncidentRepository) Get(ctx context.Context, id string) (*domain.Incident, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}

	incident, exists := m.incidents[id]
	if !exists {
		return nil, ports.ErrNotFound
	}

	// Create a copy with events
	copy := *incident
	if events, hasEvents := m.events[id]; hasEvents {
		copy.Events = make([]domain.Event, len(events))
		for i, event := range events {
			copy.Events[i] = *event
		}
	}

	return &copy, nil
}

// List implements ports.IncidentRepository.
func (m *MockIncidentRepository) List(ctx context.Context, filter ports.ListFilter) (*ports.ListResult, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}

	// Simple implementation for testing - just return all incidents
	var incidents []*domain.Incident
	for _, incident := range m.incidents {
		incidents = append(incidents, incident)
	}

	return &ports.ListResult{
		Incidents: incidents,
		Total:     len(incidents),
		Limit:     filter.Limit,
		Offset:    filter.Offset,
		HasMore:   false,
	}, nil
}

// Update implements ports.IncidentRepository.
func (m *MockIncidentRepository) Update(ctx context.Context, incident *domain.Incident) error {
	if m.updateErr != nil {
		return m.updateErr
	}

	if _, exists := m.incidents[incident.ID]; !exists {
		return ports.ErrNotFound
	}

	// Create a copy to avoid external modifications
	copy := *incident
	m.incidents[incident.ID] = &copy
	return nil
}

// AddEvent implements ports.IncidentRepository.
func (m *MockIncidentRepository) AddEvent(ctx context.Context, event *domain.Event) error {
	if m.addEventErr != nil {
		return m.addEventErr
	}

	if _, exists := m.incidents[event.IncidentID]; !exists {
		return ports.ErrNotFound
	}

	// Create a copy to avoid external modifications
	eventCopy := *event
	m.events[event.IncidentID] = append(m.events[event.IncidentID], &eventCopy)
	return nil
}

// Delete implements ports.IncidentRepository.
func (m *MockIncidentRepository) Delete(ctx context.Context, id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}

	if _, exists := m.incidents[id]; !exists {
		return ports.ErrNotFound
	}

	delete(m.incidents, id)
	delete(m.events, id)
	return nil
}

// GetWithEvents implements ports.IncidentRepository.
func (m *MockIncidentRepository) GetWithEvents(ctx context.Context, id string) (*domain.Incident, error) {
	return m.Get(ctx, id)
}

// GetWithSignals implements ports.IncidentRepository.
func (m *MockIncidentRepository) GetWithSignals(ctx context.Context, id string) (*domain.Incident, error) {
	return m.Get(ctx, id)
}

// createTestLogger creates a logger for testing.
func createTestLogger() *logging.Logger {
	config := logging.Config{
		Environment: logging.Test,
		Level:       -8, // Debug level for comprehensive testing
		AddSource:   false,
	}
	logger, _ := logging.NewLogger(config)
	return logger
}

// createTestIncidentService creates a service with mocked dependencies for testing.
func createTestIncidentService() (*IncidentService, *MockIncidentRepository) {
	repo := NewMockIncidentRepository()
	logger := createTestLogger()
	service, _ := NewIncidentService(repo, logger)
	return service, repo
}

func TestNewIncidentService(t *testing.T) {
	logger := createTestLogger()

	t.Run("valid dependencies", func(t *testing.T) {
		repo := NewMockIncidentRepository()
		service, err := NewIncidentService(repo, logger)

		if err != nil {
			t.Errorf("NewIncidentService() error = %v, want nil", err)
		}

		if service == nil {
			t.Error("NewIncidentService() returned nil service")
		}

		if service.repo != repo {
			t.Error("Service should store the provided repository")
		}

		if service.logger != logger {
			t.Error("Service should store the provided logger")
		}

		if service.idCounter == nil {
			t.Error("Service should initialise ID counter")
		}
	})

	t.Run("nil repository", func(t *testing.T) {
		_, err := NewIncidentService(nil, logger)

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("NewIncidentService(nil repo) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("nil logger", func(t *testing.T) {
		repo := NewMockIncidentRepository()
		_, err := NewIncidentService(repo, nil)

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("NewIncidentService(nil logger) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentService_CreateIncident(t *testing.T) {
	ctx := context.Background()

	t.Run("valid incident creation", func(t *testing.T) {
		service, repo := createTestIncidentService()

		incident, err := service.CreateIncident(ctx, "Database connection failure", "critical", "team-1")

		if err != nil {
			t.Errorf("CreateIncident() error = %v, want nil", err)
		}

		if incident == nil {
			t.Fatal("CreateIncident() returned nil incident")
		}

		// Verify incident properties
		if incident.Title != "Database connection failure" {
			t.Errorf("Expected title 'Database connection failure', got %s", incident.Title)
		}

		if incident.Severity != domain.SeverityCritical {
			t.Errorf("Expected severity critical, got %s", incident.Severity)
		}

		if incident.Status != domain.StatusTriggered {
			t.Errorf("Expected status triggered, got %s", incident.Status)
		}

		if incident.ID == "" {
			t.Error("Incident should have a UUID")
		}

		if incident.Labels["incident_id"] == "" {
			t.Error("Incident should have human-readable ID in labels")
		}

		if !strings.HasPrefix(incident.Labels["incident_id"], "INC-") {
			t.Errorf("Human ID should start with INC-, got %s", incident.Labels["incident_id"])
		}

		// Verify incident was stored
		stored, err := repo.Get(ctx, incident.ID)
		if err != nil {
			t.Errorf("Failed to retrieve stored incident: %v", err)
		}

		if stored.Title != incident.Title {
			t.Error("Stored incident should match created incident")
		}

		// Verify creation event was recorded
		if len(repo.events[incident.ID]) == 0 {
			t.Error("Creation event should have been recorded")
		}

		event := repo.events[incident.ID][0]
		if event.Type != "incident_created" {
			t.Errorf("Expected event type 'incident_created', got %s", event.Type)
		}
	})

	t.Run("empty title", func(t *testing.T) {
		service, _ := createTestIncidentService()

		_, err := service.CreateIncident(ctx, "", "high", "team-1")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("CreateIncident(empty title) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("title too long", func(t *testing.T) {
		service, _ := createTestIncidentService()
		longTitle := strings.Repeat("a", 201)

		_, err := service.CreateIncident(ctx, longTitle, "high", "team-1")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("CreateIncident(long title) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("invalid severity", func(t *testing.T) {
		service, _ := createTestIncidentService()

		_, err := service.CreateIncident(ctx, "Test incident", "invalid", "team-1")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("CreateIncident(invalid severity) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("empty team ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		_, err := service.CreateIncident(ctx, "Test incident", "high", "")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("CreateIncident(empty team ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetCreateError(ports.ErrConnectionFailed)

		_, err := service.CreateIncident(ctx, "Test incident", "high", "team-1")

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("CreateIncident(repo error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})

	t.Run("event recording failure doesn't fail creation", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetAddEventError(ports.ErrConnectionFailed)

		incident, err := service.CreateIncident(ctx, "Test incident", "medium", "team-1")

		if err != nil {
			t.Errorf("CreateIncident() should succeed even if event recording fails, got error: %v", err)
		}

		if incident == nil {
			t.Error("Incident should be created even if event recording fails")
		}
	})
}

func TestIncidentService_GetIncident(t *testing.T) {
	ctx := context.Background()

	t.Run("existing incident", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			TeamID:    "team-1",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		incident, err := service.GetIncident(ctx, "test-id")

		if err != nil {
			t.Errorf("GetIncident() error = %v, want nil", err)
		}

		if incident == nil {
			t.Fatal("GetIncident() returned nil incident")
		}

		if incident.ID != "test-id" {
			t.Errorf("Expected ID 'test-id', got %s", incident.ID)
		}
	})

	t.Run("empty ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		_, err := service.GetIncident(ctx, "")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("GetIncident(empty ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("non-existent incident", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetGetError(ports.ErrNotFound)

		_, err := service.GetIncident(ctx, "non-existent")

		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("GetIncident(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetGetError(ports.ErrConnectionFailed)

		_, err := service.GetIncident(ctx, "test-id")

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("GetIncident(repo error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})
}

func TestIncidentService_ListIncidents(t *testing.T) {
	ctx := context.Background()

	t.Run("valid filter", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Add test incidents
		for i := 0; i < 3; i++ {
			incident := &domain.Incident{
				ID:        fmt.Sprintf("test-id-%d", i),
				Title:     fmt.Sprintf("Test incident %d", i),
				Status:    domain.StatusTriggered,
				TeamID:    "team-1",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			repo.incidents[incident.ID] = incident
		}

		filter := ports.ListFilter{
			Limit:  10,
			Offset: 0,
		}

		result, err := service.ListIncidents(ctx, filter)

		if err != nil {
			t.Errorf("ListIncidents() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("ListIncidents() returned nil result")
		}

		if result.Total != 3 {
			t.Errorf("Expected total 3, got %d", result.Total)
		}

		if len(result.Incidents) != 3 {
			t.Errorf("Expected 3 incidents, got %d", len(result.Incidents))
		}
	})

	t.Run("normalised filter", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Add test incidents
		for i := 0; i < 3; i++ {
			incident := &domain.Incident{
				ID:        fmt.Sprintf("test-id-%d", i),
				Title:     fmt.Sprintf("Test incident %d", i),
				Status:    domain.StatusTriggered,
				Severity:  domain.SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			repo.incidents[incident.ID] = incident
		}

		filter := ports.ListFilter{
			Limit:  -1, // Should be normalised to 50
			Offset: 0,
		}

		result, err := service.ListIncidents(ctx, filter)

		if err != nil {
			t.Errorf("ListIncidents(negative limit) should succeed after normalisation, got error: %v", err)
		}

		if result == nil {
			t.Error("Result should not be nil when filter is normalised")
		}

		if result != nil && result.Limit != 50 {
			t.Errorf("Expected normalised limit of 50, got %d", result.Limit)
		}
	})

	t.Run("repository error", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetListError(ports.ErrConnectionFailed)

		filter := ports.ListFilter{}

		_, err := service.ListIncidents(ctx, filter)

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("ListIncidents(repo error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})
}

func TestIncidentService_UpdateStatus(t *testing.T) {
	ctx := context.Background()

	t.Run("valid status transition", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			TeamID:    "team-1",
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		err := service.UpdateStatus(ctx, "test-id", domain.StatusAcknowledged)

		if err != nil {
			t.Errorf("UpdateStatus() error = %v, want nil", err)
		}

		// Verify status was updated
		updated, _ := repo.Get(ctx, "test-id")
		if updated.Status != domain.StatusAcknowledged {
			t.Errorf("Expected status acknowledged, got %s", updated.Status)
		}

		// Verify status change event was recorded
		if len(repo.events["test-id"]) == 0 {
			t.Error("Status change event should have been recorded")
		}

		event := repo.events["test-id"][0]
		if event.Type != "status_changed" {
			t.Errorf("Expected event type 'status_changed', got %s", event.Type)
		}
	})

	t.Run("empty incident ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		err := service.UpdateStatus(ctx, "", domain.StatusAcknowledged)

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("UpdateStatus(empty ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		service, _ := createTestIncidentService()

		err := service.UpdateStatus(ctx, "test-id", domain.Status("invalid"))

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("UpdateStatus(invalid status) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("non-existent incident", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetGetError(ports.ErrNotFound)

		err := service.UpdateStatus(ctx, "non-existent", domain.StatusAcknowledged)

		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("UpdateStatus(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident in resolved status
		testIncident := &domain.Incident{
			ID:         "test-id",
			Title:      "Test incident",
			Status:     domain.StatusResolved,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
			ResolvedAt: func() *time.Time { t := time.Now().UTC(); return &t }(),
		}
		repo.incidents[testIncident.ID] = testIncident

		// Try to transition from resolved to acknowledged (invalid)
		err := service.UpdateStatus(ctx, "test-id", domain.StatusAcknowledged)

		if !errors.Is(err, ports.ErrConflict) {
			t.Errorf("UpdateStatus(invalid transition) error = %v, want %v", err, ports.ErrConflict)
		}
	})

	t.Run("repository update error", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident
		repo.SetUpdateError(ports.ErrConnectionFailed)

		err := service.UpdateStatus(ctx, "test-id", domain.StatusAcknowledged)

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("UpdateStatus(update error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})

	t.Run("event recording failure doesn't fail update", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident
		repo.SetAddEventError(ports.ErrConnectionFailed)

		err := service.UpdateStatus(ctx, "test-id", domain.StatusAcknowledged)

		if err != nil {
			t.Errorf("UpdateStatus() should succeed even if event recording fails, got error: %v", err)
		}

		// Verify status was still updated
		updated, _ := repo.Get(ctx, "test-id")
		if updated.Status != domain.StatusAcknowledged {
			t.Error("Status should be updated even if event recording fails")
		}
	})
}

func TestIncidentService_AddResponder(t *testing.T) {
	ctx := context.Background()

	t.Run("valid responder assignment", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		err := service.AddResponder(ctx, "test-id", "user-123")

		if err != nil {
			t.Errorf("AddResponder() error = %v, want nil", err)
		}

		// Verify responder was assigned
		updated, _ := repo.Get(ctx, "test-id")
		if updated.AssigneeID != "user-123" {
			t.Errorf("Expected assignee 'user-123', got %s", updated.AssigneeID)
		}

		// Verify responder assignment event was recorded
		if len(repo.events["test-id"]) == 0 {
			t.Error("Responder assignment event should have been recorded")
		}

		event := repo.events["test-id"][0]
		if event.Type != "responder_assigned" {
			t.Errorf("Expected event type 'responder_assigned', got %s", event.Type)
		}
	})

	t.Run("responder change", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident with existing assignee
		testIncident := &domain.Incident{
			ID:         "test-id",
			Title:      "Test incident",
			Status:     domain.StatusTriggered,
			AssigneeID: "old-user",
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		err := service.AddResponder(ctx, "test-id", "new-user")

		if err != nil {
			t.Errorf("AddResponder() error = %v, want nil", err)
		}

		// Verify responder was changed
		updated, _ := repo.Get(ctx, "test-id")
		if updated.AssigneeID != "new-user" {
			t.Errorf("Expected assignee 'new-user', got %s", updated.AssigneeID)
		}

		// Verify responder change event was recorded
		if len(repo.events["test-id"]) == 0 {
			t.Error("Responder change event should have been recorded")
		}

		event := repo.events["test-id"][0]
		if event.Type != "responder_changed" {
			t.Errorf("Expected event type 'responder_changed', got %s", event.Type)
		}
	})

	t.Run("empty incident ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		err := service.AddResponder(ctx, "", "user-123")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("AddResponder(empty incident ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("empty responder ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		err := service.AddResponder(ctx, "test-id", "")

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("AddResponder(empty responder ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("non-existent incident", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetGetError(ports.ErrNotFound)

		err := service.AddResponder(ctx, "non-existent", "user-123")

		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("AddResponder(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("repository update error", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident
		repo.SetUpdateError(ports.ErrConnectionFailed)

		err := service.AddResponder(ctx, "test-id", "user-123")

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("AddResponder(update error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})
}

func TestIncidentService_RecordEvent(t *testing.T) {
	ctx := context.Background()

	t.Run("valid event recording", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		event := domain.Event{
			Type:        "investigation_started",
			Actor:       "user@example.com",
			Description: "Started investigating the database issue",
			Metadata: map[string]interface{}{
				"action": "investigation",
				"tool":   "datadog",
			},
		}

		err := service.RecordEvent(ctx, "test-id", event)

		if err != nil {
			t.Errorf("RecordEvent() error = %v, want nil", err)
		}

		// Verify event was recorded
		if len(repo.events["test-id"]) == 0 {
			t.Error("Event should have been recorded")
		}

		recorded := repo.events["test-id"][0]
		if recorded.Type != "investigation_started" {
			t.Errorf("Expected event type 'investigation_started', got %s", recorded.Type)
		}

		if recorded.IncidentID != "test-id" {
			t.Errorf("Expected incident ID 'test-id', got %s", recorded.IncidentID)
		}

		if recorded.ID == "" {
			t.Error("Event should have generated ID")
		}

		if recorded.OccurredAt.IsZero() {
			t.Error("Event should have timestamp")
		}
	})

	t.Run("event with preset ID and timestamp", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		presetTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		event := domain.Event{
			ID:          "preset-id",
			Type:        "note_added",
			Actor:       "user@example.com",
			Description: "Added a note",
			OccurredAt:  presetTime,
		}

		err := service.RecordEvent(ctx, "test-id", event)

		if err != nil {
			t.Errorf("RecordEvent() error = %v, want nil", err)
		}

		// Verify preset values were preserved
		recorded := repo.events["test-id"][0]
		if recorded.ID != "preset-id" {
			t.Errorf("Expected preset ID 'preset-id', got %s", recorded.ID)
		}

		if !recorded.OccurredAt.Equal(presetTime) {
			t.Errorf("Expected preset timestamp, got %v", recorded.OccurredAt)
		}
	})

	t.Run("empty incident ID", func(t *testing.T) {
		service, _ := createTestIncidentService()

		event := domain.Event{
			Type:        "test_event",
			Actor:       "user@example.com",
			Description: "Test event",
		}

		err := service.RecordEvent(ctx, "", event)

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("RecordEvent(empty incident ID) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("invalid event", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident

		event := domain.Event{
			// Missing required fields
			Description: "Test event",
		}

		err := service.RecordEvent(ctx, "test-id", event)

		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("RecordEvent(invalid event) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("non-existent incident", func(t *testing.T) {
		service, repo := createTestIncidentService()
		repo.SetGetError(ports.ErrNotFound)

		event := domain.Event{
			Type:        "test_event",
			Actor:       "user@example.com",
			Description: "Test event",
		}

		err := service.RecordEvent(ctx, "non-existent", event)

		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("RecordEvent(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("repository add event error", func(t *testing.T) {
		service, repo := createTestIncidentService()

		// Create test incident
		testIncident := &domain.Incident{
			ID:        "test-id",
			Title:     "Test incident",
			Status:    domain.StatusTriggered,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		repo.incidents[testIncident.ID] = testIncident
		repo.SetAddEventError(ports.ErrConnectionFailed)

		event := domain.Event{
			Type:        "test_event",
			Actor:       "user@example.com",
			Description: "Test event",
		}

		err := service.RecordEvent(ctx, "test-id", event)

		if !errors.Is(err, ports.ErrConnectionFailed) {
			t.Errorf("RecordEvent(add event error) error = %v, want %v", err, ports.ErrConnectionFailed)
		}
	})
}

func TestIncidentService_GenerateHumanReadableID(t *testing.T) {
	service, _ := createTestIncidentService()

	t.Run("sequential ID generation", func(t *testing.T) {
		id1 := service.generateHumanReadableID()
		id2 := service.generateHumanReadableID()

		// Both should start with INC-
		if !strings.HasPrefix(id1, "INC-") {
			t.Errorf("ID should start with INC-, got %s", id1)
		}

		if !strings.HasPrefix(id2, "INC-") {
			t.Errorf("ID should start with INC-, got %s", id2)
		}

		// Should be sequential
		if id1 == id2 {
			t.Error("Sequential IDs should be different")
		}

		// Should contain current year
		currentYear := fmt.Sprintf("%d", time.Now().UTC().Year())
		if !strings.Contains(id1, currentYear) {
			t.Errorf("ID should contain current year %s, got %s", currentYear, id1)
		}
	})

	t.Run("year rollover handling", func(t *testing.T) {
		// Manually set year in the past to test rollover
		service.idCounter.year = 2023
		service.idCounter.sequence = 100

		id := service.generateHumanReadableID()

		currentYear := time.Now().UTC().Year()
		expectedPrefix := fmt.Sprintf("INC-%d-001", currentYear)

		if id != expectedPrefix {
			t.Errorf("Expected %s after year rollover, got %s", expectedPrefix, id)
		}

		// Verify counter was reset
		if service.idCounter.sequence != 1 {
			t.Errorf("Expected sequence 1 after year rollover, got %d", service.idCounter.sequence)
		}

		if service.idCounter.year != currentYear {
			t.Errorf("Expected year %d after rollover, got %d", currentYear, service.idCounter.year)
		}
	})

	t.Run("large sequence number formatting", func(t *testing.T) {
		service.idCounter.sequence = 999

		id := service.generateHumanReadableID()
		if !strings.HasSuffix(id, "1000") {
			t.Errorf("Expected ID to end with 1000, got %s", id)
		}
	})
}

func TestIncidentService_FormatSequence(t *testing.T) {
	service, _ := createTestIncidentService()

	tests := []struct {
		input    int
		expected string
	}{
		{1, "001"},
		{10, "010"},
		{100, "100"},
		{999, "999"},
		{1000, "1000"},
		{1234, "1234"},
	}

	for _, test := range tests {
		t.Run(fmt.Sprintf("sequence_%d", test.input), func(t *testing.T) {
			result := service.formatSequence(test.input)
			if result != test.expected {
				t.Errorf("formatSequence(%d) = %s, want %s", test.input, result, test.expected)
			}
		})
	}
}

func TestIncidentService_ConcurrentIDGeneration(t *testing.T) {
	service, _ := createTestIncidentService()

	const numGoroutines = 100
	ids := make(chan string, numGoroutines)

	// Generate IDs concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			ids <- service.generateHumanReadableID()
		}()
	}

	// Collect all IDs
	generatedIDs := make(map[string]bool)
	for i := 0; i < numGoroutines; i++ {
		id := <-ids
		if generatedIDs[id] {
			t.Errorf("Duplicate ID generated: %s", id)
		}
		generatedIDs[id] = true
	}

	// Verify we got all unique IDs
	if len(generatedIDs) != numGoroutines {
		t.Errorf("Expected %d unique IDs, got %d", numGoroutines, len(generatedIDs))
	}
}
