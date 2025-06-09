// Package memory provides in-memory implementations of repository interfaces.
//
// These implementations are designed for testing, development, and small deployments
// where persistence is not required. All data is stored in memory and will be lost
// when the application restarts.
package memory

import (
	"context"
	"sync"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
)

// ScheduleRepository provides an in-memory implementation of the ScheduleRepository interface.
//
// This implementation is thread-safe and suitable for development, testing, and small deployments.
// All data is stored in memory and will be lost when the application restarts.
//
// The repository maintains schedules in a map keyed by schedule ID, with read-write mutex
// protection for concurrent access. Deep copies are made of all schedules to ensure
// immutability and prevent accidental modification of stored data.
type ScheduleRepository struct {
	mu        sync.RWMutex
	schedules map[string]*domain.Schedule
}

// NewScheduleRepository creates a new in-memory schedule repository.
//
// Returns a ready-to-use repository with empty storage.
func NewScheduleRepository() *ScheduleRepository {
	return &ScheduleRepository{
		schedules: make(map[string]*domain.Schedule),
	}
}

// Create stores a new schedule in the repository.
//
// The schedule must have a unique ID and pass domain validation.
// A deep copy of the schedule is stored to ensure immutability.
// Returns ErrAlreadyExists if a schedule with the same ID already exists.
// Returns ErrInvalidInput if the schedule fails validation.
func (r *ScheduleRepository) Create(ctx context.Context, schedule *domain.Schedule) error {
	if schedule == nil {
		return ports.ErrInvalidInput
	}

	// Validate the schedule
	if err := schedule.Validate(); err != nil {
		return ports.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if schedule already exists
	if _, exists := r.schedules[schedule.ID]; exists {
		return ports.ErrAlreadyExists
	}

	// Store a deep copy
	r.schedules[schedule.ID] = r.copySchedule(schedule)

	return nil
}

// Get retrieves a schedule by its unique identifier.
//
// Returns a deep copy of the schedule to ensure immutability.
// Returns ErrNotFound if no schedule exists with the given ID.
func (r *ScheduleRepository) Get(ctx context.Context, id string) (*domain.Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	schedule, exists := r.schedules[id]
	if !exists {
		return nil, ports.ErrNotFound
	}

	return r.copySchedule(schedule), nil
}

// GetByTeamID retrieves all schedules for a specific team.
//
// Returns a list of deep copies of schedules belonging to the specified team.
// Returns an empty list if no schedules exist for the team.
func (r *ScheduleRepository) GetByTeamID(ctx context.Context, teamID string) ([]*domain.Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result := make([]*domain.Schedule, 0)
	for _, schedule := range r.schedules {
		if schedule.TeamID == teamID {
			result = append(result, r.copySchedule(schedule))
		}
	}

	return result, nil
}

// List retrieves all schedules in the repository.
//
// Returns deep copies of all schedules with optional filtering by enabled status.
// If enabledOnly is true, only enabled schedules are returned.
func (r *ScheduleRepository) List(ctx context.Context, enabledOnly bool) ([]*domain.Schedule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	result := make([]*domain.Schedule, 0)
	for _, schedule := range r.schedules {
		if !enabledOnly || schedule.Enabled {
			result = append(result, r.copySchedule(schedule))
		}
	}

	return result, nil
}

// Update modifies an existing schedule in the repository.
//
// The schedule must exist and pass domain validation.
// A deep copy of the updated schedule is stored.
// Returns ErrNotFound if the schedule doesn't exist.
// Returns ErrInvalidInput if the schedule fails validation.
func (r *ScheduleRepository) Update(ctx context.Context, schedule *domain.Schedule) error {
	if schedule == nil {
		return ports.ErrInvalidInput
	}

	// Validate the schedule
	if err := schedule.Validate(); err != nil {
		return ports.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if schedule exists
	if _, exists := r.schedules[schedule.ID]; !exists {
		return ports.ErrNotFound
	}

	// Update the schedule (with updated timestamp)
	updatedSchedule := r.copySchedule(schedule)
	updatedSchedule.UpdatedAt = time.Now().UTC()
	r.schedules[schedule.ID] = updatedSchedule

	return nil
}

// Delete removes a schedule from the repository.
//
// This is a hard delete operation that permanently removes the schedule.
// Returns ErrNotFound if the schedule doesn't exist.
func (r *ScheduleRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check if schedule exists
	if _, exists := r.schedules[id]; !exists {
		return ports.ErrNotFound
	}

	// Delete the schedule
	delete(r.schedules, id)

	return nil
}

// Clear removes all schedules from the repository.
//
// This method is primarily intended for testing purposes.
func (r *ScheduleRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.schedules = make(map[string]*domain.Schedule)
}

// Count returns the total number of schedules in the repository.
//
// This method is primarily intended for testing purposes.
func (r *ScheduleRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.schedules)
}

// copySchedule creates a deep copy of a schedule to ensure immutability.
//
// This prevents external modifications from affecting the stored data
// and ensures that returned schedules are independent copies.
func (r *ScheduleRepository) copySchedule(schedule *domain.Schedule) *domain.Schedule {
	if schedule == nil {
		return nil
	}

	// Create a new schedule with copied values
	copied := &domain.Schedule{
		ID:        schedule.ID,
		Name:      schedule.Name,
		TeamID:    schedule.TeamID,
		Timezone:  schedule.Timezone,
		CreatedAt: schedule.CreatedAt,
		UpdatedAt: schedule.UpdatedAt,
		Enabled:   schedule.Enabled,
		Rotation: domain.Rotation{
			Type:         schedule.Rotation.Type,
			Participants: make([]string, len(schedule.Rotation.Participants)),
			StartDate:    schedule.Rotation.StartDate,
			Duration:     schedule.Rotation.Duration,
		},
	}

	// Deep copy the participants slice
	copy(copied.Rotation.Participants, schedule.Rotation.Participants)

	return copied
}
