package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"locog/internal/models"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

// newTestServerWithHub creates a test server with a running WebSocket hub.
func newTestServerWithHub(t *testing.T) *server {
	t.Helper()
	hub := newWSHub()
	go hub.run()
	return &server{
		db:      newTestDB(t),
		limiter: newIPRateLimiter(rate.Limit(100), 100),
		hub:     hub,
	}
}

// TestWebSocketConnect tests that a client can connect via WebSocket.
func TestWebSocketConnect(t *testing.T) {
	srv := newTestServerWithHub(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("expected status %d, got %d", http.StatusSwitchingProtocols, resp.StatusCode)
	}

	// Wait briefly for registration
	time.Sleep(50 * time.Millisecond)

	if srv.hub.clientCount() != 1 {
		t.Errorf("expected 1 connected client, got %d", srv.hub.clientCount())
	}
}

// TestWebSocketReceivesLogs tests that connected clients receive broadcast logs.
func TestWebSocketReceivesLogs(t *testing.T) {
	srv := newTestServerWithHub(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Wait for registration
	time.Sleep(50 * time.Millisecond)

	// Broadcast test logs
	testLogs := []models.Log{
		{
			Timestamp: time.Now(),
			Service:   "test-svc",
			Level:     "INFO",
			Message:   "websocket test message",
			Host:      "ws-host",
		},
	}
	srv.hub.broadcastLogs(testLogs)

	// Read the message
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read message: %v", err)
	}

	var receivedLogs []models.Log
	if err := json.Unmarshal(message, &receivedLogs); err != nil {
		t.Fatalf("failed to unmarshal message: %v", err)
	}

	if len(receivedLogs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(receivedLogs))
	}
	if receivedLogs[0].Service != "test-svc" {
		t.Errorf("expected service 'test-svc', got '%s'", receivedLogs[0].Service)
	}
	if receivedLogs[0].Message != "websocket test message" {
		t.Errorf("expected message 'websocket test message', got '%s'", receivedLogs[0].Message)
	}
}

// TestWebSocketDisconnect tests that disconnected clients are cleaned up.
func TestWebSocketDisconnect(t *testing.T) {
	srv := newTestServerWithHub(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Wait for registration
	time.Sleep(50 * time.Millisecond)
	if srv.hub.clientCount() != 1 {
		t.Errorf("expected 1 connected client, got %d", srv.hub.clientCount())
	}

	// Close the connection
	conn.Close()

	// Wait for unregistration
	time.Sleep(100 * time.Millisecond)
	if srv.hub.clientCount() != 0 {
		t.Errorf("expected 0 connected clients after disconnect, got %d", srv.hub.clientCount())
	}
}

// TestWebSocketMultipleClients tests broadcasting to multiple clients.
func TestWebSocketMultipleClients(t *testing.T) {
	srv := newTestServerWithHub(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", srv.handleWebSocket)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"

	// Connect two clients
	conn1, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("client 1 failed to connect: %v", err)
	}
	defer conn1.Close()

	conn2, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("client 2 failed to connect: %v", err)
	}
	defer conn2.Close()

	time.Sleep(50 * time.Millisecond)

	if srv.hub.clientCount() != 2 {
		t.Errorf("expected 2 connected clients, got %d", srv.hub.clientCount())
	}

	// Broadcast
	testLogs := []models.Log{
		{Timestamp: time.Now(), Service: "multi", Level: "INFO", Message: "broadcast test"},
	}
	srv.hub.broadcastLogs(testLogs)

	// Both clients should receive the message
	for i, conn := range []*websocket.Conn{conn1, conn2} {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("client %d failed to read: %v", i+1, err)
			continue
		}
		var logs []models.Log
		json.Unmarshal(msg, &logs)
		if len(logs) != 1 || logs[0].Message != "broadcast test" {
			t.Errorf("client %d received unexpected data", i+1)
		}
	}
}

// TestIngestBroadcastsViaWebSocket tests the full flow: ingest triggers WebSocket broadcast.
func TestIngestBroadcastsViaWebSocket(t *testing.T) {
	srv := newTestServerWithHub(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ws", srv.handleWebSocket)
	mux.HandleFunc("/api/ingest", srv.handleIngest)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Connect WebSocket client
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	time.Sleep(50 * time.Millisecond)

	// Ingest a log via HTTP
	logJSON := `{"service":"ws-test","level":"info","message":"realtime log"}`
	resp, err := http.Post(ts.URL+"/api/ingest", "application/json", strings.NewReader(logJSON))
	if err != nil {
		t.Fatalf("ingest request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, resp.StatusCode)
	}

	// WebSocket client should receive the log
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, message, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("failed to read WebSocket message: %v", err)
	}

	var receivedLogs []models.Log
	if err := json.Unmarshal(message, &receivedLogs); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(receivedLogs) != 1 {
		t.Fatalf("expected 1 log, got %d", len(receivedLogs))
	}
	if receivedLogs[0].Service != "ws-test" {
		t.Errorf("expected service 'ws-test', got '%s'", receivedLogs[0].Service)
	}
	if receivedLogs[0].Message != "realtime log" {
		t.Errorf("expected message 'realtime log', got '%s'", receivedLogs[0].Message)
	}
}
