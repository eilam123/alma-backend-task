# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run the program (processes spans, prints JSON, starts HTTP servers)
go run main.go

# Run all tests
go test ./...

# Run tests with race detector and coverage (as CI does)
go test -race -coverprofile=coverage.out ./...

# Run tests for a specific package
go test ./db/...
go test ./backend/...
go test ./server/...
go test ./config/...

# Run a single test
go test ./backend/processor/... -run TestProcess_PII_Email

# Lint (requires golangci-lint v2)
golangci-lint run ./...

# Docker build
docker build -t alma-backend .
```

## Architecture

Go telemetry analysis system that processes eBPF spans from Kubernetes observability agents. Module: `github.com/alma/assignment`. Go 1.26. Only external dependency: `github.com/prometheus/client_golang`.

### Data Flow

1. `config/` loads env vars (`DATA_PATH`, `HTTP_PORT`, `METRICS_PORT`, `LOG_LEVEL`)
2. `schema/schema.go` creates 4 DB tables: `app_items`, `components`, `component_piis`, `connections`
3. `ebpf_agent/` reads `data/ebpf_spans.json` into `[]models.RawSpan`
4. `backend/processor/` processes spans → upserts app items, components, PIIs, connections into the DB
5. `backend/api/` queries the DB to build catalog and connections responses
6. `server/` exposes REST API (`:8080/catalog`, `:8080/connections`) and Prometheus metrics (`:9090/metrics`)
7. `main.go` wires everything together with graceful SIGINT/SIGTERM shutdown

### Key Packages

- **`models/`** — `RawSpan` struct with `Attributes map[string]string`. Key attribute keys: `ebpf.span.type` (NETWORK/MESSAGE/QUERY), `ebpf.source`, `ebpf.destination`, `ebpf.http.path`, `ebpf.queue.topic`, `ebpf.db.query`, `ebpf.http.req_body`, `ebpf.http.resp_body`, `ebpf.queue.payload`, `ebpf.db.query.values`
- **`db/`** — In-memory ORM-like DB. `DB` implements the `Database` interface. Records are `map[string]any`. Tables require a `TableSchema` with `PrimaryKey` and optional `Indexes`. Join results prefix field names with `"tableName."`. Note: `Select().Where()` and `Join()` do full table scans — declared indexes are maintained but not used for query lookups.
- **`schema/`** — Centralized `CreateSchema()` defining all 4 tables with deterministic composite primary keys
- **`backend/processor/`** — `SpanProcessor` with pluggable `SpanTypeHandler` interface (NETWORK/QUERY/MESSAGE) and pluggable `PIIDetector` list. Uses functional options pattern (`WithLogger`, `WithSpanHandler`, `WithPIIDetectors`).
- **`backend/api/`** — `APIBackend` with `GetCatalog()` and `GetConnections()`. Fetches all records via `db.All()` and builds in-memory indexes for assembly.
- **`metrics/`** — Prometheus counters, histograms, and gauges. `metrics.Register()` to init. Instrumented in processor, API, and DB operations.
- **`server/`** — `APIServer` (REST) and `MetricsServer` (Prometheus). Both support graceful shutdown.
- **`config/`** — Env-based config with defaults. `SlogLevel()` converts string log level.
- **`ebpf_agent/`** — Reads and parses `data/ebpf_spans.json` into `[]models.RawSpan`

### DB Schema

All primary keys are deterministic composites — the processor computes them from span attributes and upserts directly (O(1) PK lookup, no query-before-write needed).

| Table | PK | Example PK | Write Pattern |
|---|---|---|---|
| `app_items` | `name` | `"postgres"` | `Upsert` |
| `components` | `id` = `TYPE:destination:value` | `"ENDPOINT:backend:/users"` | `Upsert` |
| `component_piis` | `id` = `componentID:piiType` | `"ENDPOINT:backend:/users:EMAIL"` | `InsertOnConflict(DoNothing)` |
| `connections` | `id` = `source:destination` | `"frontend:backend"` | `InsertOnConflict(DoUpdate)` with `mergeUniqueStrings` on `component_ids` |

### DB API Patterns

```go
db.CreateTable(ctx, db.TableSchema{Name: "...", Fields: [...], PrimaryKey: "id", Indexes: []string{"field"}})
db.Upsert(ctx, "table", record)
db.Get(ctx, "table", primaryKeyValue)
db.All(ctx, "table")
db.Select(ctx, "table").Where("field", value).Execute(ctx)
db.InsertOnConflict(ctx, "table", record, db.ConflictOptions{
    Action: db.ConflictDoUpdate,
    UpdateFields: []string{"field"},
    MergeFuncs: map[string]db.MergeFunc{"field": func(existing, new any) any { ... }},
})
db.Join("left", "right").On("left_field", "right_field").Where("right.field", val).Execute(ctx)
```

### Domain Concepts

**App Item Types** (determined from span attributes):
- `INTERNET` — source is `"internet"`
- `DATABASE` — destination of a `QUERY` span
- `QUEUE` — destination of a `MESSAGE` span
- `SERVICE` — everything else

**Component Types** (always belong to the **destination** app item):
- `ENDPOINT` — from NETWORK spans; uses `ebpf.http.path`
- `QUEUE` — from MESSAGE spans; uses `ebpf.queue.topic`
- `QUERY` — from QUERY spans; uses `ebpf.db.query`

**PII Detection** — regex-based scanning of `ebpf.http.req_body`, `ebpf.http.resp_body`, `ebpf.queue.payload`, `ebpf.db.query.values`. Detects EMAIL, CREDIT_CARD, SSN, PHONE, IP_ADDRESS. PII belongs to the component of the destination app item. Each span type handler defines which attributes to scan via `PIIFields()`.

### CI

GitHub Actions runs `go vet`, `go test -race` with coverage, and `golangci-lint` v2.11.1. Docker build verification in separate workflow.
