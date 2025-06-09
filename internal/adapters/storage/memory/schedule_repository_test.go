package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
)

func createTestSchedule(id string) *domain.Schedule {
	return &domain.Schedule{
		ID:       id,
		Name:     "Test Schedule " + id,
		TeamID:   "team-" + id,
		Timezone: "UTC",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeWeekly,
			Participants: []string{"user1@example.com", "user2@example.com"},
			StartDate:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		Enabled:   true,
	}
}

func createCustomSchedule(id string, rotationType domain.RotationType, duration time.Duration) *domain.Schedule {
	schedule := createTestSchedule(id)
	schedule.Rotation.Type = rotationType
	if rotationType == domain.RotationTypeCustom {
		schedule.Rotation.Duration = duration
	}
	return schedule
}

func TestNewScheduleRepository(t *testing.T) {
	repo := NewScheduleRepository()

	if repo == nil {
		t.Fatal("NewScheduleRepository() returned nil")
	}

	if repo.schedules == nil {
		t.Error("schedules map not initialized")
	}

	if len(repo.schedules) != 0 {
		t.Errorf("Expected empty schedules map, got %d items", len(repo.schedules))
	}

	if repo.Count() != 0 {
		t.Errorf("Expected count 0, got %d", repo.Count())
	}
}

func TestScheduleRepository_Create(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	tests := []struct {
		name     string
		schedule *domain.Schedule
		wantErr  bool
		errType  error
	}{
		{
			name:     "valid schedule",
			schedule: createTestSchedule("test-1"),
			wantErr:  false,
		},
		{
			name:     "nil schedule",
			schedule: nil,
			wantErr:  true,
			errType:  ports.ErrInvalidInput,
		},
		{
			name: "invalid schedule - empty ID",
			schedule: &domain.Schedule{
				Name:     "Test Schedule",
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			wantErr: true,
			errType: ports.ErrInvalidInput,
		},
		{
			name: "invalid schedule - invalid timezone",
			schedule: &domain.Schedule{
				ID:       "test-invalid",
				Name:     "Test Schedule",
				TeamID:   "team-1",
				Timezone: "Invalid/Timezone",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user1@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			wantErr: true,
			errType: ports.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Create() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errType != nil && err != tt.errType {
				t.Errorf("Create() error = %v, want error type %v", err, tt.errType)
			}
			if !tt.wantErr && tt.schedule != nil {
				// Verify schedule was stored
				stored, err := repo.Get(ctx, tt.schedule.ID)
				if err != nil {
					t.Errorf("Failed to retrieve created schedule: %v", err)
				}
				if stored.ID != tt.schedule.ID {
					t.Errorf("Stored schedule ID = %v, want %v", stored.ID, tt.schedule.ID)
				}
			}
		})
	}
}

func TestScheduleRepository_Create_Duplicate(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()
	schedule := createTestSchedule("duplicate-test")

	// Create first schedule
	err := repo.Create(ctx, schedule)
	if err != nil {
		t.Fatalf("First create failed: %v", err)
	}

	// Try to create duplicate
	err = repo.Create(ctx, schedule)
	if err != ports.ErrAlreadyExists {
		t.Errorf("Expected ErrAlreadyExists, got %v", err)
	}

	// Verify only one schedule exists
	if repo.Count() != 1 {
		t.Errorf("Expected count 1, got %d", repo.Count())
	}
}

func TestScheduleRepository_Get(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()
	schedule := createTestSchedule("get-test")

	// Test getting non-existent schedule
	_, err := repo.Get(ctx, "non-existent")
	if err != ports.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// Create schedule
	err = repo.Create(ctx, schedule)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Test getting existing schedule
	retrieved, err := repo.Get(ctx, schedule.ID)
	if err != nil {
		t.Errorf("Get() error = %v", err)
		return
	}

	// Verify all fields match
	if retrieved.ID != schedule.ID {
		t.Errorf("ID = %v, want %v", retrieved.ID, schedule.ID)
	}
	if retrieved.Name != schedule.Name {
		t.Errorf("Name = %v, want %v", retrieved.Name, schedule.Name)
	}
	if retrieved.TeamID != schedule.TeamID {
		t.Errorf("TeamID = %v, want %v", retrieved.TeamID, schedule.TeamID)
	}
	if retrieved.Timezone != schedule.Timezone {
		t.Errorf("Timezone = %v, want %v", retrieved.Timezone, schedule.Timezone)
	}
	if retrieved.Enabled != schedule.Enabled {
		t.Errorf("Enabled = %v, want %v", retrieved.Enabled, schedule.Enabled)
	}

	// Verify rotation fields
	if retrieved.Rotation.Type != schedule.Rotation.Type {
		t.Errorf("Rotation.Type = %v, want %v", retrieved.Rotation.Type, schedule.Rotation.Type)
	}
	if len(retrieved.Rotation.Participants) != len(schedule.Rotation.Participants) {
		t.Errorf("Participants length = %v, want %v", len(retrieved.Rotation.Participants), len(schedule.Rotation.Participants))
	}
	for i, participant := range schedule.Rotation.Participants {
		if retrieved.Rotation.Participants[i] != participant {
			t.Errorf("Participant[%d] = %v, want %v", i, retrieved.Rotation.Participants[i], participant)
		}
	}

	// Verify it's a deep copy - modifying retrieved shouldn't affect stored
	retrieved.Name = "Modified Name"
	retrieved.Rotation.Participants[0] = "modified@example.com"

	// Get again and verify original data is unchanged
	retrieved2, err := repo.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Second get failed: %v", err)
	}
	if retrieved2.Name != schedule.Name {
		t.Error("Retrieved schedule is not a deep copy - modification affected stored data")
	}
	if retrieved2.Rotation.Participants[0] != schedule.Rotation.Participants[0] {
		t.Error("Retrieved schedule participants is not a deep copy")
	}
}

func TestScheduleRepository_GetByTeamID(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	// Create schedules for different teams
	schedule1 := createTestSchedule("team1-schedule1")
	schedule1.TeamID = "team-1"
	schedule2 := createTestSchedule("team1-schedule2")
	schedule2.TeamID = "team-1"
	schedule3 := createTestSchedule("team2-schedule1")
	schedule3.TeamID = "team-2"

	schedules := []*domain.Schedule{schedule1, schedule2, schedule3}
	for _, schedule := range schedules {
		err := repo.Create(ctx, schedule)
		if err != nil {
			t.Fatalf("Create failed for %s: %v", schedule.ID, err)
		}
	}

	// Test getting schedules for team-1
	team1Schedules, err := repo.GetByTeamID(ctx, "team-1")
	if err != nil {
		t.Errorf("GetByTeamID() error = %v", err)
		return
	}

	if len(team1Schedules) != 2 {
		t.Errorf("Expected 2 schedules for team-1, got %d", len(team1Schedules))
	}

	// Verify all returned schedules belong to team-1
	for _, schedule := range team1Schedules {
		if schedule.TeamID != "team-1" {
			t.Errorf("Expected teamID 'team-1', got '%s'", schedule.TeamID)
		}
	}

	// Test getting schedules for team-2
	team2Schedules, err := repo.GetByTeamID(ctx, "team-2")
	if err != nil {
		t.Errorf("GetByTeamID() error = %v", err)
		return
	}

	if len(team2Schedules) != 1 {
		t.Errorf("Expected 1 schedule for team-2, got %d", len(team2Schedules))
	}

	// Test getting schedules for non-existent team
	nonExistentSchedules, err := repo.GetByTeamID(ctx, "non-existent-team")
	if err != nil {
		t.Errorf("GetByTeamID() error = %v", err)
		return
	}

	if len(nonExistentSchedules) != 0 {
		t.Errorf("Expected 0 schedules for non-existent team, got %d", len(nonExistentSchedules))
	}
}

func TestScheduleRepository_List(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	// Create schedules with different enabled states
	enabledSchedule := createTestSchedule("enabled-schedule")
	enabledSchedule.Enabled = true
	disabledSchedule := createTestSchedule("disabled-schedule")
	disabledSchedule.Enabled = false

	schedules := []*domain.Schedule{enabledSchedule, disabledSchedule}
	for _, schedule := range schedules {
		err := repo.Create(ctx, schedule)
		if err != nil {
			t.Fatalf("Create failed for %s: %v", schedule.ID, err)
		}
	}

	// Test listing all schedules
	allSchedules, err := repo.List(ctx, false)
	if err != nil {
		t.Errorf("List(false) error = %v", err)
		return
	}

	if len(allSchedules) != 2 {
		t.Errorf("Expected 2 schedules, got %d", len(allSchedules))
	}

	// Test listing only enabled schedules
	enabledSchedules, err := repo.List(ctx, true)
	if err != nil {
		t.Errorf("List(true) error = %v", err)
		return
	}

	if len(enabledSchedules) != 1 {
		t.Errorf("Expected 1 enabled schedule, got %d", len(enabledSchedules))
	}

	if enabledSchedules[0].ID != enabledSchedule.ID {
		t.Errorf("Expected enabled schedule ID %s, got %s", enabledSchedule.ID, enabledSchedules[0].ID)
	}

	// Test listing from empty repository
	emptyRepo := NewScheduleRepository()
	emptySchedules, err := emptyRepo.List(ctx, false)
	if err != nil {
		t.Errorf("List() error on empty repo = %v", err)
		return
	}

	if len(emptySchedules) != 0 {
		t.Errorf("Expected 0 schedules from empty repo, got %d", len(emptySchedules))
	}
}

func TestScheduleRepository_Update(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()
	schedule := createTestSchedule("update-test")

	// Test updating non-existent schedule
	err := repo.Update(ctx, schedule)
	if err != ports.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// Create schedule
	err = repo.Create(ctx, schedule)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update schedule
	originalUpdatedAt := schedule.UpdatedAt
	schedule.Name = "Updated Name"
	schedule.Enabled = false
	schedule.Rotation.Participants = []string{"updated-user@example.com"}

	err = repo.Update(ctx, schedule)
	if err != nil {
		t.Errorf("Update() error = %v", err)
		return
	}

	// Retrieve and verify updates
	updated, err := repo.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}

	// Verify updated timestamp was set on the stored schedule
	if !updated.UpdatedAt.After(originalUpdatedAt) {
		t.Error("UpdatedAt timestamp was not updated in stored schedule")
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Name = %v, want 'Updated Name'", updated.Name)
	}
	if updated.Enabled != false {
		t.Errorf("Enabled = %v, want false", updated.Enabled)
	}
	if len(updated.Rotation.Participants) != 1 || updated.Rotation.Participants[0] != "updated-user@example.com" {
		t.Errorf("Participants = %v, want ['updated-user@example.com']", updated.Rotation.Participants)
	}
}

func TestScheduleRepository_Update_Validation(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	tests := []struct {
		name     string
		schedule *domain.Schedule
		wantErr  bool
		errType  error
	}{
		{
			name:     "nil schedule",
			schedule: nil,
			wantErr:  true,
			errType:  ports.ErrInvalidInput,
		},
		{
			name: "invalid schedule",
			schedule: &domain.Schedule{
				ID:       "invalid-update",
				Name:     "", // Invalid: empty name
				TeamID:   "team-1",
				Timezone: "UTC",
				Rotation: domain.Rotation{
					Type:         domain.RotationTypeWeekly,
					Participants: []string{"user@example.com"},
					StartDate:    time.Now(),
				},
				Enabled: true,
			},
			wantErr: true,
			errType: ports.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Update(ctx, tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Update() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errType != nil && err != tt.errType {
				t.Errorf("Update() error = %v, want error type %v", err, tt.errType)
			}
		})
	}
}

func TestScheduleRepository_Delete(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()
	schedule := createTestSchedule("delete-test")

	// Test deleting non-existent schedule
	err := repo.Delete(ctx, "non-existent")
	if err != ports.ErrNotFound {
		t.Errorf("Expected ErrNotFound, got %v", err)
	}

	// Create schedule
	err = repo.Create(ctx, schedule)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if repo.Count() != 1 {
		t.Errorf("Expected count 1 after create, got %d", repo.Count())
	}

	// Delete schedule
	err = repo.Delete(ctx, schedule.ID)
	if err != nil {
		t.Errorf("Delete() error = %v", err)
		return
	}

	// Verify schedule is deleted
	if repo.Count() != 0 {
		t.Errorf("Expected count 0 after delete, got %d", repo.Count())
	}

	// Verify we can't retrieve deleted schedule
	_, err = repo.Get(ctx, schedule.ID)
	if err != ports.ErrNotFound {
		t.Errorf("Expected ErrNotFound after delete, got %v", err)
	}
}

func TestScheduleRepository_Clear(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	// Create multiple schedules
	for i := 0; i < 5; i++ {
		schedule := createTestSchedule(fmt.Sprintf("clear-test-%d", i))
		err := repo.Create(ctx, schedule)
		if err != nil {
			t.Fatalf("Create failed for schedule %d: %v", i, err)
		}
	}

	if repo.Count() != 5 {
		t.Errorf("Expected count 5, got %d", repo.Count())
	}

	// Clear repository
	repo.Clear()

	if repo.Count() != 0 {
		t.Errorf("Expected count 0 after clear, got %d", repo.Count())
	}

	// Verify schedules are gone
	schedules, err := repo.List(ctx, false)
	if err != nil {
		t.Errorf("List() error after clear = %v", err)
		return
	}

	if len(schedules) != 0 {
		t.Errorf("Expected 0 schedules after clear, got %d", len(schedules))
	}
}

func TestScheduleRepository_Count(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	if repo.Count() != 0 {
		t.Errorf("Expected initial count 0, got %d", repo.Count())
	}

	// Add schedules one by one and verify count
	for i := 1; i <= 3; i++ {
		schedule := createTestSchedule(fmt.Sprintf("count-test-%d", i))
		err := repo.Create(ctx, schedule)
		if err != nil {
			t.Fatalf("Create failed for schedule %d: %v", i, err)
		}

		if repo.Count() != i {
			t.Errorf("Expected count %d, got %d", i, repo.Count())
		}
	}

	// Delete one and verify count decreases
	err := repo.Delete(ctx, "count-test-2")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if repo.Count() != 2 {
		t.Errorf("Expected count 2 after delete, got %d", repo.Count())
	}
}

func TestScheduleRepository_ContextCancellation(t *testing.T) {
	repo := NewScheduleRepository()
	schedule := createTestSchedule("context-test")

	// Test context cancellation for all methods
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Create with cancelled context",
			fn:   func() error { return repo.Create(cancelledCtx, schedule) },
		},
		{
			name: "Get with cancelled context",
			fn:   func() error { _, err := repo.Get(cancelledCtx, "test"); return err },
		},
		{
			name: "GetByTeamID with cancelled context",
			fn:   func() error { _, err := repo.GetByTeamID(cancelledCtx, "team"); return err },
		},
		{
			name: "List with cancelled context",
			fn:   func() error { _, err := repo.List(cancelledCtx, false); return err },
		},
		{
			name: "Update with cancelled context",
			fn:   func() error { return repo.Update(cancelledCtx, schedule) },
		},
		{
			name: "Delete with cancelled context",
			fn:   func() error { return repo.Delete(cancelledCtx, "test") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err != context.Canceled {
				t.Errorf("Expected context.Canceled, got %v", err)
			}
		})
	}
}

func TestScheduleRepository_ConcurrentAccess(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	const numGoroutines = 10
	const operationsPerGoroutine = 20

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// Run concurrent operations
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < operationsPerGoroutine; j++ {
				scheduleID := fmt.Sprintf("concurrent-%d-%d", goroutineID, j)
				schedule := createTestSchedule(scheduleID)

				// Create
				if err := repo.Create(ctx, schedule); err != nil {
					errors <- fmt.Errorf("create error: %w", err)
					continue
				}

				// Get
				if _, err := repo.Get(ctx, scheduleID); err != nil {
					errors <- fmt.Errorf("get error: %w", err)
					continue
				}

				// Update
				schedule.Name = "Updated " + schedule.Name
				if err := repo.Update(ctx, schedule); err != nil {
					errors <- fmt.Errorf("update error: %w", err)
					continue
				}

				// List
				if _, err := repo.List(ctx, false); err != nil {
					errors <- fmt.Errorf("list error: %w", err)
					continue
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Concurrent operation error: %v", err)
	}

	// Verify final state
	expectedCount := numGoroutines * operationsPerGoroutine
	if repo.Count() != expectedCount {
		t.Errorf("Expected final count %d, got %d", expectedCount, repo.Count())
	}
}

func TestScheduleRepository_DeepCopy(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	// Create schedule with complex data
	schedule := &domain.Schedule{
		ID:       "deep-copy-test",
		Name:     "Deep Copy Test",
		TeamID:   "team-1",
		Timezone: "America/New_York",
		Rotation: domain.Rotation{
			Type:         domain.RotationTypeCustom,
			Participants: []string{"user1@example.com", "user2@example.com", "user3@example.com"},
			StartDate:    time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC),
			Duration:     72 * time.Hour,
		},
		CreatedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC),
		Enabled:   true,
	}

	err := repo.Create(ctx, schedule)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Get the schedule
	retrieved, err := repo.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Verify it's a complete copy
	if retrieved == schedule {
		t.Error("Retrieved schedule is the same object reference")
	}

	if &retrieved.Rotation == &schedule.Rotation {
		t.Error("Rotation is the same object reference")
	}

	if &retrieved.Rotation.Participants == &schedule.Rotation.Participants {
		t.Error("Participants slice is the same reference")
	}

	// Modify retrieved and verify original is unchanged
	retrieved.Name = "Modified Name"
	retrieved.TeamID = "modified-team"
	retrieved.Rotation.Type = domain.RotationTypeDaily
	retrieved.Rotation.Participants[0] = "modified-user@example.com"
	retrieved.Rotation.Participants = append(retrieved.Rotation.Participants, "new-user@example.com")

	// Get again and verify original data
	retrieved2, err := repo.Get(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("Second get failed: %v", err)
	}

	if retrieved2.Name != schedule.Name {
		t.Errorf("Name was modified in storage: got %s, want %s", retrieved2.Name, schedule.Name)
	}
	if retrieved2.TeamID != schedule.TeamID {
		t.Errorf("TeamID was modified in storage: got %s, want %s", retrieved2.TeamID, schedule.TeamID)
	}
	if retrieved2.Rotation.Type != schedule.Rotation.Type {
		t.Errorf("Rotation type was modified in storage: got %s, want %s", retrieved2.Rotation.Type, schedule.Rotation.Type)
	}
	if len(retrieved2.Rotation.Participants) != len(schedule.Rotation.Participants) {
		t.Errorf("Participants length was modified: got %d, want %d", len(retrieved2.Rotation.Participants), len(schedule.Rotation.Participants))
	}
	if retrieved2.Rotation.Participants[0] != schedule.Rotation.Participants[0] {
		t.Errorf("Participant was modified in storage: got %s, want %s", retrieved2.Rotation.Participants[0], schedule.Rotation.Participants[0])
	}
}

func TestScheduleRepository_DifferentRotationTypes(t *testing.T) {
	repo := NewScheduleRepository()
	ctx := context.Background()

	tests := []struct {
		name     string
		schedule *domain.Schedule
	}{
		{
			name:     "weekly rotation",
			schedule: createCustomSchedule("weekly-test", domain.RotationTypeWeekly, 0),
		},
		{
			name:     "daily rotation",
			schedule: createCustomSchedule("daily-test", domain.RotationTypeDaily, 0),
		},
		{
			name:     "custom rotation",
			schedule: createCustomSchedule("custom-test", domain.RotationTypeCustom, 48*time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := repo.Create(ctx, tt.schedule)
			if err != nil {
				t.Errorf("Create() error = %v", err)
				return
			}

			retrieved, err := repo.Get(ctx, tt.schedule.ID)
			if err != nil {
				t.Errorf("Get() error = %v", err)
				return
			}

			if retrieved.Rotation.Type != tt.schedule.Rotation.Type {
				t.Errorf("Rotation type = %v, want %v", retrieved.Rotation.Type, tt.schedule.Rotation.Type)
			}

			if tt.schedule.Rotation.Type == domain.RotationTypeCustom {
				if retrieved.Rotation.Duration != tt.schedule.Rotation.Duration {
					t.Errorf("Duration = %v, want %v", retrieved.Rotation.Duration, tt.schedule.Rotation.Duration)
				}
			}
		})
	}
}

func TestScheduleRepository_copySchedule(t *testing.T) {
	repo := NewScheduleRepository()

	// Test copying nil schedule
	copied := repo.copySchedule(nil)
	if copied != nil {
		t.Error("copySchedule(nil) should return nil")
	}

	// Test copying valid schedule
	original := createTestSchedule("copy-test")
	copied = repo.copySchedule(original)

	if copied == nil {
		t.Fatal("copySchedule() returned nil for valid schedule")
	}

	// Verify all fields are copied
	if copied.ID != original.ID {
		t.Errorf("ID = %v, want %v", copied.ID, original.ID)
	}
	if copied.Name != original.Name {
		t.Errorf("Name = %v, want %v", copied.Name, original.Name)
	}
	if copied.TeamID != original.TeamID {
		t.Errorf("TeamID = %v, want %v", copied.TeamID, original.TeamID)
	}
	if copied.Timezone != original.Timezone {
		t.Errorf("Timezone = %v, want %v", copied.Timezone, original.Timezone)
	}
	if copied.Enabled != original.Enabled {
		t.Errorf("Enabled = %v, want %v", copied.Enabled, original.Enabled)
	}

	// Verify rotation is copied
	if copied.Rotation.Type != original.Rotation.Type {
		t.Errorf("Rotation.Type = %v, want %v", copied.Rotation.Type, original.Rotation.Type)
	}
	if !copied.Rotation.StartDate.Equal(original.Rotation.StartDate) {
		t.Errorf("Rotation.StartDate = %v, want %v", copied.Rotation.StartDate, original.Rotation.StartDate)
	}
	if copied.Rotation.Duration != original.Rotation.Duration {
		t.Errorf("Rotation.Duration = %v, want %v", copied.Rotation.Duration, original.Rotation.Duration)
	}

	// Verify participants slice is deeply copied
	if len(copied.Rotation.Participants) != len(original.Rotation.Participants) {
		t.Errorf("Participants length = %v, want %v", len(copied.Rotation.Participants), len(original.Rotation.Participants))
	}

	for i, participant := range original.Rotation.Participants {
		if copied.Rotation.Participants[i] != participant {
			t.Errorf("Participant[%d] = %v, want %v", i, copied.Rotation.Participants[i], participant)
		}
	}

	// Verify it's a deep copy
	if &copied.Rotation.Participants == &original.Rotation.Participants {
		t.Error("Participants slice shares the same reference")
	}

	// Modify copied and verify original is unchanged
	copied.Rotation.Participants[0] = "modified@example.com"
	if original.Rotation.Participants[0] == "modified@example.com" {
		t.Error("Modifying copied participants affected original")
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
