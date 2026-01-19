package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"locog/internal/db"
	"locog/internal/models"
)

// server holds the application dependencies
type server struct {
	db *db.DB
}

func main() {
	dbPath := flag.String("db", "logs.db", "Path to SQLite database")
	addr := flag.String("addr", ":8080", "HTTP service address")
	flag.Parse()

	database, err := db.New(*dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	srv := &server{db: database}

	// Start cleanup routine (runs daily)
	go srv.cleanupRoutine()

	mux := http.NewServeMux()

	// Ingestion endpoint (used by Vector)
	mux.HandleFunc("/api/ingest", srv.handleIngest)

	// Query endpoints (used by Web UI)
	mux.HandleFunc("/api/logs", srv.handleQueryLogs)
	mux.HandleFunc("/api/filters", srv.handleGetFilters)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Serve static files (Web UI)
	mux.Handle("/", http.FileServer(http.Dir("./web/static")))

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: corsMiddleware(mux),
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		log.Println("Shutting down gracefully...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
	}()

	log.Printf("Log service starting on %s", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal("HTTP server error:", err)
	}
	log.Println("Server stopped")
}

// corsMiddleware adds CORS headers to responses
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// maxBodySize is the maximum allowed request body size (10MB)
const maxBodySize = 10 << 20

func (s *server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit request body size to prevent memory exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	defer r.Body.Close()

	// Read the body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body or body too large", http.StatusBadRequest)
		return
	}

	// Support both single log and batch
	var logs []models.Log

	// Try to decode as array first
	if err := json.Unmarshal(bodyBytes, &logs); err != nil {
		// If that fails, try single log
		var singleLog models.Log
		if err := json.Unmarshal(bodyBytes, &singleLog); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		logs = []models.Log{singleLog}
	}

	// Validate and set defaults for each log
	for i := range logs {
		// Set timestamp if not provided
		if logs[i].Timestamp.IsZero() {
			logs[i].Timestamp = time.Now()
		}

		// Validate required fields
		if err := validateLog(&logs[i]); err != nil {
			log.Printf("Invalid log entry at index %d: %v", i, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Batch insert for better performance
	if len(logs) > 1 {
		if err := s.db.InsertBatch(logs); err != nil {
			log.Printf("Failed to insert batch: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	} else if len(logs) == 1 {
		if err := s.db.InsertLog(&logs[0]); err != nil {
			log.Printf("Failed to insert log: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func (s *server) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	filter := models.LogFilter{
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Host:    r.URL.Query().Get("host"),
		Search:  r.URL.Query().Get("search"),
	}

	if limit := r.URL.Query().Get("limit"); limit != "" {
		filter.Limit, _ = strconv.Atoi(limit)
	}

	if start := r.URL.Query().Get("start"); start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err == nil {
			filter.StartTime = &t
		}
	}

	if end := r.URL.Query().Get("end"); end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err == nil {
			filter.EndTime = &t
		}
	}

	logs, err := s.db.QueryLogs(filter)
	if err != nil {
		log.Printf("Query failed: %v", err)
		http.Error(w, "Query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *server) handleGetFilters(w http.ResponseWriter, r *http.Request) {
	options, err := s.db.GetFilterOptions()
	if err != nil {
		log.Printf("Failed to get filter options: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func (s *server) cleanupRoutine() {
	// Run cleanup immediately on startup
	s.runCleanup()

	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		s.runCleanup()
	}
}

func (s *server) runCleanup() {
	// Delete logs older than 30 days
	deleted, err := s.db.DeleteOldLogs(30 * 24 * time.Hour)
	if err != nil {
		log.Printf("Cleanup failed: %v", err)
	} else if deleted > 0 {
		log.Printf("Cleaned up %d old logs", deleted)
	}
}

func validateLog(l *models.Log) error {
	if strings.TrimSpace(l.Service) == "" {
		return fmt.Errorf("missing required field: service")
	}
	if strings.TrimSpace(l.Level) == "" {
		return fmt.Errorf("missing required field: level")
	}
	if strings.TrimSpace(l.Message) == "" {
		return fmt.Errorf("missing required field: message")
	}
	return nil
}
