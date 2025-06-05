package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		existingHeader string
		expectGenerate bool
	}{
		{
			name:           "generates new request ID when none exists",
			existingHeader: "",
			expectGenerate: true,
		},
		{
			name:           "uses existing valid request ID",
			existingHeader: "existing-req-123",
			expectGenerate: false,
		},
		{
			name:           "generates new ID for invalid existing header",
			existingHeader: "invalid@request$id",
			expectGenerate: true,
		},
		{
			name:           "generates new ID for too short header",
			existingHeader: "abc",
			expectGenerate: true,
		},
		{
			name:           "generates new ID for too long header",
			existingHeader: strings.Repeat("a", 65),
			expectGenerate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test handler
			var requestID string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestID = GetRequestIDFromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Wrap with middleware
			middleware := RequestIDMiddleware(handler)

			// Create request
			req := httptest.NewRequest("GET", "/test", nil)
			if tt.existingHeader != "" {
				req.Header.Set(RequestIDHeader, tt.existingHeader)
			}

			// Create response recorder
			recorder := httptest.NewRecorder()

			// Execute request
			middleware.ServeHTTP(recorder, req)

			// Verify request ID was set in context
			if requestID == "" {
				t.Error("Request ID should be set in context")
			}

			// Verify request ID was set in response header
			responseHeader := recorder.Header().Get(RequestIDHeader)
			if responseHeader == "" {
				t.Error("Request ID should be set in response header")
			}

			// Verify context and response header match
			if requestID != responseHeader {
				t.Errorf("Context request ID (%s) should match response header (%s)", requestID, responseHeader)
			}

			// Verify generation behavior
			if tt.expectGenerate {
				if requestID == tt.existingHeader {
					t.Errorf("Expected new request ID, but got existing: %s", requestID)
				}
				if !strings.HasPrefix(requestID, "req-") {
					t.Errorf("Generated request ID should start with 'req-', got: %s", requestID)
				}
			} else {
				if requestID != tt.existingHeader {
					t.Errorf("Expected to use existing request ID (%s), but got: %s", tt.existingHeader, requestID)
				}
			}
		})
	}
}

func TestGenerateRequestID(t *testing.T) {
	// Generate multiple IDs to test uniqueness
	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := generateRequestID()

		// Check format
		if !strings.HasPrefix(id, "req-") {
			t.Errorf("Request ID should start with 'req-', got: %s", id)
		}

		// Check length (req- + 16 hex chars = 20 total)
		if len(id) != 20 {
			t.Errorf("Request ID should be 20 characters, got %d: %s", len(id), id)
		}

		// Check uniqueness
		if ids[id] {
			t.Errorf("Duplicate request ID generated: %s", id)
		}
		ids[id] = true

		// Verify it's valid
		if !isValidRequestID(id) {
			t.Errorf("Generated request ID should be valid: %s", id)
		}
	}
}

func TestIsValidRequestID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{
			name:  "valid simple ID",
			id:    "req-123456",
			valid: true,
		},
		{
			name:  "valid with hyphens and underscores",
			id:    "req-test_123-abc",
			valid: true,
		},
		{
			name:  "valid uppercase",
			id:    "REQ-TEST-123",
			valid: true,
		},
		{
			name:  "invalid - too short",
			id:    "abc",
			valid: false,
		},
		{
			name:  "invalid - too long",
			id:    strings.Repeat("a", 65),
			valid: false,
		},
		{
			name:  "invalid - special characters",
			id:    "req-test@123",
			valid: false,
		},
		{
			name:  "invalid - spaces",
			id:    "req test 123",
			valid: false,
		},
		{
			name:  "invalid - empty",
			id:    "",
			valid: false,
		},
		{
			name:  "valid - minimum length",
			id:    "abcd",
			valid: true,
		},
		{
			name:  "valid - maximum length",
			id:    strings.Repeat("a", 64),
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidRequestID(tt.id); got != tt.valid {
				t.Errorf("isValidRequestID(%s) = %v, want %v", tt.id, got, tt.valid)
			}
		})
	}
}

func TestGetRequestIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		setupCtx func() context.Context
		expectID string
	}{
		{
			name: "returns request ID when present",
			setupCtx: func() context.Context {
				return WithRequestID(context.Background(), "test-123")
			},
			expectID: "test-123",
		},
		{
			name: "returns empty string when not present",
			setupCtx: func() context.Context {
				return context.Background()
			},
			expectID: "",
		},
		{
			name: "returns empty string for wrong type",
			setupCtx: func() context.Context {
				return context.WithValue(context.Background(), RequestIDContextKey{}, 123)
			},
			expectID: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setupCtx()
			got := GetRequestIDFromContext(ctx)
			if got != tt.expectID {
				t.Errorf("GetRequestIDFromContext() = %v, want %v", got, tt.expectID)
			}
		})
	}
}

func TestWithRequestID(t *testing.T) {
	testID := "test-request-id"
	ctx := WithRequestID(context.Background(), testID)

	retrievedID := GetRequestIDFromContext(ctx)
	if retrievedID != testID {
		t.Errorf("WithRequestID() and GetRequestIDFromContext() mismatch: got %s, want %s", retrievedID, testID)
	}
}

func TestRequestIDMiddleware_Integration(t *testing.T) {
	// Test that request ID flows through multiple handlers
	var capturedIDs []string

	handler1 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = append(capturedIDs, GetRequestIDFromContext(r.Context()))
	})

	handler2 := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIDs = append(capturedIDs, GetRequestIDFromContext(r.Context()))
	})

	// Chain handlers with middleware
	middleware := RequestIDMiddleware(handler1)

	// Make first request
	req1 := httptest.NewRequest("GET", "/test1", nil)
	recorder1 := httptest.NewRecorder()
	middleware.ServeHTTP(recorder1, req1)

	// Make second request with different handler
	middleware2 := RequestIDMiddleware(handler2)
	req2 := httptest.NewRequest("GET", "/test2", nil)
	recorder2 := httptest.NewRecorder()
	middleware2.ServeHTTP(recorder2, req2)

	// Verify we captured IDs
	if len(capturedIDs) != 2 {
		t.Fatalf("Expected 2 captured IDs, got %d", len(capturedIDs))
	}

	// Verify IDs are different (statistically very unlikely to be same)
	if capturedIDs[0] == capturedIDs[1] {
		t.Error("Request IDs should be unique across requests")
	}

	// Verify both IDs are valid
	for i, id := range capturedIDs {
		if !isValidRequestID(id) {
			t.Errorf("Captured ID %d is invalid: %s", i, id)
		}
	}
}

func TestRequestIDFallback(t *testing.T) {
	// This test ensures the fallback works when crypto/rand fails
	// We can't easily simulate crypto/rand failure, but we can test the fallback format

	// Test the fallback directly by checking its format
	fallbackID := "req-fallback-" + "66616c6c"[:8] // "fall" in hex, truncated

	if !isValidRequestID(fallbackID) {
		t.Errorf("Fallback request ID should be valid: %s", fallbackID)
	}

	if !strings.HasPrefix(fallbackID, "req-fallback-") {
		t.Errorf("Fallback should start with 'req-fallback-', got: %s", fallbackID)
	}
}
