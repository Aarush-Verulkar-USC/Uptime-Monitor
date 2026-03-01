# Uptime Monitor — Complete Project Explanation

---

## The 10-Second Pitch

> "I built a real-time uptime monitoring service from scratch in Go. You give it a list of websites, and it concurrently pings each one on a schedule, stores every response time in PostgreSQL, caches live status in Redis, and automatically tracks incidents — when a site goes down and when it comes back up. There's a server-rendered dashboard that updates in real time without a single line of React."

That's the version for recruiters. Now let's go deep.

---

## Table of Contents

1. [What This Project Actually Is](#1-what-this-project-actually-is)
2. [Why This Project Matters for Backend Roles](#2-why-this-project-matters-for-backend-roles)
3. [Architecture Overview](#3-architecture-overview)
4. [The Request Lifecycle — What Happens When You Add a Monitor](#4-the-request-lifecycle)
5. [The Check Lifecycle — What Happens Every 30 Seconds](#5-the-check-lifecycle)
6. [Component Deep-Dives](#6-component-deep-dives)
   - [6a. The Scheduler (Goroutine-Per-Monitor Pattern)](#6a-the-scheduler)
   - [6b. The HTTP Checker](#6b-the-http-checker)
   - [6c. PostgreSQL Store (Database Design)](#6c-postgresql-store)
   - [6d. Redis Cache (Caching Strategy)](#6d-redis-cache)
   - [6e. Incident Engine (Automatic Detection & Resolution)](#6e-incident-engine)
   - [6f. REST API Design](#6f-rest-api-design)
   - [6g. Dashboard (Server-Side Rendering + HTMX)](#6g-dashboard)
   - [6h. Graceful Shutdown](#6h-graceful-shutdown)
7. [Database Schema Explained](#7-database-schema-explained)
8. [Key Design Decisions and Trade-offs](#8-key-design-decisions-and-trade-offs)
9. [What an Interviewer Would Ask (and How to Answer)](#9-interview-prep)
10. [Resume Bullet Points](#10-resume-bullet-points)
11. [How to Talk About This Project](#11-how-to-talk-about-this-project)
12. [Future Enhancements (If They Ask "What Would You Add?")](#12-future-enhancements)

---

## 1. What This Project Actually Is

Think about UptimeRobot, Pingdom, or Better Uptime. Companies pay $20–200/month for these services to answer one question: **"Is my website up right now?"**

This project is a simplified version of that, built from scratch. Here's what it does:

1. You register a URL to monitor (e.g., `https://google.com`)
2. The system sends an HTTP request to that URL every N seconds (default: 30)
3. It records the response: status code, response time in milliseconds, whether it was "up" or "down"
4. All of this data goes into PostgreSQL for historical analysis
5. The latest status is cached in Redis so the dashboard loads instantly
6. If a site goes down, the system automatically opens an **incident**
7. When the site comes back up, the system automatically **resolves** the incident
8. A live dashboard shows everything: status, uptime percentage, response time charts, incident history

The entire thing is one Go binary, three database tables, two external dependencies (PostgreSQL + Redis), and zero JavaScript frameworks.

---

## 2. Why This Project Matters for Backend Roles

This project hits **six core backend skills** that interviewers look for:

| Skill | Where It Shows Up |
|-------|-------------------|
| **Concurrency** | One goroutine per monitor, managed with context cancellation and mutexes |
| **Database design** | Time-series check data, composite indexes, PERCENTILE_CONT for P95 latency |
| **Caching** | Redis for live status (write-through), PostgreSQL for historical queries |
| **API design** | RESTful JSON API with proper HTTP status codes, input validation, pagination |
| **Systems thinking** | Graceful shutdown, connection pooling, timeout layering, structured logging |
| **Infrastructure** | Docker Compose, multi-stage Dockerfile, health checks, environment-based config |

Most student projects are CRUD apps that read/write a database. This project has a **background processing engine** (the scheduler) running alongside the HTTP server — that's what makes it a backend project, not a web app.

---

## 3. Architecture Overview

```
                    ┌─────────────────────────────────────────────┐
                    │              Go HTTP Server                  │
                    │                                             │
  Browser/API ────▶ │  ┌─────────┐    ┌──────────┐               │
   (port 8080)      │  │  chi    │    │ Scheduler │               │
                    │  │ Router  │    │           │               │
                    │  └────┬────┘    │ ┌───────┐ │               │
                    │       │         │ │ Loop 1│──────┐          │
                    │  ┌────▼────┐    │ │ (30s) │ │    │          │
                    │  │ Handler │    │ ├───────┤ │    │          │
                    │  │  Layer  │    │ │ Loop 2│──────┤          │
                    │  └────┬────┘    │ │ (30s) │ │    │          │
                    │       │         │ ├───────┤ │    ▼          │
                    │       │         │ │ Loop 3│───▶ Checker     │
                    │       │         │ │ (30s) │ │  (HTTP GET)   │
                    │       │         │ └───────┘ │               │
                    │       │         └──────────┘               │
                    └───────┼─────────────────┼──────────────────┘
                            │                 │
                   ┌────────▼──┐        ┌─────▼─────┐
                   │ PostgreSQL│        │   Redis    │
                   │           │        │            │
                   │ monitors  │        │ Live status│
                   │ checks    │◀───────│ per monitor│
                   │ incidents │        │ (5m TTL)   │
                   └───────────┘        └────────────┘
```

There are **two independent processes** running inside the same Go binary:

1. **The HTTP server** — handles API requests and serves the dashboard
2. **The Scheduler** — runs background goroutines that perform health checks

They share the same PostgreSQL and Redis connections but otherwise operate independently. This is important: even if nobody is looking at the dashboard, the scheduler is still checking your sites.

---

## 4. The Request Lifecycle

**What happens when you create a monitor:**

```
POST /api/monitors {"name": "Google", "url": "https://google.com"}
                │
                ▼
    ┌─ api.go: CreateMonitor() ─┐
    │                           │
    │  1. Validate input        │
    │  2. Apply defaults        │
    │     (method=GET,          │
    │      interval=30s,        │
    │      timeout=10s,         │
    │      expected_status=200) │
    │                           │
    │  3. INSERT into           │
    │     PostgreSQL            │
    │     "monitors" table      │
    │                           │
    │  4. scheduler.Schedule()  │──────▶ Spawns a new goroutine
    │                           │        that starts checking
    │  5. Return 201 Created    │        immediately
    │     with monitor JSON     │
    └───────────────────────────┘
```

The key thing here: **step 4 happens in the same request**. The moment your API call returns, the scheduler has already started a goroutine for that monitor. The first check fires immediately — you don't have to wait for the interval.

**What happens when the dashboard loads:**

```
GET /
  │
  ▼
pages.go: Dashboard()
  │
  ├─ 1. List all monitors from PostgreSQL
  │
  ├─ 2. For each monitor:
  │     ├─ Try Redis cache first (O(1), sub-ms)
  │     │   └─ Cache hit? → Use cached status
  │     └─ Cache miss? → Query PostgreSQL for last check
  │
  ├─ 3. Compute uptime % for each (SQL aggregate)
  │
  └─ 4. Render HTML template, send to browser
       │
       └─ HTMX polls GET /partials/monitors every 10s
          to refresh the cards without a full page reload
```

The dashboard **never** shows stale data for longer than 10 seconds (the HTMX polling interval).

---

## 5. The Check Lifecycle

**What happens every 30 seconds for each monitor:**

```
Ticker fires
    │
    ▼
scheduler.runCheck()
    │
    ├─ 1. checker.Check()
    │     ├─ Create HTTP request with per-monitor timeout
    │     ├─ Send GET to the URL
    │     ├─ Measure elapsed time (time.Since)
    │     ├─ Compare status code to expected (e.g., 200)
    │     └─ Return Check{is_up, status_code, response_time_ms, error}
    │
    ├─ 2. store.InsertCheck()
    │     └─ INSERT into PostgreSQL "checks" table
    │
    ├─ 3. cache.SetMonitorStatus()
    │     └─ SET in Redis with 5-minute TTL
    │        Key: "monitor:status:{id}"
    │        Value: JSON{is_up, status_code, response_time_ms, checked_at}
    │
    └─ 4. scheduler.reconcileIncident()
          ├─ Query: is there an open incident for this monitor?
          │
          ├─ Site is DOWN + no open incident → CREATE incident
          ├─ Site is UP + open incident exists → RESOLVE incident
          └─ Otherwise → do nothing
```

This entire pipeline runs **per monitor, independently, in its own goroutine**. If you have 100 monitors, there are 100 goroutines running this loop concurrently. They don't block each other.

---

## 6. Component Deep-Dives

### 6a. The Scheduler

**File:** `internal/monitor/scheduler.go`

The scheduler is the heart of the system. It manages a **map of goroutines** — one per active monitor.

```
Scheduler
├── cancels: map[monitorID] → cancelFunc
├── Schedule(monitor)   → spawns goroutine, stores cancelFunc
├── Unschedule(id)      → calls cancelFunc, removes from map
└── Stop()              → calls all cancelFuncs (graceful shutdown)
```

**How it works:**

When you call `Schedule(monitor)`, the scheduler:

1. **Acquires a mutex lock** — because multiple HTTP requests could try to schedule monitors concurrently
2. **Checks if this monitor already has a running goroutine** — if so, cancels it first (prevents duplicates)
3. **Creates a new context** derived from the app-level context (not the HTTP request context — this was a bug we fixed)
4. **Stores the cancel function** in the map so it can be stopped later
5. **Launches a goroutine** that runs `loop(ctx, monitor)`

The `loop` function:
- Runs the first check **immediately** (don't wait 30 seconds for the first data point)
- Starts a `time.Ticker` at the monitor's interval
- Uses a `select` statement to either: (a) run a check when the ticker fires, or (b) exit when the context is canceled

**Why this matters:** This is the **goroutine-per-connection** pattern, adapted for scheduled tasks. It's the same pattern used in production systems like Prometheus node exporters and Kubernetes health checkers. The mutex + cancel function map ensures no goroutine leaks.

**Why not use a single goroutine with a priority queue?** A single goroutine would be simpler but creates a bottleneck — if one check is slow (e.g., 10 second timeout), it blocks all other checks. With goroutine-per-monitor, a slow check only affects that one monitor.

### 6b. The HTTP Checker

**File:** `internal/monitor/checker.go`

The checker is deliberately simple: it makes an HTTP request and measures how long it takes.

**Key details:**

- **Per-request timeout:** Each check creates its own `context.WithTimeout` using the monitor's configured timeout (default 10 seconds). This is layered on top of the HTTP client's global 30-second timeout. The per-request timeout wins because it's shorter.
- **Custom User-Agent:** Sets `User-Agent: UptimeMonitor/1.0` so site owners can identify monitoring traffic in their access logs.
- **Follows redirects:** The HTTP client follows redirects by default. So `https://google.com` → 301 → `https://www.google.com` → 200 counts as "up" (status 200).
- **No body reading:** We don't read the response body. We only care about the status code and timing. This keeps checks lightweight.
- **Error handling:** If the request fails entirely (DNS error, connection refused, timeout), `is_up` is false and the error message is stored.

**What gets measured:** `time.Since(start)` captures the full round-trip: DNS resolution + TCP handshake + TLS handshake + HTTP request/response. This is the same metric that tools like `curl -w %{time_total}` report.

### 6c. PostgreSQL Store

**File:** `internal/store/postgres.go`

The PostgreSQL store is the data layer. Every function takes a `context.Context` (for request cancellation and timeouts) and uses parameterized queries (for SQL injection protection).

**Connection pool configuration:**

```go
db.SetMaxOpenConns(25)     // At most 25 connections to PostgreSQL
db.SetMaxIdleConns(5)      // Keep 5 idle connections warm
db.SetConnMaxLifetime(5m)  // Recycle connections after 5 minutes
```

Why these numbers?
- **25 max connections:** PostgreSQL defaults to 100 max connections. With one service instance, 25 is plenty. Leaving headroom means you can run 3–4 instances before hitting limits.
- **5 idle connections:** Keeps common operations fast (no connection setup overhead) without wasting resources.
- **5-minute lifetime:** Prevents stale connections from accumulating, especially important in containerized environments where the database IP might change.

**Important queries:**

**Uptime percentage:**
```sql
SELECT COUNT(*), COUNT(*) FILTER (WHERE is_up = TRUE)
FROM checks WHERE monitor_id = $1 AND checked_at > $2
```
This uses PostgreSQL's `FILTER` clause — a single pass over the data returns both total checks and successful checks. The percentage is computed in Go: `up / total * 100`.

**P95 latency:**
```sql
SELECT PERCENTILE_CONT(0.95) WITHIN GROUP (ORDER BY response_time_ms)
FROM checks WHERE monitor_id = $1 AND checked_at > $2 AND is_up = TRUE
```
`PERCENTILE_CONT` is a PostgreSQL aggregate function that computes exact percentiles. P95 means "95% of requests were faster than this." It's the industry-standard latency metric — more useful than average because it isn't skewed by outliers.

We only compute P95 on successful checks (`is_up = TRUE`) because failed checks with 0ms response time would distort the metric.

### 6d. Redis Cache

**File:** `internal/store/cache.go`

Redis serves one purpose: **cache the current status of each monitor so the dashboard doesn't have to query PostgreSQL on every page load.**

**Cache key pattern:** `monitor:status:{uuid}`

**Cache value:** JSON containing `is_up`, `status_code`, `response_time_ms`, `checked_at`, `error`

**TTL:** 5 minutes. If the scheduler stops updating (e.g., the process crashes), the cached data automatically expires rather than showing stale "up" status indefinitely.

**Write-through pattern:** Every time a check completes, the result is written to both PostgreSQL (permanent record) AND Redis (live cache) in sequence. This means:
- The dashboard reads from Redis (fast, O(1), sub-millisecond)
- Historical analytics read from PostgreSQL (powerful queries, aggregations)
- If Redis is down, the dashboard falls back to PostgreSQL (graceful degradation)

**Why not just use PostgreSQL?** For a dashboard serving one user, PostgreSQL would be fine. But imagine 100 monitors with a dashboard that refreshes every 10 seconds — that's 100 `SELECT ... ORDER BY checked_at DESC LIMIT 1` queries every 10 seconds. With Redis, it's 100 `GET` commands that each return in <1ms. This pattern matters at scale even though it seems like over-engineering for a small project. It shows you understand **read-path optimization**.

### 6e. Incident Engine

**File:** `internal/monitor/scheduler.go` — `reconcileIncident()`

The incident engine runs after every check and implements a simple state machine:

```
                    Site goes DOWN
                 ┌─────────────────┐
                 ▼                 │
  ┌─────────┐       ┌──────────┐
  │ No open │──────▶│ Incident │
  │ incident│       │  OPEN    │
  └─────────┘       └────┬─────┘
       ▲                 │
       │    Site comes UP│
       │                 ▼
       │           ┌──────────┐
       └───────────│ Incident │
                   │ RESOLVED │
                   └──────────┘
```

**Rules:**
- Site is DOWN + no open incident → **Open a new incident** (record the cause)
- Site is UP + there IS an open incident → **Resolve it** (set `resolved_at = NOW()`)
- Site is DOWN + already have an open incident → **Do nothing** (don't create duplicates)
- Site is UP + no open incident → **Do nothing** (everything is fine)

**Why this matters:** This is **alert deduplication**. Without it, you'd get a new alert every 30 seconds while a site is down. The incident model groups a continuous outage into a single record with a start time, end time, and duration.

**What gets stored:** The incident records the `cause` — the actual error message from the failed check (e.g., "dial tcp: lookup example.com: no such host" or "expected status 200, got 503"). This is useful for post-incident analysis.

### 6f. REST API Design

**File:** `internal/handler/api.go`

The API follows RESTful conventions:

| Endpoint | Method | Status Codes | Purpose |
|----------|--------|-------------|---------|
| `/api/monitors` | POST | 201, 400, 500 | Create a monitor |
| `/api/monitors` | GET | 200, 500 | List all monitors with live status |
| `/api/monitors/{id}` | GET | 200, 404, 500 | Get one monitor with full stats |
| `/api/monitors/{id}` | DELETE | 204, 500 | Delete a monitor |
| `/api/monitors/{id}/toggle` | PATCH | 200, 404, 500 | Pause/resume monitoring |
| `/api/monitors/{id}/checks` | GET | 200, 500 | Get check history (paginated) |

**Design choices:**

- **201 Created** (not 200) for POST — correct REST semantics
- **204 No Content** for DELETE — no body needed, operation succeeded
- **PATCH for toggle** — partial update to a resource, not a full replacement
- **Query param pagination:** `?limit=50` with a maximum of 500. Prevents clients from dumping the entire checks table.
- **Input validation:** Name and URL are required. Defaults are applied server-side for optional fields (method=GET, interval=30s, timeout=10s, expected_status=200).

**The `enrichMonitor` pattern:** When listing monitors, the API doesn't just return the database record. It "enriches" each monitor with live status from Redis, uptime stats from PostgreSQL, and latency percentiles. This pattern — loading a base object and augmenting it with computed data — is extremely common in backend APIs.

### 6g. Dashboard (Server-Side Rendering + HTMX)

**Files:** `internal/handler/pages.go`, `internal/handler/templates/`

The dashboard is server-rendered HTML. No React, no Vue, no npm. This is a deliberate choice.

**Technology stack:**
- **Go `html/template`** — server-side HTML rendering with Go's standard library
- **HTMX** — 14KB library that makes HTML elements issue AJAX requests. Loaded via CDN.
- **Tailwind CSS** — utility-first CSS framework. Loaded via CDN.
- **Chart.js** — charting library for the response time graph. Loaded via CDN.

**How HTMX auto-refresh works:**

The monitor cards container has this attribute:
```html
<div hx-get="/partials/monitors" hx-trigger="every 10s" hx-swap="innerHTML">
```

Every 10 seconds, HTMX makes a GET request to `/partials/monitors`, which returns a fresh HTML fragment (just the cards, not the entire page). HTMX swaps the inner HTML of the container. No page reload, no JavaScript state management, no virtual DOM diffing. Just HTML in, HTML out.

**Template architecture:**

Go's template system doesn't have native "layout inheritance" like Jinja2. We work around this:
1. `base.html` defines a `{{define "base"}}` block with the full page shell (head, nav, main)
2. Inside `base.html`, it calls `{{template "content" .}}`
3. Each page template (`dashboard.html`, `detail.html`) defines a `{{define "content"}}` block
4. At startup, we parse `base.html` + each page template together into separate `*template.Template` objects
5. To render a page, we call `ExecuteTemplate(w, "base", data)`

**Why server-rendered?** For a backend project, you don't want the conversation to be about React. Server-side rendering keeps the focus on Go, and HTMX gives you real-time updates with zero frontend complexity. In an interview, you can say: "The dashboard is server-rendered with Go templates. I used HTMX for live updates — it polls the server every 10 seconds and swaps in fresh HTML fragments. No JavaScript framework, no build step."

### 6h. Graceful Shutdown

**File:** `cmd/server/main.go`

When the process receives SIGINT (Ctrl+C) or SIGTERM (Docker stop), the shutdown sequence is:

```
Signal received (SIGINT/SIGTERM)
        │
        ▼
1. scheduler.Stop()
   ├── Acquires mutex lock
   ├── Calls cancel() on every monitor goroutine
   ├── Each goroutine's select{} picks up ctx.Done()
   └── All goroutines exit cleanly (no mid-flight HTTP requests abandoned)
        │
        ▼
2. srv.Shutdown(10s timeout)
   ├── Stops accepting new connections
   ├── Waits for in-flight HTTP requests to complete
   └── Times out after 10 seconds if requests are still hanging
        │
        ▼
3. defer pg.Close()
   └── Closes all PostgreSQL connections in the pool

4. defer cache.Close()
   └── Closes the Redis connection
        │
        ▼
Process exits cleanly
```

**Why this matters:** Without graceful shutdown, `docker stop` would kill the process mid-check. A check that already sent an HTTP request but hasn't written to PostgreSQL yet would be lost. Graceful shutdown ensures every in-flight operation completes before the process exits.

---

## 7. Database Schema Explained

### `monitors` table — What we're watching

```sql
id UUID PRIMARY KEY DEFAULT gen_random_uuid()
```
UUIDs instead of auto-increment integers. Why? UUIDs can be generated anywhere (client, server, another service) without a database round-trip. They don't leak information about how many monitors exist (auto-increment IDs do: "oh, they only have 47 monitors").

```sql
interval_seconds INT NOT NULL DEFAULT 30
timeout_seconds INT NOT NULL DEFAULT 10
expected_status INT NOT NULL DEFAULT 200
```
Every check parameter is configurable per monitor. You might check your payment API every 10 seconds with a 5-second timeout, but check your blog every 5 minutes with a 30-second timeout.

### `checks` table — Raw time-series data

```sql
id BIGSERIAL PRIMARY KEY
```
BIGSERIAL (not UUID) for checks because: (a) checks are never referenced by ID from outside the system, (b) auto-increment integers are more storage-efficient, (c) BIGSERIAL supports billions of rows without overflow.

```sql
CREATE INDEX idx_checks_monitor_time ON checks(monitor_id, checked_at DESC);
```
This **composite index** is critical. Every query we run filters by `monitor_id` and sorts by `checked_at DESC`. Without this index, these queries would do a full table scan. With it, PostgreSQL can jump directly to the right monitor's checks in reverse chronological order.

**Why DESC?** Because we almost always want the most recent checks first (dashboard, last check, recent history). The descending index means PostgreSQL doesn't have to reverse-sort at query time.

### `incidents` table — Outage tracking

```sql
resolved_at TIMESTAMPTZ  -- nullable!
```
`resolved_at` is nullable. When an incident is open (ongoing), `resolved_at IS NULL`. When resolved, it gets a timestamp. This one nullable column lets us answer two questions with simple queries:
- Open incidents: `WHERE resolved_at IS NULL`
- Historical incidents: `WHERE resolved_at IS NOT NULL`

---

## 8. Key Design Decisions and Trade-offs

### Decision 1: One binary, not microservices

The HTTP server and scheduler run in the **same process**. This is intentional.

**Why not separate them?** Microservices add operational complexity: service discovery, inter-service communication, deployment coordination, distributed debugging. For a monitoring service with a single data flow (check → store → display), a monolith is simpler and has zero network overhead between components.

**When would you split them?** If the scheduler needed to run on dedicated machines (CPU-intensive checks) or if the API needed to scale independently (high dashboard traffic), you'd extract the scheduler into its own service communicating via Kafka or Redis pub/sub.

### Decision 2: Write-through cache, not cache-aside

Every check writes to both PostgreSQL AND Redis. Alternative: only write to PostgreSQL, and populate Redis lazily when the dashboard reads (cache-aside).

**Why write-through?** The dashboard polls every 10 seconds. With cache-aside, the first request after a check would be a cache miss → PostgreSQL query → cache write. With write-through, the cache is always warm. The dashboard never waits for PostgreSQL.

**Trade-off:** Write-through doubles the write path (two writes per check). For 100 monitors checking every 30 seconds, that's ~3 Redis writes per second. Redis handles millions of writes per second. This is negligible.

### Decision 3: Goroutine-per-monitor, not worker pool

Each monitor gets its own goroutine. Alternative: a fixed-size worker pool that processes checks from a queue.

**Why goroutine-per-monitor?** Simplicity. Each goroutine has its own ticker and context. Adding/removing monitors is just spawning/canceling goroutines. No queue to manage, no worker coordination.

**Trade-off:** With 10,000 monitors, you'd have 10,000 goroutines. Go handles this fine (goroutines are ~2KB of stack), but a worker pool would use less memory. For the expected scale (tens to hundreds of monitors), goroutine-per-monitor is the right choice.

### Decision 4: PostgreSQL for time-series data, not a time-series database

Checks are time-series data (timestamp + value). We store them in regular PostgreSQL tables, not a time-series database like TimescaleDB or InfluxDB.

**Why?** PostgreSQL is good enough. With the composite index on `(monitor_id, checked_at DESC)`, the queries we need (recent checks, uptime percentage, latency percentiles) are fast even at millions of rows. Adding TimescaleDB would be a one-line change (it's a PostgreSQL extension), but it's unnecessary for the expected data volume.

**When would you switch?** When the `checks` table exceeds ~100 million rows, you'd benefit from TimescaleDB's automatic partitioning (hypertables), compression, and retention policies.

### Decision 5: HTMX, not React

The dashboard uses server-rendered HTML with HTMX for live updates. No JavaScript framework.

**Why?** This is a backend project. Adding React would shift the conversation in interviews from "tell me about your Go backend" to "why did you choose React over Vue." HTMX keeps the dashboard logic in Go (template rendering, data fetching) and adds just enough interactivity (auto-refresh) without a build step.

---

## 9. Interview Prep — What They'll Ask and How to Answer

### "Walk me through the architecture."

Start with the two-process model: "There's an HTTP server and a background scheduler running in the same Go process. The scheduler manages one goroutine per monitor — each goroutine independently checks its URL on a configurable interval. Check results go to PostgreSQL for historical data and Redis for live status. The HTTP server serves both a JSON API and a server-rendered dashboard."

### "How do you handle concurrency?"

"Each monitor runs in its own goroutine with its own ticker and context. The scheduler uses a mutex-protected map of cancel functions to track active goroutines. When a monitor is added, Schedule() spawns a goroutine and stores its cancel function. When removed, Unschedule() calls the cancel function, which signals the goroutine to exit via context.Done(). This prevents goroutine leaks and ensures clean shutdown."

### "What if a check is slow? Does it block other monitors?"

"No. Each monitor runs in its own goroutine with its own timeout. If Google takes 10 seconds to respond, that goroutine is blocked but the other 99 goroutines continue checking independently. The per-request timeout (configurable, default 10s) ensures no goroutine hangs forever."

### "Why Redis? Isn't PostgreSQL enough?"

"For the dashboard, I need the current status of every monitor on every page load. With PostgreSQL, that's N queries like `SELECT ... ORDER BY checked_at DESC LIMIT 1` — one per monitor, hitting the disk. With Redis, it's N `GET` commands that return in sub-millisecond time from memory. At 100 monitors with a 10-second poll interval, Redis saves about 600 database queries per minute. Plus, if PostgreSQL is under load from write-heavy check inserts, the read path (dashboard) is completely decoupled."

### "How would you scale this to 10,000 monitors?"

"Three things: (1) Add a worker pool with a semaphore to cap concurrent outbound HTTP requests — 10,000 simultaneous connections would be aggressive. (2) Batch-insert checks instead of one INSERT per check — use PostgreSQL's `COPY` protocol or multi-row inserts. (3) Partition the checks table by time (weekly or monthly) so queries only scan recent partitions. Redis handles the live status fine at any scale."

### "How would you add alerting?"

"After the incident is created in reconcileIncident(), I'd publish to an alert channel. This could be a webhook POST to Slack/Discord, or a message to a Redis pub/sub channel that an alert worker consumes. The key design choice is: the check loop should not block on alert delivery. So the alert would be asynchronous — fire-and-forget to a queue, with the alert worker handling retries."

### "What happens if the service crashes mid-check?"

"The worst case is a check that completed the HTTP request but didn't write to PostgreSQL yet — we lose that one data point. It's acceptable because the next check runs 30 seconds later. Redis has a 5-minute TTL, so stale cached status automatically expires. When the service restarts, it loads all active monitors from PostgreSQL and starts checking them again. No manual intervention needed."

### "How do you prevent SQL injection?"

"Every query uses parameterized placeholders ($1, $2, etc.) instead of string concatenation. The `database/sql` package sends the query and parameters separately to PostgreSQL, which handles escaping. There's no place in the codebase where user input is interpolated into a SQL string."

---

## 10. Resume Bullet Points

Pick 2–3 of these depending on what you want to emphasize:

**Architecture focus:**
> Built a real-time uptime monitoring service in Go with a concurrent goroutine-per-monitor scheduler, PostgreSQL for time-series check storage, and Redis write-through caching for sub-millisecond dashboard reads

**Database focus:**
> Designed a time-series data model in PostgreSQL with composite indexes and aggregate queries (PERCENTILE_CONT, FILTER) computing uptime percentage and P95 latency across configurable time windows

**Concurrency focus:**
> Implemented a goroutine-per-monitor scheduling engine with context-based cancellation, mutex-protected lifecycle management, and graceful shutdown draining all in-flight checks before process exit

**Full-stack backend focus:**
> Engineered an uptime monitoring platform with a RESTful API, automatic incident detection/resolution, and a server-rendered HTMX dashboard with 10-second live refresh — deployed as a single Docker Compose stack

---

## 11. How to Talk About This Project

### To a recruiter (30 seconds):

"I built a website monitoring service — you give it a list of URLs, and it checks if they're up every 30 seconds. It tracks response time, uptime percentage, and automatically detects outages. It has a live dashboard that shows everything in real time. Built in Go with PostgreSQL and Redis."

### To an engineer (2 minutes):

"It's an uptime monitoring service in Go. The core is a scheduler that runs one goroutine per monitor, each on its own ticker. Every check does an HTTP request, measures latency, writes the result to PostgreSQL, and updates a Redis cache for the dashboard. There's an incident engine that automatically opens and resolves incidents based on check results — basically a simple state machine that deduplicates alerts. The dashboard is server-rendered with HTMX for live polling. Everything runs in Docker Compose — one binary, three Postgres tables, one Redis instance."

### To a hiring manager (1 minute):

"I wanted to build something that shows real backend skills — not just a CRUD app. This is a monitoring service like UptimeRobot. It runs background workers that check websites, stores time-series data in PostgreSQL, uses Redis caching for the dashboard, and has automatic incident tracking. It's all in Go, containerized with Docker, and the architecture is designed for clear separation of concerns — the scheduler, data layer, API, and dashboard are all independent packages."

---

## 12. Future Enhancements

If an interviewer asks "what would you add next?", pick one:

1. **Webhook alerting** — POST to Slack/Discord/PagerDuty when an incident opens or resolves. Would add an async alert worker with retry and exponential backoff.

2. **Multi-region checking** — Deploy checker instances in different AWS regions. Same monitor gets checked from 3 locations. Only open an incident if 2/3 agree it's down (prevents false positives from network partitions).

3. **SSL certificate monitoring** — During the TLS handshake, extract the certificate expiry date. Alert 30 days before it expires. One extra field per check, zero extra HTTP requests.

4. **Check data retention** — Background job that deletes checks older than 90 days, or rolls them up into hourly/daily aggregates. Prevents the checks table from growing unbounded.

5. **Horizontal scaling** — Use Redis distributed locks (SETNX) so multiple service instances don't check the same monitor twice. Each instance acquires a lock before scheduling a monitor.
