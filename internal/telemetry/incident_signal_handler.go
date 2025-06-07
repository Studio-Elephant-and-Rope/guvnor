// Package telemetry provides signal processing and incident integration.
package telemetry

import (
	"context"
	"fmt"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/services"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// IncidentSignalHandler implements SignalHandler by creating incidents from telemetry signals.
//
// This handler bridges the gap between telemetry signal processing and Guvnor's
// incident management system. It receives processed signals from the telemetry
// receiver and converts them into incidents using the incident service.
type IncidentSignalHandler struct {
	incidentService *services.IncidentService
	logger          *logging.Logger
	config          SignalHandlerConfig
}

// SignalHandlerConfig configures how signals are converted to incidents.
type SignalHandlerConfig struct {
	// DefaultTeamID is the team to assign incidents when the signal doesn't specify one
	DefaultTeamID string `yaml:"default_team_id" json:"default_team_id"`
	// SeverityMapping maps signal severity to incident severity
	SeverityMapping map[domain.Severity]domain.Severity `yaml:"severity_mapping" json:"severity_mapping"`
	// ServiceLabelKey is the label key used to identify the service from signal labels
	ServiceLabelKey string `yaml:"service_label_key" json:"service_label_key"`
	// TeamLabelKey is the label key used to identify the team from signal labels
	TeamLabelKey string `yaml:"team_label_key" json:"team_label_key"`
	// CreateIncidents determines whether to actually create incidents or just log
	CreateIncidents bool `yaml:"create_incidents" json:"create_incidents"`
}

// DefaultSignalHandlerConfig returns sensible defaults for signal handling.
func DefaultSignalHandlerConfig() SignalHandlerConfig {
	return SignalHandlerConfig{
		DefaultTeamID: "platform-team",
		SeverityMapping: map[domain.Severity]domain.Severity{
			domain.SeverityInfo:     domain.SeverityLow,
			domain.SeverityLow:      domain.SeverityLow,
			domain.SeverityMedium:   domain.SeverityMedium,
			domain.SeverityHigh:     domain.SeverityHigh,
			domain.SeverityCritical: domain.SeverityCritical,
		},
		ServiceLabelKey: "service.name",
		TeamLabelKey:    "team",
		CreateIncidents: true,
	}
}

// NewIncidentSignalHandler creates a new signal handler that creates incidents.
//
// The handler requires an incident service for creating incidents, a logger for
// observability, and configuration for controlling signal-to-incident conversion.
//
// Returns an error if required dependencies are nil.
func NewIncidentSignalHandler(
	incidentService *services.IncidentService,
	logger *logging.Logger,
	config SignalHandlerConfig,
) (*IncidentSignalHandler, error) {
	if incidentService == nil {
		return nil, fmt.Errorf("incident service is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &IncidentSignalHandler{
		incidentService: incidentService,
		logger:          logger,
		config:          config,
	}, nil
}

// HandleSignal processes a telemetry signal and potentially creates an incident.
//
// The signal is converted to an incident using the configured mapping rules.
// If CreateIncidents is false, the signal is only logged and no incident is created.
//
// The incident creation process:
//  1. Extract team ID from signal labels or use default
//  2. Map signal severity to incident severity
//  3. Create incident with signal metadata
//  4. Attach the original signal to the incident
//
// Returns an error if incident creation fails.
func (h *IncidentSignalHandler) HandleSignal(ctx context.Context, signal domain.Signal) error {
	// Log the signal processing
	h.logger.Info("Processing telemetry signal",
		"signal_id", signal.ID,
		"signal_source", signal.Source,
		"signal_severity", signal.Severity,
		"signal_title", signal.Title,
		"create_incidents", h.config.CreateIncidents,
	)

	// If incident creation is disabled, just log and return
	if !h.config.CreateIncidents {
		h.logger.Info("Incident creation disabled, signal logged only",
			"signal_id", signal.ID,
		)
		return nil
	}

	// Extract team ID from signal labels
	teamID := h.extractTeamID(signal)

	// Map signal severity to incident severity
	incidentSeverity := h.mapSeverity(signal.Severity)

	// Extract service information
	serviceID := h.extractServiceID(signal)

	// Create incident title and description
	title := h.createIncidentTitle(signal)
	description := h.createIncidentDescription(signal)

	// Create the incident
	incident, err := h.incidentService.CreateIncident(ctx, title, incidentSeverity.String(), teamID)
	if err != nil {
		h.logger.Error("Failed to create incident from signal",
			"signal_id", signal.ID,
			"signal_source", signal.Source,
			"team_id", teamID,
			"severity", incidentSeverity,
			"error", err,
		)
		return fmt.Errorf("failed to create incident from signal %s: %w", signal.ID, err)
	}

	// Update incident with additional metadata
	updatedIncident := false

	if serviceID != "" {
		incident.ServiceID = serviceID
		updatedIncident = true
	}

	// Set incident description
	incident.Description = description
	updatedIncident = true

	// Copy labels from signal to incident
	if incident.Labels == nil {
		incident.Labels = make(map[string]string)
	}
	for key, value := range signal.Labels {
		incident.Labels[key] = value
		updatedIncident = true
	}

	// Add telemetry metadata
	incident.Labels["telemetry.signal_id"] = signal.ID
	incident.Labels["telemetry.source"] = signal.Source
	incident.Labels["telemetry.received_at"] = signal.ReceivedAt.Format("2006-01-02T15:04:05Z")

	// Add common service and environment labels if available
	if serviceID != "" {
		incident.Labels["service.name"] = serviceID
	}
	if teamID != "" {
		incident.Labels["team"] = teamID
	}
	if env, exists := signal.Labels["environment"]; exists {
		incident.Labels["environment"] = env
	}

	// Update the timestamp
	incident.UpdatedAt = time.Now().UTC()

	// TODO: Add UpdateIncident method to incident service to properly persist these changes
	// For now, the labels and service ID will be in memory but not persisted
	// This is a known limitation that needs to be addressed
	if updatedIncident {
		h.logger.Debug("Incident metadata updated (in memory only)",
			"incident_id", incident.ID,
			"signal_id", signal.ID,
			"service_id", serviceID,
			"labels_count", len(incident.Labels),
		)
	}

	// Record the signal attachment as an event
	event := domain.Event{
		Type:        "signal_attached",
		Actor:       "telemetry-receiver",
		Description: fmt.Sprintf("Telemetry signal attached from %s", signal.Source),
		Metadata: map[string]interface{}{
			"signal_id":     signal.ID,
			"signal_source": signal.Source,
			"signal_title":  signal.Title,
		},
	}

	if err := h.incidentService.RecordEvent(ctx, incident.ID, event); err != nil {
		h.logger.Error("Failed to record signal attachment event",
			"incident_id", incident.ID,
			"signal_id", signal.ID,
			"error", err,
		)
		// Don't return error here as the incident was created successfully
	}

	h.logger.Info("Successfully created incident from telemetry signal",
		"incident_id", incident.ID,
		"signal_id", signal.ID,
		"signal_source", signal.Source,
		"team_id", teamID,
		"severity", incidentSeverity,
		"service_id", serviceID,
	)

	return nil
}

// extractTeamID extracts the team ID from signal labels or returns the default.
func (h *IncidentSignalHandler) extractTeamID(signal domain.Signal) string {
	if teamID, exists := signal.Labels[h.config.TeamLabelKey]; exists && teamID != "" {
		return teamID
	}

	// Try common team label variations
	commonTeamKeys := []string{"team", "owner", "responsible_team", "squad"}
	for _, key := range commonTeamKeys {
		if teamID, exists := signal.Labels[key]; exists && teamID != "" {
			return teamID
		}
	}

	return h.config.DefaultTeamID
}

// extractServiceID extracts the service ID from signal labels.
func (h *IncidentSignalHandler) extractServiceID(signal domain.Signal) string {
	if serviceID, exists := signal.Labels[h.config.ServiceLabelKey]; exists {
		return serviceID
	}

	// Try common service label variations
	commonServiceKeys := []string{"service", "app", "application", "component"}
	for _, key := range commonServiceKeys {
		if serviceID, exists := signal.Labels[key]; exists && serviceID != "" {
			return serviceID
		}
	}

	return ""
}

// mapSeverity maps a signal severity to an incident severity using the configured mapping.
func (h *IncidentSignalHandler) mapSeverity(signalSeverity domain.Severity) domain.Severity {
	if mapped, exists := h.config.SeverityMapping[signalSeverity]; exists {
		return mapped
	}

	// Default mapping if not configured
	switch signalSeverity {
	case domain.SeverityInfo:
		return domain.SeverityLow
	case domain.SeverityLow:
		return domain.SeverityLow
	case domain.SeverityMedium:
		return domain.SeverityMedium
	case domain.SeverityHigh:
		return domain.SeverityHigh
	case domain.SeverityCritical:
		return domain.SeverityCritical
	default:
		return domain.SeverityMedium // Safe default
	}
}

// createIncidentTitle creates a descriptive title for the incident from the signal.
func (h *IncidentSignalHandler) createIncidentTitle(signal domain.Signal) string {
	// If signal has a specific title, use it
	if signal.Title != "" {
		return signal.Title
	}

	// Create a generic title based on signal metadata
	source := signal.Source
	if source == "" {
		source = "telemetry"
	}

	severity := signal.Severity.String()

	return fmt.Sprintf("%s alert detected from %s", severity, source)
}

// createIncidentDescription creates a detailed description for the incident from the signal.
func (h *IncidentSignalHandler) createIncidentDescription(signal domain.Signal) string {
	description := signal.Description

	// If signal doesn't have a description, create one
	if description == "" {
		description = fmt.Sprintf("Incident automatically created from %s telemetry signal", signal.Source)
	}

	// Add metadata if available
	if len(signal.Labels) > 0 {
		description += "\n\nSignal Labels:\n"
		for key, value := range signal.Labels {
			description += fmt.Sprintf("- %s: %s\n", key, value)
		}
	}

	if len(signal.Annotations) > 0 {
		description += "\nSignal Annotations:\n"
		for key, value := range signal.Annotations {
			description += fmt.Sprintf("- %s: %s\n", key, value)
		}
	}

	// Add signal metadata
	description += fmt.Sprintf("\nSignal Details:\n")
	description += fmt.Sprintf("- Signal ID: %s\n", signal.ID)
	description += fmt.Sprintf("- Source: %s\n", signal.Source)
	description += fmt.Sprintf("- Received At: %s\n", signal.ReceivedAt.Format("2006-01-02 15:04:05 UTC"))

	return description
}
