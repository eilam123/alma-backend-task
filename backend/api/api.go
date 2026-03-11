package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/metrics"
	"github.com/alma/assignment/models"
)

type APIBackend struct {
	db     db.Database
	logger *slog.Logger
	cache  responseCache
}

// APIOption configures an APIBackend.
type APIOption func(*APIBackend)

// WithAPILogger sets a custom logger for the API backend.
func WithAPILogger(logger *slog.Logger) APIOption {
	return func(a *APIBackend) {
		a.logger = logger
	}
}

func New(database db.Database, opts ...APIOption) *APIBackend {
	a := &APIBackend{db: database, logger: slog.Default()}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// InvalidateCache clears all cached API responses, forcing the next
// call to GetCatalog/GetConnections to re-read from the DB.
func (a *APIBackend) InvalidateCache() {
	a.cache.invalidate()
	metrics.CacheInvalidationsTotal.Inc()
	a.logger.Debug("API cache invalidated")
}

type CatalogComponentResponse struct {
	ComponentType models.ComponentType `json:"component_type"`
	Value         string               `json:"value"`
	PIIs          []models.PIIType     `json:"piis"`
}

type AppItemResponse struct {
	Name       string                     `json:"name"`
	Type       models.AppItemType         `json:"type"`
	Components []CatalogComponentResponse `json:"components"`
}

type CatalogResponse struct {
	AppItems map[string]AppItemResponse `json:"app_items"`
}

type ConnectionComponentResponse struct {
	ComponentType models.ComponentType `json:"component_type"`
	Value         string               `json:"value"`
}

type ConnectionResponse struct {
	Source      string                        `json:"source"`
	Destination string                        `json:"destination"`
	Components  []ConnectionComponentResponse `json:"components"`
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func buildCatalogComponent(rec db.Record, piis []models.PIIType) CatalogComponentResponse {
	return CatalogComponentResponse{
		ComponentType: models.ComponentType(str(rec["component_type"])),
		Value:         str(rec["value"]),
		PIIs:          piis,
	}
}

func buildConnectionComponent(rec db.Record) ConnectionComponentResponse {
	return ConnectionComponentResponse{
		ComponentType: models.ComponentType(str(rec["component_type"])),
		Value:         str(rec["value"]),
	}
}

func (a *APIBackend) GetCatalog(ctx context.Context) (*CatalogResponse, error) {
	start := time.Now()
	defer func() {
		d := time.Since(start)
		metrics.APIQueryDuration.WithLabelValues("catalog").Observe(d.Seconds())
		a.logger.Info("GetCatalog completed", "duration", d)
	}()

	cached, gen := a.cache.getCatalog()
	if cached != nil {
		metrics.CacheHitsTotal.WithLabelValues("catalog").Inc()
		return cached, nil
	}
	metrics.CacheMissesTotal.WithLabelValues("catalog").Inc()

	// Fetch all 3 tables in parallel, using AllGroupedBy for pre-indexed results
	var (
		appItemRecords                        []db.Record
		componentsByAppItem, piisByComponentR map[any][]db.Record
		errItems, errComps, errPIIs           error
		wg                                    sync.WaitGroup
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		appItemRecords, errItems = a.db.All(ctx, "app_items")
	}()
	go func() {
		defer wg.Done()
		componentsByAppItem, errComps = a.db.AllGroupedBy(ctx, "components", "app_item_name")
	}()
	go func() {
		defer wg.Done()
		piisByComponentR, errPIIs = a.db.AllGroupedBy(ctx, "component_piis", "component_id")
	}()
	wg.Wait()

	if err := errors.Join(errItems, errComps, errPIIs); err != nil {
		return nil, err
	}

	result := &CatalogResponse{AppItems: make(map[string]AppItemResponse)}
	for _, aiRec := range appItemRecords {
		name := str(aiRec["name"])
		comps := componentsByAppItem[name]
		components := make([]CatalogComponentResponse, 0, len(comps))
		for _, compRec := range comps {
			compID := str(compRec["id"])
			piiRecs := piisByComponentR[compID]
			piis := make([]models.PIIType, 0, len(piiRecs))
			for _, pr := range piiRecs {
				piis = append(piis, models.PIIType(str(pr["pii_type"])))
			}
			components = append(components, buildCatalogComponent(compRec, piis))
		}
		result.AppItems[name] = AppItemResponse{
			Name:       name,
			Type:       models.AppItemType(str(aiRec["type"])),
			Components: components,
		}
	}

	a.cache.setCatalog(gen, result)
	return result, nil
}

func (a *APIBackend) GetConnections(ctx context.Context) ([]ConnectionResponse, error) {
	start := time.Now()
	defer func() {
		d := time.Since(start)
		metrics.APIQueryDuration.WithLabelValues("connections").Observe(d.Seconds())
		a.logger.Info("GetConnections completed", "duration", d)
	}()

	cached, gen, ok := a.cache.getConnections()
	if ok {
		metrics.CacheHitsTotal.WithLabelValues("connections").Inc()
		return cached, nil
	}
	metrics.CacheMissesTotal.WithLabelValues("connections").Inc()

	// Fetch connections and components in parallel
	var (
		connRecords, allComps []db.Record
		errConns, errComps    error
		wg                    sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		connRecords, errConns = a.db.All(ctx, "connections")
	}()
	go func() {
		defer wg.Done()
		allComps, errComps = a.db.All(ctx, "components")
	}()
	wg.Wait()

	if err := errors.Join(errConns, errComps); err != nil {
		return nil, err
	}

	compByID := make(map[string]db.Record, len(allComps))
	for _, c := range allComps {
		compByID[str(c["id"])] = c
	}

	connections := make([]ConnectionResponse, 0, len(connRecords))
	for _, connRec := range connRecords {
		compIDs, _ := connRec["component_ids"].([]string)

		components := make([]ConnectionComponentResponse, 0, len(compIDs))
		for _, compID := range compIDs {
			rec, ok := compByID[compID]
			if !ok {
				return nil, fmt.Errorf("component %q not found", compID)
			}
			components = append(components, buildConnectionComponent(rec))
		}

		connections = append(connections, ConnectionResponse{
			Source:      str(connRec["source"]),
			Destination: str(connRec["destination"]),
			Components:  components,
		})
	}

	a.cache.setConnections(gen, connections)
	return connections, nil
}
