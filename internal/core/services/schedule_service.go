// Package services contains the application's business logic and use cases.
//
// Services coordinate between the domain layer and the infrastructure layer,
// implementing complex business workflows while maintaining separation of concerns.
// They provide the primary interface for external systems to interact with
// the application's core functionality.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// ScheduleService provides business logic for managing on-call schedules.
//
// This service handles schedule creation, retrieval, updates, and on-call calculations.
// It coordinates between the domain layer (business rules) and the repository layer
// (data persistence), ensuring that all operations maintain data consistency and
// business rule compliance.
//
// The service is responsible for:
//   - Schedule lifecycle management (create, read, update, delete)
//   - On-call calculation and rotation logic
//   - Business rule enforcement and validation
//   - Audit logging and observability
//   - Error handling and recovery
type ScheduleService struct {
	repo   ports.ScheduleRepository
	logger *logging.Logger
}

// NewScheduleService creates a new instance of the schedule service.
//
// The service requires a repository for data persistence and a logger for observability.
// Returns an error if any required dependencies are nil.
func NewScheduleService(repo ports.ScheduleRepository, logger *logging.Logger) (*ScheduleService, error) {
	if repo == nil {
		return nil, fmt.Errorf("schedule repository is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &ScheduleService{
		repo:   repo,
		logger: logger,
	}, nil
}

// CreateSchedule creates a new on-call schedule.
//
// This method validates the schedule configuration, sets timestamps,
// and persists the schedule to the repository. The schedule must have
// a unique ID and valid configuration.
//
// Returns the created schedule or an error if creation fails.
func (s *ScheduleService) CreateSchedule(ctx context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
	// Validate input first to avoid nil pointer dereference
	if schedule == nil {
		start := s.logger.LogOperationStart("create_schedule", "schedule_id", "nil")
		s.logger.LogOperationEnd("create_schedule", start, fmt.Errorf("schedule is nil"), "schedule_id", "nil")
		return nil, fmt.Errorf("schedule is required")
	}

	start := s.logger.LogOperationStart("create_schedule", "schedule_id", schedule.ID)

	// Set creation and update timestamps
	now := time.Now().UTC()
	schedule.CreatedAt = now
	schedule.UpdatedAt = now

	// Validate the schedule
	if err := schedule.Validate(); err != nil {
		s.logger.LogOperationEnd("create_schedule", start, err, "schedule_id", schedule.ID, "validation_error", err.Error())
		return nil, fmt.Errorf("schedule validation failed: %w", err)
	}

	// Create the schedule in the repository
	if err := s.repo.Create(ctx, schedule); err != nil {
		s.logger.LogOperationEnd("create_schedule", start, err, "schedule_id", schedule.ID)
		return nil, fmt.Errorf("failed to create schedule: %w", err)
	}

	s.logger.LogOperationEnd("create_schedule", start, nil,
		"schedule_id", schedule.ID,
		"team_id", schedule.TeamID,
		"rotation_type", schedule.Rotation.Type.String(),
		"participant_count", len(schedule.Rotation.Participants),
	)

	return schedule, nil
}

// GetSchedule retrieves a schedule by its unique identifier.
//
// Returns the schedule if found, or an error if the schedule doesn't exist
// or if retrieval fails.
func (s *ScheduleService) GetSchedule(ctx context.Context, id string) (*domain.Schedule, error) {
	start := s.logger.LogOperationStart("get_schedule", "schedule_id", id)

	if id == "" {
		s.logger.LogOperationEnd("get_schedule", start, fmt.Errorf("empty schedule ID"), "schedule_id", id)
		return nil, fmt.Errorf("schedule ID is required")
	}

	schedule, err := s.repo.Get(ctx, id)
	if err != nil {
		s.logger.LogOperationEnd("get_schedule", start, err, "schedule_id", id)
		return nil, fmt.Errorf("failed to retrieve schedule: %w", err)
	}

	s.logger.LogOperationEnd("get_schedule", start, nil, "schedule_id", id, "team_id", schedule.TeamID)
	return schedule, nil
}

// GetSchedulesByTeam retrieves all schedules for a specific team.
//
// Returns a list of schedules belonging to the team, or an empty list
// if no schedules exist for the team.
func (s *ScheduleService) GetSchedulesByTeam(ctx context.Context, teamID string) ([]*domain.Schedule, error) {
	start := s.logger.LogOperationStart("get_schedules_by_team", "team_id", teamID)

	if teamID == "" {
		s.logger.LogOperationEnd("get_schedules_by_team", start, fmt.Errorf("empty team ID"), "team_id", teamID)
		return nil, fmt.Errorf("team ID is required")
	}

	schedules, err := s.repo.GetByTeamID(ctx, teamID)
	if err != nil {
		s.logger.LogOperationEnd("get_schedules_by_team", start, err, "team_id", teamID)
		return nil, fmt.Errorf("failed to retrieve schedules for team: %w", err)
	}

	s.logger.LogOperationEnd("get_schedules_by_team", start, nil,
		"team_id", teamID,
		"schedule_count", len(schedules),
	)

	return schedules, nil
}

// ListSchedules retrieves all schedules with optional filtering.
//
// If enabledOnly is true, only enabled schedules are returned.
// Returns a list of all matching schedules.
func (s *ScheduleService) ListSchedules(ctx context.Context, enabledOnly bool) ([]*domain.Schedule, error) {
	start := s.logger.LogOperationStart("list_schedules", "enabled_only", enabledOnly)

	schedules, err := s.repo.List(ctx, enabledOnly)
	if err != nil {
		s.logger.LogOperationEnd("list_schedules", start, err, "enabled_only", enabledOnly)
		return nil, fmt.Errorf("failed to list schedules: %w", err)
	}

	s.logger.LogOperationEnd("list_schedules", start, nil,
		"enabled_only", enabledOnly,
		"schedule_count", len(schedules),
	)

	return schedules, nil
}

// UpdateSchedule modifies an existing schedule.
//
// The schedule must exist and pass validation. The update timestamp
// is automatically set to the current time.
//
// Returns the updated schedule or an error if the update fails.
func (s *ScheduleService) UpdateSchedule(ctx context.Context, schedule *domain.Schedule) (*domain.Schedule, error) {
	// Validate input first to avoid nil pointer dereference
	if schedule == nil {
		start := s.logger.LogOperationStart("update_schedule", "schedule_id", "nil")
		s.logger.LogOperationEnd("update_schedule", start, fmt.Errorf("schedule is nil"), "schedule_id", "nil")
		return nil, fmt.Errorf("schedule is required")
	}

	start := s.logger.LogOperationStart("update_schedule", "schedule_id", schedule.ID)

	// Set update timestamp
	schedule.UpdatedAt = time.Now().UTC()

	// Validate the schedule
	if err := schedule.Validate(); err != nil {
		s.logger.LogOperationEnd("update_schedule", start, err, "schedule_id", schedule.ID, "validation_error", err.Error())
		return nil, fmt.Errorf("schedule validation failed: %w", err)
	}

	// Update the schedule in the repository
	if err := s.repo.Update(ctx, schedule); err != nil {
		s.logger.LogOperationEnd("update_schedule", start, err, "schedule_id", schedule.ID)
		return nil, fmt.Errorf("failed to update schedule: %w", err)
	}

	s.logger.LogOperationEnd("update_schedule", start, nil,
		"schedule_id", schedule.ID,
		"team_id", schedule.TeamID,
		"enabled", schedule.Enabled,
	)

	return schedule, nil
}

// DeleteSchedule removes a schedule from the repository.
//
// This permanently deletes the schedule and cannot be undone.
// Returns an error if the schedule doesn't exist or deletion fails.
func (s *ScheduleService) DeleteSchedule(ctx context.Context, id string) error {
	start := s.logger.LogOperationStart("delete_schedule", "schedule_id", id)

	if id == "" {
		s.logger.LogOperationEnd("delete_schedule", start, fmt.Errorf("empty schedule ID"), "schedule_id", id)
		return fmt.Errorf("schedule ID is required")
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		s.logger.LogOperationEnd("delete_schedule", start, err, "schedule_id", id)
		return fmt.Errorf("failed to delete schedule: %w", err)
	}

	s.logger.LogOperationEnd("delete_schedule", start, nil, "schedule_id", id)
	return nil
}

// GetCurrentOnCall determines who is currently on call for a given schedule.
//
// This method retrieves the schedule and calculates the current on-call person
// based on the rotation configuration and the specified time.
//
// Returns the participant ID of the person currently on call,
// or an error if the calculation fails or the schedule is not active.
func (s *ScheduleService) GetCurrentOnCall(ctx context.Context, scheduleID string, at time.Time) (string, error) {
	start := s.logger.LogOperationStart("get_current_on_call", "schedule_id", scheduleID, "time", at.Format(time.RFC3339))

	if scheduleID == "" {
		s.logger.LogOperationEnd("get_current_on_call", start, fmt.Errorf("empty schedule ID"), "schedule_id", scheduleID)
		return "", fmt.Errorf("schedule ID is required")
	}

	// Retrieve the schedule
	schedule, err := s.repo.Get(ctx, scheduleID)
	if err != nil {
		s.logger.LogOperationEnd("get_current_on_call", start, err, "schedule_id", scheduleID)
		return "", fmt.Errorf("failed to retrieve schedule: %w", err)
	}

	// Calculate current on-call person
	participant, err := schedule.GetCurrentOnCall(at)
	if err != nil {
		s.logger.LogOperationEnd("get_current_on_call", start, err,
			"schedule_id", scheduleID,
			"schedule_enabled", schedule.Enabled,
			"calculation_error", err.Error(),
		)
		return "", fmt.Errorf("failed to calculate current on-call: %w", err)
	}

	s.logger.LogOperationEnd("get_current_on_call", start, nil,
		"schedule_id", scheduleID,
		"current_on_call", participant,
		"time", at.Format(time.RFC3339),
	)

	return participant, nil
}

// GetNextRotation calculates when the next rotation will occur for a given schedule.
//
// This method retrieves the schedule and calculates the next rotation time
// and the person who will be on call.
//
// Returns the time of the next rotation and the participant ID,
// or an error if the calculation fails or the schedule is not active.
func (s *ScheduleService) GetNextRotation(ctx context.Context, scheduleID string, at time.Time) (time.Time, string, error) {
	start := s.logger.LogOperationStart("get_next_rotation", "schedule_id", scheduleID, "time", at.Format(time.RFC3339))

	if scheduleID == "" {
		err := fmt.Errorf("schedule ID is required")
		s.logger.LogOperationEnd("get_next_rotation", start, err, "schedule_id", scheduleID)
		return time.Time{}, "", err
	}

	// Retrieve the schedule
	schedule, err := s.repo.Get(ctx, scheduleID)
	if err != nil {
		s.logger.LogOperationEnd("get_next_rotation", start, err, "schedule_id", scheduleID)
		return time.Time{}, "", fmt.Errorf("failed to retrieve schedule: %w", err)
	}

	// Calculate next rotation
	nextTime, nextParticipant, err := schedule.GetNextRotation(at)
	if err != nil {
		s.logger.LogOperationEnd("get_next_rotation", start, err,
			"schedule_id", scheduleID,
			"schedule_enabled", schedule.Enabled,
			"calculation_error", err.Error(),
		)
		return time.Time{}, "", fmt.Errorf("failed to calculate next rotation: %w", err)
	}

	s.logger.LogOperationEnd("get_next_rotation", start, nil,
		"schedule_id", scheduleID,
		"next_rotation_time", nextTime.Format(time.RFC3339),
		"next_on_call", nextParticipant,
	)

	return nextTime, nextParticipant, nil
}

// IsScheduleActive determines if a schedule is currently active.
//
// A schedule is active if it's enabled and the current time is after the start date.
func (s *ScheduleService) IsScheduleActive(ctx context.Context, scheduleID string, at time.Time) (bool, error) {
	start := s.logger.LogOperationStart("is_schedule_active", "schedule_id", scheduleID, "time", at.Format(time.RFC3339))

	if scheduleID == "" {
		err := fmt.Errorf("schedule ID is required")
		s.logger.LogOperationEnd("is_schedule_active", start, err, "schedule_id", scheduleID)
		return false, err
	}

	// Retrieve the schedule
	schedule, err := s.repo.Get(ctx, scheduleID)
	if err != nil {
		s.logger.LogOperationEnd("is_schedule_active", start, err, "schedule_id", scheduleID)
		return false, fmt.Errorf("failed to retrieve schedule: %w", err)
	}

	// Check if schedule is active
	isActive := schedule.IsActive(at)

	s.logger.LogOperationEnd("is_schedule_active", start, nil,
		"schedule_id", scheduleID,
		"is_active", isActive,
		"enabled", schedule.Enabled,
	)

	return isActive, nil
}
