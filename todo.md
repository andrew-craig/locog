# Locog - Remaining Improvements

Issues identified during code review, prioritized for future implementation.

## 1. Critical Priority


## 2. High Priority

## 3. Medium Priority


## 4. Low Priority / Code Quality

### 4.1. Add Metrics/Observability
**Problem:** No visibility into service health or performance.
**Solution:** Add Prometheus metrics:
- `logs_ingested_total` (counter)
- `logs_query_duration_seconds` (histogram)
- `database_size_bytes` (gauge)

### 4.2. WebSocket for Real-time Updates
**Problem:** UI polls every 10 seconds, not truly real-time.
**Solution:** Add WebSocket endpoint for streaming new logs to connected clients.

