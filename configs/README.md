# Guvnor Configuration Examples

This directory contains example configurations for various Guvnor components.

## Correlation Engine Examples

The correlation engine groups incoming signals based on configurable rules. Here are example configurations for different scenarios:

### 📁 `correlation-basic.example.yml`
- **Use Case**: Simple setup for most environments
- **Features**: Basic service grouping, critical alert separation, time-based fallback
- **Best For**: Getting started, small to medium deployments

### 📁 `correlation-advanced.example.yml`
- **Use Case**: Production environments with complex requirements
- **Features**: Multiple team-based rules, regional grouping, priority-based correlation
- **Best For**: Large organisations, multi-team environments

### 📁 `correlation-kubernetes.example.yml`
- **Use Case**: Kubernetes-native environments
- **Features**: Pod/node correlation, namespace grouping, container-specific rules
- **Best For**: Cloud-native applications, microservices architectures

## Configuration Structure

All correlation configurations follow this structure:

```yaml
correlation:
  # Global settings
  time_window: 5m          # How long signals can be grouped together
  max_group_size: 50       # Maximum signals per group
  cleanup_interval: 10m    # How often to clean up old groups
  group_ttl: 30m          # How long to keep inactive groups
  enable_deduplication: true  # Remove exact duplicate signals

  # Correlation rules (processed by priority - highest first)
  rules:
    - name: "rule-name"
      type: "service|labels|fingerprint|time"
      priority: 100         # Higher = processed first
      enabled: true          # Can disable without removing

      # For service-based rules:
      group_by: ["service.name", "environment"]

      # For label-based rules:
      match_labels:
        severity: "critical"
        team: "platform"
```

## Rule Types

### 🔄 **Service-based** (`type: "service"`)
Groups signals from the same service/component:
```yaml
- name: "service-correlation"
  type: "service"
  group_by: ["service.name", "environment"]
  priority: 100
```

### 🏷️ **Label-based** (`type: "labels"`)
Groups signals with exact label matches:
```yaml
- name: "critical-alerts"
  type: "labels"
  match_labels:
    severity: "critical"
  priority: 200
```

### 📊 **Fingerprint-based** (`type: "fingerprint"`)
Groups signals with similar content patterns:
```yaml
- name: "error-patterns"
  type: "fingerprint"
  priority: 150
```

### ⏰ **Time-based** (`type: "time"`)
Groups any signals within the time window:
```yaml
- name: "time-fallback"
  type: "time"
  priority: 10
```

## Priority Guidelines

- **300+**: Emergency/critical alerts requiring immediate attention
- **200-299**: Team-specific or high-priority grouping
- **100-199**: Standard service/component correlation
- **50-99**: Environment-specific rules
- **1-49**: Fallback and catch-all rules

## Getting Started

1. Choose the example closest to your environment
2. Copy the example file to your actual config:
   ```bash
   cp configs/correlation-basic.example.yml configs/correlation.yml
   ```
3. Adjust the rules for your specific labels and services
4. Test with a few signals before rolling out
5. Monitor correlation effectiveness and adjust priorities

> **Note**: The `.example.yml` files are templates. Copy them to your actual configuration files (e.g., `correlation.yml`) and customize as needed. Your actual config files should be added to `.gitignore` to avoid committing sensitive configuration.

## Tips

- **Start Simple**: Begin with `correlation-basic.example.yml` and add complexity as needed
- **Test Priorities**: Higher priority rules are checked first - ensure critical rules have high priorities
- **Monitor Performance**: Watch group sizes and cleanup frequency
- **Label Consistency**: Ensure your signal sources use consistent label names
- **Time Windows**: Shorter windows = more groups, longer windows = larger groups
