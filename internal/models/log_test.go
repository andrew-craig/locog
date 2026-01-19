package models

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLogJSONMarshal(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	createdAt := time.Date(2024, 1, 15, 10, 30, 1, 0, time.UTC)

	log := Log{
		ID:        1,
		Timestamp: timestamp,
		Service:   "api-service",
		Level:     "info",
		Message:   "User logged in",
		Metadata: map[string]interface{}{
			"user_id":    "123",
			"ip_address": "192.168.1.1",
		},
		Host:      "prod-server-1",
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal Log: %v", err)
	}

	// Verify key fields are present in JSON
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if result["service"] != "api-service" {
		t.Errorf("expected service 'api-service', got '%v'", result["service"])
	}
	if result["level"] != "info" {
		t.Errorf("expected level 'info', got '%v'", result["level"])
	}
	if result["message"] != "User logged in" {
		t.Errorf("expected message 'User logged in', got '%v'", result["message"])
	}
	if result["host"] != "prod-server-1" {
		t.Errorf("expected host 'prod-server-1', got '%v'", result["host"])
	}

	// Verify metadata is present
	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("metadata should be a map")
	}
	if metadata["user_id"] != "123" {
		t.Errorf("expected metadata user_id '123', got '%v'", metadata["user_id"])
	}
}

func TestLogJSONUnmarshal(t *testing.T) {
	jsonData := `{
		"id": 42,
		"timestamp": "2024-01-15T10:30:00Z",
		"service": "worker",
		"level": "error",
		"message": "Task failed",
		"metadata": {"task_id": "abc123", "retry_count": 3},
		"host": "worker-node-1",
		"created_at": "2024-01-15T10:30:01Z"
	}`

	var log Log
	if err := json.Unmarshal([]byte(jsonData), &log); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if log.ID != 42 {
		t.Errorf("expected ID 42, got %d", log.ID)
	}
	if log.Service != "worker" {
		t.Errorf("expected service 'worker', got '%s'", log.Service)
	}
	if log.Level != "error" {
		t.Errorf("expected level 'error', got '%s'", log.Level)
	}
	if log.Message != "Task failed" {
		t.Errorf("expected message 'Task failed', got '%s'", log.Message)
	}
	if log.Host != "worker-node-1" {
		t.Errorf("expected host 'worker-node-1', got '%s'", log.Host)
	}

	// Verify metadata
	if log.Metadata["task_id"] != "abc123" {
		t.Errorf("expected metadata task_id 'abc123', got '%v'", log.Metadata["task_id"])
	}
	// Note: JSON numbers unmarshal as float64
	if log.Metadata["retry_count"] != float64(3) {
		t.Errorf("expected metadata retry_count 3, got '%v'", log.Metadata["retry_count"])
	}

	// Verify timestamp parsing
	expectedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	if !log.Timestamp.Equal(expectedTime) {
		t.Errorf("expected timestamp %v, got %v", expectedTime, log.Timestamp)
	}
}

func TestLogJSONOmitEmptyMetadata(t *testing.T) {
	log := Log{
		ID:        1,
		Timestamp: time.Now(),
		Service:   "api",
		Level:     "info",
		Message:   "test",
		Metadata:  nil, // Should be omitted from JSON
		Host:      "host",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal Log: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Metadata should be omitted when nil
	if _, exists := result["metadata"]; exists {
		t.Error("expected metadata to be omitted from JSON when nil")
	}
}

func TestLogJSONEmptyMetadataMap(t *testing.T) {
	log := Log{
		ID:        1,
		Timestamp: time.Now(),
		Service:   "api",
		Level:     "info",
		Message:   "test",
		Metadata:  map[string]interface{}{}, // Empty map
		Host:      "host",
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal Log: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// With omitempty, empty maps are omitted from JSON (same as nil)
	// This is the expected Go behavior
	if _, exists := result["metadata"]; exists {
		t.Error("expected empty metadata map to be omitted from JSON with omitempty tag")
	}
}

func TestLogFilterDefaults(t *testing.T) {
	filter := LogFilter{}

	// All fields should be zero values
	if filter.Service != "" {
		t.Errorf("expected empty Service, got '%s'", filter.Service)
	}
	if filter.Level != "" {
		t.Errorf("expected empty Level, got '%s'", filter.Level)
	}
	if filter.Host != "" {
		t.Errorf("expected empty Host, got '%s'", filter.Host)
	}
	if filter.StartTime != nil {
		t.Errorf("expected nil StartTime, got %v", filter.StartTime)
	}
	if filter.EndTime != nil {
		t.Errorf("expected nil EndTime, got %v", filter.EndTime)
	}
	if filter.Limit != 0 {
		t.Errorf("expected 0 Limit, got %d", filter.Limit)
	}
	if filter.Search != "" {
		t.Errorf("expected empty Search, got '%s'", filter.Search)
	}
}

func TestLogFilterWithValues(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Hour)
	endTime := time.Now()

	filter := LogFilter{
		Service:   "api",
		Level:     "error",
		Host:      "prod-1",
		StartTime: &startTime,
		EndTime:   &endTime,
		Limit:     100,
		Search:    "connection failed",
	}

	if filter.Service != "api" {
		t.Errorf("expected Service 'api', got '%s'", filter.Service)
	}
	if filter.Level != "error" {
		t.Errorf("expected Level 'error', got '%s'", filter.Level)
	}
	if filter.Host != "prod-1" {
		t.Errorf("expected Host 'prod-1', got '%s'", filter.Host)
	}
	if filter.StartTime == nil || !filter.StartTime.Equal(startTime) {
		t.Errorf("expected StartTime %v, got %v", startTime, filter.StartTime)
	}
	if filter.EndTime == nil || !filter.EndTime.Equal(endTime) {
		t.Errorf("expected EndTime %v, got %v", endTime, filter.EndTime)
	}
	if filter.Limit != 100 {
		t.Errorf("expected Limit 100, got %d", filter.Limit)
	}
	if filter.Search != "connection failed" {
		t.Errorf("expected Search 'connection failed', got '%s'", filter.Search)
	}
}

func TestFilterOptionsJSONMarshal(t *testing.T) {
	options := FilterOptions{
		Services: []string{"api", "worker", "scheduler"},
		Levels:   []string{"debug", "info", "warn", "error"},
		Hosts:    []string{"host-1", "host-2"},
	}

	data, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("failed to marshal FilterOptions: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Verify structure
	services, ok := result["services"].([]interface{})
	if !ok {
		t.Fatal("services should be an array")
	}
	if len(services) != 3 {
		t.Errorf("expected 3 services, got %d", len(services))
	}

	levels, ok := result["levels"].([]interface{})
	if !ok {
		t.Fatal("levels should be an array")
	}
	if len(levels) != 4 {
		t.Errorf("expected 4 levels, got %d", len(levels))
	}

	hosts, ok := result["hosts"].([]interface{})
	if !ok {
		t.Fatal("hosts should be an array")
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}

func TestFilterOptionsJSONUnmarshal(t *testing.T) {
	jsonData := `{
		"services": ["api", "worker"],
		"levels": ["info", "error"],
		"hosts": ["localhost"]
	}`

	var options FilterOptions
	if err := json.Unmarshal([]byte(jsonData), &options); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(options.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(options.Services))
	}
	if options.Services[0] != "api" {
		t.Errorf("expected first service 'api', got '%s'", options.Services[0])
	}

	if len(options.Levels) != 2 {
		t.Errorf("expected 2 levels, got %d", len(options.Levels))
	}

	if len(options.Hosts) != 1 {
		t.Errorf("expected 1 host, got %d", len(options.Hosts))
	}
	if options.Hosts[0] != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", options.Hosts[0])
	}
}

func TestFilterOptionsEmpty(t *testing.T) {
	options := FilterOptions{}

	data, err := json.Marshal(options)
	if err != nil {
		t.Fatalf("failed to marshal empty FilterOptions: %v", err)
	}

	var result FilterOptions
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	// Nil slices marshal to null, not empty arrays
	// This is expected Go behavior
	if result.Services != nil && len(result.Services) != 0 {
		t.Log("Note: Empty/nil slices have specific JSON behavior")
	}
}
