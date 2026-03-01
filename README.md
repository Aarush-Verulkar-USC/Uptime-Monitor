# Uptime Monitor

A real-time website uptime monitoring service built in Go. Add URLs to watch, and the service concurrently checks them at configurable intervals, stores response latency in PostgreSQL, caches live status in Redis, and delivers failure alerts with automatic incident tracking.

Includes a server-rendered dashboard (Go templates + HTMX) that auto-refreshes every 10 seconds.

## Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│   Dashboard  │────▶│   Go HTTP    │────▶│  PostgreSQL   │
│  (HTMX +     │     │   Server     │     │  (checks,     │
│   Tailwind)  │◀────│              │     │   incidents)  │
└──────────────┘     │  ┌────────┐  │     └──────────────┘
                     │  │Scheduler│  │
┌──────────────┐     │  │        │  │     ┌──────────────┐
│   REST API   │────▶│  │Checker │──│────▶│    Redis      │
│  (JSON)      │◀────│  │Goroutines│ │    │  (status      │
└──────────────┘     │  └────────┘  │     │   cache)      │
                     └──────────────┘     └──────────────┘
```

**Key components:**

- **Scheduler** — manages one goroutine per active monitor, each running on its own ticker
- **Checker** — performs HTTP requests with configurable timeouts, records response time and status
- **Incident Engine** — automatically opens incidents on failure and resolves them on recovery
- **Redis Cache** — stores current status per monitor so the dashboard never hits PostgreSQL for live status
- **HTMX Dashboard** — server-rendered HTML that auto-refreshes monitor cards every 10 seconds

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22 |
| HTTP Router | chi |
| Database | PostgreSQL 16 |
| Cache | Redis 7 |
| Dashboard | Go html/template + HTMX + Tailwind CSS + Chart.js |
| Infrastructure | Docker Compose |

## Quick Start

```bash
# Start everything (PostgreSQL + Redis + App)
make up

# Open dashboard
open http://localhost:8080
```

**Development mode** (run Go server locally, databases in Docker):

```bash
make dev
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Dashboard UI |
| `GET` | `/monitors/{id}` | Monitor detail page |
| `POST` | `/api/monitors` | Create a monitor |
| `GET` | `/api/monitors` | List all monitors with live status |
| `GET` | `/api/monitors/{id}` | Get monitor details + stats |
| `DELETE` | `/api/monitors/{id}` | Delete a monitor |
| `PATCH` | `/api/monitors/{id}/toggle` | Pause/resume monitoring |
| `GET` | `/api/monitors/{id}/checks` | Get recent check history |

### Create a Monitor

```bash
curl -X POST http://localhost:8080/api/monitors \
  -H "Content-Type: application/json" \
  -d '{"name": "Google", "url": "https://google.com", "interval_seconds": 30}'
```

### List Monitors with Status

```bash
curl http://localhost:8080/api/monitors | jq
```

## Project Structure

```
├── cmd/server/main.go          # Entrypoint, graceful shutdown
├── internal/
│   ├── models/models.go        # Data types
│   ├── store/
│   │   ├── postgres.go         # All database operations
│   │   └── cache.go            # Redis status cache
│   ├── monitor/
│   │   ├── checker.go          # HTTP health check logic
│   │   └── scheduler.go        # Goroutine-per-monitor scheduling
│   └── handler/
│       ├── router.go           # Route definitions
│       ├── api.go              # REST API handlers
│       ├── pages.go            # Dashboard page rendering
│       └── templates/          # HTML templates (embedded)
├── migrations/001_init.sql     # Database schema
├── docker-compose.yml          # PostgreSQL + Redis + App
├── Dockerfile                  # Multi-stage build
└── Makefile                    # Build/run shortcuts
```

## What This Project Demonstrates

- **Concurrency** — goroutine-per-monitor pattern with context-based cancellation
- **Database design** — time-series check data with proper indexing, uptime/percentile queries
- **Caching** — Redis for live status, PostgreSQL for historical data
- **Incident management** — automatic open/resolve lifecycle tracking
- **API design** — RESTful JSON API with proper status codes and validation
- **Graceful shutdown** — signal handling, scheduler drain, HTTP server shutdown
- **Server-side rendering** — Go templates with HTMX for real-time updates without a SPA
