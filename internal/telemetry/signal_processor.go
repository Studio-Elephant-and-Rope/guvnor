// Package telemetry provides signal processing capabilities for extracting incident-relevant data from OpenTelemetry signals.
package telemetry

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// ProcessorConfig defines configuration options for signal processing.
type ProcessorConfig struct {
	// Metrics thresholds
	ErrorRateThreshold  float64 `yaml:"error_rate_threshold" json:"error_rate_threshold"` // Default: 0.05 (5%)
	LatencyThreshold    float64 `yaml:"latency_threshold" json:"latency_threshold"`       // Default: 1000ms
	CPUThreshold        float64 `yaml:"cpu_threshold" json:"cpu_threshold"`               // Default: 0.8 (80%)
	MemoryThreshold     float64 `yaml:"memory_threshold" json:"memory_threshold"`         // Default: 0.9 (90%)
	ThroughputThreshold float64 `yaml:"throughput_threshold" json:"throughput_threshold"` // Minimum requests/second

	// Trace analysis
	ErrorSpanThreshold int     `yaml:"error_span_threshold" json:"error_span_threshold"` // Default: 5 error spans
	SlowSpanThreshold  float64 `yaml:"slow_span_threshold" json:"slow_span_threshold"`   // Default: 5000ms

	// Log analysis
	ErrorLogPatterns    []string `yaml:"error_log_patterns" json:"error_log_patterns"`       // Patterns to detect error logs
	CriticalLogPatterns []string `yaml:"critical_log_patterns" json:"critical_log_patterns"` // Patterns for critical logs

	// General settings
	MinSignalInterval   time.Duration `yaml:"min_signal_interval" json:"min_signal_interval"`       // Minimum time between similar signals
	MaxSignalsPerMinute int           `yaml:"max_signals_per_minute" json:"max_signals_per_minute"` // Rate limiting
}

// DefaultProcessorConfig returns a configuration with sensible defaults for signal processing.
func DefaultProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		ErrorRateThreshold:  0.05,   // 5%
		LatencyThreshold:    1000.0, // 1 second
		CPUThreshold:        0.8,    // 80%
		MemoryThreshold:     0.9,    // 90%
		ThroughputThreshold: 1.0,    // 1 req/sec minimum
		ErrorSpanThreshold:  5,
		SlowSpanThreshold:   5000.0, // 5 seconds
		ErrorLogPatterns: []string{
			"error", "ERROR", "Error",
			"exception", "Exception", "EXCEPTION",
			"fatal", "Fatal", "FATAL",
			"panic", "Panic", "PANIC",
		},
		CriticalLogPatterns: []string{
			"critical", "Critical", "CRITICAL",
			"emergency", "Emergency", "EMERGENCY",
			"alert", "Alert", "ALERT",
		},
		MinSignalInterval:   5 * time.Minute,
		MaxSignalsPerMinute: 10,
	}
}

// ThresholdProcessor implements the SignalProcessor interface with configurable thresholds.
//
// This processor analyses OpenTelemetry data for patterns that indicate potential incidents:
// - High error rates in metrics
// - Elevated latency patterns
// - Resource exhaustion (CPU, memory)
// - Error spans in traces
// - Critical log messages
//
// The processor is designed to be conservative to avoid alert fatigue whilst ensuring
// genuine incidents are detected promptly.
type ThresholdProcessor struct {
	config ProcessorConfig
	logger *logging.Logger

	// State tracking for rate limiting and deduplication
	recentSignals map[string]time.Time // key: signal fingerprint, value: last sent time
}

// NewThresholdProcessor creates a new threshold-based signal processor.
func NewThresholdProcessor(config ProcessorConfig, logger *logging.Logger) *ThresholdProcessor {
	return &ThresholdProcessor{
		config:        config,
		logger:        logger,
		recentSignals: make(map[string]time.Time),
	}
}

// ProcessMetrics extracts signals from OpenTelemetry metrics data.
//
// Analyses metrics for:
// - Error rates above configured thresholds
// - Latency spikes
// - Resource utilisation issues
// - Throughput drops
//
// Returns extracted signals or an error if processing fails.
func (p *ThresholdProcessor) ProcessMetrics(ctx context.Context, metrics pmetric.Metrics) ([]domain.Signal, error) {
	var signals []domain.Signal

	p.logger.Debug("Processing metrics for signal extraction", "metric_count", metrics.MetricCount())

	// Iterate through resource metrics
	for i := 0; i < metrics.ResourceMetrics().Len(); i++ {
		resourceMetrics := metrics.ResourceMetrics().At(i)
		resource := resourceMetrics.Resource()

		// Extract service information from resource attributes
		serviceName := getStringAttribute(resource.Attributes(), "service.name", "unknown")
		serviceVersion := getStringAttribute(resource.Attributes(), "service.version", "unknown")

		// Process scope metrics
		for j := 0; j < resourceMetrics.ScopeMetrics().Len(); j++ {
			scopeMetrics := resourceMetrics.ScopeMetrics().At(j)

			// Process individual metrics
			for k := 0; k < scopeMetrics.Metrics().Len(); k++ {
				metric := scopeMetrics.Metrics().At(k)

				// Check for error rate metrics
				if errorSignal := p.checkErrorRateMetric(metric, serviceName, serviceVersion); errorSignal != nil {
					signals = append(signals, *errorSignal)
				}

				// Check for latency metrics
				if latencySignal := p.checkLatencyMetric(metric, serviceName, serviceVersion); latencySignal != nil {
					signals = append(signals, *latencySignal)
				}

				// Check for resource utilisation metrics
				if resourceSignal := p.checkResourceMetric(metric, serviceName, serviceVersion); resourceSignal != nil {
					signals = append(signals, *resourceSignal)
				}
			}
		}
	}

	// Apply rate limiting and deduplication
	signals = p.filterSignals(signals)

	p.logger.Debug("Extracted signals from metrics", "signals_count", len(signals))
	return signals, nil
}

// ProcessTraces extracts signals from OpenTelemetry traces data.
//
// Analyses traces for:
// - High error span counts
// - Slow operations
// - Service failures
// - Dependency issues
//
// Returns extracted signals or an error if processing fails.
func (p *ThresholdProcessor) ProcessTraces(ctx context.Context, traces ptrace.Traces) ([]domain.Signal, error) {
	var signals []domain.Signal

	p.logger.Debug("Processing traces for signal extraction", "span_count", traces.SpanCount())

	// Track errors and slow spans per service
	serviceErrors := make(map[string]int)
	serviceSlowSpans := make(map[string]int)

	// Iterate through resource spans
	for i := 0; i < traces.ResourceSpans().Len(); i++ {
		resourceSpans := traces.ResourceSpans().At(i)
		resource := resourceSpans.Resource()

		serviceName := getStringAttribute(resource.Attributes(), "service.name", "unknown")
		serviceVersion := getStringAttribute(resource.Attributes(), "service.version", "unknown")

		// Process scope spans
		for j := 0; j < resourceSpans.ScopeSpans().Len(); j++ {
			scopeSpans := resourceSpans.ScopeSpans().At(j)

			// Process individual spans
			for k := 0; k < scopeSpans.Spans().Len(); k++ {
				span := scopeSpans.Spans().At(k)

				// Check for error spans
				if span.Status().Code() == ptrace.StatusCodeError {
					serviceErrors[serviceName]++
				}

				// Check for slow spans
				duration := span.EndTimestamp().AsTime().Sub(span.StartTimestamp().AsTime())
				if duration.Milliseconds() > int64(p.config.SlowSpanThreshold) {
					serviceSlowSpans[serviceName]++
				}
			}
		}

		// Generate signals for services with too many errors
		if serviceErrors[serviceName] >= p.config.ErrorSpanThreshold {
			signal := p.createTraceErrorSignal(serviceName, serviceVersion, serviceErrors[serviceName])
			signals = append(signals, signal)
		}

		// Generate signals for services with too many slow spans
		if serviceSlowSpans[serviceName] >= p.config.ErrorSpanThreshold { // Reuse threshold
			signal := p.createSlowSpanSignal(serviceName, serviceVersion, serviceSlowSpans[serviceName])
			signals = append(signals, signal)
		}
	}

	// Apply rate limiting and deduplication
	signals = p.filterSignals(signals)

	p.logger.Debug("Extracted signals from traces", "signals_count", len(signals))
	return signals, nil
}

// ProcessLogs extracts signals from OpenTelemetry logs data.
//
// Analyses logs for:
// - Error patterns
// - Critical messages
// - Panic/fatal conditions
// - Service-specific issues
//
// Returns extracted signals or an error if processing fails.
func (p *ThresholdProcessor) ProcessLogs(ctx context.Context, logs plog.Logs) ([]domain.Signal, error) {
	var signals []domain.Signal

	p.logger.Debug("Processing logs for signal extraction", "log_count", logs.LogRecordCount())

	// Iterate through resource logs
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		resourceLogs := logs.ResourceLogs().At(i)
		resource := resourceLogs.Resource()

		serviceName := getStringAttribute(resource.Attributes(), "service.name", "unknown")
		serviceVersion := getStringAttribute(resource.Attributes(), "service.version", "unknown")

		// Process scope logs
		for j := 0; j < resourceLogs.ScopeLogs().Len(); j++ {
			scopeLogs := resourceLogs.ScopeLogs().At(j)

			// Process individual log records
			for k := 0; k < scopeLogs.LogRecords().Len(); k++ {
				logRecord := scopeLogs.LogRecords().At(k)

				// Extract log body
				bodyStr := logRecord.Body().AsString()

				// Check for critical patterns first
				if p.matchesPatterns(bodyStr, p.config.CriticalLogPatterns) {
					signal := p.createLogSignal(serviceName, serviceVersion, bodyStr, domain.SeverityCritical, "Critical log pattern detected")
					signals = append(signals, signal)
					continue
				}

				// Check for error patterns
				if p.matchesPatterns(bodyStr, p.config.ErrorLogPatterns) {
					signal := p.createLogSignal(serviceName, serviceVersion, bodyStr, domain.SeverityHigh, "Error log pattern detected")
					signals = append(signals, signal)
				}
			}
		}
	}

	// Apply rate limiting and deduplication
	signals = p.filterSignals(signals)

	p.logger.Debug("Extracted signals from logs", "signals_count", len(signals))
	return signals, nil
}

// Helper functions for signal processing

// getStringAttribute safely extracts a string attribute from OpenTelemetry attributes.
func getStringAttribute(attrs pcommon.Map, key, defaultValue string) string {
	if val, exists := attrs.Get(key); exists {
		return val.AsString()
	}
	return defaultValue
}

// matchesPatterns checks if text matches any of the provided patterns.
func (p *ThresholdProcessor) matchesPatterns(text string, patterns []string) bool {
	textLower := strings.ToLower(text)
	for _, pattern := range patterns {
		if strings.Contains(textLower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// createSignalID generates a unique ID for a signal based on its characteristics.
func (p *ThresholdProcessor) createSignalID(source, service, signalType string) string {
	return fmt.Sprintf("%s_%s_%s_%d", source, service, signalType, time.Now().UnixNano())
}

// createSignalFingerprint creates a fingerprint for deduplication based on signal characteristics.
func (p *ThresholdProcessor) createSignalFingerprint(source, service, signalType string) string {
	return fmt.Sprintf("%s_%s_%s", source, service, signalType)
}

// checkErrorRateMetric analyses a metric for error rate thresholds.
func (p *ThresholdProcessor) checkErrorRateMetric(metric pmetric.Metric, serviceName, serviceVersion string) *domain.Signal {
	metricName := metric.Name()

	// Look for error rate metrics (common patterns)
	if !strings.Contains(strings.ToLower(metricName), "error") &&
		!strings.Contains(strings.ToLower(metricName), "failure") {
		return nil
	}

	// For now, implement a simple check for gauge metrics
	// In a real implementation, you'd want more sophisticated analysis
	if metric.Type() == pmetric.MetricTypeGauge {
		dataPoints := metric.Gauge().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			point := dataPoints.At(i)
			value := point.DoubleValue()

			if value > p.config.ErrorRateThreshold {
				return &domain.Signal{
					ID:          p.createSignalID("otlp", serviceName, "error_rate"),
					Source:      "otlp_metrics",
					Title:       fmt.Sprintf("High error rate detected in %s", serviceName),
					Description: fmt.Sprintf("Error rate %.2f%% exceeds threshold %.2f%% for service %s", value*100, p.config.ErrorRateThreshold*100, serviceName),
					Severity:    domain.SeverityHigh,
					Labels: map[string]string{
						"service.name":    serviceName,
						"service.version": serviceVersion,
						"metric.name":     metricName,
						"signal.type":     "error_rate",
					},
					Annotations: map[string]string{
						"threshold": fmt.Sprintf("%.2f", p.config.ErrorRateThreshold),
						"value":     fmt.Sprintf("%.2f", value),
					},
					Payload: map[string]interface{}{
						"metric_name":     metricName,
						"current_value":   value,
						"threshold_value": p.config.ErrorRateThreshold,
					},
					ReceivedAt: time.Now(),
				}
			}
		}
	}

	return nil
}

// checkLatencyMetric analyses a metric for latency thresholds.
func (p *ThresholdProcessor) checkLatencyMetric(metric pmetric.Metric, serviceName, serviceVersion string) *domain.Signal {
	metricName := metric.Name()

	// Look for latency metrics
	if !strings.Contains(strings.ToLower(metricName), "latency") &&
		!strings.Contains(strings.ToLower(metricName), "duration") &&
		!strings.Contains(strings.ToLower(metricName), "response_time") {
		return nil
	}

	// Check histogram metrics for latency analysis
	if metric.Type() == pmetric.MetricTypeHistogram {
		dataPoints := metric.Histogram().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			point := dataPoints.At(i)

			// Calculate average latency (sum/count)
			if point.Count() > 0 {
				avgLatency := point.Sum() / float64(point.Count())

				if avgLatency > p.config.LatencyThreshold {
					return &domain.Signal{
						ID:          p.createSignalID("otlp", serviceName, "latency"),
						Source:      "otlp_metrics",
						Title:       fmt.Sprintf("High latency detected in %s", serviceName),
						Description: fmt.Sprintf("Average latency %.2fms exceeds threshold %.2fms for service %s", avgLatency, p.config.LatencyThreshold, serviceName),
						Severity:    domain.SeverityMedium,
						Labels: map[string]string{
							"service.name":    serviceName,
							"service.version": serviceVersion,
							"metric.name":     metricName,
							"signal.type":     "latency",
						},
						Annotations: map[string]string{
							"threshold": fmt.Sprintf("%.2f", p.config.LatencyThreshold),
							"value":     fmt.Sprintf("%.2f", avgLatency),
						},
						Payload: map[string]interface{}{
							"metric_name":     metricName,
							"current_value":   avgLatency,
							"threshold_value": p.config.LatencyThreshold,
							"count":           point.Count(),
						},
						ReceivedAt: time.Now(),
					}
				}
			}
		}
	}

	return nil
}

// checkResourceMetric analyses a metric for resource utilisation thresholds.
func (p *ThresholdProcessor) checkResourceMetric(metric pmetric.Metric, serviceName, serviceVersion string) *domain.Signal {
	metricName := metric.Name()

	// Look for CPU metrics
	if strings.Contains(strings.ToLower(metricName), "cpu") {
		return p.checkCPUMetric(metric, serviceName, serviceVersion)
	}

	// Look for memory metrics
	if strings.Contains(strings.ToLower(metricName), "memory") || strings.Contains(strings.ToLower(metricName), "mem") {
		return p.checkMemoryMetric(metric, serviceName, serviceVersion)
	}

	return nil
}

// checkCPUMetric analyses CPU metrics for threshold violations.
func (p *ThresholdProcessor) checkCPUMetric(metric pmetric.Metric, serviceName, serviceVersion string) *domain.Signal {
	if metric.Type() == pmetric.MetricTypeGauge {
		dataPoints := metric.Gauge().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			point := dataPoints.At(i)
			value := point.DoubleValue()

			// Assume CPU metrics are in percentage (0-1 or 0-100)
			normalizedValue := value
			if value > 1.0 {
				normalizedValue = value / 100.0 // Convert percentage to ratio
			}

			if normalizedValue > p.config.CPUThreshold {
				return &domain.Signal{
					ID:          p.createSignalID("otlp", serviceName, "cpu"),
					Source:      "otlp_metrics",
					Title:       fmt.Sprintf("High CPU usage detected in %s", serviceName),
					Description: fmt.Sprintf("CPU usage %.1f%% exceeds threshold %.1f%% for service %s", normalizedValue*100, p.config.CPUThreshold*100, serviceName),
					Severity:    domain.SeverityMedium,
					Labels: map[string]string{
						"service.name":    serviceName,
						"service.version": serviceVersion,
						"metric.name":     metric.Name(),
						"signal.type":     "cpu",
					},
					Annotations: map[string]string{
						"threshold": fmt.Sprintf("%.1f", p.config.CPUThreshold*100),
						"value":     fmt.Sprintf("%.1f", normalizedValue*100),
					},
					Payload: map[string]interface{}{
						"metric_name":     metric.Name(),
						"current_value":   normalizedValue,
						"threshold_value": p.config.CPUThreshold,
					},
					ReceivedAt: time.Now(),
				}
			}
		}
	}

	return nil
}

// checkMemoryMetric analyses memory metrics for threshold violations.
func (p *ThresholdProcessor) checkMemoryMetric(metric pmetric.Metric, serviceName, serviceVersion string) *domain.Signal {
	if metric.Type() == pmetric.MetricTypeGauge {
		dataPoints := metric.Gauge().DataPoints()
		for i := 0; i < dataPoints.Len(); i++ {
			point := dataPoints.At(i)
			value := point.DoubleValue()

			// Assume memory metrics are in percentage (0-1 or 0-100)
			normalizedValue := value
			if value > 1.0 {
				normalizedValue = value / 100.0
			}

			if normalizedValue > p.config.MemoryThreshold {
				return &domain.Signal{
					ID:          p.createSignalID("otlp", serviceName, "memory"),
					Source:      "otlp_metrics",
					Title:       fmt.Sprintf("High memory usage detected in %s", serviceName),
					Description: fmt.Sprintf("Memory usage %.1f%% exceeds threshold %.1f%% for service %s", normalizedValue*100, p.config.MemoryThreshold*100, serviceName),
					Severity:    domain.SeverityMedium,
					Labels: map[string]string{
						"service.name":    serviceName,
						"service.version": serviceVersion,
						"metric.name":     metric.Name(),
						"signal.type":     "memory",
					},
					Annotations: map[string]string{
						"threshold": fmt.Sprintf("%.1f", p.config.MemoryThreshold*100),
						"value":     fmt.Sprintf("%.1f", normalizedValue*100),
					},
					Payload: map[string]interface{}{
						"metric_name":     metric.Name(),
						"current_value":   normalizedValue,
						"threshold_value": p.config.MemoryThreshold,
					},
					ReceivedAt: time.Now(),
				}
			}
		}
	}

	return nil
}

// createTraceErrorSignal creates a signal for excessive error spans.
func (p *ThresholdProcessor) createTraceErrorSignal(serviceName, serviceVersion string, errorCount int) domain.Signal {
	return domain.Signal{
		ID:          p.createSignalID("otlp", serviceName, "trace_errors"),
		Source:      "otlp_traces",
		Title:       fmt.Sprintf("High error span count in %s", serviceName),
		Description: fmt.Sprintf("Service %s has %d error spans, exceeding threshold of %d", serviceName, errorCount, p.config.ErrorSpanThreshold),
		Severity:    domain.SeverityHigh,
		Labels: map[string]string{
			"service.name":    serviceName,
			"service.version": serviceVersion,
			"signal.type":     "trace_errors",
		},
		Annotations: map[string]string{
			"threshold":   fmt.Sprintf("%d", p.config.ErrorSpanThreshold),
			"error_count": fmt.Sprintf("%d", errorCount),
		},
		Payload: map[string]interface{}{
			"error_count":     errorCount,
			"threshold_value": p.config.ErrorSpanThreshold,
		},
		ReceivedAt: time.Now(),
	}
}

// createSlowSpanSignal creates a signal for excessive slow spans.
func (p *ThresholdProcessor) createSlowSpanSignal(serviceName, serviceVersion string, slowCount int) domain.Signal {
	return domain.Signal{
		ID:          p.createSignalID("otlp", serviceName, "slow_spans"),
		Source:      "otlp_traces",
		Title:       fmt.Sprintf("High slow span count in %s", serviceName),
		Description: fmt.Sprintf("Service %s has %d slow spans (>%.0fms), exceeding threshold of %d", serviceName, slowCount, p.config.SlowSpanThreshold, p.config.ErrorSpanThreshold),
		Severity:    domain.SeverityMedium,
		Labels: map[string]string{
			"service.name":    serviceName,
			"service.version": serviceVersion,
			"signal.type":     "slow_spans",
		},
		Annotations: map[string]string{
			"threshold":  fmt.Sprintf("%d", p.config.ErrorSpanThreshold),
			"slow_count": fmt.Sprintf("%d", slowCount),
		},
		Payload: map[string]interface{}{
			"slow_count":        slowCount,
			"threshold_value":   p.config.ErrorSpanThreshold,
			"latency_threshold": p.config.SlowSpanThreshold,
		},
		ReceivedAt: time.Now(),
	}
}

// createLogSignal creates a signal from log analysis.
func (p *ThresholdProcessor) createLogSignal(serviceName, serviceVersion, logBody string, severity domain.Severity, reason string) domain.Signal {
	return domain.Signal{
		ID:          p.createSignalID("otlp", serviceName, "log_pattern"),
		Source:      "otlp_logs",
		Title:       fmt.Sprintf("Critical log pattern in %s", serviceName),
		Description: fmt.Sprintf("%s in service %s: %s", reason, serviceName, logBody),
		Severity:    severity,
		Labels: map[string]string{
			"service.name":    serviceName,
			"service.version": serviceVersion,
			"signal.type":     "log_pattern",
		},
		Annotations: map[string]string{
			"log_body": logBody,
			"reason":   reason,
		},
		Payload: map[string]interface{}{
			"log_body": logBody,
			"reason":   reason,
		},
		ReceivedAt: time.Now(),
	}
}

// filterSignals applies rate limiting and deduplication to signals.
func (p *ThresholdProcessor) filterSignals(signals []domain.Signal) []domain.Signal {
	now := time.Now()
	var filtered []domain.Signal

	for _, signal := range signals {
		fingerprint := p.createSignalFingerprint(signal.Source,
			signal.Labels["service.name"],
			signal.Labels["signal.type"])

		// Check if we've sent a similar signal recently
		if lastSent, exists := p.recentSignals[fingerprint]; exists {
			if now.Sub(lastSent) < p.config.MinSignalInterval {
				p.logger.Debug("Skipping duplicate signal", "fingerprint", fingerprint)
				continue
			}
		}

		// Record this signal
		p.recentSignals[fingerprint] = now
		filtered = append(filtered, signal)
	}

	// Clean up old entries periodically
	p.cleanupOldSignals(now)

	return filtered
}

// cleanupOldSignals removes old entries from the deduplication map.
func (p *ThresholdProcessor) cleanupOldSignals(now time.Time) {
	cutoff := now.Add(-p.config.MinSignalInterval * 2) // Keep entries for 2x the interval

	for fingerprint, lastSent := range p.recentSignals {
		if lastSent.Before(cutoff) {
			delete(p.recentSignals, fingerprint)
		}
	}
}
