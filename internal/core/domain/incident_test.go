package domain

import (
	"testing"
	"time"
)

func TestStatus_String(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   string
	}{
		{"triggered", StatusTriggered, "triggered"},
		{"acknowledged", StatusAcknowledged, "acknowledged"},
		{"investigating", StatusInvestigating, "investigating"},
		{"resolved", StatusResolved, "resolved"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.String(); got != tt.want {
				t.Errorf("Status.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"valid triggered", StatusTriggered, true},
		{"valid acknowledged", StatusAcknowledged, true},
		{"valid investigating", StatusInvestigating, true},
		{"valid resolved", StatusResolved, true},
		{"invalid empty", Status(""), false},
		{"invalid random", Status("random"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.status.IsValid(); got != tt.want {
				t.Errorf("Status.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeverity_String(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     string
	}{
		{"critical", SeverityCritical, "critical"},
		{"high", SeverityHigh, "high"},
		{"medium", SeverityMedium, "medium"},
		{"low", SeverityLow, "low"},
		{"info", SeverityInfo, "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.severity.String(); got != tt.want {
				t.Errorf("Severity.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSeverity_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		severity Severity
		want     bool
	}{
		{"valid critical", SeverityCritical, true},
		{"valid high", SeverityHigh, true},
		{"valid medium", SeverityMedium, true},
		{"valid low", SeverityLow, true},
		{"valid info", SeverityInfo, true},
		{"invalid empty", Severity(""), false},
		{"invalid random", Severity("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.severity.IsValid(); got != tt.want {
				t.Errorf("Severity.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelType_String(t *testing.T) {
	tests := []struct {
		name    string
		channel ChannelType
		want    string
	}{
		{"email", ChannelTypeEmail, "email"},
		{"slack", ChannelTypeSlack, "slack"},
		{"sms", ChannelTypeSMS, "sms"},
		{"webhook", ChannelTypeWebhook, "webhook"},
		{"pagerduty", ChannelTypePagerDuty, "pagerduty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.channel.String(); got != tt.want {
				t.Errorf("ChannelType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChannelType_IsValid(t *testing.T) {
	tests := []struct {
		name    string
		channel ChannelType
		want    bool
	}{
		{"valid email", ChannelTypeEmail, true},
		{"valid slack", ChannelTypeSlack, true},
		{"valid sms", ChannelTypeSMS, true},
		{"valid webhook", ChannelTypeWebhook, true},
		{"valid pagerduty", ChannelTypePagerDuty, true},
		{"invalid empty", ChannelType(""), false},
		{"invalid random", ChannelType("teams"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.channel.IsValid(); got != tt.want {
				t.Errorf("ChannelType.IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSignal_Validate(t *testing.T) {
	validTime := time.Now().UTC()

	tests := []struct {
		name    string
		signal  Signal
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid signal",
			signal: Signal{
				ID:          "signal-1",
				Source:      "prometheus",
				Title:       "High CPU usage",
				Description: "CPU usage above 90%",
				Severity:    SeverityHigh,
				ReceivedAt:  validTime,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			signal: Signal{
				Source:     "prometheus",
				Title:      "High CPU usage",
				Severity:   SeverityHigh,
				ReceivedAt: validTime,
			},
			wantErr: true,
			errMsg:  "signal ID is required",
		},
		{
			name: "missing source",
			signal: Signal{
				ID:         "signal-1",
				Title:      "High CPU usage",
				Severity:   SeverityHigh,
				ReceivedAt: validTime,
			},
			wantErr: true,
			errMsg:  "signal source is required",
		},
		{
			name: "missing title",
			signal: Signal{
				ID:         "signal-1",
				Source:     "prometheus",
				Severity:   SeverityHigh,
				ReceivedAt: validTime,
			},
			wantErr: true,
			errMsg:  "signal title is required",
		},
		{
			name: "invalid severity",
			signal: Signal{
				ID:         "signal-1",
				Source:     "prometheus",
				Title:      "High CPU usage",
				Severity:   Severity("invalid"),
				ReceivedAt: validTime,
			},
			wantErr: true,
			errMsg:  "invalid severity: invalid",
		},
		{
			name: "missing received_at",
			signal: Signal{
				ID:       "signal-1",
				Source:   "prometheus",
				Title:    "High CPU usage",
				Severity: SeverityHigh,
			},
			wantErr: true,
			errMsg:  "signal received_at timestamp is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.signal.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Signal.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("Signal.Validate() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestEvent_Validate(t *testing.T) {
	validTime := time.Now().UTC()

	tests := []struct {
		name    string
		event   Event
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid event",
			event: Event{
				ID:          "event-1",
				IncidentID:  "incident-1",
				Type:        "status_changed",
				Actor:       "user@example.com",
				Description: "Status changed to acknowledged",
				OccurredAt:  validTime,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			event: Event{
				IncidentID: "incident-1",
				Type:       "status_changed",
				Actor:      "user@example.com",
				OccurredAt: validTime,
			},
			wantErr: true,
			errMsg:  "event ID is required",
		},
		{
			name: "missing incident_id",
			event: Event{
				ID:         "event-1",
				Type:       "status_changed",
				Actor:      "user@example.com",
				OccurredAt: validTime,
			},
			wantErr: true,
			errMsg:  "event incident_id is required",
		},
		{
			name: "missing type",
			event: Event{
				ID:         "event-1",
				IncidentID: "incident-1",
				Actor:      "user@example.com",
				OccurredAt: validTime,
			},
			wantErr: true,
			errMsg:  "event type is required",
		},
		{
			name: "missing actor",
			event: Event{
				ID:         "event-1",
				IncidentID: "incident-1",
				Type:       "status_changed",
				OccurredAt: validTime,
			},
			wantErr: true,
			errMsg:  "event actor is required",
		},
		{
			name: "missing occurred_at",
			event: Event{
				ID:         "event-1",
				IncidentID: "incident-1",
				Type:       "status_changed",
				Actor:      "user@example.com",
			},
			wantErr: true,
			errMsg:  "event occurred_at timestamp is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.event.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Event.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("Event.Validate() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestIncident_Validate(t *testing.T) {
	now := time.Now().UTC()
	resolved := now.Add(time.Hour)

	tests := []struct {
		name     string
		incident Incident
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid incident",
			incident: Incident{
				ID:          "incident-1",
				Title:       "Database connection issues",
				Description: "Unable to connect to primary database",
				Status:      StatusTriggered,
				Severity:    SeverityHigh,
				TeamID:      "team-1",
				CreatedAt:   now,
				UpdatedAt:   now,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			incident: Incident{
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident ID is required",
		},
		{
			name: "missing title",
			incident: Incident{
				ID:        "incident-1",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident title is required",
		},
		{
			name: "title too long",
			incident: Incident{
				ID:        "incident-1",
				Title:     "This is a very long title that exceeds the maximum allowed length of 200 characters and should cause validation to fail because it's way too long for a proper incident title that should be concise and to the point",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident title exceeds maximum length of 200 characters",
		},
		{
			name: "invalid status",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    Status("invalid"),
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "invalid status: invalid",
		},
		{
			name: "invalid severity",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  Severity("invalid"),
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "invalid severity: invalid",
		},
		{
			name: "missing team_id",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident team_id is required",
		},
		{
			name: "missing created_at",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident created_at timestamp is required",
		},
		{
			name: "missing updated_at",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
			},
			wantErr: true,
			errMsg:  "incident updated_at timestamp is required",
		},
		{
			name: "updated_at before created_at",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusTriggered,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now.Add(-time.Hour),
			},
			wantErr: true,
			errMsg:  "incident updated_at cannot be before created_at",
		},
		{
			name: "resolved without resolved_at",
			incident: Incident{
				ID:        "incident-1",
				Title:     "Database connection issues",
				Status:    StatusResolved,
				Severity:  SeverityHigh,
				TeamID:    "team-1",
				CreatedAt: now,
				UpdatedAt: now,
			},
			wantErr: true,
			errMsg:  "resolved incidents must have a resolved_at timestamp",
		},
		{
			name: "not resolved with resolved_at",
			incident: Incident{
				ID:         "incident-1",
				Title:      "Database connection issues",
				Status:     StatusTriggered,
				Severity:   SeverityHigh,
				TeamID:     "team-1",
				CreatedAt:  now,
				UpdatedAt:  now,
				ResolvedAt: &resolved,
			},
			wantErr: true,
			errMsg:  "only resolved incidents can have a resolved_at timestamp",
		},
		{
			name: "valid resolved incident",
			incident: Incident{
				ID:         "incident-1",
				Title:      "Database connection issues",
				Status:     StatusResolved,
				Severity:   SeverityHigh,
				TeamID:     "team-1",
				CreatedAt:  now,
				UpdatedAt:  now,
				ResolvedAt: &resolved,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.incident.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Incident.Validate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && err.Error() != tt.errMsg {
				t.Errorf("Incident.Validate() error = %v, want %v", err.Error(), tt.errMsg)
			}
		})
	}
}

func TestIncident_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name          string
		currentStatus Status
		targetStatus  Status
		want          bool
	}{
		// Same status transitions (no-op)
		{"triggered to triggered", StatusTriggered, StatusTriggered, true},
		{"acknowledged to acknowledged", StatusAcknowledged, StatusAcknowledged, true},
		{"investigating to investigating", StatusInvestigating, StatusInvestigating, true},
		{"resolved to resolved", StatusResolved, StatusResolved, true},

		// Valid forward transitions
		{"triggered to acknowledged", StatusTriggered, StatusAcknowledged, true},
		{"triggered to investigating", StatusTriggered, StatusInvestigating, true},
		{"triggered to resolved", StatusTriggered, StatusResolved, true},
		{"acknowledged to investigating", StatusAcknowledged, StatusInvestigating, true},
		{"acknowledged to resolved", StatusAcknowledged, StatusResolved, true},
		{"investigating to resolved", StatusInvestigating, StatusResolved, true},

		// Reopening (any status to triggered)
		{"acknowledged to triggered", StatusAcknowledged, StatusTriggered, true},
		{"investigating to triggered", StatusInvestigating, StatusTriggered, true},
		{"resolved to triggered", StatusResolved, StatusTriggered, true},

		// Invalid transitions
		{"investigating to acknowledged", StatusInvestigating, StatusAcknowledged, false},
		{"resolved to acknowledged", StatusResolved, StatusAcknowledged, false},
		{"resolved to investigating", StatusResolved, StatusInvestigating, false},

		// Invalid status
		{"triggered to invalid", StatusTriggered, Status("invalid"), false},

		// Invalid current status (testing default case)
		{"invalid to triggered", Status("invalid"), StatusTriggered, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			incident := &Incident{Status: tt.currentStatus}
			if got := incident.CanTransitionTo(tt.targetStatus); got != tt.want {
				t.Errorf("Incident.CanTransitionTo() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncident_AttachSignal(t *testing.T) {
	now := time.Now().UTC()

	validSignal := Signal{
		ID:         "signal-1",
		Source:     "prometheus",
		Title:      "High CPU usage",
		Severity:   SeverityHigh,
		ReceivedAt: now,
	}

	invalidSignal := Signal{
		ID:       "signal-2",
		Source:   "prometheus",
		Severity: SeverityHigh,
		// Missing required fields
	}

	incident := &Incident{
		ID:        "incident-1",
		Title:     "Test incident",
		Status:    StatusTriggered,
		Severity:  SeverityHigh,
		TeamID:    "team-1",
		CreatedAt: now,
		UpdatedAt: now,
	}

	t.Run("attach valid signal", func(t *testing.T) {
		err := incident.AttachSignal(validSignal)
		if err != nil {
			t.Errorf("Incident.AttachSignal() error = %v, want nil", err)
		}
		if len(incident.Signals) != 1 {
			t.Errorf("Expected 1 signal, got %d", len(incident.Signals))
		}
		if incident.Signals[0].ID != validSignal.ID {
			t.Errorf("Expected signal ID %s, got %s", validSignal.ID, incident.Signals[0].ID)
		}
	})

	t.Run("attach duplicate signal", func(t *testing.T) {
		initialCount := len(incident.Signals)
		err := incident.AttachSignal(validSignal)
		if err != nil {
			t.Errorf("Incident.AttachSignal() error = %v, want nil", err)
		}
		if len(incident.Signals) != initialCount {
			t.Errorf("Expected %d signals (no duplicate), got %d", initialCount, len(incident.Signals))
		}
	})

	t.Run("attach invalid signal", func(t *testing.T) {
		err := incident.AttachSignal(invalidSignal)
		if err == nil {
			t.Error("Incident.AttachSignal() error = nil, want error")
		}
		if !contains(err.Error(), "cannot attach invalid signal") {
			t.Errorf("Expected error to contain 'cannot attach invalid signal', got %v", err)
		}
	})
}

func TestIncident_RecordEvent(t *testing.T) {
	now := time.Now().UTC()

	incident := &Incident{
		ID:        "incident-1",
		Title:     "Test incident",
		Status:    StatusTriggered,
		Severity:  SeverityHigh,
		TeamID:    "team-1",
		CreatedAt: now,
		UpdatedAt: now,
	}

	validEvent := Event{
		ID:          "event-1",
		Type:        "comment_added",
		Actor:       "user@example.com",
		Description: "Added a comment",
		OccurredAt:  now,
	}

	invalidEvent := Event{
		ID:   "event-2",
		Type: "comment_added",
		// Missing required fields
	}

	t.Run("record valid event", func(t *testing.T) {
		err := incident.RecordEvent(validEvent)
		if err != nil {
			t.Errorf("Incident.RecordEvent() error = %v, want nil", err)
		}
		if len(incident.Events) != 1 {
			t.Errorf("Expected 1 event, got %d", len(incident.Events))
		}
		if incident.Events[0].IncidentID != incident.ID {
			t.Errorf("Expected event incident_id %s, got %s", incident.ID, incident.Events[0].IncidentID)
		}
	})

	t.Run("record invalid event", func(t *testing.T) {
		err := incident.RecordEvent(invalidEvent)
		if err == nil {
			t.Error("Incident.RecordEvent() error = nil, want error")
		}
		if !contains(err.Error(), "cannot record invalid event") {
			t.Errorf("Expected error to contain 'cannot record invalid event', got %v", err)
		}
	})
}

func TestIncident_TransitionTo(t *testing.T) {
	now := time.Now().UTC()

	createTestIncident := func() *Incident {
		return &Incident{
			ID:        "incident-1",
			Title:     "Test incident",
			Status:    StatusTriggered,
			Severity:  SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now,
			UpdatedAt: now,
		}
	}

	t.Run("valid transition", func(t *testing.T) {
		incident := createTestIncident()
		err := incident.TransitionTo(StatusAcknowledged, "user@example.com")
		if err != nil {
			t.Errorf("Incident.TransitionTo() error = %v, want nil", err)
		}
		if incident.Status != StatusAcknowledged {
			t.Errorf("Expected status %s, got %s", StatusAcknowledged, incident.Status)
		}
		if len(incident.Events) != 1 {
			t.Errorf("Expected 1 event recorded, got %d", len(incident.Events))
		}
		if incident.Events[0].Type != "status_changed" {
			t.Errorf("Expected event type 'status_changed', got %s", incident.Events[0].Type)
		}
	})

	t.Run("transition to resolved sets resolved_at", func(t *testing.T) {
		incident := createTestIncident()
		err := incident.TransitionTo(StatusResolved, "user@example.com")
		if err != nil {
			t.Errorf("Incident.TransitionTo() error = %v, want nil", err)
		}
		if incident.Status != StatusResolved {
			t.Errorf("Expected status %s, got %s", StatusResolved, incident.Status)
		}
		if incident.ResolvedAt == nil {
			t.Error("Expected resolved_at to be set")
		}
	})

	t.Run("transition away from resolved clears resolved_at", func(t *testing.T) {
		incident := createTestIncident()
		resolved := now
		incident.Status = StatusResolved
		incident.ResolvedAt = &resolved

		err := incident.TransitionTo(StatusTriggered, "user@example.com")
		if err != nil {
			t.Errorf("Incident.TransitionTo() error = %v, want nil", err)
		}
		if incident.Status != StatusTriggered {
			t.Errorf("Expected status %s, got %s", StatusTriggered, incident.Status)
		}
		if incident.ResolvedAt != nil {
			t.Error("Expected resolved_at to be cleared")
		}
	})

	t.Run("invalid transition", func(t *testing.T) {
		incident := createTestIncident()
		incident.Status = StatusInvestigating

		err := incident.TransitionTo(StatusAcknowledged, "user@example.com")
		if err == nil {
			t.Error("Incident.TransitionTo() error = nil, want error")
		}
		if !contains(err.Error(), "cannot transition from investigating to acknowledged") {
			t.Errorf("Expected error about invalid transition, got %v", err)
		}
	})
}

func TestIncident_ValidateWithSignalsAndEvents(t *testing.T) {
	now := time.Now().UTC()

	validSignal := Signal{
		ID:         "signal-1",
		Source:     "prometheus",
		Title:      "High CPU usage",
		Severity:   SeverityHigh,
		ReceivedAt: now,
	}

	invalidSignal := Signal{
		ID:       "signal-2",
		Source:   "prometheus",
		Severity: SeverityHigh,
		// Missing title and received_at
	}

	validEvent := Event{
		ID:          "event-1",
		IncidentID:  "incident-1",
		Type:        "status_changed",
		Actor:       "user@example.com",
		Description: "Status changed",
		OccurredAt:  now,
	}

	invalidEvent := Event{
		ID:         "event-2",
		IncidentID: "incident-1",
		Type:       "comment_added",
		// Missing actor and occurred_at
	}

	mismatchedEvent := Event{
		ID:          "event-3",
		IncidentID:  "different-incident",
		Type:        "status_changed",
		Actor:       "user@example.com",
		Description: "Status changed",
		OccurredAt:  now,
	}

	t.Run("valid incident with signals and events", func(t *testing.T) {
		incident := &Incident{
			ID:        "incident-1",
			Title:     "Test incident",
			Status:    StatusTriggered,
			Severity:  SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now,
			UpdatedAt: now,
			Signals:   []Signal{validSignal},
			Events:    []Event{validEvent},
		}

		err := incident.Validate()
		if err != nil {
			t.Errorf("Incident.Validate() error = %v, want nil", err)
		}
	})

	t.Run("incident with invalid signal", func(t *testing.T) {
		incident := &Incident{
			ID:        "incident-1",
			Title:     "Test incident",
			Status:    StatusTriggered,
			Severity:  SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now,
			UpdatedAt: now,
			Signals:   []Signal{invalidSignal},
		}

		err := incident.Validate()
		if err == nil {
			t.Error("Incident.Validate() error = nil, want error")
		}
		if !contains(err.Error(), "invalid signal at index 0") {
			t.Errorf("Expected error about invalid signal, got %v", err)
		}
	})

	t.Run("incident with invalid event", func(t *testing.T) {
		incident := &Incident{
			ID:        "incident-1",
			Title:     "Test incident",
			Status:    StatusTriggered,
			Severity:  SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now,
			UpdatedAt: now,
			Events:    []Event{invalidEvent},
		}

		err := incident.Validate()
		if err == nil {
			t.Error("Incident.Validate() error = nil, want error")
		}
		if !contains(err.Error(), "invalid event at index 0") {
			t.Errorf("Expected error about invalid event, got %v", err)
		}
	})

	t.Run("incident with mismatched event incident_id", func(t *testing.T) {
		incident := &Incident{
			ID:        "incident-1",
			Title:     "Test incident",
			Status:    StatusTriggered,
			Severity:  SeverityHigh,
			TeamID:    "team-1",
			CreatedAt: now,
			UpdatedAt: now,
			Events:    []Event{mismatchedEvent},
		}

		err := incident.Validate()
		if err == nil {
			t.Error("Incident.Validate() error = nil, want error")
		}
		if !contains(err.Error(), "event at index 0 has mismatched incident_id") {
			t.Errorf("Expected error about mismatched incident_id, got %v", err)
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
