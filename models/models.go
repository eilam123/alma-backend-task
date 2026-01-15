package models

import "time"

type RawSpan struct {
	Id         string            `json:"id"`
	Kind       string            `json:"kind"`
	Name       string            `json:"name"`
	StartAt    time.Time         `json:"start_at"`
	EndAt      time.Time         `json:"end_at"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type RawSpansFile struct {
	Spans []RawSpan `json:"spans"`
}
