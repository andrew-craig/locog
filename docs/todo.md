# Locog - Remaining Improvements

Issues identified during code review, prioritized for future implementation.

## Medium Priority


## Low Priority / Code Quality



## Architecture Suggestions

### 25. Add Metrics/Observability
**Problem:** No visibility into service health or performance.
**Solution:** Add Prometheus metrics:
- `logs_ingested_total` (counter)
- `logs_query_duration_seconds` (histogram)
- `database_size_bytes` (gauge)

### 26. WebSocket for Real-time Updates
**Problem:** UI polls every 10 seconds, not truly real-time.
**Solution:** Add WebSocket endpoint for streaming new logs to connected clients.

