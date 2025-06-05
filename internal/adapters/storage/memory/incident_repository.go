// Package memory provides in-memory storage implementations for testing and development.
//
// This package contains thread-safe, in-memory implementations of repository interfaces
// that can be used for testing, local development, and as reference implementations
// for the actual database-backed repositories.
package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
)

// IncidentRepository is a thread-safe in-memory implementation of ports.IncidentRepository.
//
// This implementation stores incidents in memory using maps for quick access.
// It provides full thread safety through read-write mutex protection and supports
// all filtering and sorting operations defined in the interface.
//
// Note: Data is lost when the process terminates. This implementation is intended
// for testing and development use only.
type IncidentRepository struct {
	mu        sync.RWMutex
	incidents map[string]*domain.Incident
	events    map[string][]*domain.Event // incident_id -> events
	nextID    int64                      // For generating event IDs
}

// NewIncidentRepository creates a new in-memory incident repository.
//
// The repository is initialized with empty storage and is ready for use.
// All operations are thread-safe and can be called concurrently.
func NewIncidentRepository() *IncidentRepository {
	return &IncidentRepository{
		incidents: make(map[string]*domain.Incident),
		events:    make(map[string][]*domain.Event),
		nextID:    1,
	}
}

// Create stores a new incident in the repository.
//
// The incident is validated before storage. Returns ErrAlreadyExists if an
// incident with the same ID already exists.
func (r *IncidentRepository) Create(ctx context.Context, incident *domain.Incident) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if incident == nil {
		return ports.ErrInvalidInput
	}

	// Validate incident
	if err := incident.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if incident already exists
	if _, exists := r.incidents[incident.ID]; exists {
		return ports.ErrAlreadyExists
	}

	// Create a deep copy to avoid external modifications
	incidentCopy := r.copyIncident(incident)
	r.incidents[incident.ID] = incidentCopy

	// Store events if any
	if len(incident.Events) > 0 {
		eventsCopy := make([]*domain.Event, len(incident.Events))
		for i, event := range incident.Events {
			eventsCopy[i] = r.copyEvent(&event)
		}
		r.events[incident.ID] = eventsCopy
	}

	return nil
}

// Get retrieves an incident by its unique identifier.
//
// Returns the complete incident including attached signals and events.
// Returns ErrNotFound if no incident exists with the given ID.
func (r *IncidentRepository) Get(ctx context.Context, id string) (*domain.Incident, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	if id == "" {
		return nil, ports.ErrInvalidInput
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	incident, exists := r.incidents[id]
	if !exists {
		return nil, ports.ErrNotFound
	}

	// Create a deep copy with events
	incidentCopy := r.copyIncident(incident)
	if events, hasEvents := r.events[id]; hasEvents {
		incidentCopy.Events = make([]domain.Event, len(events))
		for i, event := range events {
			incidentCopy.Events[i] = *r.copyEvent(event)
		}
	}

	return incidentCopy, nil
}

// List retrieves incidents based on the provided filter criteria.
//
// Returns a paginated list of incidents with metadata about the total count.
// The filter is validated before execution.
func (r *IncidentRepository) List(ctx context.Context, filter ports.ListFilter) (*ports.ListResult, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate filter
	if err := filter.Validate(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Collect all incidents that match the filter
	var matched []*domain.Incident
	for _, incident := range r.incidents {
		if r.matchesFilter(incident, filter) {
			matched = append(matched, r.copyIncident(incident))
		}
	}

	// Sort results
	r.sortIncidents(matched, filter.SortBy, filter.SortOrder)

	// Calculate pagination
	total := len(matched)
	start := filter.Offset
	end := start + filter.Limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	// Extract page
	var page []*domain.Incident
	if start < end {
		page = matched[start:end]
	}

	return &ports.ListResult{
		Incidents: page,
		Total:     total,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
		HasMore:   end < total,
	}, nil
}

// Update modifies an existing incident in the repository.
//
// The incident must exist and pass domain validation.
// Returns ErrNotFound if the incident doesn't exist.
func (r *IncidentRepository) Update(ctx context.Context, incident *domain.Incident) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if incident == nil {
		return ports.ErrInvalidInput
	}

	// Validate incident
	if err := incident.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if incident exists
	if _, exists := r.incidents[incident.ID]; !exists {
		return ports.ErrNotFound
	}

	// Update the incident
	r.incidents[incident.ID] = r.copyIncident(incident)

	// Update events if any
	if len(incident.Events) > 0 {
		eventsCopy := make([]*domain.Event, len(incident.Events))
		for i, event := range incident.Events {
			eventsCopy[i] = r.copyEvent(&event)
		}
		r.events[incident.ID] = eventsCopy
	}

	return nil
}

// AddEvent appends an event to an incident's history.
//
// The event must be valid and reference an existing incident.
// Events are append-only and cannot be modified once created.
func (r *IncidentRepository) AddEvent(ctx context.Context, event *domain.Event) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if event == nil {
		return ports.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if incident exists
	if _, exists := r.incidents[event.IncidentID]; !exists {
		return ports.ErrNotFound
	}

	// Generate ID if not set
	eventCopy := r.copyEvent(event)
	if eventCopy.ID == "" {
		eventCopy.ID = fmt.Sprintf("event-%d", r.nextID)
		r.nextID++
	}

	// Validate event after ID generation
	if err := eventCopy.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ports.ErrInvalidInput, err)
	}

	// Add event to incident's event list
	r.events[event.IncidentID] = append(r.events[event.IncidentID], eventCopy)

	return nil
}

// Delete removes an incident from the repository.
//
// This is a soft delete operation that marks the incident as deleted
// but preserves the data for audit purposes.
func (r *IncidentRepository) Delete(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	if id == "" {
		return ports.ErrInvalidInput
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if incident exists
	if _, exists := r.incidents[id]; !exists {
		return ports.ErrNotFound
	}

	// For in-memory implementation, we actually delete the data
	// In a real implementation, this would be a soft delete
	delete(r.incidents, id)
	delete(r.events, id)

	return nil
}

// GetWithEvents retrieves an incident with its complete event history.
//
// This is the same as Get() in the memory implementation since events
// are always loaded together.
func (r *IncidentRepository) GetWithEvents(ctx context.Context, id string) (*domain.Incident, error) {
	return r.Get(ctx, id)
}

// GetWithSignals retrieves an incident with its attached signals.
//
// This is the same as Get() in the memory implementation since signals
// are stored as part of the incident.
func (r *IncidentRepository) GetWithSignals(ctx context.Context, id string) (*domain.Incident, error) {
	return r.Get(ctx, id)
}

// Clear removes all data from the repository.
//
// This method is useful for testing to reset the repository state.
func (r *IncidentRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.incidents = make(map[string]*domain.Incident)
	r.events = make(map[string][]*domain.Event)
	r.nextID = 1
}

// Count returns the total number of incidents in the repository.
//
// This method is useful for testing and monitoring.
func (r *IncidentRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.incidents)
}

// copyIncident creates a deep copy of an incident to prevent external modifications.
func (r *IncidentRepository) copyIncident(incident *domain.Incident) *domain.Incident {
	copy := &domain.Incident{
		ID:          incident.ID,
		Title:       incident.Title,
		Description: incident.Description,
		Status:      incident.Status,
		Severity:    incident.Severity,
		TeamID:      incident.TeamID,
		AssigneeID:  incident.AssigneeID,
		ServiceID:   incident.ServiceID,
		CreatedAt:   incident.CreatedAt,
		UpdatedAt:   incident.UpdatedAt,
	}

	if incident.ResolvedAt != nil {
		resolvedAt := *incident.ResolvedAt
		copy.ResolvedAt = &resolvedAt
	}

	if incident.Labels != nil {
		copy.Labels = make(map[string]string)
		for k, v := range incident.Labels {
			copy.Labels[k] = v
		}
	}

	if incident.Signals != nil {
		copy.Signals = make([]domain.Signal, len(incident.Signals))
		for i, signal := range incident.Signals {
			copy.Signals[i] = r.copySignal(signal)
		}
	}

	return copy
}

// copySignal creates a deep copy of a signal.
func (r *IncidentRepository) copySignal(signal domain.Signal) domain.Signal {
	copy := domain.Signal{
		ID:          signal.ID,
		Source:      signal.Source,
		Title:       signal.Title,
		Description: signal.Description,
		Severity:    signal.Severity,
		ReceivedAt:  signal.ReceivedAt,
	}

	if signal.Labels != nil {
		copy.Labels = make(map[string]string)
		for k, v := range signal.Labels {
			copy.Labels[k] = v
		}
	}

	if signal.Annotations != nil {
		copy.Annotations = make(map[string]string)
		for k, v := range signal.Annotations {
			copy.Annotations[k] = v
		}
	}

	if signal.Payload != nil {
		copy.Payload = make(map[string]interface{})
		for k, v := range signal.Payload {
			copy.Payload[k] = v
		}
	}

	return copy
}

// copyEvent creates a deep copy of an event.
func (r *IncidentRepository) copyEvent(event *domain.Event) *domain.Event {
	copy := &domain.Event{
		ID:          event.ID,
		IncidentID:  event.IncidentID,
		Type:        event.Type,
		Actor:       event.Actor,
		Description: event.Description,
		OccurredAt:  event.OccurredAt,
	}

	if event.Metadata != nil {
		copy.Metadata = make(map[string]interface{})
		for k, v := range event.Metadata {
			copy.Metadata[k] = v
		}
	}

	return copy
}

// matchesFilter checks if an incident matches the given filter criteria.
func (r *IncidentRepository) matchesFilter(incident *domain.Incident, filter ports.ListFilter) bool {
	// Team filter
	if filter.TeamID != "" && incident.TeamID != filter.TeamID {
		return false
	}

	// Status filter
	if len(filter.Status) > 0 {
		found := false
		for _, status := range filter.Status {
			if incident.Status == status {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Severity filter
	if len(filter.Severity) > 0 {
		found := false
		for _, severity := range filter.Severity {
			if incident.Severity == severity {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Assignee filter
	if filter.AssigneeID != "" && incident.AssigneeID != filter.AssigneeID {
		return false
	}

	// Service filter
	if filter.ServiceID != "" && incident.ServiceID != filter.ServiceID {
		return false
	}

	// Labels filter (incident must have all specified labels)
	if len(filter.Labels) > 0 {
		if incident.Labels == nil {
			return false
		}
		for key, value := range filter.Labels {
			if incidentValue, exists := incident.Labels[key]; !exists || incidentValue != value {
				return false
			}
		}
	}

	// Time filters
	if filter.CreatedAfter != nil && incident.CreatedAt.Before(*filter.CreatedAfter) {
		return false
	}
	if filter.CreatedBefore != nil && incident.CreatedAt.After(*filter.CreatedBefore) {
		return false
	}
	if filter.UpdatedAfter != nil && incident.UpdatedAt.Before(*filter.UpdatedAfter) {
		return false
	}
	if filter.UpdatedBefore != nil && incident.UpdatedAt.After(*filter.UpdatedBefore) {
		return false
	}

	// Search filter (case-insensitive search in title and description)
	if filter.Search != "" {
		searchLower := strings.ToLower(filter.Search)
		titleMatch := strings.Contains(strings.ToLower(incident.Title), searchLower)
		descMatch := strings.Contains(strings.ToLower(incident.Description), searchLower)
		if !titleMatch && !descMatch {
			return false
		}
	}

	return true
}

// sortIncidents sorts the incidents slice according to the specified criteria.
func (r *IncidentRepository) sortIncidents(incidents []*domain.Incident, sortBy, sortOrder string) {
	sort.Slice(incidents, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "created_at":
			less = incidents[i].CreatedAt.Before(incidents[j].CreatedAt)
		case "updated_at":
			less = incidents[i].UpdatedAt.Before(incidents[j].UpdatedAt)
		case "severity":
			// Sort by severity priority: critical > high > medium > low > info
			severityOrder := map[domain.Severity]int{
				domain.SeverityCritical: 0,
				domain.SeverityHigh:     1,
				domain.SeverityMedium:   2,
				domain.SeverityLow:      3,
				domain.SeverityInfo:     4,
			}
			less = severityOrder[incidents[i].Severity] < severityOrder[incidents[j].Severity]
		case "status":
			// Sort by status: triggered > acknowledged > investigating > resolved
			statusOrder := map[domain.Status]int{
				domain.StatusTriggered:     0,
				domain.StatusAcknowledged:  1,
				domain.StatusInvestigating: 2,
				domain.StatusResolved:      3,
			}
			less = statusOrder[incidents[i].Status] < statusOrder[incidents[j].Status]
		default:
			// Default to created_at
			less = incidents[i].CreatedAt.Before(incidents[j].CreatedAt)
		}

		// Reverse for descending order
		if sortOrder == "desc" {
			return !less
		}
		return less
	})
}
