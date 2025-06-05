// Package middleware provides HTTP middleware components for the Guvnor incident management platform.
//
// This package includes middleware for request tracing, logging, and other cross-cutting concerns
// that need to be applied consistently across HTTP endpoints.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

// RequestIDHeader is the header name for request IDs.
const RequestIDHeader = "X-Request-ID"

// RequestIDContextKey is used to store request IDs in context.
type RequestIDContextKey struct{}

// RequestIDMiddleware adds a unique request ID to each HTTP request.
//
// The request ID is:
//   - Generated using crypto/rand for uniqueness
//   - Added to the request context for access by handlers
//   - Added to the response header for client tracking
//   - Used existing request ID from header if present and valid
//
// This middleware is essential for distributed tracing and log correlation
// across the incident management platform.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := getOrGenerateRequestID(r)

		// Add request ID to context
		ctx := context.WithValue(r.Context(), RequestIDContextKey{}, requestID)
		r = r.WithContext(ctx)

		// Add request ID to response header
		w.Header().Set(RequestIDHeader, requestID)

		next.ServeHTTP(w, r)
	})
}

// getOrGenerateRequestID extracts request ID from headers or generates a new one.
func getOrGenerateRequestID(r *http.Request) string {
	// Check if request ID already exists in headers
	if existingID := r.Header.Get(RequestIDHeader); existingID != "" && isValidRequestID(existingID) {
		return existingID
	}

	// Generate new request ID
	return generateRequestID()
}

// generateRequestID creates a new cryptographically secure request ID.
func generateRequestID() string {
	bytes := make([]byte, 8) // 16 character hex string
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simpler method if crypto/rand fails
		return "req-fallback-" + hex.EncodeToString([]byte("fallback"))[:8]
	}
	return "req-" + hex.EncodeToString(bytes)
}

// isValidRequestID checks if a request ID has a valid format.
//
// Valid request IDs should:
//   - Be non-empty
//   - Be reasonable length (between 4 and 64 characters)
//   - Contain only safe characters (alphanumeric, hyphens, underscores)
func isValidRequestID(id string) bool {
	if len(id) < 4 || len(id) > 64 {
		return false
	}

	for _, char := range id {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_') {
			return false
		}
	}

	return true
}

// GetRequestIDFromContext extracts the request ID from the given context.
//
// Returns an empty string if no request ID is found in context.
// This function is safe to call even if the context doesn't contain a request ID.
func GetRequestIDFromContext(ctx context.Context) string {
	if requestID, ok := ctx.Value(RequestIDContextKey{}).(string); ok {
		return requestID
	}
	return ""
}

// WithRequestID adds a request ID to the given context.
//
// This is useful for testing or for adding request IDs in contexts where
// the middleware hasn't been applied.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDContextKey{}, requestID)
}
