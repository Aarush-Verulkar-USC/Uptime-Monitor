package handler

import (
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/aarush/uptime-monitor/internal/monitor"
	"github.com/aarush/uptime-monitor/internal/store"
)

func NewRouter(pg *store.PostgresStore, cache *store.Cache, sched *monitor.Scheduler) *chi.Mux {
	api := &APIHandler{store: pg, cache: cache, scheduler: sched}
	pages := NewPageHandler(pg, cache)

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	// Dashboard pages
	r.Get("/", pages.Dashboard)
	r.Get("/monitors/{id}", pages.Detail)

	// HTMX partials
	r.Get("/partials/monitors", pages.MonitorCards)

	// JSON API
	r.Route("/api", func(r chi.Router) {
		r.Post("/monitors", api.CreateMonitor)
		r.Get("/monitors", api.ListMonitors)
		r.Get("/monitors/{id}", api.GetMonitor)
		r.Delete("/monitors/{id}", api.DeleteMonitor)
		r.Patch("/monitors/{id}/toggle", api.ToggleMonitor)
		r.Get("/monitors/{id}/checks", api.GetChecks)
	})

	return r
}
