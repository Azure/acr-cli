# Structured Logging in ACR CLI

This document describes the structured logging features added to the Azure Container Registry CLI.

## Overview

The ACR CLI now uses [zerolog](https://github.com/rs/zerolog) for structured logging, providing better observability, performance, and flexibility for log management and analysis.

## New Command Line Flags

### `--log-level`
Controls the verbosity of log output:
- `debug`: Detailed operational information (default for development)
- `info`: General operational information (default)
- `warn`: Warning conditions and recoverable errors
- `error`: Error conditions only

### `--log-format`
Controls the output format:
- `console`: Human-readable format with colors (default for CLI usage)
- `json`: Machine-readable JSON format (for log aggregation tools)

## Usage Examples

```bash
# Basic usage with default settings (info level, console format)
acr purge -r myregistry --filter "myrepo:.*" --ago 7d

# Debug level for detailed troubleshooting
acr --log-level debug purge -r myregistry --filter "myrepo:.*" --ago 7d

# JSON format for log aggregation
acr --log-level info --log-format json purge -r myregistry --filter "myrepo:.*" --ago 7d

# Error level only for production scripts
acr --log-level error purge -r myregistry --filter "myrepo:.*" --ago 7d
```

## Enhanced Manifest Purge Logging

The manifest purge operations now include detailed structured logging:

### Debug Level Logs
- **Manifest Evaluation**: Why each manifest is included or excluded from purge
- **Tag Analysis**: Tag count analysis and retention logic
- **Dependency Checking**: Multiarch manifest and referrer evaluation
- **Criteria Details**: Detailed decision points for each manifest

### Info Level Logs
- **Operation Summaries**: Start/completion of purge operations
- **Deletion Counts**: Number of manifests processed and deleted
- **Repository Context**: Clear repository identification in all logs

### Warn Level Logs
- **404 Errors**: Manifests not found during deletion (assumed already deleted)
- **Skipped Operations**: Operations skipped due to various conditions
- **Repository Issues**: Repository access or availability problems

### Error Level Logs
- **Operation Failures**: Failed deletion attempts with full context
- **Authentication Issues**: Registry access problems
- **API Errors**: Underlying registry API failures

## Example Log Entries

### Console Format (Human-Readable)
```
INF Starting manifest purge operation repository=myrepo dry_run=false
DBG Manifest excluded from purge - has remaining tags repository=myrepo manifest=sha256:abc123 reason=has_tags tag_count=3
WRN Manifest not found during deletion, assuming already deleted repository=myrepo manifest=sha256:def456 status_code=404
INF Successfully completed manifest purge operation repository=myrepo deleted_count=5 attempted_count=7
```

### JSON Format (Machine-Readable)
```json
{"level":"info","time":"2024-01-15T10:30:00Z","repository":"myrepo","dry_run":false,"message":"Starting manifest purge operation"}
{"level":"debug","time":"2024-01-15T10:30:01Z","repository":"myrepo","manifest":"sha256:abc123","reason":"has_tags","tag_count":3,"message":"Manifest excluded from purge - has remaining tags"}
{"level":"warn","time":"2024-01-15T10:30:02Z","repository":"myrepo","manifest":"sha256:def456","status_code":404,"message":"Manifest not found during deletion, assuming already deleted"}
{"level":"info","time":"2024-01-15T10:30:03Z","repository":"myrepo","deleted_count":5,"attempted_count":7,"message":"Successfully completed manifest purge operation"}
```

## Structured Fields

All log entries include relevant structured fields:

- `repository`: Repository name being processed
- `manifest`: Manifest digest when applicable
- `tag`: Tag name when applicable
- `reason`: Reason for exclusion or inclusion decisions
- `status_code`: HTTP status codes for API responses
- `*_count`: Various counts (deleted, attempted, candidates, etc.)
- `dry_run`: Whether operation is in dry-run mode
- `error`: Full error details when applicable

## Backward Compatibility

- **User Output**: All existing user-facing output (console messages) preserved
- **Default Behavior**: No change in default behavior (info level, console format)
- **Existing Commands**: All commands work exactly as before
- **Test Compatibility**: All existing tests continue to pass

## Benefits

1. **Improved Debugging**: Detailed decision logic for manifest purge operations
2. **Better Monitoring**: Structured logs suitable for log aggregation systems
3. **Performance**: Non-blocking structured logging with minimal overhead
4. **Flexibility**: Configurable verbosity and format for different use cases
5. **Observability**: Clear operational context in all log entries

## Integration with Log Aggregation

The JSON format is compatible with popular log aggregation tools:

- **ELK Stack**: Elasticsearch, Logstash, Kibana
- **Splunk**: Native JSON log parsing
- **Azure Monitor**: Log Analytics workspace ingestion
- **Prometheus/Grafana**: Via log exporters

Example Logstash configuration:
```ruby
input {
  file {
    path => "/var/log/acr-cli/*.log"
    codec => "json"
  }
}

filter {
  if [repository] {
    mutate {
      add_tag => ["acr-operation"]
    }
  }
}
```

## Performance Considerations

- **Zero Allocation**: Zerolog's zero-allocation approach minimizes GC pressure
- **Conditional Logging**: Debug logs only processed when debug level enabled
- **Concurrent Safe**: Thread-safe logging for concurrent manifest operations
- **Minimal Overhead**: Structured logging adds <1% overhead to operations