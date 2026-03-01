package models

import "time"

type Monitor struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	Method          string    `json:"method"`
	IntervalSeconds int       `json:"interval_seconds"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	ExpectedStatus  int       `json:"expected_status"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type Check struct {
	ID             int64     `json:"id"`
	MonitorID      string    `json:"monitor_id"`
	StatusCode     int       `json:"status_code"`
	ResponseTimeMs int       `json:"response_time_ms"`
	IsUp           bool      `json:"is_up"`
	Error          string    `json:"error,omitempty"`
	CheckedAt      time.Time `json:"checked_at"`
}

type Incident struct {
	ID         string     `json:"id"`
	MonitorID  string     `json:"monitor_id"`
	StartedAt  time.Time  `json:"started_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
	Cause      string     `json:"cause"`
}

type CreateMonitorRequest struct {
	Name            string `json:"name"`
	URL             string `json:"url"`
	Method          string `json:"method,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty"`
	ExpectedStatus  int    `json:"expected_status,omitempty"`
}

type MonitorStats struct {
	Uptime24h  float64 `json:"uptime_24h"`
	Uptime7d   float64 `json:"uptime_7d"`
	Uptime30d  float64 `json:"uptime_30d"`
	AvgLatency float64 `json:"avg_latency_ms"`
	P95Latency float64 `json:"p95_latency_ms"`
}

type MonitorWithStatus struct {
	Monitor
	IsUp           bool         `json:"is_up"`
	LastCheckAt    *time.Time   `json:"last_check_at,omitempty"`
	ResponseTimeMs int          `json:"response_time_ms"`
	Stats          MonitorStats `json:"stats"`
}
