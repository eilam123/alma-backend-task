# Telemetry Analysis System

A Go application that processes eBPF telemetry spans from Kubernetes observability agents, builds a service catalog with PII detection, and maps connections between distributed system components.

## What It Does

The system ingests raw eBPF spans (network calls, database queries, message queue interactions) and produces:

- **App Catalog** -- all discovered services, databases, queues, and internet sources, each with their components (endpoints, queries, topics) and detected PII types
- **Connection Map** -- all source-to-destination communication paths with the components involved

### Example Output

Given spans describing `internet -> mysupermarket-service -> users-service -> postgres-db`, the system produces:

```
=== Catalog ===
{
  "app_items": {
    "internet":              { "type": "INTERNET",  "components": [] },
    "mysupermarket-service": { "type": "SERVICE",   "components": [{ "component_type": "ENDPOINT", "value": "/checkout", "piis": ["CREDIT_CARD"] }] },
    "users-service":         { "type": "SERVICE",   "components": [{ "component_type": "ENDPOINT", "value": "/users/user-789", "piis": ["EMAIL", "PHONE"] }] },
    "kafka":                 { "type": "QUEUE",     "components": [{ "component_type": "QUEUE", "value": "order-events", "piis": [] }] },
    "postgres-db":           { "type": "DATABASE",  "components": [{ "component_type": "QUERY", "value": "SELECT * FROM users ...", "piis": ["EMAIL"] }] }
  }
}

=== Connections ===
[
  { "source": "internet",              "destination": "mysupermarket-service", "components": [{ "component_type": "ENDPOINT", "value": "/checkout" }] },
  { "source": "mysupermarket-service", "destination": "users-service",         "components": [{ "component_type": "ENDPOINT", "value": "/users/user-789" }] },
  ...
]
```

---

## Quick Start

### Prerequisites

- [Go 1.26+](https://go.dev/doc/install)

### Run

```bash
go run main.go
```

### Test

```bash
# All tests
go test ./...

# Specific package
go test ./backend/processor/...
go test ./backend/api/...
go test ./db/...

# Single test
go test ./backend/processor/... -run TestProcess_PII_Email

# Verbose output
go test -v ./...

# With race detection and coverage
go test -race -coverprofile=coverage.out ./...
```

### Docker

```bash
docker build -t telemetry-analysis .
docker run telemetry-analysis
```

### Configuration

| Environment Variable | Default | Description |
|---|---|---|
| `DATA_PATH` | `data/ebpf_spans.json` | Path to the eBPF spans input file |

---

## Architecture

### Data Flow

```
ebpf_spans.json
      |
      v
  EBPFAgent.GetSpans()          -- parse JSON into []RawSpan
      |
      v
  SpanProcessor.Process()       -- classify app items, extract components, detect PIIs, record connections
      |
      v
  In-Memory DB                  -- 4 tables: app_items, components, component_piis, connections
      |
      v
  APIBackend.GetCatalog()       -- assemble full catalog with components and PIIs
  APIBackend.GetConnections()   -- assemble connection map with components
      |
      v
  JSON output to stdout
```

### Project Structure

```
.
├── main.go                          # Entry point: orchestrates the pipeline
├── config/
│   └── config.go                    # Environment-based configuration
├── schema/
│   └── schema.go                    # Database table definitions
├── models/
│   └── models.go                    # Domain types: RawSpan, AppItem, Component, PII types
├── db/
│   ├── interface.go                 # Database interface (abstraction for DIP)
│   ├── memory.go                    # In-memory database implementation
│   └── example_test.go              # DB usage examples
├── ebpf_agent/
│   └── ebpf.go                      # Reads and parses ebpf_spans.json
├── backend/
│   ├── processor/
│   │   ├── processor.go             # Span processing logic
│   │   ├── processor_test.go        # 18 processor unit tests
│   │   ├── span_handler.go          # Extensible handler registry per span type
│   │   └── pii.go                   # PII detector definitions and regex patterns
│   └── api/
│       ├── api.go                   # GetCatalog and GetConnections implementation
│       └── api_test.go              # 6 API integration tests
├── data/
│   └── ebpf_spans.json              # 1600+ real eBPF span records
├── docs/                            # Comprehensive code walkthroughs (see Documentation)
├── Dockerfile                       # Multi-stage build
├── .github/workflows/
│   ├── ci.yml                       # Go vet, test with race/coverage, golangci-lint
│   ├── docker.yml                   # Docker build verification
│   ├── claude-code-review.yml       # Automated code review
│   └── claude.yml                   # PR assistant
├── CLAUDE.md                        # AI assistant instructions
└── go.mod                           # Module: github.com/alma/assignment
```

### Database Schema

Four in-memory tables, created by `schema.CreateSchema()`:

| Table | Primary Key | Fields | Indexes | Purpose |
|---|---|---|---|---|
| `app_items` | `name` | `name`, `type` | `type` | Discovered services, databases, queues, internet |
| `components` | `id` | `id`, `app_item_name`, `component_type`, `value` | `app_item_name`, `component_type` | Endpoints, queue topics, DB queries |
| `component_piis` | `id` | `id`, `component_id`, `pii_type` | `component_id`, `pii_type` | Detected PII per component |
| `connections` | `id` | `id`, `source`, `destination`, `component_ids` (JSON) | `source`, `destination` | Source-to-destination links |

---

## Domain Concepts

### App Item Types

Determined from span attributes:

| Type | Rule |
|---|---|
| `INTERNET` | Source is `"internet"` |
| `DATABASE` | Destination of a `QUERY` span |
| `QUEUE` | Destination of a `MESSAGE` span |
| `SERVICE` | Everything else |

### Component Types

Components always belong to the **destination** app item:

| Type | Span Type | Value Source |
|---|---|---|
| `ENDPOINT` | `NETWORK` | `ebpf.http.path` |
| `QUEUE` | `MESSAGE` | `ebpf.queue.topic` |
| `QUERY` | `QUERY` | `ebpf.db.query` |

### PII Detection

The processor scans payload fields for sensitive data using regex patterns and associates detected PII with the destination's component:

| PII Type | Pattern | Scanned Fields |
|---|---|---|
| `EMAIL` | Email addresses | HTTP req/resp body, queue payload, DB query values |
| `CREDIT_CARD` | 16-digit card numbers | HTTP req/resp body |
| `SSN` | `NNN-NN-NNNN` | DB query values |
| `PHONE` | Phone numbers with optional country code | HTTP req/resp body |
| `IP_ADDRESS` | IPv4 addresses | Queue payload |

Which fields are scanned depends on the span type:

| Span Type | Fields Scanned |
|---|---|
| `NETWORK` | `ebpf.http.req_body`, `ebpf.http.resp_body` |
| `QUERY` | `ebpf.db.query.values` |
| `MESSAGE` | `ebpf.queue.payload` |

---

## Design Principles

### Dependency Inversion

All consumers (`SpanProcessor`, `APIBackend`, `CreateSchema`) depend on the `db.Database` interface, not the concrete `*db.DB` implementation. This enables testability and backend swappability.

```go
type SpanProcessor struct {
    db           db.Database              // interface, not *db.DB
    handlers     map[string]SpanTypeHandler
    piiDetectors []PIIDetector
}
```

### Extensible Handler Registry

Span type processing is driven by a `SpanTypeHandler` interface and a registry map, not hard-coded switch statements. New span types can be added without modifying existing code:

```go
// Register a custom handler for a new span type
p := processor.New(database,
    processor.WithSpanHandler("GRPC", myGRPCHandler{}),
)
```

Each handler implements three methods:
- `DestinationAppItemType()` -- what type the destination becomes
- `ComponentInfo(attrs)` -- what component type and value to extract
- `PIIFields(attrs)` -- which attribute fields to scan for PII

### Pluggable PII Detectors

PII detection patterns can be replaced or extended:

```go
p := processor.New(database,
    processor.WithPIIDetectors(customDetectors),
)
```

### Efficient Querying

Both `GetCatalog` and `GetConnections` use bulk-fetch-then-index (3 `All()` calls + in-memory maps) instead of N+1 per-row queries.

---

## CI/CD

GitHub Actions workflows on every push to `main` and on pull requests:

- **CI** (`ci.yml`) -- `go vet`, `go test -race` with coverage, `golangci-lint` v2.11.1
- **Docker** (`docker.yml`) -- verifies the multi-stage Docker build succeeds
- **Code Review** (`claude-code-review.yml`) -- automated code review on PRs
- **PR Assistant** (`claude.yml`) -- AI-assisted PR feedback

---



## Testing

24 tests across two packages:

**Processor tests** (`backend/processor/processor_test.go`):
- Edge cases: empty spans, unknown span types
- App item type resolution: internet, service, database, queue
- Component creation: endpoints, queues, queries
- PII detection: email, credit card, SSN, phone, IP address, no false positives
- Deduplication: components and connections with merged component IDs

**API tests** (`backend/api/api_test.go`):
- Full catalog with all app items, types, components, and PIIs
- All connections with correct components
- Empty DB returns empty results (not errors)
- SSN and IP address PII types in catalog
- Multiple components merged into a single connection

---

## License

This project was developed as a backend engineering assignment.
