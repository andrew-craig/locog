package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"locog/internal/db"
	"locog/internal/models"
)

// newTestDB creates an in-memory SQLite database for testing.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	database, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

// newTestServer creates a server with an in-memory database for testing.
func newTestServer(t *testing.T) *server {
	t.Helper()
	return &server{
		db:      newTestDB(t),
		limiter: newIPRateLimiter(rate.Limit(100), 100),
	}
}

// sampleLogJSON returns a sample log entry as JSON bytes.
func sampleLogJSON() []byte {
	log := map[string]interface{}{
		"service": "test-service",
		"level":   "info",
		"message": "test message",
		"host":    "test-host",
	}
	data, _ := json.Marshal(log)
	return data
}

// TestHandleIngest_SingleLog tests ingesting a single log entry.
func TestHandleIngest_SingleLog(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(sampleLogJSON()))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rr.Code, rr.Body.String())
	}

	// Verify log was stored
	logs, _ := srv.db.QueryLogs(req.Context(), models.LogFilter{})
	if len(logs) != 1 {
		t.Errorf("expected 1 log in database, got %d", len(logs))
	}
}

// TestHandleIngest_BatchLogs tests ingesting multiple log entries.
func TestHandleIngest_BatchLogs(t *testing.T) {
	srv := newTestServer(t)

	logs := []map[string]interface{}{
		{"service": "svc1", "level": "info", "message": "msg1", "host": "h1"},
		{"service": "svc2", "level": "warn", "message": "msg2", "host": "h2"},
		{"service": "svc3", "level": "error", "message": "msg3", "host": "h3"},
	}
	body, _ := json.Marshal(logs)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rr.Code, rr.Body.String())
	}

	// Verify all logs were stored
	result, _ := srv.db.QueryLogs(req.Context(), models.LogFilter{})
	if len(result) != 3 {
		t.Errorf("expected 3 logs in database, got %d", len(result))
	}
}

// TestHandleIngest_InvalidJSON tests handling of malformed JSON.
func TestHandleIngest_InvalidJSON(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", strings.NewReader("{invalid json}"))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestHandleIngest_MissingService tests validation of missing service field.
func TestHandleIngest_MissingService(t *testing.T) {
	srv := newTestServer(t)

	log := map[string]interface{}{
		"level":   "info",
		"message": "test message",
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "service") {
		t.Errorf("expected error message to mention 'service', got: %s", rr.Body.String())
	}
}

// TestHandleIngest_MissingLevel tests validation of missing level field.
func TestHandleIngest_MissingLevel(t *testing.T) {
	srv := newTestServer(t)

	log := map[string]interface{}{
		"service": "test-service",
		"message": "test message",
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "level") {
		t.Errorf("expected error message to mention 'level', got: %s", rr.Body.String())
	}
}

// TestHandleIngest_MissingMessage tests validation of missing message field.
func TestHandleIngest_MissingMessage(t *testing.T) {
	srv := newTestServer(t)

	log := map[string]interface{}{
		"service": "test-service",
		"level":   "info",
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "message") {
		t.Errorf("expected error message to mention 'message', got: %s", rr.Body.String())
	}
}

// TestHandleIngest_WhitespaceOnly tests validation of whitespace-only fields.
func TestHandleIngest_WhitespaceOnly(t *testing.T) {
	srv := newTestServer(t)

	log := map[string]interface{}{
		"service": "   ",
		"level":   "info",
		"message": "test",
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for whitespace-only service, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestHandleIngest_SetTimestamp tests auto-setting timestamp when not provided.
func TestHandleIngest_SetTimestamp(t *testing.T) {
	srv := newTestServer(t)

	// Log without timestamp
	log := map[string]interface{}{
		"service": "test-service",
		"level":   "info",
		"message": "test message",
	}
	body, _ := json.Marshal(log)

	before := time.Now()
	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)
	after := time.Now()

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	// Verify timestamp was set
	logs, _ := srv.db.QueryLogs(req.Context(), models.LogFilter{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	// Timestamp should be between before and after
	if logs[0].Timestamp.Before(before) || logs[0].Timestamp.After(after) {
		t.Errorf("expected timestamp between %v and %v, got %v", before, after, logs[0].Timestamp)
	}
}

// TestHandleIngest_WithTimestamp tests preserving provided timestamp.
func TestHandleIngest_WithTimestamp(t *testing.T) {
	srv := newTestServer(t)

	providedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	log := map[string]interface{}{
		"timestamp": providedTime.Format(time.RFC3339),
		"service":   "test-service",
		"level":     "info",
		"message":   "test message",
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}

	logs, _ := srv.db.QueryLogs(req.Context(), models.LogFilter{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}

	if !logs[0].Timestamp.Equal(providedTime) {
		t.Errorf("expected timestamp %v, got %v", providedTime, logs[0].Timestamp)
	}
}

// TestHandleIngest_MethodNotAllowed tests rejection of non-POST methods.
func TestHandleIngest_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	methods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/ingest", nil)
			rr := httptest.NewRecorder()
			srv.handleIngest(rr, req)

			if rr.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected status %d for %s, got %d", http.StatusMethodNotAllowed, method, rr.Code)
			}
		})
	}
}

// TestHandleIngest_RateLimit tests rate limiting behavior.
func TestHandleIngest_RateLimit(t *testing.T) {
	// Create server with very restrictive rate limit
	srv := &server{
		db:      newTestDB(t),
		limiter: newIPRateLimiter(rate.Limit(1), 1), // 1 request/sec, burst of 1
	}

	// First request should succeed
	req1 := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(sampleLogJSON()))
	req1.Header.Set("Content-Type", "application/json")
	req1.RemoteAddr = "192.168.1.1:12345"

	rr1 := httptest.NewRecorder()
	srv.handleIngest(rr1, req1)

	if rr1.Code != http.StatusCreated {
		t.Errorf("first request: expected status %d, got %d", http.StatusCreated, rr1.Code)
	}

	// Second request should be rate limited
	req2 := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(sampleLogJSON()))
	req2.Header.Set("Content-Type", "application/json")
	req2.RemoteAddr = "192.168.1.1:12345"

	rr2 := httptest.NewRecorder()
	srv.handleIngest(rr2, req2)

	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("second request: expected status %d (rate limited), got %d", http.StatusTooManyRequests, rr2.Code)
	}
}

// TestHandleIngest_WithMetadata tests ingesting logs with metadata.
func TestHandleIngest_WithMetadata(t *testing.T) {
	srv := newTestServer(t)

	log := map[string]interface{}{
		"service": "test-service",
		"level":   "info",
		"message": "test message",
		"metadata": map[string]interface{}{
			"request_id": "abc123",
			"user_id":    42,
			"tags":       []string{"important", "auth"},
		},
	}
	body, _ := json.Marshal(log)

	req := httptest.NewRequest(http.MethodPost, "/api/ingest", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "192.168.1.1:12345"

	rr := httptest.NewRecorder()
	srv.handleIngest(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d: %s", http.StatusCreated, rr.Code, rr.Body.String())
	}

	logs, _ := srv.db.QueryLogs(req.Context(), models.LogFilter{})
	if len(logs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(logs))
	}
	if logs[0].Metadata["request_id"] != "abc123" {
		t.Errorf("expected metadata request_id 'abc123', got '%v'", logs[0].Metadata["request_id"])
	}
}

// TestHandleQueryLogs tests basic log querying.
func TestHandleQueryLogs(t *testing.T) {
	srv := newTestServer(t)

	// Insert test data
	srv.db.InsertLog(t.Context(), &models.Log{
		Timestamp: time.Now(),
		Service:   "api",
		Level:     "info",
		Message:   "test message",
		Host:      "host-1",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	rr := httptest.NewRecorder()
	srv.handleQueryLogs(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	// Verify response is JSON array
	var logs []models.Log
	if err := json.NewDecoder(rr.Body).Decode(&logs); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 log, got %d", len(logs))
	}
}

// TestHandleQueryLogs_WithFilters tests log querying with query parameters.
func TestHandleQueryLogs_WithFilters(t *testing.T) {
	srv := newTestServer(t)

	// Insert test data
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg1", Host: "h1"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "error", Message: "msg2", Host: "h1"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "worker", Level: "info", Message: "msg3", Host: "h2"})

	tests := []struct {
		name     string
		query    string
		expected int
	}{
		{"no filter", "", 3},
		{"service filter", "service=api", 2},
		{"level filter", "level=error", 1},
		{"host filter", "host=h2", 1},
		{"combined filters", "service=api&level=info", 1},
		{"limit filter", "limit=2", 2},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			url := "/api/logs"
			if tc.query != "" {
				url += "?" + tc.query
			}

			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()
			srv.handleQueryLogs(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
			}

			var logs []models.Log
			json.NewDecoder(rr.Body).Decode(&logs)
			if len(logs) != tc.expected {
				t.Errorf("expected %d logs, got %d", tc.expected, len(logs))
			}
		})
	}
}

// TestHandleQueryLogs_SearchFilter tests full-text search in messages.
func TestHandleQueryLogs_SearchFilter(t *testing.T) {
	srv := newTestServer(t)

	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "user logged in", Host: "h"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "error", Message: "database error", Host: "h"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "user logged out", Host: "h"})

	req := httptest.NewRequest(http.MethodGet, "/api/logs?search=user", nil)
	rr := httptest.NewRecorder()
	srv.handleQueryLogs(rr, req)

	var logs []models.Log
	json.NewDecoder(rr.Body).Decode(&logs)
	if len(logs) != 2 {
		t.Errorf("expected 2 logs matching 'user', got %d", len(logs))
	}
}

// TestHandleQueryLogs_TimeFilters tests time range filtering.
func TestHandleQueryLogs_TimeFilters(t *testing.T) {
	srv := newTestServer(t)

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: past.Add(-1 * time.Hour), Service: "api", Level: "info", Message: "old", Host: "h"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: now, Service: "api", Level: "info", Message: "current", Host: "h"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: future.Add(1 * time.Hour), Service: "api", Level: "info", Message: "future", Host: "h"})

	// Query with time range
	url := "/api/logs?start=" + past.Format(time.RFC3339) + "&end=" + future.Format(time.RFC3339)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	srv.handleQueryLogs(rr, req)

	var logs []models.Log
	json.NewDecoder(rr.Body).Decode(&logs)
	if len(logs) != 1 {
		t.Errorf("expected 1 log in time range, got %d", len(logs))
	}
}

// TestHandleQueryLogs_MethodNotAllowed tests rejection of non-GET methods.
func TestHandleQueryLogs_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/logs", nil)
	rr := httptest.NewRecorder()
	srv.handleQueryLogs(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

// TestHandleGetFilters tests retrieving filter options.
func TestHandleGetFilters(t *testing.T) {
	srv := newTestServer(t)

	// Insert test data
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "api", Level: "info", Message: "msg", Host: "host-1"})
	srv.db.InsertLog(t.Context(), &models.Log{Timestamp: time.Now(), Service: "worker", Level: "error", Message: "msg", Host: "host-2"})

	req := httptest.NewRequest(http.MethodGet, "/api/filters", nil)
	rr := httptest.NewRecorder()
	srv.handleGetFilters(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var options models.FilterOptions
	if err := json.NewDecoder(rr.Body).Decode(&options); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(options.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(options.Services))
	}
	if len(options.Levels) != 2 {
		t.Errorf("expected 2 levels, got %d", len(options.Levels))
	}
	if len(options.Hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(options.Hosts))
	}
}

// TestHandleGetFilters_MethodNotAllowed tests rejection of non-GET methods.
func TestHandleGetFilters_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/filters", nil)
	rr := httptest.NewRecorder()
	srv.handleGetFilters(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

// TestHealthEndpoint tests the health check endpoint.
func TestHealthEndpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got '%s'", rr.Body.String())
	}
}

// TestCORSMiddleware tests that CORS headers are set correctly.
func TestCORSMiddleware(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Check CORS headers
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got '%s'", rr.Header().Get("Access-Control-Allow-Origin"))
	}
	if rr.Header().Get("Access-Control-Allow-Methods") != "GET, POST, OPTIONS" {
		t.Errorf("expected Access-Control-Allow-Methods 'GET, POST, OPTIONS', got '%s'", rr.Header().Get("Access-Control-Allow-Methods"))
	}
	if rr.Header().Get("Access-Control-Allow-Headers") != "Content-Type" {
		t.Errorf("expected Access-Control-Allow-Headers 'Content-Type', got '%s'", rr.Header().Get("Access-Control-Allow-Headers"))
	}
}

// TestCORSMiddleware_Preflight tests OPTIONS preflight handling.
func TestCORSMiddleware_Preflight(t *testing.T) {
	handler := corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/ingest", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected status %d for OPTIONS, got %d", http.StatusNoContent, rr.Code)
	}
}

// TestValidateLog tests all validation cases.
func TestValidateLog(t *testing.T) {
	tests := []struct {
		name    string
		log     models.Log
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid log",
			log:     models.Log{Service: "api", Level: "info", Message: "test"},
			wantErr: false,
		},
		{
			name:    "missing service",
			log:     models.Log{Level: "info", Message: "test"},
			wantErr: true,
			errMsg:  "service",
		},
		{
			name:    "missing level",
			log:     models.Log{Service: "api", Message: "test"},
			wantErr: true,
			errMsg:  "level",
		},
		{
			name:    "missing message",
			log:     models.Log{Service: "api", Level: "info"},
			wantErr: true,
			errMsg:  "message",
		},
		{
			name:    "whitespace service",
			log:     models.Log{Service: "   ", Level: "info", Message: "test"},
			wantErr: true,
			errMsg:  "service",
		},
		{
			name:    "whitespace level",
			log:     models.Log{Service: "api", Level: "   ", Message: "test"},
			wantErr: true,
			errMsg:  "level",
		},
		{
			name:    "whitespace message",
			log:     models.Log{Service: "api", Level: "info", Message: "   "},
			wantErr: true,
			errMsg:  "message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateLog(&tc.log)
			if tc.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if !strings.Contains(err.Error(), tc.errMsg) {
					t.Errorf("expected error to contain '%s', got '%s'", tc.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got '%v'", err)
				}
			}
		})
	}
}

// TestGetClientIP_XForwardedFor tests IP extraction from X-Forwarded-For header.
func TestGetClientIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195")
	req.RemoteAddr = "192.168.1.1:12345"

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected IP '203.0.113.195', got '%s'", ip)
	}
}

// TestGetClientIP_MultipleIPs tests extraction of first IP from comma-separated list.
func TestGetClientIP_MultipleIPs(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18, 150.172.238.178")
	req.RemoteAddr = "192.168.1.1:12345"

	ip := getClientIP(req)
	if ip != "203.0.113.195" {
		t.Errorf("expected first IP '203.0.113.195', got '%s'", ip)
	}
}

// TestGetClientIP_RemoteAddr tests fallback to RemoteAddr.
func TestGetClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("expected IP '192.168.1.1', got '%s'", ip)
	}
}

// TestGetClientIP_RemoteAddrNoPort tests RemoteAddr without port.
func TestGetClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.168.1.1"

	ip := getClientIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("expected IP '192.168.1.1', got '%s'", ip)
	}
}

// TestIPRateLimiter tests per-IP rate limiter behavior.
func TestIPRateLimiter(t *testing.T) {
	limiter := newIPRateLimiter(rate.Limit(10), 10)

	// Get limiter for an IP
	l1 := limiter.getLimiter("192.168.1.1")
	if l1 == nil {
		t.Fatal("expected non-nil limiter")
	}

	// Same IP should return same limiter
	l2 := limiter.getLimiter("192.168.1.1")
	if l1 != l2 {
		t.Error("expected same limiter for same IP")
	}

	// Different IP should return different limiter
	l3 := limiter.getLimiter("192.168.1.2")
	if l1 == l3 {
		t.Error("expected different limiter for different IP")
	}
}

// TestIPRateLimiter_Allow tests rate limiter Allow behavior.
func TestIPRateLimiter_Allow(t *testing.T) {
	// Very restrictive: 1 request per second, burst of 1
	limiter := newIPRateLimiter(rate.Limit(1), 1)

	l := limiter.getLimiter("192.168.1.1")

	// First request should be allowed
	if !l.Allow() {
		t.Error("first request should be allowed")
	}

	// Second immediate request should be denied
	if l.Allow() {
		t.Error("second immediate request should be denied")
	}
}
