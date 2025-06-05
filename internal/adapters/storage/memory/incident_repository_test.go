package memory

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
)

// createTestIncident creates a valid incident for testing.
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
		Labels:      map[string]string{"env": "production", "service": "api"},
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// createTestEvent creates a valid event for testing.
func createTestEvent(incidentID string) *domain.Event {
	return &domain.Event{
		ID:          "event-1",
		IncidentID:  incidentID,
		Type:        "status_changed",
		Actor:       "user@example.com",
		Description: "Incident status changed",
		Metadata:    map[string]interface{}{"old_status": "triggered", "new_status": "acknowledged"},
		OccurredAt:  time.Now().UTC(),
	}
}

func TestNewIncidentRepository(t *testing.T) {
	repo := NewIncidentRepository()

	if repo == nil {
		t.Fatal("NewIncidentRepository() returned nil")
	}

	if repo.incidents == nil {
		t.Error("Expected incidents map to be initialized")
	}

	if repo.events == nil {
		t.Error("Expected events map to be initialized")
	}

	if repo.nextID != 1 {
		t.Errorf("Expected nextID to be 1, got %d", repo.nextID)
	}

	if repo.Count() != 0 {
		t.Errorf("Expected empty repository, got count %d", repo.Count())
	}
}

func TestIncidentRepository_Create(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	t.Run("create valid incident", func(t *testing.T) {
		incident := createTestIncident("incident-1")
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Create() error = %v, want nil", err)
		}

		if repo.Count() != 1 {
			t.Errorf("Expected count 1 after create, got %d", repo.Count())
		}
	})

	t.Run("create nil incident", func(t *testing.T) {
		err := repo.Create(ctx, nil)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Create(nil) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("create invalid incident", func(t *testing.T) {
		incident := &domain.Incident{} // Missing required fields
		err := repo.Create(ctx, incident)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Create(invalid) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("create duplicate incident", func(t *testing.T) {
		incident := createTestIncident("incident-2")

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

	t.Run("create with cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		incident := createTestIncident("incident-3")
		err := repo.Create(cancelledCtx, incident)
		if err != context.Canceled {
			t.Errorf("Create(cancelled context) error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestIncidentRepository_Get(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incident
	original := createTestIncident("incident-1")
	err := repo.Create(ctx, original)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("get existing incident", func(t *testing.T) {
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Errorf("Get() error = %v, want nil", err)
		}

		if retrieved == nil {
			t.Fatal("Get() returned nil incident")
		}

		if retrieved.ID != original.ID {
			t.Errorf("Expected ID %s, got %s", original.ID, retrieved.ID)
		}

		if retrieved.Title != original.Title {
			t.Errorf("Expected title %s, got %s", original.Title, retrieved.Title)
		}

		// Verify it's a copy (different pointer)
		if retrieved == original {
			t.Error("Get() should return a copy, not the original")
		}
	})

	t.Run("get non-existent incident", func(t *testing.T) {
		_, err := repo.Get(ctx, "non-existent")
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

	t.Run("get with cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		_, err := repo.Get(cancelledCtx, "incident-1")
		if err != context.Canceled {
			t.Errorf("Get(cancelled context) error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestIncidentRepository_Update(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incident
	original := createTestIncident("incident-1")
	err := repo.Create(ctx, original)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("update existing incident", func(t *testing.T) {
		updated := createTestIncident("incident-1")
		updated.Title = "Updated title"
		updated.Status = domain.StatusAcknowledged
		updated.UpdatedAt = time.Now().UTC()

		err := repo.Update(ctx, updated)
		if err != nil {
			t.Errorf("Update() error = %v, want nil", err)
		}

		// Verify update
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get after update failed: %v", err)
		}

		if retrieved.Title != "Updated title" {
			t.Errorf("Expected updated title, got %s", retrieved.Title)
		}

		if retrieved.Status != domain.StatusAcknowledged {
			t.Errorf("Expected updated status, got %s", retrieved.Status)
		}
	})

	t.Run("update non-existent incident", func(t *testing.T) {
		incident := createTestIncident("non-existent")
		err := repo.Update(ctx, incident)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Update(non-existent) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("update nil incident", func(t *testing.T) {
		err := repo.Update(ctx, nil)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Update(nil) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("update invalid incident", func(t *testing.T) {
		invalid := &domain.Incident{ID: "incident-1"} // Missing required fields
		err := repo.Update(ctx, invalid)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("Update(invalid) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentRepository_Delete(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incident
	incident := createTestIncident("incident-1")
	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("delete existing incident", func(t *testing.T) {
		err := repo.Delete(ctx, "incident-1")
		if err != nil {
			t.Errorf("Delete() error = %v, want nil", err)
		}

		// Verify deletion
		_, err = repo.Get(ctx, "incident-1")
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("Expected incident to be deleted, but Get() error = %v", err)
		}

		if repo.Count() != 0 {
			t.Errorf("Expected count 0 after delete, got %d", repo.Count())
		}
	})

	t.Run("delete non-existent incident", func(t *testing.T) {
		err := repo.Delete(ctx, "non-existent")
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

func TestIncidentRepository_AddEvent(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incident
	incident := createTestIncident("incident-1")
	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("add valid event", func(t *testing.T) {
		event := createTestEvent("incident-1")
		err := repo.AddEvent(ctx, event)
		if err != nil {
			t.Errorf("AddEvent() error = %v, want nil", err)
		}

		// Verify event was added
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get after AddEvent failed: %v", err)
		}

		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(retrieved.Events))
		}

		if retrieved.Events[0].Type != "status_changed" {
			t.Errorf("Expected event type 'status_changed', got %s", retrieved.Events[0].Type)
		}
	})

	t.Run("add event to non-existent incident", func(t *testing.T) {
		event := createTestEvent("non-existent")
		err := repo.AddEvent(ctx, event)
		if !errors.Is(err, ports.ErrNotFound) {
			t.Errorf("AddEvent(non-existent incident) error = %v, want %v", err, ports.ErrNotFound)
		}
	})

	t.Run("add nil event", func(t *testing.T) {
		err := repo.AddEvent(ctx, nil)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("AddEvent(nil) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("add invalid event", func(t *testing.T) {
		event := &domain.Event{
			IncidentID: "incident-1", // Valid incident ID
			// Missing other required fields like Type, Actor, etc.
		}
		err := repo.AddEvent(ctx, event)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("AddEvent(invalid) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})

	t.Run("add event without ID generates ID", func(t *testing.T) {
		// Get current event count
		beforeAdd, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get before AddEvent failed: %v", err)
		}
		beforeCount := len(beforeAdd.Events)

		event := createTestEvent("incident-1")
		event.ID = "" // Clear ID to test generation

		err = repo.AddEvent(ctx, event)
		if err != nil {
			t.Errorf("AddEvent() error = %v, want nil", err)
		}

		// Verify event was added with generated ID
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get after AddEvent failed: %v", err)
		}

		if len(retrieved.Events) != beforeCount+1 {
			t.Errorf("Expected %d events, got %d", beforeCount+1, len(retrieved.Events))
		}

		// Find the event we just added (should have generated ID that starts with "event-")
		foundGenerated := false
		for _, e := range retrieved.Events {
			if strings.HasPrefix(e.ID, "event-") && e.Type == "status_changed" {
				foundGenerated = true
				break
			}
		}
		if !foundGenerated {
			t.Error("Expected to find event with generated ID starting with 'event-'")
		}
	})
}

func TestIncidentRepository_List(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incidents
	now := time.Now().UTC()
	incidents := []*domain.Incident{
		{
			ID:        "incident-1",
			Title:     "Critical database issue",
			Status:    domain.StatusTriggered,
			Severity:  domain.SeverityCritical,
			TeamID:    "team-1",
			Labels:    map[string]string{"env": "production"},
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "incident-2",
			Title:     "API performance issue",
			Status:    domain.StatusAcknowledged,
			Severity:  domain.SeverityHigh,
			TeamID:    "team-1",
			Labels:    map[string]string{"env": "production", "service": "api"},
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "incident-3",
			Title:     "Minor logging issue",
			Status:    domain.StatusResolved,
			Severity:  domain.SeverityLow,
			TeamID:    "team-2",
			Labels:    map[string]string{"env": "staging"},
			CreatedAt: now.Add(-30 * time.Minute),
			UpdatedAt: now.Add(-10 * time.Minute),
			ResolvedAt: func() *time.Time { t := now.Add(-10 * time.Minute); return &t }(),
		},
	}

	for _, incident := range incidents {
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
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

		if result.Limit != 50 {
			t.Errorf("Expected default limit 50, got %d", result.Limit)
		}

		if result.HasMore {
			t.Error("Expected HasMore to be false")
		}
	})

	t.Run("filter by team", func(t *testing.T) {
		filter := ports.ListFilter{TeamID: "team-1"}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 2 {
			t.Errorf("Expected total 2 for team-1, got %d", result.Total)
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

	t.Run("filter by severity", func(t *testing.T) {
		filter := ports.ListFilter{Severity: []domain.Severity{domain.SeverityCritical, domain.SeverityHigh}}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if result.Total != 2 {
			t.Errorf("Expected total 2 for critical/high severity, got %d", result.Total)
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

		// Second page
		filter.Offset = 2
		result, err = repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		if len(result.Incidents) != 1 {
			t.Errorf("Expected 1 incident in second page, got %d", len(result.Incidents))
		}

		if result.HasMore {
			t.Error("Expected HasMore to be false for last page")
		}
	})

	t.Run("sort by severity", func(t *testing.T) {
		filter := ports.ListFilter{SortBy: "severity", SortOrder: "asc"}
		result, err := repo.List(ctx, filter)
		if err != nil {
			t.Errorf("List() error = %v, want nil", err)
		}

		// Should be sorted: critical, high, low
		expectedOrder := []domain.Severity{domain.SeverityCritical, domain.SeverityHigh, domain.SeverityLow}
		for i, incident := range result.Incidents {
			if incident.Severity != expectedOrder[i] {
				t.Errorf("Expected severity %s at position %d, got %s", expectedOrder[i], i, incident.Severity)
			}
		}
	})

	t.Run("invalid filter", func(t *testing.T) {
		filter := ports.ListFilter{Offset: -1} // Negative offset should fail
		_, err := repo.List(ctx, filter)
		if !errors.Is(err, ports.ErrInvalidInput) {
			t.Errorf("List(invalid filter) error = %v, want %v", err, ports.ErrInvalidInput)
		}
	})
}

func TestIncidentRepository_GetWithEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create incident and add event
	incident := createTestIncident("incident-1")
	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	event := createTestEvent("incident-1")
	err = repo.AddEvent(ctx, event)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("get with events", func(t *testing.T) {
		retrieved, err := repo.GetWithEvents(ctx, "incident-1")
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
	repo := NewIncidentRepository()

	// Create incident with signals
	incident := createTestIncident("incident-1")
	incident.Signals = []domain.Signal{
		{
			ID:         "signal-1",
			Source:     "prometheus",
			Title:      "High CPU",
			Severity:   domain.SeverityHigh,
			ReceivedAt: time.Now().UTC(),
		},
	}

	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("get with signals", func(t *testing.T) {
		retrieved, err := repo.GetWithSignals(ctx, "incident-1")
		if err != nil {
			t.Errorf("GetWithSignals() error = %v, want nil", err)
		}

		if len(retrieved.Signals) != 1 {
			t.Errorf("Expected 1 signal, got %d", len(retrieved.Signals))
		}
	})
}

func TestIncidentRepository_Clear(t *testing.T) {
	repo := NewIncidentRepository()

	// Add some data
	incident := createTestIncident("incident-1")
	err := repo.Create(context.Background(), incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	if repo.Count() != 1 {
		t.Fatalf("Expected count 1 after create, got %d", repo.Count())
	}

	// Clear repository
	repo.Clear()

	if repo.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", repo.Count())
	}

	// Verify data is actually gone
	_, err = repo.Get(context.Background(), "incident-1")
	if !errors.Is(err, ports.ErrNotFound) {
		t.Errorf("Expected incident to be gone after clear, but Get() error = %v", err)
	}
}

func TestIncidentRepository_Concurrency(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Test concurrent operations
	const numGoroutines = 10
	const numOperations = 100

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*numOperations)

	// Concurrent creates
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				incident := createTestIncident(fmt.Sprintf("incident-%d-%d", workerID, j))
				if err := repo.Create(ctx, incident); err != nil {
					errors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation failed: %v", err)
	}

	// Verify all incidents were created
	expectedCount := numGoroutines * numOperations
	if repo.Count() != expectedCount {
		t.Errorf("Expected count %d after concurrent creates, got %d", expectedCount, repo.Count())
	}

	// Test concurrent reads
	wg.Add(numGoroutines)
	readErrors := make(chan error, numGoroutines*numOperations)

	for i := 0; i < numGoroutines; i++ {
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				id := fmt.Sprintf("incident-%d-%d", workerID, j)
				if _, err := repo.Get(ctx, id); err != nil {
					readErrors <- err
				}
			}
		}(i)
	}

	wg.Wait()
	close(readErrors)

	// Check for read errors
	for err := range readErrors {
		t.Errorf("Concurrent read failed: %v", err)
	}
}

func TestIncidentRepository_DeepCopy(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create incident with complex data
	incident := createTestIncident("incident-1")
	incident.Labels = map[string]string{"env": "production"}
	incident.Signals = []domain.Signal{
		{
			ID:          "signal-1",
			Source:      "prometheus",
			Title:       "High CPU",
			Severity:    domain.SeverityHigh,
			Labels:      map[string]string{"host": "web-01"},
			Annotations: map[string]string{"runbook": "http://example.com"},
			Payload:     map[string]interface{}{"value": 85.5},
			ReceivedAt:  time.Now().UTC(),
		},
	}

	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	// Get the incident
	retrieved, err := repo.Get(ctx, "incident-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Modify retrieved incident
	retrieved.Labels["env"] = "staging"
	retrieved.Signals[0].Labels["host"] = "web-02"

	// Get again and verify original data wasn't modified
	retrieved2, err := repo.Get(ctx, "incident-1")
	if err != nil {
		t.Fatalf("Second get failed: %v", err)
	}

	if retrieved2.Labels["env"] != "production" {
		t.Error("Original incident labels were modified (not deep copied)")
	}

	if retrieved2.Signals[0].Labels["host"] != "web-01" {
		t.Error("Original signal labels were modified (not deep copied)")
	}
}

// Additional comprehensive test cases to improve coverage

func TestIncidentRepository_CreateWithEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	t.Run("create incident with events", func(t *testing.T) {
		incident := createTestIncident("incident-1")
		incident.Events = []domain.Event{
			{
				ID:          "event-1",
				IncidentID:  "incident-1",
				Type:        "created",
				Actor:       "system",
				Description: "Incident created",
				OccurredAt:  time.Now().UTC(),
			},
		}

		err := repo.Create(ctx, incident)
		if err != nil {
			t.Errorf("Create with events failed: %v", err)
		}

		// Verify events were stored
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get after create failed: %v", err)
		}

		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(retrieved.Events))
		}
	})
}

func TestIncidentRepository_UpdateWithEvents(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create initial incident
	incident := createTestIncident("incident-1")
	err := repo.Create(ctx, incident)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}

	t.Run("update with events", func(t *testing.T) {
		updated := createTestIncident("incident-1")
		updated.Title = "Updated title"
		updated.Events = []domain.Event{
			{
				ID:          "event-update-1",
				IncidentID:  "incident-1",
				Type:        "updated",
				Actor:       "user@example.com",
				Description: "Incident updated",
				OccurredAt:  time.Now().UTC(),
			},
		}

		err := repo.Update(ctx, updated)
		if err != nil {
			t.Errorf("Update with events failed: %v", err)
		}

		// Verify events were updated
		retrieved, err := repo.Get(ctx, "incident-1")
		if err != nil {
			t.Fatalf("Get after update failed: %v", err)
		}

		if len(retrieved.Events) != 1 {
			t.Errorf("Expected 1 event after update, got %d", len(retrieved.Events))
		}

		if retrieved.Events[0].Type != "updated" {
			t.Errorf("Expected event type 'updated', got %s", retrieved.Events[0].Type)
		}
	})

	t.Run("update with cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		updated := createTestIncident("incident-1")
		err := repo.Update(cancelledCtx, updated)
		if err != context.Canceled {
			t.Errorf("Update with cancelled context error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestIncidentRepository_ContextCancellation(t *testing.T) {
	repo := NewIncidentRepository()

	t.Run("AddEvent with cancelled context", func(t *testing.T) {
		// Create incident first
		incident := createTestIncident("incident-1")
		err := repo.Create(context.Background(), incident)
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}

		// Test with cancelled context
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		event := createTestEvent("incident-1")
		err = repo.AddEvent(cancelledCtx, event)
		if err != context.Canceled {
			t.Errorf("AddEvent with cancelled context error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("Delete with cancelled context", func(t *testing.T) {
		// Create incident first
		incident := createTestIncident("incident-2")
		err := repo.Create(context.Background(), incident)
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}

		// Test with cancelled context
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err = repo.Delete(cancelledCtx, "incident-2")
		if err != context.Canceled {
			t.Errorf("Delete with cancelled context error = %v, want %v", err, context.Canceled)
		}
	})

	t.Run("List with cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		filter := ports.ListFilter{}
		_, err := repo.List(cancelledCtx, filter)
		if err != context.Canceled {
			t.Errorf("List with cancelled context error = %v, want %v", err, context.Canceled)
		}
	})
}

func TestIncidentRepository_SortingEdgeCases(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create test incidents with different timestamps and statuses
	now := time.Now().UTC()
	incidents := []*domain.Incident{
		{
			ID:        "incident-1",
			Title:     "First incident",
			Status:    domain.StatusTriggered,
			Severity:  domain.SeverityCritical,
			TeamID:    "team-1",
			CreatedAt: now.Add(-3 * time.Hour),
			UpdatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "incident-2",
			Title:     "Second incident",
			Status:    domain.StatusAcknowledged,
			Severity:  domain.SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now.Add(-2 * time.Hour),
			UpdatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "incident-3",
			Title:     "Third incident",
			Status:    domain.StatusInvestigating,
			Severity:  domain.SeverityMedium,
			TeamID:    "team-1",
			CreatedAt: now.Add(-1 * time.Hour),
			UpdatedAt: now.Add(-30 * time.Minute),
		},
		{
			ID:          "incident-4",
			Title:       "Fourth incident",
			Status:      domain.StatusResolved,
			Severity:    domain.SeverityLow,
			TeamID:      "team-1",
			CreatedAt:   now.Add(-30 * time.Minute),
			UpdatedAt:   now.Add(-10 * time.Minute),
			ResolvedAt:  func() *time.Time { t := now.Add(-10 * time.Minute); return &t }(),
		},
	}

	for _, incident := range incidents {
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	testCases := []struct {
		name        string
		sortBy      string
		sortOrder   string
		expectedIDs []string
	}{
		{
			name:        "sort by created_at asc",
			sortBy:      "created_at",
			sortOrder:   "asc",
			expectedIDs: []string{"incident-1", "incident-2", "incident-3", "incident-4"},
		},
		{
			name:        "sort by created_at desc",
			sortBy:      "created_at",
			sortOrder:   "desc",
			expectedIDs: []string{"incident-4", "incident-3", "incident-2", "incident-1"},
		},
		{
			name:        "sort by updated_at asc",
			sortBy:      "updated_at",
			sortOrder:   "asc",
			expectedIDs: []string{"incident-2", "incident-1", "incident-3", "incident-4"},
		},
		{
			name:        "sort by updated_at desc",
			sortBy:      "updated_at",
			sortOrder:   "desc",
			expectedIDs: []string{"incident-4", "incident-3", "incident-1", "incident-2"},
		},
		{
			name:        "sort by status asc",
			sortBy:      "status",
			sortOrder:   "asc",
			expectedIDs: []string{"incident-1", "incident-2", "incident-3", "incident-4"},
		},
		{
			name:        "sort by status desc",
			sortBy:      "status",
			sortOrder:   "desc",
			expectedIDs: []string{"incident-4", "incident-3", "incident-2", "incident-1"},
		},
		{
			name:        "sort by severity desc",
			sortBy:      "severity",
			sortOrder:   "desc",
			expectedIDs: []string{"incident-4", "incident-3", "incident-2", "incident-1"},
		},
		{
			name:        "sort by empty field defaults to created_at",
			sortBy:      "",
			sortOrder:   "asc",
			expectedIDs: []string{"incident-1", "incident-2", "incident-3", "incident-4"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := ports.ListFilter{
				SortBy:    tc.sortBy,
				SortOrder: tc.sortOrder,
			}

			result, err := repo.List(ctx, filter)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}

			if len(result.Incidents) != len(tc.expectedIDs) {
				t.Fatalf("Expected %d incidents, got %d", len(tc.expectedIDs), len(result.Incidents))
			}

			for i, expectedID := range tc.expectedIDs {
				if result.Incidents[i].ID != expectedID {
					t.Errorf("Position %d: expected ID %s, got %s", i, expectedID, result.Incidents[i].ID)
				}
			}
		})
	}
}

func TestIncidentRepository_FilteringEdgeCases(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create incidents with various properties
	now := time.Now().UTC()
	incidents := []*domain.Incident{
		{
			ID:          "incident-1",
			Title:       "Database connection issue",
			Description: "Cannot connect to primary database",
			Status:      domain.StatusTriggered,
			Severity:    domain.SeverityCritical,
			TeamID:      "team-db",
			AssigneeID:  "user-1",
			ServiceID:   "database-service",
			Labels:      map[string]string{"env": "production", "component": "database"},
			CreatedAt:   now.Add(-2 * time.Hour),
			UpdatedAt:   now.Add(-1 * time.Hour),
		},
		{
			ID:          "incident-2",
			Title:       "API rate limiting",
			Description: "API is rate limiting requests",
			Status:      domain.StatusAcknowledged,
			Severity:    domain.SeverityHigh,
			TeamID:      "team-api",
			AssigneeID:  "user-2",
			ServiceID:   "api-service",
			Labels:      map[string]string{"env": "production", "component": "api"},
			CreatedAt:   now.Add(-1 * time.Hour),
			UpdatedAt:   now.Add(-30 * time.Minute),
		},
		{
			ID:          "incident-3",
			Title:       "Minor logging issue",
			Description: "Some logs are not appearing",
			Status:      domain.StatusResolved,
			Severity:    domain.SeverityLow,
			TeamID:      "team-ops",
			ServiceID:   "logging-service",
			// No labels or assignee
			CreatedAt:  now.Add(-3 * time.Hour),
			UpdatedAt:  now.Add(-10 * time.Minute),
			ResolvedAt: func() *time.Time { t := now.Add(-10 * time.Minute); return &t }(),
		},
	}

	for _, incident := range incidents {
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Setup failed: %v", err)
		}
	}

	testCases := []struct {
		name          string
		filter        ports.ListFilter
		expectedCount int
		expectedIDs   []string
	}{
		{
			name: "filter by assignee (existing)",
			filter: ports.ListFilter{
				AssigneeID: "user-1",
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
		{
			name: "filter by assignee (non-existent)",
			filter: ports.ListFilter{
				AssigneeID: "user-999",
			},
			expectedCount: 0,
		},
		{
			name: "filter by service",
			filter: ports.ListFilter{
				ServiceID: "api-service",
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-2"},
		},
		{
			name: "filter by multiple status values",
			filter: ports.ListFilter{
				Status: []domain.Status{domain.StatusTriggered, domain.StatusResolved},
			},
			expectedCount: 2,
			expectedIDs:   []string{"incident-1", "incident-3"},
		},
		{
			name: "filter by multiple severity values",
			filter: ports.ListFilter{
				Severity: []domain.Severity{domain.SeverityCritical, domain.SeverityHigh},
			},
			expectedCount: 2,
			expectedIDs:   []string{"incident-1", "incident-2"},
		},
		{
			name: "filter by labels - incident with no labels",
			filter: ports.ListFilter{
				Labels: map[string]string{"env": "production"},
			},
			expectedCount: 2, // incident-3 has no labels, so doesn't match
			expectedIDs:   []string{"incident-1", "incident-2"},
		},
		{
			name: "filter by specific label value",
			filter: ports.ListFilter{
				Labels: map[string]string{"component": "database"},
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
		{
			name: "filter by non-existent label",
			filter: ports.ListFilter{
				Labels: map[string]string{"nonexistent": "value"},
			},
			expectedCount: 0,
		},
		{
			name: "search in title",
			filter: ports.ListFilter{
				Search: "database",
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
		{
			name: "search in description",
			filter: ports.ListFilter{
				Search: "rate limiting",
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-2"},
		},
		{
			name: "search case insensitive",
			filter: ports.ListFilter{
				Search: "DATABASE",
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
		{
			name: "search no matches",
			filter: ports.ListFilter{
				Search: "nonexistent term",
			},
			expectedCount: 0,
		},
		{
			name: "time filter - created after",
			filter: ports.ListFilter{
				CreatedAfter: func() *time.Time { t := now.Add(-90 * time.Minute); return &t }(),
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-2"},
		},
		{
			name: "time filter - created before",
			filter: ports.ListFilter{
				CreatedBefore: func() *time.Time { t := now.Add(-90 * time.Minute); return &t }(),
			},
			expectedCount: 2,
			expectedIDs:   []string{"incident-1", "incident-3"},
		},
		{
			name: "time filter - updated after",
			filter: ports.ListFilter{
				UpdatedAfter: func() *time.Time { t := now.Add(-45 * time.Minute); return &t }(),
			},
			expectedCount: 2,
			expectedIDs:   []string{"incident-2", "incident-3"},
		},
		{
			name: "time filter - updated before",
			filter: ports.ListFilter{
				UpdatedBefore: func() *time.Time { t := now.Add(-45 * time.Minute); return &t }(),
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
		{
			name: "combined filters",
			filter: ports.ListFilter{
				TeamID:   "team-db",
				Status:   []domain.Status{domain.StatusTriggered},
				Severity: []domain.Severity{domain.SeverityCritical},
				Labels:   map[string]string{"env": "production"},
			},
			expectedCount: 1,
			expectedIDs:   []string{"incident-1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := repo.List(ctx, tc.filter)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}

			if result.Total != tc.expectedCount {
				t.Errorf("Expected total %d, got %d", tc.expectedCount, result.Total)
			}

			if len(result.Incidents) != tc.expectedCount {
				t.Errorf("Expected %d incidents, got %d", tc.expectedCount, len(result.Incidents))
			}

			// Check specific IDs if provided
			if tc.expectedIDs != nil {
				actualIDs := make([]string, len(result.Incidents))
				for i, incident := range result.Incidents {
					actualIDs[i] = incident.ID
				}

				// Convert to maps for easier comparison (order might vary)
				expectedMap := make(map[string]bool)
				actualMap := make(map[string]bool)

				for _, id := range tc.expectedIDs {
					expectedMap[id] = true
				}
				for _, id := range actualIDs {
					actualMap[id] = true
				}

				for expectedID := range expectedMap {
					if !actualMap[expectedID] {
						t.Errorf("Expected incident %s not found in results", expectedID)
					}
				}

				for actualID := range actualMap {
					if !expectedMap[actualID] {
						t.Errorf("Unexpected incident %s found in results", actualID)
					}
				}
			}
		})
	}
}

func TestIncidentRepository_PaginationEdgeCases(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	// Create more incidents for pagination testing
	for i := 1; i <= 10; i++ {
		incident := createTestIncident(fmt.Sprintf("incident-%d", i))
		err := repo.Create(ctx, incident)
		if err != nil {
			t.Fatalf("Setup failed for incident %d: %v", i, err)
		}
	}

	testCases := []struct {
		name          string
		limit         int
		offset        int
		expectedCount int
		expectedHasMore bool
	}{
		{
			name:            "first page",
			limit:           3,
			offset:          0,
			expectedCount:   3,
			expectedHasMore: true,
		},
		{
			name:            "middle page",
			limit:           3,
			offset:          3,
			expectedCount:   3,
			expectedHasMore: true,
		},
		{
			name:            "last page",
			limit:           3,
			offset:          9,
			expectedCount:   1,
			expectedHasMore: false,
		},
		{
			name:            "offset beyond total",
			limit:           3,
			offset:          15,
			expectedCount:   0,
			expectedHasMore: false,
		},
		{
			name:            "large limit",
			limit:           100,
			offset:          0,
			expectedCount:   10,
			expectedHasMore: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			filter := ports.ListFilter{
				Limit:  tc.limit,
				Offset: tc.offset,
			}

			result, err := repo.List(ctx, filter)
			if err != nil {
				t.Fatalf("List failed: %v", err)
			}

			if len(result.Incidents) != tc.expectedCount {
				t.Errorf("Expected %d incidents, got %d", tc.expectedCount, len(result.Incidents))
			}

			if result.HasMore != tc.expectedHasMore {
				t.Errorf("Expected HasMore %t, got %t", tc.expectedHasMore, result.HasMore)
			}

			if result.Total != 10 {
				t.Errorf("Expected total 10, got %d", result.Total)
			}
		})
	}
}

func TestIncidentRepository_InvalidFilterValidation(t *testing.T) {
	ctx := context.Background()
	repo := NewIncidentRepository()

	testCases := []struct {
		name   string
		filter ports.ListFilter
	}{
		{
			name: "invalid sort field",
			filter: ports.ListFilter{
				SortBy: "invalid_field",
			},
		},
		{
			name: "invalid sort order",
			filter: ports.ListFilter{
				SortOrder: "invalid_order",
			},
		},
		{
			name: "limit too high",
			filter: ports.ListFilter{
				Limit: 2000,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := repo.List(ctx, tc.filter)
			if !errors.Is(err, ports.ErrInvalidInput) {
				t.Errorf("Expected ErrInvalidInput for %s, got: %v", tc.name, err)
			}
		})
	}
}
