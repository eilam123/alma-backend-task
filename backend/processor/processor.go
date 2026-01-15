package processor

import (
	"context"

	"github.com/alma/assignment/models"
)

type SpanProcessor struct{}

func New() *SpanProcessor {
	return &SpanProcessor{}
}

// TODO: Implement Process
func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error {
	return nil
}
