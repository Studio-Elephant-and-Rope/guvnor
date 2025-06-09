// Package config provides configuration loading functionality for the Guvnor incident management platform.
//
// This file implements YAML-based schedule configuration loading, enabling GitOps-friendly
// schedule management where on-call rotations can be defined in version-controlled YAML files.
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

// ScheduleConfig represents the root structure of a schedule configuration file.
//
// This mirrors the YAML structure expected for schedule definitions,
// allowing teams to define their on-call rotations in a declarative format.
type ScheduleConfig struct {
	// Schedules maps schedule IDs to their configuration
	Schedules map[string]ScheduleDefinition `yaml:"schedules"`
}

// ScheduleDefinition represents a single schedule configuration in YAML.
//
// This structure is designed to be simple and GitOps-friendly,
// focusing on the essential information needed for on-call rotations.
type ScheduleDefinition struct {
	// Name is a human-readable name for the schedule (optional, defaults to schedule ID)
	Name string `yaml:"name,omitempty"`
	// TeamID identifies the team this schedule belongs to (optional, defaults to schedule ID)
	TeamID string `yaml:"team_id,omitempty"`
	// Timezone specifies the timezone for this schedule (IANA timezone name)
	Timezone string `yaml:"timezone"`
	// Rotation defines the rotation pattern and participants
	Rotation RotationDefinition `yaml:"rotation"`
	// Enabled indicates if the schedule is active (optional, defaults to true)
	Enabled *bool `yaml:"enabled,omitempty"`
}

// RotationDefinition represents rotation configuration in YAML.
//
// This maps directly to the domain Rotation type but uses string
// representations suitable for YAML configuration.
type RotationDefinition struct {
	// Type specifies the rotation pattern (weekly, daily, custom)
	Type string `yaml:"type"`
	// Participants contains the list of user IDs in rotation order
	Participants []string `yaml:"participants"`
	// StartDate is when this rotation begins (ISO 8601 format)
	StartDate string `yaml:"start_date,omitempty"`
	// Duration specifies the length of each rotation period (for custom rotations)
	Duration string `yaml:"duration,omitempty"`
}

// LoadSchedulesFromFile loads schedule configurations from a YAML file.
//
// This function reads a YAML file containing schedule definitions and converts
// them to domain Schedule objects. The file should follow the expected structure
// with a top-level "schedules" key mapping schedule IDs to their configurations.
//
// Returns a slice of Schedule objects or an error if loading fails.
func LoadSchedulesFromFile(filename string) ([]*domain.Schedule, error) {
	// Read the YAML file
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read schedule config file %s: %w", filename, err)
	}

	return LoadSchedulesFromYAML(data)
}

// LoadSchedulesFromYAML loads schedule configurations from YAML data.
//
// This function parses YAML data containing schedule definitions and converts
// them to domain Schedule objects. It performs validation and type conversion
// to ensure the resulting schedules are valid.
//
// Returns a slice of Schedule objects or an error if parsing fails.
func LoadSchedulesFromYAML(data []byte) ([]*domain.Schedule, error) {
	var config ScheduleConfig

	// Parse YAML
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Convert to domain objects
	var schedules []*domain.Schedule
	for scheduleID, definition := range config.Schedules {
		schedule, err := convertToDomainSchedule(scheduleID, definition)
		if err != nil {
			return nil, fmt.Errorf("failed to convert schedule %s: %w", scheduleID, err)
		}
		schedules = append(schedules, schedule)
	}

	return schedules, nil
}

// convertToDomainSchedule converts a YAML schedule definition to a domain Schedule object.
//
// This function handles type conversion, validation, and default value assignment
// to create a valid domain Schedule from the YAML configuration.
func convertToDomainSchedule(scheduleID string, definition ScheduleDefinition) (*domain.Schedule, error) {
	// Set defaults
	name := definition.Name
	if name == "" {
		name = scheduleID
	}

	teamID := definition.TeamID
	if teamID == "" {
		teamID = scheduleID
	}

	enabled := true
	if definition.Enabled != nil {
		enabled = *definition.Enabled
	}

	// Validate required fields
	if definition.Timezone == "" {
		return nil, fmt.Errorf("timezone is required")
	}

	if definition.Rotation.Type == "" {
		return nil, fmt.Errorf("rotation type is required")
	}

	if len(definition.Rotation.Participants) == 0 {
		return nil, fmt.Errorf("rotation must have at least one participant")
	}

	// Convert rotation type
	rotationType := domain.RotationType(definition.Rotation.Type)
	if !rotationType.IsValid() {
		return nil, fmt.Errorf("invalid rotation type: %s", definition.Rotation.Type)
	}

	// Parse start date
	var startDate time.Time
	if definition.Rotation.StartDate != "" {
		var err error
		startDate, err = time.Parse(time.RFC3339, definition.Rotation.StartDate)
		if err != nil {
			// Try alternative formats
			startDate, err = time.Parse("2006-01-02T15:04:05", definition.Rotation.StartDate)
			if err != nil {
				startDate, err = time.Parse("2006-01-02", definition.Rotation.StartDate)
				if err != nil {
					return nil, fmt.Errorf("invalid start date format %s: expected RFC3339, ISO 8601, or YYYY-MM-DD", definition.Rotation.StartDate)
				}
			}
		}
	} else {
		// Default to current time if not specified
		startDate = time.Now().UTC()
	}

	// Parse duration for custom rotations
	var duration time.Duration
	if rotationType == domain.RotationTypeCustom {
		if definition.Rotation.Duration == "" {
			return nil, fmt.Errorf("custom rotation type requires duration to be specified")
		}
		var err error
		duration, err = time.ParseDuration(definition.Rotation.Duration)
		if err != nil {
			return nil, fmt.Errorf("invalid duration format %s: %w", definition.Rotation.Duration, err)
		}
	}

	// Create domain objects
	rotation := domain.Rotation{
		Type:         rotationType,
		Participants: make([]string, len(definition.Rotation.Participants)),
		StartDate:    startDate,
		Duration:     duration,
	}
	copy(rotation.Participants, definition.Rotation.Participants)

	schedule := &domain.Schedule{
		ID:        scheduleID,
		Name:      name,
		TeamID:    teamID,
		Timezone:  definition.Timezone,
		Rotation:  rotation,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		Enabled:   enabled,
	}

	// Validate the resulting schedule
	if err := schedule.Validate(); err != nil {
		return nil, fmt.Errorf("schedule validation failed: %w", err)
	}

	return schedule, nil
}

// WriteSchedulesToFile writes schedule configurations to a YAML file.
//
// This function converts domain Schedule objects back to YAML format
// and writes them to a file. This is useful for exporting schedules
// or creating configuration templates.
//
// Returns an error if writing fails.
func WriteSchedulesToFile(filename string, schedules []*domain.Schedule) error {
	data, err := WriteSchedulesToYAML(schedules)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

// WriteSchedulesToYAML converts schedule configurations to YAML data.
//
// This function converts domain Schedule objects to YAML format,
// creating a structure that can be parsed back by LoadSchedulesFromYAML.
//
// Returns YAML data or an error if conversion fails.
func WriteSchedulesToYAML(schedules []*domain.Schedule) ([]byte, error) {
	config := ScheduleConfig{
		Schedules: make(map[string]ScheduleDefinition),
	}

	for _, schedule := range schedules {
		definition := convertFromDomainSchedule(schedule)
		config.Schedules[schedule.ID] = definition
	}

	return yaml.Marshal(config)
}

// convertFromDomainSchedule converts a domain Schedule object to YAML schedule definition.
//
// This function handles the reverse conversion from domain objects to YAML-friendly
// structures for configuration export or templating.
func convertFromDomainSchedule(schedule *domain.Schedule) ScheduleDefinition {
	// Convert start date to RFC3339 format
	startDate := schedule.Rotation.StartDate.Format(time.RFC3339)

	// Convert duration to string
	var duration string
	if schedule.Rotation.Type == domain.RotationTypeCustom {
		duration = schedule.Rotation.Duration.String()
	}

	rotation := RotationDefinition{
		Type:         string(schedule.Rotation.Type),
		Participants: make([]string, len(schedule.Rotation.Participants)),
		StartDate:    startDate,
		Duration:     duration,
	}
	copy(rotation.Participants, schedule.Rotation.Participants)

	definition := ScheduleDefinition{
		Name:     schedule.Name,
		TeamID:   schedule.TeamID,
		Timezone: schedule.Timezone,
		Rotation: rotation,
		Enabled:  &schedule.Enabled,
	}

	// Omit name and team_id if they match the schedule ID
	if schedule.Name == schedule.ID {
		definition.Name = ""
	}
	if schedule.TeamID == schedule.ID {
		definition.TeamID = ""
	}

	return definition
}
