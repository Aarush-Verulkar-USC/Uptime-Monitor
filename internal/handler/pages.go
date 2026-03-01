package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/aarush/uptime-monitor/internal/models"
	"github.com/aarush/uptime-monitor/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

var funcMap = template.FuncMap{
	"timeAgo": func(t *time.Time) string {
		if t == nil {
			return "never"
		}
		d := time.Since(*t)
		switch {
		case d < time.Minute:
			return "just now"
		case d < time.Hour:
			return fmt.Sprintf("%dm ago", int(d.Minutes()))
		case d < 24*time.Hour:
			return fmt.Sprintf("%dh ago", int(d.Hours()))
		default:
			return fmt.Sprintf("%dd ago", int(d.Hours()/24))
		}
	},
	"formatUptime": func(v float64) string {
		return fmt.Sprintf("%.2f", v)
	},
	"formatMs": func(ms int) string {
		return fmt.Sprintf("%d", ms)
	},
	"int": func(f float64) int {
		return int(f)
	},
	"formatTime": func(t time.Time) string {
		return t.Format("Jan 02, 15:04")
	},
	"formatDuration": func(start time.Time, end *time.Time) string {
		e := time.Now()
		if end != nil {
			e = *end
		}
		d := e.Sub(start)
		switch {
		case d < time.Minute:
			return fmt.Sprintf("%ds", int(d.Seconds()))
		case d < time.Hour:
			return fmt.Sprintf("%dm", int(d.Minutes()))
		default:
			return fmt.Sprintf("%dh %dm", int(d.Hours()), int(math.Mod(d.Minutes(), 60)))
		}
	},
	"jsonMarshal": func(v any) template.JS {
		b, _ := json.Marshal(v)
		return template.JS(b)
	},
	"reverse": func(checks []models.Check) []models.Check {
		n := len(checks)
		out := make([]models.Check, n)
		for i, c := range checks {
			out[n-1-i] = c
		}
		return out
	},
}

var pageTmpl map[string]*template.Template

func init() {
	pages := []string{"dashboard.html", "detail.html"}
	pageTmpl = make(map[string]*template.Template, len(pages))
	for _, p := range pages {
		t := template.Must(
			template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/base.html", "templates/"+p),
		)
		pageTmpl[p] = t
	}
	// Standalone partial (no base layout)
	pageTmpl["monitor_cards.html"] = template.Must(
		template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/monitor_cards.html"),
	)
}

type PageHandler struct {
	store *store.PostgresStore
	cache *store.Cache
}

func NewPageHandler(pg *store.PostgresStore, c *store.Cache) *PageHandler {
	return &PageHandler{store: pg, cache: c}
}

type dashboardData struct {
	Monitors []monitorCard
	Total    int
	Up       int
	Down     int
}

type monitorCard struct {
	models.Monitor
	IsUp           bool
	ResponseTimeMs int
	LastCheckAt    *time.Time
	Uptime24h      float64
}

func (h *PageHandler) buildCards(r *http.Request) ([]monitorCard, error) {
	monitors, err := h.store.ListMonitors(r.Context())
	if err != nil {
		return nil, err
	}

	cards := make([]monitorCard, 0, len(monitors))
	for _, m := range monitors {
		card := monitorCard{Monitor: m, IsUp: true, Uptime24h: 100}

		if cached, _ := h.cache.GetMonitorStatus(r.Context(), m.ID); cached != nil {
			card.IsUp = cached.IsUp
			card.ResponseTimeMs = cached.ResponseTimeMs
			t := time.Unix(cached.CheckedAt, 0)
			card.LastCheckAt = &t
		} else if last, _ := h.store.GetLastCheck(r.Context(), m.ID); last != nil {
			card.IsUp = last.IsUp
			card.ResponseTimeMs = last.ResponseTimeMs
			card.LastCheckAt = &last.CheckedAt
		}

		card.Uptime24h, _ = h.store.GetUptimePercent(r.Context(), m.ID, time.Now().Add(-24*time.Hour))
		cards = append(cards, card)
	}
	return cards, nil
}

func (h *PageHandler) Dashboard(w http.ResponseWriter, r *http.Request) {
	cards, err := h.buildCards(r)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}

	data := dashboardData{Monitors: cards, Total: len(cards)}
	for _, c := range cards {
		if c.IsUp {
			data.Up++
		} else {
			data.Down++
		}
	}

	w.Header().Set("Content-Type", "text/html")
	pageTmpl["dashboard.html"].ExecuteTemplate(w, "base", data)
}

func (h *PageHandler) MonitorCards(w http.ResponseWriter, r *http.Request) {
	cards, err := h.buildCards(r)
	if err != nil {
		http.Error(w, "internal error", 500)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	pageTmpl["monitor_cards.html"].ExecuteTemplate(w, "monitor_cards", cards)
}

type detailData struct {
	Monitor        models.Monitor
	IsUp           bool
	ResponseTimeMs int
	LastCheckAt    *time.Time
	Stats          models.MonitorStats
	Checks         []models.Check
	Incidents      []models.Incident
	ChartLabels    []string
	ChartValues    []int
}

func (h *PageHandler) Detail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	m, err := h.store.GetMonitor(ctx, id)
	if err != nil || m == nil {
		http.Error(w, "not found", 404)
		return
	}

	data := detailData{Monitor: *m, IsUp: true}
	now := time.Now()

	if cached, _ := h.cache.GetMonitorStatus(ctx, m.ID); cached != nil {
		data.IsUp = cached.IsUp
		data.ResponseTimeMs = cached.ResponseTimeMs
		t := time.Unix(cached.CheckedAt, 0)
		data.LastCheckAt = &t
	} else if last, _ := h.store.GetLastCheck(ctx, m.ID); last != nil {
		data.IsUp = last.IsUp
		data.ResponseTimeMs = last.ResponseTimeMs
		data.LastCheckAt = &last.CheckedAt
	}

	data.Stats.Uptime24h, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-24*time.Hour))
	data.Stats.Uptime7d, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-7*24*time.Hour))
	data.Stats.Uptime30d, _ = h.store.GetUptimePercent(ctx, m.ID, now.Add(-30*24*time.Hour))
	data.Stats.AvgLatency, data.Stats.P95Latency, _ = h.store.GetLatencyStats(ctx, m.ID, now.Add(-24*time.Hour))

	data.Checks, _ = h.store.GetRecentChecks(ctx, m.ID, 100)
	data.Incidents, _ = h.store.GetIncidents(ctx, m.ID, 20)

	for _, c := range data.Checks {
		data.ChartLabels = append(data.ChartLabels, c.CheckedAt.Format("15:04"))
		data.ChartValues = append(data.ChartValues, c.ResponseTimeMs)
	}

	w.Header().Set("Content-Type", "text/html")
	pageTmpl["detail.html"].ExecuteTemplate(w, "base", data)
}
