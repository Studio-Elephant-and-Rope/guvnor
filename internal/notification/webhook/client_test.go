package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// Test helpers
func createTestLogger() *logging.Logger {
	cfg := logging.DefaultConfig(logging.Test)
	logger, _ := logging.NewLogger(cfg)
	return logger
}

func createTestIncident() *domain.Incident {
	return &domain.Incident{
		ID:          "inc-123",
		Title:       "Database connection timeout",
		Description: "Users experiencing timeouts",
		Status:      domain.StatusTriggered,
		Severity:    domain.SeverityCritical,
		TeamID:      "backend-team",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func createTestWebhookConfig(url string) config.WebhookConfig {
	return config.WebhookConfig{
		URL:        url,
		Secret:     "test-secret",
		Headers:    map[string]string{"X-Custom": "test"},
		Timeout:    "30s",
		MaxRetries: 3,
		Enabled:    true,
	}
}

// Test WebhookPayload creation
func TestClient_createPayload(t *testing.T) {
	logger := createTestLogger()
	client := NewClient(ClientConfig{BaseURL: "https://guvnor.example.com"}, logger)

	incident := createTestIncident()
	payload := client.createPayload(incident)

	assert.Equal(t, incident.ID, payload.IncidentID)
	assert.Equal(t, incident.Title, payload.Title)
	assert.Equal(t, string(incident.Status), payload.Status)
	assert.Equal(t, "SEV1", payload.Severity) // Critical maps to SEV1
	assert.Equal(t, incident.UpdatedAt, payload.Timestamp)
	assert.Equal(t, "https://guvnor.example.com/incidents/inc-123", payload.URL)
}

// Test severity mapping
func TestClient_mapSeverityToSEV(t *testing.T) {
	logger := createTestLogger()
	client := NewClient(ClientConfig{}, logger)

	tests := []struct {
		input    domain.Severity
		expected string
	}{
		{domain.SeverityCritical, "SEV1"},
		{domain.SeverityHigh, "SEV2"},
		{domain.SeverityMedium, "SEV3"},
		{domain.SeverityLow, "SEV4"},
		{domain.SeverityInfo, "SEV5"},
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			result := client.mapSeverityToSEV(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Test HMAC signature generation
func TestClient_generateHMACSignature(t *testing.T) {
	logger := createTestLogger()
	client := NewClient(ClientConfig{}, logger)

	payload := []byte(`{"test": "data"}`)
	secret := "my-secret"

	signature := client.generateHMACSignature(payload, secret)

	// Verify format
	assert.True(t, strings.HasPrefix(signature, "sha256="))

	// Verify signature is correct
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(payload)
	expected := "sha256=" + hex.EncodeToString(h.Sum(nil))
	assert.Equal(t, expected, signature)
}

// Test successful webhook delivery
func TestClient_SendNotification_Success(t *testing.T) {
	logger := createTestLogger()

	// Create test server
	receivedPayloads := []WebhookPayload{}
	receivedHeaders := []http.Header{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record headers
		receivedHeaders = append(receivedHeaders, r.Header.Clone())

		// Decode payload
		var payload WebhookPayload
		err := json.NewDecoder(r.Body).Decode(&payload)
		require.NoError(t, err)
		receivedPayloads = append(receivedPayloads, payload)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status": "received"}`))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)

	assert.NoError(t, err)
	assert.Len(t, receivedPayloads, 1)
	assert.Equal(t, incident.ID, receivedPayloads[0].IncidentID)

	// Verify headers
	assert.Len(t, receivedHeaders, 1)
	headers := receivedHeaders[0]
	assert.Equal(t, "application/json", headers.Get("Content-Type"))
	assert.Equal(t, "Guvnor-Webhook/1.0", headers.Get("User-Agent"))
	assert.Equal(t, "test", headers.Get("X-Custom"))
	assert.NotEmpty(t, headers.Get("X-Guvnor-Signature"))
}

// Test webhook delivery with retry
func TestClient_SendNotification_Retry(t *testing.T) {
	logger := createTestLogger()

	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "received"}`))
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)

	assert.NoError(t, err)
	assert.Equal(t, 3, attemptCount) // Should retry twice, then succeed
}

// Test webhook delivery failure after max retries
func TestClient_SendNotification_MaxRetriesExceeded(t *testing.T) {
	logger := createTestLogger()

	attemptCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attemptCount++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed after 3 attempts")
	assert.Equal(t, 3, attemptCount) // Should try 3 times then fail
}

// Test circuit breaker functionality
func TestCircuitBreaker_States(t *testing.T) {
	cb := NewCircuitBreaker()

	// Initially closed
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
	assert.True(t, cb.CanExecute())

	// Record failures to open circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}

	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	// Record success to transition to half-open
	cb.RecordSuccess()
	assert.Equal(t, CircuitBreakerOpen, cb.GetState()) // Still open until reset timeout
}

// Test circuit breaker with failing endpoint
func TestClient_SendNotification_CircuitBreaker(t *testing.T) {
	logger := createTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	// Send two requests to accumulate enough failures (each request tries 3 times)
	// First request: 3 failures
	err1 := client.SendNotification(context.Background(), incident, webhooks)
	assert.Error(t, err1)

	// Second request: 2 more failures should open the circuit (total 5)
	webhook2 := createTestWebhookConfig(server.URL)
	webhook2.MaxRetries = 2 // Only 2 failures needed to reach the 5 total
	webhooks2 := []config.WebhookConfig{webhook2}

	err2 := client.SendNotification(context.Background(), incident, webhooks2)
	assert.Error(t, err2)

	// Circuit breaker should now be open
	cb := client.getCircuitBreaker(server.URL)
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
}

// Test multiple webhook endpoints
func TestClient_SendNotification_MultipleEndpoints(t *testing.T) {
	logger := createTestLogger()

	// Server 1: Always succeeds
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	// Server 2: Always fails
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server2.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{
		createTestWebhookConfig(server1.URL),
		createTestWebhookConfig(server2.URL),
	}

	// Should succeed because at least one endpoint works
	err := client.SendNotification(context.Background(), incident, webhooks)
	assert.NoError(t, err)
}

// Test disabled webhook endpoint
func TestClient_SendNotification_DisabledEndpoint(t *testing.T) {
	logger := createTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("Should not be called for disabled endpoint")
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()

	webhook := createTestWebhookConfig(server.URL)
	webhook.Enabled = false
	webhooks := []config.WebhookConfig{webhook}

	err := client.SendNotification(context.Background(), incident, webhooks)
	assert.NoError(t, err) // Should succeed but not call the endpoint
}

// Test context cancellation
func TestClient_SendNotification_ContextCancelled(t *testing.T) {
	logger := createTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond) // Delay to allow cancellation
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := client.SendNotification(ctx, incident, webhooks)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

// Test delivery history audit log
func TestClient_DeliveryHistory(t *testing.T) {
	logger := createTestLogger()

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)
	assert.NoError(t, err)

	// Check delivery history
	history := client.GetDeliveryHistory(10)
	assert.Len(t, history, 3) // 2 failures + 1 success

	// First two should be failures
	assert.NotEmpty(t, history[0].Error)
	assert.NotEmpty(t, history[1].Error)
	assert.Equal(t, 500, history[0].StatusCode)
	assert.Equal(t, 500, history[1].StatusCode)

	// Last one should be success
	assert.Empty(t, history[2].Error)
	assert.Equal(t, 200, history[2].StatusCode)

	// Check attempt numbers
	assert.Equal(t, 1, history[0].AttemptNumber)
	assert.Equal(t, 2, history[1].AttemptNumber)
	assert.Equal(t, 3, history[2].AttemptNumber)
}

// Test HMAC signature verification
func TestClient_HMACSignatureVerification(t *testing.T) {
	logger := createTestLogger()

	var receivedSignature string
	var receivedPayload []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSignature = r.Header.Get("X-Guvnor-Signature")

		// Read the body
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		receivedPayload = body

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)
	assert.NoError(t, err)

	// Verify signature was sent
	assert.NotEmpty(t, receivedSignature)
	assert.True(t, strings.HasPrefix(receivedSignature, "sha256="))

	// Verify signature is correct
	expectedSignature := client.generateHMACSignature(receivedPayload, "test-secret")
	assert.Equal(t, expectedSignature, receivedSignature)
}

// Test NewClient with different configurations
func TestNewClient(t *testing.T) {
	logger := createTestLogger()

	t.Run("default timeout", func(t *testing.T) {
		client := NewClient(ClientConfig{}, logger)
		assert.Equal(t, 30*time.Second, client.httpClient.Timeout)
	})

	t.Run("custom timeout", func(t *testing.T) {
		client := NewClient(ClientConfig{Timeout: 60 * time.Second}, logger)
		assert.Equal(t, 60*time.Second, client.httpClient.Timeout)
	})

	t.Run("custom base URL", func(t *testing.T) {
		client := NewClient(ClientConfig{BaseURL: "https://custom.example.com"}, logger)
		assert.Equal(t, "https://custom.example.com", client.baseURL)
	})
}

// Test URL creation
func TestClient_createIncidentURL(t *testing.T) {
	logger := createTestLogger()

	t.Run("with base URL", func(t *testing.T) {
		client := NewClient(ClientConfig{BaseURL: "https://guvnor.example.com"}, logger)
		url := client.createIncidentURL("inc-123")
		assert.Equal(t, "https://guvnor.example.com/incidents/inc-123", url)
	})

	t.Run("without base URL", func(t *testing.T) {
		client := NewClient(ClientConfig{}, logger)
		url := client.createIncidentURL("inc-123")
		assert.Equal(t, "https://guvnor.local/incidents/inc-123", url)
	})

	t.Run("with invalid base URL", func(t *testing.T) {
		client := NewClient(ClientConfig{BaseURL: "://invalid"}, logger)
		url := client.createIncidentURL("inc-123")
		assert.Equal(t, "https://guvnor.local/incidents/inc-123", url)
	})
}

// Test circuit breaker state transitions
func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker()

	// Start closed
	assert.Equal(t, CircuitBreakerClosed, cb.GetState())
	assert.True(t, cb.CanExecute())

	// Record failures
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
		assert.Equal(t, CircuitBreakerClosed, cb.GetState())
	}

	// 5th failure should open circuit
	cb.RecordFailure()
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	// Should stay open even with success
	cb.RecordSuccess()
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
}

// Test circuit breaker reset
func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker()
	cb.resetTimeout = 100 * time.Millisecond

	// Open the circuit
	for i := 0; i < 5; i++ {
		cb.RecordFailure()
	}
	assert.Equal(t, CircuitBreakerOpen, cb.GetState())
	assert.False(t, cb.CanExecute())

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)
	assert.True(t, cb.CanExecute()) // Should allow execution after timeout
}

// Test circuit breaker status retrieval
func TestClient_GetCircuitBreakerStatus(t *testing.T) {
	logger := createTestLogger()
	client := NewClient(ClientConfig{}, logger)

	// Initially no circuit breakers
	status := client.GetCircuitBreakerStatus()
	assert.Empty(t, status)

	// Create circuit breakers by getting them
	client.getCircuitBreaker("http://example1.com")
	cb2 := client.getCircuitBreaker("http://example2.com")

	// Open one circuit breaker
	for i := 0; i < 5; i++ {
		cb2.RecordFailure()
	}

	status = client.GetCircuitBreakerStatus()
	assert.Len(t, status, 2)
	assert.Equal(t, CircuitBreakerClosed, status["http://example1.com"])
	assert.Equal(t, CircuitBreakerOpen, status["http://example2.com"])
}

// Test no webhooks configured
func TestClient_SendNotification_NoWebhooks(t *testing.T) {
	logger := createTestLogger()
	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()

	err := client.SendNotification(context.Background(), incident, []config.WebhookConfig{})
	assert.NoError(t, err)
}

// Test all endpoints fail
func TestClient_SendNotification_AllEndpointsFail(t *testing.T) {
	logger := createTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	err := client.SendNotification(context.Background(), incident, webhooks)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to send notification to any webhook endpoint")
}

// Benchmark webhook delivery
func BenchmarkClient_SendNotification(b *testing.B) {
	logger := createTestLogger()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(ClientConfig{}, logger)
	incident := createTestIncident()
	webhooks := []config.WebhookConfig{createTestWebhookConfig(server.URL)}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		client.SendNotification(context.Background(), incident, webhooks)
	}
}
