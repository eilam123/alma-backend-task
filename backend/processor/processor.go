package processor

import (
	"context"
	"fmt"
	"regexp"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

var piiPatterns = map[models.PIIType]*regexp.Regexp{
	models.PIITypeEmail:      regexp.MustCompile(`\b[\w.%+\-]+@[\w.\-]+\.[a-zA-Z]{2,}\b`),
	models.PIITypeCreditCard: regexp.MustCompile(`\b\d{4}[- ]?\d{4}[- ]?\d{4}[- ]?\d{4}\b`),
	models.PIITypeSSN:        regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	models.PIITypePhone:      regexp.MustCompile(`\b(\+\d{1,3}[- ]?)?\(?\d{3}\)?[- ]?\d{3}[- ]?\d{4}\b`),
	models.PIITypeIPAddress:  regexp.MustCompile(`\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b`),
}

type SpanProcessor struct {
	db db.Database
}

func New(database db.Database) *SpanProcessor {
	return &SpanProcessor{db: database}
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

	if err := p.upsertAppItems(ctx, source, destination, spanType); err != nil {
		return err
	}

	componentID, err := p.upsertComponent(ctx, destination, spanType, attrs)
	if err != nil {
		return err
	}

	if err := p.detectAndInsertPIIs(ctx, componentID, spanType, attrs); err != nil {
		return err
	}

	return p.upsertConnection(ctx, source, destination, componentID)
}

func (p *SpanProcessor) upsertAppItems(ctx context.Context, source, destination, spanType string) error {
	if err := p.db.Upsert(ctx, "app_items", db.Record{
		"name": source,
		"type": string(sourceAppItemType(source)),
	}); err != nil {
		return fmt.Errorf("upsert source app item %q: %w", source, err)
	}

	if err := p.db.Upsert(ctx, "app_items", db.Record{
		"name": destination,
		"type": string(destinationAppItemType(spanType)),
	}); err != nil {
		return fmt.Errorf("upsert destination app item %q: %w", destination, err)
	}

	return nil
}

func (p *SpanProcessor) upsertComponent(ctx context.Context, destination, spanType string, attrs map[string]string) (string, error) {
	componentType, value := componentInfo(spanType, attrs)
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

func (p *SpanProcessor) detectAndInsertPIIs(ctx context.Context, componentID, spanType string, attrs map[string]string) error {
	piiFields := piiFieldsForSpanType(spanType, attrs)
	detectedPIIs := make(map[models.PIIType]struct{})
	for _, text := range piiFields {
		for _, piiType := range detectPIIs(text) {
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

func destinationAppItemType(spanType string) models.AppItemType {
	switch spanType {
	case "QUERY":
		return models.AppItemTypeDatabase
	case "MESSAGE":
		return models.AppItemTypeQueue
	default:
		return models.AppItemTypeService
	}
}

func componentInfo(spanType string, attrs map[string]string) (models.ComponentType, string) {
	switch spanType {
	case "QUERY":
		return models.ComponentTypeQuery, attrs[models.AttrDBQuery]
	case "MESSAGE":
		return models.ComponentTypeQueue, attrs[models.AttrQueueTopic]
	default:
		return models.ComponentTypeEndpoint, attrs[models.AttrHTTPPath]
	}
}

func piiFieldsForSpanType(spanType string, attrs map[string]string) []string {
	switch spanType {
	case "NETWORK":
		return []string{attrs[models.AttrHTTPReqBody], attrs[models.AttrHTTPRespBody]}
	case "QUERY":
		return []string{attrs[models.AttrDBQueryValues]}
	case "MESSAGE":
		return []string{attrs[models.AttrQueuePayload]}
	default:
		return nil
	}
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

func detectPIIs(text string) []models.PIIType {
	var found []models.PIIType
	for piiType, pattern := range piiPatterns {
		if pattern.MatchString(text) {
			found = append(found, piiType)
		}
	}
	return found
}
