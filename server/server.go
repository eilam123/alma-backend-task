package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/alma/assignment/backend/api"
)

// APIServer serves the REST API endpoints.
type APIServer struct {
	httpServer *http.Server
	api        *api.APIBackend
	logger     *slog.Logger
}

// NewAPIServer creates a new REST API server on the given address.
func NewAPIServer(addr string, apiBackend *api.APIBackend, logger *slog.Logger) *APIServer {
	s := &APIServer{api: apiBackend, logger: logger}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /catalog", s.handleCatalog)
	mux.HandleFunc("GET /connections", s.handleConnections)

	s.httpServer = &http.Server{Addr: addr, Handler: mux}
	return s
}

// Start begins listening. It returns http.ErrServerClosed on graceful shutdown.
func (s *APIServer) Start() error {
	s.logger.Info("API server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *APIServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *APIServer) handleCatalog(w http.ResponseWriter, r *http.Request) {
	catalog, err := s.api.GetCatalog(r.Context())
	if err != nil {
		s.logger.Error("GetCatalog failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(catalog); err != nil {
		s.logger.Error("encode catalog failed", "error", err)
	}
}

func (s *APIServer) handleConnections(w http.ResponseWriter, r *http.Request) {
	connections, err := s.api.GetConnections(r.Context())
	if err != nil {
		s.logger.Error("GetConnections failed", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(connections); err != nil {
		s.logger.Error("encode connections failed", "error", err)
	}
}
