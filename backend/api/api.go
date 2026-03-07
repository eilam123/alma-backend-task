package api

import (
	"context"
	"fmt"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

type APIBackend struct {
	db db.Database
}

func New(database db.Database) *APIBackend {
	return &APIBackend{db: database}
}

type CatalogComponentResponse struct {
	ComponentType models.ComponentType `json:"component_type"`
	Path          string               `json:"path,omitempty"`
	Topic         string               `json:"topic,omitempty"`
	Query         string               `json:"query,omitempty"`
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
	Path          string               `json:"path,omitempty"`
	Topic         string               `json:"topic,omitempty"`
	Query         string               `json:"query,omitempty"`
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
	base := buildConnectionComponent(rec)
	return CatalogComponentResponse{
		ComponentType: base.ComponentType,
		Path:          base.Path,
		Topic:         base.Topic,
		Query:         base.Query,
		PIIs:          piis,
	}
}

func buildConnectionComponent(rec db.Record) ConnectionComponentResponse {
	ct := models.ComponentType(str(rec["component_type"]))
	value := str(rec["value"])
	comp := ConnectionComponentResponse{ComponentType: ct}
	switch ct {
	case models.ComponentTypeEndpoint:
		comp.Path = value
	case models.ComponentTypeQueue:
		comp.Topic = value
	case models.ComponentTypeQuery:
		comp.Query = value
	}
	return comp
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
