package main

import (
	"encoding/json"
	"flag"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"locog/internal/db"
	"locog/internal/models"
)

var database *db.DB

func main() {
	dbPath := flag.String("db", "logs.db", "Path to SQLite database")
	addr := flag.String("addr", ":8080", "HTTP service address")
	flag.Parse()

	var err error
	database, err = db.New(*dbPath)
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// Start cleanup routine (runs daily)
	go cleanupRoutine()

	// Ingestion endpoint (used by Vector)
	http.HandleFunc("/api/ingest", handleIngest)

	// Query endpoints (used by Web UI)
	http.HandleFunc("/api/logs", handleQueryLogs)
	http.HandleFunc("/api/filters", handleGetFilters)

	// Health check
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Serve static files (Web UI)
	http.Handle("/", http.FileServer(http.Dir("./web/static")))

	log.Printf("Log service starting on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func handleIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read the body
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

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

	// Set timestamp if not provided
	for i := range logs {
		if logs[i].Timestamp.IsZero() {
			logs[i].Timestamp = time.Now()
		}
	}

	// Batch insert for better performance
	if len(logs) > 1 {
		if err := database.InsertBatch(logs); err != nil {
			log.Printf("Failed to insert batch: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	} else if len(logs) == 1 {
		if err := database.InsertLog(&logs[0]); err != nil {
			log.Printf("Failed to insert log: %v", err)
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func handleQueryLogs(w http.ResponseWriter, r *http.Request) {
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

	logs, err := database.QueryLogs(filter)
	if err != nil {
		log.Printf("Query failed: %v", err)
		http.Error(w, "Query failed", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

func handleGetFilters(w http.ResponseWriter, r *http.Request) {
	options, err := database.GetFilterOptions()
	if err != nil {
		log.Printf("Failed to get filter options: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(options)
}

func cleanupRoutine() {
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		// Delete logs older than 30 days
		deleted, err := database.DeleteOldLogs(30 * 24 * time.Hour)
		if err != nil {
			log.Printf("Cleanup failed: %v", err)
		} else if deleted > 0 {
			log.Printf("Cleaned up %d old logs", deleted)
		}
	}
}
