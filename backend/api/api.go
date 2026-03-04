package api

import (
	"github.com/alma/assignment/db"
)

type APIBackend struct {
	db *db.DB
}

func New(database *db.DB) *APIBackend {
	return &APIBackend{db: database}
}

// TODO: Implement GetCatalog

// TODO: Implement GetConnections
