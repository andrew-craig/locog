package db

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"locog/internal/models"
)

//go:embed schema.sql
var schema string

type DB struct {
	conn *sql.DB
}

func New(dbPath string) (*DB, error) {
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	// Set pragmas for better performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",      // Write-Ahead Logging for better concurrency
		"PRAGMA synchronous=NORMAL",    // Faster writes, still safe
		"PRAGMA cache_size=-64000",     // 64MB cache
		"PRAGMA busy_timeout=5000",     // Wait 5s on lock
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	// Initialize schema
	if err := initSchema(conn); err != nil {
		return nil, err
	}

	return &DB{conn: conn}, nil
}

func initSchema(conn *sql.DB) error {
	_, err := conn.Exec(schema)
	return err
}

func (db *DB) InsertLog(log *models.Log) error {
	var metadataJSON []byte
	if log.Metadata != nil {
		var err error
		metadataJSON, err = json.Marshal(log.Metadata)
		if err != nil {
			return err
		}
	}

	_, err := db.conn.Exec(`
		INSERT INTO logs (timestamp, service, level, message, metadata, host)
		VALUES (?, ?, ?, ?, ?, ?)`,
		log.Timestamp, log.Service, log.Level, log.Message, metadataJSON, log.Host,
	)
	return err
}

func (db *DB) InsertBatch(logs []models.Log) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO logs (timestamp, service, level, message, metadata, host)
		VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, logEntry := range logs {
		var metadataJSON []byte
		if logEntry.Metadata != nil {
			var marshalErr error
			metadataJSON, marshalErr = json.Marshal(logEntry.Metadata)
			if marshalErr != nil {
				log.Printf("Failed to marshal metadata for log (service=%s): %v", logEntry.Service, marshalErr)
				// Continue with nil metadata rather than failing the entire batch
				metadataJSON = nil
			}
		}

		_, err = stmt.Exec(logEntry.Timestamp, logEntry.Service, logEntry.Level,
			logEntry.Message, metadataJSON, logEntry.Host)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (db *DB) QueryLogs(filter models.LogFilter) ([]models.Log, error) {
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

	rows, err := db.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.Log
	for rows.Next() {
		var log models.Log
		var metadataJSON []byte

		err := rows.Scan(&log.ID, &log.Timestamp, &log.Service, &log.Level,
			&log.Message, &metadataJSON, &log.Host, &log.CreatedAt)
		if err != nil {
			return nil, err
		}

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &log.Metadata)
		}

		logs = append(logs, log)
	}

	return logs, nil
}

func (db *DB) GetFilterOptions() (models.FilterOptions, error) {
	var options models.FilterOptions

	// Get distinct services
	services, err := db.getDistinctValues("service")
	if err != nil {
		return options, err
	}
	options.Services = services

	// Get distinct levels
	levels, err := db.getDistinctValues("level")
	if err != nil {
		return options, err
	}
	options.Levels = levels

	// Get distinct hosts
	hosts, err := db.getDistinctValues("host")
	if err != nil {
		return options, err
	}
	options.Hosts = hosts

	return options, nil
}

// allowedFilterColumns defines the only column names that can be used in getDistinctValues
// to prevent SQL injection if the function is ever called with user input.
var allowedFilterColumns = map[string]bool{
	"service": true,
	"level":   true,
	"host":    true,
}

func (db *DB) getDistinctValues(column string) ([]string, error) {
	// Validate column name against allowlist to prevent SQL injection
	if !allowedFilterColumns[column] {
		return nil, fmt.Errorf("invalid column name: %s", column)
	}

	query := fmt.Sprintf("SELECT DISTINCT %s FROM logs WHERE %s IS NOT NULL ORDER BY %s",
		column, column, column)
	rows, err := db.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var val string
		if err := rows.Scan(&val); err != nil {
			return nil, err
		}
		values = append(values, val)
	}
	return values, nil
}

func (db *DB) DeleteOldLogs(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan)
	result, err := db.conn.Exec("DELETE FROM logs WHERE timestamp < ?", cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (db *DB) Close() error {
	return db.conn.Close()
}
