package api_test

import (
	"context"
	"sync"
	"testing"

	backendapi "github.com/alma/assignment/backend/api"
	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/models"
)

func TestGetCatalog_CacheHit(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)

	// First call: cache miss, populates cache
	first, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("first GetCatalog failed: %v", err)
	}

	// Second call: cache hit, returns same pointer
	second, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("second GetCatalog failed: %v", err)
	}

	if first != second {
		t.Error("expected cache hit to return same pointer")
	}
}

func TestGetConnections_CacheHit(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)

	first, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("first GetConnections failed: %v", err)
	}

	second, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("second GetConnections failed: %v", err)
	}

	if len(first) != len(second) {
		t.Errorf("expected same length, got %d vs %d", len(first), len(second))
	}
}

func TestInvalidateCache_ForcesRefresh(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)

	api := backendapi.New(database)

	// Populate cache on empty DB
	catalog1, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("first GetCatalog failed: %v", err)
	}
	if len(catalog1.AppItems) != 0 {
		t.Fatalf("expected empty catalog, got %d items", len(catalog1.AppItems))
	}

	// Process spans into DB
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Without invalidation, cache still returns empty result
	catalog2, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("second GetCatalog failed: %v", err)
	}
	if len(catalog2.AppItems) != 0 {
		t.Error("expected stale cache to return empty catalog")
	}

	// Invalidate and verify fresh data is returned
	api.InvalidateCache()

	catalog3, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("third GetCatalog failed: %v", err)
	}
	if len(catalog3.AppItems) != 5 {
		t.Errorf("expected 5 app items after invalidation, got %d", len(catalog3.AppItems))
	}
}

func TestInvalidateCache_ConnectionsRefresh(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	api := backendapi.New(database)

	// Cache empty connections
	conns1, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("first GetConnections failed: %v", err)
	}
	if len(conns1) != 0 {
		t.Fatalf("expected empty connections, got %d", len(conns1))
	}

	// Process spans
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Invalidate and verify
	api.InvalidateCache()

	conns2, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("second GetConnections failed: %v", err)
	}
	if len(conns2) != 4 {
		t.Errorf("expected 4 connections after invalidation, got %d", len(conns2))
	}
}

func TestProcessorInvalidatesCache(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	api := backendapi.New(database)

	// Warm up cache with empty result
	_, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("initial GetCatalog failed: %v", err)
	}

	// Process with cache invalidator wired — should auto-invalidate
	p := processor.New(database, processor.WithCacheInvalidator(api))
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Cache was invalidated by processor, so this should return fresh data
	catalog, err := api.GetCatalog(ctx)
	if err != nil {
		t.Fatalf("GetCatalog after process failed: %v", err)
	}
	if len(catalog.AppItems) != 5 {
		t.Errorf("expected 5 app items (cache should have been invalidated), got %d", len(catalog.AppItems))
	}
}

func TestConcurrentCacheAccess(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	p := processor.New(database)
	if err := p.Process(ctx, testSpans); err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	api := backendapi.New(database)

	// Prime cache
	_, _ = api.GetCatalog(ctx)
	_, _ = api.GetConnections(ctx)

	// Concurrent reads with interleaved invalidations
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%5 == 0 {
				api.InvalidateCache()
			}
			catalog, err := api.GetCatalog(ctx)
			if err != nil {
				t.Errorf("concurrent GetCatalog failed: %v", err)
				return
			}
			if catalog == nil {
				t.Error("concurrent GetCatalog returned nil")
			}
		}()
	}
	wg.Wait()
}

func TestCacheEmptyConnections(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)
	api := backendapi.New(database)

	// Empty DB — connections is an empty slice, not nil
	conns1, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("first GetConnections failed: %v", err)
	}
	if len(conns1) != 0 {
		t.Fatalf("expected empty connections")
	}

	// Second call should be a cache hit
	conns2, err := api.GetConnections(ctx)
	if err != nil {
		t.Fatalf("second GetConnections failed: %v", err)
	}
	if len(conns2) != 0 {
		t.Errorf("expected empty connections from cache")
	}
}

func TestProcessorWithoutCacheInvalidator(t *testing.T) {
	ctx := context.Background()
	database := setupDB(ctx)

	// No cache invalidator — should not panic
	p := processor.New(database)
	spans := []models.RawSpan{
		{
			Id: "span-001",
			Attributes: map[string]string{
				"ebpf.span.type":   "NETWORK",
				"ebpf.source":      "frontend",
				"ebpf.destination": "backend",
				"ebpf.http.path":   "/api",
			},
		},
	}
	if err := p.Process(ctx, spans); err != nil {
		t.Fatalf("Process without invalidator failed: %v", err)
	}
}
