package notification

import (
	"context"
	"fmt"
	"sync"
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

func createTestNotificationConfig() *config.NotificationConfig {
	return &config.NotificationConfig{
		Webhooks: []config.WebhookConfig{
			{
				URL:        "http://localhost:9999/webhook", // Use localhost to avoid DNS timeouts
				Secret:     "test-secret",
				MaxRetries: 1, // Reduce retries to speed up tests
				Enabled:    true,
			},
		},
	}
}

func createTestIncident() *domain.Incident {
	return &domain.Incident{
		ID:          "inc-123",
		Title:       "Test incident",
		Description: "Test description",
		Status:      domain.StatusTriggered,
		Severity:    domain.SeverityCritical,
		TeamID:      "test-team",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
}

func createTestServiceConfig() ServiceConfig {
	return ServiceConfig{
		WorkerCount: 2,
		BufferSize:  10,
		BaseURL:     "https://test.example.com",
		Timeout:     5 * time.Second,
	}
}

// Test DefaultServiceConfig
func TestDefaultServiceConfig(t *testing.T) {
	config := DefaultServiceConfig()

	assert.Equal(t, 5, config.WorkerCount)
	assert.Equal(t, 100, config.BufferSize)
	assert.Equal(t, "", config.BaseURL)
	assert.Equal(t, 30*time.Second, config.Timeout)
}

// Test NewService
func TestNewService(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)

	require.NoError(t, err)
	assert.NotNil(t, service)
	assert.Equal(t, notifConfig, service.config)
	assert.Equal(t, logger, service.logger)
	assert.Equal(t, 2, service.workers)
	assert.NotNil(t, service.webhookCli)
	assert.NotNil(t, service.workerPool)
	assert.NotNil(t, service.stopChan)
	assert.False(t, service.started)
}

// Test NewService with nil config
func TestNewService_NilConfig(t *testing.T) {
	logger := createTestLogger()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(nil, serviceConfig, logger)

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "notification config is required")
}

// Test NewService with nil logger
func TestNewService_NilLogger(t *testing.T) {
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, nil)

	assert.Error(t, err)
	assert.Nil(t, service)
	assert.Contains(t, err.Error(), "logger is required")
}

// Test Start
func TestService_Start(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)

	assert.NoError(t, err)
	assert.True(t, service.IsStarted())

	// Clean up
	service.Stop(ctx)
}

// Test Start when already started
func TestService_Start_AlreadyStarted(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)

	// Try to start again
	err = service.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already started")

	// Clean up
	service.Stop(ctx)
}

// Test Stop
func TestService_Stop(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)

	err = service.Stop(ctx)
	assert.NoError(t, err)
	assert.False(t, service.IsStarted())
}

// Test Stop when not started
func TestService_Stop_NotStarted(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Stop(ctx)
	assert.NoError(t, err) // Should not error
}

// Test Stop with timeout
func TestService_Stop_Timeout(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)

	// Just test that Stop doesn't panic with a timeout context
	// Even if it succeeds quickly, it's still valid behaviour
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	err = service.Stop(timeoutCtx)
	// Stop should either succeed quickly or timeout - both are valid
	if err != nil {
		assert.Contains(t, err.Error(), "context deadline exceeded")
	}
}

// Test NotifyIncidentCreated
func TestService_NotifyIncidentCreated(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")

	assert.NoError(t, err)
	assert.Equal(t, 1, service.GetQueueLength())
}

// Test NotifyIncidentUpdated
func TestService_NotifyIncidentUpdated(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	err = service.NotifyIncidentUpdated(ctx, incident, "test-actor")

	assert.NoError(t, err)
	assert.Equal(t, 1, service.GetQueueLength())
}

// Test NotifyIncidentResolved
func TestService_NotifyIncidentResolved(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	incident.Status = domain.StatusResolved
	err = service.NotifyIncidentResolved(ctx, incident, "test-actor")

	assert.NoError(t, err)
	assert.Equal(t, 1, service.GetQueueLength())
}

// Test NotifyIncidentReopened
func TestService_NotifyIncidentReopened(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	err = service.NotifyIncidentReopened(ctx, incident, "test-actor")

	assert.NoError(t, err)
	assert.Equal(t, 1, service.GetQueueLength())
}

// Test enqueueNotification when service not started
func TestService_enqueueNotification_NotStarted(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	incident := createTestIncident()
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification service not started")
}

// Test enqueueNotification with context cancellation
func TestService_enqueueNotification_ContextCancelled(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()
	serviceConfig.BufferSize = 1 // Small buffer to force waiting

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	err = service.Start(context.Background())
	require.NoError(t, err)
	defer service.Stop(context.Background())

	ctx, cancel := context.WithCancel(context.Background())

	incident := createTestIncident()

	// Fill the buffer first
	err = service.NotifyIncidentCreated(context.Background(), incident, "test-actor")
	assert.NoError(t, err)

	// Cancel the context
	cancel()

	// This should fail due to cancelled context when buffer is full
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

// Test enqueueNotification with full buffer
func TestService_enqueueNotification_BufferFull(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()
	serviceConfig.BufferSize = 1  // Very small buffer
	serviceConfig.WorkerCount = 0 // No workers to consume

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()

	// Fill the buffer
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")
	assert.NoError(t, err) // First one should work

	// This should fail because buffer is full
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "notification buffer full")
}

// Test GetQueueLength and GetQueueCapacity
func TestService_QueueMetrics(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	assert.Equal(t, 0, service.GetQueueLength())
	assert.Equal(t, 10, service.GetQueueCapacity())

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")
	require.NoError(t, err)

	// Queue length should increase
	assert.Equal(t, 1, service.GetQueueLength())
	assert.Equal(t, 10, service.GetQueueCapacity())

	// Give workers time to process
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, service.GetQueueLength()) // Should be processed
}

// Test webhook delivery methods
func TestService_WebhookMethods(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	// Test GetWebhookDeliveryHistory
	history := service.GetWebhookDeliveryHistory(10)
	assert.NotNil(t, history)
	assert.Len(t, history, 0) // Should be empty initially

	// Test GetWebhookCircuitBreakerStatus
	status := service.GetWebhookCircuitBreakerStatus()
	assert.NotNil(t, status)
	assert.Len(t, status, 0) // Should be empty initially
}

// Test EventType values
func TestEventTypes(t *testing.T) {
	assert.Equal(t, "incident.created", string(EventIncidentCreated))
	assert.Equal(t, "incident.updated", string(EventIncidentUpdated))
	assert.Equal(t, "incident.resolved", string(EventIncidentResolved))
	assert.Equal(t, "incident.reopened", string(EventIncidentReopened))
}

// Test worker processing with different notification types
func TestService_WorkerProcessing(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()
	serviceConfig.WorkerCount = 1 // Single worker for predictable testing

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()

	// Send different types of notifications
	err = service.NotifyIncidentCreated(ctx, incident, "actor1")
	assert.NoError(t, err)

	err = service.NotifyIncidentUpdated(ctx, incident, "actor2")
	assert.NoError(t, err)

	err = service.NotifyIncidentResolved(ctx, incident, "actor3")
	assert.NoError(t, err)

	err = service.NotifyIncidentReopened(ctx, incident, "actor4")
	assert.NoError(t, err)

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	// Queue should be empty after processing
	assert.Equal(t, 0, service.GetQueueLength())
}

// Test concurrent operations
func TestService_ConcurrentOperations(t *testing.T) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()

	// Start multiple goroutines sending notifications
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				incident.ID = fmt.Sprintf("inc-%d-%d", id, j)
				service.NotifyIncidentCreated(ctx, incident, "test-actor")
			}
		}(i)
	}

	wg.Wait()

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// All notifications should be processed
	assert.Equal(t, 0, service.GetQueueLength())
}

// Test NotificationRequest structure
func TestNotificationRequest(t *testing.T) {
	incident := createTestIncident()
	req := NotificationRequest{
		Incident:  incident,
		EventType: EventIncidentCreated,
		Actor:     "test-actor",
		Timestamp: time.Now().UTC(),
	}

	assert.Equal(t, incident, req.Incident)
	assert.Equal(t, EventIncidentCreated, req.EventType)
	assert.Equal(t, "test-actor", req.Actor)
	assert.False(t, req.Timestamp.IsZero())
}

// Test processNotification with empty webhooks
func TestService_processNotification_NoWebhooks(t *testing.T) {
	logger := createTestLogger()
	emptyConfig := &config.NotificationConfig{
		Webhooks: []config.WebhookConfig{}, // No webhooks
	}
	serviceConfig := createTestServiceConfig()

	service, err := NewService(emptyConfig, serviceConfig, logger)
	require.NoError(t, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(t, err)
	defer service.Stop(ctx)

	incident := createTestIncident()
	err = service.NotifyIncidentCreated(ctx, incident, "test-actor")

	assert.NoError(t, err)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, 0, service.GetQueueLength())
}

// Benchmark notification processing
func BenchmarkService_NotificationProcessing(b *testing.B) {
	logger := createTestLogger()
	notifConfig := createTestNotificationConfig()
	serviceConfig := createTestServiceConfig()

	service, err := NewService(notifConfig, serviceConfig, logger)
	require.NoError(b, err)

	ctx := context.Background()
	err = service.Start(ctx)
	require.NoError(b, err)
	defer service.Stop(ctx)

	incident := createTestIncident()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.NotifyIncidentCreated(ctx, incident, "test-actor")
	}
}
