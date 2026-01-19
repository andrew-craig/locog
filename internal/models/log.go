package models

import "time"

type Log struct {
	ID        int64                  `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Service   string                 `json:"service"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Host      string                 `json:"host"`
	CreatedAt time.Time              `json:"created_at"`
}

type LogFilter struct {
	Service   string
	Level     string
	Host      string
	StartTime *time.Time
	EndTime   *time.Time
	Limit     int
	Search    string // Optional: full-text search in message
}

type FilterOptions struct {
	Services []string `json:"services"`
	Levels   []string `json:"levels"`
	Hosts    []string `json:"hosts"`
}
