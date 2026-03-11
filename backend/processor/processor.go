package processor

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

type SpanProcessor struct {
	db                  db.Database
	handlers            map[string]SpanTypeHandler
	piiDetectors        []PIIDetector
	logger              *slog.Logger
	batchFlushThreshold int // 0 means accumulate all before flushing
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

// accumulator collects all DB writes in memory before flushing in batch.
type accumulator struct {
	appItems    map[string]db.Record // keyed by PK (name)
	components  map[string]db.Record // keyed by PK (id)
	piis        map[string]db.Record // keyed by PK (id)
	connections map[string]db.Record // keyed by PK (id)
}

func newAccumulator() *accumulator {
	return &accumulator{
		appItems:    make(map[string]db.Record),
		components:  make(map[string]db.Record),
		piis:        make(map[string]db.Record),
		connections: make(map[string]db.Record),
	}
}

func (a *accumulator) exceedsThreshold(threshold int) bool {
	return len(a.appItems) > threshold ||
		len(a.components) > threshold ||
		len(a.piis) > threshold ||
		len(a.connections) > threshold
}

func (a *accumulator) reset() {
	a.appItems = make(map[string]db.Record)
	a.components = make(map[string]db.Record)
	a.piis = make(map[string]db.Record)
	a.connections = make(map[string]db.Record)
}

func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error {
	start := time.Now()
	p.logger.Info("starting span processing", "count", len(rawSpans))

	acc := newAccumulator()

	// Phase 1: Accumulate writes in memory, flushing when threshold is exceeded
	for _, span := range rawSpans {
		p.accumulateSpan(acc, span)
		metrics.SpansProcessedTotal.Inc()

		if p.batchFlushThreshold > 0 && acc.exceedsThreshold(p.batchFlushThreshold) {
			if err := p.flush(ctx, acc); err != nil {
				p.logger.Error("flush failed", "error", err)
				metrics.SpansErrorsTotal.Inc()
				return err
			}
			acc.reset()
		}
	}

	// Phase 2: Flush remaining accumulated data
	if err := p.flush(ctx, acc); err != nil {
		p.logger.Error("flush failed", "error", err)
		metrics.SpansErrorsTotal.Inc()
		return err
	}

	duration := time.Since(start)
	metrics.SpanProcessingDuration.Observe(duration.Seconds())
	p.logger.Info("span processing complete", "count", len(rawSpans), "duration", duration)

	if p.batchFlushThreshold > 0 {
		// With threshold flushing, accumulator was reset between flushes; read from DB
		p.recordGaugesFromDB(ctx)
	} else {
		// Without threshold, accumulator has the full picture
		p.recordGaugesFromAccumulator(acc)
	}
	return nil
}

func (p *SpanProcessor) accumulateSpan(acc *accumulator, span models.RawSpan) {
	attrs := span.Attributes
	spanType := attrs[models.AttrSpanType]
	source := attrs[models.AttrSource]
	destination := attrs[models.AttrDestination]

	p.logger.Debug("processing span", "type", spanType, "source", source, "destination", destination)

	handler := p.handlerFor(spanType)

	// Accumulate app items
	acc.appItems[source] = db.Record{
		"name": source,
		"type": string(sourceAppItemType(source)),
	}
	acc.appItems[destination] = db.Record{
		"name": destination,
		"type": string(handler.DestinationAppItemType()),
	}

	// Accumulate component
	componentType, value := handler.ComponentInfo(attrs)
	componentID := string(componentType) + ":" + destination + ":" + value
	acc.components[componentID] = db.Record{
		"id":             componentID,
		"app_item_name":  destination,
		"component_type": string(componentType),
		"value":          value,
	}

	// Accumulate PIIs
	piiFields := handler.PIIFields(attrs)
	for _, text := range piiFields {
		for _, piiType := range p.detectPIIs(text) {
			p.logger.Info("PII detected", "type", piiType, "component_id", componentID)
			metrics.PIIDetectionsTotal.WithLabelValues(string(piiType)).Inc()
			piiID := componentID + ":" + string(piiType)
			if _, exists := acc.piis[piiID]; !exists {
				acc.piis[piiID] = db.Record{
					"id":           piiID,
					"component_id": componentID,
					"pii_type":     string(piiType),
				}
			}
		}
	}

	// Accumulate connection with in-memory mergeUniqueStrings
	connID := source + ":" + destination
	if existing, ok := acc.connections[connID]; ok {
		existingIDs, _ := existing["component_ids"].([]string)
		merged := mergeUniqueStrings(existingIDs, []string{componentID})
		existing["component_ids"] = merged
	} else {
		acc.connections[connID] = db.Record{
			"id":            connID,
			"source":        source,
			"destination":   destination,
			"component_ids": []string{componentID},
		}
	}
}

func (p *SpanProcessor) flush(ctx context.Context, acc *accumulator) error {
	// Wave 1: app_items and components in parallel (independent tables)
	var errItems, errComps error
	var wg sync.WaitGroup

	if len(acc.appItems) > 0 || len(acc.components) > 0 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if len(acc.appItems) == 0 {
				return
			}
			records := mapValues(acc.appItems)
			if err := p.db.UpsertBatch(ctx, "app_items", records); err != nil {
				errItems = fmt.Errorf("batch upsert app_items: %w", err)
				return
			}
			metrics.DBOperationsTotal.WithLabelValues("app_items", "upsert").Add(float64(len(records)))
		}()
		go func() {
			defer wg.Done()
			if len(acc.components) == 0 {
				return
			}
			records := mapValues(acc.components)
			if err := p.db.UpsertBatch(ctx, "components", records); err != nil {
				errComps = fmt.Errorf("batch upsert components: %w", err)
				return
			}
			metrics.DBOperationsTotal.WithLabelValues("components", "upsert").Add(float64(len(records)))
		}()
		wg.Wait()

		if err := errors.Join(errItems, errComps); err != nil {
			return err
		}
	}

	// Wave 2: PIIs and connections in parallel (independent tables)
	var errPIIs, errConns error

	if len(acc.piis) > 0 || len(acc.connections) > 0 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if len(acc.piis) == 0 {
				return
			}
			records := mapValues(acc.piis)
			if err := p.db.InsertBatchOnConflict(ctx, "component_piis", records, db.ConflictOptions{
				Action: db.ConflictDoNothing,
			}); err != nil {
				errPIIs = fmt.Errorf("batch insert component_piis: %w", err)
				return
			}
			metrics.DBOperationsTotal.WithLabelValues("component_piis", "insert").Add(float64(len(records)))
		}()
		go func() {
			defer wg.Done()
			if len(acc.connections) == 0 {
				return
			}
			records := mapValues(acc.connections)
			if err := p.db.InsertBatchOnConflict(ctx, "connections", records, db.ConflictOptions{
				Action:       db.ConflictDoUpdate,
				UpdateFields: []string{"component_ids"},
				MergeFuncs: map[string]db.MergeFunc{
					"component_ids": mergeUniqueStrings,
				},
			}); err != nil {
				errConns = fmt.Errorf("batch insert connections: %w", err)
				return
			}
			metrics.DBOperationsTotal.WithLabelValues("connections", "upsert").Add(float64(len(records)))
		}()
		wg.Wait()

		if err := errors.Join(errPIIs, errConns); err != nil {
			return err
		}
	}

	return nil
}

func mapValues(m map[string]db.Record) []db.Record {
	records := make([]db.Record, 0, len(m))
	for _, r := range m {
		records = append(records, r)
	}
	return records
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

func (p *SpanProcessor) recordGaugesFromDB(ctx context.Context) {
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

func (p *SpanProcessor) recordGaugesFromAccumulator(acc *accumulator) {
	// Compute app item type counts from accumulator
	typeCounts := map[string]float64{}
	for _, rec := range acc.appItems {
		t, _ := rec["type"].(string)
		typeCounts[t]++
	}
	for t, count := range typeCounts {
		metrics.AppItemsTotal.WithLabelValues(t).Set(count)
	}

	// Compute component type counts from accumulator
	compCounts := map[string]float64{}
	for _, rec := range acc.components {
		t, _ := rec["component_type"].(string)
		compCounts[t]++
	}
	for t, count := range compCounts {
		metrics.ComponentsTotal.WithLabelValues(t).Set(count)
	}

	metrics.ConnectionsTotal.Set(float64(len(acc.connections)))
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
