package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

func TestLoggingMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		method         string
		path           string
		statusCode     int
		responseBody   string
		userAgent      string
		clientIP       string
		queryString    string
		withRequestID  bool
		expectLogLevel string
	}{
		{
			name:           "successful GET request",
			method:         "GET",
			path:           "/api/incidents",
			statusCode:     http.StatusOK,
			responseBody:   `{"status":"ok"}`,
			userAgent:      "test-agent/1.0",
			expectLogLevel: "INFO",
		},
		{
			name:           "client error generates warning",
			method:         "POST",
			path:           "/api/incidents",
			statusCode:     http.StatusBadRequest,
			responseBody:   `{"error":"invalid input"}`,
			expectLogLevel: "WARN",
		},
		{
			name:           "server error generates error log",
			method:         "GET",
			path:           "/api/incidents/123",
			statusCode:     http.StatusInternalServerError,
			responseBody:   `{"error":"database error"}`,
			expectLogLevel: "ERROR",
		},
		{
			name:           "request with query parameters",
			method:         "GET",
			path:           "/api/incidents",
			statusCode:     http.StatusOK,
			queryString:    "status=open&limit=10",
			expectLogLevel: "INFO",
		},
		{
			name:           "request with ID in context",
			method:         "PUT",
			path:           "/api/incidents/456",
			statusCode:     http.StatusAccepted,
			withRequestID:  true,
			expectLogLevel: "INFO",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create buffer to capture logs
			var buf bytes.Buffer
			config := logging.Config{
				Environment: logging.Production,
				Level:       slog.LevelDebug,
				Output:      &buf,
				AddSource:   false,
			}

			logger, err := logging.NewLogger(config)
			if err != nil {
				t.Fatalf("Failed to create logger: %v", err)
			}

			// Create test handler
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.responseBody != "" {
					w.Write([]byte(tt.responseBody))
				}
			})

			// Apply logging middleware
			middleware := LoggingMiddleware(logger)(handler)

			// Create request
			url := tt.path
			if tt.queryString != "" {
				url += "?" + tt.queryString
			}
			req := httptest.NewRequest(tt.method, url, nil)

			// Set user agent if specified
			if tt.userAgent != "" {
				req.Header.Set("User-Agent", tt.userAgent)
			}

			// Add request ID to context if specified
			if tt.withRequestID {
				ctx := WithRequestID(req.Context(), "test-req-123")
				req = req.WithContext(ctx)
			}

			// Set client IP headers for testing
			if tt.clientIP != "" {
				req.Header.Set("X-Forwarded-For", tt.clientIP)
			}

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Execute request
			middleware.ServeHTTP(recorder, req)

			// Parse log output
			if buf.Len() == 0 {
				t.Fatal("Expected log output but got none")
			}

			var logEntry map[string]interface{}
			if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
				t.Fatalf("Failed to parse log JSON: %v", err)
			}

			// Verify log level
			if logEntry["level"] != tt.expectLogLevel {
				t.Errorf("Expected log level %s, got %v", tt.expectLogLevel, logEntry["level"])
			}

			// Verify request details
			if logEntry["method"] != tt.method {
				t.Errorf("Expected method %s, got %v", tt.method, logEntry["method"])
			}
			if logEntry["path"] != tt.path {
				t.Errorf("Expected path %s, got %v", tt.path, logEntry["path"])
			}
			if int(logEntry["status"].(float64)) != tt.statusCode {
				t.Errorf("Expected status %d, got %v", tt.statusCode, logEntry["status"])
			}

			// Verify response size
			expectedSize := len(tt.responseBody)
			if int(logEntry["size_bytes"].(float64)) != expectedSize {
				t.Errorf("Expected size %d, got %v", expectedSize, logEntry["size_bytes"])
			}

			// Verify duration exists and is reasonable
			if duration, exists := logEntry["duration_ms"]; !exists {
				t.Error("Expected duration_ms field")
			} else if duration.(float64) < 0 {
				t.Error("Duration should be non-negative")
			}

			// Verify user agent if set
			if tt.userAgent != "" {
				if logEntry["user_agent"] != tt.userAgent {
					t.Errorf("Expected user agent %s, got %v", tt.userAgent, logEntry["user_agent"])
				}
			}

			// Verify query parameters if present
			if tt.queryString != "" {
				if logEntry["query"] != tt.queryString {
					t.Errorf("Expected query %s, got %v", tt.queryString, logEntry["query"])
				}
			}

			// Verify request ID if present
			if tt.withRequestID {
				if logEntry["request_id"] != "test-req-123" {
					t.Errorf("Expected request_id test-req-123, got %v", logEntry["request_id"])
				}
			}

			// Verify message
			if logEntry["msg"] != "HTTP request processed" {
				t.Errorf("Expected message 'HTTP request processed', got %v", logEntry["msg"])
			}
		})
	}
}

func TestGetClientIP(t *testing.T) {
	tests := []struct {
		name       string
		setupReq   func() *http.Request
		expectedIP string
	}{
		{
			name: "X-Forwarded-For single IP",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Forwarded-For", "192.168.1.100")
				return req
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Forwarded-For multiple IPs",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Forwarded-For", "192.168.1.100, 10.0.0.1, 172.16.0.1")
				return req
			},
			expectedIP: "192.168.1.100",
		},
		{
			name: "X-Real-IP header",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Real-IP", "203.0.113.1")
				return req
			},
			expectedIP: "203.0.113.1",
		},
		{
			name: "RemoteAddr fallback",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.RemoteAddr = "198.51.100.1:54321"
				return req
			},
			expectedIP: "198.51.100.1:54321",
		},
		{
			name: "X-Forwarded-For takes precedence over X-Real-IP",
			setupReq: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("X-Forwarded-For", "192.168.1.100")
				req.Header.Set("X-Real-IP", "203.0.113.1")
				return req
			},
			expectedIP: "192.168.1.100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupReq()
			ip := getClientIP(req)
			if ip != tt.expectedIP {
				t.Errorf("getClientIP() = %v, want %v", ip, tt.expectedIP)
			}
		})
	}
}

func TestResponseWriter(t *testing.T) {
	// Test the response writer wrapper
	recorder := httptest.NewRecorder()
	wrapped := &responseWriter{
		ResponseWriter: recorder,
		statusCode:     http.StatusOK,
		size:           0,
	}

	// Test WriteHeader
	wrapped.WriteHeader(http.StatusCreated)
	if wrapped.statusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, wrapped.statusCode)
	}

	// Test Write
	testData := []byte("test response data")
	n, err := wrapped.Write(testData)
	if err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(testData), n)
	}
	if wrapped.size != len(testData) {
		t.Errorf("Expected size %d, got %d", len(testData), wrapped.size)
	}

	// Verify data was actually written
	if recorder.Body.String() != string(testData) {
		t.Errorf("Expected body %s, got %s", string(testData), recorder.Body.String())
	}
}

func TestHealthCheckSkipMiddleware(t *testing.T) {
	skipPaths := []string{"/health", "/ping"}

	// Create a handler that always logs (to test skipping)
	var loggedPaths []string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loggedPaths = append(loggedPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
	})

	middleware := HealthCheckSkipMiddleware(skipPaths)(handler)

	tests := []struct {
		path        string
		shouldLog   bool
	}{
		{"/health", true},    // Should still call handler, just skip logging in real middleware
		{"/ping", true},      // Should still call handler, just skip logging in real middleware
		{"/api/incidents", true}, // Normal path should log
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			initialCount := len(loggedPaths)

			req := httptest.NewRequest("GET", tt.path, nil)
			recorder := httptest.NewRecorder()

			middleware.ServeHTTP(recorder, req)

			// Handler should always be called (middleware just affects logging)
			if len(loggedPaths) != initialCount+1 {
				t.Errorf("Handler should be called for path %s", tt.path)
			}
		})
	}
}

func TestCombinedLoggingMiddleware(t *testing.T) {
	var buf bytes.Buffer
	config := logging.Config{
		Environment: logging.Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := logging.NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create combined middleware with additional skip path
	middleware := CombinedLoggingMiddleware(logger, "/custom-health")(handler)

	tests := []struct {
		path       string
		shouldSkip bool
	}{
		{"/health", true},
		{"/healthz", true},
		{"/ping", true},
		{"/ready", true},
		{"/alive", true},
		{"/metrics", true},
		{"/custom-health", true},
		{"/api/incidents", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			buf.Reset() // Clear buffer for each test

			req := httptest.NewRequest("GET", tt.path, nil)
			recorder := httptest.NewRecorder()

			middleware.ServeHTTP(recorder, req)

			// For health check paths, there should be no log output
			// For regular paths, there should be log output
			hasLogOutput := buf.Len() > 0

			if tt.shouldSkip && hasLogOutput {
				t.Errorf("Path %s should skip logging but generated log output", tt.path)
			}
			if !tt.shouldSkip && !hasLogOutput {
				t.Errorf("Path %s should generate log output but didn't", tt.path)
			}
		})
	}
}

func TestLoggingMiddleware_Performance(t *testing.T) {
	var buf bytes.Buffer
	config := logging.Config{
		Environment: logging.Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := logging.NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate some work
		time.Sleep(1 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	middleware := LoggingMiddleware(logger)(handler)

	// Run multiple requests to ensure middleware doesn't leak memory or slow down
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		recorder := httptest.NewRecorder()

		start := time.Now()
		middleware.ServeHTTP(recorder, req)
		duration := time.Since(start)

		// Middleware overhead should be minimal (less than 10ms)
		if duration > 10*time.Millisecond {
			t.Errorf("Request %d took too long: %v", i, duration)
		}
	}
}

func TestLoggingMiddleware_WithRequestIDIntegration(t *testing.T) {
	var buf bytes.Buffer
	config := logging.Config{
		Environment: logging.Production,
		Level:       slog.LevelInfo,
		Output:      &buf,
		AddSource:   false,
	}

	logger, err := logging.NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chain middlewares: RequestID first, then Logging
	middleware := RequestIDMiddleware(LoggingMiddleware(logger)(handler))

	req := httptest.NewRequest("GET", "/test", nil)
	recorder := httptest.NewRecorder()

	middleware.ServeHTTP(recorder, req)

	// Parse log output
	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log JSON: %v", err)
	}

	// Verify request ID was included in log
	requestID, exists := logEntry["request_id"]
	if !exists {
		t.Error("Expected request_id in log output")
	}

	// Verify request ID was also set in response header
	responseRequestID := recorder.Header().Get(RequestIDHeader)
	if responseRequestID == "" {
		t.Error("Expected request ID in response header")
	}

	// Verify they match
	if requestID != responseRequestID {
		t.Errorf("Log request ID (%v) should match response header (%s)", requestID, responseRequestID)
	}
}
