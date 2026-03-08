package api

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

type APIBackend struct {
	db     db.Database
	logger *slog.Logger
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

func indexPIIsByComponent(records []db.Record) map[string][]models.PIIType {
	result := map[string][]models.PIIType{}
	for _, rec := range records {
		compID := str(rec["component_id"])
		result[compID] = append(result[compID], models.PIIType(str(rec["pii_type"])))
	}
	return result
}

func indexComponentsByAppItem(records []db.Record) map[string][]db.Record {
	result := map[string][]db.Record{}
	for _, rec := range records {
		name := str(rec["app_item_name"])
		result[name] = append(result[name], rec)
	}
	return result
}

func (a *APIBackend) GetCatalog(ctx context.Context) (*CatalogResponse, error) {
	start := time.Now()
	defer func() { a.logger.Info("GetCatalog completed", "duration", time.Since(start)) }()

	appItemRecords, err := a.db.All(ctx, "app_items")
	if err != nil {
		return nil, fmt.Errorf("fetch app_items: %w", err)
	}

	allComponents, err := a.db.All(ctx, "components")
	if err != nil {
		return nil, fmt.Errorf("fetch components: %w", err)
	}

	allPIIs, err := a.db.All(ctx, "component_piis")
	if err != nil {
		return nil, fmt.Errorf("fetch component_piis: %w", err)
	}

	piisByComponent := indexPIIsByComponent(allPIIs)
	componentsByAppItem := indexComponentsByAppItem(allComponents)

	result := &CatalogResponse{AppItems: make(map[string]AppItemResponse)}
	for _, aiRec := range appItemRecords {
		name := str(aiRec["name"])
		comps := componentsByAppItem[name]
		components := make([]CatalogComponentResponse, 0, len(comps))
		for _, compRec := range comps {
			compID := str(compRec["id"])
			piis := piisByComponent[compID]
			if piis == nil {
				piis = []models.PIIType{}
			}
			components = append(components, buildCatalogComponent(compRec, piis))
		}
		result.AppItems[name] = AppItemResponse{
			Name:       name,
			Type:       models.AppItemType(str(aiRec["type"])),
			Components: components,
		}
	}

	return result, nil
}

func (a *APIBackend) GetConnections(ctx context.Context) ([]ConnectionResponse, error) {
	start := time.Now()
	defer func() { a.logger.Info("GetConnections completed", "duration", time.Since(start)) }()

	connRecords, err := a.db.All(ctx, "connections")
	if err != nil {
		return nil, fmt.Errorf("fetch connections: %w", err)
	}

	allComps, err := a.db.All(ctx, "components")
	if err != nil {
		return nil, fmt.Errorf("fetch components: %w", err)
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

	return connections, nil
}
