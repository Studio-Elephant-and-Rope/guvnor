// Package dto provides Data Transfer Objects for the Guvnor incident management API.
//
// This package defines request and response structures for HTTP endpoints,
// providing clear separation between API representations and domain models.
// All DTOs include JSON tags and validation rules following OpenAPI standards.
package dto

import (
	"fmt"
	"strings"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

// CreateIncidentRequest represents the request payload for creating a new incident.
//
// @Description Request body for creating an incident
// @Accept json
type CreateIncidentRequest struct {
	// Title is a brief description of the incident
	// @Description Brief title describing the incident
	// @Example "Database connection timeout in production"
	// @MaxLength 200
	// @Required
	Title string `json:"title" validate:"required,max=200"`

	// Description provides detailed information about the incident
	// @Description Detailed description of the incident
	// @Example "Users are experiencing timeouts when trying to connect to the primary database. Error rates have increased by 300% in the last 5 minutes."
	Description string `json:"description,omitempty"`

	// Severity indicates the impact level of the incident
	// @Description Severity level of the incident
	// @Enum critical,high,medium,low,info
	// @Required
	Severity string `json:"severity" validate:"required,oneof=critical high medium low info"`

	// TeamID identifies the team responsible for handling the incident
	// @Description ID of the team responsible for this incident
	// @Example "team-backend"
	// @Required
	TeamID string `json:"team_id" validate:"required"`

	// ServiceID identifies the affected service (optional)
	// @Description ID of the service affected by this incident
	// @Example "service-database"
	ServiceID string `json:"service_id,omitempty"`

	// Labels provide additional metadata as key-value pairs
	// @Description Additional metadata labels
	// @Example {"environment": "production", "component": "database"}
	Labels map[string]string `json:"labels,omitempty"`
}

// IncidentResponse represents the response payload for incident operations.
//
// @Description Incident details response
type IncidentResponse struct {
	// ID is the unique identifier for the incident
	// @Description Unique incident identifier
	// @Example "01933dad-c5c1-7532-8c72-123456789abc"
	ID string `json:"id"`

	// Title is a brief description of the incident
	// @Description Brief title describing the incident
	// @Example "Database connection timeout in production"
	Title string `json:"title"`

	// Description provides detailed information about the incident
	// @Description Detailed description of the incident
	Description string `json:"description"`

	// Status indicates the current state of the incident
	// @Description Current status of the incident
	// @Enum triggered,acknowledged,investigating,resolved
	Status string `json:"status"`

	// Severity indicates the impact level of the incident
	// @Description Severity level of the incident
	// @Enum critical,high,medium,low,info
	Severity string `json:"severity"`

	// TeamID identifies the team responsible for handling the incident
	// @Description ID of the team responsible for this incident
	TeamID string `json:"team_id"`

	// AssigneeID identifies the individual assigned to the incident (optional)
	// @Description ID of the person assigned to this incident
	AssigneeID string `json:"assignee_id,omitempty"`

	// ServiceID identifies the affected service (optional)
	// @Description ID of the service affected by this incident
	ServiceID string `json:"service_id,omitempty"`

	// Labels provide additional metadata as key-value pairs
	// @Description Additional metadata labels
	Labels map[string]string `json:"labels,omitempty"`

	// CreatedAt is the timestamp when the incident was created
	// @Description Timestamp when the incident was created
	// @Example "2024-01-15T10:30:00Z"
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the timestamp when the incident was last updated
	// @Description Timestamp when the incident was last updated
	// @Example "2024-01-15T10:35:00Z"
	UpdatedAt time.Time `json:"updated_at"`

	// ResolvedAt is the timestamp when the incident was resolved (optional)
	// @Description Timestamp when the incident was resolved
	// @Example "2024-01-15T11:00:00Z"
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	// Signals contains the monitoring signals that triggered this incident
	// @Description Monitoring signals associated with this incident
	Signals []SignalResponse `json:"signals,omitempty"`

	// Events contains the history of actions taken on this incident
	// @Description History of events for this incident
	Events []EventResponse `json:"events,omitempty"`
}

// ListIncidentsRequest represents query parameters for listing incidents.
//
// @Description Query parameters for listing incidents
type ListIncidentsRequest struct {
	// TeamID filters incidents by team ownership
	// @Description Filter by team ID
	// @Example "team-backend"
	TeamID string `form:"team_id"`

	// Status filters incidents by current status
	// @Description Filter by incident status (comma-separated for multiple)
	// @Example "triggered,acknowledged"
	// @Enum triggered,acknowledged,investigating,resolved
	Status string `form:"status"`

	// Severity filters incidents by severity level
	// @Description Filter by severity (comma-separated for multiple)
	// @Example "critical,high"
	// @Enum critical,high,medium,low,info
	Severity string `form:"severity"`

	// AssigneeID filters incidents by assignee
	// @Description Filter by assignee ID
	// @Example "user-123"
	AssigneeID string `form:"assignee_id"`

	// ServiceID filters incidents by affected service
	// @Description Filter by service ID
	// @Example "service-database"
	ServiceID string `form:"service_id"`

	// Search performs text search on title and description
	// @Description Text search in title and description
	// @Example "database timeout"
	Search string `form:"search"`

	// Limit sets the maximum number of results to return
	// @Description Maximum number of results (1-1000)
	// @Minimum 1
	// @Maximum 1000
	// @Default 50
	Limit int `form:"limit"`

	// Offset sets the number of results to skip for pagination
	// @Description Number of results to skip
	// @Minimum 0
	// @Default 0
	Offset int `form:"offset"`

	// SortBy specifies the field to sort by
	// @Description Field to sort by
	// @Enum created_at,updated_at,severity,status
	// @Default created_at
	SortBy string `form:"sort_by"`

	// SortOrder specifies the sort direction
	// @Description Sort direction
	// @Enum asc,desc
	// @Default desc
	SortOrder string `form:"sort_order"`
}

// ListIncidentsResponse represents the response for listing incidents.
//
// @Description Paginated list of incidents
type ListIncidentsResponse struct {
	// Incidents contains the list of incidents
	// @Description List of incidents
	Incidents []IncidentResponse `json:"incidents"`

	// Total is the total number of incidents matching the filter
	// @Description Total number of incidents (ignoring pagination)
	Total int `json:"total"`

	// Limit is the limit that was applied
	// @Description Maximum number of results per page
	Limit int `json:"limit"`

	// Offset is the offset that was applied
	// @Description Number of results skipped
	Offset int `json:"offset"`

	// HasMore indicates if there are more results available
	// @Description Whether more results are available
	HasMore bool `json:"has_more"`
}

// UpdateStatusRequest represents the request payload for updating incident status.
//
// @Description Request body for updating incident status
// @Accept json
type UpdateStatusRequest struct {
	// Status is the new status for the incident
	// @Description New status for the incident
	// @Enum triggered,acknowledged,investigating,resolved
	// @Required
	Status string `json:"status" validate:"required,oneof=triggered acknowledged investigating resolved"`

	// Actor identifies who is making the status change
	// @Description ID of the person making this change
	// @Example "user-123"
	Actor string `json:"actor,omitempty"`

	// Comment provides additional context for the status change
	// @Description Optional comment explaining the status change
	// @Example "Database connection restored, monitoring for stability"
	Comment string `json:"comment,omitempty"`
}

// AddEventRequest represents the request payload for adding an event to an incident.
//
// @Description Request body for adding an event to an incident
// @Accept json
type AddEventRequest struct {
	// Type describes the kind of event
	// @Description Type of event being recorded
	// @Example "status_changed"
	// @Required
	Type string `json:"type" validate:"required"`

	// Actor identifies who performed the action
	// @Description ID of the person who performed this action
	// @Example "user-123"
	Actor string `json:"actor,omitempty"`

	// Description provides details about the event
	// @Description Description of what happened
	// @Example "Escalated to senior engineer due to impact severity"
	// @Required
	Description string `json:"description" validate:"required"`

	// Metadata contains additional structured data about the event
	// @Description Additional metadata for the event
	// @Example {"previous_assignee": "user-456", "escalation_reason": "expertise_required"}
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// EventResponse represents an event in the incident timeline.
//
// @Description Event in the incident timeline
type EventResponse struct {
	// ID is the unique identifier for the event
	// @Description Unique event identifier
	ID string `json:"id"`

	// IncidentID identifies the incident this event belongs to
	// @Description ID of the incident this event belongs to
	IncidentID string `json:"incident_id"`

	// Type describes the kind of event
	// @Description Type of event
	Type string `json:"type"`

	// Actor identifies who performed the action
	// @Description ID of the person who performed this action
	Actor string `json:"actor"`

	// Description provides details about the event
	// @Description Description of what happened
	Description string `json:"description"`

	// Metadata contains additional structured data about the event
	// @Description Additional metadata for the event
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// OccurredAt is the timestamp when the event occurred
	// @Description Timestamp when the event occurred
	OccurredAt time.Time `json:"occurred_at"`
}

// SignalResponse represents a monitoring signal attached to an incident.
//
// @Description Monitoring signal attached to an incident
type SignalResponse struct {
	// ID is the unique identifier for the signal
	// @Description Unique signal identifier
	ID string `json:"id"`

	// Source identifies where the signal originated
	// @Description Source system that generated this signal
	Source string `json:"source"`

	// Title is a brief description of the signal
	// @Description Brief title describing the signal
	Title string `json:"title"`

	// Description provides detailed information about the signal
	// @Description Detailed description of the signal
	Description string `json:"description"`

	// Severity indicates the impact level of the signal
	// @Description Severity level of the signal
	Severity string `json:"severity"`

	// Labels provide additional metadata as key-value pairs
	// @Description Additional metadata labels
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations provide additional context information
	// @Description Additional context annotations
	Annotations map[string]string `json:"annotations,omitempty"`

	// Payload contains the raw signal data
	// @Description Raw signal data payload
	Payload map[string]interface{} `json:"payload,omitempty"`

	// ReceivedAt is the timestamp when the signal was received
	// @Description Timestamp when the signal was received
	ReceivedAt time.Time `json:"received_at"`
}

// ErrorResponse represents an error response from the API.
//
// @Description Error response structure
type ErrorResponse struct {
	// Error is the human-readable error message
	// @Description Human-readable error message
	// @Example "Invalid input: title is required"
	Error string `json:"error"`

	// Code is the machine-readable error code
	// @Description Machine-readable error code
	// @Example "VALIDATION_ERROR"
	Code string `json:"code"`

	// RequestID helps with debugging and correlation
	// @Description Request ID for debugging
	// @Example "01933dad-c5c1-7532-8c72-123456789abc"
	RequestID string `json:"request_id"`

	// Details provides additional context (optional)
	// @Description Additional error details
	Details map[string]interface{} `json:"details,omitempty"`
}

// Validate validates the CreateIncidentRequest.
func (r *CreateIncidentRequest) Validate() error {
	var errors []string

	if strings.TrimSpace(r.Title) == "" {
		errors = append(errors, "title is required")
	}
	if len(r.Title) > 200 {
		errors = append(errors, "title must not exceed 200 characters")
	}

	if strings.TrimSpace(r.Severity) == "" {
		errors = append(errors, "severity is required")
	} else {
		validSeverities := map[string]bool{
			"critical": true, "high": true, "medium": true, "low": true, "info": true,
		}
		if !validSeverities[r.Severity] {
			errors = append(errors, "severity must be one of: critical, high, medium, low, info")
		}
	}

	if strings.TrimSpace(r.TeamID) == "" {
		errors = append(errors, "team_id is required")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Validate validates the ListIncidentsRequest.
func (r *ListIncidentsRequest) Validate() error {
	var errors []string

	if r.Limit < 0 {
		r.Limit = 50 // Default limit
	}
	if r.Limit > 1000 {
		errors = append(errors, "limit must not exceed 1000")
	}

	if r.Offset < 0 {
		r.Offset = 0 // Default offset
	}

	if r.SortBy == "" {
		r.SortBy = "created_at" // Default sort field
	} else {
		validSortFields := map[string]bool{
			"created_at": true, "updated_at": true, "severity": true, "status": true,
		}
		if !validSortFields[r.SortBy] {
			errors = append(errors, "sort_by must be one of: created_at, updated_at, severity, status")
		}
	}

	if r.SortOrder == "" {
		r.SortOrder = "desc" // Default sort order
	} else if r.SortOrder != "asc" && r.SortOrder != "desc" {
		errors = append(errors, "sort_order must be either 'asc' or 'desc'")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Validate validates the UpdateStatusRequest.
func (r *UpdateStatusRequest) Validate() error {
	var errors []string

	if strings.TrimSpace(r.Status) == "" {
		errors = append(errors, "status is required")
	} else {
		validStatuses := map[string]bool{
			"triggered": true, "acknowledged": true, "investigating": true, "resolved": true,
		}
		if !validStatuses[r.Status] {
			errors = append(errors, "status must be one of: triggered, acknowledged, investigating, resolved")
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// Validate validates the AddEventRequest.
func (r *AddEventRequest) Validate() error {
	var errors []string

	if strings.TrimSpace(r.Type) == "" {
		errors = append(errors, "type is required")
	}

	if strings.TrimSpace(r.Description) == "" {
		errors = append(errors, "description is required")
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed: %s", strings.Join(errors, "; "))
	}

	return nil
}

// ToIncident converts a CreateIncidentRequest to a domain.Incident.
func (r *CreateIncidentRequest) ToIncident() (*domain.Incident, error) {
	// Parse severity
	var severity domain.Severity
	switch r.Severity {
	case "critical":
		severity = domain.SeverityCritical
	case "high":
		severity = domain.SeverityHigh
	case "medium":
		severity = domain.SeverityMedium
	case "low":
		severity = domain.SeverityLow
	case "info":
		severity = domain.SeverityInfo
	default:
		return nil, fmt.Errorf("invalid severity: %s", r.Severity)
	}

	now := time.Now().UTC()
	incident := &domain.Incident{
		Title:       r.Title,
		Description: r.Description,
		Status:      domain.StatusTriggered,
		Severity:    severity,
		TeamID:      r.TeamID,
		ServiceID:   r.ServiceID,
		Labels:      r.Labels,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	return incident, nil
}

// FromIncident converts a domain.Incident to an IncidentResponse.
func FromIncident(incident *domain.Incident) *IncidentResponse {
	response := &IncidentResponse{
		ID:          incident.ID,
		Title:       incident.Title,
		Description: incident.Description,
		Status:      incident.Status.String(),
		Severity:    incident.Severity.String(),
		TeamID:      incident.TeamID,
		AssigneeID:  incident.AssigneeID,
		ServiceID:   incident.ServiceID,
		Labels:      incident.Labels,
		CreatedAt:   incident.CreatedAt,
		UpdatedAt:   incident.UpdatedAt,
		ResolvedAt:  incident.ResolvedAt,
	}

	// Convert signals
	if len(incident.Signals) > 0 {
		response.Signals = make([]SignalResponse, len(incident.Signals))
		for i, signal := range incident.Signals {
			response.Signals[i] = FromSignal(signal)
		}
	}

	// Convert events
	if len(incident.Events) > 0 {
		response.Events = make([]EventResponse, len(incident.Events))
		for i, event := range incident.Events {
			response.Events[i] = FromEvent(event)
		}
	}

	return response
}

// FromSignal converts a domain.Signal to a SignalResponse.
func FromSignal(signal domain.Signal) SignalResponse {
	return SignalResponse{
		ID:          signal.ID,
		Source:      signal.Source,
		Title:       signal.Title,
		Description: signal.Description,
		Severity:    signal.Severity.String(),
		Labels:      signal.Labels,
		Annotations: signal.Annotations,
		Payload:     signal.Payload,
		ReceivedAt:  signal.ReceivedAt,
	}
}

// FromEvent converts a domain.Event to an EventResponse.
func FromEvent(event domain.Event) EventResponse {
	return EventResponse{
		ID:          event.ID,
		IncidentID:  event.IncidentID,
		Type:        event.Type,
		Actor:       event.Actor,
		Description: event.Description,
		Metadata:    event.Metadata,
		OccurredAt:  event.OccurredAt,
	}
}

// ToEvent converts an AddEventRequest to a domain.Event.
func (r *AddEventRequest) ToEvent(incidentID string) *domain.Event {
	return &domain.Event{
		IncidentID:  incidentID,
		Type:        r.Type,
		Actor:       r.Actor,
		Description: r.Description,
		Metadata:    r.Metadata,
		OccurredAt:  time.Now().UTC(),
	}
}
