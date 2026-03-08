package processor

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/metrics"
	"github.com/alma/assignment/models"
)

type SpanProcessor struct {
	db           db.Database
	handlers     map[string]SpanTypeHandler
	piiDetectors []PIIDetector
	logger       *slog.Logger
}

func New(database db.Database, opts ...Option) *SpanProcessor {
	p := &SpanProcessor{
		db:           database,
		handlers:     defaultHandlers(),
		piiDetectors: DefaultPIIDetectors(),
		logger:       slog.Default(),
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
	start := time.Now()
	p.logger.Info("starting span processing", "count", len(rawSpans))

	for _, span := range rawSpans {
		if err := p.processSpan(ctx, span); err != nil {
			p.logger.Error("span processing failed", "error", err)
			metrics.SpansErrorsTotal.Inc()
			return err
		}
		metrics.SpansProcessedTotal.Inc()
	}

	duration := time.Since(start)
	metrics.SpanProcessingDuration.Observe(duration.Seconds())
	p.logger.Info("span processing complete", "count", len(rawSpans), "duration", duration)

	p.recordGauges(ctx)
	return nil
}

func (p *SpanProcessor) processSpan(ctx context.Context, span models.RawSpan) error {
	attrs := span.Attributes
	spanType := attrs[models.AttrSpanType]
	source := attrs[models.AttrSource]
	destination := attrs[models.AttrDestination]

	p.logger.Debug("processing span", "type", spanType, "source", source, "destination", destination)

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
	metrics.DBOperationsTotal.WithLabelValues("app_items", "upsert").Inc()

	if err := p.db.Upsert(ctx, "app_items", db.Record{
		"name": destination,
		"type": string(handler.DestinationAppItemType()),
	}); err != nil {
		return fmt.Errorf("upsert destination app item %q: %w", destination, err)
	}
	metrics.DBOperationsTotal.WithLabelValues("app_items", "upsert").Inc()

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
	metrics.DBOperationsTotal.WithLabelValues("components", "upsert").Inc()

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
		p.logger.Info("PII detected", "type", piiType, "component_id", componentID)
		metrics.PIIDetectionsTotal.WithLabelValues(string(piiType)).Inc()
		piiID := componentID + ":" + string(piiType)
		if err := p.db.InsertOnConflict(ctx, "component_piis", db.Record{
			"id":           piiID,
			"component_id": componentID,
			"pii_type":     string(piiType),
		}, db.ConflictOptions{Action: db.ConflictDoNothing}); err != nil {
			return fmt.Errorf("insert pii %q for component %q: %w", piiType, componentID, err)
		}
		metrics.DBOperationsTotal.WithLabelValues("component_piis", "insert").Inc()
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
	metrics.DBOperationsTotal.WithLabelValues("connections", "upsert").Inc()
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

func (p *SpanProcessor) recordGauges(ctx context.Context) {
	appItems, _ := p.db.All(ctx, "app_items")
	typeCounts := map[string]float64{}
	for _, rec := range appItems {
		t, _ := rec["type"].(string)
		typeCounts[t]++
	}
	for t, count := range typeCounts {
		metrics.AppItemsTotal.WithLabelValues(t).Set(count)
	}

	components, _ := p.db.All(ctx, "components")
	compCounts := map[string]float64{}
	for _, rec := range components {
		t, _ := rec["component_type"].(string)
		compCounts[t]++
	}
	for t, count := range compCounts {
		metrics.ComponentsTotal.WithLabelValues(t).Set(count)
	}

	connections, _ := p.db.All(ctx, "connections")
	metrics.ConnectionsTotal.Set(float64(len(connections)))
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
