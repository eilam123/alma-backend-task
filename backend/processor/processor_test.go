package processor_test

import (
	"context"
	"testing"

	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

// setupTestDB creates an in-memory DB with the same schema used by main.go.
func setupTestDB(ctx context.Context) *db.DB {
	database := db.New()
	_ = database.CreateTable(ctx, db.TableSchema{
		Name: "app_items",
		Fields: []db.Field{
			{Name: "name", Type: db.FieldTypeString},
			{Name: "type", Type: db.FieldTypeString},
		},
		PrimaryKey: "name",
		Indexes:    []string{"type"},
	})
	_ = database.CreateTable(ctx, db.TableSchema{
		Name: "components",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "app_item_name", Type: db.FieldTypeString},
			{Name: "component_type", Type: db.FieldTypeString},
			{Name: "value", Type: db.FieldTypeString},
		},
		PrimaryKey: "id",
		Indexes:    []string{"app_item_name", "component_type"},
	})
	_ = database.CreateTable(ctx, db.TableSchema{
		Name: "component_piis",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "component_id", Type: db.FieldTypeString},
			{Name: "pii_type", Type: db.FieldTypeString},
		},
		PrimaryKey: "id",
		Indexes:    []string{"component_id", "pii_type"},
	})
	_ = database.CreateTable(ctx, db.TableSchema{
		Name: "connections",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "source", Type: db.FieldTypeString},
			{Name: "destination", Type: db.FieldTypeString},
			{Name: "component_ids", Type: db.FieldTypeJSON},
		},
		PrimaryKey: "id",
		Indexes:    []string{"source", "destination"},
	})
	return database
}

func strVal(rec db.Record, key string) string {
	s, _ := rec[key].(string)
	return s
}

// --- Edge cases ---

func TestProcess_EmptySpans(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	if err := p.Process(ctx, nil); err != nil {
		t.Fatalf("Process(nil) returned error: %v", err)
	}

	for _, table := range []string{"app_items", "components", "component_piis", "connections"} {
		n, err := database.Count(ctx, table)
		if err != nil {
			t.Fatalf("Count(%s): %v", table, err)
		}
		if n != 0 {
			t.Errorf("expected 0 records in %s, got %d", table, n)
		}
	}
}

func TestProcess_UnknownSpanType_DefaultsToEndpointAndService(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-unknown",
		Attributes: map[string]string{
			"ebpf.span.type":  "",
			"ebpf.source":     "svc-a",
			"ebpf.destination": "svc-b",
			"ebpf.http.path":  "/some/path",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	destRec, err := database.Get(ctx, "app_items", "svc-b")
	if err != nil {
		t.Fatalf("Get app_items svc-b: %v", err)
	}
	if strVal(destRec, "type") != string(models.AppItemTypeService) {
		t.Errorf("expected destination type SERVICE, got %q", strVal(destRec, "type"))
	}

	recs, err := database.All(ctx, "components")
	if err != nil {
		t.Fatalf("All components: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 component, got %d", len(recs))
	}
	if strVal(recs[0], "component_type") != string(models.ComponentTypeEndpoint) {
		t.Errorf("expected component type ENDPOINT, got %q", strVal(recs[0], "component_type"))
	}
}

// --- App item type resolution ---

func TestProcess_SourceAppItemType_Internet(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-internet",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "internet",
			"ebpf.destination": "svc-x",
			"ebpf.http.path":   "/api",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	rec, err := database.Get(ctx, "app_items", "internet")
	if err != nil {
		t.Fatalf("Get app_items internet: %v", err)
	}
	if strVal(rec, "type") != string(models.AppItemTypeInternet) {
		t.Errorf("expected INTERNET, got %q", strVal(rec, "type"))
	}
}

func TestProcess_SourceAppItemType_Service(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-svc",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "my-service",
			"ebpf.destination": "other-service",
			"ebpf.http.path":   "/health",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	rec, err := database.Get(ctx, "app_items", "my-service")
	if err != nil {
		t.Fatalf("Get app_items my-service: %v", err)
	}
	if strVal(rec, "type") != string(models.AppItemTypeService) {
		t.Errorf("expected SERVICE, got %q", strVal(rec, "type"))
	}
}

func TestProcess_DestinationAppItemType_Database(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-db",
		Attributes: map[string]string{
			"ebpf.span.type":   "QUERY",
			"ebpf.source":      "app",
			"ebpf.destination": "postgres",
			"ebpf.db.query":    "SELECT 1",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	rec, err := database.Get(ctx, "app_items", "postgres")
	if err != nil {
		t.Fatalf("Get app_items postgres: %v", err)
	}
	if strVal(rec, "type") != string(models.AppItemTypeDatabase) {
		t.Errorf("expected DATABASE, got %q", strVal(rec, "type"))
	}
}

func TestProcess_DestinationAppItemType_Queue(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-queue",
		Attributes: map[string]string{
			"ebpf.span.type":   "MESSAGE",
			"ebpf.source":      "producer",
			"ebpf.destination": "kafka",
			"ebpf.queue.topic": "events",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	rec, err := database.Get(ctx, "app_items", "kafka")
	if err != nil {
		t.Fatalf("Get app_items kafka: %v", err)
	}
	if strVal(rec, "type") != string(models.AppItemTypeQueue) {
		t.Errorf("expected QUEUE, got %q", strVal(rec, "type"))
	}
}

func TestProcess_DestinationAppItemType_Service(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-svc-dest",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "a",
			"ebpf.destination": "b",
			"ebpf.http.path":   "/",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	rec, err := database.Get(ctx, "app_items", "b")
	if err != nil {
		t.Fatalf("Get app_items b: %v", err)
	}
	if strVal(rec, "type") != string(models.AppItemTypeService) {
		t.Errorf("expected SERVICE, got %q", strVal(rec, "type"))
	}
}

// --- Component creation ---

func TestProcess_Component_Endpoint(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-ep",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "frontend",
			"ebpf.destination": "backend",
			"ebpf.http.path":   "/users",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	recs, err := database.All(ctx, "components")
	if err != nil {
		t.Fatalf("All components: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 component, got %d", len(recs))
	}
	if strVal(recs[0], "component_type") != string(models.ComponentTypeEndpoint) {
		t.Errorf("expected ENDPOINT, got %q", strVal(recs[0], "component_type"))
	}
	if strVal(recs[0], "value") != "/users" {
		t.Errorf("expected value /users, got %q", strVal(recs[0], "value"))
	}
	if strVal(recs[0], "app_item_name") != "backend" {
		t.Errorf("expected app_item_name backend, got %q", strVal(recs[0], "app_item_name"))
	}
}

func TestProcess_Component_Queue(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-queue-comp",
		Attributes: map[string]string{
			"ebpf.span.type":   "MESSAGE",
			"ebpf.source":      "producer",
			"ebpf.destination": "kafka",
			"ebpf.queue.topic": "order-events",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	recs, err := database.All(ctx, "components")
	if err != nil {
		t.Fatalf("All components: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 component, got %d", len(recs))
	}
	if strVal(recs[0], "component_type") != string(models.ComponentTypeQueue) {
		t.Errorf("expected QUEUE, got %q", strVal(recs[0], "component_type"))
	}
	if strVal(recs[0], "value") != "order-events" {
		t.Errorf("expected value order-events, got %q", strVal(recs[0], "value"))
	}
}

func TestProcess_Component_Query(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-query-comp",
		Attributes: map[string]string{
			"ebpf.span.type":   "QUERY",
			"ebpf.source":      "app",
			"ebpf.destination": "pg",
			"ebpf.db.query":    "SELECT * FROM users",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	recs, err := database.All(ctx, "components")
	if err != nil {
		t.Fatalf("All components: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("expected 1 component, got %d", len(recs))
	}
	if strVal(recs[0], "component_type") != string(models.ComponentTypeQuery) {
		t.Errorf("expected QUERY, got %q", strVal(recs[0], "component_type"))
	}
	if strVal(recs[0], "value") != "SELECT * FROM users" {
		t.Errorf("unexpected value: %q", strVal(recs[0], "value"))
	}
}

// --- PII detection ---

func piiTypesForComponent(t *testing.T, database *db.DB, componentID string) []models.PIIType {
	t.Helper()
	ctx := context.Background()
	recs, err := database.Select(ctx, "component_piis").Where("component_id", componentID).Execute(ctx)
	if err != nil {
		t.Fatalf("Select component_piis: %v", err)
	}
	var piis []models.PIIType
	for _, r := range recs {
		piis = append(piis, models.PIIType(strVal(r, "pii_type")))
	}
	return piis
}

func containsPIIType(piis []models.PIIType, target models.PIIType) bool {
	for _, p := range piis {
		if p == target {
			return true
		}
	}
	return false
}

func TestProcess_PII_Email(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-email",
		Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "svc",
			"ebpf.destination":    "dest",
			"ebpf.http.path":      "/profile",
			"ebpf.http.resp_body": `{"email": "user@example.com"}`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	componentID := "ENDPOINT:dest:/profile"
	piis := piiTypesForComponent(t, database, componentID)
	if !containsPIIType(piis, models.PIITypeEmail) {
		t.Errorf("expected EMAIL PII, got %v", piis)
	}
}

func TestProcess_PII_CreditCard(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-cc",
		Attributes: map[string]string{
			"ebpf.span.type":     "NETWORK",
			"ebpf.source":        "svc",
			"ebpf.destination":   "checkout",
			"ebpf.http.path":     "/pay",
			"ebpf.http.req_body": `{"card": "4111-1111-1111-1111"}`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	componentID := "ENDPOINT:checkout:/pay"
	piis := piiTypesForComponent(t, database, componentID)
	if !containsPIIType(piis, models.PIITypeCreditCard) {
		t.Errorf("expected CREDIT_CARD PII, got %v", piis)
	}
}

func TestProcess_PII_SSN(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-ssn",
		Attributes: map[string]string{
			"ebpf.span.type":       "QUERY",
			"ebpf.source":          "app",
			"ebpf.destination":     "pg",
			"ebpf.db.query":        "INSERT INTO users VALUES ($1)",
			"ebpf.db.query.values": `["123-45-6789"]`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	componentID := "QUERY:pg:INSERT INTO users VALUES ($1)"
	piis := piiTypesForComponent(t, database, componentID)
	if !containsPIIType(piis, models.PIITypeSSN) {
		t.Errorf("expected SSN PII, got %v", piis)
	}
}

func TestProcess_PII_Phone(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-phone",
		Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "svc",
			"ebpf.destination":    "users",
			"ebpf.http.path":      "/contact",
			"ebpf.http.resp_body": `{"phone": "+1-555-123-4567"}`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	componentID := "ENDPOINT:users:/contact"
	piis := piiTypesForComponent(t, database, componentID)
	if !containsPIIType(piis, models.PIITypePhone) {
		t.Errorf("expected PHONE PII, got %v", piis)
	}
}

func TestProcess_PII_IPAddress(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-ip",
		Attributes: map[string]string{
			"ebpf.span.type":     "MESSAGE",
			"ebpf.source":        "svc",
			"ebpf.destination":   "kafka",
			"ebpf.queue.topic":   "audit",
			"ebpf.queue.payload": `{"client_ip": "192.168.1.1"}`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	componentID := "QUEUE:kafka:audit"
	piis := piiTypesForComponent(t, database, componentID)
	if !containsPIIType(piis, models.PIITypeIPAddress) {
		t.Errorf("expected IP_ADDRESS PII, got %v", piis)
	}
}

func TestProcess_PII_NoFalsePositives(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-clean",
		Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "svc",
			"ebpf.destination":    "api",
			"ebpf.http.path":      "/status",
			"ebpf.http.req_body":  `{"action": "ping"}`,
			"ebpf.http.resp_body": `{"status": "ok"}`,
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	n, err := database.Count(ctx, "component_piis")
	if err != nil {
		t.Fatalf("Count component_piis: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 PIIs on clean data, got %d", n)
	}
}

// --- Deduplication / merging ---

func TestProcess_Deduplication_Component(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span := models.RawSpan{
		Id: "span-dup",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "a",
			"ebpf.destination": "b",
			"ebpf.http.path":   "/same",
		},
	}
	// Process the same span twice
	if err := p.Process(ctx, []models.RawSpan{span, span}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	n, err := database.Count(ctx, "components")
	if err != nil {
		t.Fatalf("Count components: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 component after deduplication, got %d", n)
	}
}

func TestProcess_Deduplication_Connection_MergesComponents(t *testing.T) {
	ctx := context.Background()
	database := setupTestDB(ctx)
	p := processor.New(database)

	span1 := models.RawSpan{
		Id: "span-conn1",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "frontend",
			"ebpf.destination": "backend",
			"ebpf.http.path":   "/users",
		},
	}
	span2 := models.RawSpan{
		Id: "span-conn2",
		Attributes: map[string]string{
			"ebpf.span.type":   "NETWORK",
			"ebpf.source":      "frontend",
			"ebpf.destination": "backend",
			"ebpf.http.path":   "/orders",
		},
	}
	if err := p.Process(ctx, []models.RawSpan{span1, span2}); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	connRecs, err := database.All(ctx, "connections")
	if err != nil {
		t.Fatalf("All connections: %v", err)
	}
	if len(connRecs) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(connRecs))
	}

	compIDs, _ := connRecs[0]["component_ids"].([]string)
	if len(compIDs) != 2 {
		t.Errorf("expected 2 component IDs in connection, got %d: %v", len(compIDs), compIDs)
	}
}
