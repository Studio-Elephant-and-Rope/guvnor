package dto

import (
	"strings"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
)

func TestCreateIncidentRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request CreateIncidentRequest
		wantErr bool
	}{
		{
			name: "valid request",
			request: CreateIncidentRequest{
				Title:    "Database connection timeout",
				Severity: "critical",
				TeamID:   "team-backend",
			},
			wantErr: false,
		},
		{
			name: "missing title",
			request: CreateIncidentRequest{
				Severity: "critical",
				TeamID:   "team-backend",
			},
			wantErr: true,
		},
		{
			name: "empty title",
			request: CreateIncidentRequest{
				Title:    "",
				Severity: "critical",
				TeamID:   "team-backend",
			},
			wantErr: true,
		},
		{
			name: "title too long",
			request: CreateIncidentRequest{
				Title:    strings.Repeat("x", 201), // Exceeds 200 char limit
				Severity: "critical",
				TeamID:   "team-backend",
			},
			wantErr: true,
		},
		{
			name: "missing severity",
			request: CreateIncidentRequest{
				Title:  "Database issue",
				TeamID: "team-backend",
			},
			wantErr: true,
		},
		{
			name: "invalid severity",
			request: CreateIncidentRequest{
				Title:    "Database issue",
				Severity: "invalid",
				TeamID:   "team-backend",
			},
			wantErr: true,
		},
		{
			name: "missing team_id",
			request: CreateIncidentRequest{
				Title:    "Database issue",
				Severity: "critical",
			},
			wantErr: true,
		},
		{
			name: "empty team_id",
			request: CreateIncidentRequest{
				Title:    "Database issue",
				Severity: "critical",
				TeamID:   "",
			},
			wantErr: true,
		},
		{
			name: "valid with optional fields",
			request: CreateIncidentRequest{
				Title:       "API performance issue",
				Description: "API response times are elevated",
				Severity:    "high",
				TeamID:      "team-api",
				ServiceID:   "api-service",
				Labels:      map[string]string{"env": "production"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestListIncidentsRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request ListIncidentsRequest
		wantErr bool
	}{
		{
			name:    "valid empty request",
			request: ListIncidentsRequest{},
			wantErr: false,
		},
		{
			name: "valid with all fields",
			request: ListIncidentsRequest{
				TeamID:     "team-1",
				Status:     "triggered,acknowledged",
				Severity:   "critical,high",
				AssigneeID: "user-1",
				ServiceID:  "service-1",
				Search:     "database",
				Limit:      25,
				Offset:     0,
				SortBy:     "created_at",
				SortOrder:  "desc",
			},
			wantErr: false,
		},
		{
			name: "negative limit gets corrected",
			request: ListIncidentsRequest{
				Limit: -1,
			},
			wantErr: false, // It gets corrected to default, no error
		},
		{
			name: "limit too high",
			request: ListIncidentsRequest{
				Limit: 1001,
			},
			wantErr: true,
		},
		{
			name: "negative offset gets corrected",
			request: ListIncidentsRequest{
				Offset: -1,
			},
			wantErr: false, // It gets corrected to default, no error
		},
		{
			name: "invalid sort by",
			request: ListIncidentsRequest{
				SortBy: "invalid_field",
			},
			wantErr: true,
		},
		{
			name: "invalid sort order",
			request: ListIncidentsRequest{
				SortOrder: "invalid_order",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpdateStatusRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request UpdateStatusRequest
		wantErr bool
	}{
		{
			name: "valid status",
			request: UpdateStatusRequest{
				Status: "acknowledged",
			},
			wantErr: false,
		},
		{
			name: "valid with optional fields",
			request: UpdateStatusRequest{
				Status:  "investigating",
				Actor:   "user-123",
				Comment: "Looking into the issue",
			},
			wantErr: false,
		},
		{
			name: "missing status",
			request: UpdateStatusRequest{
				Actor: "user-123",
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			request: UpdateStatusRequest{
				Status: "invalid_status",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAddEventRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		request AddEventRequest
		wantErr bool
	}{
		{
			name: "valid event",
			request: AddEventRequest{
				Type:        "escalation",
				Description: "Escalated to senior engineer",
			},
			wantErr: false,
		},
		{
			name: "valid with optional fields",
			request: AddEventRequest{
				Type:        "comment",
				Actor:       "user-123",
				Description: "Added debugging information",
				Metadata:    map[string]interface{}{"debug_level": "verbose"},
			},
			wantErr: false,
		},
		{
			name: "missing type",
			request: AddEventRequest{
				Description: "Some event",
			},
			wantErr: true,
		},
		{
			name: "missing description",
			request: AddEventRequest{
				Type: "comment",
			},
			wantErr: true,
		},
		{
			name: "empty type",
			request: AddEventRequest{
				Type:        "",
				Description: "Some event",
			},
			wantErr: true,
		},
		{
			name: "empty description",
			request: AddEventRequest{
				Type:        "comment",
				Description: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCreateIncidentRequest_ToIncident(t *testing.T) {
	now := time.Now().UTC()

	request := CreateIncidentRequest{
		Title:       "Database connection timeout",
		Description: "Users are experiencing timeouts",
		Severity:    "critical",
		TeamID:      "team-backend",
		ServiceID:   "database-service",
		Labels:      map[string]string{"env": "production", "component": "database"},
	}

	incident, err := request.ToIncident()
	if err != nil {
		t.Fatalf("ToIncident() error = %v", err)
	}

	if incident.Title != request.Title {
		t.Errorf("Expected title %s, got %s", request.Title, incident.Title)
	}

	if incident.Description != request.Description {
		t.Errorf("Expected description %s, got %s", request.Description, incident.Description)
	}

	if incident.Severity.String() != request.Severity {
		t.Errorf("Expected severity %s, got %s", request.Severity, incident.Severity.String())
	}

	if incident.TeamID != request.TeamID {
		t.Errorf("Expected team ID %s, got %s", request.TeamID, incident.TeamID)
	}

	if incident.ServiceID != request.ServiceID {
		t.Errorf("Expected service ID %s, got %s", request.ServiceID, incident.ServiceID)
	}

	if len(incident.Labels) != len(request.Labels) {
		t.Errorf("Expected %d labels, got %d", len(request.Labels), len(incident.Labels))
	}

	for key, value := range request.Labels {
		if incident.Labels[key] != value {
			t.Errorf("Expected label %s = %s, got %s", key, value, incident.Labels[key])
		}
	}

	if incident.Status != domain.StatusTriggered {
		t.Errorf("Expected default status %s, got %s", domain.StatusTriggered, incident.Status)
	}

	if incident.CreatedAt.Before(now) || incident.CreatedAt.After(time.Now().UTC()) {
		t.Error("Created timestamp should be around current time")
	}

	if incident.UpdatedAt.Before(now) || incident.UpdatedAt.After(time.Now().UTC()) {
		t.Error("Updated timestamp should be around current time")
	}

	// Test with invalid severity
	invalidRequest := request
	invalidRequest.Severity = "invalid"
	_, err = invalidRequest.ToIncident()
	if err == nil {
		t.Error("Expected error with invalid severity")
	}
}

func TestFromIncident(t *testing.T) {
	now := time.Now().UTC()
	resolvedAt := now.Add(1 * time.Hour)

	incident := &domain.Incident{
		ID:          "incident-123",
		Title:       "Database issue",
		Description: "Connection timeouts",
		Status:      domain.StatusResolved,
		Severity:    domain.SeverityCritical,
		TeamID:      "team-backend",
		AssigneeID:  "user-123",
		ServiceID:   "database-service",
		Labels:      map[string]string{"env": "production"},
		CreatedAt:   now,
		UpdatedAt:   now.Add(30 * time.Minute),
		ResolvedAt:  &resolvedAt,
		Signals: []domain.Signal{
			{
				ID:         "signal-1",
				Source:     "prometheus",
				Title:      "High CPU",
				Severity:   domain.SeverityHigh,
				ReceivedAt: now,
			},
		},
		Events: []domain.Event{
			{
				ID:          "event-1",
				IncidentID:  "incident-123",
				Type:        "created",
				Actor:       "system",
				Description: "Incident created",
				OccurredAt:  now,
			},
		},
	}

	response := FromIncident(incident)

	if response.ID != incident.ID {
		t.Errorf("Expected ID %s, got %s", incident.ID, response.ID)
	}

	if response.Title != incident.Title {
		t.Errorf("Expected title %s, got %s", incident.Title, response.Title)
	}

	if response.Status != incident.Status.String() {
		t.Errorf("Expected status %s, got %s", incident.Status.String(), response.Status)
	}

	if response.Severity != incident.Severity.String() {
		t.Errorf("Expected severity %s, got %s", incident.Severity.String(), response.Severity)
	}

	if response.ResolvedAt == nil {
		t.Error("Expected resolved_at to be set")
	} else if !response.ResolvedAt.Equal(*incident.ResolvedAt) {
		t.Errorf("Expected resolved_at %v, got %v", *incident.ResolvedAt, *response.ResolvedAt)
	}

	if len(response.Signals) != len(incident.Signals) {
		t.Errorf("Expected %d signals, got %d", len(incident.Signals), len(response.Signals))
	}

	if len(response.Events) != len(incident.Events) {
		t.Errorf("Expected %d events, got %d", len(incident.Events), len(response.Events))
	}

	// Test with nil resolved_at
	incidentNoResolved := *incident
	incidentNoResolved.ResolvedAt = nil
	responseNoResolved := FromIncident(&incidentNoResolved)
	if responseNoResolved.ResolvedAt != nil {
		t.Error("Expected resolved_at to be nil")
	}
}

func TestFromSignal(t *testing.T) {
	signal := domain.Signal{
		ID:          "signal-123",
		Source:      "prometheus",
		Title:       "High CPU usage",
		Description: "CPU usage exceeded 90%",
		Severity:    domain.SeverityCritical,
		Labels:      map[string]string{"host": "web-01"},
		Annotations: map[string]string{"runbook": "http://example.com"},
		Payload:     map[string]interface{}{"value": 95.5},
		ReceivedAt:  time.Now().UTC(),
	}

	response := FromSignal(signal)

	if response.ID != signal.ID {
		t.Errorf("Expected ID %s, got %s", signal.ID, response.ID)
	}

	if response.Source != signal.Source {
		t.Errorf("Expected source %s, got %s", signal.Source, response.Source)
	}

	if response.Title != signal.Title {
		t.Errorf("Expected title %s, got %s", signal.Title, response.Title)
	}

	if response.Severity != signal.Severity.String() {
		t.Errorf("Expected severity %s, got %s", signal.Severity.String(), response.Severity)
	}

	if len(response.Labels) != len(signal.Labels) {
		t.Errorf("Expected %d labels, got %d", len(signal.Labels), len(response.Labels))
	}
}

func TestFromEvent(t *testing.T) {
	event := domain.Event{
		ID:          "event-123",
		IncidentID:  "incident-123",
		Type:        "status_changed",
		Actor:       "user-123",
		Description: "Status changed to acknowledged",
		Metadata:    map[string]interface{}{"old_status": "triggered"},
		OccurredAt:  time.Now().UTC(),
	}

	response := FromEvent(event)

	if response.ID != event.ID {
		t.Errorf("Expected ID %s, got %s", event.ID, response.ID)
	}

	if response.IncidentID != event.IncidentID {
		t.Errorf("Expected incident ID %s, got %s", event.IncidentID, response.IncidentID)
	}

	if response.Type != event.Type {
		t.Errorf("Expected type %s, got %s", event.Type, response.Type)
	}

	if response.Actor != event.Actor {
		t.Errorf("Expected actor %s, got %s", event.Actor, response.Actor)
	}

	if len(response.Metadata) != len(event.Metadata) {
		t.Errorf("Expected %d metadata items, got %d", len(event.Metadata), len(response.Metadata))
	}
}

func TestAddEventRequest_ToEvent(t *testing.T) {
	incidentID := "incident-123"
	request := AddEventRequest{
		Type:        "escalation",
		Actor:       "user-123",
		Description: "Escalated to senior engineer",
		Metadata:    map[string]interface{}{"escalation_reason": "expertise_required"},
	}

	event := request.ToEvent(incidentID)

	if event.IncidentID != incidentID {
		t.Errorf("Expected incident ID %s, got %s", incidentID, event.IncidentID)
	}

	if event.Type != request.Type {
		t.Errorf("Expected type %s, got %s", request.Type, event.Type)
	}

	if event.Actor != request.Actor {
		t.Errorf("Expected actor %s, got %s", request.Actor, event.Actor)
	}

	if event.Description != request.Description {
		t.Errorf("Expected description %s, got %s", request.Description, event.Description)
	}

	if len(event.Metadata) != len(request.Metadata) {
		t.Errorf("Expected %d metadata items, got %d", len(request.Metadata), len(event.Metadata))
	}

	if event.OccurredAt.IsZero() {
		t.Error("Expected OccurredAt to be set to current time")
	}

	// Test with empty actor
	requestNoActor := request
	requestNoActor.Actor = ""
	eventNoActor := requestNoActor.ToEvent(incidentID)
	if eventNoActor.Actor != "" {
		t.Errorf("Expected empty actor, got %s", eventNoActor.Actor)
	}
}

func TestErrorResponse(t *testing.T) {
	// Test that ErrorResponse struct can be created and used
	errorResp := ErrorResponse{
		Error:     "Validation failed",
		Code:      "VALIDATION_ERROR",
		RequestID: "req-123",
		Details:   map[string]interface{}{"field": "title"},
	}

	if errorResp.Error != "Validation failed" {
		t.Errorf("Expected error message 'Validation failed', got %s", errorResp.Error)
	}

	if errorResp.Code != "VALIDATION_ERROR" {
		t.Errorf("Expected error code 'VALIDATION_ERROR', got %s", errorResp.Code)
	}

	if errorResp.RequestID != "req-123" {
		t.Errorf("Expected request ID 'req-123', got %s", errorResp.RequestID)
	}
}

func TestResponseStructures(t *testing.T) {
	// Test that response structures can be created
	incidentResp := IncidentResponse{
		ID:       "incident-123",
		Title:    "Test incident",
		Status:   "triggered",
		Severity: "critical",
	}

	if incidentResp.ID != "incident-123" {
		t.Errorf("Expected ID 'incident-123', got %s", incidentResp.ID)
	}

	listResp := ListIncidentsResponse{
		Incidents: []IncidentResponse{incidentResp},
		Total:     1,
		Limit:     50,
		Offset:    0,
		HasMore:   false,
	}

	if len(listResp.Incidents) != 1 {
		t.Errorf("Expected 1 incident, got %d", len(listResp.Incidents))
	}

	if listResp.Total != 1 {
		t.Errorf("Expected total 1, got %d", listResp.Total)
	}
}
