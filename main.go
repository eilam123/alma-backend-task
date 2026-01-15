package main

import (
	"context"
	"fmt"
	"os"

	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/ebpf_agent"
)

func main() {
	ctx := context.Background()

	createDBSchema()

	ebpfAgent := ebpf_agent.NewEBPFAgent("data/ebpf_spans.json")
	spans, err := ebpfAgent.GetSpans()
	if err != nil {
		fmt.Printf("Error loading spans: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Loaded %d spans\n", len(spans))

	p := processor.New()

	if err := p.Process(ctx, spans); err != nil {
		fmt.Printf("Error processing spans: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Processed spans successfully")
}

// TODO: Implement createDBSchema
func createDBSchema() {
}
