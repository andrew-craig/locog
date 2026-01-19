# Locog - Lightweight Log Viewer

A minimal, self-contained log aggregation and viewing solution using SQLite for storage, a single Go binary for collection and viewing, and Vector for log shipping.

## Features

- **Lightweight**: Single Go binary, ~50MB RAM usage
- **Simple deployment**: SQLite database, no external dependencies
- **Real-time viewing**: Web UI with auto-refresh
- **Flexible filtering**: By service, level, host, and full-text search
- **Production-ready**: Follows 12-factor app principles
- **Decoupled logging**: Applications log to stdout, Vector ships to collector

## Architecture

```
┌─────────────────┐
│  App (Python)   │──┐
│  logs to stdout │  │
└─────────────────┘  │
                     ├──> ┌─────────┐      ┌──────────────────┐      ┌─────────┐
┌─────────────────┐  │    │ Vector  │─────>│  Log Service     │─────>│ SQLite  │
│  App (Go)       │──┘    │ (shipper)│      │  (Go binary)     │      │  DB     │
│  logs to stdout │       └─────────┘      │  :5081           │      └─────────┘
└─────────────────┘                        └──────────────────┘
                                                   │
                                                   ▼
                                            ┌──────────────┐
                                            │  Web UI      │
                                            │  (browser)   │
                                            └──────────────┘
```

## Quick Start

### Using Docker Compose (Recommended)

1. Start the services:
```bash
cd deployments
docker-compose up -d
```

2. Access the web UI at `http://localhost:5081`

### Building from Source

1. Build the binary:
```bash
go build -o logservice ./cmd/logservice
```

2. Run the service:
```bash
./logservice -db logs.db -addr :5081
```

3. Access the web UI at `http://localhost:5081`

## Usage

### Accessing the Web UI

Open your browser to `http://localhost:5081`

The web UI provides:
- Filter logs by service, level, and host
- Full-text search in messages
- Adjustable result limits (100-5000)
- Auto-refresh every 10 seconds
- Color-coded log levels

### Manual Log Ingestion (for testing)

```bash
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
```

Batch ingestion:
```bash
curl -X POST http://localhost:5081/api/ingest \
  -H "Content-Type: application/json" \
  -d '[
    {"service": "api", "level": "INFO", "message": "Request received", "host": "web-1"},
    {"service": "api", "level": "ERROR", "message": "Database timeout", "host": "web-1"}
  ]'
```

### Querying via API

Get latest 100 ERROR logs from api-service:
```bash
curl "http://localhost:5081/api/logs?service=api-service&level=ERROR&limit=100"
```

Search for "database" in messages:
```bash
curl "http://localhost:5081/api/logs?search=database"
```

Get logs from specific time range:
```bash
curl "http://localhost:5081/api/logs?start=2025-01-19T00:00:00Z&end=2025-01-19T23:59:59Z"
```

## Application Integration

### Python Application

```python
import logging
import json
import sys
from datetime import datetime

class JSONFormatter(logging.Formatter):
    def format(self, record):
        log_data = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "level": record.levelname,
            "service": "my-python-app",
            "message": record.getMessage(),
            "logger": record.name,
        }

        if hasattr(record, 'extra'):
            log_data["metadata"] = record.extra

        return json.dumps(log_data)

def setup_logging():
    handler = logging.StreamHandler(sys.stdout)
    handler.setFormatter(JSONFormatter())

    logger = logging.getLogger()
    logger.addHandler(handler)
    logger.setLevel(logging.INFO)

    return logger

# Usage
logger = setup_logging()
logger.info("User logged in", extra={"user_id": 123, "ip": "192.168.1.1"})
```

### Go Application

```go
import (
    "go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

func setupLogger(serviceName string) (*zap.Logger, error) {
    config := zap.NewProductionConfig()
    config.EncoderConfig.TimeKey = "timestamp"
    config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
    config.OutputPaths = []string{"stdout"}

    logger, err := config.Build()
    if err != nil {
        return nil, err
    }

    logger = logger.With(zap.String("service", serviceName))
    return logger, nil
}

// Usage
logger, _ := setupLogger("my-go-app")
logger.Info("user logged in",
    zap.Int("user_id", 123),
    zap.String("ip", "192.168.1.1"))
```

## Configuration

### Log Service

Command-line flags:
- `-db`: Path to SQLite database (default: `logs.db`)
- `-addr`: HTTP service address (default: `:5081`)

Example:
```bash
./logservice -db /data/logs.db -addr :9000
```

### Vector

Edit `deployments/vector.toml` to configure log sources and shipping behavior.

Key settings:
- `sources.docker_logs`: Collect from Docker containers
- `sources.file_logs`: Collect from log files
- `sinks.log_collector.uri`: Log service endpoint
- `batch.max_events`: Batch size (default: 100)
- `buffer.max_size`: Buffer size (default: 256MB)

## Maintenance

### Database Cleanup

The service automatically deletes logs older than 30 days. To change the retention period, modify `cmd/logservice/main.go`:

```go
deleted, err := database.DeleteOldLogs(30 * 24 * time.Hour) // Change 30 to desired days
```

Manual cleanup:
```bash
sqlite3 logs.db "DELETE FROM logs WHERE timestamp < datetime('now', '-30 days');"
sqlite3 logs.db "VACUUM;"
```

### Backup

```bash
# Backup SQLite database
sqlite3 logs.db ".backup /backup/logs-$(date +%Y%m%d).db"

# Or simple file copy (stop service first for consistency)
cp logs.db logs-backup.db
```

### Monitoring

Check service health:
```bash
curl http://localhost:5081/health
```

Monitor Vector:
```bash
docker logs -f vector
```

Database statistics:
```bash
sqlite3 logs.db "SELECT COUNT(*) FROM logs;"
du -h logs.db
```

## Performance

- **Ingestion rate**: 10,000+ logs/second (batched)
- **Query performance**: Sub-second for most queries on millions of logs
- **Memory usage**: ~50MB for service, ~10MB for Vector
- **Disk usage**: ~100-200 bytes per log entry
- **Default retention**: 30 days

## Scaling

For higher scale:

1. **Multiple collectors**: Run Vector on each server pointing to central log service
2. **Database sharding**: Split logs by time period into separate databases
3. **Read replicas**: Copy SQLite files for multiple read-only query instances
4. **Partition by service**: Run separate instances per service with different databases

If you exceed ~100K logs/minute, consider moving to Loki, OpenSearch, or similar.

## Troubleshooting

### No logs appearing

- Check Vector is running: `docker ps` or `systemctl status vector`
- Verify Vector config: `vector validate /etc/vector/vector.toml`
- Check Vector logs for errors
- Test manual ingestion with curl

### Slow queries

- Check indexes exist: `sqlite3 logs.db ".schema"`
- Reduce query time range or use filters
- Consider increasing cache size in `internal/db/sqlite.go`

### Database locked errors

- Ensure WAL mode is enabled (should be automatic)
- Check for long-running queries
- Reduce concurrent writes

## Security Considerations

- **No authentication**: Add reverse proxy (nginx) with basic auth for production
- **Network exposure**: Bind to localhost only or use firewall rules
- **Rate limiting**: Add to reverse proxy or Vector config to prevent abuse

## License

MIT

## Project Structure

```
locog/
├── cmd/
│   └── logservice/
│       └── main.go           # Single binary for collector + viewer
├── internal/
│   ├── db/
│   │   └── sqlite.go         # Database operations
│   └── models/
│       └── log.go            # Log data structures
├── web/
│   └── static/
│       ├── index.html        # Web UI
│       └── app.js            # Frontend JavaScript
├── deployments/
│   ├── docker-compose.yml    # Docker deployment
│   └── vector.toml           # Vector configuration
├── schema.sql                # Database schema
├── Dockerfile                # Docker build
├── go.mod                    # Go dependencies
└── README.md                 # This file
```
