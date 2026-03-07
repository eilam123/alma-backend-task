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
	"github.com/alma/assignment/schema"
)

func main() {
	ctx := context.Background()

	database := db.New()
	if err := schema.CreateSchema(ctx, database); err != nil {
		fmt.Printf("Error creating schema: %v\n", err)
		os.Exit(1)
	}

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
