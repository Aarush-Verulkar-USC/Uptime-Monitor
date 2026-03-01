# Uptime Monitor

A real-time website uptime monitoring service built in Go. Register URLs to watch, and the service concurrently checks them at configurable intervals, stores every response in PostgreSQL, caches live status in Redis, and automatically tracks incidents — opening when a site goes down, resolving when it recovers.

Includes a server-rendered dashboard (Go templates + HTMX) that auto-refreshes every 10 seconds without a JavaScript framework.

## Architecture

<p align="center">
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 820 480" width="820" height="480" font-family="monospace, sans-serif" font-size="13">

  <!-- Background -->
  <rect width="820" height="480" fill="#0d1117" rx="12"/>

  <!-- Go HTTP Server box -->
  <rect x="260" y="30" width="300" height="310" rx="10" fill="#161b22" stroke="#30363d" stroke-width="1.5"/>
  <text x="410" y="56" text-anchor="middle" fill="#58a6ff" font-size="14" font-weight="bold">Go HTTP Server</text>

  <!-- chi Router -->
  <rect x="280" y="70" width="100" height="44" rx="6" fill="#1f2937" stroke="#374151" stroke-width="1"/>
  <text x="330" y="89" text-anchor="middle" fill="#e6edf3" font-size="11">chi</text>
  <text x="330" y="104" text-anchor="middle" fill="#e6edf3" font-size="11">Router</text>

  <!-- Handler Layer -->
  <rect x="280" y="140" width="100" height="44" rx="6" fill="#1f2937" stroke="#374151" stroke-width="1"/>
  <text x="330" y="159" text-anchor="middle" fill="#e6edf3" font-size="11">Handler</text>
  <text x="330" y="174" text-anchor="middle" fill="#e6edf3" font-size="11">Layer</text>

  <!-- Scheduler box -->
  <rect x="410" y="65" width="130" height="200" rx="6" fill="#1a2332" stroke="#1d4ed8" stroke-width="1"/>
  <text x="475" y="85" text-anchor="middle" fill="#60a5fa" font-size="11" font-weight="bold">Scheduler</text>

  <!-- Loop boxes inside Scheduler -->
  <rect x="422" y="95" width="106" height="32" rx="4" fill="#172033" stroke="#2563eb" stroke-width="1"/>
  <text x="475" y="116" text-anchor="middle" fill="#93c5fd" font-size="10">Loop 1 (30s)</text>

  <rect x="422" y="136" width="106" height="32" rx="4" fill="#172033" stroke="#2563eb" stroke-width="1"/>
  <text x="475" y="157" text-anchor="middle" fill="#93c5fd" font-size="10">Loop 2 (30s)</text>

  <rect x="422" y="177" width="106" height="32" rx="4" fill="#172033" stroke="#2563eb" stroke-width="1"/>
  <text x="475" y="198" text-anchor="middle" fill="#93c5fd" font-size="10">Loop N (30s)</text>

  <!-- Checker -->
  <rect x="422" y="225" width="106" height="32" rx="4" fill="#172033" stroke="#2563eb" stroke-width="1"/>
  <text x="475" y="241" text-anchor="middle" fill="#93c5fd" font-size="10">Checker</text>
  <text x="475" y="252" text-anchor="middle" fill="#93c5fd" font-size="10">(HTTP GET)</text>

  <!-- Arrow: chi to Handler -->
  <line x1="330" y1="114" x2="330" y2="140" stroke="#4b5563" stroke-width="1.5" marker-end="url(#arr)"/>

  <!-- Arrow: Loop N to Checker -->
  <line x1="475" y1="209" x2="475" y2="225" stroke="#3b82f6" stroke-width="1.5" marker-end="url(#arr)"/>

  <!-- Browser/API label + arrow to chi -->
  <rect x="60" y="78" width="110" height="28" rx="6" fill="#1f2937" stroke="#374151" stroke-width="1"/>
  <text x="115" y="97" text-anchor="middle" fill="#9ca3af" font-size="11">Browser / API</text>
  <line x1="170" y1="92" x2="280" y2="92" stroke="#4b5563" stroke-width="1.5" marker-end="url(#arr)"/>

  <!-- PostgreSQL box -->
  <rect x="60" y="360" width="200" height="90" rx="8" fill="#161b22" stroke="#16a34a" stroke-width="1.5"/>
  <text x="160" y="385" text-anchor="middle" fill="#4ade80" font-size="13" font-weight="bold">PostgreSQL</text>
  <text x="160" y="405" text-anchor="middle" fill="#86efac" font-size="11">monitors · checks</text>
  <text x="160" y="420" text-anchor="middle" fill="#86efac" font-size="11">incidents</text>
  <text x="160" y="438" text-anchor="middle" fill="#6b7280" font-size="10">time-series + aggregates</text>

  <!-- Redis box -->
  <rect x="560" y="360" width="200" height="90" rx="8" fill="#161b22" stroke="#dc2626" stroke-width="1.5"/>
  <text x="660" y="385" text-anchor="middle" fill="#f87171" font-size="13" font-weight="bold">Redis</text>
  <text x="660" y="405" text-anchor="middle" fill="#fca5a5" font-size="11">monitor:status:{id}</text>
  <text x="660" y="420" text-anchor="middle" fill="#fca5a5" font-size="11">live status cache</text>
  <text x="660" y="438" text-anchor="middle" fill="#6b7280" font-size="10">5-minute TTL</text>

  <!-- Arrow: Handler to PostgreSQL -->
  <line x1="330" y1="184" x2="160" y2="360" stroke="#16a34a" stroke-width="1.5" stroke-dasharray="6,3" marker-end="url(#arrg)"/>

  <!-- Arrow: Checker to PostgreSQL -->
  <line x1="440" y1="257" x2="200" y2="362" stroke="#16a34a" stroke-width="1.5" marker-end="url(#arrg)"/>

  <!-- Arrow: Checker to Redis -->
  <line x1="528" y1="245" x2="620" y2="362" stroke="#dc2626" stroke-width="1.5" marker-end="url(#arrr)"/>

  <!-- Arrow: Handler reads Redis (dashed) -->
  <line x1="380" y1="175" x2="620" y2="362" stroke="#dc2626" stroke-width="1.5" stroke-dasharray="6,3" marker-end="url(#arrr)"/>

  <!-- Labels on arrows -->
  <text x="200" y="315" fill="#4ade80" font-size="10" transform="rotate(-30 200 315)">write</text>
  <text x="520" y="310" fill="#f87171" font-size="10" transform="rotate(25 520 310)">write</text>
  <text x="450" y="290" fill="#f87171" font-size="10" transform="rotate(18 450 290)">read (cache)</text>

  <!-- Legend -->
  <text x="60" y="470" fill="#4b5563" font-size="10">━━ solid = sync write path   ╍╍ dashed = read path</text>

  <!-- Arrow markers -->
  <defs>
    <marker id="arr" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
      <path d="M0,0 L0,6 L8,3 z" fill="#4b5563"/>
    </marker>
    <marker id="arrg" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
      <path d="M0,0 L0,6 L8,3 z" fill="#16a34a"/>
    </marker>
    <marker id="arrr" markerWidth="8" markerHeight="8" refX="6" refY="3" orient="auto">
      <path d="M0,0 L0,6 L8,3 z" fill="#dc2626"/>
    </marker>
  </defs>
</svg>
</p>

Two independent processes run in the same Go binary: the **HTTP server** handles API requests and the dashboard, while the **Scheduler** runs background goroutines that continuously perform health checks — even with no users on the dashboard.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Language | Go 1.22 |
| HTTP Router | chi |
| Database | PostgreSQL 16 |
| Cache | Redis 7 |
| Dashboard | Go html/template + HTMX + Tailwind CSS + Chart.js |
| Infrastructure | Docker Compose, multi-stage Dockerfile |

## Quick Start

```bash
# Start everything (PostgreSQL + Redis + App)
make up

# Open the dashboard
open http://localhost:8080
```

**Development mode** (run Go server locally with databases in Docker):

```bash
make dev
```

## API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Dashboard UI |
| `GET` | `/monitors/{id}` | Monitor detail page |
| `POST` | `/api/monitors` | Create a monitor |
| `GET` | `/api/monitors` | List all monitors with live status |
| `GET` | `/api/monitors/{id}` | Get monitor details + uptime stats |
| `DELETE` | `/api/monitors/{id}` | Delete a monitor |
| `PATCH` | `/api/monitors/{id}/toggle` | Pause/resume monitoring |
| `GET` | `/api/monitors/{id}/checks` | Get paginated check history |

### Create a Monitor

```bash
curl -X POST http://localhost:8080/api/monitors \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Google",
    "url": "https://google.com",
    "interval_seconds": 30,
    "timeout_seconds": 10,
    "expected_status": 200
  }'
```

### List Monitors

```bash
curl http://localhost:8080/api/monitors | jq
```

## Project Structure

```
├── cmd/server/main.go              # Entrypoint, graceful shutdown
├── internal/
│   ├── models/models.go            # Shared data types
│   ├── store/
│   │   ├── postgres.go             # All database operations
│   │   └── cache.go                # Redis status cache
│   ├── monitor/
│   │   ├── checker.go              # HTTP health check logic
│   │   └── scheduler.go            # Goroutine-per-monitor engine + incident reconciliation
│   └── handler/
│       ├── router.go               # Route definitions
│       ├── api.go                  # REST API handlers
│       ├── pages.go                # Dashboard page rendering
│       └── templates/              # Embedded HTML templates
├── migrations/001_init.sql         # Database schema
├── docker-compose.yml              # PostgreSQL + Redis + App
├── Dockerfile                      # Multi-stage build
└── Makefile                        # Build/run shortcuts
```

## How It Works

### Scheduler

The scheduler maintains a mutex-protected `map[monitorID]cancelFunc`. Calling `Schedule(monitor)` spawns a goroutine that immediately runs the first check, then fires on a `time.Ticker` at the configured interval. Calling `Unschedule(id)` cancels that goroutine's context. This is the goroutine-per-monitor pattern — each monitor's slow check never blocks another.

### Check Pipeline (per tick)

1. **Checker** — sends HTTP GET with per-request timeout, measures total round-trip latency
2. **PostgreSQL write** — inserts result into the `checks` time-series table
3. **Redis write** — updates `monitor:status:{id}` (5-minute TTL) for instant dashboard reads
4. **Incident reconciliation** — down + no open incident → create; up + open incident → resolve

### Incident Engine

A state machine that deduplicates alerts. A continuous outage becomes a single incident record with `started_at` and an eventual `resolved_at`. The `cause` field captures the actual error (e.g., `"dial tcp: no such host"`) for post-incident analysis.

### Dashboard

Server-rendered HTML with HTMX polling every 10 seconds — no React, no build step:

```html
<div hx-get="/partials/monitors" hx-trigger="every 10s" hx-swap="innerHTML">
```

### Caching Strategy

Write-through: every check writes to both PostgreSQL (permanent) and Redis (live cache). The dashboard reads from Redis (sub-ms), falling back to PostgreSQL on cache miss. Historical analytics — uptime %, P95 latency — always query PostgreSQL.

## Database Schema

| Table | Purpose |
|-------|---------|
| `monitors` | What to watch (URL, interval, timeout, expected status, enabled flag) |
| `checks` | Raw time-series results (is_up, status_code, response_time_ms, checked_at) |
| `incidents` | Outage records (started_at, resolved_at nullable, cause) |

Key index: `CREATE INDEX idx_checks_monitor_time ON checks(monitor_id, checked_at DESC)` — all dashboard and history queries filter on `monitor_id` and sort by time descending.

## Environment Variables

| Variable | Description |
|----------|-------------|
| `PORT` | HTTP server port (default: `8080`) |
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_URL` | Redis connection string |

## What This Demonstrates

- **Concurrency** — goroutine-per-monitor with context cancellation and mutex-guarded lifecycle
- **Database design** — time-series schema with composite indexes, `PERCENTILE_CONT` for P95, `FILTER` for uptime %
- **Caching** — write-through Redis cache decoupling the read path from write-heavy check inserts
- **Incident management** — automatic open/resolve lifecycle with alert deduplication
- **API design** — RESTful JSON with correct status codes (201, 204), server-side defaults, pagination
- **Graceful shutdown** — SIGINT/SIGTERM handling that drains the scheduler before process exit
- **Server-side rendering** — Go templates with HTMX for live updates without a SPA
