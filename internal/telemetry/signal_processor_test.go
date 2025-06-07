package telemetry

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// createTestLogger creates a logger for testing
func createTestLogger() *logging.Logger {
	config := logging.DefaultConfig(logging.Development)
	logger, _ := logging.NewLogger(config)
	return logger
}

func TestDefaultProcessorConfig(t *testing.T) {
	config := DefaultProcessorConfig()

	// Test default values
	if config.ErrorRateThreshold != 0.05 {
		t.Errorf("Expected default error rate threshold 0.05, got %f", config.ErrorRateThreshold)
	}
	if config.LatencyThreshold != 1000.0 {
		t.Errorf("Expected default latency threshold 1000.0, got %f", config.LatencyThreshold)
	}
	if config.CPUThreshold != 0.8 {
		t.Errorf("Expected default CPU threshold 0.8, got %f", config.CPUThreshold)
	}
	if config.MemoryThreshold != 0.9 {
		t.Errorf("Expected default memory threshold 0.9, got %f", config.MemoryThreshold)
	}
	if config.ErrorSpanThreshold != 5 {
		t.Errorf("Expected default error span threshold 5, got %d", config.ErrorSpanThreshold)
	}
	if config.SlowSpanThreshold != 5000.0 {
		t.Errorf("Expected default slow span threshold 5000.0, got %f", config.SlowSpanThreshold)
	}
	if config.MinSignalInterval != 5*time.Minute {
		t.Errorf("Expected default min signal interval 5m, got %v", config.MinSignalInterval)
	}
	if config.MaxSignalsPerMinute != 10 {
		t.Errorf("Expected default max signals per minute 10, got %d", config.MaxSignalsPerMinute)
	}

	// Test error patterns
	expectedErrorPatterns := []string{"error", "ERROR", "Error", "exception", "Exception", "EXCEPTION", "fatal", "Fatal", "FATAL", "panic", "Panic", "PANIC"}
	if len(config.ErrorLogPatterns) != len(expectedErrorPatterns) {
		t.Errorf("Expected %d error patterns, got %d", len(expectedErrorPatterns), len(config.ErrorLogPatterns))
	}

	// Test critical patterns
	expectedCriticalPatterns := []string{"critical", "Critical", "CRITICAL", "emergency", "Emergency", "EMERGENCY", "alert", "Alert", "ALERT"}
	if len(config.CriticalLogPatterns) != len(expectedCriticalPatterns) {
		t.Errorf("Expected %d critical patterns, got %d", len(expectedCriticalPatterns), len(config.CriticalLogPatterns))
	}
}

func TestNewThresholdProcessor(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()

	processor := NewThresholdProcessor(config, logger)

	if processor == nil {
		t.Fatal("Expected processor to be created, got nil")
	}
	if processor.config.ErrorRateThreshold != config.ErrorRateThreshold {
		t.Errorf("Expected error rate threshold %f, got %f", config.ErrorRateThreshold, processor.config.ErrorRateThreshold)
	}
	if processor.logger != logger {
		t.Error("Expected logger to be set correctly")
	}
	if processor.recentSignals == nil {
		t.Error("Expected recentSignals map to be initialized")
	}
}

func TestProcessMetrics_ErrorRate(t *testing.T) {
	config := DefaultProcessorConfig()
	config.ErrorRateThreshold = 0.05 // 5%
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test metrics
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	// Set resource attributes
	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("service.name", "test-service")
	resource.Attributes().PutStr("service.version", "1.0.0")

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	metric := scopeMetrics.Metrics().AppendEmpty()
	metric.SetName("error_rate")

	// Create gauge metric with high error rate
	gauge := metric.SetEmptyGauge()
	dataPoint := gauge.DataPoints().AppendEmpty()
	dataPoint.SetDoubleValue(0.10) // 10% error rate - above threshold

	ctx := context.Background()
	signals, err := processor.ProcessMetrics(ctx, metrics)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Source != "otlp_metrics" {
		t.Errorf("Expected source 'otlp_metrics', got %s", signal.Source)
	}
	if signal.Severity != domain.SeverityHigh {
		t.Errorf("Expected high severity, got %s", signal.Severity)
	}
	if signal.Labels["service.name"] != "test-service" {
		t.Errorf("Expected service name 'test-service', got %s", signal.Labels["service.name"])
	}
	if signal.Labels["signal.type"] != "error_rate" {
		t.Errorf("Expected signal type 'error_rate', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessMetrics_Latency(t *testing.T) {
	config := DefaultProcessorConfig()
	config.LatencyThreshold = 1000.0 // 1 second
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test metrics
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("service.name", "slow-service")
	resource.Attributes().PutStr("service.version", "2.0.0")

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	metric := scopeMetrics.Metrics().AppendEmpty()
	metric.SetName("response_latency")

	// Create histogram metric with high latency
	histogram := metric.SetEmptyHistogram()
	dataPoint := histogram.DataPoints().AppendEmpty()
	dataPoint.SetSum(15000.0) // 15 seconds total
	dataPoint.SetCount(10)    // 10 requests = 1.5 seconds average

	ctx := context.Background()
	signals, err := processor.ProcessMetrics(ctx, metrics)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Source != "otlp_metrics" {
		t.Errorf("Expected source 'otlp_metrics', got %s", signal.Source)
	}
	if signal.Severity != domain.SeverityMedium {
		t.Errorf("Expected medium severity, got %s", signal.Severity)
	}
	if signal.Labels["signal.type"] != "latency" {
		t.Errorf("Expected signal type 'latency', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessMetrics_CPU(t *testing.T) {
	config := DefaultProcessorConfig()
	config.CPUThreshold = 0.8 // 80%
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test metrics
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("service.name", "cpu-heavy-service")

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	metric := scopeMetrics.Metrics().AppendEmpty()
	metric.SetName("cpu_usage")

	// Create gauge metric with high CPU usage (95%)
	gauge := metric.SetEmptyGauge()
	dataPoint := gauge.DataPoints().AppendEmpty()
	dataPoint.SetDoubleValue(95.0) // 95% - will be normalized to 0.95

	ctx := context.Background()
	signals, err := processor.ProcessMetrics(ctx, metrics)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Labels["signal.type"] != "cpu" {
		t.Errorf("Expected signal type 'cpu', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessMetrics_Memory(t *testing.T) {
	config := DefaultProcessorConfig()
	config.MemoryThreshold = 0.9 // 90%
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test metrics
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("service.name", "memory-heavy-service")

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	metric := scopeMetrics.Metrics().AppendEmpty()
	metric.SetName("memory_usage")

	// Create gauge metric with high memory usage (0.95 = 95%)
	gauge := metric.SetEmptyGauge()
	dataPoint := gauge.DataPoints().AppendEmpty()
	dataPoint.SetDoubleValue(0.95) // 95% as ratio

	ctx := context.Background()
	signals, err := processor.ProcessMetrics(ctx, metrics)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Labels["signal.type"] != "memory" {
		t.Errorf("Expected signal type 'memory', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessMetrics_NoSignals(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test metrics with low values
	metrics := pmetric.NewMetrics()
	resourceMetrics := metrics.ResourceMetrics().AppendEmpty()

	resource := resourceMetrics.Resource()
	resource.Attributes().PutStr("service.name", "healthy-service")

	scopeMetrics := resourceMetrics.ScopeMetrics().AppendEmpty()
	metric := scopeMetrics.Metrics().AppendEmpty()
	metric.SetName("error_rate")

	gauge := metric.SetEmptyGauge()
	dataPoint := gauge.DataPoints().AppendEmpty()
	dataPoint.SetDoubleValue(0.01) // 1% error rate - below threshold

	ctx := context.Background()
	signals, err := processor.ProcessMetrics(ctx, metrics)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 0 {
		t.Fatalf("Expected 0 signals, got %d", len(signals))
	}
}

func TestProcessTraces_ErrorSpans(t *testing.T) {
	config := DefaultProcessorConfig()
	config.ErrorSpanThreshold = 3 // Lower threshold for testing
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test traces
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("service.name", "error-service")
	resource.Attributes().PutStr("service.version", "1.0.0")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	// Add multiple spans with errors
	for i := 0; i < 5; i++ {
		span := scopeSpans.Spans().AppendEmpty()
		span.SetName("test-span")
		span.Status().SetCode(ptrace.StatusCodeError)
		span.Status().SetMessage("test error")
	}

	ctx := context.Background()
	signals, err := processor.ProcessTraces(ctx, traces)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Source != "otlp_traces" {
		t.Errorf("Expected source 'otlp_traces', got %s", signal.Source)
	}
	if signal.Severity != domain.SeverityHigh {
		t.Errorf("Expected high severity, got %s", signal.Severity)
	}
	if signal.Labels["signal.type"] != "trace_errors" {
		t.Errorf("Expected signal type 'trace_errors', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessTraces_SlowSpans(t *testing.T) {
	config := DefaultProcessorConfig()
	config.ErrorSpanThreshold = 3     // Lower threshold for testing
	config.SlowSpanThreshold = 1000.0 // 1 second
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test traces
	traces := ptrace.NewTraces()
	resourceSpans := traces.ResourceSpans().AppendEmpty()

	resource := resourceSpans.Resource()
	resource.Attributes().PutStr("service.name", "slow-service")

	scopeSpans := resourceSpans.ScopeSpans().AppendEmpty()

	// Add multiple slow spans
	now := time.Now()
	for i := 0; i < 5; i++ {
		span := scopeSpans.Spans().AppendEmpty()
		span.SetName("slow-span")
		span.SetStartTimestamp(pcommon.NewTimestampFromTime(now))
		span.SetEndTimestamp(pcommon.NewTimestampFromTime(now.Add(2 * time.Second))) // 2 seconds duration
		span.Status().SetCode(ptrace.StatusCodeOk)
	}

	ctx := context.Background()
	signals, err := processor.ProcessTraces(ctx, traces)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Labels["signal.type"] != "slow_spans" {
		t.Errorf("Expected signal type 'slow_spans', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessLogs_ErrorPatterns(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test logs
	logs := plog.NewLogs()
	resourceLogs := logs.ResourceLogs().AppendEmpty()

	resource := resourceLogs.Resource()
	resource.Attributes().PutStr("service.name", "log-service")
	resource.Attributes().PutStr("service.version", "1.0.0")

	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()

	// Add log with error pattern
	logRecord := scopeLogs.LogRecords().AppendEmpty()
	logRecord.Body().SetStr("ERROR: Database connection failed")
	logRecord.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))

	ctx := context.Background()
	signals, err := processor.ProcessLogs(ctx, logs)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Source != "otlp_logs" {
		t.Errorf("Expected source 'otlp_logs', got %s", signal.Source)
	}
	if signal.Severity != domain.SeverityHigh {
		t.Errorf("Expected high severity, got %s", signal.Severity)
	}
	if signal.Labels["signal.type"] != "log_pattern" {
		t.Errorf("Expected signal type 'log_pattern', got %s", signal.Labels["signal.type"])
	}
}

func TestProcessLogs_CriticalPatterns(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create test logs
	logs := plog.NewLogs()
	resourceLogs := logs.ResourceLogs().AppendEmpty()

	resource := resourceLogs.Resource()
	resource.Attributes().PutStr("service.name", "critical-service")

	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()

	// Add log with critical pattern
	logRecord := scopeLogs.LogRecords().AppendEmpty()
	logRecord.Body().SetStr("CRITICAL: System shutdown imminent")

	ctx := context.Background()
	signals, err := processor.ProcessLogs(ctx, logs)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if len(signals) != 1 {
		t.Fatalf("Expected 1 signal, got %d", len(signals))
	}

	signal := signals[0]
	if signal.Severity != domain.SeverityCritical {
		t.Errorf("Expected critical severity, got %s", signal.Severity)
	}
}

func TestFilterSignals_Deduplication(t *testing.T) {
	config := DefaultProcessorConfig()
	config.MinSignalInterval = 1 * time.Minute
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create similar signals
	signal1 := domain.Signal{
		ID:     "signal-1",
		Source: "test_source",
		Labels: map[string]string{
			"service.name": "test-service",
			"signal.type":  "error_rate",
		},
		ReceivedAt: time.Now(),
	}

	signal2 := domain.Signal{
		ID:     "signal-2",
		Source: "test_source",
		Labels: map[string]string{
			"service.name": "test-service",
			"signal.type":  "error_rate",
		},
		ReceivedAt: time.Now(),
	}

	// Filter the first batch
	signals1 := processor.filterSignals([]domain.Signal{signal1})
	if len(signals1) != 1 {
		t.Fatalf("Expected 1 signal in first batch, got %d", len(signals1))
	}

	// Filter the second batch immediately (should be deduplicated)
	signals2 := processor.filterSignals([]domain.Signal{signal2})
	if len(signals2) != 0 {
		t.Fatalf("Expected 0 signals in second batch (deduplicated), got %d", len(signals2))
	}
}

func TestFilterSignals_NoDuplication(t *testing.T) {
	config := DefaultProcessorConfig()
	config.MinSignalInterval = 1 * time.Minute
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Create different signals
	signal1 := domain.Signal{
		ID:     "signal-1",
		Source: "test_source",
		Labels: map[string]string{
			"service.name": "service-1",
			"signal.type":  "error_rate",
		},
		ReceivedAt: time.Now(),
	}

	signal2 := domain.Signal{
		ID:     "signal-2",
		Source: "test_source",
		Labels: map[string]string{
			"service.name": "service-2", // Different service
			"signal.type":  "error_rate",
		},
		ReceivedAt: time.Now(),
	}

	// Filter signals (should both pass through)
	signals := processor.filterSignals([]domain.Signal{signal1, signal2})
	if len(signals) != 2 {
		t.Fatalf("Expected 2 signals, got %d", len(signals))
	}
}

func TestGetStringAttribute(t *testing.T) {
	attrs := pcommon.NewMap()
	attrs.PutStr("test_key", "test_value")

	// Test existing attribute
	value := getStringAttribute(attrs, "test_key", "default")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %s", value)
	}

	// Test non-existing attribute
	value = getStringAttribute(attrs, "missing_key", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got %s", value)
	}
}

func TestMatchesPatterns(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	patterns := []string{"error", "warning", "alert"}

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"matches error", "This is an error message", true},
		{"matches warning", "warning: something happened", true},
		{"matches alert", "ALERT: critical issue", true},
		{"no match", "info: normal operation", false},
		{"case insensitive", "ERROR in caps", true}, // "ERROR" matches "error" pattern
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.matchesPatterns(tt.text, patterns)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v for text: %s", tt.expected, result, tt.text)
			}
		})
	}
}

func TestCreateSignalID(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	id1 := processor.createSignalID("source", "service", "type")
	// Small delay to ensure different nanosecond timestamps
	time.Sleep(1 * time.Nanosecond)
	id2 := processor.createSignalID("source", "service", "type")

	// IDs should be different due to timestamp
	if id1 == id2 {
		t.Errorf("Expected different signal IDs, got %s and %s", id1, id2)
	}

	// IDs should contain the components
	if !contains(id1, "source") || !contains(id1, "service") || !contains(id1, "type") {
		t.Errorf("Signal ID should contain components: %s", id1)
	}
}

func TestCreateSignalFingerprint(t *testing.T) {
	config := DefaultProcessorConfig()
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	fp1 := processor.createSignalFingerprint("source", "service", "type")
	fp2 := processor.createSignalFingerprint("source", "service", "type")

	// Fingerprints should be identical
	if fp1 != fp2 {
		t.Errorf("Expected identical fingerprints, got %s and %s", fp1, fp2)
	}

	// Different inputs should give different fingerprints
	fp3 := processor.createSignalFingerprint("different", "service", "type")
	if fp1 == fp3 {
		t.Error("Expected different fingerprints for different inputs")
	}
}

func TestCleanupOldSignals(t *testing.T) {
	config := DefaultProcessorConfig()
	config.MinSignalInterval = 1 * time.Minute
	logger := createTestLogger()
	processor := NewThresholdProcessor(config, logger)

	// Add some old signals
	processor.recentSignals["old_signal"] = time.Now().Add(-10 * time.Minute)
	processor.recentSignals["recent_signal"] = time.Now().Add(-30 * time.Second)

	if len(processor.recentSignals) != 2 {
		t.Fatalf("Expected 2 signals before cleanup, got %d", len(processor.recentSignals))
	}

	processor.cleanupOldSignals(time.Now())

	// Old signal should be removed, recent one should remain
	if len(processor.recentSignals) != 1 {
		t.Fatalf("Expected 1 signal after cleanup, got %d", len(processor.recentSignals))
	}

	if _, exists := processor.recentSignals["recent_signal"]; !exists {
		t.Error("Recent signal should still exist after cleanup")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				containsSubstring(s, substr))))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
