// Package domain contains the core business entities and value objects for the Guvnor incident management platform.
//
// This file defines schedule-related domain types for on-call rotation management.
// Schedules enable teams to define rotation patterns for incident response,
// supporting timezone-aware calculations and GitOps-friendly YAML configuration.
package domain

import (
	"fmt"
	"strings"
	"time"
)

// RotationType defines the type of rotation schedule.
type RotationType string

const (
	// RotationTypeWeekly represents a weekly rotation pattern
	RotationTypeWeekly RotationType = "weekly"
	// RotationTypeDaily represents a daily rotation pattern
	RotationTypeDaily RotationType = "daily"
	// RotationTypeCustom represents a custom rotation pattern
	RotationTypeCustom RotationType = "custom"
)

// String returns the string representation of the rotation type.
func (r RotationType) String() string {
	return string(r)
}

// IsValid checks if the rotation type is valid.
func (r RotationType) IsValid() bool {
	switch r {
	case RotationTypeWeekly, RotationTypeDaily, RotationTypeCustom:
		return true
	default:
		return false
	}
}

// Rotation defines the rotation pattern for a schedule.
type Rotation struct {
	// Type specifies the rotation pattern (weekly, daily, custom)
	Type RotationType `json:"type" yaml:"type"`
	// Participants contains the list of user IDs in rotation order
	Participants []string `json:"participants" yaml:"participants"`
	// StartDate is when this rotation begins
	StartDate time.Time `json:"start_date" yaml:"start_date"`
	// Duration specifies the length of each rotation period (for custom rotations)
	Duration time.Duration `json:"duration,omitempty" yaml:"duration,omitempty"`
}

// Validate checks that the rotation configuration is valid.
//
// Returns an error if the rotation is invalid, describing the specific issue.
func (r *Rotation) Validate() error {
	if !r.Type.IsValid() {
		return fmt.Errorf("invalid rotation type: %s", r.Type)
	}

	if len(r.Participants) == 0 {
		return fmt.Errorf("rotation must have at least one participant")
	}

	// Check for duplicate participants
	seen := make(map[string]bool)
	for _, participant := range r.Participants {
		if participant == "" {
			return fmt.Errorf("participant ID cannot be empty")
		}
		if seen[participant] {
			return fmt.Errorf("duplicate participant: %s", participant)
		}
		seen[participant] = true
	}

	if r.StartDate.IsZero() {
		return fmt.Errorf("start date is required")
	}

	// Custom rotations require a duration
	if r.Type == RotationTypeCustom && r.Duration <= 0 {
		return fmt.Errorf("custom rotation type requires a positive duration")
	}

	return nil
}

// GetCurrentOnCall calculates who is currently on call at the specified time.
//
// This method handles timezone conversion and rotation calculations to determine
// the current on-call person based on the rotation pattern and start date.
//
// Returns the participant ID of the person currently on call.
func (r *Rotation) GetCurrentOnCall(at time.Time, timezone string) (string, error) {
	if err := r.Validate(); err != nil {
		return "", fmt.Errorf("invalid rotation: %w", err)
	}

	// Load the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	// Convert times to the schedule's timezone
	startTime := r.StartDate.In(loc)
	currentTime := at.In(loc)

	// If current time is before start time, no one is on call yet
	if currentTime.Before(startTime) {
		return "", fmt.Errorf("schedule has not started yet")
	}

	var rotationDuration time.Duration
	switch r.Type {
	case RotationTypeDaily:
		rotationDuration = 24 * time.Hour
	case RotationTypeWeekly:
		rotationDuration = 7 * 24 * time.Hour
	case RotationTypeCustom:
		rotationDuration = r.Duration
	}

	// Calculate elapsed time since start
	elapsed := currentTime.Sub(startTime)

	// Calculate how many complete rotation periods have passed
	rotationPeriods := int(elapsed / rotationDuration)

	// Determine current participant index
	participantIndex := rotationPeriods % len(r.Participants)

	return r.Participants[participantIndex], nil
}

// GetNextRotation calculates when the next rotation will occur.
//
// Returns the time when the next person will become on call.
func (r *Rotation) GetNextRotation(at time.Time, timezone string) (time.Time, string, error) {
	if err := r.Validate(); err != nil {
		return time.Time{}, "", fmt.Errorf("invalid rotation: %w", err)
	}

	// Load the timezone
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("invalid timezone %s: %w", timezone, err)
	}

	// Convert times to the schedule's timezone
	startTime := r.StartDate.In(loc)
	currentTime := at.In(loc)

	var rotationDuration time.Duration
	switch r.Type {
	case RotationTypeDaily:
		rotationDuration = 24 * time.Hour
	case RotationTypeWeekly:
		rotationDuration = 7 * 24 * time.Hour
	case RotationTypeCustom:
		rotationDuration = r.Duration
	}

	// Calculate the next rotation time
	if currentTime.Before(startTime) {
		// Schedule hasn't started yet, next rotation is the start time
		nextParticipantIndex := 0
		return startTime, r.Participants[nextParticipantIndex], nil
	}

	// Calculate elapsed time since start
	elapsed := currentTime.Sub(startTime)

	// Calculate how many complete rotation periods have passed
	rotationPeriods := int(elapsed / rotationDuration)

	// Calculate next rotation time
	nextRotationTime := startTime.Add(time.Duration(rotationPeriods+1) * rotationDuration)

	// Calculate next participant index
	nextParticipantIndex := (rotationPeriods + 1) % len(r.Participants)

	return nextRotationTime, r.Participants[nextParticipantIndex], nil
}

// Schedule represents an on-call schedule configuration.
type Schedule struct {
	// ID uniquely identifies the schedule
	ID string `json:"id" yaml:"id"`
	// Name is a human-readable name for the schedule
	Name string `json:"name" yaml:"name"`
	// TeamID identifies the team this schedule belongs to
	TeamID string `json:"team_id" yaml:"team_id"`
	// Timezone specifies the timezone for this schedule (IANA timezone name)
	Timezone string `json:"timezone" yaml:"timezone"`
	// Rotation defines the rotation pattern and participants
	Rotation Rotation `json:"rotation" yaml:"rotation"`
	// CreatedAt is when the schedule was created
	CreatedAt time.Time `json:"created_at" yaml:"created_at"`
	// UpdatedAt is when the schedule was last modified
	UpdatedAt time.Time `json:"updated_at" yaml:"updated_at"`
	// Enabled indicates if the schedule is active
	Enabled bool `json:"enabled" yaml:"enabled"`
}

// Validate checks that the schedule configuration is valid.
//
// This performs comprehensive validation of all schedule fields,
// ensuring they are within acceptable ranges and that required fields are set.
//
// Returns an error describing the first validation failure encountered.
func (s *Schedule) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("schedule ID is required")
	}

	if s.Name == "" {
		return fmt.Errorf("schedule name is required")
	}

	if s.TeamID == "" {
		return fmt.Errorf("team ID is required")
	}

	if s.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}

	// Validate timezone by attempting to load it
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return fmt.Errorf("invalid timezone %s: %w", s.Timezone, err)
	}

	// Validate the rotation
	if err := s.Rotation.Validate(); err != nil {
		return fmt.Errorf("invalid rotation: %w", err)
	}

	return nil
}

// GetCurrentOnCall determines who is currently on call for this schedule.
//
// This is a convenience method that delegates to the rotation's GetCurrentOnCall
// method using the schedule's timezone.
//
// Returns the participant ID of the person currently on call.
func (s *Schedule) GetCurrentOnCall(at time.Time) (string, error) {
	if !s.Enabled {
		return "", fmt.Errorf("schedule %s is disabled", s.ID)
	}

	return s.Rotation.GetCurrentOnCall(at, s.Timezone)
}

// GetNextRotation calculates when the next rotation will occur for this schedule.
//
// Returns the time when the next person will become on call and their ID.
func (s *Schedule) GetNextRotation(at time.Time) (time.Time, string, error) {
	if !s.Enabled {
		return time.Time{}, "", fmt.Errorf("schedule %s is disabled", s.ID)
	}

	return s.Rotation.GetNextRotation(at, s.Timezone)
}

// IsActive determines if the schedule is currently active.
//
// A schedule is active if it's enabled and the current time is after the start date.
func (s *Schedule) IsActive(at time.Time) bool {
	if !s.Enabled {
		return false
	}

	// Load the timezone
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return false
	}

	// Convert times to the schedule's timezone
	startTime := s.Rotation.StartDate.In(loc)
	currentTime := at.In(loc)

	return !currentTime.Before(startTime)
}

// Clone creates a deep copy of the schedule.
//
// This is useful for testing and ensuring immutability of domain objects.
func (s *Schedule) Clone() *Schedule {
	clone := &Schedule{
		ID:        s.ID,
		Name:      s.Name,
		TeamID:    s.TeamID,
		Timezone:  s.Timezone,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
		Enabled:   s.Enabled,
		Rotation: Rotation{
			Type:         s.Rotation.Type,
			Participants: make([]string, len(s.Rotation.Participants)),
			StartDate:    s.Rotation.StartDate,
			Duration:     s.Rotation.Duration,
		},
	}

	copy(clone.Rotation.Participants, s.Rotation.Participants)
	return clone
}

// String returns a human-readable representation of the schedule.
func (s *Schedule) String() string {
	var participants []string
	for _, p := range s.Rotation.Participants {
		participants = append(participants, p)
	}

	return fmt.Sprintf("Schedule{ID: %s, Name: %s, Team: %s, Type: %s, Participants: [%s]}",
		s.ID, s.Name, s.TeamID, s.Rotation.Type, strings.Join(participants, ", "))
}
