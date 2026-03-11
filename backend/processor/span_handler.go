package processor

import (
	"log/slog"

	"github.com/alma/assignment/models"
)

// SpanTypeHandler defines how a specific span type is processed.
type SpanTypeHandler interface {
	DestinationAppItemType() models.AppItemType
	ComponentInfo(attrs map[string]string) (models.ComponentType, string)
	PIIFields(attrs map[string]string) []string
}

// Option configures a SpanProcessor.
type Option func(*SpanProcessor)

// WithLogger sets a custom logger for the processor.
func WithLogger(logger *slog.Logger) Option {
	return func(p *SpanProcessor) {
		p.logger = logger
	}
}

// WithSpanHandler registers a handler for a given span type (e.g. "NETWORK", "QUERY").
func WithSpanHandler(spanType string, handler SpanTypeHandler) Option {
	return func(p *SpanProcessor) {
		p.handlers[spanType] = handler
	}
}

// WithPIIDetectors replaces the default PII detectors.
func WithPIIDetectors(detectors []PIIDetector) Option {
	return func(p *SpanProcessor) {
		p.piiDetectors = detectors
	}
}

// WithBatchFlushThreshold sets the threshold for flushing accumulated records.
// When any accumulator map exceeds this size, a flush is triggered.
// A value of 0 (default) means all spans are accumulated before a single flush.
func WithBatchFlushThreshold(n int) Option {
	return func(p *SpanProcessor) {
		p.batchFlushThreshold = n
	}
}

// networkHandler handles NETWORK spans.
type networkHandler struct{}

func (networkHandler) DestinationAppItemType() models.AppItemType {
	return models.AppItemTypeService
}

func (networkHandler) ComponentInfo(attrs map[string]string) (models.ComponentType, string) {
	return models.ComponentTypeEndpoint, attrs[models.AttrHTTPPath]
}

func (networkHandler) PIIFields(attrs map[string]string) []string {
	return []string{attrs[models.AttrHTTPReqBody], attrs[models.AttrHTTPRespBody]}
}

// queryHandler handles QUERY spans.
type queryHandler struct{}

func (queryHandler) DestinationAppItemType() models.AppItemType {
	return models.AppItemTypeDatabase
}

func (queryHandler) ComponentInfo(attrs map[string]string) (models.ComponentType, string) {
	return models.ComponentTypeQuery, attrs[models.AttrDBQuery]
}

func (queryHandler) PIIFields(attrs map[string]string) []string {
	return []string{attrs[models.AttrDBQueryValues]}
}

// messageHandler handles MESSAGE spans.
type messageHandler struct{}

func (messageHandler) DestinationAppItemType() models.AppItemType {
	return models.AppItemTypeQueue
}

func (messageHandler) ComponentInfo(attrs map[string]string) (models.ComponentType, string) {
	return models.ComponentTypeQueue, attrs[models.AttrQueueTopic]
}

func (messageHandler) PIIFields(attrs map[string]string) []string {
	return []string{attrs[models.AttrQueuePayload]}
}

// defaultHandlers returns the built-in span type handler registry.
func defaultHandlers() map[string]SpanTypeHandler {
	return map[string]SpanTypeHandler{
		"NETWORK": networkHandler{},
		"QUERY":   queryHandler{},
		"MESSAGE": messageHandler{},
	}
}
