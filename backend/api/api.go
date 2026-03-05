package api

import (
	"context"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

type APIBackend struct {
	db *db.DB
}

func New(database *db.DB) *APIBackend {
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
	ct := models.ComponentType(str(rec["component_type"]))
	value := str(rec["value"])
	comp := CatalogComponentResponse{
		ComponentType: ct,
		PIIs:          piis,
	}
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

func (a *APIBackend) GetCatalog(ctx context.Context) (*CatalogResponse, error) {
	appItemRecords, err := a.db.All(ctx, "app_items")
	if err != nil {
		return nil, err
	}

	allComponents, err := a.db.All(ctx, "components")
	if err != nil {
		return nil, err
	}

	allPIIs, err := a.db.All(ctx, "component_piis")
	if err != nil {
		return nil, err
	}

	piisByComponent := map[string][]models.PIIType{}
	for _, piiRec := range allPIIs {
		compID := str(piiRec["component_id"])
		piisByComponent[compID] = append(piisByComponent[compID], models.PIIType(str(piiRec["pii_type"])))
	}

	componentsByAppItem := map[string][]db.Record{}
	for _, compRec := range allComponents {
		name := str(compRec["app_item_name"])
		componentsByAppItem[name] = append(componentsByAppItem[name], compRec)
	}

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
		return nil, err
	}

	connections := make([]ConnectionResponse, 0, len(connRecords))
	for _, connRec := range connRecords {
		compIDs, _ := connRec["component_ids"].([]string)

		components := make([]ConnectionComponentResponse, 0, len(compIDs))
		for _, compID := range compIDs {
			compRec, err := a.db.Get(ctx, "components", compID)
			if err != nil {
				return nil, err
			}
			components = append(components, buildConnectionComponent(compRec))
		}

		connections = append(connections, ConnectionResponse{
			Source:      str(connRec["source"]),
			Destination: str(connRec["destination"]),
			Components:  components,
		})
	}

	return connections, nil
}
