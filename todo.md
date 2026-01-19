# Locog - Remaining Improvements

Issues identified during code review, prioritized for future implementation.

## Medium Priority

### 9. Hardcoded Schema Path (`sqlite.go:47`)
**Problem:** `os.ReadFile("schema.sql")` assumes the working directory contains the schema file.
**Solution:** Use `//go:embed` to embed the schema into the binary.

### 10. No Graceful Shutdown (`main.go:51`)
**Problem:** `log.Fatal(http.ListenAndServe(...))` doesn't handle SIGTERM/SIGINT gracefully.
**Solution:** Use `http.Server` with context-based shutdown:
```go
srv := &http.Server{Addr: *addr}
go func() {
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    <-sigChan
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    srv.Shutdown(ctx)
}()
srv.ListenAndServe()
```

### 11. Global Database Variable (`main.go:17`)
**Problem:** `var database *db.DB` is global state, making testing difficult.
**Solution:** Use dependency injection - pass database to handlers via closure or struct.

### 12. Missing CORS Headers
**Problem:** No CORS configuration; UI won't work if served from different origin.
**Solution:** Add CORS middleware or headers for API endpoints.

### 13. No Rate Limiting
**Problem:** Ingestion endpoint has no rate limiting, susceptible to abuse.
**Solution:** Add rate limiting middleware (e.g., `golang.org/x/time/rate`).

### 15. Filter Options Query Could Be Slow
**Problem:** `GetFilterOptions()` runs 3 separate `SELECT DISTINCT` queries that become expensive as table grows.
**Solution:** Cache results with TTL, or combine into single query.

### 16. No Limit on Filter Options (`sqlite.go:198-215`)
**Problem:** Thousands of unique services/hosts would make dropdowns unusable.
**Solution:** Add `LIMIT 100` to distinct queries.

## Low Priority / Code Quality

### 17. Inconsistent Error Handling Pattern
**Problem:** Some handlers log errors before returning, others don't.
**Solution:** Standardize: always log server errors, never log client errors.

### 18. No Context Propagation
**Problem:** Database operations don't accept `context.Context`.
**Solution:** Add context parameter to all DB methods for cancellation and timeouts:
```go
func (db *DB) QueryLogs(ctx context.Context, filter LogFilter) ([]Log, error)
```

### 19. Missing HTTP Method Check on Read Endpoints
**Problem:** `handleQueryLogs` and `handleGetFilters` accept any HTTP method.
**Solution:** Add `if r.Method != http.MethodGet` check.

### 20. Defer After Error Check (`main.go:65-66`)
**Problem:** `defer r.Body.Close()` is placed after reading the body.
**Solution:** Move defer before `io.ReadAll()` (minor, as MaxBytesReader wraps it).

### 21. No Tests
**Problem:** No unit or integration tests exist.
**Solution:** Add tests for:
- Log validation
- Batch insertion
- Query filtering
- API endpoints (httptest)

### 22. `rows.Err()` Not Checked (`sqlite.go:165-168`)
**Problem:** After iterating `rows.Next()`, `rows.Err()` should be checked.
**Solution:** Add `if err := rows.Err(); err != nil { return nil, err }` before return.

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
