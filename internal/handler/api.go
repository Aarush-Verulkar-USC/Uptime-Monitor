package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aarush/uptime-monitor/internal/models"
	"github.com/aarush/uptime-monitor/internal/monitor"
	"github.com/aarush/uptime-monitor/internal/store"
)

type APIHandler struct {
	store     *store.PostgresStore
	cache     *store.Cache
	scheduler *monitor.Scheduler
}

func (h *APIHandler) CreateMonitor(w http.ResponseWriter, r *http.Request) {
	var req models.CreateMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == "" || req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name and url are required"})
		return
	}

	m, err := h.store.CreateMonitor(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create monitor"})
		return
	}

	h.scheduler.Schedule(m)
	writeJSON(w, http.StatusCreated, m)
}

func (h *APIHandler) ListMonitors(w http.ResponseWriter, r *http.Request) {
	monitors, err := h.store.ListMonitors(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list monitors"})
		return
	}

	result := make([]models.MonitorWithStatus, 0, len(monitors))
	for _, m := range monitors {
		result = append(result, h.enrichMonitor(r, m))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *APIHandler) GetMonitor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.store.GetMonitor(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database error"})
		return
	}
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitor not found"})
		return
	}

	result := h.enrichMonitor(r, *m)
	writeJSON(w, http.StatusOK, result)
}

func (h *APIHandler) DeleteMonitor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	h.scheduler.Unschedule(id)
	_ = h.cache.DeleteMonitorStatus(r.Context(), id)

	if err := h.store.DeleteMonitor(r.Context(), id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) ToggleMonitor(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	m, err := h.store.GetMonitor(r.Context(), id)
	if err != nil || m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "monitor not found"})
		return
	}

	newState := !m.IsActive
	if err := h.store.ToggleMonitor(r.Context(), id, newState); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to toggle"})
		return
	}

	if newState {
		m.IsActive = true
		h.scheduler.Schedule(m)
	} else {
		h.scheduler.Unschedule(id)
	}

	writeJSON(w, http.StatusOK, map[string]bool{"is_active": newState})
}

func (h *APIHandler) GetChecks(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	checks, err := h.store.GetRecentChecks(r.Context(), id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get checks"})
		return
	}
	writeJSON(w, http.StatusOK, checks)
}

func (h *APIHandler) enrichMonitor(r *http.Request, m models.Monitor) models.MonitorWithStatus {
	ctx := r.Context()
	now := time.Now()

	ws := models.MonitorWithStatus{Monitor: m}

	if cached, _ := h.cache.GetMonitorStatus(ctx, m.ID); cached != nil {
		ws.IsUp = cached.IsUp
		ws.ResponseTimeMs = cached.ResponseTimeMs
		t := time.Unix(cached.CheckedAt, 0)
		ws.LastCheckAt = &t
	} else if last, _ := h.store.GetLastCheck(ctx, m.ID); last != nil {
		ws.IsUp = last.IsUp
		ws.ResponseTimeMs = last.ResponseTimeMs
		ws.LastCheckAt = &last.CheckedAt
	}

	ws.Stats.Uptime24h, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-24*time.Hour))
	ws.Stats.Uptime7d, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-7*24*time.Hour))
	ws.Stats.Uptime30d, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-30*24*time.Hour))
	ws.Stats.AvgLatency, ws.Stats.P95Latency, _ = h.store.GetLatencyStats(ctx, m.ID, now.Add(-24*time.Hour))

	return ws
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
