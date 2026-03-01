package monitor

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/aarush/uptime-monitor/internal/models"
	"github.com/aarush/uptime-monitor/internal/store"
)

type Scheduler struct {
	store   *store.PostgresStore
	cache   *store.Cache
	checker *Checker

	appCtx  context.Context
	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewScheduler(s *store.PostgresStore, c *store.Cache, ch *Checker) *Scheduler {
	return &Scheduler{
		store:   s,
		cache:   c,
		checker: ch,
		cancels: make(map[string]context.CancelFunc),
	}
}

// Start loads all active monitors and begins checking them.
func (s *Scheduler) Start(ctx context.Context) error {
	s.appCtx = ctx

	monitors, err := s.store.ListMonitors(ctx)
	if err != nil {
		return err
	}
	for i := range monitors {
		if monitors[i].IsActive {
			s.Schedule(&monitors[i])
		}
	}
	slog.Info("scheduler started", "monitors", len(monitors))
	return nil
}

// Schedule starts a background goroutine that periodically checks a monitor.
// Uses the long-lived app context, not the caller's request context.
func (s *Scheduler) Schedule(m *models.Monitor) {
	s.mu.Lock()
	if cancel, exists := s.cancels[m.ID]; exists {
		cancel()
	}
	ctx, cancel := context.WithCancel(s.appCtx)
	s.cancels[m.ID] = cancel
	s.mu.Unlock()

	go s.loop(ctx, m)
}

// Unschedule stops the background goroutine for a monitor.
func (s *Scheduler) Unschedule(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cancel, exists := s.cancels[id]; exists {
		cancel()
		delete(s.cancels, id)
	}
}

func (s *Scheduler) loop(ctx context.Context, m *models.Monitor) {
	slog.Info("monitor started", "id", m.ID, "url", m.URL, "interval_s", m.IntervalSeconds)

	s.runCheck(ctx, m)

	ticker := time.NewTicker(time.Duration(m.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("monitor stopped", "id", m.ID)
			return
		case <-ticker.C:
			s.runCheck(ctx, m)
		}
	}
}

func (s *Scheduler) runCheck(ctx context.Context, m *models.Monitor) {
	check := s.checker.Check(ctx, m)

	if err := s.store.InsertCheck(ctx, check); err != nil {
		slog.Error("failed to persist check", "monitor_id", m.ID, "err", err)
		return
	}

	_ = s.cache.SetMonitorStatus(ctx, m.ID, store.CachedStatus{
		IsUp:           check.IsUp,
		StatusCode:     check.StatusCode,
		ResponseTimeMs: check.ResponseTimeMs,
		CheckedAt:      check.CheckedAt.Unix(),
		Error:          check.Error,
	})

	s.reconcileIncident(ctx, m, check)

	slog.Debug("check done", "monitor_id", m.ID, "up", check.IsUp, "ms", check.ResponseTimeMs)
}

func (s *Scheduler) reconcileIncident(ctx context.Context, m *models.Monitor, check *models.Check) {
	open, err := s.store.GetOpenIncident(ctx, m.ID)
	if err != nil {
		slog.Error("incident lookup failed", "err", err)
		return
	}

	if !check.IsUp && open == nil {
		cause := check.Error
		if cause == "" {
			cause = "endpoint unreachable"
		}
		if _, err := s.store.CreateIncident(ctx, m.ID, cause); err != nil {
			slog.Error("create incident failed", "err", err)
		}
		slog.Warn("incident opened", "monitor_id", m.ID, "cause", cause)
	} else if check.IsUp && open != nil {
		if err := s.store.ResolveIncident(ctx, open.ID); err != nil {
			slog.Error("resolve incident failed", "err", err)
		}
		slog.Info("incident resolved", "monitor_id", m.ID)
	}
}

// Stop cancels all running monitor goroutines.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, cancel := range s.cancels {
		cancel()
		delete(s.cancels, id)
	}
	slog.Info("scheduler stopped")
}
