package processor

import (
	"context"

	"github.com/alma/assignment/db"
	"github.com/alma/assignment/models"
)

type SpanProcessor struct {
	db *db.DB
}

func New(database *db.DB) *SpanProcessor {
	return &SpanProcessor{db: database}
}

// TODO: Implement Process
func (p *SpanProcessor) Process(ctx context.Context, rawSpans []models.RawSpan) error {
	return nil
}
