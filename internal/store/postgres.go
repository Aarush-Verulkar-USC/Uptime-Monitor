package store

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/lib/pq"

	"github.com/aarush/uptime-monitor/internal/models"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(databaseURL string) (*PostgresStore, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, err
	}
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// --- Monitors ---

func (s *PostgresStore) CreateMonitor(ctx context.Context, req models.CreateMonitorRequest) (*models.Monitor, error) {
	method := req.Method
	if method == "" {
		method = "GET"
	}
	interval := req.IntervalSeconds
	if interval == 0 {
		interval = 30
	}
	timeout := req.TimeoutSeconds
	if timeout == 0 {
		timeout = 10
	}
	expected := req.ExpectedStatus
	if expected == 0 {
		expected = 200
	}

	m := &models.Monitor{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO monitors (name, url, method, interval_seconds, timeout_seconds, expected_status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, name, url, method, interval_seconds, timeout_seconds, expected_status, is_active, created_at, updated_at`,
		req.Name, req.URL, method, interval, timeout, expected,
	).Scan(&m.ID, &m.Name, &m.URL, &m.Method, &m.IntervalSeconds, &m.TimeoutSeconds, &m.ExpectedStatus, &m.IsActive, &m.CreatedAt, &m.UpdatedAt)
	return m, err
}

func (s *PostgresStore) GetMonitor(ctx context.Context, id string) (*models.Monitor, error) {
	m := &models.Monitor{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, method, interval_seconds, timeout_seconds, expected_status, is_active, created_at, updated_at
		 FROM monitors WHERE id = $1`, id,
	).Scan(&m.ID, &m.Name, &m.URL, &m.Method, &m.IntervalSeconds, &m.TimeoutSeconds, &m.ExpectedStatus, &m.IsActive, &m.CreatedAt, &m.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return m, err
}

func (s *PostgresStore) ListMonitors(ctx context.Context) ([]models.Monitor, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, method, interval_seconds, timeout_seconds, expected_status, is_active, created_at, updated_at
		 FROM monitors ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var monitors []models.Monitor
	for rows.Next() {
		var m models.Monitor
		if err := rows.Scan(&m.ID, &m.Name, &m.URL, &m.Method, &m.IntervalSeconds, &m.TimeoutSeconds, &m.ExpectedStatus, &m.IsActive, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		monitors = append(monitors, m)
	}
	return monitors, rows.Err()
}

func (s *PostgresStore) DeleteMonitor(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM monitors WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) ToggleMonitor(ctx context.Context, id string, active bool) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE monitors SET is_active = $1, updated_at = NOW() WHERE id = $2`, active, id)
	return err
}

// --- Checks ---

func (s *PostgresStore) InsertCheck(ctx context.Context, check *models.Check) error {
	return s.db.QueryRowContext(ctx,
		`INSERT INTO checks (monitor_id, status_code, response_time_ms, is_up, error)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id, checked_at`,
		check.MonitorID, check.StatusCode, check.ResponseTimeMs, check.IsUp, check.Error,
	).Scan(&check.ID, &check.CheckedAt)
}

func (s *PostgresStore) GetRecentChecks(ctx context.Context, monitorID string, limit int) ([]models.Check, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, monitor_id, status_code, response_time_ms, is_up, COALESCE(error, ''), checked_at
		 FROM checks WHERE monitor_id = $1
		 ORDER BY checked_at DESC LIMIT $2`, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var checks []models.Check
	for rows.Next() {
		var c models.Check
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.StatusCode, &c.ResponseTimeMs, &c.IsUp, &c.Error, &c.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

func (s *PostgresStore) GetUptimePercent(ctx context.Context, monitorID string, since time.Time) (float64, error) {
	var total, up int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COUNT(*) FILTER (WHERE is_up = TRUE)
		 FROM checks WHERE monitor_id = $1 AND checked_at > $2`,
		monitorID, since,
	).Scan(&total, &up)
	if err != nil || total == 0 {
		return 100.0, err
	}
	return float64(up) / float64(total) * 100, nil
}

func (s *PostgresStore) GetLatencyStats(ctx context.Context, monitorID string, since time.Time) (avg, p95 float64, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COALESCE(AVG(response_time_ms), 0),
		        COALESCE(PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time_ms), 0)
		 FROM checks WHERE monitor_id = $1 AND checked_at > $2 AND is_up = TRUE`,
		monitorID, since,
	).Scan(&avg, &p95)
	return
}

func (s *PostgresStore) GetLastCheck(ctx context.Context, monitorID string) (*models.Check, error) {
	c := &models.Check{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, monitor_id, status_code, response_time_ms, is_up, COALESCE(error, ''), checked_at
		 FROM checks WHERE monitor_id = $1
		 ORDER BY checked_at DESC LIMIT 1`, monitorID,
	).Scan(&c.ID, &c.MonitorID, &c.StatusCode, &c.ResponseTimeMs, &c.IsUp, &c.Error, &c.CheckedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// --- Incidents ---

func (s *PostgresStore) GetOpenIncident(ctx context.Context, monitorID string) (*models.Incident, error) {
	inc := &models.Incident{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, monitor_id, started_at, resolved_at, cause
		 FROM incidents WHERE monitor_id = $1 AND resolved_at IS NULL
		 ORDER BY started_at DESC LIMIT 1`, monitorID,
	).Scan(&inc.ID, &inc.MonitorID, &inc.StartedAt, &inc.ResolvedAt, &inc.Cause)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return inc, err
}

func (s *PostgresStore) CreateIncident(ctx context.Context, monitorID, cause string) (*models.Incident, error) {
	inc := &models.Incident{}
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO incidents (monitor_id, cause) VALUES ($1, $2)
		 RETURNING id, monitor_id, started_at, resolved_at, cause`,
		monitorID, cause,
	).Scan(&inc.ID, &inc.MonitorID, &inc.StartedAt, &inc.ResolvedAt, &inc.Cause)
	return inc, err
}

func (s *PostgresStore) ResolveIncident(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE incidents SET resolved_at = NOW() WHERE id = $1`, id)
	return err
}

func (s *PostgresStore) GetIncidents(ctx context.Context, monitorID string, limit int) ([]models.Incident, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, monitor_id, started_at, resolved_at, cause
		 FROM incidents WHERE monitor_id = $1
		 ORDER BY started_at DESC LIMIT $2`, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var incidents []models.Incident
	for rows.Next() {
		var inc models.Incident
		if err := rows.Scan(&inc.ID, &inc.MonitorID, &inc.StartedAt, &inc.ResolvedAt, &inc.Cause); err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}
