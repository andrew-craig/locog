# Locog - Remaining Improvements

Issues identified during code review, prioritized for future implementation.

## Medium Priority


## Low Priority / Code Quality


### 18. No Context Propagation
**Problem:** Database operations don't accept `context.Context`.
**Solution:** Add context parameter to all DB methods for cancellation and timeouts:
```go
func (db *DB) QueryLogs(ctx context.Context, filter LogFilter) ([]Log, error)
```

### 21. No Tests
**Problem:** No unit or integration tests exist.
**Solution:** Add tests for:
- Log validation
- Batch insertion
- Query filtering
- API endpoints (httptest)

## Architecture Suggestions

### 23. Consider Embedding Static Files
**Problem:** Static files require specific directory structure at runtime.
**Solution:** Use `//go:embed web/static/*` to embed into binary for single-file deployment.

### 24. Add Structured Logging
**Problem:** `log.Printf` produces unstructured text logs.
**Solution:** Use `log/slog` (Go 1.21+) for structured JSON logging - fitting for a log viewer.

### 25. Add Metrics/Observability
**Problem:** No visibility into service health or performance.
**Solution:** Add Prometheus metrics:
- `logs_ingested_total` (counter)
- `logs_query_duration_seconds` (histogram)
- `database_size_bytes` (gauge)

### 26. WebSocket for Real-time Updates
**Problem:** UI polls every 10 seconds, not truly real-time.
**Solution:** Add WebSocket endpoint for streaming new logs to connected clients.

## Deployment Issues

### 27. Docker Compose `depends_on` Doesn't Wait for Health
**Problem:** `depends_on: [logservice]` only waits for container start, not healthcheck.
**Solution:** Update `docker-compose.yml`:
```yaml
depends_on:
  logservice:
    condition: service_healthy
```

### 28. Volume Permissions
**Problem:** `./data` volume may have permission issues depending on host OS.
**Solution:** Document required permissions or add init container to set ownership.
