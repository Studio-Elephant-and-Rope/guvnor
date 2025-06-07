// Package telemetry provides OpenTelemetry receiver capabilities for the Guvnor incident management platform.
//
// This package implements an OTLP (OpenTelemetry Protocol) receiver that can ingest metrics, traces, and logs
// from OpenTelemetry-compatible sources. The receiver processes incoming telemetry data and extracts signals
// that could indicate incidents, converting them into Guvnor's internal Signal format for further processing.
//
// The receiver supports both gRPC and HTTP transports on configurable ports (default: 4317 for gRPC, 4318 for HTTP).
// Signal processing includes threshold-based detection, error rate analysis, and pattern matching to identify
// potential incidents in the incoming telemetry data.
package telemetry

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/pipeline"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/config"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// SignalProcessor defines the interface for processing telemetry signals into incidents.
// This interface allows for pluggable signal processing strategies and testing.
type SignalProcessor interface {
	// ProcessMetrics extracts signals from OpenTelemetry metrics data
	ProcessMetrics(ctx context.Context, metrics pmetric.Metrics) ([]domain.Signal, error)
	// ProcessTraces extracts signals from OpenTelemetry traces data
	ProcessTraces(ctx context.Context, traces ptrace.Traces) ([]domain.Signal, error)
	// ProcessLogs extracts signals from OpenTelemetry logs data
	ProcessLogs(ctx context.Context, logs plog.Logs) ([]domain.Signal, error)
}

// SignalHandler defines the interface for handling processed signals.
// This allows the receiver to delegate signal processing to the appropriate service layer.
type SignalHandler interface {
	// HandleSignal processes a signal and potentially creates an incident
	HandleSignal(ctx context.Context, signal domain.Signal) error
}

// Receiver wraps the OpenTelemetry OTLP receiver with signal processing capabilities.
//
// The receiver acts as a bridge between OpenTelemetry data and Guvnor's incident management,
// processing incoming telemetry and extracting actionable signals for incident detection.
type Receiver struct {
	config          *config.ReceiverConfig
	logger          *logging.Logger
	processor       SignalProcessor
	handler         SignalHandler
	tracesReceiver  receiver.Traces
	metricsReceiver receiver.Metrics
	logsReceiver    receiver.Logs
	isRunning       bool
	componentHost   component.Host
}

// consumerWrapper implements the OpenTelemetry consumer interfaces and delegates to our signal processing.
type consumerWrapper struct {
	receiver *Receiver
}

// NewReceiver creates a new OTLP receiver with signal processing capabilities.
//
// The receiver requires configuration for ports and hosts, a logger for observability,
// a signal processor for extracting incidents from telemetry, and a handler for processing
// the extracted signals.
//
// Returns an error if the configuration is invalid or if the OTLP receiver cannot be created.
func NewReceiver(
	cfg *config.ReceiverConfig,
	logger *logging.Logger,
	processor SignalProcessor,
	handler SignalHandler,
) (*Receiver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("receiver configuration is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	if processor == nil {
		return nil, fmt.Errorf("signal processor is required")
	}

	if handler == nil {
		return nil, fmt.Errorf("signal handler is required")
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid receiver configuration: %w", err)
	}

	return &Receiver{
		config:    cfg,
		logger:    logger,
		processor: processor,
		handler:   handler,
		isRunning: false,
	}, nil
}

// Start starts the OTLP receiver on the configured ports.
//
// This method creates and starts the OpenTelemetry collector's OTLP receiver,
// configuring it to listen on both gRPC and HTTP endpoints. The receiver
// will begin accepting telemetry data and processing it for signals.
//
// Returns an error if the receiver is already running or if startup fails.
func (r *Receiver) Start(ctx context.Context) error {
	if r.isRunning {
		return fmt.Errorf("receiver is already running")
	}

	if !r.config.Enabled {
		r.logger.Info("OTLP receiver is disabled in configuration")
		return nil
	}

	r.logger.Info("Starting OTLP receiver",
		"grpc_endpoint", net.JoinHostPort(r.config.GRPCHost, strconv.Itoa(r.config.GRPCPort)),
		"http_endpoint", net.JoinHostPort(r.config.HTTPHost, strconv.Itoa(r.config.HTTPPort)),
	)

	// Create component host for the receiver
	host, err := r.createComponentHost()
	if err != nil {
		return fmt.Errorf("failed to create component host: %w", err)
	}
	r.componentHost = host

	// Create OTLP receiver configuration
	receiverConfig, err := r.createOTLPConfig()
	if err != nil {
		return fmt.Errorf("failed to create OTLP configuration: %w", err)
	}

	// Create receiver factory
	factory := otlpreceiver.NewFactory()

	// Create consumer wrapper
	consumer := &consumerWrapper{receiver: r}

	// Create the OTLP receivers for all signal types
	receiverSettings := receiver.Settings{
		ID:        component.NewID(component.MustNewType("otlp")),
		BuildInfo: component.NewDefaultBuildInfo(),
		TelemetrySettings: component.TelemetrySettings{
			Logger: nil, // Let the collector use its own logger
		},
	}

	// Create traces receiver
	tracesReceiver, err := factory.CreateTraces(ctx, receiverSettings, receiverConfig, consumer)
	if err != nil {
		return fmt.Errorf("failed to create OTLP traces receiver: %w", err)
	}
	r.tracesReceiver = tracesReceiver

	// Create metrics receiver
	metricsReceiver, err := factory.CreateMetrics(ctx, receiverSettings, receiverConfig, consumer)
	if err != nil {
		return fmt.Errorf("failed to create OTLP metrics receiver: %w", err)
	}
	r.metricsReceiver = metricsReceiver

	// Create logs receiver
	logsReceiver, err := factory.CreateLogs(ctx, receiverSettings, receiverConfig, consumer)
	if err != nil {
		return fmt.Errorf("failed to create OTLP logs receiver: %w", err)
	}
	r.logsReceiver = logsReceiver

	// Start all receivers
	if err := r.tracesReceiver.Start(ctx, r.componentHost); err != nil {
		return fmt.Errorf("failed to start OTLP traces receiver: %w", err)
	}
	if err := r.metricsReceiver.Start(ctx, r.componentHost); err != nil {
		return fmt.Errorf("failed to start OTLP metrics receiver: %w", err)
	}
	if err := r.logsReceiver.Start(ctx, r.componentHost); err != nil {
		return fmt.Errorf("failed to start OTLP logs receiver: %w", err)
	}

	r.isRunning = true
	r.logger.Info("OTLP receiver started successfully")

	return nil
}

// Stop gracefully shuts down the OTLP receiver.
//
// This method stops the OpenTelemetry collector's OTLP receiver, ensuring
// all in-flight requests are completed before shutdown.
//
// Returns an error if shutdown fails.
func (r *Receiver) Stop(ctx context.Context) error {
	if !r.isRunning {
		return nil
	}

	r.logger.Info("Stopping OTLP receivers")

	// Stop all receivers, collecting any errors
	var errors []error

	if r.tracesReceiver != nil {
		if err := r.tracesReceiver.Shutdown(ctx); err != nil {
			r.logger.Error("Error shutting down OTLP traces receiver", "error", err)
			errors = append(errors, fmt.Errorf("failed to shutdown traces receiver: %w", err))
		}
	}

	if r.metricsReceiver != nil {
		if err := r.metricsReceiver.Shutdown(ctx); err != nil {
			r.logger.Error("Error shutting down OTLP metrics receiver", "error", err)
			errors = append(errors, fmt.Errorf("failed to shutdown metrics receiver: %w", err))
		}
	}

	if r.logsReceiver != nil {
		if err := r.logsReceiver.Shutdown(ctx); err != nil {
			r.logger.Error("Error shutting down OTLP logs receiver", "error", err)
			errors = append(errors, fmt.Errorf("failed to shutdown logs receiver: %w", err))
		}
	}

	// Return combined error if any shutdowns failed
	if len(errors) > 0 {
		return fmt.Errorf("shutdown errors: %v", errors)
	}

	r.isRunning = false
	r.logger.Info("OTLP receiver stopped successfully")

	return nil
}

// IsRunning returns whether the receiver is currently running.
func (r *Receiver) IsRunning() bool {
	return r.isRunning
}

// createComponentHost creates a minimal component host for the OTLP receiver.
func (r *Receiver) createComponentHost() (component.Host, error) {
	return &componentHost{
		logger: r.logger,
	}, nil
}

// createOTLPConfig creates the configuration for the OTLP receiver.
func (r *Receiver) createOTLPConfig() (component.Config, error) {
	// Create confmap with receiver configuration using string map
	grpcEndpoint := net.JoinHostPort(r.config.GRPCHost, strconv.Itoa(r.config.GRPCPort))
	httpEndpoint := net.JoinHostPort(r.config.HTTPHost, strconv.Itoa(r.config.HTTPPort))

	configData := map[string]any{
		"protocols": map[string]any{
			"grpc": map[string]any{
				"endpoint": grpcEndpoint,
			},
			"http": map[string]any{
				"endpoint": httpEndpoint,
			},
		},
	}

	configMap := confmap.NewFromStringMap(configData)

	// Create and unmarshal configuration
	factory := otlpreceiver.NewFactory()
	config := factory.CreateDefaultConfig()

	if err := configMap.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal OTLP config: %w", err)
	}

	return config, nil
}

// componentHost implements a minimal component.Host for the OTLP receiver.
type componentHost struct {
	logger *logging.Logger
}

// GetFactory implements component.Host.
func (h *componentHost) GetFactory(kind component.Kind, componentType component.Type) component.Factory {
	return nil
}

// GetExtensions implements component.Host.
func (h *componentHost) GetExtensions() map[component.ID]component.Component {
	return make(map[component.ID]component.Component)
}

// GetExporters implements component.Host.
func (h *componentHost) GetExporters() map[pipeline.Signal]map[component.ID]component.Component {
	return make(map[pipeline.Signal]map[component.ID]component.Component)
}

// GetProcessors implements component.Host.
func (h *componentHost) GetProcessors() map[pipeline.Signal]map[component.ID]component.Component {
	return make(map[pipeline.Signal]map[component.ID]component.Component)
}

// GetReceivers implements component.Host.
func (h *componentHost) GetReceivers() map[pipeline.Signal]map[component.ID]component.Component {
	return make(map[pipeline.Signal]map[component.ID]component.Component)
}

// ReportFatalError implements component.Host.
func (h *componentHost) ReportFatalError(err error) {
	h.logger.Error("Fatal error in component", "error", err)
}

// ReportComponentStatus implements component.Host.
func (h *componentHost) ReportComponentStatus(id component.ID, event *componentstatus.Event) {
	h.logger.Debug("Component status change",
		"component_id", id.String(),
		"status", event.Status().String(),
		"timestamp", event.Timestamp(),
	)
}

// Consumer interface implementations for consumerWrapper

// Capabilities implements consumer.Consumer.
func (c *consumerWrapper) Capabilities() consumer.Capabilities {
	return consumer.Capabilities{MutatesData: false}
}

// ConsumeTraces implements consumer.Traces.
func (c *consumerWrapper) ConsumeTraces(ctx context.Context, traces ptrace.Traces) error {
	signals, err := c.receiver.processor.ProcessTraces(ctx, traces)
	if err != nil {
		c.receiver.logger.Error("Failed to process traces", "error", err)
		return fmt.Errorf("failed to process traces: %w", err)
	}

	for _, signal := range signals {
		if err := c.receiver.handler.HandleSignal(ctx, signal); err != nil {
			c.receiver.logger.Error("Failed to handle signal", "signal_id", signal.ID, "error", err)
			// Continue processing other signals even if one fails
		}
	}

	c.receiver.logger.Debug("Processed traces", "trace_count", traces.SpanCount(), "signals_extracted", len(signals))
	return nil
}

// ConsumeMetrics implements consumer.Metrics.
func (c *consumerWrapper) ConsumeMetrics(ctx context.Context, metrics pmetric.Metrics) error {
	signals, err := c.receiver.processor.ProcessMetrics(ctx, metrics)
	if err != nil {
		c.receiver.logger.Error("Failed to process metrics", "error", err)
		return fmt.Errorf("failed to process metrics: %w", err)
	}

	for _, signal := range signals {
		if err := c.receiver.handler.HandleSignal(ctx, signal); err != nil {
			c.receiver.logger.Error("Failed to handle signal", "signal_id", signal.ID, "error", err)
			// Continue processing other signals even if one fails
		}
	}

	c.receiver.logger.Debug("Processed metrics", "metric_count", metrics.MetricCount(), "signals_extracted", len(signals))
	return nil
}

// ConsumeLogs implements consumer.Logs.
func (c *consumerWrapper) ConsumeLogs(ctx context.Context, logs plog.Logs) error {
	signals, err := c.receiver.processor.ProcessLogs(ctx, logs)
	if err != nil {
		c.receiver.logger.Error("Failed to process logs", "error", err)
		return fmt.Errorf("failed to process logs: %w", err)
	}

	for _, signal := range signals {
		if err := c.receiver.handler.HandleSignal(ctx, signal); err != nil {
			c.receiver.logger.Error("Failed to handle signal", "signal_id", signal.ID, "error", err)
			// Continue processing other signals even if one fails
		}
	}

	c.receiver.logger.Debug("Processed logs", "log_count", logs.LogRecordCount(), "signals_extracted", len(signals))
	return nil
}
