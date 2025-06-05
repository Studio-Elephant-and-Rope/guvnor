// Package services provides business logic orchestration for the Guvnor incident management platform.
//
// This package contains service implementations that coordinate between domain objects
// and repository interfaces. Services handle business rules, validation, event generation,
// and cross-cutting concerns like logging and metrics.
//
// All services follow dependency injection patterns and are designed for testability
// with proper error handling and context awareness.
package services

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// IncidentService orchestrates incident management business logic.
//
// This service handles all incident-related operations including creation, updates,
// status transitions, responder management, and event recording. It enforces
// business rules while delegating persistence to repository implementations.
//
// Key responsibilities:
//   - Incident creation with auto-generated identifiers
//   - Status transition validation and management
//   - Responder assignment and management
//   - Event recording for audit trails
//   - Business rule enforcement
//   - Cross-cutting concerns (logging, metrics)
//
// The service is designed for concurrent access and handles all edge cases
// gracefully with proper error responses.
type IncidentService struct {
	repo      ports.IncidentRepository
	logger    *logging.Logger
	idCounter *incidentCounter
}

// incidentCounter manages auto-generated incident identifiers.
//
// Incident IDs follow the format: INC-YYYY-NNN where:
//   - INC is the fixed prefix
//   - YYYY is the current year
//   - NNN is a zero-padded sequential number (001, 002, etc.)
//
// The counter is thread-safe and handles year rollovers automatically.
type incidentCounter struct {
	mu       sync.Mutex
	year     int
	sequence int
}

// NewIncidentService creates a new incident service with the provided dependencies.
//
// The service requires a repository implementation for persistence and a logger
// for structured logging. Both dependencies are required and the function will
// return an error if either is nil.
//
// Possible errors:
//   - ErrInvalidInput: repository or logger is nil
func NewIncidentService(repo ports.IncidentRepository, logger *logging.Logger) (*IncidentService, error) {
	if repo == nil {
		return nil, fmt.Errorf("%w: repository cannot be nil", ports.ErrInvalidInput)
	}
	if logger == nil {
		return nil, fmt.Errorf("%w: logger cannot be nil", ports.ErrInvalidInput)
	}

	return &IncidentService{
		repo:   repo,
		logger: logger,
		idCounter: &incidentCounter{
			year:     time.Now().UTC().Year(),
			sequence: 0,
		},
	}, nil
}

// CreateIncident creates a new incident with the specified title and severity.
//
// This method handles the complete incident creation workflow:
//   - Generates a unique UUID and human-readable identifier
//   - Sets appropriate timestamps and initial status
//   - Validates the incident against domain rules
//   - Persists the incident via repository
//   - Records a creation event for audit trail
//   - Returns the fully populated incident
//
// The incident is created in "triggered" status and will have both a UUID
// for internal references and a human-readable ID (INC-2024-001 format)
// for external communication.
//
// Possible errors:
//   - ErrInvalidInput: title is empty, exceeds length limit, or severity is invalid
//   - ErrAlreadyExists: generated ID conflicts (highly unlikely)
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) CreateIncident(ctx context.Context, title, severity, teamID string) (*domain.Incident, error) {
	logger := s.logger.WithFields("operation", "create_incident", "title", title, "severity", severity, "team_id", teamID)
	logger.Debug("Creating new incident")

	// Validate inputs
	if title == "" {
		logger.Debug("Incident creation failed: empty title")
		return nil, fmt.Errorf("%w: title cannot be empty", ports.ErrInvalidInput)
	}

	if len(title) > 200 {
		logger.Debug("Incident creation failed: title too long")
		return nil, fmt.Errorf("%w: title exceeds maximum length of 200 characters", ports.ErrInvalidInput)
	}

	if teamID == "" {
		logger.Debug("Incident creation failed: empty team ID")
		return nil, fmt.Errorf("%w: team ID cannot be empty", ports.ErrInvalidInput)
	}

	// Parse and validate severity
	sev := domain.Severity(severity)
	if !sev.IsValid() {
		logger.WithFields("provided_severity", severity).Debug("Incident creation failed: invalid severity")
		return nil, fmt.Errorf("%w: invalid severity '%s', must be one of: critical, high, medium, low, info", ports.ErrInvalidInput, severity)
	}

	// Generate identifiers
	id := uuid.New().String()
	humanID := s.generateHumanReadableID()

	// Set timestamps
	now := time.Now().UTC()

	// Create incident
	incident := &domain.Incident{
		ID:          id,
		Title:       title,
		Description: fmt.Sprintf("Incident %s: %s", humanID, title),
		Status:      domain.StatusTriggered,
		Severity:    sev,
		TeamID:      teamID,
		CreatedAt:   now,
		UpdatedAt:   now,
		Labels:      map[string]string{"incident_id": humanID},
	}

	// Validate incident
	if err := incident.Validate(); err != nil {
		logger.WithError(err).Error("Incident validation failed")
		return nil, fmt.Errorf("%w: incident validation failed: %v", ports.ErrInvalidInput, err)
	}

	// Store incident
	if err := s.repo.Create(ctx, incident); err != nil {
		logger.WithError(err).Error("Failed to store incident")
		return nil, err
	}

	// Record creation event
	creationEvent := &domain.Event{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        "incident_created",
		Actor:       "system", // Will be replaced with actual user when auth is implemented
		Description: fmt.Sprintf("Incident %s created with severity %s", humanID, severity),
		Metadata: map[string]interface{}{
			"human_id":       humanID,
			"initial_status": string(domain.StatusTriggered),
			"severity":       string(sev),
		},
		OccurredAt: now,
	}

	if err := s.repo.AddEvent(ctx, creationEvent); err != nil {
		logger.WithError(err).Warn("Failed to record creation event")
		// Don't fail the operation for event recording failures
	}

	logger.WithFields("incident_id", incident.ID, "human_id", humanID).Info("Incident created successfully")
	return incident, nil
}

// GetIncident retrieves an incident by its unique identifier.
//
// This method fetches the complete incident including all attached signals
// and events. The incident is returned with its full audit trail and
// current state.
//
// Possible errors:
//   - ErrInvalidInput: id is empty
//   - ErrNotFound: incident does not exist
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) GetIncident(ctx context.Context, id string) (*domain.Incident, error) {
	if id == "" {
		return nil, fmt.Errorf("%w: incident ID cannot be empty", ports.ErrInvalidInput)
	}

	logger := s.logger.WithFields("operation", "get_incident", "incident_id", id)
	logger.Debug("Retrieving incident")

	incident, err := s.repo.Get(ctx, id)
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve incident")
		return nil, err
	}

	logger.Debug("Incident retrieved successfully")
	return incident, nil
}

// ListIncidents retrieves incidents based on the provided filter criteria.
//
// This method supports comprehensive filtering, sorting, and pagination
// of incidents. The filter is validated before execution to ensure
// proper query construction.
//
// Possible errors:
//   - ErrInvalidInput: filter parameters are invalid
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) ListIncidents(ctx context.Context, filter ports.ListFilter) (*ports.ListResult, error) {
	logger := s.logger.WithFields("operation", "list_incidents", "limit", filter.Limit, "offset", filter.Offset)
	logger.Debug("Listing incidents with filters")

	// Validate filter
	if err := filter.Validate(); err != nil {
		logger.WithError(err).Debug("Invalid filter parameters")
		return nil, err
	}

	result, err := s.repo.List(ctx, filter)
	if err != nil {
		logger.WithError(err).Error("Failed to list incidents")
		return nil, err
	}

	logger.WithFields("total", result.Total, "returned", len(result.Incidents)).Debug("Incidents listed successfully")
	return result, nil
}

// UpdateStatus transitions an incident to a new status.
//
// This method enforces status transition rules defined in the domain model.
// Valid transitions are validated before applying the change, and an event
// is recorded for the audit trail.
//
// Valid status transitions:
//   - triggered → acknowledged, investigating, resolved
//   - acknowledged → investigating, resolved, triggered (reopen)
//   - investigating → resolved, triggered (reopen)
//   - resolved → triggered (reopen)
//
// Possible errors:
//   - ErrInvalidInput: incident ID is empty or status is invalid
//   - ErrNotFound: incident does not exist
//   - ErrConflict: status transition is not allowed or concurrent modification
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) UpdateStatus(ctx context.Context, id string, status domain.Status) error {
	if id == "" {
		return fmt.Errorf("%w: incident ID cannot be empty", ports.ErrInvalidInput)
	}

	if !status.IsValid() {
		return fmt.Errorf("%w: invalid status '%s'", ports.ErrInvalidInput, status)
	}

	logger := s.logger.WithFields("operation", "update_status", "incident_id", id, "new_status", string(status))
	logger.Debug("Updating incident status")

	// Get current incident
	incident, err := s.repo.Get(ctx, id)
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve incident for status update")
		return err
	}

	oldStatus := incident.Status

	// Check if transition is valid
	if !incident.CanTransitionTo(status) {
		logger.WithFields("current_status", string(oldStatus), "target_status", string(status)).
			Warn("Invalid status transition attempted")
		return fmt.Errorf("%w: cannot transition from %s to %s", ports.ErrConflict, oldStatus, status)
	}

	// Apply status transition
	if err := incident.TransitionTo(status, "system"); err != nil {
		logger.WithError(err).Error("Failed to apply status transition")
		return fmt.Errorf("%w: status transition failed: %v", ports.ErrConflict, err)
	}

	// Update incident
	if err := s.repo.Update(ctx, incident); err != nil {
		logger.WithError(err).Error("Failed to update incident status")
		return err
	}

	// Record status change event
	statusEvent := &domain.Event{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        "status_changed",
		Actor:       "system", // Will be replaced with actual user when auth is implemented
		Description: fmt.Sprintf("Status changed from %s to %s", oldStatus, status),
		Metadata: map[string]interface{}{
			"old_status": string(oldStatus),
			"new_status": string(status),
		},
		OccurredAt: time.Now().UTC(),
	}

	if err := s.repo.AddEvent(ctx, statusEvent); err != nil {
		logger.WithError(err).Warn("Failed to record status change event")
		// Don't fail the operation for event recording failures
	}

	logger.WithFields("old_status", string(oldStatus), "new_status", string(status)).
		Info("Incident status updated successfully")
	return nil
}

// AddResponder assigns a responder to an incident.
//
// This method adds a responder to the incident's team and records the
// assignment as an event. The responder ID should be a valid user
// identifier in the system.
//
// Note: In the current implementation, responders are stored in the
// AssigneeID field. Future versions may support multiple responders.
//
// Possible errors:
//   - ErrInvalidInput: incident ID or responder ID is empty
//   - ErrNotFound: incident does not exist
//   - ErrConflict: concurrent modification detected
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) AddResponder(ctx context.Context, incidentID, responderID string) error {
	if incidentID == "" {
		return fmt.Errorf("%w: incident ID cannot be empty", ports.ErrInvalidInput)
	}

	if responderID == "" {
		return fmt.Errorf("%w: responder ID cannot be empty", ports.ErrInvalidInput)
	}

	logger := s.logger.WithFields("operation", "add_responder", "incident_id", incidentID, "responder_id", responderID)
	logger.Debug("Adding responder to incident")

	// Get current incident
	incident, err := s.repo.Get(ctx, incidentID)
	if err != nil {
		logger.WithError(err).Error("Failed to retrieve incident for responder assignment")
		return err
	}

	oldAssignee := incident.AssigneeID

	// Assign responder (currently single assignee only)
	incident.AssigneeID = responderID
	incident.UpdatedAt = time.Now().UTC()

	// Update incident
	if err := s.repo.Update(ctx, incident); err != nil {
		logger.WithError(err).Error("Failed to update incident with responder")
		return err
	}

	// Record responder assignment event
	var description string
	var eventType string
	if oldAssignee == "" {
		description = fmt.Sprintf("Responder %s assigned to incident", responderID)
		eventType = "responder_assigned"
	} else {
		description = fmt.Sprintf("Responder changed from %s to %s", oldAssignee, responderID)
		eventType = "responder_changed"
	}

	responderEvent := &domain.Event{
		ID:          uuid.New().String(),
		IncidentID:  incident.ID,
		Type:        eventType,
		Actor:       "system", // Will be replaced with actual user when auth is implemented
		Description: description,
		Metadata: map[string]interface{}{
			"responder_id":  responderID,
			"old_assignee":  oldAssignee,
			"new_assignee":  responderID,
			"assignment_at": time.Now().UTC(),
		},
		OccurredAt: time.Now().UTC(),
	}

	if err := s.repo.AddEvent(ctx, responderEvent); err != nil {
		logger.WithError(err).Warn("Failed to record responder assignment event")
		// Don't fail the operation for event recording failures
	}

	logger.WithFields("old_assignee", oldAssignee, "new_assignee", responderID).
		Info("Responder added to incident successfully")
	return nil
}

// RecordEvent adds a custom event to an incident's audit trail.
//
// This method allows external systems or users to record significant
// events related to an incident. The event is validated before being
// stored and must contain all required fields.
//
// Common event types include:
//   - investigation_started
//   - mitigation_applied
//   - external_communication
//   - escalation
//   - note_added
//
// Possible errors:
//   - ErrInvalidInput: incident ID is empty or event is invalid
//   - ErrNotFound: incident does not exist
//   - ErrConnectionFailed: repository is unavailable
//   - ErrTimeout: operation exceeded context deadline
func (s *IncidentService) RecordEvent(ctx context.Context, incidentID string, event domain.Event) error {
	if incidentID == "" {
		return fmt.Errorf("%w: incident ID cannot be empty", ports.ErrInvalidInput)
	}

	logger := s.logger.WithFields("operation", "record_event", "incident_id", incidentID, "event_type", event.Type)
	logger.Debug("Recording custom event for incident")

	// Ensure event references the correct incident
	event.IncidentID = incidentID

	// Generate ID if not provided
	if event.ID == "" {
		event.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now().UTC()
	}

	// Validate event
	if err := event.Validate(); err != nil {
		logger.WithError(err).Debug("Event validation failed")
		return fmt.Errorf("%w: event validation failed: %v", ports.ErrInvalidInput, err)
	}

	// Check incident exists before adding event
	_, err := s.repo.Get(ctx, incidentID)
	if err != nil {
		logger.WithError(err).Error("Failed to verify incident exists before recording event")
		return err
	}

	// Store event
	if err := s.repo.AddEvent(ctx, &event); err != nil {
		logger.WithError(err).Error("Failed to record event")
		return err
	}

	logger.WithFields("event_id", event.ID, "event_type", event.Type).
		Info("Event recorded successfully")
	return nil
}

// generateHumanReadableID creates a human-readable incident identifier.
//
// The format is INC-YYYY-NNN where:
//   - INC is the fixed prefix
//   - YYYY is the current year
//   - NNN is a zero-padded sequential number
//
// This method is thread-safe and handles year rollovers automatically.
// The sequence resets to 1 at the beginning of each year.
func (s *IncidentService) generateHumanReadableID() string {
	s.idCounter.mu.Lock()
	defer s.idCounter.mu.Unlock()

	currentYear := time.Now().UTC().Year()

	// Reset sequence if year has changed
	if currentYear != s.idCounter.year {
		s.idCounter.year = currentYear
		s.idCounter.sequence = 0
	}

	// Increment sequence
	s.idCounter.sequence++

	// Format as INC-YYYY-NNN
	return fmt.Sprintf("INC-%d-%s", currentYear, s.formatSequence(s.idCounter.sequence))
}

// formatSequence formats a sequence number with leading zeros.
//
// Numbers are zero-padded to 3 digits (001, 002, etc.) up to 999,
// then expand as needed (1000, 1001, etc.).
func (s *IncidentService) formatSequence(seq int) string {
	if seq < 1000 {
		return fmt.Sprintf("%03d", seq)
	}
	return strconv.Itoa(seq)
}
