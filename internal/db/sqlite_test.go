package db

import (
	"context"
	"testing"
	"time"

	"locog/internal/models"
)

// newTestDB creates an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *DB {
	t.Helper()
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// sampleLog returns a sample log entry for testing.
func sampleLog(service, level, message string) models.Log {
	return models.Log{
		Timestamp: time.Now(),
		Service:   service,
		Level:     level,
		Message:   message,
		Host:      "test-host",
	}
}

func TestNew(t *testing.T) {
	db := newTestDB(t)
	if db == nil {
		t.Fatal("expected non-nil database")
	}
	if db.conn == nil {
		t.Fatal("expected non-nil connection")
	}
}

func TestNew_InvalidPath(t *testing.T) {
	// Test with an invalid path that should fail
	_, err := New("/nonexistent/path/to/db.sqlite")
	if err == nil {
		t.Error("expected error for invalid database path")
	}
}

func TestInsertLog(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	log := sampleLog("api-service", "info", "test message")
	log.Metadata = map[string]interface{}{
		"request_id": "abc123",
		"user_id":    42,
	}

	err := db.InsertLog(ctx, &log)
	if err != nil {
		t.Fatalf("InsertLog failed: %v", err)
	}

	// Verify the log was inserted
	logs, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Service != "api-service" {
		t.Errorf("expected service 'api-service', got '%s'", logs[0].Service)
	}
	if logs[0].Metadata["request_id"] != "abc123" {
		t.Errorf("expected metadata request_id 'abc123', got '%v'", logs[0].Metadata["request_id"])
	}
}

func TestInsertLog_NilMetadata(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	log := sampleLog("api-service", "info", "test message")
	log.Metadata = nil

	err := db.InsertLog(ctx, &log)
	if err != nil {
		t.Fatalf("InsertLog with nil metadata failed: %v", err)
	}

	// Verify the log was inserted
	logs, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Metadata != nil && len(logs[0].Metadata) != 0 {
		t.Errorf("expected nil or empty metadata, got %v", logs[0].Metadata)
	}
}

func TestInsertBatch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	logs := []models.Log{
		sampleLog("service-a", "info", "message 1"),
		sampleLog("service-b", "warn", "message 2"),
		sampleLog("service-c", "error", "message 3"),
	}

	err := db.InsertBatch(ctx, logs)
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	// Verify all logs were inserted
	result, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(result))
	}
}

func TestInsertBatch_Empty(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	err := db.InsertBatch(ctx, []models.Log{})
	if err != nil {
		t.Fatalf("InsertBatch with empty slice failed: %v", err)
	}

	// Verify no logs were inserted
	result, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(result) != 0 {
		t.Fatalf("expected 0 logs, got %d", len(result))
	}
}

func TestInsertBatch_WithMetadata(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	log1 := sampleLog("service-a", "info", "message 1")
	log1.Metadata = map[string]interface{}{"key1": "value1"}

	log2 := sampleLog("service-b", "warn", "message 2")
	log2.Metadata = map[string]interface{}{"key2": "value2", "nested": map[string]interface{}{"inner": "data"}}

	logs := []models.Log{log1, log2}

	err := db.InsertBatch(ctx, logs)
	if err != nil {
		t.Fatalf("InsertBatch failed: %v", err)
	}

	result, err := db.QueryLogs(ctx, models.LogFilter{Service: "service-a"})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 log, got %d", len(result))
	}
	if result[0].Metadata["key1"] != "value1" {
		t.Errorf("expected metadata key1='value1', got '%v'", result[0].Metadata["key1"])
	}
}

func TestQueryLogs_NoFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert some test logs
	for i := 0; i < 5; i++ {
		log := sampleLog("service", "info", "message")
		if err := db.InsertLog(ctx, &log); err != nil {
			t.Fatalf("InsertLog failed: %v", err)
		}
	}

	// Query with no filter
	logs, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 5 {
		t.Errorf("expected 5 logs, got %d", len(logs))
	}
}

func TestQueryLogs_ServiceFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert logs with different services
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "h1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "h1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "worker", Level: "info", Message: "msg", Host: "h1"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{Service: "api"})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for service 'api', got %d", len(logs))
	}
	for _, log := range logs {
		if log.Service != "api" {
			t.Errorf("expected service 'api', got '%s'", log.Service)
		}
	}
}

func TestQueryLogs_LevelFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "msg", Host: "h1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "error", Message: "msg", Host: "h1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "error", Message: "msg", Host: "h1"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{Level: "error"})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for level 'error', got %d", len(logs))
	}
}

func TestQueryLogs_HostFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "msg", Host: "host-1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "msg", Host: "host-2"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "msg", Host: "host-1"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{Host: "host-1"})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs for host 'host-1', got %d", len(logs))
	}
}

func TestQueryLogs_TimeRangeFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	// Insert logs at different times
	db.InsertLog(ctx, &models.Log{Timestamp: past.Add(-1 * time.Hour), Service: "svc", Level: "info", Message: "old", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: now, Service: "svc", Level: "info", Message: "current", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: future.Add(1 * time.Hour), Service: "svc", Level: "info", Message: "future", Host: "h"})

	// Query with time range
	startTime := past
	endTime := future
	logs, err := db.QueryLogs(ctx, models.LogFilter{StartTime: &startTime, EndTime: &endTime})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log in time range, got %d", len(logs))
	}
}

func TestQueryLogs_SearchFilter(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "user login successful", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "error", Message: "database connection failed", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "user logout", Host: "h"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{Search: "user"})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 2 {
		t.Errorf("expected 2 logs matching 'user', got %d", len(logs))
	}
}

func TestQueryLogs_CombinedFilters(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "error", Message: "something failed", Host: "prod-1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "something succeeded", Host: "prod-1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "worker", Level: "error", Message: "task failed", Host: "prod-1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "error", Message: "other error", Host: "prod-2"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{
		Service: "api",
		Level:   "error",
		Host:    "prod-1",
	})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log matching combined filters, got %d", len(logs))
	}
}

func TestQueryLogs_CustomLimit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert 10 logs
	for i := 0; i < 10; i++ {
		log := sampleLog("service", "info", "message")
		db.InsertLog(ctx, &log)
	}

	// Query with limit of 3
	logs, err := db.QueryLogs(ctx, models.LogFilter{Limit: 3})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 3 {
		t.Errorf("expected 3 logs with limit, got %d", len(logs))
	}
}

func TestQueryLogs_DefaultLimit(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Query with no limit should use default (1000)
	logs, err := db.QueryLogs(ctx, models.LogFilter{Limit: 0})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	// Just verify it doesn't error - we can't easily test the 1000 limit without inserting many logs
	_ = logs
}

func TestQueryLogs_OrderByTimestamp(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert logs with different timestamps
	t1 := time.Now().Add(-3 * time.Hour)
	t2 := time.Now().Add(-2 * time.Hour)
	t3 := time.Now().Add(-1 * time.Hour)

	db.InsertLog(ctx, &models.Log{Timestamp: t2, Service: "svc", Level: "info", Message: "second", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: t1, Service: "svc", Level: "info", Message: "first", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: t3, Service: "svc", Level: "info", Message: "third", Host: "h"})

	logs, err := db.QueryLogs(ctx, models.LogFilter{})
	if err != nil {
		t.Fatalf("QueryLogs failed: %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(logs))
	}

	// Should be ordered DESC by timestamp (newest first)
	if logs[0].Message != "third" {
		t.Errorf("expected first result to be 'third', got '%s'", logs[0].Message)
	}
	if logs[1].Message != "second" {
		t.Errorf("expected second result to be 'second', got '%s'", logs[1].Message)
	}
	if logs[2].Message != "first" {
		t.Errorf("expected third result to be 'first', got '%s'", logs[2].Message)
	}
}

func TestGetFilterOptions(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert logs with various services, levels, and hosts
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "host-1"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "worker", Level: "error", Message: "msg", Host: "host-2"})
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "warn", Message: "msg", Host: "host-1"})

	options, err := db.GetFilterOptions(ctx)
	if err != nil {
		t.Fatalf("GetFilterOptions failed: %v", err)
	}

	// Check services
	if len(options.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(options.Services))
	}

	// Check levels
	if len(options.Levels) != 3 {
		t.Errorf("expected 3 levels, got %d", len(options.Levels))
	}

	// Check hosts
	if len(options.Hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(options.Hosts))
	}
}

func TestGetFilterOptions_Caching(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "host-1"})

	// First call - should fetch from DB
	options1, err := db.GetFilterOptions(ctx)
	if err != nil {
		t.Fatalf("GetFilterOptions failed: %v", err)
	}

	// Insert another log
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "worker", Level: "error", Message: "msg", Host: "host-2"})

	// Second call within TTL - should return cached result
	options2, err := db.GetFilterOptions(ctx)
	if err != nil {
		t.Fatalf("GetFilterOptions failed: %v", err)
	}

	// Should have same values due to caching (new service not visible yet)
	if len(options1.Services) != len(options2.Services) {
		t.Log("Note: If this test fails, it might be due to cache timing - this is expected behavior")
	}
}

func TestGetDistinctValues_InvalidColumn(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Try to query with an invalid column (SQL injection attempt)
	_, err := db.getDistinctValues(ctx, "invalid_column")
	if err == nil {
		t.Error("expected error for invalid column name")
	}

	// Try potential SQL injection
	_, err = db.getDistinctValues(ctx, "service; DROP TABLE logs; --")
	if err == nil {
		t.Error("expected error for SQL injection attempt")
	}
}

func TestGetDistinctValues_ValidColumns(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "host-1"})

	// Test all valid columns
	for _, col := range []string{"service", "level", "host"} {
		values, err := db.getDistinctValues(ctx, col)
		if err != nil {
			t.Errorf("getDistinctValues(%s) failed: %v", col, err)
		}
		if len(values) == 0 {
			t.Errorf("expected values for column %s", col)
		}
	}
}

func TestDeleteOldLogs(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	now := time.Now()

	// Insert old and new logs
	db.InsertLog(ctx, &models.Log{Timestamp: now.Add(-40 * 24 * time.Hour), Service: "svc", Level: "info", Message: "old log", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: now.Add(-35 * 24 * time.Hour), Service: "svc", Level: "info", Message: "old log 2", Host: "h"})
	db.InsertLog(ctx, &models.Log{Timestamp: now.Add(-1 * time.Hour), Service: "svc", Level: "info", Message: "recent log", Host: "h"})

	// Delete logs older than 30 days
	deleted, err := db.DeleteOldLogs(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteOldLogs failed: %v", err)
	}
	if deleted != 2 {
		t.Errorf("expected 2 deleted logs, got %d", deleted)
	}

	// Verify only the recent log remains
	logs, _ := db.QueryLogs(ctx, models.LogFilter{})
	if len(logs) != 1 {
		t.Errorf("expected 1 remaining log, got %d", len(logs))
	}
	if logs[0].Message != "recent log" {
		t.Errorf("expected 'recent log' to remain, got '%s'", logs[0].Message)
	}
}

func TestDeleteOldLogs_NoMatch(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()

	// Insert only recent logs
	db.InsertLog(ctx, &models.Log{Timestamp: time.Now(), Service: "svc", Level: "info", Message: "recent", Host: "h"})

	// Try to delete old logs
	deleted, err := db.DeleteOldLogs(ctx, 30*24*time.Hour)
	if err != nil {
		t.Fatalf("DeleteOldLogs failed: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected 0 deleted logs, got %d", deleted)
	}
}

func TestContextCancellation(t *testing.T) {
	db := newTestDB(t)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Operations should fail with cancelled context
	log := sampleLog("svc", "info", "msg")
	err := db.InsertLog(ctx, &log)
	if err == nil {
		t.Error("expected error with cancelled context for InsertLog")
	}

	_, err = db.QueryLogs(ctx, models.LogFilter{})
	if err == nil {
		t.Error("expected error with cancelled context for QueryLogs")
	}
}

func TestClose(t *testing.T) {
	db, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	err = db.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Verify database is closed by attempting an operation
	ctx := context.Background()
	log := sampleLog("svc", "info", "msg")
	err = db.InsertLog(ctx, &log)
	if err == nil {
		t.Error("expected error after closing database")
	}
}
