# Telemetry and OpenTelemetry Integration

Guvnor provides native OpenTelemetry integration for incident detection and signal processing. This allows you to ingest telemetry data from OpenTelemetry collectors and automatically detect incidents based on configurable thresholds and patterns.

## Features

- **OTLP Receiver**: Native support for OpenTelemetry Protocol (OTLP) over gRPC and HTTP
- **Signal Processing**: Automatic incident detection from metrics, traces, and logs
- **Configurable Thresholds**: Customizable error rates, latency, and resource utilisation limits
- **Pattern Matching**: Log analysis for error conditions and anomalies
- **Rate Limiting**: Prevents signal flooding and ensures system stability

## Configuration

### Receiver Configuration

Enable the OTLP receiver in your Guvnor configuration:

```yaml
telemetry:
  enabled: true
  service_name: "guvnor"
  service_version: "1.0.0"
  endpoint: "http://jaeger:14268/api/traces"  # Optional: export Guvnor's own traces
  sample_rate: 0.1
  receiver:
    enabled: true
    grpc_port: 4317
    http_port: 4318
    grpc_host: "0.0.0.0"
    http_host: "0.0.0.0"
```

### Environment Variables

You can also configure the receiver using environment variables:

```bash
export GUVNOR_TELEMETRY_RECEIVER_ENABLED=true
export GUVNOR_TELEMETRY_RECEIVER_GRPC_PORT=4317
export GUVNOR_TELEMETRY_RECEIVER_HTTP_PORT=4318
export GUVNOR_TELEMETRY_RECEIVER_GRPC_HOST=0.0.0.0
export GUVNOR_TELEMETRY_RECEIVER_HTTP_HOST=0.0.0.0
```

### Signal Processing Configuration

Configure thresholds for automatic incident detection:

```yaml
signal_processor:
  error_rate_threshold: 5.0        # Error rate percentage (5%)
  latency_threshold_ms: 1000       # Response time threshold
  cpu_threshold_percent: 80.0      # CPU utilisation threshold
  memory_threshold_percent: 85.0   # Memory utilisation threshold
  error_patterns:                  # Log patterns that indicate errors
    - "ERROR"
    - "FATAL"
    - "panic:"
    - "exception"
  rate_limit:
    signals_per_minute: 100        # Maximum signals per minute
    burst_size: 20                 # Burst allowance
```

## OpenTelemetry Collector Integration

### Basic Setup

1. **Start Guvnor with receiver enabled:**
   ```bash
   export GUVNOR_TELEMETRY_RECEIVER_ENABLED=true
   ./guvnor serve
   ```

2. **Configure OpenTelemetry Collector:**
   Use the provided example configuration in `configs/otel-collector.example.yaml`:
   ```bash
   otelcol-contrib --config=otel-collector.example.yaml
   ```

### Collector Configuration

The collector should be configured to send data to Guvnor's OTLP endpoints:

```yaml
exporters:
  otlp/guvnor:
    endpoint: http://localhost:4317  # Guvnor's gRPC OTLP receiver
    tls:
      insecure: true
    headers:
      x-source: "otel-collector"
      x-environment: "production"

  otlphttp/guvnor:
    endpoint: http://localhost:4318  # Guvnor's HTTP OTLP receiver
    tls:
      insecure: true
```

## Signal Processing

Guvnor automatically analyses incoming telemetry data and generates incidents based on:

### Metrics Processing

- **Error Rate**: Tracks request failure rates and triggers incidents when thresholds are exceeded
- **Latency**: Monitors response times and detects performance degradation
- **Resource Utilisation**: Monitors CPU, memory, disk, and network usage
- **Custom Metrics**: Configurable thresholds for application-specific metrics

### Trace Processing

- **Error Spans**: Detects failed operations and error conditions
- **High Latency**: Identifies slow operations and bottlenecks
- **Service Dependencies**: Analyses service call patterns and failures

### Log Processing

- **Error Patterns**: Searches for configured error patterns in log messages
- **Severity Levels**: Processes log severity and generates appropriate alerts
- **Structured Logs**: Extracts metadata from structured log entries

## Incident Generation

When telemetry signals exceed configured thresholds, Guvnor automatically:

1. **Creates Incidents**: Generates new incidents with relevant context
2. **Enriches Data**: Adds metadata from telemetry attributes
3. **Correlates Signals**: Groups related signals into single incidents
4. **Applies Routing**: Routes incidents to appropriate teams based on service metadata

### Example Incident from Telemetry

```json
{
  "id": "inc-20240607-001",
  "title": "High Error Rate Detected",
  "description": "Service user-api experiencing 12.5% error rate (threshold: 5.0%)",
  "severity": "high",
  "status": "open",
  "source": "telemetry",
  "metadata": {
    "service_name": "user-api",
    "environment": "production",
    "error_rate": "12.5%",
    "threshold": "5.0%",
    "signal_type": "metric"
  },
  "created_at": "2024-06-07T04:16:17Z"
}
```

## API Integration

### Direct OTLP Ingestion

Applications can send telemetry data directly to Guvnor:

```bash
# gRPC endpoint
curl -X POST http://localhost:4317/v1/traces \\
  -H "Content-Type: application/x-protobuf" \\
  --data-binary @traces.pb

# HTTP endpoint
curl -X POST http://localhost:4318/v1/traces \\
  -H "Content-Type: application/json" \\
  -d @traces.json
```

### Health Check

Check receiver health:

```bash
curl http://localhost:8080/health
```

## Monitoring and Observability

### Receiver Metrics

Guvnor exposes metrics about the receiver itself:

- `guvnor_telemetry_signals_received_total`: Total signals received
- `guvnor_telemetry_signals_processed_total`: Total signals processed
- `guvnor_telemetry_incidents_created_total`: Total incidents created from telemetry
- `guvnor_telemetry_processing_duration_seconds`: Signal processing latency

### Logs

Structured logs are emitted for:

- Signal reception and processing
- Incident creation from telemetry
- Configuration changes
- Error conditions

Example log entry:

```json
{
  "timestamp": "2024-06-07T04:16:17Z",
  "level": "info",
  "msg": "Signal processed successfully",
  "signal_id": "sig-123",
  "signal_type": "metric",
  "source_service": "user-api",
  "incidents_created": 1,
  "processing_duration_ms": 45
}
```

## Troubleshooting

### Common Issues

1. **Receiver not starting:**
   - Check that ports 4317 and 4318 are available
   - Verify receiver is enabled in configuration
   - Check logs for specific error messages

2. **No incidents being created:**
   - Verify signal processor thresholds are appropriate
   - Check that telemetry data contains required attributes
   - Enable debug logging to see signal processing details

3. **High memory usage:**
   - Reduce signal processing rate limits
   - Increase batch processing intervals
   - Monitor telemetry data volume

### Debug Mode

Enable debug logging for detailed signal processing information:

```yaml
logging:
  level: debug
  format: json
```

### Testing

Test the receiver with sample data:

```bash
# Send test traces
make test-telemetry-traces

# Send test metrics
make test-telemetry-metrics

# Send test logs
make test-telemetry-logs
```

## Performance Considerations

### Resource Requirements

- **CPU**: 100-200m per 1000 signals/second
- **Memory**: 256-512MB base + 1MB per 1000 signals in buffer
- **Network**: Bandwidth depends on telemetry volume and retention

### Scaling

- Use horizontal scaling for high-volume deployments
- Configure appropriate rate limits to prevent overload
- Monitor processing latency and adjust thresholds accordingly

### Best Practices

1. **Sampling**: Use appropriate sampling rates to reduce data volume
2. **Batching**: Configure collectors to batch data before sending
3. **Filtering**: Filter irrelevant telemetry at the collector level
4. **Thresholds**: Set realistic thresholds based on service baselines
5. **Rate Limiting**: Configure rate limits to match your incident response capacity

## Security

### Network Security

- Use TLS for production deployments
- Implement network-level access controls
- Consider using authentication headers

### Data Privacy

- Telemetry data may contain sensitive information
- Configure collectors to scrub sensitive attributes
- Implement appropriate data retention policies

### Example Production Setup

```yaml
telemetry:
  receiver:
    enabled: true
    grpc_port: 4317
    http_port: 4318
    grpc_host: "0.0.0.0"  # Bind to all interfaces
    http_host: "0.0.0.0"
    tls:
      enabled: true
      cert_file: "/etc/certs/guvnor.crt"
      key_file: "/etc/certs/guvnor.key"
    auth:
      enabled: true
      header: "x-api-key"
      keys:
        - "prod-collector-key-1"
        - "prod-collector-key-2"
```

## Migration Guide

### From Prometheus AlertManager

1. Configure OpenTelemetry Collector to scrape Prometheus metrics
2. Set up equivalent thresholds in Guvnor signal processor
3. Update alerting rules to use Guvnor's incident management

### From Custom Monitoring

1. Instrument applications with OpenTelemetry SDKs
2. Configure collectors to send data to Guvnor
3. Define signal processing rules for your metrics

For more information, see the [OpenTelemetry documentation](https://opentelemetry.io/docs/).
