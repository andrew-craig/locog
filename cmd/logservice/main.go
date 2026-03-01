package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"locog/internal/db"
	"locog/internal/models"

	"golang.org/x/time/rate"
)

//go:embed static/*
var staticFiles embed.FS

// server holds the application dependencies
type server struct {
	db      *db.DB
	limiter *ipRateLimiter
	hub     *wsHub
}

// ipRateLimiter implements per-IP rate limiting
type ipRateLimiter struct {
	limiters sync.Map // map[string]*rate.Limiter
	rate     rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	return &ipRateLimiter{
		rate:  r,
		burst: burst,
	}
}

func (l *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	if limiter, exists := l.limiters.Load(ip); exists {
		return limiter.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(l.rate, l.burst)
	l.limiters.Store(ip, limiter)
	return limiter
}

func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Take the first IP in the list
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Fall back to RemoteAddr
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func main() {
	dbPath := flag.String("db", "logs.db", "Path to SQLite database")
	addr := flag.String("addr", ":5081", "HTTP service address")
	flag.Parse()

	// Initialize structured JSON logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	database, err := db.New(*dbPath)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// Rate limiter: 100 requests/sec per IP with burst of 100
	limiter := newIPRateLimiter(rate.Limit(100), 100)

	hub := newWSHub()
	go hub.run()

	srv := &server{db: database, limiter: limiter, hub: hub}

	// Start cleanup routine (runs daily)
	go srv.cleanupRoutine()

	mux := http.NewServeMux()

	// Ingestion endpoint (used by Vector)
	mux.HandleFunc("/api/ingest", srv.handleIngest)

	// WebSocket endpoint for real-time log streaming
	mux.HandleFunc("/api/ws", srv.handleWebSocket)

	// Query endpoints (used by Web UI)
	mux.HandleFunc("/api/logs", srv.handleQueryLogs)
	mux.HandleFunc("/api/filters", srv.handleGetFilters)

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Serve embedded static files (Web UI)
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		slog.Error("failed to create static file system", "error", err)
		os.Exit(1)
	}
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: corsMiddleware(mux),
	}

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan

		slog.Info("shutting down gracefully")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			slog.Error("http server shutdown error", "error", err)
		}
	}()

	slog.Info("log service starting", "addr", *addr)
	if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("http server error", "error", err)
		os.Exit(1)
	}
	slog.Info("server stopped")
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

	// Check rate limit
	ip := getClientIP(r)
	if !s.limiter.getLimiter(ip).Allow() {
		http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
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
			// Marshal the invalid log entry for debugging (truncate if too large)
			logJSON, _ := json.Marshal(logs[i])
			logBody := string(logJSON)
			if len(logBody) > 500 {
				logBody = logBody[:500] + "... (truncated)"
			}

			slog.Warn("invalid log entry",
				"sender", ip,
				"index", i,
				"total_logs", len(logs),
				"reason", err.Error(),
				"log_body", logBody,
			)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	// Batch insert for better performance
	if len(logs) > 1 {
		if err := s.db.InsertBatch(r.Context(), logs); err != nil {
			slog.Error("failed to insert batch", "error", err, "count", len(logs))
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	} else if len(logs) == 1 {
		if err := s.db.InsertLog(r.Context(), &logs[0]); err != nil {
			slog.Error("failed to insert log", "error", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	// Broadcast new logs to WebSocket clients
	if s.hub != nil {
		s.hub.broadcastLogs(logs)
	}

	w.WriteHeader(http.StatusCreated)
}

// apiError is a structured JSON error response for API endpoints.
type apiError struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

func writeJSONError(w http.ResponseWriter, status int, code, message, details string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiError{Error: message, Code: code, Details: details})
}

// retentionPeriod is the log retention window used for query warnings.
const retentionPeriod = 30 * 24 * time.Hour

func (s *server) handleQueryLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filter := models.LogFilter{
		Service: r.URL.Query().Get("service"),
		Level:   r.URL.Query().Get("level"),
		Host:    r.URL.Query().Get("host"),
		Search:  r.URL.Query().Get("search"),
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			slog.Warn("invalid limit", "limit", limitStr, "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid_limit",
				"Invalid limit value",
				fmt.Sprintf("'limit' must be a positive integer, got: %s", limitStr))
			return
		}
		if limit < 0 {
			slog.Warn("negative limit", "limit", limit)
			writeJSONError(w, http.StatusBadRequest, "invalid_limit",
				"Invalid limit value", "limit must not be negative")
			return
		}
		filter.Limit = limit
	}

	if start := r.URL.Query().Get("start"); start != "" {
		t, err := time.Parse(time.RFC3339, start)
		if err != nil {
			slog.Warn("invalid start date", "start", start, "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid_date",
				"Invalid start date format",
				fmt.Sprintf("'start' must be RFC3339 (e.g. 2025-01-15T00:00:00Z), got: %s", start))
			return
		}
		filter.StartTime = &t
	}

	if end := r.URL.Query().Get("end"); end != "" {
		t, err := time.Parse(time.RFC3339, end)
		if err != nil {
			slog.Warn("invalid end date", "end", end, "error", err)
			writeJSONError(w, http.StatusBadRequest, "invalid_date",
				"Invalid end date format",
				fmt.Sprintf("'end' must be RFC3339 (e.g. 2025-01-15T23:59:59Z), got: %s", end))
			return
		}
		filter.EndTime = &t
	}

	if filter.StartTime != nil && filter.EndTime != nil && filter.StartTime.After(*filter.EndTime) {
		slog.Warn("start date after end date",
			"start", filter.StartTime.Format(time.RFC3339),
			"end", filter.EndTime.Format(time.RFC3339))
		writeJSONError(w, http.StatusBadRequest, "date_range_invalid",
			"Start date must be before end date",
			fmt.Sprintf("start (%s) is after end (%s)",
				filter.StartTime.Format(time.RFC3339), filter.EndTime.Format(time.RFC3339)))
		return
	}

	// Warn when query falls outside the retention window
	retentionCutoff := time.Now().Add(-retentionPeriod)
	if filter.EndTime != nil && filter.EndTime.Before(retentionCutoff) {
		w.Header().Set("X-Locog-Warning", "Query end date is beyond the 30-day retention window. Logs older than 30 days are automatically deleted.")
		slog.Info("query entirely outside retention window",
			"end", filter.EndTime.Format(time.RFC3339),
			"retention_cutoff", retentionCutoff.Format(time.RFC3339))
	} else if filter.StartTime != nil && filter.StartTime.Before(retentionCutoff) {
		w.Header().Set("X-Locog-Warning", fmt.Sprintf(
			"Query start date is beyond the 30-day retention window. Results will only include logs from %s onwards.",
			retentionCutoff.Format("2006-01-02")))
		slog.Info("query partially outside retention window",
			"start", filter.StartTime.Format(time.RFC3339),
			"retention_cutoff", retentionCutoff.Format(time.RFC3339))
	}

	logs, err := s.db.QueryLogs(r.Context(), filter)
	if err != nil {
		slog.Error("query failed", "error", err, "filter", filter)
		writeJSONError(w, http.StatusInternalServerError, "query_failed",
			"Query failed", "An internal error occurred while querying logs")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func (s *server) handleGetFilters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()
	options, err := s.db.GetFilterOptions(r.Context())
	duration := time.Since(start)
	if err != nil {
		slog.Error("failed to get filter options", "error", err, "duration_ms", duration.Milliseconds())
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if duration > 500*time.Millisecond {
		slog.Warn("slow filter options response", "duration_ms", duration.Milliseconds())
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
	// Use a timeout context for cleanup operations
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Delete logs older than 30 days
	start := time.Now()
	slog.Info("starting log cleanup")
	deleted, err := s.db.DeleteOldLogs(ctx, 30*24*time.Hour)
	duration := time.Since(start)
	if err != nil {
		slog.Error("cleanup failed", "error", err, "duration_ms", duration.Milliseconds())
	} else {
		slog.Info("log cleanup completed", "deleted", deleted, "duration_ms", duration.Milliseconds())
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
