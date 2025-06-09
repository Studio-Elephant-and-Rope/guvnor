// Package notification provides incident notification functionality.
//
// The service handles async delivery of notifications to various channels
// (webhooks, email, Slack, etc.) when incidents are created or updated.
// It implements worker pools for high-volume environments and provides
// comprehensive audit logging for compliance and debugging.
package notification

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/notification/webhook"
)

// NotificationRequest represents a request to send notifications for an incident.
type NotificationRequest struct {
	Incident  *domain.Incident
	EventType EventType
	Actor     string
	Timestamp time.Time
}

// EventType represents the type of incident event that triggers notifications.
type EventType string

const (
	// EventIncidentCreated is triggered when a new incident is created
	EventIncidentCreated EventType = "incident.created"
	// EventIncidentUpdated is triggered when an incident is updated
	EventIncidentUpdated EventType = "incident.updated"
	// EventIncidentResolved is triggered when an incident is resolved
	EventIncidentResolved EventType = "incident.resolved"
	// EventIncidentReopened is triggered when a resolved incident is reopened
	EventIncidentReopened EventType = "incident.reopened"
)

// Service provides notification functionality for incidents.
type Service struct {
	config     *config.NotificationConfig
	logger     *logging.Logger
	webhookCli *webhook.Client
	workerPool chan NotificationRequest
	workers    int
	stopChan   chan struct{}
	wg         sync.WaitGroup
	started    bool
	mu         sync.RWMutex
}

// ServiceConfig contains configuration for the notification service.
type ServiceConfig struct {
	WorkerCount int
	BufferSize  int
	BaseURL     string
	Timeout     time.Duration
}

// DefaultServiceConfig returns default configuration for the notification service.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		WorkerCount: 5,
		BufferSize:  100,
		BaseURL:     "",
		Timeout:     30 * time.Second,
	}
}

// NewService creates a new notification service.
func NewService(notificationConfig *config.NotificationConfig, serviceConfig ServiceConfig, logger *logging.Logger) (*Service, error) {
	if notificationConfig == nil {
		return nil, fmt.Errorf("notification config is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Create webhook client
	webhookClient := webhook.NewClient(webhook.ClientConfig{
		BaseURL: serviceConfig.BaseURL,
		Timeout: serviceConfig.Timeout,
	}, logger)

	// Create worker pool channel
	workerPool := make(chan NotificationRequest, serviceConfig.BufferSize)

	service := &Service{
		config:     notificationConfig,
		logger:     logger,
		webhookCli: webhookClient,
		workerPool: workerPool,
		workers:    serviceConfig.WorkerCount,
		stopChan:   make(chan struct{}),
	}

	return service, nil
}

// Start begins processing notifications asynchronously.
func (s *Service) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("notification service is already started")
	}

	s.logger.Info("Starting notification service",
		"workers", s.workers,
		"buffer_size", cap(s.workerPool))

	// Start worker goroutines
	for i := 0; i < s.workers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}

	s.started = true
	return nil
}

// Stop gracefully shuts down the notification service.
func (s *Service) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	s.logger.Info("Stopping notification service")

	// Signal workers to stop
	close(s.stopChan)

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Notification service stopped gracefully")
	case <-ctx.Done():
		s.logger.Warn("Notification service shutdown timeout exceeded")
		return ctx.Err()
	}

	s.started = false
	return nil
}

// NotifyIncidentCreated sends notifications when an incident is created.
func (s *Service) NotifyIncidentCreated(ctx context.Context, incident *domain.Incident, actor string) error {
	return s.enqueueNotification(ctx, NotificationRequest{
		Incident:  incident,
		EventType: EventIncidentCreated,
		Actor:     actor,
		Timestamp: time.Now().UTC(),
	})
}

// NotifyIncidentUpdated sends notifications when an incident is updated.
func (s *Service) NotifyIncidentUpdated(ctx context.Context, incident *domain.Incident, actor string) error {
	return s.enqueueNotification(ctx, NotificationRequest{
		Incident:  incident,
		EventType: EventIncidentUpdated,
		Actor:     actor,
		Timestamp: time.Now().UTC(),
	})
}

// NotifyIncidentResolved sends notifications when an incident is resolved.
func (s *Service) NotifyIncidentResolved(ctx context.Context, incident *domain.Incident, actor string) error {
	return s.enqueueNotification(ctx, NotificationRequest{
		Incident:  incident,
		EventType: EventIncidentResolved,
		Actor:     actor,
		Timestamp: time.Now().UTC(),
	})
}

// NotifyIncidentReopened sends notifications when an incident is reopened.
func (s *Service) NotifyIncidentReopened(ctx context.Context, incident *domain.Incident, actor string) error {
	return s.enqueueNotification(ctx, NotificationRequest{
		Incident:  incident,
		EventType: EventIncidentReopened,
		Actor:     actor,
		Timestamp: time.Now().UTC(),
	})
}

// enqueueNotification adds a notification request to the worker pool.
func (s *Service) enqueueNotification(ctx context.Context, req NotificationRequest) error {
	s.mu.RLock()
	started := s.started
	s.mu.RUnlock()

	if !started {
		s.logger.Warn("Notification service not started, dropping notification",
			"incident_id", req.Incident.ID,
			"event_type", req.EventType)
		return fmt.Errorf("notification service not started")
	}

	select {
	case s.workerPool <- req:
		s.logger.Debug("Notification enqueued",
			"incident_id", req.Incident.ID,
			"event_type", req.EventType,
			"actor", req.Actor)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		// Buffer is full, log and drop the notification
		s.logger.Error("Notification buffer full, dropping notification",
			"incident_id", req.Incident.ID,
			"event_type", req.EventType,
			"buffer_size", cap(s.workerPool))
		return fmt.Errorf("notification buffer full")
	}
}

// worker processes notifications from the worker pool.
func (s *Service) worker(ctx context.Context, workerID int) {
	defer s.wg.Done()

	workerLogger := s.logger.WithFields("worker_id", workerID)
	workerLogger.Debug("Notification worker started")

	for {
		select {
		case req := <-s.workerPool:
			s.processNotification(ctx, req, workerLogger)
		case <-s.stopChan:
			workerLogger.Debug("Notification worker stopping")
			return
		case <-ctx.Done():
			workerLogger.Debug("Notification worker context cancelled")
			return
		}
	}
}

// processNotification handles the actual notification delivery.
func (s *Service) processNotification(ctx context.Context, req NotificationRequest, logger *logging.Logger) {
	startTime := time.Now()
	logger = logger.WithFields(
		"incident_id", req.Incident.ID,
		"event_type", req.EventType,
		"actor", req.Actor,
	)

	logger.Info("Processing notification")

	// Send webhook notifications
	if len(s.config.Webhooks) > 0 {
		err := s.webhookCli.SendNotification(ctx, req.Incident, s.config.Webhooks)
		if err != nil {
			logger.Error("Failed to send webhook notifications",
				"error", err,
				"duration", time.Since(startTime))
		} else {
			logger.Info("Webhook notifications sent successfully",
				"duration", time.Since(startTime))
		}
	} else {
		logger.Debug("No webhook endpoints configured")
	}

	// TODO: Add other notification channels (email, Slack, etc.)
	// Future implementation could include:
	// - Email notifications
	// - Slack/Teams integrations
	// - SMS notifications
	// - PagerDuty integration

	logger.Info("Notification processing completed",
		"duration", time.Since(startTime))
}

// GetWebhookDeliveryHistory returns recent webhook delivery attempts for debugging.
func (s *Service) GetWebhookDeliveryHistory(limit int) []webhook.DeliveryAttempt {
	return s.webhookCli.GetDeliveryHistory(limit)
}

// GetWebhookCircuitBreakerStatus returns the status of webhook circuit breakers.
func (s *Service) GetWebhookCircuitBreakerStatus() map[string]webhook.CircuitBreakerState {
	return s.webhookCli.GetCircuitBreakerStatus()
}

// IsStarted returns whether the notification service is currently running.
func (s *Service) IsStarted() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.started
}

// GetQueueLength returns the current number of pending notifications.
func (s *Service) GetQueueLength() int {
	return len(s.workerPool)
}

// GetQueueCapacity returns the maximum capacity of the notification queue.
func (s *Service) GetQueueCapacity() int {
	return cap(s.workerPool)
}
