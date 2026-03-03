package db

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"locog/internal/models"
)

//go:embed schema.sql
var schema string

// filterCache caches filter options with a TTL
type filterCache struct {
	mu      sync.RWMutex
	options models.FilterOptions
	expires time.Time
}

const filterCacheTTL = 30 * time.Second

type DB struct {
	conn        *sql.DB
	filterCache filterCache
}

func New(dbPath string) (*DB, error) {
	// Configure pragmas via DSN so they apply to ALL connections created by
	// the pool, not just the first one. Without this, new pool connections
	// default to busy_timeout=0 and fail immediately on lock contention.
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-64000"

	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database %q: %w", dbPath, err)
	}

	// Initialize schema
	if err := initSchema(conn); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &DB{conn: conn}, nil
}

func initSchema(conn *sql.DB) error {
	_, err := conn.Exec(schema)
	return err
}

func (db *DB) InsertLog(ctx context.Context, log *models.Log) error {
	var metadataJSON []byte
	if log.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(log.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata for service=%s: %w", log.Service, err)
		}
	}

	_, err := db.conn.ExecContext(ctx, `
		INSERT INTO logs (timestamp, service, level, message, metadata, host)
		VALUES (?, ?, ?, ?, ?, ?)`,
		log.Timestamp, log.Service, log.Level, log.Message, metadataJSON, log.Host,
	)
	if err != nil {
		return fmt.Errorf("insert failed for service=%s level=%s host=%s: %w", log.Service, log.Level, log.Host, err)
	}
	return nil
}

func (db *DB) InsertBatch(ctx context.Context, logs []models.Log) error {
	tx, err := db.conn.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction for batch of %d logs: %w", len(logs), err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO logs (timestamp, service, level, message, metadata, host)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("failed to prepare batch insert statement: %w", err)
	}
	defer stmt.Close()

	for i, logEntry := range logs {
		var metadataJSON []byte
		if logEntry.Metadata != nil {
			var marshalErr error
			metadataJSON, marshalErr = json.Marshal(logEntry.Metadata)
			if marshalErr != nil {
				slog.Warn("skipping metadata due to marshal failure",
					"service", logEntry.Service,
					"host", logEntry.Host,
					"index", i,
					"batch_size", len(logs),
					"error", marshalErr,
				)
				// Continue with nil metadata rather than failing the entire batch
				metadataJSON = nil
			}
		}

		_, err = stmt.ExecContext(ctx, logEntry.Timestamp, logEntry.Service, logEntry.Level,
			logEntry.Message, metadataJSON, logEntry.Host)
		if err != nil {
			return fmt.Errorf("insert failed at index %d/%d (service=%s level=%s host=%s): %w",
				i, len(logs), logEntry.Service, logEntry.Level, logEntry.Host, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit batch of %d logs: %w", len(logs), err)
	}
	return nil
}

func (db *DB) QueryLogs(ctx context.Context, filter models.LogFilter) ([]models.Log, error) {
	query := `SELECT id, timestamp, service, level, message, metadata, host, created_at
              FROM logs WHERE 1=1`
	args := []interface{}{}

	if filter.Service != "" {
		query += " AND service = ?"
		args = append(args, filter.Service)
	}
	if filter.Level != "" {
		query += " AND level = ?"
		args = append(args, filter.Level)
	}
	if filter.Host != "" {
		query += " AND host = ?"
		args = append(args, filter.Host)
	}
	if filter.StartTime != nil {
		query += " AND timestamp >= ?"
		args = append(args, filter.StartTime)
	}
	if filter.EndTime != nil {
		query += " AND timestamp <= ?"
		args = append(args, filter.EndTime)
	}
	if filter.Search != "" {
		query += " AND message LIKE ?"
		args = append(args, "%"+filter.Search+"%")
	}

	query += " ORDER BY timestamp DESC"

	limit := filter.Limit
	if limit <= 0 {
		limit = 1000 // Default limit
	}
	query += " LIMIT ?"
	args = append(args, limit)

	rows, err := db.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed (service=%q level=%q host=%q search=%q limit=%d): %w",
			filter.Service, filter.Level, filter.Host, filter.Search, limit, err)
	}
	defer rows.Close()

	var logs []models.Log
	for rows.Next() {
		var logEntry models.Log
		var metadataJSON []byte

		err := rows.Scan(&logEntry.ID, &logEntry.Timestamp, &logEntry.Service, &logEntry.Level,
			&logEntry.Message, &metadataJSON, &logEntry.Host, &logEntry.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log row: %w", err)
		}

		if len(metadataJSON) > 0 {
			if unmarshalErr := json.Unmarshal(metadataJSON, &logEntry.Metadata); unmarshalErr != nil {
				slog.Warn("failed to unmarshal log metadata",
					"log_id", logEntry.ID,
					"service", logEntry.Service,
					"error", unmarshalErr,
				)
			}
		}

		logs = append(logs, logEntry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log rows: %w", err)
	}

	return logs, nil
}

func (db *DB) GetFilterOptions(ctx context.Context) (models.FilterOptions, error) {
	// Check cache first
	db.filterCache.mu.RLock()
	if time.Now().Before(db.filterCache.expires) {
		options := db.filterCache.options
		db.filterCache.mu.RUnlock()
		slog.Debug("filter options served from cache")
		return options, nil
	}
	db.filterCache.mu.RUnlock()

	// Cache miss or expired - fetch from database
	totalStart := time.Now()
	var options models.FilterOptions

	// Get distinct services
	queryStart := time.Now()
	services, err := db.getDistinctValues(ctx, "service")
	if err != nil {
		slog.Error("filter query failed", "column", "service", "duration_ms", time.Since(queryStart).Milliseconds(), "error", err)
		return options, err
	}
	slog.Info("filter query completed", "column", "service", "count", len(services), "duration_ms", time.Since(queryStart).Milliseconds())
	options.Services = services

	// Get distinct levels
	queryStart = time.Now()
	levels, err := db.getDistinctValues(ctx, "level")
	if err != nil {
		slog.Error("filter query failed", "column", "level", "duration_ms", time.Since(queryStart).Milliseconds(), "error", err)
		return options, err
	}
	slog.Info("filter query completed", "column", "level", "count", len(levels), "duration_ms", time.Since(queryStart).Milliseconds())
	options.Levels = levels

	// Get distinct hosts
	queryStart = time.Now()
	hosts, err := db.getDistinctValues(ctx, "host")
	if err != nil {
		slog.Error("filter query failed", "column", "host", "duration_ms", time.Since(queryStart).Milliseconds(), "error", err)
		return options, err
	}
	slog.Info("filter query completed", "column", "host", "count", len(hosts), "duration_ms", time.Since(queryStart).Milliseconds())
	options.Hosts = hosts

	slog.Info("filter options fetched from database", "total_duration_ms", time.Since(totalStart).Milliseconds(),
		"services", len(services), "levels", len(levels), "hosts", len(hosts))

	// Update cache
	db.filterCache.mu.Lock()
	db.filterCache.options = options
	db.filterCache.expires = time.Now().Add(filterCacheTTL)
	db.filterCache.mu.Unlock()

	return options, nil
}

// allowedFilterColumns defines the only column names that can be used in getDistinctValues
// to prevent SQL injection if the function is ever called with user input.
var allowedFilterColumns = map[string]bool{
	"service": true,
	"level":   true,
	"host":    true,
}

func (db *DB) getDistinctValues(ctx context.Context, column string) ([]string, error) {
	// Validate column name against allowlist to prevent SQL injection
	if !allowedFilterColumns[column] {
		return nil, fmt.Errorf("invalid column name: %s", column)
	}

	// Limit to 100 values to keep dropdowns usable
	query := fmt.Sprintf("SELECT DISTINCT %s FROM logs WHERE %s IS NOT NULL ORDER BY %s LIMIT 100",
		column, column, column)
	rows, err := db.conn.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query distinct %s values: %w", column, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			return nil, fmt.Errorf("failed to scan distinct %s value: %w", column, err)
		}
		values = append(values, val)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating distinct %s rows: %w", column, err)
	}

	return values, nil
}

func (db *DB) DeleteOldLogs(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := db.conn.ExecContext(ctx, "DELETE FROM logs WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete logs older than %s: %w", cutoff.Format(time.RFC3339), err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get deleted row count: %w", err)
	}
	return affected, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}
