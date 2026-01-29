# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Locog is a lightweight log aggregation system written in Go. It provides log ingestion via HTTP API, SQLite storage, and a web UI for viewing/filtering logs. Designed as a simpler alternative to enterprise solutions like Loki or OpenSearch.

## Build Commands

```bash
# Build the service
go build -o logservice ./cmd/logservice

# Run locally
./logservice -db /tmp/logs.db -addr :5081

# Run tests
go test ./...

# Run tests with coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out

# Build and run with Docker (self-build)
cd self-build && docker-compose up -d
```

## Architecture

**Data Flow:** Applications (stdout) → Vector (shipper) → Log Service (HTTP API) → SQLite DB → Web UI

**Key Components:**
- `cmd/logservice/main.go` - Single binary entry point, HTTP server, API handlers
- `internal/db/sqlite.go` - Database layer with prepared statements, connection pooling, WAL mode
- `internal/models/log.go` - Data models (Log, LogFilter, FilterOptions)
- `web/static/` - Browser-based UI with real-time filtering (vanilla JS, dark theme)
- `self-build/` - Docker Compose and Vector configuration (for building locally)

**Test Files:**
- `cmd/logservice/main_test.go` - HTTP handler and utility function tests
- `internal/db/sqlite_test.go` - Database layer tests (uses in-memory SQLite)
- `internal/models/log_test.go` - Model JSON serialization tests

**API Endpoints:**
- `POST /api/ingest` - Accept single or batch log entries
- `GET /api/logs` - Query logs with filtering (service, level, host, search, time range)
- `GET /api/filters` - Get available filter values for dropdowns
- `GET /health` - Health check
- `GET /` - Serve web UI

## Data Flow Diagrams

### Log Ingestion Flow

```mermaid
sequenceDiagram
    participant App as Application
    participant Vector as Vector Shipper
    participant API as HTTP API<br/>(main.go)
    participant Rate as Rate Limiter<br/>(per-IP)
    participant Val as Validator
    participant DB as Database Layer<br/>(sqlite.go)
    participant SQLite as SQLite DB<br/>(WAL mode)

    App->>Vector: stdout logs
    Vector->>API: POST /api/ingest<br/>(JSON single or batch)
    API->>Rate: Check rate limit
    alt Rate limit exceeded
        Rate-->>API: Deny
        API-->>Vector: 429 Too Many Requests
    else Rate limit OK
        Rate-->>API: Allow
        API->>API: Parse JSON<br/>(single or batch)
        API->>Val: Validate each log
        Val->>Val: Check required fields<br/>(service, level, message)
        alt Validation fails
            Val-->>API: Error
            API-->>Vector: 400 Bad Request
        else Validation OK
            Val-->>API: Valid
            alt Batch (multiple logs)
                API->>DB: InsertBatch(logs)
                DB->>DB: Begin transaction
                DB->>SQLite: Prepared statement<br/>(batch insert)
                SQLite-->>DB: Success
                DB->>DB: Commit transaction
            else Single log
                API->>DB: InsertLog(log)
                DB->>SQLite: Parameterized query<br/>(single insert)
                SQLite-->>DB: Success
            end
            DB-->>API: Success
            API-->>Vector: 201 Created
        end
    end

    Note over SQLite: - WAL journal mode<br/>- 64MB cache<br/>- Indexed on timestamp,<br/>  service, level, host
```

### User Query Flow

```mermaid
sequenceDiagram
    participant User as User Browser
    participant UI as Web UI<br/>(app.js)
    participant API as HTTP API<br/>(main.go)
    participant Cache as Filter Cache<br/>(30s TTL)
    participant DB as Database Layer<br/>(sqlite.go)
    participant SQLite as SQLite DB

    User->>UI: Open log viewer
    UI->>API: GET /api/filters
    API->>Cache: Check filter cache
    alt Cache hit (fresh)
        Cache-->>API: Return cached options
    else Cache miss/expired
        API->>DB: GetFilterOptions()
        DB->>SQLite: SELECT DISTINCT service
        SQLite-->>DB: Services list
        DB->>SQLite: SELECT DISTINCT level
        SQLite-->>DB: Levels list
        DB->>SQLite: SELECT DISTINCT host
        SQLite-->>DB: Hosts list
        DB-->>API: FilterOptions
        API->>Cache: Update cache (30s)
    end
    API-->>UI: JSON {services, levels, hosts}
    UI->>UI: Populate dropdowns

    User->>UI: Select filters/search
    UI->>API: GET /api/logs?service=X&level=Y&search=Z&limit=N
    API->>DB: QueryLogs(filter)
    DB->>DB: Build WHERE clause
    DB->>SQLite: SELECT * FROM logs<br/>WHERE conditions<br/>ORDER BY timestamp DESC<br/>LIMIT N
    Note over SQLite: Uses indexes for<br/>efficient filtering
    SQLite-->>DB: Result rows
    DB->>DB: Unmarshal metadata JSON
    DB-->>API: []Log
    API-->>UI: JSON log array
    UI->>UI: Render log entries
    UI-->>User: Display logs

    opt Auto-refresh enabled
        loop Every 10 seconds
            UI->>API: GET /api/logs (same filters)
            API->>DB: QueryLogs(filter)
            DB->>SQLite: SELECT query
            SQLite-->>DB: Result rows
            DB-->>API: []Log
            API-->>UI: Updated logs
            UI->>UI: Re-render display
        end
    end

    opt Cleanup routine (daily)
        API->>DB: DeleteOldLogs(30 days)
        DB->>SQLite: DELETE FROM logs<br/>WHERE timestamp < cutoff
        SQLite-->>DB: Rows deleted
        DB-->>API: Deleted count
    end
```

## Code Conventions

- Database operations go in `internal/db` package
- Models/data structures go in `internal/models` package
- Use prepared statements for all database queries
- Use parameterized queries (SQL injection prevention)
- Frontend uses Fetch API, no frameworks

## Database Schema

The SQLite database uses the following schema (see `schema.sql`):

```sql
CREATE TABLE logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    service VARCHAR(100) NOT NULL,
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    metadata JSON,
    host VARCHAR(255),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

Indexes exist on: `timestamp DESC`, `service`, `level`, `host`, and composite `(service, timestamp DESC)`.

## SQLite Configuration

The database uses these pragmas for performance:
- `journal_mode=WAL` - Write-Ahead Logging for better concurrency
- `synchronous=NORMAL` - Faster writes, still safe
- `cache_size=-64000` - 64MB cache
- `busy_timeout=5000` - Wait 5s on lock

## Log Retention

The service automatically deletes logs older than 30 days via a daily cleanup routine.

## Manual Testing

```bash
# Ingest a test log
curl -X POST http://localhost:5081/api/ingest \
  -H "Content-Type: application/json" \
  -d '{
    "timestamp": "2025-01-19T10:30:00Z",
    "service": "test-app",
    "level": "INFO",
    "message": "Test log message",
    "host": "test-host",
    "metadata": {"user_id": 123}
  }'

# Query logs
curl "http://localhost:5081/api/logs?service=api-service&level=ERROR&limit=100"
curl "http://localhost:5081/api/logs?search=database"
```
