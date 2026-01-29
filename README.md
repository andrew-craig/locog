# Locog - Lightweight Log Viewer

Locog is a minimal, self-contained log aggregation and viewing solution.

NOTE: AI built this app. Do not use in production scenarios or with sensitive data.

## Features

- **Lightweight**: Single Go binary
- **Simple deployment**: SQLite database, no external dependencies
- **Real-time viewing**: Web UI with auto-refresh
- **Flexible filtering**: By service, level, host, and full-text search
- **Production-ready**: Follows 12-factor app principles
- **Decoupled logging**: Receives logs from log shippers like Vector

## Architecture

```
┌─────────────────┐
│  App (Python)   │──┐
│  logs to stdout │  │    ┌───────────┐      ┌──────────────────┐      ┌─────────┐
└─────────────────┘  │    │ Vector    │─────>│  Log Service     │─────>│ SQLite  │
                     ├──> │ (shipper) │      │  (Go binary)     │      │  DB     │
┌─────────────────┐  │    └───────────┘      │  :5081           │      └─────────┘
│  App (Go)       │──┘                       └──────────────────┘
│  logs to stdout │                                   │ 
└─────────────────┘                                   ▼ 
                                               ┌──────────────┐
                                               │  Web UI      │
                                               │  (browser)   │
                                               └──────────────┘
```

## Quick Start

### Using Docker Compose with Pre-built Images (Recommended)

1. Create a `docker-compose.yml` file:

```yaml
services:
  locog:
    image: ghcr.io/andrew-craig/locog:main
    ports:
      - "5081:5081"
    volumes:
      - ./data:/data
    command: ["-db", "/data/logs.db", "-addr", ":5081"]
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:5081/health"]
      interval: 30s
      timeout: 5s
      retries: 3

  vector:
    image: timberio/vector:0.35.0-alpine
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./vector.toml:/etc/vector/vector.toml:ro
      - vector-data:/var/lib/vector
    depends_on:
      locog:
        condition: service_healthy
    restart: unless-stopped

volumes:
  vector-data:
```

2. Create a `vector.toml` file:

```toml
# Vector configuration for log shipping
data_dir = "/var/lib/vector"

# Source: Collect logs from Docker containers
[sources.docker_logs]
type = "docker_logs"
# Optionally filter by container name/label
# include_containers = ["my-app-*"]

# Transform: Parse JSON logs and format for Locog API
[transforms.parse_and_enrich]
type = "remap"
inputs = ["docker_logs"]
source = '''
  # Parse JSON from log message
  parsed = {}
  if is_string(.message) {
    parsed, err = parse_json(.message)
    if err != null {
      # If not JSON, treat the whole message as a plain log
      parsed = {
        "message": .message,
        "level": "INFO"
      }
    }
  }

  # Construct output with only Locog API fields
  . = {
    "timestamp": parsed.timestamp ?? .timestamp ?? now(),
    "service": parsed.service ?? .container_name ?? "unknown",
    "level": parsed.level ?? "INFO",
    "message": parsed.message ?? parsed.msg ?? "no message",
    "host": parsed.host ?? get_hostname!(),
  }

  # Add metadata if present (optional field)
  if exists(parsed.metadata) {
    .metadata = parsed.metadata
  }
'''

# Sink: Send to log collector service
[sinks.log_collector]
type = "http"
inputs = ["parse_and_enrich"]
uri = "http://locog:5081/api/ingest"
encoding.codec = "json"

# Batch settings for better performance
batch.max_bytes = 1048576  # 1MB
batch.max_events = 100
batch.timeout_secs = 5

# Retry settings
buffer.type = "disk"
buffer.max_size = 268435488  # 256MB buffer
buffer.when_full = "drop_newest"

# Health check configuration
healthcheck.enabled = true
```

3. Start the services:
```bash
docker-compose up -d
```

4. Access the web UI at `http://localhost:5081`

### Building from Source (Docker self-build)

If you prefer to build the container locally:

```bash
cd self-build
docker-compose up -d
```

### Building the Binary

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

**Important:** When integrating applications with Vector and Locog:
1. Applications output JSON logs to stdout (see examples below)
2. Vector collects these logs and parses the JSON
3. Vector sends the parsed logs to Locog's HTTP API
4. Locog stores and displays them in the web UI

The Vector configuration must include the JSON parsing transform shown in the [Vector Configuration](#vector-configuration) section above.

### Vector Configuration

Vector ships logs from your applications to the Locog service. The configuration depends on how your applications run.

#### Docker Container Logs

This configuration works with the Python/Go examples above that log JSON to stdout:

```toml
# Vector configuration for Docker-based applications
data_dir = "/var/lib/vector"

# Source: Collect logs from Docker containers
[sources.docker_logs]
type = "docker_logs"
# Optionally filter by container name/label
# include_containers = ["my-app-*"]

# Transform: Parse JSON logs and format for Locog API
[transforms.parse_and_enrich]
type = "remap"
inputs = ["docker_logs"]
source = '''
  # Parse JSON from log message
  parsed = {}
  if is_string(.message) {
    parsed, err = parse_json(.message)
    if err != null {
      # If not JSON, treat the whole message as a plain log
      parsed = {
        "message": .message,
        "level": "INFO"
      }
    }
  }

  # Construct output with only Locog API fields
  # This ensures no Docker metadata pollutes the logs
  . = {
    "timestamp": parsed.timestamp ?? .timestamp ?? now(),
    "service": parsed.service ?? .container_name ?? "unknown",
    "level": parsed.level ?? "INFO",
    "message": parsed.message ?? parsed.msg ?? "no message",
    "host": parsed.host ?? get_hostname!(),
  }

  # Add metadata if present (optional field)
  if exists(parsed.metadata) {
    .metadata = parsed.metadata
  }
'''

# Sink: Send to Locog service
[sinks.log_collector]
type = "http"
inputs = ["parse_and_enrich"]
uri = "http://locog:5081/api/ingest"  # Change to your Locog server address
encoding.codec = "json"

# Batch settings for better performance
batch.max_bytes = 1048576  # 1MB
batch.max_events = 100
batch.timeout_secs = 5

# Retry and buffer settings
buffer.type = "disk"
buffer.max_size = 268435488  # 256MB
buffer.when_full = "drop_newest"

# Health check
healthcheck.enabled = true
```

**Key Points:**
- The `parse_and_enrich` transform parses JSON from your application logs
- It explicitly constructs a clean output object with only the fields Locog needs (timestamp, service, level, message, host, metadata)
- This prevents Docker metadata fields (container_id, stream, etc.) from polluting your logs
- If your app doesn't specify a `service` field, it uses the Docker container name
- Handles both JSON logs and plain text logs (non-JSON gets wrapped with default level)
- The sink sends to Locog using HTTP with batching for performance

#### File-Based Logs

For applications that write logs to files instead of Docker stdout:

```toml
data_dir = "/var/lib/vector"

# Source: Collect from log files
[sources.file_logs]
type = "file"
include = ["/var/log/apps/*.log"]  # Adjust path to your log files
read_from = "end"  # Start from end, or use "beginning" for existing logs

# Transform: Same as Docker example above
[transforms.parse_and_enrich]
type = "remap"
inputs = ["file_logs"]
source = '''
  # Parse JSON from log message
  parsed = {}
  if is_string(.message) {
    parsed, err = parse_json(.message)
    if err != null {
      parsed = {
        "message": .message,
        "level": "INFO"
      }
    }
  }

  # Construct output with only Locog API fields
  . = {
    "timestamp": parsed.timestamp ?? .timestamp ?? now(),
    "service": parsed.service ?? "unknown",
    "level": parsed.level ?? "INFO",
    "message": parsed.message ?? parsed.msg ?? "no message",
    "host": parsed.host ?? get_hostname!(),
  }

  # Add metadata if present
  if exists(parsed.metadata) {
    .metadata = parsed.metadata
  }
'''

# Sink: Same as above
[sinks.log_collector]
type = "http"
inputs = ["parse_and_enrich"]
uri = "http://localhost:5081/api/ingest"
encoding.codec = "json"
batch.max_bytes = 1048576
batch.max_events = 100
batch.timeout_secs = 5
buffer.type = "disk"
buffer.max_size = 268435488
buffer.when_full = "drop_newest"
healthcheck.enabled = true
```

#### Remote Server System Logs

To collect system logs from a remote server and send them to a central Locog instance, use this Vector configuration:

```toml
# Vector configuration for remote system log shipping
data_dir = "/var/lib/vector"

# Source: Collect from systemd journal
[sources.journald]
type = "journald"

# Transform: Map journald logs to Locog API format
[transforms.format_for_locog]
type = "remap"
inputs = ["journald"]
source = '''
  # Map journald priority to log level
  priority = to_int!(.PRIORITY)
  level = if priority <= 3 {
    "ERROR"
  } else if priority <= 4 {
    "WARN"
  } else if priority <= 6 {
    "INFO"
  } else {
    "DEBUG"
  }

  # Set service name from systemd identifier or unit
  service = if exists(.SYSLOG_IDENTIFIER) {
    to_string!(.SYSLOG_IDENTIFIER)
  } else if exists(._SYSTEMD_UNIT) {
    to_string!(._SYSTEMD_UNIT)
  } else {
    "system"
  }

  # Set message (handle missing or non-string MESSAGE field)
  message = if exists(.message) {
    to_string!(.message)
  } else if exists(.MESSAGE) {
    to_string!(.MESSAGE)
  } else {
    "no message"
  }

  # Construct output with only Locog API fields
  . = {
    "timestamp": .timestamp ?? now(),
    "service": service,
    "level": level,
    "message": message,
    "host": to_string(.host) ?? get_hostname!(),
    "metadata": {
      "pid": ._PID,
      "unit": ._SYSTEMD_UNIT
    }
  }
'''

# Sink: Send to remote Locog instance
[sinks.remote_locog]
type = "http"
inputs = ["format_for_locog"]
uri = "http://your-locog-server:5081/api/ingest"
encoding.codec = "json"

# Batch settings for better performance
batch.max_bytes = 1048576  # 1MB
batch.max_events = 100
batch.timeout_secs = 5

# Retry and buffer settings
buffer.type = "disk"
buffer.max_size = 536870912  # 512MB
buffer.when_full = "drop_newest"

# Health check
healthcheck.enabled = true
```

**Installation on remote server:**

```bash
# Install Vector (example for Ubuntu/Debian)
curl -1sLf 'https://repositories.timber.io/public/vector/cfg/setup/bash.deb.sh' | sudo -E bash
sudo apt-get install vector

# Copy configuration
sudo cp vector.toml /etc/vector/vector.toml

# Update the Locog server address in the config
sudo sed -i 's/your-locog-server:5081/actual-server:5081/g' /etc/vector/vector.toml

# Enable and start Vector
sudo systemctl enable vector
sudo systemctl start vector

# Check status
sudo systemctl status vector
sudo journalctl -u vector -f
```

### Python Application

```python
import logging
import json
import sys
from datetime import datetime

class JSONFormatter(logging.Formatter):
    # Standard logging record attributes to exclude from metadata
    RESERVED_ATTRS = {
        'name', 'msg', 'args', 'created', 'filename', 'funcName', 'levelname',
        'levelno', 'lineno', 'module', 'msecs', 'message', 'pathname', 'process',
        'processName', 'relativeCreated', 'thread', 'threadName', 'exc_info',
        'exc_text', 'stack_info', 'getMessage', 'getMessage'
    }

    def format(self, record):
        log_data = {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "level": record.levelname,
            "service": "my-python-app",  # Change this to your service name
            "message": record.getMessage(),
            "logger": record.name,
        }

        # Extract extra fields passed via logger.info(..., extra={...})
        extra = {k: v for k, v in record.__dict__.items()
                 if k not in self.RESERVED_ATTRS}
        if extra:
            log_data["metadata"] = extra

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

**Expected output to stdout:**
```json
{"timestamp": "2026-01-27T10:30:00.123Z", "level": "INFO", "service": "my-python-app", "message": "User logged in", "logger": "__main__", "metadata": {"user_id": 123, "ip": "192.168.1.1"}}
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

**Expected output to stdout:**
```json
{"level":"info","timestamp":"2026-01-27T10:30:00.123Z","service":"my-go-app","msg":"user logged in","user_id":123,"ip":"192.168.1.1"}
```

### Testing Your Integration

To verify your application, Vector, and Locog are working together:

1. **Test application logging:**
   ```bash
   # Run your application and verify JSON is written to stdout
   python your_app.py  # Should see JSON lines printed
   ```

2. **Verify Vector is collecting logs:**
   ```bash
   # Check Vector logs for any errors
   docker logs vector  # Or: sudo journalctl -u vector -f

   # Should see lines like:
   # INFO vector::sources::docker_logs: Collecting from container. container_name="my-app"
   # INFO vector::sinks::http: Sending batch of 10 events.
   ```

3. **Check Locog received the logs:**
   - Open the web UI at `http://localhost:5081`
   - Look for your service name in the service filter dropdown
   - Verify the log structure looks correct (not double-JSON-encoded)
   - Check that metadata fields are properly displayed in the expandable log details

4. **If logs aren't appearing or look wrong:**
   - Verify your application outputs valid JSON (test with `jq`): `python your_app.py | jq`
   - Check Vector is parsing the JSON correctly - look for Vector errors
   - Test manual ingestion to isolate the issue:
     ```bash
     # Copy a log line from your app's output and send directly to Locog
     curl -X POST http://localhost:5081/api/ingest \
       -H "Content-Type: application/json" \
       -d '{"timestamp":"2026-01-27T10:30:00Z","service":"test","level":"INFO","message":"test"}'
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
├── self-build/
│   ├── docker-compose.yml    # Self-build Docker deployment
│   └── vector.toml           # Vector configuration
├── schema.sql                # Database schema
├── Dockerfile                # Docker build
├── go.mod                    # Go dependencies
└── README.md                 # This file
```
