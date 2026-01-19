-- schema.sql
CREATE TABLE IF NOT EXISTS logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,
    service VARCHAR(100) NOT NULL,
    level VARCHAR(20) NOT NULL,
    message TEXT NOT NULL,
    metadata JSON,
    host VARCHAR(255),
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_timestamp ON logs(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_service ON logs(service);
CREATE INDEX IF NOT EXISTS idx_level ON logs(level);
CREATE INDEX IF NOT EXISTS idx_host ON logs(host);
CREATE INDEX IF NOT EXISTS idx_service_timestamp ON logs(service, timestamp DESC);

-- Optional: Auto-cleanup of old logs (30 days)
-- Run this periodically via cron or within the service
-- DELETE FROM logs WHERE timestamp < datetime('now', '-30 days');
