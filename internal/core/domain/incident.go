// Package domain contains the core domain models for the Guvnor incident management platform.
//
// This package defines the fundamental types and business logic for incidents,
// signals, and events without any external dependencies.
package domain

import (
	"errors"
	"fmt"
	"time"
)

// Status represents the current state of an incident.
type Status string

// Status constants define the valid incident statuses and their transitions.
const (
	StatusTriggered     Status = "triggered"
	StatusAcknowledged  Status = "acknowledged"
	StatusInvestigating Status = "investigating"
	StatusResolved      Status = "resolved"
)

// String returns the string representation of the status.
func (s Status) String() string {
	return string(s)
}

// IsValid checks if the status is one of the defined valid statuses.
func (s Status) IsValid() bool {
	switch s {
	case StatusTriggered, StatusAcknowledged, StatusInvestigating, StatusResolved:
		return true
	default:
		return false
	}
}

// Severity represents the criticality level of an incident.
type Severity string

// Severity constants define the valid incident severity levels.
const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
	SeverityInfo     Severity = "info"
)

// String returns the string representation of the severity.
func (s Severity) String() string {
	return string(s)
}

// IsValid checks if the severity is one of the defined valid severities.
func (s Severity) IsValid() bool {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow, SeverityInfo:
		return true
	default:
		return false
	}
}

// ChannelType represents the notification channel type.
type ChannelType string

// ChannelType constants define the valid notification channels.
const (
	ChannelTypeEmail     ChannelType = "email"
	ChannelTypeSlack     ChannelType = "slack"
	ChannelTypeSMS       ChannelType = "sms"
	ChannelTypeWebhook   ChannelType = "webhook"
	ChannelTypePagerDuty ChannelType = "pagerduty"
)

// String returns the string representation of the channel type.
func (c ChannelType) String() string {
	return string(c)
}

// IsValid checks if the channel type is one of the defined valid types.
func (c ChannelType) IsValid() bool {
	switch c {
	case ChannelTypeEmail, ChannelTypeSlack, ChannelTypeSMS, ChannelTypeWebhook, ChannelTypePagerDuty:
		return true
	default:
		return false
	}
}

// Signal represents an incoming alert or notification that can trigger an incident.
type Signal struct {
	ID          string                 `json:"id"`
	Source      string                 `json:"source"`
	Title       string                 `json:"title"`
	Description string                 `json:"description"`
	Severity    Severity               `json:"severity"`
	Labels      map[string]string      `json:"labels"`
	Annotations map[string]string      `json:"annotations"`
	Payload     map[string]interface{} `json:"payload"`
	ReceivedAt  time.Time              `json:"received_at"`
}

// Validate checks if the signal has all required fields and valid values.
//
// Returns an error if validation fails, detailing what is invalid.
func (s *Signal) Validate() error {
	if s.ID == "" {
		return errors.New("signal ID is required")
	}
	if s.Source == "" {
		return errors.New("signal source is required")
	}
	if s.Title == "" {
		return errors.New("signal title is required")
	}
	if !s.Severity.IsValid() {
		return fmt.Errorf("invalid severity: %s", s.Severity)
	}
	if s.ReceivedAt.IsZero() {
		return errors.New("signal received_at timestamp is required")
	}
	return nil
}

// Event represents a recorded action or change within an incident.
type Event struct {
	ID          string                 `json:"id"`
	IncidentID  string                 `json:"incident_id"`
	Type        string                 `json:"type"`
	Actor       string                 `json:"actor"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	OccurredAt  time.Time              `json:"occurred_at"`
}

// Validate checks if the event has all required fields.
//
// Returns an error if validation fails, detailing what is invalid.
func (e *Event) Validate() error {
	if e.ID == "" {
		return errors.New("event ID is required")
	}
	if e.IncidentID == "" {
		return errors.New("event incident_id is required")
	}
	if e.Type == "" {
		return errors.New("event type is required")
	}
	if e.Actor == "" {
		return errors.New("event actor is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("event occurred_at timestamp is required")
	}
	return nil
}

// Incident represents a service disruption or issue that requires attention.
type Incident struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      Status            `json:"status"`
	Severity    Severity          `json:"severity"`
	TeamID      string            `json:"team_id"`
	AssigneeID  string            `json:"assignee_id,omitempty"`
	ServiceID   string            `json:"service_id,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	ResolvedAt  *time.Time        `json:"resolved_at,omitempty"`

	// Related entities
	Signals []Signal `json:"signals,omitempty"`
	Events  []Event  `json:"events,omitempty"`
}

// Validate checks if the incident has all required fields and valid values.
//
// Returns an error if validation fails, detailing what is invalid.
//
// Possible errors:
//   - ErrMissingID: incident ID is empty
//   - ErrMissingTitle: incident title is empty
//   - ErrInvalidStatus: status is not one of the valid statuses
//   - ErrInvalidSeverity: severity is not one of the valid severities
//   - ErrMissingTeam: team ID is empty
//   - ErrInvalidTimestamp: created_at or updated_at is zero
func (i *Incident) Validate() error {
	if i.ID == "" {
		return errors.New("incident ID is required")
	}
	if i.Title == "" {
		return errors.New("incident title is required")
	}
	if i.Title != "" && len(i.Title) > 200 {
		return errors.New("incident title exceeds maximum length of 200 characters")
	}
	if !i.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", i.Status)
	}
	if !i.Severity.IsValid() {
		return fmt.Errorf("invalid severity: %s", i.Severity)
	}
	if i.TeamID == "" {
		return errors.New("incident team_id is required")
	}
	if i.CreatedAt.IsZero() {
		return errors.New("incident created_at timestamp is required")
	}
	if i.UpdatedAt.IsZero() {
		return errors.New("incident updated_at timestamp is required")
	}
	if i.UpdatedAt.Before(i.CreatedAt) {
		return errors.New("incident updated_at cannot be before created_at")
	}
	if i.Status == StatusResolved && i.ResolvedAt == nil {
		return errors.New("resolved incidents must have a resolved_at timestamp")
	}
	if i.Status != StatusResolved && i.ResolvedAt != nil {
		return errors.New("only resolved incidents can have a resolved_at timestamp")
	}

	// Validate attached signals
	for idx, signal := range i.Signals {
		if err := signal.Validate(); err != nil {
			return fmt.Errorf("invalid signal at index %d: %w", idx, err)
		}
	}

	// Validate attached events
	for idx, event := range i.Events {
		if err := event.Validate(); err != nil {
			return fmt.Errorf("invalid event at index %d: %w", idx, err)
		}
		if event.IncidentID != i.ID {
			return fmt.Errorf("event at index %d has mismatched incident_id", idx)
		}
	}

	return nil
}

// CanTransitionTo checks if the incident can transition from its current status to the target status.
//
// Valid transitions:
//   - triggered → acknowledged
//   - triggered → investigating
//   - triggered → resolved
//   - acknowledged → investigating
//   - acknowledged → resolved
//   - investigating → resolved
//   - Any valid status → triggered (reopening)
//
// Returns true if the transition is valid, false otherwise.
func (i *Incident) CanTransitionTo(targetStatus Status) bool {
	if !targetStatus.IsValid() {
		return false
	}

	if !i.Status.IsValid() {
		return false
	}

	// Same status is always valid (no-op)
	if i.Status == targetStatus {
		return true
	}

	// Can always reopen an incident (transition to triggered) from valid statuses
	if targetStatus == StatusTriggered {
		return true
	}

	switch i.Status {
	case StatusTriggered:
		// From triggered, can go to acknowledged, investigating, or resolved
		return targetStatus == StatusAcknowledged ||
			targetStatus == StatusInvestigating ||
			targetStatus == StatusResolved
	case StatusAcknowledged:
		// From acknowledged, can go to investigating or resolved
		return targetStatus == StatusInvestigating ||
			targetStatus == StatusResolved
	case StatusInvestigating:
		// From investigating, can only go to resolved
		return targetStatus == StatusResolved
	case StatusResolved:
		// From resolved, can only go to triggered (reopening)
		return targetStatus == StatusTriggered
	default:
		return false
	}
}

// AttachSignal adds a signal to the incident if it's not already attached.
//
// The signal is validated before attachment. If validation fails,
// an error is returned and the signal is not attached.
func (i *Incident) AttachSignal(signal Signal) error {
	if err := signal.Validate(); err != nil {
		return fmt.Errorf("cannot attach invalid signal: %w", err)
	}

	// Check if signal is already attached
	for _, existingSignal := range i.Signals {
		if existingSignal.ID == signal.ID {
			return nil // Signal already attached, no error
		}
	}

	i.Signals = append(i.Signals, signal)
	i.UpdatedAt = time.Now().UTC()
	return nil
}

// RecordEvent adds an event to the incident's history.
//
// The event is validated before recording. If validation fails,
// an error is returned and the event is not recorded.
// The event's incident_id is automatically set to match this incident.
func (i *Incident) RecordEvent(event Event) error {
	// Set the incident ID to ensure consistency
	event.IncidentID = i.ID

	if err := event.Validate(); err != nil {
		return fmt.Errorf("cannot record invalid event: %w", err)
	}

	i.Events = append(i.Events, event)
	i.UpdatedAt = time.Now().UTC()
	return nil
}

// TransitionTo changes the incident status to the target status if the transition is valid.
//
// This method automatically handles side effects of status transitions:
//   - Sets resolved_at timestamp when transitioning to resolved
//   - Clears resolved_at timestamp when transitioning away from resolved
//   - Updates the updated_at timestamp
//   - Records a status change event
//
// Returns an error if the transition is invalid or if recording the event fails.
func (i *Incident) TransitionTo(targetStatus Status, actor string) error {
	if !i.CanTransitionTo(targetStatus) {
		return fmt.Errorf("cannot transition from %s to %s", i.Status, targetStatus)
	}

	oldStatus := i.Status
	i.Status = targetStatus
	i.UpdatedAt = time.Now().UTC()

	// Handle resolved status side effects
	if targetStatus == StatusResolved && i.ResolvedAt == nil {
		now := time.Now().UTC()
		i.ResolvedAt = &now
	} else if targetStatus != StatusResolved && i.ResolvedAt != nil {
		i.ResolvedAt = nil
	}

	// Record status change event
	event := Event{
		ID:          fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:        "status_changed",
		Actor:       actor,
		Description: fmt.Sprintf("Status changed from %s to %s", oldStatus, targetStatus),
		Metadata: map[string]interface{}{
			"old_status": oldStatus.String(),
			"new_status": targetStatus.String(),
		},
		OccurredAt: i.UpdatedAt,
	}

	return i.RecordEvent(event)
}
