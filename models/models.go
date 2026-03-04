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

type AppItemType string

const (
	AppItemTypeInternet  AppItemType = "INTERNET"
	AppItemTypeService   AppItemType = "SERVICE"
	AppItemTypeDatabase  AppItemType = "DATABASE"
	AppItemTypeQueue     AppItemType = "QUEUE"
)

type ComponentType string

const (
	ComponentTypeEndpoint ComponentType = "ENDPOINT"
	ComponentTypeQueue    ComponentType = "QUEUE"
	ComponentTypeQuery    ComponentType = "QUERY"
)

type PIIType string

const (
	PIITypeEmail       PIIType = "EMAIL"
	PIITypeCreditCard  PIIType = "CREDIT_CARD"
	PIITypeSSN         PIIType = "SSN"
	PIITypePhone       PIIType = "PHONE"
	PIITypeIPAddress   PIIType = "IP_ADDRESS"
)

type Component struct {
	ID            string    `json:"id"`
	AppItemName   string    `json:"app_item_name"`
	ComponentType ComponentType `json:"component_type"`
	Value         string    `json:"value"`
	PIIs          []PIIType `json:"piis"`
}

type AppItem struct {
	Name       string      `json:"name"`
	Type       AppItemType `json:"type"`
	Components []Component `json:"components"`
}

type Connection struct {
	Source      string      `json:"source"`
	Destination string      `json:"destination"`
	Components  []Component `json:"components"`
}

type Catalog struct {
	AppItems []AppItem `json:"app_items"`
}
