package schema

import (
	"context"
	"fmt"

	"github.com/alma/assignment/db"
)

// CreateSchema sets up the database tables for app items, components, PIIs, and connections.
func CreateSchema(ctx context.Context, database db.Database) error {
	if err := database.CreateTable(ctx, db.TableSchema{
		Name: "app_items",
		Fields: []db.Field{
			{Name: "name", Type: db.FieldTypeString},
			{Name: "type", Type: db.FieldTypeString},
		},
		PrimaryKey: "name",
		Indexes:    []string{"type"},
	}); err != nil {
		return fmt.Errorf("create app_items table: %w", err)
	}

	if err := database.CreateTable(ctx, db.TableSchema{
		Name: "components",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "app_item_name", Type: db.FieldTypeString},
			{Name: "component_type", Type: db.FieldTypeString},
			{Name: "value", Type: db.FieldTypeString},
		},
		PrimaryKey: "id",
		Indexes:    []string{"app_item_name", "component_type"},
	}); err != nil {
		return fmt.Errorf("create components table: %w", err)
	}

	if err := database.CreateTable(ctx, db.TableSchema{
		Name: "component_piis",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "component_id", Type: db.FieldTypeString},
			{Name: "pii_type", Type: db.FieldTypeString},
		},
		PrimaryKey: "id",
		Indexes:    []string{"component_id", "pii_type"},
	}); err != nil {
		return fmt.Errorf("create component_piis table: %w", err)
	}

	if err := database.CreateTable(ctx, db.TableSchema{
		Name: "connections",
		Fields: []db.Field{
			{Name: "id", Type: db.FieldTypeString},
			{Name: "source", Type: db.FieldTypeString},
			{Name: "destination", Type: db.FieldTypeString},
			{Name: "component_ids", Type: db.FieldTypeJSON},
		},
		PrimaryKey: "id",
		Indexes:    []string{"source", "destination"},
	}); err != nil {
		return fmt.Errorf("create connections table: %w", err)
	}

	return nil
}
