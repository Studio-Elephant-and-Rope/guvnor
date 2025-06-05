package middleware

import (
	"context"
	"net/http"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

// WriteHeader captures the status code before writing.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the response size and writes the data.
func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// LoggingMiddleware logs HTTP requests with structured logging.
//
// For each request, logs:
//   - Request method, path, user agent
//   - Response status code and size
//   - Request duration in milliseconds
//   - Request ID for correlation
//   - Client IP address
//
// Uses different log levels based on response status:
//   - 5xx errors: ERROR level
//   - 4xx errors: WARN level
//   - All others: INFO level
//
// This middleware should be applied after RequestIDMiddleware to ensure
// request IDs are available for correlation.
func LoggingMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Check if logging should be skipped
			if skip, ok := r.Context().Value("skip_logging").(bool); ok && skip {
				next.ServeHTTP(w, r)
				return
			}

			// Get request ID from context
			requestID := GetRequestIDFromContext(r.Context())

			// Wrap response writer to capture status and size
			wrapped := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status
				size:           0,
			}

			// Process request
			next.ServeHTTP(wrapped, r)

			// Calculate duration
			duration := time.Since(start)

			// Extract client IP
			clientIP := getClientIP(r)

			// Create enriched logger with request context
			logCtx := logger.WithFields(
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"size_bytes", wrapped.size,
				"client_ip", clientIP,
				"user_agent", r.UserAgent(),
			)

			// Add request ID if available
			if requestID != "" {
				logCtx = logCtx.WithRequestID(requestID)
			}

			// Add query parameters if present
			if r.URL.RawQuery != "" {
				logCtx = logCtx.WithFields("query", r.URL.RawQuery)
			}

			// Log with appropriate level based on status code
			message := "HTTP request processed"
			switch {
			case wrapped.statusCode >= 500:
				logCtx.Error(message)
			case wrapped.statusCode >= 400:
				logCtx.Warn(message)
			default:
				logCtx.Info(message)
			}
		})
	}
}

// getClientIP extracts the client IP address from the request.
//
// Checks headers in order of preference:
//  1. X-Forwarded-For (proxy/load balancer)
//  2. X-Real-IP (nginx)
//  3. RemoteAddr (direct connection)
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (may contain multiple IPs)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		// Take the first IP in the list
		if idx := len(forwarded); idx > 0 {
			if commaIdx := 0; commaIdx < len(forwarded) {
				for i, char := range forwarded {
					if char == ',' {
						commaIdx = i
						break
					}
				}
				if commaIdx > 0 {
					return forwarded[:commaIdx]
				}
			}
			return forwarded
		}
	}

	// Check X-Real-IP header
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// HealthCheckSkipMiddleware skips logging for health check endpoints.
//
// Health checks can be very noisy in logs and typically don't provide
// value for debugging. This middleware allows skipping logging for
// specific paths like /health or /ping by setting a context flag.
func HealthCheckSkipMiddleware(paths []string) func(http.Handler) http.Handler {
	skipPaths := make(map[string]bool)
	for _, path := range paths {
		skipPaths[path] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Mark health check paths in context
			if skipPaths[r.URL.Path] {
				ctx := context.WithValue(r.Context(), "skip_logging", true)
				r = r.WithContext(ctx)
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CombinedMiddleware creates a logging middleware that combines logging with health check skipping.
//
// This is a convenience function that applies health check skipping before logging.
// Common health check paths are pre-configured, but additional paths can be specified.
func CombinedLoggingMiddleware(logger *logging.Logger, additionalSkipPaths ...string) func(http.Handler) http.Handler {
	// Default health check paths to skip
	defaultSkipPaths := []string{
		"/health",
		"/healthz",
		"/ping",
		"/ready",
		"/alive",
		"/metrics", // Prometheus metrics can be noisy
	}

	// Combine with additional paths
	allSkipPaths := append(defaultSkipPaths, additionalSkipPaths...)

	return func(next http.Handler) http.Handler {
		// Apply health check skipping first, then logging
		return HealthCheckSkipMiddleware(allSkipPaths)(LoggingMiddleware(logger)(next))
	}
}
