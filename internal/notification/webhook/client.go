// Package webhook provides a robust HTTP webhook client for sending incident notifications.
//
// The client implements retry logic with exponential backoff, circuit breaker pattern
// for failing endpoints, HMAC signature verification for security, and comprehensive
// audit logging for debugging and compliance.
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// WebhookPayload represents the JSON payload sent to webhook endpoints.
type WebhookPayload struct {
	IncidentID string    `json:"incident_id"`
	Title      string    `json:"title"`
	Status     string    `json:"status"`
	Severity   string    `json:"severity"`
	Timestamp  time.Time `json:"timestamp"`
	URL        string    `json:"url"`
}

// DeliveryAttempt represents a single attempt to deliver a webhook.
type DeliveryAttempt struct {
	ID            string            `json:"id"`
	WebhookURL    string            `json:"webhook_url"`
	Payload       WebhookPayload    `json:"payload"`
	AttemptNumber int               `json:"attempt_number"`
	StartedAt     time.Time         `json:"started_at"`
	CompletedAt   *time.Time        `json:"completed_at,omitempty"`
	StatusCode    int               `json:"status_code"`
	ResponseBody  string            `json:"response_body,omitempty"`
	Error         string            `json:"error,omitempty"`
	Duration      time.Duration     `json:"duration"`
	Headers       map[string]string `json:"headers"`
}

// CircuitBreakerState represents the state of a circuit breaker.
type CircuitBreakerState int

const (
	// CircuitBreakerClosed allows requests through normally
	CircuitBreakerClosed CircuitBreakerState = iota
	// CircuitBreakerOpen blocks all requests
	CircuitBreakerOpen
	// CircuitBreakerHalfOpen allows a limited number of test requests
	CircuitBreakerHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for webhook endpoints.
type CircuitBreaker struct {
	mu               sync.RWMutex
	state            CircuitBreakerState
	failures         int
	successes        int
	lastFailureTime  time.Time
	nextRetryTime    time.Time
	maxFailures      int
	resetTimeout     time.Duration
	halfOpenMaxCalls int
}

// NewCircuitBreaker creates a new circuit breaker with default settings.
func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitBreakerClosed,
		maxFailures:      5,
		resetTimeout:     60 * time.Second,
		halfOpenMaxCalls: 3,
	}
}

// CanExecute returns true if the circuit breaker allows the request to proceed.
func (cb *CircuitBreaker) CanExecute() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case CircuitBreakerClosed:
		return true
	case CircuitBreakerOpen:
		return time.Now().After(cb.nextRetryTime)
	case CircuitBreakerHalfOpen:
		return cb.successes < cb.halfOpenMaxCalls
	default:
		return false
	}
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures = 0
	cb.successes++

	if cb.state == CircuitBreakerHalfOpen && cb.successes >= cb.halfOpenMaxCalls {
		cb.state = CircuitBreakerClosed
		cb.successes = 0
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case CircuitBreakerClosed:
		if cb.failures >= cb.maxFailures {
			cb.state = CircuitBreakerOpen
			cb.nextRetryTime = time.Now().Add(cb.resetTimeout)
		}
	case CircuitBreakerHalfOpen:
		cb.state = CircuitBreakerOpen
		cb.nextRetryTime = time.Now().Add(cb.resetTimeout)
		cb.successes = 0
	}
}

// GetState returns the current state of the circuit breaker.
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Client provides webhook delivery functionality with retry logic and circuit breakers.
type Client struct {
	httpClient      *http.Client
	logger          *logging.Logger
	baseURL         string
	circuitBreakers map[string]*CircuitBreaker
	cbMutex         sync.RWMutex
	deliveryLog     []DeliveryAttempt
	logMutex        sync.RWMutex
}

// ClientConfig contains configuration options for the webhook client.
type ClientConfig struct {
	BaseURL string
	Timeout time.Duration
}

// NewClient creates a new webhook client with the specified configuration.
func NewClient(cfg ClientConfig, logger *logging.Logger) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger:          logger,
		baseURL:         cfg.BaseURL,
		circuitBreakers: make(map[string]*CircuitBreaker),
		deliveryLog:     make([]DeliveryAttempt, 0),
	}
}

// SendNotification sends a webhook notification for an incident to all configured endpoints.
func (c *Client) SendNotification(ctx context.Context, incident *domain.Incident, webhooks []config.WebhookConfig) error {
	if len(webhooks) == 0 {
		c.logger.Debug("No webhook endpoints configured, skipping notification")
		return nil
	}

	payload := c.createPayload(incident)

	var lastErr error
	successCount := 0

	// Send to all configured webhook endpoints
	for _, webhook := range webhooks {
		if !webhook.Enabled {
			c.logger.Debug("Webhook endpoint disabled, skipping",
				"url", webhook.URL)
			continue
		}

		err := c.sendToEndpoint(ctx, webhook, payload)
		if err != nil {
			c.logger.Error("Failed to send webhook notification",
				"url", webhook.URL,
				"incident_id", incident.ID,
				"error", err)
			lastErr = err
		} else {
			successCount++
		}
	}

	if successCount == 0 && lastErr != nil {
		return fmt.Errorf("failed to send notification to any webhook endpoint: %w", lastErr)
	}

	c.logger.Info("Webhook notifications sent",
		"incident_id", incident.ID,
		"successful", successCount,
		"total", len(webhooks))

	return nil
}

// sendToEndpoint sends a webhook notification to a single endpoint with retry logic.
func (c *Client) sendToEndpoint(ctx context.Context, webhook config.WebhookConfig, payload WebhookPayload) error {
	// Get or create circuit breaker for this endpoint
	cb := c.getCircuitBreaker(webhook.URL)

	// Check if circuit breaker allows the request
	if !cb.CanExecute() {
		return fmt.Errorf("circuit breaker is open for endpoint %s", webhook.URL)
	}

	maxRetries := webhook.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		deliveryAttempt := c.createDeliveryAttempt(webhook.URL, payload, attempt)

		err := c.executeWebhookRequest(ctx, webhook, payload, &deliveryAttempt)
		c.logDeliveryAttempt(deliveryAttempt)

		if err == nil {
			cb.RecordSuccess()
			return nil
		}

		lastErr = err
		cb.RecordFailure()

		// Don't retry if this is the last attempt
		if attempt < maxRetries {
			// Exponential backoff: 1s, 2s, 4s, 8s, ...
			backoffDuration := time.Duration(1<<uint(attempt-1)) * time.Second
			c.logger.Debug("Retrying webhook request after backoff",
				"url", webhook.URL,
				"attempt", attempt,
				"backoff", backoffDuration,
				"error", err)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoffDuration):
				continue
			}
		}
	}

	return fmt.Errorf("webhook request failed after %d attempts: %w", maxRetries, lastErr)
}

// executeWebhookRequest performs the actual HTTP request to the webhook endpoint.
func (c *Client) executeWebhookRequest(ctx context.Context, webhook config.WebhookConfig, payload WebhookPayload, delivery *DeliveryAttempt) error {
	// Marshal payload to JSON
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		delivery.Error = fmt.Sprintf("failed to marshal payload: %v", err)
		delivery.CompletedAt = &delivery.StartedAt
		return err
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhook.URL, bytes.NewBuffer(jsonPayload))
	if err != nil {
		delivery.Error = fmt.Sprintf("failed to create request: %v", err)
		delivery.CompletedAt = &delivery.StartedAt
		return err
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Guvnor-Webhook/1.0")

	// Add custom headers from configuration
	for key, value := range webhook.Headers {
		req.Header.Set(key, value)
	}

	// Add HMAC signature if secret is configured
	if webhook.Secret != "" {
		signature := c.generateHMACSignature(jsonPayload, webhook.Secret)
		req.Header.Set("X-Guvnor-Signature", signature)
	}

	// Record headers for audit
	delivery.Headers = make(map[string]string)
	for key, values := range req.Header {
		if len(values) > 0 {
			delivery.Headers[key] = values[0]
		}
	}

	// Execute request
	startTime := time.Now()
	resp, err := c.httpClient.Do(req)
	duration := time.Since(startTime)
	completedAt := time.Now()

	delivery.Duration = duration
	delivery.CompletedAt = &completedAt

	if err != nil {
		delivery.Error = fmt.Sprintf("HTTP request failed: %v", err)
		return err
	}
	defer resp.Body.Close()

	delivery.StatusCode = resp.StatusCode

	// Read response body (limited to 1KB for logging)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if err != nil {
		delivery.Error = fmt.Sprintf("failed to read response body: %v", err)
	} else {
		delivery.ResponseBody = string(body)
	}

	// Check if response indicates success
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		delivery.Error = fmt.Sprintf("HTTP %d: %s", resp.StatusCode, delivery.ResponseBody)
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// createPayload creates a webhook payload from an incident.
func (c *Client) createPayload(incident *domain.Incident) WebhookPayload {
	return WebhookPayload{
		IncidentID: incident.ID,
		Title:      incident.Title,
		Status:     string(incident.Status),
		Severity:   c.mapSeverityToSEV(incident.Severity),
		Timestamp:  incident.UpdatedAt,
		URL:        c.createIncidentURL(incident.ID),
	}
}

// mapSeverityToSEV converts domain severity to SEV1-5 format.
func (c *Client) mapSeverityToSEV(severity domain.Severity) string {
	switch severity {
	case domain.SeverityCritical:
		return "SEV1"
	case domain.SeverityHigh:
		return "SEV2"
	case domain.SeverityMedium:
		return "SEV3"
	case domain.SeverityLow:
		return "SEV4"
	case domain.SeverityInfo:
		return "SEV5"
	default:
		return "SEV3" // Default to medium
	}
}

// createIncidentURL creates a URL for viewing the incident.
func (c *Client) createIncidentURL(incidentID string) string {
	if c.baseURL == "" {
		return fmt.Sprintf("https://guvnor.local/incidents/%s", incidentID)
	}

	baseURL, err := url.Parse(c.baseURL)
	if err != nil {
		return fmt.Sprintf("https://guvnor.local/incidents/%s", incidentID)
	}

	baseURL.Path = fmt.Sprintf("/incidents/%s", incidentID)
	return baseURL.String()
}

// generateHMACSignature generates an HMAC-SHA256 signature for the payload.
func (c *Client) generateHMACSignature(payload []byte, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

// getCircuitBreaker returns the circuit breaker for a URL, creating one if needed.
func (c *Client) getCircuitBreaker(url string) *CircuitBreaker {
	c.cbMutex.RLock()
	cb, exists := c.circuitBreakers[url]
	c.cbMutex.RUnlock()

	if !exists {
		c.cbMutex.Lock()
		// Double-check pattern
		if cb, exists = c.circuitBreakers[url]; !exists {
			cb = NewCircuitBreaker()
			c.circuitBreakers[url] = cb
		}
		c.cbMutex.Unlock()
	}

	return cb
}

// createDeliveryAttempt creates a new delivery attempt record.
func (c *Client) createDeliveryAttempt(webhookURL string, payload WebhookPayload, attemptNumber int) DeliveryAttempt {
	return DeliveryAttempt{
		ID:            fmt.Sprintf("%s-%d-%d", payload.IncidentID, time.Now().Unix(), attemptNumber),
		WebhookURL:    webhookURL,
		Payload:       payload,
		AttemptNumber: attemptNumber,
		StartedAt:     time.Now(),
	}
}

// logDeliveryAttempt adds a delivery attempt to the audit log.
func (c *Client) logDeliveryAttempt(attempt DeliveryAttempt) {
	c.logMutex.Lock()
	defer c.logMutex.Unlock()

	c.deliveryLog = append(c.deliveryLog, attempt)

	// Keep only the last 1000 entries to prevent memory leaks
	if len(c.deliveryLog) > 1000 {
		c.deliveryLog = c.deliveryLog[len(c.deliveryLog)-1000:]
	}

	// Log attempt details
	level := "info"
	if attempt.Error != "" {
		level = "error"
	}

	fields := []any{
		"delivery_id", attempt.ID,
		"webhook_url", attempt.WebhookURL,
		"incident_id", attempt.Payload.IncidentID,
		"attempt", attempt.AttemptNumber,
		"status_code", attempt.StatusCode,
		"duration", attempt.Duration,
	}

	if attempt.Error != "" {
		fields = append(fields, "error", attempt.Error)
	}

	switch level {
	case "error":
		c.logger.Error("Webhook delivery attempt failed", fields...)
	default:
		c.logger.Info("Webhook delivery attempt completed", fields...)
	}
}

// GetDeliveryHistory returns recent delivery attempts for audit purposes.
func (c *Client) GetDeliveryHistory(limit int) []DeliveryAttempt {
	c.logMutex.RLock()
	defer c.logMutex.RUnlock()

	if limit <= 0 || limit > len(c.deliveryLog) {
		limit = len(c.deliveryLog)
	}

	// Return the most recent entries
	start := len(c.deliveryLog) - limit
	result := make([]DeliveryAttempt, limit)
	copy(result, c.deliveryLog[start:])

	return result
}

// GetCircuitBreakerStatus returns the status of all circuit breakers.
func (c *Client) GetCircuitBreakerStatus() map[string]CircuitBreakerState {
	c.cbMutex.RLock()
	defer c.cbMutex.RUnlock()

	status := make(map[string]CircuitBreakerState)
	for url, cb := range c.circuitBreakers {
		status[url] = cb.GetState()
	}

	return status
}
