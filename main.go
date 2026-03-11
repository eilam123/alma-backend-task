package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alma/assignment/backend/api"
	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/config"
	"github.com/alma/assignment/db"
	"github.com/alma/assignment/ebpf_agent"
	"github.com/alma/assignment/metrics"
	"github.com/alma/assignment/schema"
	"github.com/alma/assignment/server"
)

func main() {
	cfg := config.LoadConfig()

	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	}))
	slog.SetDefault(logger)

	metrics.Register()

	ctx := context.Background()

	database := db.New()
	if err := schema.CreateSchema(ctx, database); err != nil {
		logger.Error("failed to create schema", "error", err)
		os.Exit(1)
	}

	ebpfAgent := ebpf_agent.NewEBPFAgent(cfg.DataPath)
	spans, err := ebpfAgent.GetSpans()
	if err != nil {
		logger.Error("failed to load spans", "error", err)
		os.Exit(1)
	}

	processorOpts := []processor.Option{processor.WithLogger(logger)}
	if cfg.BatchFlushThreshold > 0 {
		processorOpts = append(processorOpts, processor.WithBatchFlushThreshold(cfg.BatchFlushThreshold))
	}
	p := processor.New(database, processorOpts...)
	processStart := time.Now()
	if err := p.Process(ctx, spans); err != nil {
		logger.Error("failed to process spans", "error", err)
		os.Exit(1)
	}

	apiBackend := api.New(database, api.WithAPILogger(logger))

	catalogStart := time.Now()
	catalog, err := apiBackend.GetCatalog(ctx)
	if err != nil {
		logger.Error("failed to get catalog", "error", err)
		os.Exit(1)
	}
	catalogJSON, _ := json.MarshalIndent(catalog, "", "  ")
	fmt.Println("\n=== Catalog ===")
	fmt.Println(string(catalogJSON))

	connectionsStart := time.Now()
	connections, err := apiBackend.GetConnections(ctx)
	if err != nil {
		logger.Error("failed to get connections", "error", err)
		os.Exit(1)
	}
	connectionsJSON, _ := json.MarshalIndent(connections, "", "  ")
	fmt.Println("\n=== Connections ===")
	fmt.Println(string(connectionsJSON))

	fmt.Printf("\nThis is where we expect to improve the latency for the processing and API calls.")
	fmt.Printf("\n--------------------------------")
	fmt.Printf("\n[perf] Process: %s\n", time.Since(processStart))
	fmt.Printf("[perf] GetCatalog: %s\n", time.Since(catalogStart))
	fmt.Printf("[perf] GetConnections: %s\n", time.Since(connectionsStart))
	fmt.Printf("\n--------------------------------\n")

	// Start HTTP servers
	apiAddr := fmt.Sprintf(":%d", cfg.HTTPPort)
	metricsAddr := fmt.Sprintf(":%d", cfg.MetricsPort)

	apiServer := server.NewAPIServer(apiAddr, apiBackend, logger)
	metricsServer := server.NewMetricsServer(metricsAddr, logger)

	go func() {
		if err := metricsServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	go func() {
		if err := apiServer.Start(); err != nil && err != http.ErrServerClosed {
			logger.Error("API server error", "error", err)
		}
	}()

	logger.Info("servers started", "api_port", cfg.HTTPPort, "metrics_port", cfg.MetricsPort)

	// Graceful shutdown on SIGINT/SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down servers")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := apiServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("API server shutdown error", "error", err)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("metrics server shutdown error", "error", err)
	}
	logger.Info("servers stopped")
}
