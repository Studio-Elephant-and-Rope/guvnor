// Package detection provides signal-to-incident detection rules engine.
//
// The detection engine evaluates incoming signals against configurable rules
// to automatically create incidents when thresholds are breached. It integrates
// with the correlation engine to process grouped signals and implements
// cooldown periods to prevent incident spam.
package detection

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/domain"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/core/services"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/correlation"
	"github.com/Studio-Elephant-and-Rope/guvnor/internal/logging"
)

// RuleType defines the type of detection rule.
type RuleType string

const (
	// RuleTypeErrorRate detects high error rates over time windows
	RuleTypeErrorRate RuleType = "error_rate"
	// RuleTypeMetricThreshold detects metric values exceeding thresholds
	RuleTypeMetricThreshold RuleType = "metric_threshold"
	// RuleTypeLogPattern detects specific patterns in log messages
	RuleTypeLogPattern RuleType = "log_pattern"
	// RuleTypeTraceAnomaly detects anomalies in trace data
	RuleTypeTraceAnomaly RuleType = "trace_anomaly"
)

// DetectionRule defines a rule for detecting incidents from signals.
type DetectionRule struct {
	// Name is a unique identifier for the rule
	Name string `yaml:"name" json:"name"`
	// Type specifies the kind of detection rule
	Type RuleType `yaml:"type" json:"type"`
	// Condition defines the triggering condition in simple DSL
	Condition string `yaml:"condition" json:"condition"`
	// Severity is the severity level for created incidents
	Severity domain.Severity `yaml:"severity" json:"severity"`
	// TeamID is the team to assign incidents to
	TeamID string `yaml:"team_id" json:"team_id"`
	// ServiceID is the service this rule applies to (optional)
	ServiceID string `yaml:"service_id,omitempty" json:"service_id,omitempty"`
	// Labels are additional labels for filtering signals
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	// Cooldown is the minimum time between incidents for this rule
	Cooldown time.Duration `yaml:"cooldown" json:"cooldown"`
	// Enabled determines if the rule is active
	Enabled bool `yaml:"enabled" json:"enabled"`
	// Priority determines evaluation order (higher = evaluated first)
	Priority int `yaml:"priority" json:"priority"`

	// Parsed condition components (set during initialization)
	parsedCondition *ParsedCondition `yaml:"-" json:"-"`
}

// ParsedCondition represents a parsed rule condition.
type ParsedCondition struct {
	Metric     string         `json:"metric"`
	Operator   string         `json:"operator"`
	Threshold  float64        `json:"threshold"`
	TimeWindow time.Duration  `json:"time_window,omitempty"`
	Pattern    *regexp.Regexp `json:"-"`
	Unit       string         `json:"unit,omitempty"`
}

// DetectionConfig contains configuration for the detection engine.
type DetectionConfig struct {
	// Rules define the detection rules to evaluate
	Rules []DetectionRule `yaml:"rules" json:"rules"`
	// EvaluationInterval is how often to evaluate time-window rules
	EvaluationInterval time.Duration `yaml:"evaluation_interval" json:"evaluation_interval"`
	// MaxRulesPerSignal limits rule evaluation for performance
	MaxRulesPerSignal int `yaml:"max_rules_per_signal" json:"max_rules_per_signal"`
	// DefaultCooldown is used when rule doesn't specify one
	DefaultCooldown time.Duration `yaml:"default_cooldown" json:"default_cooldown"`
	// AutoCreateIncidents determines if incidents are automatically created
	AutoCreateIncidents bool `yaml:"auto_create_incidents" json:"auto_create_incidents"`
}

// DefaultDetectionConfig returns sensible defaults for the detection engine.
func DefaultDetectionConfig() DetectionConfig {
	return DetectionConfig{
		Rules:               []DetectionRule{},
		EvaluationInterval:  30 * time.Second,
		MaxRulesPerSignal:   50,
		DefaultCooldown:     5 * time.Minute,
		AutoCreateIncidents: true,
	}
}

// RuleMatch represents a rule that matched a signal or signal group.
type RuleMatch struct {
	Rule      DetectionRule            `json:"rule"`
	Signal    *domain.Signal           `json:"signal,omitempty"`
	Group     *correlation.SignalGroup `json:"group,omitempty"`
	Value     float64                  `json:"value,omitempty"`
	Metadata  map[string]interface{}   `json:"metadata,omitempty"`
	MatchedAt time.Time                `json:"matched_at"`
}

// DetectionMetrics tracks detection engine performance.
type DetectionMetrics struct {
	SignalsProcessed   int64         `json:"signals_processed"`
	RulesEvaluated     int64         `json:"rules_evaluated"`
	RuleMatches        int64         `json:"rule_matches"`
	IncidentsCreated   int64         `json:"incidents_created"`
	CooldownsActive    int64         `json:"cooldowns_active"`
	AverageEvalTime    time.Duration `json:"average_eval_time"`
	LastEvaluationTime time.Time     `json:"last_evaluation_time"`
}

// Detector provides signal-to-incident detection functionality.
type Detector struct {
	config          DetectionConfig
	rules           []DetectionRule
	correlator      *correlation.CorrelationEngine
	incidentService *services.IncidentService
	logger          *logging.Logger
	metrics         DetectionMetrics

	// State tracking
	cooldownState map[string]time.Time           // rule_name -> last incident time
	signalHistory map[string][]timestampedSignal // for time-window rules
	mu            sync.RWMutex

	// Background evaluation
	stopEval chan struct{}
	evalDone chan struct{}
}

// timestampedSignal wraps a signal with its processing timestamp.
type timestampedSignal struct {
	Signal    domain.Signal
	Timestamp time.Time
}

// NewDetector creates a new detection engine with the specified configuration.
func NewDetector(
	config DetectionConfig,
	correlator *correlation.CorrelationEngine,
	incidentService *services.IncidentService,
	logger *logging.Logger,
) (*Detector, error) {
	if correlator == nil {
		return nil, fmt.Errorf("correlation engine is required")
	}

	if incidentService == nil {
		return nil, fmt.Errorf("incident service is required")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	// Parse and validate rules
	rules := make([]DetectionRule, 0, len(config.Rules))
	for _, rule := range config.Rules {
		if !rule.Enabled {
			continue
		}

		// Parse rule condition
		parsed, err := parseCondition(rule.Condition, rule.Type)
		if err != nil {
			logger.Error("Failed to parse rule condition",
				"rule", rule.Name,
				"condition", rule.Condition,
				"error", err)
			continue
		}

		rule.parsedCondition = parsed

		// Set default cooldown if not specified
		if rule.Cooldown == 0 {
			rule.Cooldown = config.DefaultCooldown
		}

		// Validate required fields
		if err := validateRule(rule); err != nil {
			logger.Error("Invalid rule configuration",
				"rule", rule.Name,
				"error", err)
			continue
		}

		rules = append(rules, rule)
		logger.Debug("Loaded detection rule",
			"rule", rule.Name,
			"type", rule.Type,
			"condition", rule.Condition)
	}

	// Sort rules by priority (descending)
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority < rules[j].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	detector := &Detector{
		config:          config,
		rules:           rules,
		correlator:      correlator,
		incidentService: incidentService,
		logger:          logger,
		cooldownState:   make(map[string]time.Time),
		signalHistory:   make(map[string][]timestampedSignal),
		stopEval:        make(chan struct{}),
		evalDone:        make(chan struct{}),
	}

	logger.Info("Detection engine initialized",
		"rules_loaded", len(rules),
		"total_rules", len(config.Rules),
		"evaluation_interval", config.EvaluationInterval)

	return detector, nil
}

// Start begins background evaluation of time-window rules.
func (d *Detector) Start(ctx context.Context) error {
	d.logger.Info("Starting detection engine background evaluation")

	go d.evaluationWorker(ctx)

	return nil
}

// Stop gracefully shuts down the detection engine.
func (d *Detector) Stop(ctx context.Context) error {
	d.logger.Info("Stopping detection engine")

	close(d.stopEval)

	select {
	case <-d.evalDone:
		d.logger.Info("Detection engine stopped gracefully")
	case <-ctx.Done():
		d.logger.Warn("Detection engine shutdown timeout exceeded")
		return ctx.Err()
	}

	return nil
}

// ProcessSignal evaluates a single signal against all applicable rules.
func (d *Detector) ProcessSignal(ctx context.Context, signal domain.Signal) ([]RuleMatch, error) {
	start := time.Now()
	defer func() {
		d.mu.Lock()
		d.metrics.SignalsProcessed++
		d.metrics.AverageEvalTime = time.Since(start)
		d.mu.Unlock()
	}()

	d.logger.Debug("Processing signal for detection",
		"signal_id", signal.ID,
		"source", signal.Source,
		"severity", signal.Severity)

	// Store signal for time-window evaluation
	d.storeSignalForTimeWindow(signal)

	var matches []RuleMatch
	rulesEvaluated := 0

	// Evaluate rules against signal
	for _, rule := range d.rules {
		if rulesEvaluated >= d.config.MaxRulesPerSignal {
			d.logger.Debug("Reached max rules per signal limit",
				"signal_id", signal.ID,
				"rules_evaluated", rulesEvaluated)
			break
		}

		// Check if rule applies to this signal
		if !d.signalMatchesRule(signal, rule) {
			continue
		}

		// Check cooldown
		if d.isInCooldown(rule.Name) {
			d.logger.Debug("Rule in cooldown, skipping",
				"rule", rule.Name,
				"signal_id", signal.ID)
			continue
		}

		// Evaluate rule condition
		match, err := d.evaluateRule(ctx, rule, signal, nil)
		if err != nil {
			d.logger.Error("Failed to evaluate rule",
				"rule", rule.Name,
				"signal_id", signal.ID,
				"error", err)
			continue
		}

		rulesEvaluated++

		if match != nil {
			matches = append(matches, *match)
			d.logger.Info("Rule matched signal",
				"rule", rule.Name,
				"signal_id", signal.ID,
				"value", match.Value)
		}
	}

	d.mu.Lock()
	d.metrics.RulesEvaluated += int64(rulesEvaluated)
	d.metrics.RuleMatches += int64(len(matches))
	d.mu.Unlock()

	// Create incidents from matches
	if d.config.AutoCreateIncidents {
		for _, match := range matches {
			if err := d.createIncidentFromMatch(ctx, match); err != nil {
				d.logger.Error("Failed to create incident from rule match",
					"rule", match.Rule.Name,
					"error", err)
			}
		}
	}

	return matches, nil
}

// ProcessSignalGroup evaluates a correlated signal group against applicable rules.
func (d *Detector) ProcessSignalGroup(ctx context.Context, group *correlation.SignalGroup) ([]RuleMatch, error) {
	start := time.Now()
	defer func() {
		d.metrics.AverageEvalTime = time.Since(start)
	}()

	d.logger.Debug("Processing signal group for detection",
		"group_id", group.ID,
		"signal_count", len(group.Signals))

	var matches []RuleMatch

	// Evaluate group-level rules
	for _, rule := range d.rules {
		// Check if rule applies to this group
		if !d.groupMatchesRule(*group, rule) {
			continue
		}

		// Check cooldown
		if d.isInCooldown(rule.Name) {
			continue
		}

		// Evaluate rule condition against group
		match, err := d.evaluateRule(ctx, rule, domain.Signal{}, group)
		if err != nil {
			d.logger.Error("Failed to evaluate rule against group",
				"rule", rule.Name,
				"group_id", group.ID,
				"error", err)
			continue
		}

		if match != nil {
			matches = append(matches, *match)
			d.logger.Info("Rule matched signal group",
				"rule", rule.Name,
				"group_id", group.ID,
				"value", match.Value)
		}
	}

	d.mu.Lock()
	d.metrics.RuleMatches += int64(len(matches))
	d.mu.Unlock()

	// Create incidents from matches
	if d.config.AutoCreateIncidents {
		for _, match := range matches {
			if err := d.createIncidentFromMatch(ctx, match); err != nil {
				d.logger.Error("Failed to create incident from group match",
					"rule", match.Rule.Name,
					"error", err)
			}
		}
	}

	return matches, nil
}

// GetMetrics returns current detection engine metrics.
func (d *Detector) GetMetrics() DetectionMetrics {
	d.mu.RLock()
	defer d.mu.RUnlock()

	d.metrics.CooldownsActive = int64(len(d.cooldownState))
	d.metrics.LastEvaluationTime = time.Now()

	return d.metrics
}

// evaluationWorker runs background evaluation for time-window rules.
func (d *Detector) evaluationWorker(ctx context.Context) {
	defer close(d.evalDone)

	ticker := time.NewTicker(d.config.EvaluationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.evaluateTimeWindowRules(ctx)
		case <-d.stopEval:
			return
		case <-ctx.Done():
			return
		}
	}
}

// evaluateTimeWindowRules evaluates rules that require time windows.
func (d *Detector) evaluateTimeWindowRules(ctx context.Context) {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Clean up old signal history first
	d.cleanupSignalHistory(now)

	// Evaluate time-window rules
	for _, rule := range d.rules {
		if rule.Type != RuleTypeErrorRate {
			continue
		}

		if d.isInCooldown(rule.Name) {
			continue
		}

		// Count signals in time window
		windowStart := now.Add(-rule.parsedCondition.TimeWindow)
		count := d.countSignalsInWindow(rule, windowStart, now)

		if float64(count) >= rule.parsedCondition.Threshold {
			match := RuleMatch{
				Rule:      rule,
				Value:     float64(count),
				MatchedAt: now,
				Metadata: map[string]interface{}{
					"time_window": rule.parsedCondition.TimeWindow.String(),
					"count":       count,
				},
			}

			d.logger.Info("Time-window rule matched",
				"rule", rule.Name,
				"count", count,
				"threshold", rule.parsedCondition.Threshold)

			if d.config.AutoCreateIncidents {
				if err := d.createIncidentFromMatch(ctx, match); err != nil {
					d.logger.Error("Failed to create incident from time-window match",
						"rule", rule.Name,
						"error", err)
				}
			}
		}
	}
}

// Additional helper methods would continue here...
// (I'll implement the rest in the next part due to length)

// parseCondition parses a rule condition string into a structured format.
func parseCondition(condition string, ruleType RuleType) (*ParsedCondition, error) {
	condition = strings.TrimSpace(condition)

	switch ruleType {
	case RuleTypeErrorRate:
		return parseErrorRateCondition(condition)
	case RuleTypeMetricThreshold:
		return parseMetricThresholdCondition(condition)
	case RuleTypeLogPattern:
		return parseLogPatternCondition(condition)
	case RuleTypeTraceAnomaly:
		return parseTraceAnomalyCondition(condition)
	default:
		return nil, fmt.Errorf("unsupported rule type: %s", ruleType)
	}
}

// parseErrorRateCondition parses error rate conditions like "error_rate > 10/min".
func parseErrorRateCondition(condition string) (*ParsedCondition, error) {
	// Pattern: metric operator threshold/unit
	parts := strings.Fields(condition)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid error rate condition format: %s", condition)
	}

	metric := parts[0]
	operator := parts[1]
	thresholdPart := parts[2]

	// Parse threshold and time unit
	if !strings.Contains(thresholdPart, "/") {
		return nil, fmt.Errorf("error rate condition must specify time unit: %s", thresholdPart)
	}

	thresholdStr := strings.Split(thresholdPart, "/")[0]
	unit := strings.Split(thresholdPart, "/")[1]

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid threshold value: %s", thresholdStr)
	}

	var timeWindow time.Duration
	switch unit {
	case "sec", "s":
		timeWindow = time.Second
	case "min", "m":
		timeWindow = time.Minute
	case "hour", "h":
		timeWindow = time.Hour
	default:
		return nil, fmt.Errorf("unsupported time unit: %s", unit)
	}

	return &ParsedCondition{
		Metric:     metric,
		Operator:   operator,
		Threshold:  threshold,
		TimeWindow: timeWindow,
		Unit:       unit,
	}, nil
}

// parseMetricThresholdCondition parses metric threshold conditions like "cpu > 90".
func parseMetricThresholdCondition(condition string) (*ParsedCondition, error) {
	parts := strings.Fields(condition)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid metric threshold condition format: %s", condition)
	}

	metric := parts[0]
	operator := parts[1]
	thresholdStr := parts[2]

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid threshold value: %s", thresholdStr)
	}

	return &ParsedCondition{
		Metric:    metric,
		Operator:  operator,
		Threshold: threshold,
	}, nil
}

// parseLogPatternCondition parses log pattern conditions.
func parseLogPatternCondition(condition string) (*ParsedCondition, error) {
	// Extract regex pattern from condition
	if !strings.HasPrefix(condition, "pattern:") {
		return nil, fmt.Errorf("log pattern condition must start with 'pattern:': %s", condition)
	}

	patternStr := strings.TrimPrefix(condition, "pattern:")
	patternStr = strings.TrimSpace(patternStr)

	pattern, err := regexp.Compile(patternStr)
	if err != nil {
		return nil, fmt.Errorf("invalid regex pattern: %s", err)
	}

	return &ParsedCondition{
		Pattern: pattern,
	}, nil
}

// parseTraceAnomalyCondition parses trace anomaly conditions.
func parseTraceAnomalyCondition(condition string) (*ParsedCondition, error) {
	parts := strings.Fields(condition)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid trace anomaly condition format: %s", condition)
	}

	metric := parts[0]
	operator := parts[1]
	thresholdStr := parts[2]

	threshold, err := strconv.ParseFloat(thresholdStr, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid threshold value: %s", thresholdStr)
	}

	return &ParsedCondition{
		Metric:    metric,
		Operator:  operator,
		Threshold: threshold,
	}, nil
}

// validateRule validates a detection rule configuration.
func validateRule(rule DetectionRule) error {
	if rule.Name == "" {
		return fmt.Errorf("rule name is required")
	}

	if rule.TeamID == "" {
		return fmt.Errorf("team_id is required")
	}

	if !rule.Severity.IsValid() {
		return fmt.Errorf("invalid severity: %s", rule.Severity)
	}

	if rule.Cooldown < 0 {
		return fmt.Errorf("cooldown cannot be negative")
	}

	if rule.parsedCondition == nil {
		return fmt.Errorf("failed to parse rule condition")
	}

	return nil
}

// signalMatchesRule checks if a signal matches the rule's filter criteria.
func (d *Detector) signalMatchesRule(signal domain.Signal, rule DetectionRule) bool {
	// Check service filter
	if rule.ServiceID != "" {
		if serviceID, exists := signal.Labels["service.name"]; !exists || serviceID != rule.ServiceID {
			return false
		}
	}

	// Check label filters
	for key, value := range rule.Labels {
		if signalValue, exists := signal.Labels[key]; !exists || signalValue != value {
			return false
		}
	}

	return true
}

// groupMatchesRule checks if a signal group matches the rule's filter criteria.
func (d *Detector) groupMatchesRule(group correlation.SignalGroup, rule DetectionRule) bool {
	// For now, check if any signal in the group matches
	for _, signal := range group.Signals {
		if d.signalMatchesRule(signal, rule) {
			return true
		}
	}
	return false
}

// isInCooldown checks if a rule is currently in cooldown period.
func (d *Detector) isInCooldown(ruleName string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	lastIncident, exists := d.cooldownState[ruleName]
	if !exists {
		return false
	}

	// Find the rule to get its cooldown period
	var cooldown time.Duration
	for _, rule := range d.rules {
		if rule.Name == ruleName {
			cooldown = rule.Cooldown
			break
		}
	}

	return time.Since(lastIncident) < cooldown
}

// setCooldown sets the cooldown state for a rule.
func (d *Detector) setCooldown(ruleName string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cooldownState[ruleName] = time.Now()
}

// storeSignalForTimeWindow stores a signal for time-window rule evaluation.
func (d *Detector) storeSignalForTimeWindow(signal domain.Signal) {
	d.mu.Lock()
	defer d.mu.Unlock()

	key := fmt.Sprintf("%s:%s", signal.Source, signal.Labels["service.name"])
	d.signalHistory[key] = append(d.signalHistory[key], timestampedSignal{
		Signal:    signal,
		Timestamp: time.Now(),
	})
}

// cleanupSignalHistory removes old signals from history.
func (d *Detector) cleanupSignalHistory(now time.Time) {
	maxAge := 24 * time.Hour // Keep signals for max 24 hours
	cutoff := now.Add(-maxAge)

	for key, signals := range d.signalHistory {
		var filtered []timestampedSignal
		for _, ts := range signals {
			if ts.Timestamp.After(cutoff) {
				filtered = append(filtered, ts)
			}
		}
		d.signalHistory[key] = filtered
	}
}

// countSignalsInWindow counts signals matching a rule within a time window.
func (d *Detector) countSignalsInWindow(rule DetectionRule, start, end time.Time) int {
	count := 0

	for _, signals := range d.signalHistory {
		for _, ts := range signals {
			if ts.Timestamp.After(start) && ts.Timestamp.Before(end) {
				if d.signalMatchesRule(ts.Signal, rule) {
					count++
				}
			}
		}
	}

	return count
}

// evaluateRule evaluates a rule against a signal or signal group.
func (d *Detector) evaluateRule(ctx context.Context, rule DetectionRule, signal domain.Signal, group *correlation.SignalGroup) (*RuleMatch, error) {
	switch rule.Type {
	case RuleTypeErrorRate:
		// Error rate rules are handled in background evaluation
		return nil, nil

	case RuleTypeMetricThreshold:
		return d.evaluateMetricThreshold(rule, signal, group)

	case RuleTypeLogPattern:
		return d.evaluateLogPattern(rule, signal, group)

	case RuleTypeTraceAnomaly:
		return d.evaluateTraceAnomaly(rule, signal, group)

	default:
		return nil, fmt.Errorf("unsupported rule type: %s", rule.Type)
	}
}

// evaluateMetricThreshold evaluates metric threshold rules.
func (d *Detector) evaluateMetricThreshold(rule DetectionRule, signal domain.Signal, group *correlation.SignalGroup) (*RuleMatch, error) {
	var value float64
	var found bool

	// Extract metric value from signal or group
	if group != nil {
		// For groups, take the maximum value
		for _, s := range group.Signals {
			if v, ok := extractMetricValue(s, rule.parsedCondition.Metric); ok {
				if !found || v > value {
					value = v
					found = true
				}
			}
		}
	} else {
		value, found = extractMetricValue(signal, rule.parsedCondition.Metric)
	}

	if !found {
		return nil, nil
	}

	// Evaluate condition
	matched := false
	switch rule.parsedCondition.Operator {
	case ">":
		matched = value > rule.parsedCondition.Threshold
	case ">=":
		matched = value >= rule.parsedCondition.Threshold
	case "<":
		matched = value < rule.parsedCondition.Threshold
	case "<=":
		matched = value <= rule.parsedCondition.Threshold
	case "==", "=":
		matched = value == rule.parsedCondition.Threshold
	}

	if matched {
		match := &RuleMatch{
			Rule:      rule,
			Value:     value,
			MatchedAt: time.Now(),
			Metadata: map[string]interface{}{
				"metric":    rule.parsedCondition.Metric,
				"threshold": rule.parsedCondition.Threshold,
				"operator":  rule.parsedCondition.Operator,
			},
		}

		if group != nil {
			match.Group = group
		} else {
			match.Signal = &signal
		}

		return match, nil
	}

	return nil, nil
}

// evaluateLogPattern evaluates log pattern matching rules.
func (d *Detector) evaluateLogPattern(rule DetectionRule, signal domain.Signal, group *correlation.SignalGroup) (*RuleMatch, error) {
	var matched bool
	var matchedText string

	if group != nil {
		// Check all signals in group
		for _, s := range group.Signals {
			if rule.parsedCondition.Pattern.MatchString(s.Description) {
				matched = true
				matchedText = s.Description
				break
			}
		}
	} else {
		if rule.parsedCondition.Pattern.MatchString(signal.Description) {
			matched = true
			matchedText = signal.Description
		}
	}

	if matched {
		match := &RuleMatch{
			Rule:      rule,
			MatchedAt: time.Now(),
			Metadata: map[string]interface{}{
				"pattern":      rule.parsedCondition.Pattern.String(),
				"matched_text": matchedText,
			},
		}

		if group != nil {
			match.Group = group
		} else {
			match.Signal = &signal
		}

		return match, nil
	}

	return nil, nil
}

// evaluateTraceAnomaly evaluates trace anomaly detection rules.
func (d *Detector) evaluateTraceAnomaly(rule DetectionRule, signal domain.Signal, group *correlation.SignalGroup) (*RuleMatch, error) {
	// Implementation would analyze trace data for anomalies
	// For now, return nil as this requires more complex trace analysis
	return nil, nil
}

// extractMetricValue extracts a numeric metric value from a signal.
func extractMetricValue(signal domain.Signal, metricName string) (float64, bool) {
	// Check signal payload for metric
	if value, exists := signal.Payload[metricName]; exists {
		switch v := value.(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f, true
			}
		}
	}

	// Check signal labels for metric
	if value, exists := signal.Labels[metricName]; exists {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f, true
		}
	}

	return 0, false
}

// createIncidentFromMatch creates an incident from a rule match.
func (d *Detector) createIncidentFromMatch(ctx context.Context, match RuleMatch) error {
	title := d.generateIncidentTitle(match)

	incident, err := d.incidentService.CreateIncident(ctx, title, string(match.Rule.Severity), match.Rule.TeamID)
	if err != nil {
		return fmt.Errorf("failed to create incident: %w", err)
	}

	// Set cooldown for this rule
	d.setCooldown(match.Rule.Name)

	d.mu.Lock()
	d.metrics.IncidentsCreated++
	d.mu.Unlock()

	d.logger.Info("Created incident from detection rule",
		"incident_id", incident.ID,
		"rule", match.Rule.Name,
		"title", title)

	return nil
}

// generateIncidentTitle creates a descriptive title for an incident.
func (d *Detector) generateIncidentTitle(match RuleMatch) string {
	switch match.Rule.Type {
	case RuleTypeErrorRate:
		return fmt.Sprintf("High error rate detected: %s (%.0f errors)", match.Rule.Name, match.Value)
	case RuleTypeMetricThreshold:
		return fmt.Sprintf("Metric threshold exceeded: %s (%.2f)", match.Rule.Name, match.Value)
	case RuleTypeLogPattern:
		return fmt.Sprintf("Log pattern detected: %s", match.Rule.Name)
	case RuleTypeTraceAnomaly:
		return fmt.Sprintf("Trace anomaly detected: %s", match.Rule.Name)
	default:
		return fmt.Sprintf("Detection rule triggered: %s", match.Rule.Name)
	}
}
