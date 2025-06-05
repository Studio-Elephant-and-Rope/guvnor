package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/api/dto"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/ports"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/services"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/middleware"
)

// mockIncidentRepository is a mock implementation for testing the real IncidentHandler
type mockIncidentRepository struct {
	incidents map[string]*domain.Incident
	events    map[string][]*domain.Event
	mu        sync.RWMutex
	// Control behaviour for testing
	shouldErrorOnCreate   bool
	shouldErrorOnGet      bool
	shouldErrorOnList     bool
	shouldErrorOnUpdate   bool
	shouldErrorOnAddEvent bool
	notFoundIDs           map[string]bool
}

func newMockIncidentRepository() *mockIncidentRepository {
	return &mockIncidentRepository{
		incidents:   make(map[string]*domain.Incident),
		events:      make(map[string][]*domain.Event),
		notFoundIDs: make(map[string]bool),
	}
}

func (m *mockIncidentRepository) Create(ctx context.Context, incident *domain.Incident) error {
	if m.shouldErrorOnCreate {
		return errors.New("storage connection failed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.incidents[incident.ID] = incident
	m.events[incident.ID] = []*domain.Event{}
	return nil
}

func (m *mockIncidentRepository) Get(ctx context.Context, id string) (*domain.Incident, error) {
	if m.shouldErrorOnGet {
		return nil, errors.New("storage connection failed")
	}

	if m.notFoundIDs[id] {
		return nil, ports.ErrNotFound
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	incident, exists := m.incidents[id]
	if !exists {
		return nil, ports.ErrNotFound
	}

	// Return a copy to avoid race conditions
	result := *incident
	if events, ok := m.events[id]; ok {
		result.Events = make([]domain.Event, len(events))
		for i, event := range events {
			result.Events[i] = *event
		}
	}

	return &result, nil
}

func (m *mockIncidentRepository) List(ctx context.Context, filter ports.ListFilter) (*ports.ListResult, error) {
	if m.shouldErrorOnList {
		return nil, errors.New("storage connection failed")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var incidents []*domain.Incident
	for _, incident := range m.incidents {
		incidents = append(incidents, incident)
	}

	// Simple filtering for testing
	if len(filter.Status) > 0 {
		var filtered []*domain.Incident
		for _, incident := range incidents {
			for _, status := range filter.Status {
				if incident.Status == status {
					filtered = append(filtered, incident)
					break
				}
			}
		}
		incidents = filtered
	}

	total := len(incidents)

	// Apply pagination
	start := filter.Offset
	if start > len(incidents) {
		start = len(incidents)
	}

	end := start + filter.Limit
	if end > len(incidents) {
		end = len(incidents)
	}

	if start < end {
		incidents = incidents[start:end]
	} else {
		incidents = []*domain.Incident{}
	}

	return &ports.ListResult{
		Incidents: incidents,
		Total:     total,
		Limit:     filter.Limit,
		Offset:    filter.Offset,
		HasMore:   filter.Offset+len(incidents) < total,
	}, nil
}

func (m *mockIncidentRepository) Update(ctx context.Context, incident *domain.Incident) error {
	if m.shouldErrorOnUpdate {
		return errors.New("storage connection failed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.incidents[incident.ID]; !exists {
		return ports.ErrNotFound
	}

	m.incidents[incident.ID] = incident
	return nil
}

func (m *mockIncidentRepository) AddEvent(ctx context.Context, event *domain.Event) error {
	if m.shouldErrorOnAddEvent {
		return errors.New("storage connection failed")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.incidents[event.IncidentID]; !exists {
		return ports.ErrNotFound
	}

	if m.events[event.IncidentID] == nil {
		m.events[event.IncidentID] = []*domain.Event{}
	}

	m.events[event.IncidentID] = append(m.events[event.IncidentID], event)
	return nil
}

func (m *mockIncidentRepository) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.incidents[id]; !exists {
		return ports.ErrNotFound
	}

	delete(m.incidents, id)
	delete(m.events, id)
	return nil
}

func (m *mockIncidentRepository) GetWithEvents(ctx context.Context, id string) (*domain.Incident, error) {
	return m.Get(ctx, id)
}

func (m *mockIncidentRepository) GetWithSignals(ctx context.Context, id string) (*domain.Incident, error) {
	return m.Get(ctx, id)
}

// Helper functions for creating test data
func createTestIncidentHandler() (*IncidentHandler, *mockIncidentRepository) {
	repo := newMockIncidentRepository()
	logger := createTestLogger()
	service, err := services.NewIncidentService(repo, logger)
	if err != nil {
		panic(fmt.Sprintf("Failed to create service: %v", err))
	}

	handler, err := NewIncidentHandler(service, logger)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test handler: %v", err))
	}

	return handler, repo
}

func createTestLogger() *logging.Logger {
	config := logging.Config{
		Environment: logging.Test,
		Level:       slog.LevelDebug,
		AddSource:   false,
		Output:      nil,
	}

	logger, err := logging.NewLogger(config)
	if err != nil {
		panic(fmt.Sprintf("Failed to create test logger: %v", err))
	}

	return logger
}

func createTestRequest(method, url string, body []byte) *http.Request {
	var bodyReader *bytes.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
		req := httptest.NewRequest(method, url, bodyReader)
		req.Header.Set("Content-Type", "application/json")
		return req
	}

	req := httptest.NewRequest(method, url, nil)
	return req
}

func createSampleIncident() *domain.Incident {
	now := time.Now().UTC()
	return &domain.Incident{
		ID:          "test-incident-id",
		Title:       "Test Database Issue",
		Description: "Test incident for database connectivity",
		Status:      domain.StatusTriggered,
		Severity:    domain.SeverityCritical,
		TeamID:      "team-123",
		ServiceID:   "service-456",
		AssigneeID:  "",
		Labels:      map[string]string{"environment": "production"},
		CreatedAt:   now,
		UpdatedAt:   now,
		ResolvedAt:  nil,
		Signals:     []domain.Signal{},
		Events:      []domain.Event{},
	}
}

// Test handler creation
func TestNewIncidentHandler(t *testing.T) {
	tests := []struct {
		name        string
		service     *services.IncidentService
		logger      *logging.Logger
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid dependencies",
			service: func() *services.IncidentService {
				s, _ := services.NewIncidentService(newMockIncidentRepository(), createTestLogger())
				return s
			}(),
			logger:      createTestLogger(),
			expectError: false,
		},
		{
			name:        "nil service",
			service:     nil,
			logger:      createTestLogger(),
			expectError: true,
			errorMsg:    "incident service cannot be nil",
		},
		{
			name: "nil logger",
			service: func() *services.IncidentService {
				s, _ := services.NewIncidentService(newMockIncidentRepository(), createTestLogger())
				return s
			}(),
			logger:      nil,
			expectError: true,
			errorMsg:    "logger cannot be nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, err := NewIncidentHandler(tt.service, tt.logger)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, handler)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
				assert.Equal(t, tt.service, handler.service)
				assert.Equal(t, tt.logger, handler.logger)
			}
		})
	}
}

func TestIncidentHandler_RegisterRoutes(t *testing.T) {
	handler, mockRepo := createTestIncidentHandler()

	// Add a test incident for the GET route
	incident := createSampleIncident()
	incident.ID = "test-id"
	mockRepo.incidents["test-id"] = incident

	mux := http.NewServeMux()

	// Register routes
	handler.RegisterRoutes(mux)

	// Test that routes are registered by making requests
	routes := []struct {
		method string
		path   string
	}{
		{"POST", "/api/v1/incidents"},
		{"GET", "/api/v1/incidents/test-id"},
		{"GET", "/api/v1/incidents"},
		{"PATCH", "/api/v1/incidents/test-id/status"},
		{"POST", "/api/v1/incidents/test-id/events"},
	}

	for _, route := range routes {
		t.Run(fmt.Sprintf("%s %s", route.method, route.path), func(t *testing.T) {
			req := createTestRequest(route.method, route.path, nil)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))

			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// We expect the route to be handled (not 404)
			assert.NotEqual(t, http.StatusNotFound, rr.Code, "Route should be registered")
		})
	}
}

func TestIncidentHandler_CreateIncident(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    interface{}
		setupMock      func(*mockIncidentRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name: "valid incident creation",
			requestBody: dto.CreateIncidentRequest{
				Title:       "Database connection failure",
				Description: "Cannot connect to primary database",
				Severity:    "critical",
				TeamID:      "team-123",
				ServiceID:   "db-service",
				Labels:      map[string]string{"env": "production"},
			},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "invalid JSON",
			requestBody:    "invalid-json",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "INVALID_JSON",
		},
		{
			name: "validation error - empty title",
			requestBody: dto.CreateIncidentRequest{
				Title:    "",
				Severity: "high",
				TeamID:   "team-123",
			},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "VALIDATION_ERROR",
		},
		{
			name: "service error",
			requestBody: dto.CreateIncidentRequest{
				Title:    "Test incident",
				Severity: "high",
				TeamID:   "team-123",
			},
			setupMock: func(m *mockIncidentRepository) {
				m.shouldErrorOnCreate = true
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, mockRepo := createTestIncidentHandler()
			tt.setupMock(mockRepo)

			var bodyBytes []byte
			if tt.requestBody != nil {
				if str, ok := tt.requestBody.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}

			req := createTestRequest("POST", "/api/v1/incidents", bodyBytes)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))

			rr := httptest.NewRecorder()
			handler.CreateIncident(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errorResp dto.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp.Code, tt.expectedError)
			} else if tt.expectedStatus == http.StatusCreated {
				var response dto.IncidentResponse
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.NotEmpty(t, response.ID)
			}
		})
	}
}

func TestIncidentHandler_GetIncident(t *testing.T) {
	tests := []struct {
		name           string
		incidentID     string
		setupMock      func(*mockIncidentRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name:       "existing incident",
			incidentID: "test-id",
			setupMock: func(m *mockIncidentRepository) {
				incident := createSampleIncident()
				incident.ID = "test-id"
				m.incidents["test-id"] = incident
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing incident ID",
			incidentID:     "",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "MISSING_PARAMETER",
		},
		{
			name:           "non-existent incident",
			incidentID:     "non-existent",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:       "service error",
			incidentID: "test-id",
			setupMock: func(m *mockIncidentRepository) {
				m.shouldErrorOnGet = true
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, mockRepo := createTestIncidentHandler()
			tt.setupMock(mockRepo)

			url := "/api/v1/incidents/" + tt.incidentID
			req := createTestRequest("GET", url, nil)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))
			req.SetPathValue("id", tt.incidentID)

			rr := httptest.NewRecorder()
			handler.GetIncident(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errorResp dto.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp.Code, tt.expectedError)
			} else if tt.expectedStatus == http.StatusOK {
				var response dto.IncidentResponse
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.Equal(t, tt.incidentID, response.ID)
			}
		})
	}
}

func TestIncidentHandler_ListIncidents(t *testing.T) {
	tests := []struct {
		name           string
		queryParams    string
		setupMock      func(*mockIncidentRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name:        "successful list",
			queryParams: "?limit=10&offset=0",
			setupMock: func(m *mockIncidentRepository) {
				m.incidents["id1"] = createSampleIncident()
				m.incidents["id2"] = createSampleIncident()
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid limit parameter",
			queryParams:    "?limit=invalid",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "INVALID_PARAMETERS",
		},
		{
			name:        "service error",
			queryParams: "?limit=10",
			setupMock: func(m *mockIncidentRepository) {
				m.shouldErrorOnList = true
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, mockRepo := createTestIncidentHandler()
			tt.setupMock(mockRepo)

			url := "/api/v1/incidents" + tt.queryParams
			req := createTestRequest("GET", url, nil)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))

			rr := httptest.NewRecorder()
			handler.ListIncidents(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errorResp dto.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp.Code, tt.expectedError)
			} else if tt.expectedStatus == http.StatusOK {
				var response dto.ListIncidentsResponse
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				require.NoError(t, err)
				assert.GreaterOrEqual(t, response.Total, 0)
			}
		})
	}
}

func TestIncidentHandler_UpdateStatus(t *testing.T) {
	tests := []struct {
		name           string
		incidentID     string
		requestBody    interface{}
		setupMock      func(*mockIncidentRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name:       "valid status update",
			incidentID: "test-id",
			requestBody: dto.UpdateStatusRequest{
				Status: "acknowledged",
			},
			setupMock: func(m *mockIncidentRepository) {
				incident := createSampleIncident()
				incident.ID = "test-id"
				m.incidents["test-id"] = incident
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing incident ID",
			incidentID:     "",
			requestBody:    dto.UpdateStatusRequest{Status: "acknowledged"},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "MISSING_PARAMETER",
		},
		{
			name:           "invalid JSON",
			incidentID:     "test-id",
			requestBody:    "invalid-json",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "INVALID_JSON",
		},
		{
			name:       "invalid status",
			incidentID: "test-id",
			requestBody: dto.UpdateStatusRequest{
				Status: "invalid-status",
			},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, mockRepo := createTestIncidentHandler()
			tt.setupMock(mockRepo)

			var bodyBytes []byte
			if tt.requestBody != nil {
				if str, ok := tt.requestBody.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}

			url := "/api/v1/incidents/" + tt.incidentID + "/status"
			req := createTestRequest("PATCH", url, bodyBytes)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))
			req.SetPathValue("id", tt.incidentID)

			rr := httptest.NewRecorder()
			handler.UpdateStatus(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errorResp dto.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp.Code, tt.expectedError)
			}
		})
	}
}

func TestIncidentHandler_AddEvent(t *testing.T) {
	tests := []struct {
		name           string
		incidentID     string
		requestBody    interface{}
		setupMock      func(*mockIncidentRepository)
		expectedStatus int
		expectedError  string
	}{
		{
			name:       "valid event addition",
			incidentID: "test-id",
			requestBody: dto.AddEventRequest{
				Type:        "note_added",
				Description: "Investigation update",
				Actor:       "user-123",
			},
			setupMock: func(m *mockIncidentRepository) {
				incident := createSampleIncident()
				incident.ID = "test-id"
				m.incidents["test-id"] = incident
			},
			expectedStatus: http.StatusCreated,
		},
		{
			name:           "missing incident ID",
			incidentID:     "",
			requestBody:    dto.AddEventRequest{Type: "note_added"},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "MISSING_PARAMETER",
		},
		{
			name:           "invalid JSON",
			incidentID:     "test-id",
			requestBody:    "invalid-json",
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "INVALID_JSON",
		},
		{
			name:       "validation error",
			incidentID: "test-id",
			requestBody: dto.AddEventRequest{
				Type: "", // Invalid empty type
			},
			setupMock:      func(m *mockIncidentRepository) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "VALIDATION_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, mockRepo := createTestIncidentHandler()
			tt.setupMock(mockRepo)

			var bodyBytes []byte
			if tt.requestBody != nil {
				if str, ok := tt.requestBody.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, _ = json.Marshal(tt.requestBody)
				}
			}

			url := "/api/v1/incidents/" + tt.incidentID + "/events"
			req := createTestRequest("POST", url, bodyBytes)
			req = req.WithContext(middleware.WithRequestID(req.Context(), "test-request-id"))
			req.SetPathValue("id", tt.incidentID)

			rr := httptest.NewRecorder()
			handler.AddEvent(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			if tt.expectedError != "" {
				var errorResp dto.ErrorResponse
				err := json.Unmarshal(rr.Body.Bytes(), &errorResp)
				require.NoError(t, err)
				assert.Contains(t, errorResp.Code, tt.expectedError)
			}
		})
	}
}

func TestIncidentHandler_parseJSONRequest(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	tests := []struct {
		name        string
		body        string
		expectError bool
	}{
		{
			name:        "valid JSON",
			body:        `{"title": "test", "severity": "high"}`,
			expectError: false,
		},
		{
			name:        "invalid JSON",
			body:        `{"title": "test", "severity":}`,
			expectError: true,
		},
		{
			name:        "empty body",
			body:        "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequest("POST", "/test", []byte(tt.body))

			var dst dto.CreateIncidentRequest
			err := handler.parseJSONRequest(req, &dst)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIncidentHandler_parseListIncidentsRequest(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	tests := []struct {
		name        string
		queryParams string
		expectError bool
	}{
		{
			name:        "valid parameters",
			queryParams: "?limit=10&offset=0&status=triggered",
			expectError: false,
		},
		{
			name:        "invalid limit",
			queryParams: "?limit=invalid",
			expectError: true,
		},
		{
			name:        "negative offset gets corrected",
			queryParams: "?offset=-1",
			expectError: false, // DTO validation corrects negative offset to 0
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := createTestRequest("GET", "/test"+tt.queryParams, nil)

			_, err := handler.parseListIncidentsRequest(req)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIncidentHandler_convertToListFilter(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	req := &dto.ListIncidentsRequest{
		Limit:     10,
		Offset:    0,
		Status:    "triggered,acknowledged",
		Severity:  "critical,high",
		TeamID:    "team-123",
		Search:    "database",
		SortBy:    "created_at",
		SortOrder: "desc",
	}

	filter, err := handler.convertToListFilter(req)
	require.NoError(t, err)
	assert.Equal(t, 10, filter.Limit)
	assert.Equal(t, 0, filter.Offset)
	assert.Len(t, filter.Status, 2)
	assert.Len(t, filter.Severity, 2)
}

func TestIncidentHandler_writeJSONResponse(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	rr := httptest.NewRecorder()
	data := map[string]string{"test": "data"}

	handler.writeJSONResponse(rr, http.StatusOK, data)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "data", response["test"])
}

func TestIncidentHandler_writeError(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	rr := httptest.NewRecorder()
	details := map[string]interface{}{"field": "value"}

	handler.writeError(rr, "test-request-id", http.StatusBadRequest, "TEST_ERROR", "Test error message", details)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var response dto.ErrorResponse
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "TEST_ERROR", response.Code)
	assert.Equal(t, "Test error message", response.Error)
	assert.Equal(t, "test-request-id", response.RequestID)
	assert.Equal(t, details, response.Details)
}

func TestIncidentHandler_handleServiceError(t *testing.T) {
	handler, _ := createTestIncidentHandler()

	tests := []struct {
		name         string
		serviceError error
		expectedCode int
	}{
		{
			name:         "not found error",
			serviceError: ports.ErrNotFound,
			expectedCode: http.StatusNotFound,
		},
		{
			name:         "validation error",
			serviceError: ports.ErrInvalidInput,
			expectedCode: http.StatusBadRequest,
		},
		{
			name:         "conflict error",
			serviceError: ports.ErrConflict,
			expectedCode: http.StatusConflict,
		},
		{
			name:         "generic error",
			serviceError: errors.New("generic error"),
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			handler.handleServiceError(rr, "test-request-id", tt.serviceError)

			assert.Equal(t, tt.expectedCode, rr.Code)
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

			var response dto.ErrorResponse
			err := json.Unmarshal(rr.Body.Bytes(), &response)
			require.NoError(t, err)
			assert.NotEmpty(t, response.Code)
			assert.NotEmpty(t, response.Error)
			assert.Equal(t, "test-request-id", response.RequestID)
		})
	}
}
