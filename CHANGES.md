# Project Evolution: From Scaffold to Production

This document walks through every meaningful change made to this project, PR by PR, function by function. It assumes you know Python but not Go. All Go syntax is explained using Python analogies.

---

## 1. Reading Go as a Python Developer

Before diving in, here are the Go constructs you'll encounter throughout this document:

| Go syntax | Python equivalent | Notes |
|---|---|---|
| `func foo(x int) string` | `def foo(x: int) -> str:` | Same idea, different order |
| `type Foo struct { ... }` | `@dataclass class Foo: ...` | A plain data container |
| `type Database interface { ... }` | `class Database(Protocol): ...` | Like `typing.Protocol` or an ABC — defines *what* an object must do, not *how* |
| `func (p *SpanProcessor) Process(...)` | `def process(self, ...)` on a class | Methods are defined outside the struct body in Go |
| `*db.DB` (concrete type) | Instance of a concrete class | The `*` means "pointer to" — think of it as the object itself |
| `db.Database` (interface) | Abstract base class / Protocol | Any struct that implements all the methods satisfies the interface |
| `map[string]any` | `dict[str, Any]` | `any` ≈ Python's `Any` |
| `[]string` | `list[str]` | Slices are Go's lists |
| `:=` | `=` (with inferred type) | First-time assignment; Go infers the type automatically |
| `nil` | `None` | |
| `fmt.Errorf("msg: %w", err)` | `raise ValueError("msg") from err` | Wraps an error with context, like chained exceptions |
| returning `(value, error)` | returning value or raising exception | Go functions return both a value and an error; callers must check the error |
| `ctx context.Context` | cancellation/timeout token | Thread through function calls; mostly ignore for understanding the logic |
| `var _ Database = (*DB)(nil)` | `assert issubclass(DB, Database)` | A compile-time check that `DB` satisfies the `Database` interface |

**A note on `Record`:** Throughout the codebase, `db.Record` is defined as `map[string]any` — a Python `dict[str, Any]`. Every row in the in-memory DB is one of these.

---

## 2. Initial Scaffold State

The project was scaffolded with working infrastructure (DB engine, eBPF agent, data file) but the domain logic was left as stubs — empty functions with `TODO` comments.

### What was already present

| File | State |
|---|---|
| `db/memory.go` | Fully working in-memory DB engine (not covered here) |
| `db/interface.go` | Partial `Database` interface (missing conflict methods) |
| `ebpf_agent/ebpf.go` | Reads `data/ebpf_spans.json` and returns `[]RawSpan` |
| `data/ebpf_spans.json` | 1600+ real eBPF span records |

### What was a stub

**`main.go`**
```go
func createDBSchema() {}  // empty — no tables defined
```
Also: `processor.New()` and no API wiring at all.

**`backend/processor/processor.go`**
```go
type SpanProcessor struct{}  // no fields

func New() *SpanProcessor { return &SpanProcessor{} }  // no DB injected

func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error {
    return nil  // does nothing
}
```

**`backend/api/api.go`**
```go
type APIBackend struct{}  // no fields

func New() *APIBackend { return &APIBackend{} }  // no DB injected

// TODO: Implement GetCatalog
// TODO: Implement GetConnections
```

**`models/models.go`** — only `RawSpan` and `RawSpansFile` existed. No domain types.

---

## 3. PR #1 — Domain Types + DB Schema

**Branch:** `feat/db-schema-and-domain-types`
**Commits:** `c90c14d`, `3562408`, `b5b8177`

This PR established the data model (types, structs) and wired up the database schema.

### `models/models.go` — New Types

**Typed string constants (≈ Python `enum.Enum`)**

Go uses typed string constants where Python would use an enum. Example:

```go
type AppItemType string

const (
    AppItemTypeInternet AppItemType = "INTERNET"
    AppItemTypeService  AppItemType = "SERVICE"
    AppItemTypeDatabase AppItemType = "DATABASE"
    AppItemTypeQueue    AppItemType = "QUEUE"
)
```

Python equivalent:
```python
class AppItemType(str, Enum):
    INTERNET = "INTERNET"
    SERVICE = "SERVICE"
    DATABASE = "DATABASE"
    QUEUE = "QUEUE"
```

The same pattern was used for:
- `ComponentType`: `ENDPOINT`, `QUEUE`, `QUERY`
- `PIIType`: `EMAIL`, `CREDIT_CARD`, `SSN`, `PHONE`, `IP_ADDRESS`

**New structs (≈ Python dataclasses)**

```go
type Component struct {
    ID            string        // unique identifier
    AppItemName   string        // foreign key → app_items.name
    ComponentType ComponentType // ENDPOINT / QUEUE / QUERY
    Value         string        // the HTTP path, queue topic, or SQL query text
    PIIs          []PIIType     // list of detected PII types
}

type AppItem struct {
    Name       string
    Type       AppItemType
    Components []Component
}

type Connection struct {
    Source      string
    Destination string
    Components  []Component
}

type Catalog struct {
    AppItems []AppItem
}
```

The `json:"..."` tags on fields (omitted above for clarity) control how the struct serializes to JSON — like `pydantic.Field(alias="...")` or `@dataclass` with a custom encoder.

### `main.go` — `createDBSchema(ctx, database)`

Signature (Go → Python):
```go
func createDBSchema(ctx context.Context, database *db.DB)
// def create_db_schema(ctx, database: DB) -> None:
```

This function creates 4 in-memory tables. Each table is like declaring a SQL schema:

**`app_items`** — One row per unique service/database/queue/internet endpoint seen in spans.
```go
db.TableSchema{
    Name:       "app_items",
    Fields:     []db.Field{{Name: "name", ...}, {Name: "type", ...}},
    PrimaryKey: "name",   // unique identifier; upserts match on this
    Indexes:    []string{"type"},  // enables fast lookup by type
}
```

**`components`** — One row per unique component (endpoint/queue/query) belonging to a destination app item.
Primary key: `id` (constructed as `"COMPONENT_TYPE:destination:value"`).
Indexed by `app_item_name` (for lookups) and `component_type`.

**`component_piis`** — One row per detected PII type per component.
Primary key: `id` = `"componentID:PII_TYPE"`. Indexed by `component_id` and `pii_type`.

**`connections`** — One row per unique source→destination pair.
Primary key: `id` = `"source:destination"`. The `component_ids` field is of type `FieldTypeJSON` (stores a `[]string`).

### `processor.go` + `api.go` — Constructor Changes

The constructors were updated to accept a `*db.DB` for dependency injection:

```go
// Before
func New() *SpanProcessor { return &SpanProcessor{} }

// After (PR #1)
func New(database *db.DB) *SpanProcessor {
    return &SpanProcessor{db: database}
}
```

Python equivalent:
```python
# Before
def __init__(self): pass

# After
def __init__(self, database: DB):
    self.db = database
```

The struct fields were also updated:
```go
type SpanProcessor struct { db *db.DB }   // processor
type APIBackend struct    { db *db.DB }   // api
```

---

## 4. PR #2 — Processor

**Branch:** `feat/processor-process-spans`
**Commit:** `205d6f7`

This PR implements the full span-processing pipeline in `backend/processor/processor.go`.

### Module-level PII patterns

```go
var piiPatterns = map[models.PIIType]*regexp.Regexp{
    models.PIITypeEmail:      regexp.MustCompile(`\b[\w.%+\-]+@[\w.\-]+\.[a-zA-Z]{2,}\b`),
    models.PIITypeCreditCard: regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`),
    models.PIITypeSSN:        regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
    models.PIITypePhone:      regexp.MustCompile(`\b(\+\d{1,3}[- ]?)?\(?\d{3}\)?[- ]?\d{3}[- ]?\d{4}\b`),
    models.PIITypeIPAddress:  regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
}
```

Python equivalent: a module-level `dict[PIIType, re.Pattern]` compiled once at import time.

---

### `New(database *db.DB) *SpanProcessor`

```go
// Go:   func New(database *db.DB) *SpanProcessor
// Python: def __init__(self, database: DB) -> None:
```

Stores the DB instance in the struct. Standard dependency injection — the DB is passed in rather than created inside.

---

### `Process(ctx, rawSpans []RawSpan) error`

```go
// Go:   func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error
// Python: def process(self, ctx, raw_spans: list[RawSpan]) -> None:  # raises on error
```

Public entry point. Iterates over every span and delegates to `processSpan`. If any span fails, the error is returned immediately (fail-fast).

```go
for _, span := range rawSpans {
    if err := p.processSpan(ctx, span); err != nil {
        return err
    }
}
```

Python equivalent: `for span in raw_spans: self._process_span(ctx, span)`.

---

### `processSpan(ctx, span RawSpan) error`

The core function. Extracts `spanType`, `source`, and `destination` from `span.Attributes` (a `map[string]string` ≈ `dict[str, str]`), then executes 5 steps:

**Step 1 — Upsert source app item**

Determines source type via `sourceAppItemType(source)` and writes to the `app_items` table. "Upsert" = insert if not exists, update if exists (like SQL `INSERT ... ON CONFLICT DO UPDATE`).

**Step 2 — Upsert destination app item**

Determines destination type via `destinationAppItemType(spanType)` and upserts.

**Step 3 — Build and upsert component**

Calls `componentInfo(spanType, attrs)` to get the component type and value. Constructs the component ID as `"TYPE:destination:value"` (e.g., `"ENDPOINT:backend:/users"`), then upserts into `components`.

**Step 4 — Detect and store PIIs**

Calls `piiFieldsForSpanType(spanType, attrs)` to get the relevant text fields, then `detectPIIs(text)` on each. Uses a `map[PIIType]struct{}` (≈ Python `set`) to deduplicate. Each unique PII type is inserted with `InsertOnConflict(...DoNothing)` — idempotent.

**Step 5 — Upsert connection with component merge**

Upserts a connection record. If the connection already exists, the `component_ids` list is merged (union, no duplicates) using a custom `MergeFunc`:
```go
"component_ids": func(existing, new any) any {
    // deduplicated union of existing IDs + new IDs
}
```
Python equivalent: `existing_ids = list(set(existing_ids) | set(new_ids))`.

---

### `sourceAppItemType(source string) AppItemType`

```go
// def source_app_item_type(source: str) -> AppItemType:
func sourceAppItemType(source string) models.AppItemType {
    if source == "internet" {
        return models.AppItemTypeInternet
    }
    return models.AppItemTypeService
}
```

Simple rule: only the literal string `"internet"` maps to `INTERNET`; everything else is a `SERVICE`.

---

### `destinationAppItemType(spanType string) AppItemType`

```go
// def destination_app_item_type(span_type: str) -> AppItemType:
func destinationAppItemType(spanType string) models.AppItemType {
    switch spanType {
    case "QUERY":   return models.AppItemTypeDatabase
    case "MESSAGE": return models.AppItemTypeQueue
    default:        return models.AppItemTypeService
    }
}
```

The destination's type is inferred from the span type:
- SQL query span → destination is a `DATABASE`
- Message/queue span → destination is a `QUEUE`
- Anything else (network/HTTP) → destination is a `SERVICE`

---

### `componentInfo(spanType string, attrs map[string]string) (ComponentType, string)`

```go
// def component_info(span_type: str, attrs: dict[str, str]) -> tuple[ComponentType, str]:
func componentInfo(spanType string, attrs map[string]string) (models.ComponentType, string) {
    switch spanType {
    case "QUERY":   return models.ComponentTypeQuery,    attrs["ebpf.db.query"]
    case "MESSAGE": return models.ComponentTypeQueue,    attrs["ebpf.queue.topic"]
    default:        return models.ComponentTypeEndpoint, attrs["ebpf.http.path"]
    }
}
```

Returns two values (a Go tuple): the component type and the component's identifying value.

---

### `piiFieldsForSpanType(spanType string, attrs map[string]string) []string`

```go
// def pii_fields_for_span_type(span_type: str, attrs: dict[str, str]) -> list[str]:
func piiFieldsForSpanType(spanType string, attrs map[string]string) []string {
    switch spanType {
    case "NETWORK": return []string{attrs["ebpf.http.req_body"], attrs["ebpf.http.resp_body"]}
    case "QUERY":   return []string{attrs["ebpf.db.query.values"]}
    case "MESSAGE": return []string{attrs["ebpf.queue.payload"]}
    default:        return nil  // nil ≈ None / empty list
    }
}
```

Returns the text fields that may contain PII for a given span type. These are then scanned by regex.

---

### `detectPIIs(text string) []PIIType`

```go
// def detect_piis(text: str) -> list[PIIType]:
func detectPIIs(text string) []models.PIIType {
    var found []models.PIIType
    for piiType, pattern := range piiPatterns {
        if pattern.MatchString(text) {
            found = append(found, piiType)
        }
    }
    return found
}
```

Scans `text` against all five PII regex patterns. Returns only the types that matched.

---

## 5. PR #3 — API

**Branch:** `feat/api-catalog-and-connections`
**Commit:** `26e36df`

This PR implements `GetCatalog` and `GetConnections` in `backend/api/api.go`, along with response structs and tests.

### Response Structs

These are the JSON-serializable shapes returned by the API:

**`CatalogComponentResponse`** — A component as shown in the catalog (includes PIIs):
```go
type CatalogComponentResponse struct {
    ComponentType models.ComponentType `json:"component_type"`
    Path          string               `json:"path,omitempty"`   // only for ENDPOINT
    Topic         string               `json:"topic,omitempty"`  // only for QUEUE
    Query         string               `json:"query,omitempty"`  // only for QUERY
    PIIs          []models.PIIType     `json:"piis"`
}
```
The `omitempty` tag means: don't include the field in JSON if it's empty (like `exclude_none=True` in Pydantic).

**`AppItemResponse`** — One app item with its components:
```go
type AppItemResponse struct {
    Name       string
    Type       models.AppItemType
    Components []CatalogComponentResponse
}
```

**`CatalogResponse`** — The full catalog, keyed by app item name:
```go
type CatalogResponse struct {
    AppItems map[string]AppItemResponse `json:"app_items"`
    // Python: dict[str, AppItemResponse]
}
```

**`ConnectionComponentResponse`** — A component as shown in a connection (no PIIs):
```go
type ConnectionComponentResponse struct {
    ComponentType models.ComponentType
    Path  string
    Topic string
    Query string
}
```

**`ConnectionResponse`** — One connection:
```go
type ConnectionResponse struct {
    Source      string
    Destination string
    Components  []ConnectionComponentResponse
}
```

---

### `New(database *db.DB) *APIBackend`

```go
// def __init__(self, database: DB) -> None:
func New(database *db.DB) *APIBackend {
    return &APIBackend{db: database}
}
```

Standard constructor — stores the DB.

---

### `GetCatalog(ctx) (*CatalogResponse, error)`

```go
// def get_catalog(self, ctx) -> CatalogResponse:  # raises on error
func (a *APIBackend) GetCatalog(ctx context.Context) (*CatalogResponse, error)
```

**Algorithm — 3 fetches + in-memory grouping:**

1. `a.db.All(ctx, "app_items")` — fetch all app item records
2. `a.db.All(ctx, "components")` — fetch all component records
3. `a.db.All(ctx, "component_piis")` — fetch all PII records

Then group in memory:
- Build `piisByComponent: map[string][]PIIType` (≈ `defaultdict(list)`) by iterating `component_piis` and keying by `component_id`
- Build `componentsByAppItem: map[string][]Record` by iterating `components` and keying by `app_item_name`

Finally, assemble the response: for each app item, look up its components, look up each component's PIIs, call `buildCatalogComponent(rec, piis)`, collect into `AppItemResponse`.

---

### `GetConnections(ctx) ([]ConnectionResponse, error)`

```go
// def get_connections(self, ctx) -> list[ConnectionResponse]:  # raises on error
func (a *APIBackend) GetConnections(ctx context.Context) ([]ConnectionResponse, error)
```

**Algorithm:**

1. Fetch all connection records with `a.db.All(ctx, "connections")`
2. For each connection, read `component_ids` (a `[]string`)
3. For each component ID, call `a.db.Get(ctx, "components", compID)` to fetch the component record
4. Call `buildConnectionComponent(rec)` and collect
5. Return `[]ConnectionResponse`

> **Note:** At this point in the project, this did N individual `Get` calls per connection — one per component ID. This was the N+1 problem fixed in PR #4.

---

### Helper: `buildCatalogComponent(rec db.Record, piis []PIIType) CatalogComponentResponse`

Constructs a `CatalogComponentResponse` from a raw DB record + PII list. Reads `component_type` and `value` from the record, then assigns to the right field (`Path`, `Topic`, or `Query`) based on the type.

---

### Helper: `buildConnectionComponent(rec db.Record) ConnectionComponentResponse`

Same as above but produces a `ConnectionComponentResponse` (no PIIs field).

---

### Helper: `str(v any) string`

```go
func str(v any) string {
    s, _ := v.(string)
    return s
}
```

Python equivalent: `str(v) if isinstance(v, str) else ""`. Safely type-asserts a `any` value to `string`, returning `""` on failure. Used everywhere a record field is read.

---

## 6. PR #4 — N+1 Fix in `GetCatalog`

**Branch:** `perf/fix-getcatalog-n-plus-1`
**Commit:** `9aa9205`

### What is N+1?

In ORMs (Django ORM, SQLAlchemy) the N+1 problem is: you query for N objects, then issue 1 more query *per object* to fetch a related record. Total: N+1 queries. For 1000 app items, that's 1001 DB calls.

**Before this PR**, `GetCatalog` used a join:
```go
// For each app_item, fetch its components individually:
a.db.Join("app_items", "components")
    .On("name", "app_item_name")
    .Execute(ctx)
// Then for each component, fetch its PIIs individually:
a.db.Select(ctx, "component_piis").Where("component_id", compID).Execute(ctx)
```
If there were 50 app items with 200 components total, this was ~251 queries.

**After this PR**, `GetCatalog` uses 3 bulk fetches + in-memory grouping:
```
1. All(app_items)      → 1 query
2. All(components)     → 1 query
3. All(component_piis) → 1 query
                   Total: always 3 queries
```

The in-memory grouping (building `piisByComponent` and `componentsByAppItem` dicts) replaces the per-row DB lookups entirely. This is the standard fix for N+1 in any language.

> `GetConnections` was **not** fixed at this point — it still calls `db.Get` per component ID.

---

## 7. PR #5 — SOLID Refactor + Tests

**Branch:** `refactor/solid-dip-and-tests`
**Commit:** `340c675`

This PR applied the Dependency Inversion Principle (DIP), extended the `Database` interface, and added a comprehensive test suite.

### `db/interface.go` — Interface Extended

**Before (initial scaffold):** The `Database` interface was missing:
```go
InsertOnConflict(ctx, tableName, record, opts ConflictOptions) error
InsertBatchOnConflict(ctx, tableName, records []Record, opts ConflictOptions) error
```

Without these methods in the interface, any code that depended on the `Database` interface couldn't call them — you'd have to hold a reference to the concrete `*db.DB` type. That breaks the abstraction.

**After:** Both methods were added to the interface. The compile-time assertion at the bottom:
```go
var _ Database = (*DB)(nil)
// Python: assert issubclass(DB, Database)
```
ensures that the concrete `DB` struct implements *all* interface methods. If any method is missing, the program won't compile.

---

### `processor.go` + `api.go` — Applying DIP

**The Dependency Inversion Principle** says: high-level modules should depend on abstractions, not concretions.

**Before (PR #1):** Constructors took the concrete type:
```go
func New(database *db.DB) *SpanProcessor   // concrete DB class
// Python: def __init__(self, database: PostgresDB): ...
```

**After (PR #5):** Constructors take the interface:
```go
func New(database db.Database) *SpanProcessor   // interface / Protocol
// Python: def __init__(self, database: DatabaseProtocol): ...
```

Similarly the struct fields changed:
```go
// Before
type SpanProcessor struct { db *db.DB }

// After
type SpanProcessor struct { db db.Database }
```

**Why this matters:**
- Tests can pass a fake/mock DB (any struct that implements `Database`)
- The processor and API are no longer locked to the specific in-memory implementation
- Easier to swap the DB backend without touching any domain logic

---

### Error Wrapping

All DB call errors are now wrapped with context:
```go
return fmt.Errorf("upsert source app item %q: %w", source, err)
// Python: raise ValueError(f"upsert source app item {source!r}") from err
```

The `%w` verb wraps the original error so callers can inspect the chain with `errors.Is` / `errors.As` (like Python's `__cause__`). This makes debugging much easier — instead of a bare `"record not found"` you get `"upsert source app item \"internet\": record not found"`.

---

### `processor_test.go` — 19 Tests

Go test conventions (≈ pytest):
- Test functions are named `TestXxx(t *testing.T)` — Go's test runner discovers them automatically
- `t.Fatalf("msg", ...)` ≈ `pytest.fail("msg")` — stops the current test immediately
- `t.Errorf("msg", ...)` ≈ `assert False, "msg"` — marks test as failed but continues

All 19 tests:

| Test | What it verifies |
|---|---|
| `TestProcess_EmptySpans` | Processing `nil` spans writes nothing to any table |
| `TestProcess_UnknownSpanType_DefaultsToEndpointAndService` | Empty span type → destination is SERVICE, component is ENDPOINT |
| `TestProcess_SourceAppItemType_Internet` | Source `"internet"` → app item type `INTERNET` |
| `TestProcess_SourceAppItemType_Service` | Any other source name → app item type `SERVICE` |
| `TestProcess_DestinationAppItemType_Database` | Span type `QUERY` → destination type `DATABASE` |
| `TestProcess_DestinationAppItemType_Queue` | Span type `MESSAGE` → destination type `QUEUE` |
| `TestProcess_DestinationAppItemType_Service` | Span type `NETWORK` → destination type `SERVICE` |
| `TestProcess_Component_Endpoint` | NETWORK span → creates ENDPOINT component with correct path and app_item_name |
| `TestProcess_Component_Queue` | MESSAGE span → creates QUEUE component with correct topic |
| `TestProcess_Component_Query` | QUERY span → creates QUERY component with correct SQL text |
| `TestProcess_PII_Email` | Email in HTTP response body → EMAIL PII on the ENDPOINT component |
| `TestProcess_PII_CreditCard` | Card number in HTTP request body → CREDIT_CARD PII |
| `TestProcess_PII_SSN` | SSN in DB query values → SSN PII on the QUERY component |
| `TestProcess_PII_Phone` | Phone in HTTP response body → PHONE PII |
| `TestProcess_PII_IPAddress` | IP address in queue payload → IP_ADDRESS PII on QUEUE component |
| `TestProcess_PII_NoFalsePositives` | Clean data with no PII patterns → 0 rows in component_piis |
| `TestProcess_Deduplication_Component` | Same span processed twice → only 1 component row |
| `TestProcess_Deduplication_Connection_MergesComponents` | Two different endpoints between same source/destination → 1 connection row with 2 component IDs |

---

### `api_test.go` — 6 Tests

| Test | What it verifies |
|---|---|
| `TestGetCatalog` | Full integration: processes 4 realistic spans, checks all 5 app items present with correct types, components, and PIIs (including CREDIT_CARD on `/checkout` and EMAIL+PHONE on `/users/user-789`); also asserts PIIs is never `nil` |
| `TestGetConnections` | Processes same 4 spans, checks all 4 connections present with correct component types (ENDPOINT, QUEUE, QUERY) |
| `TestGetCatalog_EmptyDB` | Empty DB → returns empty catalog without error |
| `TestGetConnections_EmptyDB` | Empty DB → returns empty slice without error |
| `TestGetCatalog_SSNAndIPAddress` | QUERY span with SSN in values → SSN PII on postgres; MESSAGE span with IP → IP_ADDRESS PII on kafka |
| `TestGetConnections_MultipleComponents` | Two different NETWORK spans between same pair → 1 connection with 2 components |

---

## 8. PR #6 — SRP / Coupling Refactor

**Branch:** `refactor/srp-coupling-fixes`
**Commit:** `90a6806`

This PR addressed four SRP violations and coupling issues that remained after the earlier refactors.

### Fix 1: `createDBSchema` — Completing DIP

**File:** `main.go`

PR #5 applied DIP to `processor.New` and `api.New` (both accept `db.Database`), but `createDBSchema` was left behind — it still accepted the concrete `*db.DB`. The function only calls `CreateTable`, which is part of the `Database` interface, so there was no reason to require the concrete type.

```go
// Before
func createDBSchema(ctx context.Context, database *db.DB)
// Python: def create_db_schema(ctx, database: PostgresDB) -> None:  # concrete class

// After
func createDBSchema(ctx context.Context, database db.Database)
// Python: def create_db_schema(ctx, database: DatabaseProtocol) -> None:  # interface
```

Now all three consumers of the database (`createDBSchema`, `processor.New`, `api.New`) depend on the abstraction, not the concrete type.

---

### Fix 2: Extract `mergeUniqueStrings` — Named & Testable

**File:** `backend/processor/processor.go`

The logic for merging two `[]string` slices with deduplication was buried inside an anonymous closure passed to `InsertOnConflict`. This made the logic invisible, untestable, and tied to the call site.

**Before:**
```go
MergeFuncs: map[string]db.MergeFunc{
    "component_ids": func(existing, new any) any {
        existingIDs, _ := existing.([]string)
        newIDs, _ := new.([]string)
        seen := make(map[string]struct{}, len(existingIDs))
        for _, id := range existingIDs {
            seen[id] = struct{}{}
        }
        for _, id := range newIDs {
            if _, ok := seen[id]; !ok {
                existingIDs = append(existingIDs, id)
            }
        }
        return existingIDs
    },
},
```

**After:**
```go
// Named package-level function — visible, testable, reusable
func mergeUniqueStrings(existing, incoming any) any {
    existingIDs, _ := existing.([]string)
    newIDs, _ := incoming.([]string)
    seen := make(map[string]struct{}, len(existingIDs))
    for _, id := range existingIDs {
        seen[id] = struct{}{}
    }
    for _, id := range newIDs {
        if _, ok := seen[id]; !ok {
            existingIDs = append(existingIDs, id)
        }
    }
    return existingIDs
}

// Call site is now a simple reference:
MergeFuncs: map[string]db.MergeFunc{
    "component_ids": mergeUniqueStrings,
},
```

Python equivalent: extracting an inline `lambda` into a named `def` at module level so it can be imported and tested independently.

---

### Fix 3: N+1 Fix in `GetConnections`

**File:** `backend/api/api.go`

PR #4 fixed the N+1 problem in `GetCatalog` (3 bulk `All()` calls + in-memory grouping), but `GetConnections` was left with the same pattern — calling `db.Get(ctx, "components", compID)` inside a loop, one query per component ID.

**Before (N+1):**
```go
for _, compID := range compIDs {
    compRec, err := a.db.Get(ctx, "components", compID)
    // 1 DB call per component ID
}
```

**After (bulk fetch + map lookup):**
```go
// 1 query for all components
allComps, err := a.db.All(ctx, "components")
compByID := make(map[string]db.Record, len(allComps))
for _, c := range allComps {
    compByID[str(c["id"])] = c
}

// Then inside the loop — O(1) map lookup, no DB call:
for _, compID := range compIDs {
    rec, ok := compByID[compID]
    if !ok {
        return nil, fmt.Errorf("component %q not found", compID)
    }
}
```

Python equivalent: replacing `for id in ids: db.get("components", id)` with `all_comps = {c["id"]: c for c in db.all("components")}` and then `all_comps[id]`.

Now `GetConnections` matches the same pattern as `GetCatalog`: bulk fetch once, look up in memory.

---

### Fix 4: DRY — `buildCatalogComponent` Delegates to `buildConnectionComponent`

**File:** `backend/api/api.go`

Both `buildCatalogComponent` and `buildConnectionComponent` contained an identical `switch ct` block that mapped a component type + value to the `Path`/`Topic`/`Query` fields. The only difference was that the catalog variant also received `piis`. Adding a new `ComponentType` would require updating both functions.

**Before (duplicated switch in both functions):**
```go
func buildCatalogComponent(rec db.Record, piis []models.PIIType) CatalogComponentResponse {
    ct := models.ComponentType(str(rec["component_type"]))
    value := str(rec["value"])
    comp := CatalogComponentResponse{ComponentType: ct, PIIs: piis}
    switch ct {
    case models.ComponentTypeEndpoint: comp.Path = value
    case models.ComponentTypeQueue:    comp.Topic = value
    case models.ComponentTypeQuery:    comp.Query = value
    }
    return comp
}

func buildConnectionComponent(rec db.Record) ConnectionComponentResponse {
    ct := models.ComponentType(str(rec["component_type"]))
    value := str(rec["value"])
    comp := ConnectionComponentResponse{ComponentType: ct}
    switch ct {                          // ← identical switch
    case models.ComponentTypeEndpoint: comp.Path = value
    case models.ComponentTypeQueue:    comp.Topic = value
    case models.ComponentTypeQuery:    comp.Query = value
    }
    return comp
}
```

**After (catalog delegates to connection):**
```go
func buildCatalogComponent(rec db.Record, piis []models.PIIType) CatalogComponentResponse {
    base := buildConnectionComponent(rec)  // reuse the switch
    return CatalogComponentResponse{
        ComponentType: base.ComponentType,
        Path:          base.Path,
        Topic:         base.Topic,
        Query:         base.Query,
        PIIs:          piis,
    }
}
```

The switch lives in exactly one place. Adding a new `ComponentType` requires one change, not two.

---

## 9. Summary Table

| PR | Files changed | Functions added / changed |
|---|---|---|
| **#1** Domain Types + DB Schema | `models/models.go`, `main.go`, `backend/processor/processor.go`, `backend/api/api.go` | Added `AppItemType`, `ComponentType`, `PIIType` constants; `Component`, `AppItem`, `Connection`, `Catalog` structs; implemented `createDBSchema()`; updated `New()` constructors to accept `*db.DB` |
| **#2** Processor | `backend/processor/processor.go` | Implemented `Process()`, `processSpan()`, `sourceAppItemType()`, `destinationAppItemType()`, `componentInfo()`, `piiFieldsForSpanType()`, `detectPIIs()`; added module-level `piiPatterns` map |
| **#3** API | `backend/api/api.go`, `backend/api/api_test.go` | Added response structs `CatalogComponentResponse`, `AppItemResponse`, `CatalogResponse`, `ConnectionComponentResponse`, `ConnectionResponse`; implemented `GetCatalog()`, `GetConnections()`; added helpers `buildCatalogComponent()`, `buildConnectionComponent()`, `str()`; added 6 API tests |
| **#4** N+1 Fix | `backend/api/api.go` | Rewrote `GetCatalog()` from join+per-row queries to 3 bulk `All()` calls + in-memory grouping |
| **#5** SOLID + Tests | `db/interface.go`, `backend/processor/processor.go`, `backend/api/api.go`, `backend/processor/processor_test.go` | Extended `Database` interface with `InsertOnConflict` + `InsertBatchOnConflict`; changed constructor signatures from `*db.DB` → `db.Database` (DIP); added error wrapping; added 19 processor tests |
| **#6** SRP / Coupling Refactor | `main.go`, `backend/processor/processor.go`, `backend/api/api.go` | Changed `createDBSchema` to accept `db.Database` (completing DIP); extracted `mergeUniqueStrings` from anonymous closure; fixed N+1 in `GetConnections` with bulk fetch + map lookup; `buildCatalogComponent` now delegates to `buildConnectionComponent` (DRY) |
