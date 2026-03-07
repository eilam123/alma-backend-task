package processor

import (
	"context"
	"fmt"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

type SpanProcessor struct {
	db           db.Database
	handlers     map[string]SpanTypeHandler
	piiDetectors []PIIDetector
}

func New(database db.Database, opts ...Option) *SpanProcessor {
	p := &SpanProcessor{
		db:           database,
		handlers:     defaultHandlers(),
		piiDetectors: DefaultPIIDetectors(),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

func (p *SpanProcessor) handlerFor(spanType string) SpanTypeHandler {
	if h, ok := p.handlers[spanType]; ok {
		return h
	}
	return networkHandler{}
}

func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error {
	for _, span := range rawSpans {
		if err := p.processSpan(ctx, span); err != nil {
			return err
		}
	}
	return nil
}

func (p *SpanProcessor) processSpan(ctx context.Context, span models.RawSpan) error {
	attrs := span.Attributes
	spanType := attrs[models.AttrSpanType]
	source := attrs[models.AttrSource]
	destination := attrs[models.AttrDestination]

	handler := p.handlerFor(spanType)

	if err := p.upsertAppItems(ctx, source, destination, handler); err != nil {
		return err
	}

	componentID, err := p.upsertComponent(ctx, destination, handler, attrs)
	if err != nil {
		return err
	}

	if err := p.detectAndInsertPIIs(ctx, componentID, handler, attrs); err != nil {
		return err
	}

	return p.upsertConnection(ctx, source, destination, componentID)
}

func (p *SpanProcessor) upsertAppItems(ctx context.Context, source, destination string, handler SpanTypeHandler) error {
	if err := p.db.Upsert(ctx, "app_items", db.Record{
		"name": source,
		"type": string(sourceAppItemType(source)),
	}); err != nil {
		return fmt.Errorf("upsert source app item %q: %w", source, err)
	}

	if err := p.db.Upsert(ctx, "app_items", db.Record{
		"name": destination,
		"type": string(handler.DestinationAppItemType()),
	}); err != nil {
		return fmt.Errorf("upsert destination app item %q: %w", destination, err)
	}

	return nil
}

func (p *SpanProcessor) upsertComponent(ctx context.Context, destination string, handler SpanTypeHandler, attrs map[string]string) (string, error) {
	componentType, value := handler.ComponentInfo(attrs)
	componentID := string(componentType) + ":" + destination + ":" + value

	if err := p.db.Upsert(ctx, "components", db.Record{
		"id":             componentID,
		"app_item_name":  destination,
		"component_type": string(componentType),
		"value":          value,
	}); err != nil {
		return "", fmt.Errorf("upsert component %q: %w", componentID, err)
	}

	return componentID, nil
}

func (p *SpanProcessor) detectAndInsertPIIs(ctx context.Context, componentID string, handler SpanTypeHandler, attrs map[string]string) error {
	piiFields := handler.PIIFields(attrs)
	detectedPIIs := make(map[models.PIIType]struct{})
	for _, text := range piiFields {
		for _, piiType := range p.detectPIIs(text) {
			detectedPIIs[piiType] = struct{}{}
		}
	}
	for piiType := range detectedPIIs {
		piiID := componentID + ":" + string(piiType)
		if err := p.db.InsertOnConflict(ctx, "component_piis", db.Record{
			"id":           piiID,
			"component_id": componentID,
			"pii_type":     string(piiType),
		}, db.ConflictOptions{Action: db.ConflictDoNothing}); err != nil {
			return fmt.Errorf("insert pii %q for component %q: %w", piiType, componentID, err)
		}
	}
	return nil
}

func (p *SpanProcessor) upsertConnection(ctx context.Context, source, destination, componentID string) error {
	connID := source + ":" + destination
	if err := p.db.InsertOnConflict(ctx, "connections", db.Record{
		"id":            connID,
		"source":        source,
		"destination":   destination,
		"component_ids": []string{componentID},
	}, db.ConflictOptions{
		Action:       db.ConflictDoUpdate,
		UpdateFields: []string{"component_ids"},
		MergeFuncs: map[string]db.MergeFunc{
			"component_ids": mergeUniqueStrings,
		},
	}); err != nil {
		return fmt.Errorf("upsert connection %q: %w", connID, err)
	}
	return nil
}

func sourceAppItemType(source string) models.AppItemType {
	if source == "internet" {
		return models.AppItemTypeInternet
	}
	return models.AppItemTypeService
}

func mergeUniqueStrings(existing, incoming any) any {
	existingIDs, _ := existing.([]string)
	newIDs, _ := incoming.([]string)
	seen := make(map[string]struct{}, len(existingIDs))
	for _, id := range existingIDs {
		seen[id] = struct{}{}
	}
	for _, id := range newIDs {
		if _, ok := seen[id]; !ok {
			existingIDs = append(existingIDs, id)
		}
	}
	return existingIDs
}

func (p *SpanProcessor) detectPIIs(text string) []models.PIIType {
	var found []models.PIIType
	for _, detector := range p.piiDetectors {
		if detector.Pattern.MatchString(text) {
			found = append(found, detector.Type)
		}
	}
	return found
}
