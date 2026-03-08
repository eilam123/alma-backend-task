package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alma/assignment/backend/api"
	"github.com/alma/assignment/backend/processor"
	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
	"github.com/alma/assignment/schema"
)

func setupTestAPI(t *testing.T) *api.APIBackend {
	t.Helper()
	ctx := context.Background()
	database := db.New()
	if err := schema.CreateSchema(ctx, database); err != nil {
		t.Fatalf("CreateSchema: %v", err)
	}

	spans := []models.RawSpan{
		{Attributes: map[string]string{
			"ebpf.span.type":      "NETWORK",
			"ebpf.source":         "internet",
			"ebpf.destination":    "web-service",
			"ebpf.http.path":      "/api/users",
			"ebpf.http.req_body":  `{"email":"test@example.com"}`,
			"ebpf.http.resp_body": "{}",
		}},
	}

	p := processor.New(database)
	if err := p.Process(ctx, spans); err != nil {
		t.Fatalf("Process: %v", err)
	}

	return api.New(database)
}

func TestHandleCatalog(t *testing.T) {
	apiBackend := setupTestAPI(t)
	srv := NewAPIServer(":0", apiBackend, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/catalog", nil)
	w := httptest.NewRecorder()
	srv.handleCatalog(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var result api.CatalogResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result.AppItems) == 0 {
		t.Error("expected non-empty catalog")
	}
}

func TestHandleConnections(t *testing.T) {
	apiBackend := setupTestAPI(t)
	srv := NewAPIServer(":0", apiBackend, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/connections", nil)
	w := httptest.NewRecorder()
	srv.handleConnections(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var result []api.ConnectionResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(result) == 0 {
		t.Error("expected non-empty connections")
	}
}

func TestHandleCatalog_MethodNotAllowed(t *testing.T) {
	apiBackend := setupTestAPI(t)
	srv := NewAPIServer(":0", apiBackend, slog.Default())

	// Use the mux directly to test routing
	req := httptest.NewRequest(http.MethodPost, "/catalog", nil)
	w := httptest.NewRecorder()
	srv.httpServer.Handler.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Error("POST /catalog should not return 200")
	}
}
