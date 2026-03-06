package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/alma/assignment/backend/api"
	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/db"
	"github.com/alma/assignment/ebpf_agent"
)

func main() {
	ctx := context.Background()

	database := db.New()
	createDBSchema(ctx, database)

	ebpfAgent := ebpf_agent.NewEBPFAgent("data/ebpf_spans.json")
	spans, err := ebpfAgent.GetSpans()
	if err != nil {
		fmt.Printf("Error loading spans: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d spans\n", len(spans))

	p := processor.New(database)

	if err := p.Process(ctx, spans); err != nil {
		fmt.Printf("Error processing spans: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Processed spans successfully")

	apiBackend := api.New(database)

	catalog, err := apiBackend.GetCatalog(ctx)
	if err != nil {
		fmt.Printf("Error getting catalog: %v\n", err)
		os.Exit(1)
	}
	catalogJSON, _ := json.MarshalIndent(catalog, "", "  ")
	fmt.Println("\n=== Catalog ===")
	fmt.Println(string(catalogJSON))

	connections, err := apiBackend.GetConnections(ctx)
	if err != nil {
		fmt.Printf("Error getting connections: %v\n", err)
		os.Exit(1)
	}
	connectionsJSON, _ := json.MarshalIndent(connections, "", "  ")
	fmt.Println("\n=== Connections ===")
	fmt.Println(string(connectionsJSON))
}

func createDBSchema(ctx context.Context, database db.Database) {
	database.CreateTable(ctx, db.TableSchema{
		Name: "app_items",
		Fields: []db.Field{
			{Name: "name", Type: db.FieldTypeString},
			{Name: "type", Type: db.FieldTypeString},
		},
		PrimaryKey: "name",
		Indexes:    []string{"type"},
	})

	database.CreateTable(ctx, db.TableSchema{
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

	database.CreateTable(ctx, db.TableSchema{
		Name: "component_piis",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "component_id", Type: db.FieldTypeString},
			{Name: "pii_type", Type: db.FieldTypeString},
		},
		PrimaryKey: "id",
		Indexes:    []string{"component_id", "pii_type"},
	})

	database.CreateTable(ctx, db.TableSchema{
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
}
