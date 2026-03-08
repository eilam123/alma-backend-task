package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsServer serves the Prometheus /metrics endpoint.
type MetricsServer struct {
	httpServer *http.Server
	logger     *slog.Logger
}

// NewMetricsServer creates a new metrics server on the given address.
func NewMetricsServer(addr string, logger *slog.Logger) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())

	return &MetricsServer{
		httpServer: &http.Server{Addr: addr, Handler: mux},
		logger:     logger,
	}
}

// Start begins listening. It returns http.ErrServerClosed on graceful shutdown.
func (s *MetricsServer) Start() error {
	s.logger.Info("metrics server starting", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *MetricsServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
