// Package handlers provides HTTP handlers for the Guvnor incident management API.
//
// This package contains REST API handlers that provide a thin layer between
// HTTP requests and business logic services. Handlers focus on HTTP concerns
// like parsing requests, validation, and response formatting while delegating
// business logic to service layer components.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/api/dto"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/services"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/middleware"
)

// IncidentHandler handles HTTP requests for incident management operations.
//
// This handler provides REST API endpoints for creating, retrieving, listing,
// and managing incidents. It acts as a thin layer between HTTP and business logic,
// delegating operations to the incident service while handling HTTP-specific
// concerns like request parsing and response formatting.
type IncidentHandler struct {
	service *services.IncidentService
	logger  *logging.Logger
}

// NewIncidentHandler creates a new incident handler with the provided service and logger.
//
// Returns an error if either service or logger is nil.
func NewIncidentHandler(service *services.IncidentService, logger *logging.Logger) (*IncidentHandler, error) {
	if service == nil {
		return nil, fmt.Errorf("incident service cannot be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	return &IncidentHandler{
		service: service,
		logger:  logger,
	}, nil
}

// RegisterRoutes registers all incident-related routes with the provided HTTP mux.
//
// Registered routes:
//   - POST   /api/v1/incidents          - Create incident
//   - GET    /api/v1/incidents/{id}     - Get incident
//   - GET    /api/v1/incidents          - List incidents
//   - PATCH  /api/v1/incidents/{id}/status - Update status
//   - POST   /api/v1/incidents/{id}/events - Add event
func (h *IncidentHandler) RegisterRoutes(mux *http.ServeMux) {
	// @Summary Create a new incident
	// @Description Creates a new incident with the provided details
	// @Tags incidents
	// @Accept json
	// @Produce json
	// @Param incident body dto.CreateIncidentRequest true "Incident details"
	// @Success 201 {object} dto.IncidentResponse "Created incident"
	// @Failure 400 {object} dto.ErrorResponse "Invalid request"
	// @Failure 500 {object} dto.ErrorResponse "Internal server error"
	// @Router /api/v1/incidents [post]
	mux.HandleFunc("POST /api/v1/incidents", h.CreateIncident)

	// @Summary Get an incident by ID
	// @Description Retrieves a specific incident including its events and signals
	// @Tags incidents
	// @Produce json
	// @Param id path string true "Incident ID"
	// @Success 200 {object} dto.IncidentResponse "Incident details"
	// @Failure 404 {object} dto.ErrorResponse "Incident not found"
	// @Failure 500 {object} dto.ErrorResponse "Internal server error"
	// @Router /api/v1/incidents/{id} [get]
	mux.HandleFunc("GET /api/v1/incidents/{id}", h.GetIncident)

	// @Summary List incidents
	// @Description Retrieves a paginated list of incidents with optional filtering
	// @Tags incidents
	// @Produce json
	// @Param team_id query string false "Filter by team ID"
	// @Param status query string false "Filter by status (comma-separated)"
	// @Param severity query string false "Filter by severity (comma-separated)"
	// @Param assignee_id query string false "Filter by assignee ID"
	// @Param service_id query string false "Filter by service ID"
	// @Param search query string false "Text search in title and description"
	// @Param limit query int false "Maximum number of results (1-1000)" default(50)
	// @Param offset query int false "Number of results to skip" default(0)
	// @Param sort_by query string false "Field to sort by" default(created_at) Enums(created_at,updated_at,severity,status)
	// @Param sort_order query string false "Sort direction" default(desc) Enums(asc,desc)
	// @Success 200 {object} dto.ListIncidentsResponse "List of incidents"
	// @Failure 400 {object} dto.ErrorResponse "Invalid request parameters"
	// @Failure 500 {object} dto.ErrorResponse "Internal server error"
	// @Router /api/v1/incidents [get]
	mux.HandleFunc("GET /api/v1/incidents", h.ListIncidents)

	// @Summary Update incident status
	// @Description Updates the status of an existing incident
	// @Tags incidents
	// @Accept json
	// @Produce json
	// @Param id path string true "Incident ID"
	// @Param status body dto.UpdateStatusRequest true "New status"
	// @Success 200 {object} dto.IncidentResponse "Updated incident"
	// @Failure 400 {object} dto.ErrorResponse "Invalid request"
	// @Failure 404 {object} dto.ErrorResponse "Incident not found"
	// @Failure 409 {object} dto.ErrorResponse "Conflict - incident was modified"
	// @Failure 500 {object} dto.ErrorResponse "Internal server error"
	// @Router /api/v1/incidents/{id}/status [patch]
	mux.HandleFunc("PATCH /api/v1/incidents/{id}/status", h.UpdateStatus)

	// @Summary Add an event to an incident
	// @Description Adds a new event to the incident timeline
	// @Tags incidents
	// @Accept json
	// @Produce json
	// @Param id path string true "Incident ID"
	// @Param event body dto.AddEventRequest true "Event details"
	// @Success 201 {object} dto.EventResponse "Created event"
	// @Failure 400 {object} dto.ErrorResponse "Invalid request"
	// @Failure 404 {object} dto.ErrorResponse "Incident not found"
	// @Failure 500 {object} dto.ErrorResponse "Internal server error"
	// @Router /api/v1/incidents/{id}/events [post]
	mux.HandleFunc("POST /api/v1/incidents/{id}/events", h.AddEvent)
}

// CreateIncident handles POST /api/v1/incidents
func (h *IncidentHandler) CreateIncident(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestIDFromContext(ctx)
	logger := h.logger.WithRequestID(requestID)

	logger.Debug("Creating new incident")

	// Parse request body
	var req dto.CreateIncidentRequest
	if err := h.parseJSONRequest(r, &req); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Convert to domain model
	incident, err := req.ToIncident()
	if err != nil {
		logger.WithError(err).Error("Failed to convert request to domain model")
		h.writeError(w, requestID, http.StatusBadRequest, "CONVERSION_ERROR", err.Error(), nil)
		return
	}

	// Create incident via service
	createdIncident, err := h.service.CreateIncident(ctx, incident.Title, incident.Severity.String(), incident.TeamID)
	if err != nil {
		logger.WithError(err).Error("Failed to create incident")
		h.handleServiceError(w, requestID, err)
		return
	}

	// Update additional fields if provided
	if req.Description != "" || req.ServiceID != "" || len(req.Labels) > 0 {
		createdIncident.Description = req.Description
		createdIncident.ServiceID = req.ServiceID
		if req.Labels != nil {
			createdIncident.Labels = req.Labels
		}
		// Note: In a full implementation, you'd call an update service method here
	}

	logger.WithFields("incident_id", createdIncident.ID).Info("Incident created successfully")

	// Convert to response DTO and return
	response := dto.FromIncident(createdIncident)
	h.writeJSONResponse(w, http.StatusCreated, response)
}

// GetIncident handles GET /api/v1/incidents/{id}
func (h *IncidentHandler) GetIncident(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestIDFromContext(ctx)
	logger := h.logger.WithRequestID(requestID)

	// Extract incident ID from path
	incidentID := r.PathValue("id")
	if incidentID == "" {
		h.writeError(w, requestID, http.StatusBadRequest, "MISSING_PARAMETER", "incident ID is required", nil)
		return
	}

	logger.WithFields("incident_id", incidentID).Debug("Retrieving incident")

	// Get incident via service
	incident, err := h.service.GetIncident(ctx, incidentID)
	if err != nil {
		logger.WithError(err).Error("Failed to get incident")
		h.handleServiceError(w, requestID, err)
		return
	}

	logger.WithFields("incident_id", incidentID).Debug("Incident retrieved successfully")

	// Convert to response DTO and return
	response := dto.FromIncident(incident)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// ListIncidents handles GET /api/v1/incidents
func (h *IncidentHandler) ListIncidents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestIDFromContext(ctx)
	logger := h.logger.WithRequestID(requestID)

	logger.Debug("Listing incidents")

	// Parse query parameters
	req, err := h.parseListIncidentsRequest(r)
	if err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_PARAMETERS", err.Error(), nil)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Convert to service filter
	filter, err := h.convertToListFilter(req)
	if err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "FILTER_ERROR", err.Error(), nil)
		return
	}

	// List incidents via service
	result, err := h.service.ListIncidents(ctx, *filter)
	if err != nil {
		logger.WithError(err).Error("Failed to list incidents")
		h.handleServiceError(w, requestID, err)
		return
	}

	logger.WithFields("total", result.Total, "returned", len(result.Incidents)).Debug("Incidents listed successfully")

	// Convert to response DTO
	response := &dto.ListIncidentsResponse{
		Incidents: make([]dto.IncidentResponse, len(result.Incidents)),
		Total:     result.Total,
		Limit:     result.Limit,
		Offset:    result.Offset,
		HasMore:   result.HasMore,
	}

	for i, incident := range result.Incidents {
		response.Incidents[i] = *dto.FromIncident(incident)
	}

	h.writeJSONResponse(w, http.StatusOK, response)
}

// UpdateStatus handles PATCH /api/v1/incidents/{id}/status
func (h *IncidentHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestIDFromContext(ctx)
	logger := h.logger.WithRequestID(requestID)

	// Extract incident ID from path
	incidentID := r.PathValue("id")
	if incidentID == "" {
		h.writeError(w, requestID, http.StatusBadRequest, "MISSING_PARAMETER", "incident ID is required", nil)
		return
	}

	logger.WithFields("incident_id", incidentID).Debug("Updating incident status")

	// Parse request body
	var req dto.UpdateStatusRequest
	if err := h.parseJSONRequest(r, &req); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Convert status string to domain type
	var status domain.Status
	switch req.Status {
	case "triggered":
		status = domain.StatusTriggered
	case "acknowledged":
		status = domain.StatusAcknowledged
	case "investigating":
		status = domain.StatusInvestigating
	case "resolved":
		status = domain.StatusResolved
	default:
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_STATUS", "invalid status value", nil)
		return
	}

	// Update status via service
	err := h.service.UpdateStatus(ctx, incidentID, status)
	if err != nil {
		logger.WithError(err).Error("Failed to update incident status")
		h.handleServiceError(w, requestID, err)
		return
	}

	// Record event if comment provided
	if req.Comment != "" {
		eventReq := &dto.AddEventRequest{
			Type:        "status_changed",
			Actor:       req.Actor,
			Description: fmt.Sprintf("Status changed to %s: %s", req.Status, req.Comment),
			Metadata: map[string]interface{}{
				"new_status": req.Status,
				"comment":    req.Comment,
			},
		}

		event := eventReq.ToEvent(incidentID)
		if err := h.service.RecordEvent(ctx, incidentID, *event); err != nil {
			logger.WithError(err).Warn("Failed to record status change event")
			// Don't fail the request for this
		}
	}

	// Get updated incident
	updatedIncident, err := h.service.GetIncident(ctx, incidentID)
	if err != nil {
		logger.WithError(err).Error("Failed to get updated incident")
		h.handleServiceError(w, requestID, err)
		return
	}

	logger.WithFields("incident_id", incidentID, "new_status", req.Status).Info("Incident status updated successfully")

	// Convert to response DTO and return
	response := dto.FromIncident(updatedIncident)
	h.writeJSONResponse(w, http.StatusOK, response)
}

// AddEvent handles POST /api/v1/incidents/{id}/events
func (h *IncidentHandler) AddEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID := middleware.GetRequestIDFromContext(ctx)
	logger := h.logger.WithRequestID(requestID)

	// Extract incident ID from path
	incidentID := r.PathValue("id")
	if incidentID == "" {
		h.writeError(w, requestID, http.StatusBadRequest, "MISSING_PARAMETER", "incident ID is required", nil)
		return
	}

	logger.WithFields("incident_id", incidentID).Debug("Adding event to incident")

	// Parse request body
	var req dto.AddEventRequest
	if err := h.parseJSONRequest(r, &req); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_JSON", err.Error(), nil)
		return
	}

	// Validate request
	if err := req.Validate(); err != nil {
		h.writeError(w, requestID, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Convert to domain event
	event := req.ToEvent(incidentID)

	// Add event via service
	err := h.service.RecordEvent(ctx, incidentID, *event)
	if err != nil {
		logger.WithError(err).Error("Failed to add event to incident")
		h.handleServiceError(w, requestID, err)
		return
	}

	logger.WithFields("incident_id", incidentID, "event_type", req.Type).Info("Event added to incident successfully")

	// Convert to response DTO and return
	response := dto.FromEvent(*event)
	h.writeJSONResponse(w, http.StatusCreated, &response)
}

// Helper methods

// parseJSONRequest parses a JSON request body into the provided struct.
func (h *IncidentHandler) parseJSONRequest(r *http.Request, dst interface{}) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("content-Type must be application/json")
	}

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	return nil
}

// parseListIncidentsRequest parses query parameters into a ListIncidentsRequest.
func (h *IncidentHandler) parseListIncidentsRequest(r *http.Request) (*dto.ListIncidentsRequest, error) {
	req := &dto.ListIncidentsRequest{
		TeamID:     r.URL.Query().Get("team_id"),
		Status:     r.URL.Query().Get("status"),
		Severity:   r.URL.Query().Get("severity"),
		AssigneeID: r.URL.Query().Get("assignee_id"),
		ServiceID:  r.URL.Query().Get("service_id"),
		Search:     r.URL.Query().Get("search"),
		SortBy:     r.URL.Query().Get("sort_by"),
		SortOrder:  r.URL.Query().Get("sort_order"),
	}

	// Parse limit
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return nil, fmt.Errorf("invalid limit parameter: %w", err)
		}
		req.Limit = limit
	}

	// Parse offset
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return nil, fmt.Errorf("invalid offset parameter: %w", err)
		}
		req.Offset = offset
	}

	return req, nil
}

// convertToListFilter converts a DTO ListIncidentsRequest to a ports.ListFilter.
func (h *IncidentHandler) convertToListFilter(req *dto.ListIncidentsRequest) (*ports.ListFilter, error) {
	filter := &ports.ListFilter{
		TeamID:     req.TeamID,
		AssigneeID: req.AssigneeID,
		ServiceID:  req.ServiceID,
		Search:     req.Search,
		Limit:      req.Limit,
		Offset:     req.Offset,
		SortBy:     req.SortBy,
		SortOrder:  req.SortOrder,
	}

	// Parse status filter
	if req.Status != "" {
		statusStrs := strings.Split(req.Status, ",")
		filter.Status = make([]domain.Status, len(statusStrs))
		for i, statusStr := range statusStrs {
			statusStr = strings.TrimSpace(statusStr)
			switch statusStr {
			case "triggered":
				filter.Status[i] = domain.StatusTriggered
			case "acknowledged":
				filter.Status[i] = domain.StatusAcknowledged
			case "investigating":
				filter.Status[i] = domain.StatusInvestigating
			case "resolved":
				filter.Status[i] = domain.StatusResolved
			default:
				return nil, fmt.Errorf("invalid status: %s", statusStr)
			}
		}
	}

	// Parse severity filter
	if req.Severity != "" {
		severityStrs := strings.Split(req.Severity, ",")
		filter.Severity = make([]domain.Severity, len(severityStrs))
		for i, severityStr := range severityStrs {
			severityStr = strings.TrimSpace(severityStr)
			switch severityStr {
			case "critical":
				filter.Severity[i] = domain.SeverityCritical
			case "high":
				filter.Severity[i] = domain.SeverityHigh
			case "medium":
				filter.Severity[i] = domain.SeverityMedium
			case "low":
				filter.Severity[i] = domain.SeverityLow
			case "info":
				filter.Severity[i] = domain.SeverityInfo
			default:
				return nil, fmt.Errorf("invalid severity: %s", severityStr)
			}
		}
	}

	return filter, nil
}

// writeJSONResponse writes a JSON response with the specified status code.
func (h *IncidentHandler) writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.WithError(err).Error("Failed to encode JSON response")
	}
}

// writeError writes an error response in the standard format.
func (h *IncidentHandler) writeError(w http.ResponseWriter, requestID string, statusCode int, code, message string, details map[string]interface{}) {
	errorResponse := &dto.ErrorResponse{
		Error:     message,
		Code:      code,
		RequestID: requestID,
		Details:   details,
	}

	h.writeJSONResponse(w, statusCode, errorResponse)
}

// handleServiceError maps service errors to appropriate HTTP responses.
func (h *IncidentHandler) handleServiceError(w http.ResponseWriter, requestID string, err error) {
	switch {
	case err == ports.ErrNotFound:
		h.writeError(w, requestID, http.StatusNotFound, "NOT_FOUND", "Resource not found", nil)
	case err == ports.ErrAlreadyExists:
		h.writeError(w, requestID, http.StatusConflict, "ALREADY_EXISTS", "Resource already exists", nil)
	case err == ports.ErrConflict:
		h.writeError(w, requestID, http.StatusConflict, "CONFLICT", "Resource was modified by another process", nil)
	case err == ports.ErrInvalidInput:
		h.writeError(w, requestID, http.StatusBadRequest, "INVALID_INPUT", "Invalid input provided", nil)
	case err == ports.ErrTimeout:
		h.writeError(w, requestID, http.StatusRequestTimeout, "TIMEOUT", "Request timed out", nil)
	case err == ports.ErrConnectionFailed:
		h.writeError(w, requestID, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable", nil)
	case err == context.DeadlineExceeded:
		h.writeError(w, requestID, http.StatusRequestTimeout, "TIMEOUT", "Request deadline exceeded", nil)
	case err == context.Canceled:
		h.writeError(w, requestID, http.StatusRequestTimeout, "CANCELED", "Request was canceled", nil)
	default:
		h.logger.WithError(err).Error("Unhandled service error")
		h.writeError(w, requestID, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred", nil)
	}
}
