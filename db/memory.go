package db

import (
	"context"
	"fmt"
	"math/rand"
	"reflect"
	"sync"
	"time"
)

// simulateLatency adds artificial delay to mimic real DB round-trips.
func simulateLatency(write bool) {
	base := 10 * time.Millisecond // read baseline
	if write {
		base = 30 * time.Millisecond // write baseline
	}
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	time.Sleep(base + jitter)
}

type FieldType string

const (
	FieldTypeString FieldType = "string"
	FieldTypeInt    FieldType = "int"
	FieldTypeBool   FieldType = "bool"
	FieldTypeTime   FieldType = "time"
	FieldTypeJSON   FieldType = "json"
)

// ConflictAction specifies what to do when a record with the same primary key already exists.
type ConflictAction int

const (
	// ConflictError returns an error if a record with the same primary key exists (default behavior).
	ConflictError ConflictAction = iota
	// ConflictDoNothing ignores the insert if a record with the same primary key exists.
	ConflictDoNothing
	// ConflictDoUpdate updates the existing record with the new values.
	ConflictDoUpdate
)

// MergeFunc is a function that merges a new value into an existing value.
// It receives the existing value and the new value, and returns the merged result.
type MergeFunc func(existing, new any) any

// ConflictOptions specifies how to handle conflicts during insert operations.
type ConflictOptions struct {
	// Action specifies what to do on conflict.
	Action ConflictAction
	// UpdateFields specifies which fields to update when Action is ConflictDoUpdate.
	// If empty, all fields from the new record will be used to update.
	UpdateFields []string
	// MergeFuncs specifies custom merge functions for specific fields.
	// When a field has a MergeFunc, it will be called with (existingValue, newValue)
	// to compute the final value. This is useful for operations like incrementing counters.
	MergeFuncs map[string]MergeFunc
}

type Field struct {
	Name     string
	Type     FieldType
	Nullable bool
}

type TableSchema struct {
	Name       string
	Fields     []Field
	PrimaryKey string
	Indexes    []string
}

type Record map[string]any

type Table struct {
	schema  TableSchema
	records map[string]Record
	indexes map[string]map[any][]string
	mu      sync.RWMutex
}

type DB struct {
	tables map[string]*Table
	mu     sync.RWMutex
}

func New() *DB {
	return &DB{
		tables: make(map[string]*Table),
	}
}

func (db *DB) CreateTable(ctx context.Context, schema TableSchema) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.tables[schema.Name]; exists {
		return fmt.Errorf("table %s already exists", schema.Name)
	}

	table := &Table{
		schema:  schema,
		records: make(map[string]Record),
		indexes: make(map[string]map[any][]string),
	}

	for _, idx := range schema.Indexes {
		table.indexes[idx] = make(map[any][]string)
	}

	db.tables[schema.Name] = table
	return nil
}

func (db *DB) DropTable(ctx context.Context, name string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if _, exists := db.tables[name]; !exists {
		return fmt.Errorf("table %s does not exist", name)
	}

	delete(db.tables, name)
	return nil
}

func (db *DB) Insert(ctx context.Context, tableName string, record Record) error {
	simulateLatency(true)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	pk, ok := record[table.schema.PrimaryKey]
	if !ok {
		return fmt.Errorf("primary key %s not found in record", table.schema.PrimaryKey)
	}

	pkStr := fmt.Sprintf("%v", pk)
	table.records[pkStr] = record

	for idxField := range table.indexes {
		if val, ok := record[idxField]; ok {
			table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
		}
	}

	return nil
}

// InsertOnConflict inserts a record with configurable conflict handling, similar to PostgreSQL's ON CONFLICT.
func (db *DB) InsertOnConflict(ctx context.Context, tableName string, record Record, opts ConflictOptions) error {
	simulateLatency(true)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	pk, ok := record[table.schema.PrimaryKey]
	if !ok {
		return fmt.Errorf("primary key %s not found in record", table.schema.PrimaryKey)
	}

	pkStr := fmt.Sprintf("%v", pk)
	existing, exists := table.records[pkStr]

	if exists {
		switch opts.Action {
		case ConflictError:
			return fmt.Errorf("record with primary key %v already exists", pk)
		case ConflictDoNothing:
			return nil
		case ConflictDoUpdate:
			fieldsToUpdate := opts.UpdateFields
			if len(fieldsToUpdate) == 0 {
				// Update all fields from the new record
				for k := range record {
					fieldsToUpdate = append(fieldsToUpdate, k)
				}
			}
			for _, field := range fieldsToUpdate {
				newVal, hasNew := record[field]
				if !hasNew {
					continue
				}
				// Check if there's a custom merge function for this field
				if opts.MergeFuncs != nil {
					if mergeFn, hasMerge := opts.MergeFuncs[field]; hasMerge {
						finalVal := mergeFn(existing[field], newVal)
						// Fix stale index if this is an indexed field
						if idx, isIndexed := table.indexes[field]; isIndexed {
							if existing[field] != finalVal {
								removeFromIndex(idx, existing[field], pkStr)
								idx[finalVal] = append(idx[finalVal], pkStr)
							}
						}
						existing[field] = finalVal
						continue
					}
				}
				// Fix stale index if this is an indexed field
				if idx, isIndexed := table.indexes[field]; isIndexed {
					if oldVal, hasOld := existing[field]; hasOld && oldVal != newVal {
						removeFromIndex(idx, oldVal, pkStr)
						idx[newVal] = append(idx[newVal], pkStr)
					}
				}
				existing[field] = newVal
			}
			return nil
		}
	}

	// Record doesn't exist, insert it
	table.records[pkStr] = record

	for idxField := range table.indexes {
		if val, ok := record[idxField]; ok {
			table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
		}
	}

	return nil
}

func (db *DB) Upsert(ctx context.Context, tableName string, record Record) error {
	simulateLatency(true)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	pk, ok := record[table.schema.PrimaryKey]
	if !ok {
		return fmt.Errorf("primary key %s not found in record", table.schema.PrimaryKey)
	}

	pkStr := fmt.Sprintf("%v", pk)

	if existing, exists := table.records[pkStr]; exists {
		// Remove stale index entries before updating
		for idxField := range table.indexes {
			if newVal, hasNew := record[idxField]; hasNew {
				if oldVal, hasOld := existing[idxField]; hasOld && oldVal != newVal {
					removeFromIndex(table.indexes[idxField], oldVal, pkStr)
					table.indexes[idxField][newVal] = append(table.indexes[idxField][newVal], pkStr)
				}
			}
		}
		for k, v := range record {
			existing[k] = v
		}
	} else {
		table.records[pkStr] = record
		for idxField := range table.indexes {
			if val, ok := record[idxField]; ok {
				table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
			}
		}
	}

	return nil
}

// removeFromIndex removes a primary key from an index entry.
func removeFromIndex(index map[any][]string, val any, pk string) {
	pks := index[val]
	for i, p := range pks {
		if p == pk {
			index[val] = append(pks[:i], pks[i+1:]...)
			break
		}
	}
	if len(index[val]) == 0 {
		delete(index, val)
	}
}

// InsertBatch inserts multiple records in a single atomic operation.
// All records must be valid - if any record fails validation, no records are inserted.
// If a record with the same primary key already exists, it will be overwritten.
func (db *DB) InsertBatch(ctx context.Context, tableName string, records []Record) error {
	simulateLatency(true)
	if len(records) == 0 {
		return nil
	}

	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	// Validate all records first before making any changes
	for i, record := range records {
		if _, ok := record[table.schema.PrimaryKey]; !ok {
			return fmt.Errorf("record %d: primary key %s not found in record", i, table.schema.PrimaryKey)
		}
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	// All validated, now insert all records
	for _, record := range records {
		pk := record[table.schema.PrimaryKey]
		pkStr := fmt.Sprintf("%v", pk)

		if existing, exists := table.records[pkStr]; exists {
			// Remove stale index entries before overwriting
			for idxField := range table.indexes {
				if oldVal, hasOld := existing[idxField]; hasOld {
					removeFromIndex(table.indexes[idxField], oldVal, pkStr)
				}
			}
		}

		table.records[pkStr] = record

		for idxField := range table.indexes {
			if val, ok := record[idxField]; ok {
				table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
			}
		}
	}

	return nil
}

// InsertBatchOnConflict inserts multiple records with configurable conflict handling.
// All records must be valid - if any record fails validation, no records are inserted.
func (db *DB) InsertBatchOnConflict(ctx context.Context, tableName string, records []Record, opts ConflictOptions) error {
	simulateLatency(true)
	if len(records) == 0 {
		return nil
	}

	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	// Validate all records first before making any changes
	for i, record := range records {
		if _, ok := record[table.schema.PrimaryKey]; !ok {
			return fmt.Errorf("record %d: primary key %s not found in record", i, table.schema.PrimaryKey)
		}
	}

	// For ConflictError action, check for conflicts before making any changes
	if opts.Action == ConflictError {
		table.mu.RLock()
		for i, record := range records {
			pk := record[table.schema.PrimaryKey]
			pkStr := fmt.Sprintf("%v", pk)
			if _, exists := table.records[pkStr]; exists {
				table.mu.RUnlock()
				return fmt.Errorf("record %d: record with primary key %v already exists", i, pk)
			}
		}
		table.mu.RUnlock()
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	for _, record := range records {
		pk := record[table.schema.PrimaryKey]
		pkStr := fmt.Sprintf("%v", pk)
		existing, exists := table.records[pkStr]

		if exists {
			switch opts.Action {
			case ConflictDoNothing:
				continue
			case ConflictDoUpdate:
				fieldsToUpdate := opts.UpdateFields
				if len(fieldsToUpdate) == 0 {
					for k := range record {
						fieldsToUpdate = append(fieldsToUpdate, k)
					}
				}
				for _, field := range fieldsToUpdate {
					newVal, hasNew := record[field]
					if !hasNew {
						continue
					}
					if opts.MergeFuncs != nil {
						if mergeFn, hasMerge := opts.MergeFuncs[field]; hasMerge {
							finalVal := mergeFn(existing[field], newVal)
							if idx, isIndexed := table.indexes[field]; isIndexed {
								if existing[field] != finalVal {
									removeFromIndex(idx, existing[field], pkStr)
									idx[finalVal] = append(idx[finalVal], pkStr)
								}
							}
							existing[field] = finalVal
							continue
						}
					}
					if idx, isIndexed := table.indexes[field]; isIndexed {
						if oldVal, hasOld := existing[field]; hasOld && oldVal != newVal {
							removeFromIndex(idx, oldVal, pkStr)
							idx[newVal] = append(idx[newVal], pkStr)
						}
					}
					existing[field] = newVal
				}
				continue
			}
		}

		// Record doesn't exist, insert it
		table.records[pkStr] = record

		for idxField := range table.indexes {
			if val, ok := record[idxField]; ok {
				table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
			}
		}
	}

	return nil
}

// UpsertBatch upserts multiple records in a single atomic operation.
// All records must be valid - if any record fails validation, no records are upserted.
// For existing records, it merges the new values into the existing record.
// For new records, it inserts them as new entries.
func (db *DB) UpsertBatch(ctx context.Context, tableName string, records []Record) error {
	simulateLatency(true)
	if len(records) == 0 {
		return nil
	}

	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	// Validate all records first before making any changes
	for i, record := range records {
		if _, ok := record[table.schema.PrimaryKey]; !ok {
			return fmt.Errorf("record %d: primary key %s not found in record", i, table.schema.PrimaryKey)
		}
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	// All validated, now upsert all records
	for _, record := range records {
		pk := record[table.schema.PrimaryKey]
		pkStr := fmt.Sprintf("%v", pk)

		if existing, exists := table.records[pkStr]; exists {
			// Remove stale index entries before updating
			for idxField := range table.indexes {
				if newVal, hasNew := record[idxField]; hasNew {
					if oldVal, hasOld := existing[idxField]; hasOld && oldVal != newVal {
						removeFromIndex(table.indexes[idxField], oldVal, pkStr)
						table.indexes[idxField][newVal] = append(table.indexes[idxField][newVal], pkStr)
					}
				}
			}
			// Merge: update existing record with new values
			for k, v := range record {
				existing[k] = v
			}
		} else {
			// Insert new record
			table.records[pkStr] = record
			for idxField := range table.indexes {
				if val, ok := record[idxField]; ok {
					table.indexes[idxField][val] = append(table.indexes[idxField][val], pkStr)
				}
			}
		}
	}

	return nil
}

func (db *DB) Get(ctx context.Context, tableName string, pk any) (Record, error) {
	simulateLatency(false)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.RLock()
	defer table.mu.RUnlock()

	pkStr := fmt.Sprintf("%v", pk)
	record, exists := table.records[pkStr]
	if !exists {
		return nil, fmt.Errorf("record with pk %v not found", pk)
	}

	return record, nil
}

func (db *DB) Delete(ctx context.Context, tableName string, pk any) error {
	simulateLatency(true)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.Lock()
	defer table.mu.Unlock()

	pkStr := fmt.Sprintf("%v", pk)

	// Remove index entries before deleting the record
	if existing, exists := table.records[pkStr]; exists {
		for idxField := range table.indexes {
			if val, hasVal := existing[idxField]; hasVal {
				removeFromIndex(table.indexes[idxField], val, pkStr)
			}
		}
	}

	delete(table.records, pkStr)
	return nil
}

func (db *DB) Select(ctx context.Context, tableName string) *QueryBuilder {
	return &QueryBuilder{
		db:        db,
		tableName: tableName,
		filters:   make(map[string]any),
	}
}

type QueryBuilder struct {
	db        *DB
	tableName string
	filters   map[string]any
	orderBy   string
	orderDesc bool
	limit     int
}

func (q *QueryBuilder) Where(field string, value any) *QueryBuilder {
	q.filters[field] = value
	return q
}

func (q *QueryBuilder) OrderBy(field string, desc bool) *QueryBuilder {
	q.orderBy = field
	q.orderDesc = desc
	return q
}

func (q *QueryBuilder) Limit(n int) *QueryBuilder {
	q.limit = n
	return q
}

func (q *QueryBuilder) Execute(ctx context.Context) ([]Record, error) {
	simulateLatency(false)
	q.db.mu.RLock()
	table, exists := q.db.tables[q.tableName]
	q.db.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("table %s does not exist", q.tableName)
	}

	table.mu.RLock()
	defer table.mu.RUnlock()

	// Try to use an index to narrow down candidates
	candidates := q.indexCandidates(table)

	var results []Record

	if candidates != nil {
		// Index-accelerated path: only check candidate records
		for _, pkStr := range candidates {
			record, ok := table.records[pkStr]
			if ok && q.matchesFilters(record) {
				results = append(results, record)
			}
		}
	} else {
		// Full scan fallback
		for _, record := range table.records {
			if q.matchesFilters(record) {
				results = append(results, record)
			}
		}
	}

	if q.limit > 0 && len(results) > q.limit {
		results = results[:q.limit]
	}

	return results, nil
}

// indexCandidates returns candidate PKs from an index if any filter field is indexed.
// Returns nil if no index can be used.
func (q *QueryBuilder) indexCandidates(table *Table) []string {
	for field, value := range q.filters {
		if idx, ok := table.indexes[field]; ok {
			if pks, found := idx[value]; found {
				return pks
			}
			return []string{} // index exists but no matching value
		}
	}
	return nil
}

func (q *QueryBuilder) matchesFilters(record Record) bool {
	for field, expected := range q.filters {
		actual, exists := record[field]
		if !exists {
			return false
		}
		if !reflect.DeepEqual(actual, expected) {
			return false
		}
	}
	return true
}

func (db *DB) Count(ctx context.Context, tableName string) (int, error) {
	simulateLatency(false)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return 0, fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.RLock()
	defer table.mu.RUnlock()

	return len(table.records), nil
}

func (db *DB) All(ctx context.Context, tableName string) ([]Record, error) {
	return db.Select(ctx, tableName).Execute(ctx)
}

// AllGroupedBy returns all records in a table grouped by the given field value.
// If the field is an indexed field, it uses the index for O(1) grouping.
func (db *DB) AllGroupedBy(ctx context.Context, tableName string, field string) (map[any][]Record, error) {
	simulateLatency(false)
	db.mu.RLock()
	table, exists := db.tables[tableName]
	db.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("table %s does not exist", tableName)
	}

	table.mu.RLock()
	defer table.mu.RUnlock()

	// If field is indexed, use the index for grouping
	if idx, hasIndex := table.indexes[field]; hasIndex {
		result := make(map[any][]Record, len(idx))
		for val, pks := range idx {
			records := make([]Record, 0, len(pks))
			for _, pk := range pks {
				if rec, ok := table.records[pk]; ok {
					records = append(records, rec)
				}
			}
			if len(records) > 0 {
				result[val] = records
			}
		}
		return result, nil
	}

	// Fallback: full scan
	result := make(map[any][]Record)
	for _, rec := range table.records {
		if val, ok := rec[field]; ok {
			result[val] = append(result[val], rec)
		}
	}
	return result, nil
}

func (db *DB) Clear(ctx context.Context) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.tables = make(map[string]*Table)
	return nil
}

func (db *DB) TableExists(ctx context.Context, name string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	_, exists := db.tables[name]
	return exists
}

type JoinType string

const (
	InnerJoin JoinType = "INNER"
	LeftJoin  JoinType = "LEFT"
)

type JoinBuilder struct {
	db           *DB
	leftTable    string
	rightTable   string
	leftField    string
	rightField   string
	joinType     JoinType
	filters      map[string]any
	selectFields []string
}

func (db *DB) Join(leftTable, rightTable string) *JoinBuilder {
	return &JoinBuilder{
		db:         db,
		leftTable:  leftTable,
		rightTable: rightTable,
		joinType:   InnerJoin,
		filters:    make(map[string]any),
	}
}

func (j *JoinBuilder) On(leftField, rightField string) *JoinBuilder {
	j.leftField = leftField
	j.rightField = rightField
	return j
}

func (j *JoinBuilder) Type(joinType JoinType) *JoinBuilder {
	j.joinType = joinType
	return j
}

func (j *JoinBuilder) Where(field string, value any) *JoinBuilder {
	j.filters[field] = value
	return j
}

func (j *JoinBuilder) Fields(fields ...string) *JoinBuilder {
	j.selectFields = fields
	return j
}

func (j *JoinBuilder) Execute(ctx context.Context) ([]Record, error) {
	simulateLatency(false)
	j.db.mu.RLock()
	leftTbl, leftExists := j.db.tables[j.leftTable]
	rightTbl, rightExists := j.db.tables[j.rightTable]
	j.db.mu.RUnlock()

	if !leftExists {
		return nil, fmt.Errorf("table %s does not exist", j.leftTable)
	}
	if !rightExists {
		return nil, fmt.Errorf("table %s does not exist", j.rightTable)
	}

	leftTbl.mu.RLock()
	rightTbl.mu.RLock()
	defer leftTbl.mu.RUnlock()
	defer rightTbl.mu.RUnlock()

	rightIndex := make(map[any][]Record)
	for _, record := range rightTbl.records {
		if val, ok := record[j.rightField]; ok {
			rightIndex[val] = append(rightIndex[val], record)
		}
	}

	var results []Record

	for _, leftRec := range leftTbl.records {
		leftVal, ok := leftRec[j.leftField]
		if !ok {
			if j.joinType == LeftJoin {
				merged := j.mergeRecords(leftRec, nil)
				if j.matchesFilters(merged) {
					results = append(results, merged)
				}
			}
			continue
		}

		rightRecs, found := rightIndex[leftVal]
		if !found {
			if j.joinType == LeftJoin {
				merged := j.mergeRecords(leftRec, nil)
				if j.matchesFilters(merged) {
					results = append(results, merged)
				}
			}
			continue
		}

		for _, rightRec := range rightRecs {
			merged := j.mergeRecords(leftRec, rightRec)
			if j.matchesFilters(merged) {
				results = append(results, merged)
			}
		}
	}

	return results, nil
}

func (j *JoinBuilder) mergeRecords(left, right Record) Record {
	result := make(Record)

	for k, v := range left {
		key := j.leftTable + "." + k
		result[key] = v
	}

	for k, v := range right {
		key := j.rightTable + "." + k
		result[key] = v
	}

	return result
}

func (j *JoinBuilder) matchesFilters(record Record) bool {
	for field, expected := range j.filters {
		actual, exists := record[field]
		if !exists {
			return false
		}
		if !reflect.DeepEqual(actual, expected) {
			return false
		}
	}
	return true
}
