package db

import "context"

// Database defines the interface for database operations.
// This interface allows for dependency injection and easier testing.
type Database interface {
	// Table management
	CreateTable(ctx context.Context, schema TableSchema) error
	DropTable(ctx context.Context, name string) error
	TableExists(ctx context.Context, name string) bool
	Clear(ctx context.Context) error

	// Single record operations
	Insert(ctx context.Context, tableName string, record Record) error
	Upsert(ctx context.Context, tableName string, record Record) error
	Get(ctx context.Context, tableName string, pk any) (Record, error)
	Delete(ctx context.Context, tableName string, pk any) error

	// Batch operations (preferred for efficiency)
	InsertBatch(ctx context.Context, tableName string, records []Record) error
	UpsertBatch(ctx context.Context, tableName string, records []Record) error

	// Conflict operations
	InsertOnConflict(ctx context.Context, tableName string, record Record, opts ConflictOptions) error
	InsertBatchOnConflict(ctx context.Context, tableName string, records []Record, opts ConflictOptions) error

	// Query operations
	Select(ctx context.Context, tableName string) *QueryBuilder
	All(ctx context.Context, tableName string) ([]Record, error)
	Count(ctx context.Context, tableName string) (int, error)

	// Join operations
	Join(leftTable, rightTable string) *JoinBuilder
}

// Ensure DB implements Database interface
var _ Database = (*DB)(nil)
