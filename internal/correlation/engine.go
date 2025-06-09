// Package correlation provides signal correlation and grouping capabilities for the Guvnor incident management platform.
//
// This package implements intelligent alert correlation and grouping, reducing alert fatigue by
// combining related signals into coherent groups. The correlation engine supports multiple
// strategies including time-based grouping, service-based grouping, label matching, and
// fingerprint-based pattern matching.
//
// The engine is designed to handle high volume (1000s signals/sec) while maintaining low latency
// for correlation decisions. State is kept in memory for performance with configurable cleanup
// policies to prevent memory leaks.
package correlation

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// CorrelationRule defines a rule for grouping signals together.
type CorrelationRule struct {
	// Name is a human-readable identifier for the rule
	Name string `yaml:"name" json:"name"`
	// Type specifies the correlation strategy
	Type CorrelationType `yaml:"type" json:"type"`
	// GroupBy specifies which signal attributes to group by
	GroupBy []string `yaml:"group_by" json:"group_by"`
	// MatchLabels specifies exact label matches required
	MatchLabels map[string]string `yaml:"match_labels" json:"match_labels"`
	// MatchPatterns specifies regex patterns for matching
	MatchPatterns []string `yaml:"match_patterns" json:"match_patterns"`
	// Priority determines rule evaluation order (higher = evaluated first)
	Priority int `yaml:"priority" json:"priority"`
	// Enabled determines if the rule is active
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// CorrelationType defines the available correlation strategies.
type CorrelationType string

const (
	// CorrelationTypeTime groups signals within a time window
	CorrelationTypeTime CorrelationType = "time"
	// CorrelationTypeService groups signals from the same service/namespace
	CorrelationTypeService CorrelationType = "service"
	// CorrelationTypeLabels groups signals with matching labels
	CorrelationTypeLabels CorrelationType = "labels"
	// CorrelationTypeFingerprint groups signals with similar patterns
	CorrelationTypeFingerprint CorrelationType = "fingerprint"
)

// CorrelationConfig configures the correlation engine behaviour.
type CorrelationConfig struct {
	// TimeWindow is the maximum time between signals to consider for grouping
	TimeWindow time.Duration `yaml:"time_window" json:"time_window"`
	// MaxGroupSize limits the number of signals in a single group
	MaxGroupSize int `yaml:"max_group_size" json:"max_group_size"`
	// CleanupInterval determines how often to clean up old groups
	CleanupInterval time.Duration `yaml:"cleanup_interval" json:"cleanup_interval"`
	// GroupTTL is how long to keep groups without new signals
	GroupTTL time.Duration `yaml:"group_ttl" json:"group_ttl"`
	// Rules define the correlation strategies to apply
	Rules []CorrelationRule `yaml:"rules" json:"rules"`
	// EnableDeduplication removes exact duplicate signals
	EnableDeduplication bool `yaml:"enable_deduplication" json:"enable_deduplication"`
}

// DefaultCorrelationConfig returns sensible defaults for signal correlation.
func DefaultCorrelationConfig() CorrelationConfig {
	return CorrelationConfig{
		TimeWindow:      5 * time.Minute,
		MaxGroupSize:    50,
		CleanupInterval: 10 * time.Minute,
		GroupTTL:        30 * time.Minute,
		Rules: []CorrelationRule{
			{
				Name:     "service-correlation",
				Type:     CorrelationTypeService,
				GroupBy:  []string{"service.name", "service.namespace"},
				Priority: 100,
				Enabled:  true,
			},
			{
				Name:        "critical-alerts",
				Type:        CorrelationTypeLabels,
				MatchLabels: map[string]string{"severity": "critical"},
				Priority:    200,
				Enabled:     true,
			},
			{
				Name:     "time-based-fallback",
				Type:     CorrelationTypeTime,
				Priority: 10,
				Enabled:  true,
			},
		},
		EnableDeduplication: true,
	}
}

// SignalGroup represents a collection of correlated signals.
type SignalGroup struct {
	// ID is a unique identifier for the group
	ID string `json:"id"`
	// Fingerprint is a hash representing the group characteristics
	Fingerprint string `json:"fingerprint"`
	// Signals contains the correlated signals
	Signals []domain.Signal `json:"signals"`
	// Rules tracks which rules contributed to this grouping
	Rules []string `json:"rules"`
	// CreatedAt is when the group was first created
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is when the group was last modified
	UpdatedAt time.Time `json:"updated_at"`
	// Labels contains metadata about the group
	Labels map[string]string `json:"labels"`
}

// CorrelationMetrics tracks correlation engine performance.
type CorrelationMetrics struct {
	// SignalsProcessed is the total number of signals processed
	SignalsProcessed int64 `json:"signals_processed"`
	// GroupsCreated is the total number of groups created
	GroupsCreated int64 `json:"groups_created"`
	// SignalsCorrelated is the total number of signals added to existing groups
	SignalsCorrelated int64 `json:"signals_correlated"`
	// DuplicatesFiltered is the number of duplicate signals removed
	DuplicatesFiltered int64 `json:"duplicates_filtered"`
	// ActiveGroups is the current number of active groups
	ActiveGroups int64 `json:"active_groups"`
	// AverageGroupSize is the mean number of signals per group
	AverageGroupSize float64 `json:"average_group_size"`
}

// CorrelationEngine manages signal correlation and grouping.
//
// The engine maintains state in memory for performance, using configurable rules
// to determine how signals should be grouped together. It supports multiple
// correlation strategies and can handle high-volume signal processing.
type CorrelationEngine struct {
	config  CorrelationConfig
	logger  *logging.Logger
	metrics CorrelationMetrics

	// In-memory state
	groups       map[string]*SignalGroup // groups by ID
	fingerprints map[string]string       // fingerprint to group ID mapping
	dedupeCache  map[string]time.Time    // signal fingerprint to received time
	mu           sync.RWMutex            // protects all state

	// Background cleanup
	stopCleanup chan struct{}
	cleanupDone chan struct{}
}

// NewCorrelationEngine creates a new correlation engine with the given configuration.
func NewCorrelationEngine(config CorrelationConfig, logger *logging.Logger) (*CorrelationEngine, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Validate configuration
	if config.TimeWindow <= 0 {
		return nil, fmt.Errorf("time window must be positive")
	}
	if config.MaxGroupSize <= 0 {
		return nil, fmt.Errorf("max group size must be positive")
	}

	// Sort rules by priority (highest first)
	rules := make([]CorrelationRule, len(config.Rules))
	copy(rules, config.Rules)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority > rules[j].Priority
	})
	config.Rules = rules

	engine := &CorrelationEngine{
		config:       config,
		logger:       logger,
		groups:       make(map[string]*SignalGroup),
		fingerprints: make(map[string]string),
		dedupeCache:  make(map[string]time.Time),
		stopCleanup:  make(chan struct{}),
		cleanupDone:  make(chan struct{}),
	}

	// Start background cleanup goroutine
	go engine.cleanupWorker()

	return engine, nil
}

// ProcessSignal processes a signal through the correlation engine.
//
// The signal is evaluated against all correlation rules in priority order.
// If a match is found, the signal is added to an existing group or a new
// group is created. Returns the group that contains the signal.
func (e *CorrelationEngine) ProcessSignal(ctx context.Context, signal domain.Signal) (*SignalGroup, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.metrics.SignalsProcessed++

	// Check for duplicates if enabled
	if e.config.EnableDeduplication {
		if e.isDuplicate(signal) {
			e.metrics.DuplicatesFiltered++
			e.logger.Debug("Filtered duplicate signal", "signal_id", signal.ID)
			return nil, nil
		}
	}

	// Try to find an existing group using correlation rules
	for _, rule := range e.config.Rules {
		if !rule.Enabled {
			continue
		}

		if group := e.findMatchingGroup(signal, rule); group != nil {
			// Add signal to existing group
			if err := e.addSignalToGroup(group, signal, rule.Name); err != nil {
				e.logger.Error("Failed to add signal to group",
					"signal_id", signal.ID,
					"group_id", group.ID,
					"rule", rule.Name,
					"error", err)
				continue
			}

			e.metrics.SignalsCorrelated++
			e.logger.Debug("Correlated signal to existing group",
				"signal_id", signal.ID,
				"group_id", group.ID,
				"rule", rule.Name)
			return group, nil
		}
	}

	// No existing group found, create a new one
	group := e.createNewGroup(signal)
	e.metrics.GroupsCreated++
	e.metrics.ActiveGroups++

	e.logger.Debug("Created new group for signal",
		"signal_id", signal.ID,
		"group_id", group.ID)

	return group, nil
}

// GetGroups returns all active signal groups.
func (e *CorrelationEngine) GetGroups() []*SignalGroup {
	e.mu.RLock()
	defer e.mu.RUnlock()

	groups := make([]*SignalGroup, 0, len(e.groups))
	for _, group := range e.groups {
		// Return a copy to avoid race conditions
		groupCopy := *group
		groupCopy.Signals = make([]domain.Signal, len(group.Signals))
		copy(groupCopy.Signals, group.Signals)
		groupCopy.Rules = make([]string, len(group.Rules))
		copy(groupCopy.Rules, group.Rules)
		groups = append(groups, &groupCopy)
	}

	return groups
}

// GetMetrics returns current correlation metrics.
func (e *CorrelationEngine) GetMetrics() CorrelationMetrics {
	e.mu.RLock()
	defer e.mu.RUnlock()

	metrics := e.metrics
	metrics.ActiveGroups = int64(len(e.groups))

	// Calculate average group size
	if len(e.groups) > 0 {
		totalSignals := int64(0)
		for _, group := range e.groups {
			totalSignals += int64(len(group.Signals))
		}
		metrics.AverageGroupSize = float64(totalSignals) / float64(len(e.groups))
	}

	return metrics
}

// Stop shuts down the correlation engine.
func (e *CorrelationEngine) Stop() {
	close(e.stopCleanup)
	<-e.cleanupDone
}

// Helper methods

// isDuplicate checks if a signal is a duplicate based on fingerprint.
func (e *CorrelationEngine) isDuplicate(signal domain.Signal) bool {
	fingerprint := e.createSignalFingerprint(signal)
	if lastSeen, exists := e.dedupeCache[fingerprint]; exists {
		// Consider it a duplicate if seen within the time window
		if time.Since(lastSeen) < e.config.TimeWindow {
			return true
		}
	}
	e.dedupeCache[fingerprint] = signal.ReceivedAt
	return false
}

// findMatchingGroup finds an existing group that matches the signal and rule.
func (e *CorrelationEngine) findMatchingGroup(signal domain.Signal, rule CorrelationRule) *SignalGroup {
	switch rule.Type {
	case CorrelationTypeTime:
		return e.findTimeBasedGroup(signal)
	case CorrelationTypeService:
		return e.findServiceBasedGroup(signal, rule)
	case CorrelationTypeLabels:
		return e.findLabelBasedGroup(signal, rule)
	case CorrelationTypeFingerprint:
		return e.findFingerprintBasedGroup(signal, rule)
	default:
		e.logger.Warn("Unknown correlation type", "type", rule.Type)
		return nil
	}
}

// findTimeBasedGroup finds a group based on time window.
func (e *CorrelationEngine) findTimeBasedGroup(signal domain.Signal) *SignalGroup {
	cutoff := signal.ReceivedAt.Add(-e.config.TimeWindow)

	for _, group := range e.groups {
		if group.UpdatedAt.After(cutoff) && len(group.Signals) < e.config.MaxGroupSize {
			return group
		}
	}
	return nil
}

// findServiceBasedGroup finds a group based on service attributes.
func (e *CorrelationEngine) findServiceBasedGroup(signal domain.Signal, rule CorrelationRule) *SignalGroup {
	groupKey := e.createGroupKey(signal, rule.GroupBy)
	cutoff := signal.ReceivedAt.Add(-e.config.TimeWindow)

	for _, group := range e.groups {
		if group.UpdatedAt.Before(cutoff) || len(group.Signals) >= e.config.MaxGroupSize {
			continue
		}

		// Check if this group matches the service criteria
		if len(group.Signals) > 0 {
			existingKey := e.createGroupKey(group.Signals[0], rule.GroupBy)
			if existingKey == groupKey {
				return group
			}
		}
	}
	return nil
}

// findLabelBasedGroup finds a group based on label matching.
func (e *CorrelationEngine) findLabelBasedGroup(signal domain.Signal, rule CorrelationRule) *SignalGroup {
	// Check if signal matches the required labels
	for key, value := range rule.MatchLabels {
		if signalValue, exists := signal.Labels[key]; !exists || signalValue != value {
			return nil
		}
	}

	cutoff := signal.ReceivedAt.Add(-e.config.TimeWindow)

	for _, group := range e.groups {
		if group.UpdatedAt.Before(cutoff) || len(group.Signals) >= e.config.MaxGroupSize {
			continue
		}

		// Check if group contains signals with matching labels
		if len(group.Signals) > 0 {
			firstSignal := group.Signals[0]
			matches := true
			for key, value := range rule.MatchLabels {
				if signalValue, exists := firstSignal.Labels[key]; !exists || signalValue != value {
					matches = false
					break
				}
			}
			if matches {
				return group
			}
		}
	}
	return nil
}

// findFingerprintBasedGroup finds a group based on content fingerprint.
func (e *CorrelationEngine) findFingerprintBasedGroup(signal domain.Signal, rule CorrelationRule) *SignalGroup {
	fingerprint := e.createContentFingerprint(signal)
	if groupID, exists := e.fingerprints[fingerprint]; exists {
		if group, exists := e.groups[groupID]; exists {
			cutoff := signal.ReceivedAt.Add(-e.config.TimeWindow)
			if group.UpdatedAt.After(cutoff) && len(group.Signals) < e.config.MaxGroupSize {
				return group
			}
		}
	}
	return nil
}

// createNewGroup creates a new signal group with the given signal.
func (e *CorrelationEngine) createNewGroup(signal domain.Signal) *SignalGroup {
	groupID := e.generateGroupID()
	now := time.Now()

	group := &SignalGroup{
		ID:          groupID,
		Fingerprint: e.createContentFingerprint(signal),
		Signals:     []domain.Signal{signal},
		Rules:       []string{"initial"},
		CreatedAt:   now,
		UpdatedAt:   now,
		Labels:      e.extractGroupLabels(signal),
	}

	e.groups[groupID] = group
	e.fingerprints[group.Fingerprint] = groupID

	return group
}

// addSignalToGroup adds a signal to an existing group.
func (e *CorrelationEngine) addSignalToGroup(group *SignalGroup, signal domain.Signal, ruleName string) error {
	if len(group.Signals) >= e.config.MaxGroupSize {
		return fmt.Errorf("group has reached maximum size")
	}

	group.Signals = append(group.Signals, signal)
	group.UpdatedAt = time.Now()

	// Add rule name if not already present
	for _, rule := range group.Rules {
		if rule == ruleName {
			return nil
		}
	}
	group.Rules = append(group.Rules, ruleName)

	return nil
}

// createGroupKey creates a key for grouping based on specified attributes.
func (e *CorrelationEngine) createGroupKey(signal domain.Signal, groupBy []string) string {
	var parts []string
	for _, key := range groupBy {
		if value, exists := signal.Labels[key]; exists {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	sort.Strings(parts) // Ensure consistent ordering
	return strings.Join(parts, "|")
}

// createSignalFingerprint creates a fingerprint for deduplication.
func (e *CorrelationEngine) createSignalFingerprint(signal domain.Signal) string {
	h := sha256.New()
	h.Write([]byte(signal.Source))
	h.Write([]byte(signal.Title))
	h.Write([]byte(signal.Severity.String()))

	// Add sorted labels for consistency
	var labelPairs []string
	for k, v := range signal.Labels {
		labelPairs = append(labelPairs, fmt.Sprintf("%s=%s", k, v))
	}
	sort.Strings(labelPairs)
	for _, pair := range labelPairs {
		h.Write([]byte(pair))
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Use first 16 chars for efficiency
}

// createContentFingerprint creates a fingerprint based on signal content.
func (e *CorrelationEngine) createContentFingerprint(signal domain.Signal) string {
	h := sha256.New()
	h.Write([]byte(signal.Source))
	h.Write([]byte(signal.Title))
	h.Write([]byte(signal.Description))

	// Add service-related labels
	serviceLabels := []string{"service.name", "service.namespace", "service.version"}
	for _, label := range serviceLabels {
		if value, exists := signal.Labels[label]; exists {
			h.Write([]byte(fmt.Sprintf("%s=%s", label, value)))
		}
	}

	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// extractGroupLabels extracts relevant labels for group metadata.
func (e *CorrelationEngine) extractGroupLabels(signal domain.Signal) map[string]string {
	labels := make(map[string]string)

	// Copy important labels
	importantLabels := []string{
		"service.name", "service.namespace", "service.version",
		"team", "environment", "severity",
	}

	for _, key := range importantLabels {
		if value, exists := signal.Labels[key]; exists {
			labels[key] = value
		}
	}

	return labels
}

// generateGroupID generates a unique group identifier.
func (e *CorrelationEngine) generateGroupID() string {
	return fmt.Sprintf("grp-%d", time.Now().UnixNano())
}

// cleanupWorker runs periodic cleanup of old groups.
func (e *CorrelationEngine) cleanupWorker() {
	defer close(e.cleanupDone)

	ticker := time.NewTicker(e.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.cleanup()
		case <-e.stopCleanup:
			return
		}
	}
}

// cleanup removes old groups and cache entries.
func (e *CorrelationEngine) cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-e.config.GroupTTL)

	// Clean up old groups
	for groupID, group := range e.groups {
		if group.UpdatedAt.Before(cutoff) {
			delete(e.groups, groupID)
			// Remove fingerprint mapping
			delete(e.fingerprints, group.Fingerprint)
			e.metrics.ActiveGroups--
			e.logger.Debug("Cleaned up old group", "group_id", groupID)
		}
	}

	// Clean up old deduplication cache entries
	dedupeCutoff := now.Add(-e.config.TimeWindow * 2) // Keep longer than time window
	for fingerprint, lastSeen := range e.dedupeCache {
		if lastSeen.Before(dedupeCutoff) {
			delete(e.dedupeCache, fingerprint)
		}
	}

	e.logger.Debug("Correlation cleanup completed",
		"active_groups", len(e.groups),
		"fingerprints", len(e.fingerprints),
		"dedupe_cache", len(e.dedupeCache))
}
