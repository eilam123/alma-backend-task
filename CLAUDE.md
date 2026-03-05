# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
# Run the program
go run main.go

# Run all tests
go test ./...

# Run tests for a specific package
go test ./db/...
go test ./backend/...

# Run a single test
go test ./db/... -run TestExampleUsage
```

## Architecture

This is a Go telemetry analysis system that processes eBPF spans from Kubernetes observability agents. Module: `github.com/alma/assignment`.

### Data Flow

1. `main.go` initializes the DB schema via `createDBSchema()`, then loads spans from `data/ebpf_spans.json` via `ebpf_agent.EBPFAgent`
2. `backend/processor/processor.go` receives raw spans and stores structured data in the DB
3. `backend/api/api.go` queries the DB to serve `GetCatalog` and `GetConnections`

### Key Packages

- **`models/`** — `RawSpan` struct with `Attributes map[string]string` containing all eBPF span fields. Key attribute keys: `ebpf.span.type` (NETWORK/MESSAGE/QUERY), `ebpf.source`, `ebpf.destination`, `ebpf.http.path`, `ebpf.queue.topic`, `ebpf.db.query`, `ebpf.http.req_body`, `ebpf.http.resp_body`, `ebpf.queue.payload`, `ebpf.db.query.values`
- **`db/`** — In-memory ORM-like DB. `DB` implements the `Database` interface. Records are `map[string]any`. Tables require a `TableSchema` with `PrimaryKey` and optional `Indexes`. Join results prefix field names with `"tableName."`.
- **`ebpf_agent/`** — Reads and parses `data/ebpf_spans.json` into `[]models.RawSpan`

### DB API Patterns

```go
// Create table
db.CreateTable(ctx, db.TableSchema{Name: "...", Fields: [...], PrimaryKey: "id", Indexes: []string{"foreign_key_field"}})

// Query with filter
records, _ := db.Select(ctx, "tableName").Where("field", value).Execute(ctx)

// Upsert with custom merge (e.g., merging slices)
db.InsertOnConflict(ctx, "table", record, db.ConflictOptions{
    Action: db.ConflictDoUpdate,
    UpdateFields: []string{"field"},
    MergeFuncs: map[string]db.MergeFunc{"field": func(existing, new any) any { ... }},
})

// Join (results use "leftTable.field" / "rightTable.field" keys)
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

**PII Detection** — scan `ebpf.http.req_body`, `ebpf.http.resp_body`, `ebpf.queue.payload`, `ebpf.db.query.values` for sensitive data patterns. PII belongs to the component of the destination app item.

### Tasks to Implement

1. `createDBSchema()` in `main.go` — define tables for app items, components, connections
2. `processor.Process()` in `backend/processor/processor.go` — parse spans, detect app items/components/PIIs, store in DB
3. `api.GetCatalog()` in `backend/api/api.go` — return all app items with their components and PIIs
4. `api.GetConnections()` in `backend/api/api.go` — return all source→destination connections with components