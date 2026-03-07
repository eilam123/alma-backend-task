package api_test

import (
	"context"
	"testing"

	backendapi "github.com/alma/assignment/backend/api"
	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
	"github.com/alma/assignment/schema"
)

func setupDB(ctx context.Context) *db.DB {
	database := db.New()
	_ = schema.CreateSchema(ctx, database)
	return database
}

var testSpans = []models.RawSpan{
	{
		Id: "span-001",
		Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "internet",
			"ebpf.destination":    "mysupermarket-service",
			"ebpf.http.path":      "/checkout",
			"ebpf.http.req_body":  `{"card": "4111-1111-1111-1111", "user": "alice"}`,
			"ebpf.http.resp_body": `{"order_id": "o1"}`,
		},
	},
	{
		Id: "span-002",
		Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "mysupermarket-service",
			"ebpf.destination":    "users-service",
			"ebpf.http.path":      "/users/user-789",
			"ebpf.http.req_body":  `{"user_id": "user-789"}`,
			"ebpf.http.resp_body": `{"email": "alice@example.com", "phone": "+1-555-123-4567"}`,
		},
	},
	{
		Id: "span-003",
		Attributes: map[string]string{
			"ebpf.span.type":     "MESSAGE",
			"ebpf.source":        "mysupermarket-service",
			"ebpf.destination":   "kafka",
			"ebpf.queue.topic":   "order-events",
			"ebpf.queue.payload": `{"order_id": "o1"}`,
		},
	},
	{
		Id: "span-004",
		Attributes: map[string]string{
			"ebpf.span.type":         "QUERY",
			"ebpf.source":            "users-service",
			"ebpf.destination":       "postgres-db",
			"ebpf.db.query":          "SELECT * FROM users WHERE id = $1",
			"ebpf.db.query.values":   `["user-789", "alice@example.com"]`,
		},
	},
}

func TestGetCatalog(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)
	catalog, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("GetCatalog failed: %v", err)
	}

	// All 5 app items present
	expectedNames := []string{"internet", "mysupermarket-service", "users-service", "kafka", "postgres-db"}
	for _, name := range expectedNames {
		if _, ok := catalog.AppItems[name]; !ok {
			t.Errorf("missing app item: %s", name)
		}
	}

	// Correct types
	typeChecks := map[string]models.AppItemType{
		"internet":              models.AppItemTypeInternet,
		"mysupermarket-service": models.AppItemTypeService,
		"users-service":         models.AppItemTypeService,
		"kafka":                 models.AppItemTypeQueue,
		"postgres-db":           models.AppItemTypeDatabase,
	}
	for name, expectedType := range typeChecks {
		item := catalog.AppItems[name]
		if item.Type != expectedType {
			t.Errorf("app item %q: expected type %q, got %q", name, expectedType, item.Type)
		}
	}

	// internet has no components
	internet := catalog.AppItems["internet"]
	if len(internet.Components) != 0 {
		t.Errorf("internet should have 0 components, got %d", len(internet.Components))
	}

	// mysupermarket-service has /checkout endpoint with CREDIT_CARD PII
	mss := catalog.AppItems["mysupermarket-service"]
	if len(mss.Components) != 1 {
		t.Fatalf("mysupermarket-service: expected 1 component, got %d", len(mss.Components))
	}
	checkoutComp := mss.Components[0]
	if checkoutComp.ComponentType != models.ComponentTypeEndpoint {
		t.Errorf("expected ENDPOINT, got %q", checkoutComp.ComponentType)
	}
	if checkoutComp.Path != "/checkout" {
		t.Errorf("expected path /checkout, got %q", checkoutComp.Path)
	}
	if !containsPII(checkoutComp.PIIs, models.PIITypeCreditCard) {
		t.Errorf("expected CREDIT_CARD PII on /checkout component")
	}

	// users-service has /users/user-789 with EMAIL and PHONE
	us := catalog.AppItems["users-service"]
	if len(us.Components) != 1 {
		t.Fatalf("users-service: expected 1 component, got %d", len(us.Components))
	}
	usersComp := us.Components[0]
	if usersComp.Path != "/users/user-789" {
		t.Errorf("expected path /users/user-789, got %q", usersComp.Path)
	}
	if !containsPII(usersComp.PIIs, models.PIITypeEmail) {
		t.Errorf("expected EMAIL PII on /users/user-789 component")
	}
	if !containsPII(usersComp.PIIs, models.PIITypePhone) {
		t.Errorf("expected PHONE PII on /users/user-789 component")
	}

	// PIIs field is never nil
	for name, item := range catalog.AppItems {
		for i, comp := range item.Components {
			if comp.PIIs == nil {
				t.Errorf("app item %q component %d: PIIs slice is nil, expected empty slice", name, i)
			}
		}
	}
}

func TestGetConnections(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)
	connections, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("GetConnections failed: %v", err)
	}

	// Expect 4 connections
	if len(connections) != 4 {
		t.Fatalf("expected 4 connections, got %d", len(connections))
	}

	// Build a map for easy lookup
	connMap := make(map[string]backendapi.ConnectionResponse)
	for _, c := range connections {
		key := c.Source + "->" + c.Destination
		connMap[key] = c
	}

	expectedConns := []string{
		"internet->mysupermarket-service",
		"mysupermarket-service->users-service",
		"mysupermarket-service->kafka",
		"users-service->postgres-db",
	}
	for _, key := range expectedConns {
		if _, ok := connMap[key]; !ok {
			t.Errorf("missing connection: %s", key)
		}
	}

	// Each connection has at least one component
	for _, c := range connections {
		if len(c.Components) == 0 {
			t.Errorf("connection %s->%s has no components", c.Source, c.Destination)
		}
	}

	// internet->mysupermarket-service component is ENDPOINT /checkout
	conn := connMap["internet->mysupermarket-service"]
	if len(conn.Components) != 1 {
		t.Fatalf("internet->mysupermarket-service: expected 1 component, got %d", len(conn.Components))
	}
	if conn.Components[0].ComponentType != models.ComponentTypeEndpoint || conn.Components[0].Path != "/checkout" {
		t.Errorf("unexpected component: %+v", conn.Components[0])
	}

	// mysupermarket-service->kafka component is QUEUE order-events
	kafkaConn := connMap["mysupermarket-service->kafka"]
	if len(kafkaConn.Components) != 1 {
		t.Fatalf("mysupermarket-service->kafka: expected 1 component, got %d", len(kafkaConn.Components))
	}
	if kafkaConn.Components[0].ComponentType != models.ComponentTypeQueue || kafkaConn.Components[0].Topic != "order-events" {
		t.Errorf("unexpected kafka component: %+v", kafkaConn.Components[0])
	}

	// users-service->postgres-db component is QUERY
	pgConn := connMap["users-service->postgres-db"]
	if len(pgConn.Components) != 1 {
		t.Fatalf("users-service->postgres-db: expected 1 component, got %d", len(pgConn.Components))
	}
	if pgConn.Components[0].ComponentType != models.ComponentTypeQuery {
		t.Errorf("expected QUERY component, got %q", pgConn.Components[0].ComponentType)
	}
}

func containsPII(piis []models.PIIType, target models.PIIType) bool {
	for _, p := range piis {
		if p == target {
			return true
		}
	}
	return false
}

func TestGetCatalog_EmptyDB(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	api := backendapi.New(database)

	catalog, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("GetCatalog on empty DB failed: %v", err)
	}
	if len(catalog.AppItems) != 0 {
		t.Errorf("expected empty catalog, got %d items", len(catalog.AppItems))
	}
}

func TestGetConnections_EmptyDB(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	api := backendapi.New(database)

	connections, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("GetConnections on empty DB failed: %v", err)
	}
	if len(connections) != 0 {
		t.Errorf("expected empty connections, got %d", len(connections))
	}
}

func TestGetCatalog_SSNAndIPAddress(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)

	spans := []models.RawSpan{
		{
			Id: "span-ssn",
			Attributes: map[string]string{
				"ebpf.span.type":       "QUERY",
				"ebpf.source":          "app",
				"ebpf.destination":     "postgres",
				"ebpf.db.query":        "INSERT INTO users VALUES ($1, $2)",
				"ebpf.db.query.values": `["123-45-6789"]`,
			},
		},
		{
			Id: "span-ip",
			Attributes: map[string]string{
				"ebpf.span.type":     "MESSAGE",
				"ebpf.source":        "app",
				"ebpf.destination":   "kafka",
				"ebpf.queue.topic":   "audit",
				"ebpf.queue.payload": `{"client_ip": "10.0.0.1"}`,
			},
		},
	}
	if err := p.Process(ctx, spans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)
	catalog, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("GetCatalog failed: %v", err)
	}

	pg := catalog.AppItems["postgres"]
	if len(pg.Components) != 1 {
		t.Fatalf("expected 1 component for postgres, got %d", len(pg.Components))
	}
	if !containsPII(pg.Components[0].PIIs, models.PIITypeSSN) {
		t.Errorf("expected SSN PII on postgres component, got %v", pg.Components[0].PIIs)
	}

	kaf := catalog.AppItems["kafka"]
	if len(kaf.Components) != 1 {
		t.Fatalf("expected 1 component for kafka, got %d", len(kaf.Components))
	}
	if !containsPII(kaf.Components[0].PIIs, models.PIITypeIPAddress) {
		t.Errorf("expected IP_ADDRESS PII on kafka component, got %v", kaf.Components[0].PIIs)
	}
}

func TestGetConnections_MultipleComponents(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)

	spans := []models.RawSpan{
		{
			Id: "span-mc1",
			Attributes: map[string]string{
				"ebpf.span.type":   "NETWORK",
				"ebpf.source":      "frontend",
				"ebpf.destination": "backend",
				"ebpf.http.path":   "/users",
			},
		},
		{
			Id: "span-mc2",
			Attributes: map[string]string{
				"ebpf.span.type":   "NETWORK",
				"ebpf.source":      "frontend",
				"ebpf.destination": "backend",
				"ebpf.http.path":   "/orders",
			},
		},
	}
	if err := p.Process(ctx, spans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)
	connections, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("GetConnections failed: %v", err)
	}
	if len(connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(connections))
	}
	if len(connections[0].Components) != 2 {
		t.Errorf("expected 2 components in connection, got %d", len(connections[0].Components))
	}
}
