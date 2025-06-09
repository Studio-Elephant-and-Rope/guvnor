// Package ports defines the interfaces that connect the core domain to external adapters.
//
// This package contains port interfaces that define contracts for external dependencies
// such as databases, message queues, and other infrastructure components. Following
// hexagonal architecture principles, these interfaces allow the core domain to remain
// independent of external concerns.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

// Common repository errors that implementations should use for consistent error handling.
var (
	// ErrNotFound indicates that a requested resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrAlreadyExists indicates that a resource already exists and cannot be created again.
	ErrAlreadyExists = errors.New("resource already exists")

	// ErrConflict indicates that the operation conflicts with the current state of the resource.
	ErrConflict = errors.New("resource conflict")

	// ErrInvalidInput indicates that the provided input is invalid.
	ErrInvalidInput = errors.New("invalid input")

	// ErrConnectionFailed indicates that the storage connection failed.
	ErrConnectionFailed = errors.New("storage connection failed")

	// ErrTimeout indicates that the operation timed out.
	ErrTimeout = errors.New("operation timed out")
)

// ListFilter defines filtering and pagination options for listing incidents.
//
// This struct provides comprehensive filtering capabilities while maintaining
// good performance through proper indexing strategies.
type ListFilter struct {
	// TeamID filters incidents by team ownership
	TeamID string `json:"team_id,omitempty"`

	// Status filters incidents by their current status
	Status []domain.Status `json:"status,omitempty"`

	// Severity filters incidents by severity level
	Severity []domain.Severity `json:"severity,omitempty"`

	// AssigneeID filters incidents by assignee
	AssigneeID string `json:"assignee_id,omitempty"`

	// ServiceID filters incidents by affected service
	ServiceID string `json:"service_id,omitempty"`

	// Labels filters incidents that have all specified labels
	Labels map[string]string `json:"labels,omitempty"`

	// CreatedAfter filters incidents created after this time
	CreatedAfter *time.Time `json:"created_after,omitempty"`

	// CreatedBefore filters incidents created before this time
	CreatedBefore *time.Time `json:"created_before,omitempty"`

	// UpdatedAfter filters incidents updated after this time
	UpdatedAfter *time.Time `json:"updated_after,omitempty"`

	// UpdatedBefore filters incidents updated before this time
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`

	// Search performs text search on title and description
	Search string `json:"search,omitempty"`

	// Limit sets the maximum number of results to return (default: 50, max: 1000)
	Limit int `json:"limit,omitempty"`

	// Offset sets the number of results to skip for pagination
	Offset int `json:"offset,omitempty"`

	// SortBy specifies the field to sort by (created_at, updated_at, severity)
	SortBy string `json:"sort_by,omitempty"`

	// SortOrder specifies the sort direction (asc, desc)
	SortOrder string `json:"sort_order,omitempty"`
}

// Validate checks if the ListFilter parameters are valid and sets defaults.
//
// Returns an error if any filter parameters are invalid or out of acceptable ranges.
func (f *ListFilter) Validate() error {
	// Set default limit if not specified
	if f.Limit <= 0 {
		f.Limit = 50
	}

	// Enforce maximum limit
	if f.Limit > 1000 {
		return ErrInvalidInput
	}

	// Validate offset
	if f.Offset < 0 {
		return ErrInvalidInput
	}

	// Validate time ranges
	if f.CreatedAfter != nil && f.CreatedBefore != nil && f.CreatedAfter.After(*f.CreatedBefore) {
		return ErrInvalidInput
	}

	if f.UpdatedAfter != nil && f.UpdatedBefore != nil && f.UpdatedAfter.After(*f.UpdatedBefore) {
		return ErrInvalidInput
	}

	// Validate sort parameters
	if f.SortBy != "" {
		validSortFields := map[string]bool{
			"created_at": true,
			"updated_at": true,
			"severity":   true,
			"status":     true,
		}
		if !validSortFields[f.SortBy] {
			return ErrInvalidInput
		}
	} else {
		f.SortBy = "created_at" // Default sort
	}

	if f.SortOrder != "" {
		if f.SortOrder != "asc" && f.SortOrder != "desc" {
			return ErrInvalidInput
		}
	} else {
		f.SortOrder = "desc" // Default to newest first
	}

	// Validate status values
	for _, status := range f.Status {
		if !status.IsValid() {
			return ErrInvalidInput
		}
	}

	// Validate severity values
	for _, severity := range f.Severity {
		if !severity.IsValid() {
			return ErrInvalidInput
		}
	}

	return nil
}

// ListResult represents the result of a list operation with pagination metadata.
type ListResult struct {
	// Incidents contains the actual incident data
	Incidents []*domain.Incident `json:"incidents"`

	// Total is the total number of incidents matching the filter (ignoring pagination)
	Total int `json:"total"`

	// Limit is the limit that was applied
	Limit int `json:"limit"`

	// Offset is the offset that was applied
	Offset int `json:"offset"`

	// HasMore indicates if there are more results available
	HasMore bool `json:"has_more"`
}

// IncidentRepository defines the contract for incident storage operations.
//
// This interface abstracts the storage layer, allowing different implementations
// (PostgreSQL, SQLite, in-memory) while maintaining consistent behavior.
// All methods should handle context cancellation and timeouts appropriately.
type IncidentRepository interface {
	// Create stores a new incident in the repository.
	//
	// The incident must have a unique ID and pass domain validation.
	// Returns ErrAlreadyExists if an incident with the same ID already exists.
	// Returns ErrInvalidInput if the incident fails validation.
	//
	// Possible errors:
	//   - ErrAlreadyExists: incident with this ID already exists
	//   - ErrInvalidInput: incident fails validation
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Create(ctx context.Context, incident *domain.Incident) error

	// Get retrieves an incident by its unique identifier.
	//
	// Returns the complete incident including attached signals and events.
	// Returns ErrNotFound if no incident exists with the given ID.
	//
	// Possible errors:
	//   - ErrNotFound: incident does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Get(ctx context.Context, id string) (*domain.Incident, error)

	// List retrieves incidents based on the provided filter criteria.
	//
	// Returns a paginated list of incidents with metadata about the total count.
	// The filter is validated before execution.
	//
	// Possible errors:
	//   - ErrInvalidInput: filter parameters are invalid
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	List(ctx context.Context, filter ListFilter) (*ListResult, error)

	// Update modifies an existing incident in the repository.
	//
	// The incident must exist and pass domain validation.
	// This operation updates the entire incident record.
	// Returns ErrNotFound if the incident doesn't exist.
	// Returns ErrConflict if the incident has been modified by another process.
	//
	// Possible errors:
	//   - ErrNotFound: incident does not exist
	//   - ErrInvalidInput: incident fails validation
	//   - ErrConflict: incident was modified by another process
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Update(ctx context.Context, incident *domain.Incident) error

	// AddEvent appends an event to an incident's history.
	//
	// The event must be valid and reference an existing incident.
	// Events are append-only and cannot be modified once created.
	// Returns ErrNotFound if the referenced incident doesn't exist.
	//
	// Possible errors:
	//   - ErrNotFound: referenced incident does not exist
	//   - ErrInvalidInput: event fails validation
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	AddEvent(ctx context.Context, event *domain.Event) error

	// Delete removes an incident from the repository.
	//
	// This is a soft delete operation that marks the incident as deleted
	// but preserves the data for audit purposes. Related events and signals
	// are also marked as deleted.
	// Returns ErrNotFound if the incident doesn't exist.
	//
	// Possible errors:
	//   - ErrNotFound: incident does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Delete(ctx context.Context, id string) error

	// GetWithEvents retrieves an incident with its complete event history.
	//
	// This is optimized for cases where the full incident context is needed.
	// Events are ordered by occurred_at timestamp.
	// Returns ErrNotFound if no incident exists with the given ID.
	//
	// Possible errors:
	//   - ErrNotFound: incident does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	GetWithEvents(ctx context.Context, id string) (*domain.Incident, error)

	// GetWithSignals retrieves an incident with its attached signals.
	//
	// This is optimized for cases where signal correlation is needed.
	// Signals are ordered by received_at timestamp.
	// Returns ErrNotFound if no incident exists with the given ID.
	//
	// Possible errors:
	//   - ErrNotFound: incident does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	GetWithSignals(ctx context.Context, id string) (*domain.Incident, error)
}

// ScheduleRepository defines the contract for schedule storage operations.
//
// This interface abstracts the storage layer for on-call schedules, allowing different implementations
// (PostgreSQL, SQLite, in-memory) while maintaining consistent behavior.
// All methods should handle context cancellation and timeouts appropriately.
type ScheduleRepository interface {
	// Create stores a new schedule in the repository.
	//
	// The schedule must have a unique ID and pass domain validation.
	// Returns ErrAlreadyExists if a schedule with the same ID already exists.
	// Returns ErrInvalidInput if the schedule fails validation.
	//
	// Possible errors:
	//   - ErrAlreadyExists: schedule with this ID already exists
	//   - ErrInvalidInput: schedule fails validation
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Create(ctx context.Context, schedule *domain.Schedule) error

	// Get retrieves a schedule by its unique identifier.
	//
	// Returns the complete schedule with all rotation information.
	// Returns ErrNotFound if no schedule exists with the given ID.
	//
	// Possible errors:
	//   - ErrNotFound: schedule does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Get(ctx context.Context, id string) (*domain.Schedule, error)

	// GetByTeamID retrieves all schedules for a specific team.
	//
	// Returns a list of schedules belonging to the specified team.
	// Returns an empty list if no schedules exist for the team.
	//
	// Possible errors:
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	GetByTeamID(ctx context.Context, teamID string) ([]*domain.Schedule, error)

	// List retrieves all schedules in the repository.
	//
	// Returns all schedules with optional filtering by enabled status.
	//
	// Possible errors:
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	List(ctx context.Context, enabledOnly bool) ([]*domain.Schedule, error)

	// Update modifies an existing schedule in the repository.
	//
	// The schedule must exist and pass domain validation.
	// This operation updates the entire schedule record.
	// Returns ErrNotFound if the schedule doesn't exist.
	//
	// Possible errors:
	//   - ErrNotFound: schedule does not exist
	//   - ErrInvalidInput: schedule fails validation
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Update(ctx context.Context, schedule *domain.Schedule) error

	// Delete removes a schedule from the repository.
	//
	// This is a hard delete operation that permanently removes the schedule.
	// Returns ErrNotFound if the schedule doesn't exist.
	//
	// Possible errors:
	//   - ErrNotFound: schedule does not exist
	//   - ErrConnectionFailed: storage system is unavailable
	//   - ErrTimeout: operation exceeded context deadline
	Delete(ctx context.Context, id string) error
}
