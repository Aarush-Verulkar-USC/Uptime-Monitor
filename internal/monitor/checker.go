package monitor

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/aarush/uptime-monitor/internal/models"
)

type Checker struct {
	client *http.Client
}

func NewChecker() *Checker {
	return &Checker{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Checker) Check(ctx context.Context, m *models.Monitor) *models.Check {
	check := &models.Check{
		MonitorID: m.ID,
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(m.TimeoutSeconds)*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, m.Method, m.URL, nil)
	if err != nil {
		check.IsUp = false
		check.Error = err.Error()
		return check
	}
	req.Header.Set("User-Agent", "UptimeMonitor/1.0")

	start := time.Now()
	resp, err := c.client.Do(req)
	check.ResponseTimeMs = int(time.Since(start).Milliseconds())

	if err != nil {
		check.IsUp = false
		check.Error = err.Error()
		return check
	}
	defer resp.Body.Close()

	check.StatusCode = resp.StatusCode
	check.IsUp = resp.StatusCode == m.ExpectedStatus
	if !check.IsUp {
		check.Error = fmt.Sprintf("expected status %d, got %d", m.ExpectedStatus, resp.StatusCode)
	}

	return check
}
