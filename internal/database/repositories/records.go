// Package repositories contains data access layer implementations.
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hypergoat/internal/atproto"
	"github.com/GainForest/hypergoat/internal/database"
)

// Batch size constants for SQL operations.
const (
	// BatchInsertSize is the number of records per INSERT batch (5 params each = 500 SQL params).
	BatchInsertSize = 100

	// SQLParamBatchSize is the batch size for IN-clause queries, kept under SQLite's 999 param limit.
	SQLParamBatchSize = 900

	// DefaultIterateBatchSize is the default batch size for IterateAll when none specified.
	DefaultIterateBatchSize = 1000

	// SearchTimeout is the maximum duration for a search query.
	SearchTimeout = 10 * time.Second

	// MaxINListSize is the maximum number of values allowed in an IN filter clause.
	// SQLite has a hard 999 parameter limit (SQLITE_MAX_VARIABLE_NUMBER).
	// We cap well below that to leave room for other query parameters.
	MaxINListSize = 100

	// MaxFilterConditions is the maximum number of individual filter conditions allowed per query.
	// The DID filter does not count toward this cap.
	MaxFilterConditions = 20
)

// Record represents an AT Protocol record stored in the database.
type Record struct {
	URI        string
	CID        string
	DID        string
	Collection string
	JSON       string
	IndexedAt  time.Time
	RKey       string
}

// FieldFilter represents a single filter condition on a JSON field.
type FieldFilter struct {
	Field     string      // JSON field name (e.g., "title", "createdAt"). Must be a valid field name.
	Operator  string      // One of: "eq", "neq", "gt", "lt", "gte", "lte", "in", "contains", "startsWith", "isNull"
	Value     interface{} // The comparison value. For "in", must be []interface{}. For "isNull", must be bool.
	FieldType string      // Lexicon type: "string", "integer", "number", "boolean", "datetime"
}

// DIDFilter represents a filter on the did column.
// Only eq and in are supported (column-level filter, not JSON).
// If both EQ and IN are set, EQ takes precedence.
type DIDFilter struct {
	EQ string   // Exact match (empty means no eq filter)
	IN []string // In list (nil/empty means no in filter)
}

// IsEmpty reports whether the DIDFilter has no conditions set.
func (d DIDFilter) IsEmpty() bool {
	return d.EQ == "" && len(d.IN) == 0
}

// SortOption specifies a sort field and direction for record queries.
type SortOption struct {
	Field     string // Field name. If "indexed_at", "uri", "did", "collection", "cid", "rkey" — use column directly. Otherwise, use JSONExtract.
	Direction string // "ASC" or "DESC"
}

// CollectionStat represents statistics for a collection.
type CollectionStat struct {
	Collection string
	Count      int64
}

// TimeSeriesDataPoint represents a single data point in a time series.
type TimeSeriesDataPoint struct {
	Date       string // YYYY-MM-DD format
	Count      int64
	Cumulative int64
}

// CollectionTimeSeries represents time series data for a collection.
type CollectionTimeSeries struct {
	Collection   string
	TotalRecords int64
	UniqueUsers  int64
	Data         []TimeSeriesDataPoint
}

// InsertResult indicates whether a record was inserted or skipped.
type InsertResult int

const (
	Inserted InsertResult = iota
	Skipped
)

// RecordsRepository handles record persistence.
type RecordsRepository struct {
	db database.Executor
}

// NewRecordsRepository creates a new records repository.
func NewRecordsRepository(db database.Executor) *RecordsRepository {
	return &RecordsRepository{db: db}
}

// recordColumns returns the columns to select based on dialect.
func (r *RecordsRepository) recordColumns() string {
	switch r.db.Dialect() {
	case database.PostgreSQL:
		return "uri, cid, did, collection, json::text, indexed_at::text, rkey"
	default:
		return "uri, cid, did, collection, json, indexed_at, rkey"
	}
}

// Insert inserts or updates a record in the database.
// Skips if the CID already exists (content unchanged).
func (r *RecordsRepository) Insert(ctx context.Context, uri, cid, did, collection, jsonData string) (InsertResult, error) {
	// Check if URI exists with same CID
	existingCID, err := r.getCIDByURI(ctx, uri)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Skipped, err
	}

	// Only skip if both the existing and incoming CID are non-empty and match.
	// If cid == "" (omitted by Tap for some events), always proceed with the
	// upsert so new records aren't silently dropped by the "" == "" comparison.
	if cid != "" && existingCID == cid {
		return Skipped, nil // Content unchanged
	}

	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	p3 := r.db.Placeholder(3)
	p4 := r.db.Placeholder(4)
	p5 := r.db.Placeholder(5)

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES (%s, %s, %s, %s, %s::jsonb)
			ON CONFLICT(uri) DO UPDATE SET
				cid = EXCLUDED.cid,
				json = EXCLUDED.json,
				indexed_at = NOW()`, p1, p2, p3, p4, p5)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES (%s, %s, %s, %s, %s)
			ON CONFLICT(uri) DO UPDATE SET
				cid = excluded.cid,
				json = excluded.json,
				indexed_at = datetime('now')`, p1, p2, p3, p4, p5)
	}

	_, err = r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(uri),
		database.Text(cid),
		database.Text(did),
		database.Text(collection),
		database.Text(jsonData),
	})
	if err != nil {
		return Skipped, err
	}

	return Inserted, nil
}

// BatchInsert inserts multiple records efficiently.
// Wraps all batch inserts in a single transaction for better performance.
func (r *RecordsRepository) BatchInsert(ctx context.Context, records []*Record) error {
	if len(records) == 0 {
		return nil
	}

	// Start transaction for all batches
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback is a no-op if Commit succeeds

	// Process in batches to stay within SQL parameter limits
	batchSize := BatchInsertSize
	for i := 0; i < len(records); i += batchSize {
		end := i + batchSize
		if end > len(records) {
			end = len(records)
		}
		batch := records[i:end]

		if err := r.insertBatchTx(ctx, tx, batch); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// insertBatchTx inserts a batch of records within a transaction.
func (r *RecordsRepository) insertBatchTx(ctx context.Context, tx *sql.Tx, records []*Record) error {
	// Build value placeholders
	var valueSets []string
	var args []any

	for i, rec := range records {
		base := i * 5
		var valueSet string

		if r.db.Dialect() == database.PostgreSQL {
			valueSet = fmt.Sprintf("(%s, %s, %s, %s, %s::jsonb)",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2),
				r.db.Placeholder(base+3),
				r.db.Placeholder(base+4),
				r.db.Placeholder(base+5))
		} else {
			valueSet = fmt.Sprintf("(%s, %s, %s, %s, %s)",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2),
				r.db.Placeholder(base+3),
				r.db.Placeholder(base+4),
				r.db.Placeholder(base+5))
		}
		valueSets = append(valueSets, valueSet)

		args = append(args, rec.URI, rec.CID, rec.DID, rec.Collection, rec.JSON)
	}

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES %s
			ON CONFLICT(uri) DO UPDATE SET
				cid = EXCLUDED.cid,
				json = EXCLUDED.json,
				indexed_at = NOW()`, strings.Join(valueSets, ", "))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES %s
			ON CONFLICT(uri) DO UPDATE SET
				cid = excluded.cid,
				json = excluded.json,
				indexed_at = datetime('now')`, strings.Join(valueSets, ", "))
	}

	_, err := tx.ExecContext(ctx, sqlStr, args...)
	return err
}

// GetByURI retrieves a record by its URI.
func (r *RecordsRepository) GetByURI(ctx context.Context, uri string) (*Record, error) {
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri = %s",
		r.recordColumns(), r.db.Placeholder(1))

	var rec Record
	var indexedAtStr string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(uri)},
		&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey)
	if err != nil {
		return nil, err
	}

	rec.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	return &rec, nil
}

// GetByURIs retrieves multiple records by their URIs.
func (r *RecordsRepository) GetByURIs(ctx context.Context, uris []string) ([]*Record, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	placeholders := r.db.Placeholders(len(uris), 1)
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri IN (%s)",
		r.recordColumns(), placeholders)

	params := make([]database.Value, len(uris))
	for i, uri := range uris {
		params[i] = database.Text(uri)
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// GetByCollection retrieves records for a specific collection.
func (r *RecordsRepository) GetByCollection(ctx context.Context, collection string, limit int) ([]*Record, error) {
	return r.GetByCollectionWithKeysetCursor(ctx, collection, limit, "", "")
}

// GetByCollectionWithCursor retrieves records for a specific collection with cursor-based pagination.
// The cursor is the indexed_at timestamp of the last record from the previous page.
// Records are ordered by indexed_at DESC (newest first) for chronological feed display.
func (r *RecordsRepository) GetByCollectionWithCursor(ctx context.Context, collection string, limit int, afterTimestamp string) ([]*Record, error) {
	var sqlStr string
	var args []any

	if afterTimestamp == "" {
		// No cursor - get first page, ordered by indexed_at DESC (newest first)
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s ORDER BY indexed_at DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), limit)
		args = []any{collection}
	} else {
		// With cursor - get records older than the cursor timestamp
		// Using indexed_at < cursor for "load more" (older posts)
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s AND indexed_at < %s ORDER BY indexed_at DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), r.db.Placeholder(2), limit)
		args = []any{collection, afterTimestamp}
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// GetByCollectionWithKeysetCursor retrieves records using deterministic keyset pagination.
// The cursor is a composite (indexed_at, uri) pair. Records are ordered by (indexed_at DESC, uri DESC).
// When afterTimestamp and afterURI are provided, returns records that sort after the cursor position.
func (r *RecordsRepository) GetByCollectionWithKeysetCursor(ctx context.Context, collection string, limit int, afterTimestamp, afterURI string) ([]*Record, error) {
	var sqlStr string
	var args []any

	if afterTimestamp == "" && afterURI == "" {
		// No cursor - get first page
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s ORDER BY indexed_at DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), limit)
		args = []any{collection}
	} else {
		// Keyset pagination: get records that sort after (afterTimestamp, afterURI)
		// ORDER BY indexed_at DESC, uri DESC means "after" = less than
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s AND (indexed_at < %s OR (indexed_at = %s AND uri < %s)) ORDER BY indexed_at DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4), limit)
		args = []any{collection, afterTimestamp, afterTimestamp, afterURI}
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// buildFilterClause builds a SQL WHERE clause fragment from a slice of FieldFilters.
// startPlaceholder is the 1-based index of the first placeholder to use.
// Returns the clause string (without leading "AND") and the parameter values.
// Returns an empty string and nil params if filters is empty.
func (r *RecordsRepository) buildFilterClause(filters []FieldFilter, startPlaceholder int) (string, []database.Value, error) {
	if len(filters) == 0 {
		return "", nil, nil
	}

	var conditions []string
	var params []database.Value
	placeholderIdx := startPlaceholder

	for _, f := range filters {
		extract := r.db.JSONExtract("json", f.Field)

		// Wrap numeric types in a CAST for proper comparison
		isNumeric := f.FieldType == "integer" || f.FieldType == "number"
		if isNumeric {
			switch r.db.Dialect() {
			case database.PostgreSQL:
				extract = fmt.Sprintf("(%s)::numeric", extract)
			default:
				extract = fmt.Sprintf("CAST(%s AS REAL)", extract)
			}
		}

		switch f.Operator {
		case "eq":
			conditions = append(conditions, fmt.Sprintf("%s = %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "neq":
			conditions = append(conditions, fmt.Sprintf("%s != %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "gt":
			conditions = append(conditions, fmt.Sprintf("%s > %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "lt":
			conditions = append(conditions, fmt.Sprintf("%s < %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "gte":
			conditions = append(conditions, fmt.Sprintf("%s >= %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "lte":
			conditions = append(conditions, fmt.Sprintf("%s <= %s", extract, r.db.Placeholder(placeholderIdx)))
			params = append(params, toDBValue(f.Value))
			placeholderIdx++
		case "contains":
			likeOp := "LIKE"
			if r.db.Dialect() == database.PostgreSQL {
				likeOp = "ILIKE"
			}
			conditions = append(conditions, fmt.Sprintf("%s %s %s ESCAPE '\\'", extract, likeOp, r.db.Placeholder(placeholderIdx)))
			val := fmt.Sprintf("%%%s%%", escapeLIKE(fmt.Sprintf("%v", f.Value)))
			params = append(params, database.Text(val))
			placeholderIdx++
		case "startsWith":
			likeOp := "LIKE"
			if r.db.Dialect() == database.PostgreSQL {
				likeOp = "ILIKE"
			}
			conditions = append(conditions, fmt.Sprintf("%s %s %s ESCAPE '\\'", extract, likeOp, r.db.Placeholder(placeholderIdx)))
			val := fmt.Sprintf("%s%%", escapeLIKE(fmt.Sprintf("%v", f.Value)))
			params = append(params, database.Text(val))
			placeholderIdx++
		case "isNull":
			isNull, _ := f.Value.(bool)
			if isNull {
				conditions = append(conditions, fmt.Sprintf("%s IS NULL", extract))
			} else {
				conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", extract))
			}
		case "in":
			inVals, _ := f.Value.([]interface{})
			if len(inVals) == 0 {
				// Empty IN list — always false
				conditions = append(conditions, "1 = 0")
				continue
			}
			if len(inVals) > MaxINListSize {
				return "", nil, fmt.Errorf("IN filter on field %q exceeds maximum of %d values", f.Field, MaxINListSize)
			}
			placeholders := r.db.Placeholders(len(inVals), placeholderIdx)
			conditions = append(conditions, fmt.Sprintf("%s IN (%s)", extract, placeholders))
			for _, v := range inVals {
				params = append(params, toDBValue(v))
				placeholderIdx++
			}
		}
	}

	return strings.Join(conditions, " AND "), params, nil
}

// buildDIDFilterClause builds a SQL WHERE clause fragment for a DIDFilter.
// startPlaceholder is the 1-based index of the first placeholder to use.
// Returns the clause string (without leading "AND"), the parameter values, and
// the number of placeholders consumed. Returns empty string and nil params if
// the filter is empty.
func (r *RecordsRepository) buildDIDFilterClause(f DIDFilter, startPlaceholder int) (string, []database.Value, int) {
	if f.IsEmpty() {
		return "", nil, 0
	}

	// EQ takes precedence over IN when both are set.
	if f.EQ != "" {
		clause := fmt.Sprintf("did = %s", r.db.Placeholder(startPlaceholder))
		return clause, []database.Value{database.Text(f.EQ)}, 1
	}

	// IN list
	if len(f.IN) == 0 {
		// Empty IN list — always false
		return "1 = 0", nil, 0
	}
	placeholders := r.db.Placeholders(len(f.IN), startPlaceholder)
	clause := fmt.Sprintf("did IN (%s)", placeholders)
	params := make([]database.Value, len(f.IN))
	for i, did := range f.IN {
		params[i] = database.Text(did)
	}
	return clause, params, len(f.IN)
}

// toDBValue converts an interface{} value to a database.Value.
func toDBValue(v interface{}) database.Value {
	switch val := v.(type) {
	case string:
		return database.Text(val)
	case int:
		return database.Int(int64(val))
	case int64:
		return database.Int(val)
	case float64:
		return database.Float(val)
	case bool:
		return database.Bool(val)
	case nil:
		return database.Null()
	default:
		return database.Text(fmt.Sprintf("%v", val))
	}
}

// GetByCollectionFilteredWithKeysetCursor retrieves records for a collection with
// optional field-level filters and keyset-based pagination.
// Filters are applied as AND conditions on JSON fields.
// If didFilter is non-empty, results are further filtered by DID.
// Records are ordered by (indexed_at DESC, uri DESC).
func (r *RecordsRepository) GetByCollectionFilteredWithKeysetCursor(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	limit int,
	afterTimestamp string,
	afterURI string,
) ([]*Record, error) {
	// Build the base WHERE clause
	// collection = ? is always param 1
	var whereParts []string
	var args []any

	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	args = append(args, collection)

	nextPlaceholder := 2

	// Keyset cursor condition
	if afterTimestamp != "" && afterURI != "" {
		p2 := r.db.Placeholder(nextPlaceholder)
		p3 := r.db.Placeholder(nextPlaceholder + 1)
		p4 := r.db.Placeholder(nextPlaceholder + 2)
		whereParts = append(whereParts, fmt.Sprintf("(indexed_at < %s OR (indexed_at = %s AND uri < %s))", p2, p3, p4))
		args = append(args, afterTimestamp, afterTimestamp, afterURI)
		nextPlaceholder += 3
	}

	// Field filters
	filterClause, filterParams, err := r.buildFilterClause(filters, nextPlaceholder)
	if err != nil {
		return nil, fmt.Errorf("failed to build filter clause: %w", err)
	}
	if filterClause != "" {
		whereParts = append(whereParts, filterClause)
		for _, p := range filterParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
			nextPlaceholder++
		}
	}

	// DID filter
	if didClause, didParams, _ := r.buildDIDFilterClause(didFilter, nextPlaceholder); didClause != "" {
		whereParts = append(whereParts, didClause)
		for _, p := range didParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY indexed_at DESC, uri DESC LIMIT %d",
		r.recordColumns(), whereClause, limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query filtered records: %w", err)
	}
	defer rows.Close()

	return scanRecords(rows)
}

// directSortColumns is the set of column names that can be used directly in ORDER BY
// without JSON extraction.
var directSortColumns = map[string]bool{
	"indexed_at": true,
	"uri":        true,
	"did":        true,
	"collection": true,
	"cid":        true,
	"rkey":       true,
}

// keysetSortFieldName returns the effective sort field name used by keyset pagination.
// Nil sort defaults to indexed_at DESC.
func keysetSortFieldName(sort *SortOption) string {
	if sort == nil {
		return "indexed_at"
	}
	return sort.Field
}

// keysetSortFieldExpr returns the SQL expression used on the record side for
// keyset comparisons.
func (r *RecordsRepository) keysetSortFieldExpr(sortField string) string {
	if sortField == "indexed_at" {
		switch r.db.Dialect() {
		case database.PostgreSQL:
			return "indexed_at"
		default:
			// SQLite stores indexed_at as TEXT and may contain mixed formats
			// (e.g. "YYYY-MM-DD HH:MM:SS" and RFC3339). Normalize to a canonical
			// sortable UTC representation before comparison.
			return "strftime('%Y-%m-%dT%H:%M:%fZ', indexed_at)"
		}
	}

	if directSortColumns[sortField] {
		return sortField
	}

	return r.db.JSONExtract("json", sortField)
}

// keysetCursorValueExpr returns the SQL expression used on the cursor side for
// keyset comparisons.
func (r *RecordsRepository) keysetCursorValueExpr(sortField, placeholder string) string {
	if sortField != "indexed_at" {
		return placeholder
	}

	switch r.db.Dialect() {
	case database.PostgreSQL:
		return fmt.Sprintf("%s::timestamptz", placeholder)
	default:
		return fmt.Sprintf("strftime('%%Y-%%m-%%dT%%H:%%M:%%fZ', %s)", placeholder)
	}
}

// buildSortExpr builds the ORDER BY expression for a given SortOption.
// If sort is nil, returns the default "indexed_at DESC, uri DESC".
// If sort.Field is a direct column, uses it as-is; otherwise uses JSONExtract.
// Always appends ", uri <direction>" as a tiebreaker unless the sort field IS uri.
// The uri tiebreaker direction matches the primary sort direction.
func (r *RecordsRepository) buildSortExpr(sort *SortOption) string {
	if sort == nil {
		return "indexed_at DESC, uri DESC"
	}

	dir := sort.Direction
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}

	var fieldExpr string
	if directSortColumns[sort.Field] {
		fieldExpr = sort.Field
	} else {
		fieldExpr = r.db.JSONExtract("json", sort.Field)
	}

	expr := fmt.Sprintf("%s %s", fieldExpr, dir)

	// Append uri tiebreaker unless the primary sort field is already uri
	if sort.Field != "uri" {
		expr += fmt.Sprintf(", uri %s", dir)
	}

	return expr
}

// GetByCollectionSortedWithKeysetCursor retrieves records for a collection with
// optional field-level filters, a configurable sort order, and keyset-based pagination.
// The sort field and direction are specified via the sort parameter (nil = default indexed_at DESC).
// afterCursorValues is [sortFieldValue, uri] for keyset pagination; empty means first page.
// If didFilter is non-empty, results are further filtered by DID.
func (r *RecordsRepository) GetByCollectionSortedWithKeysetCursor(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	sort *SortOption,
	limit int,
	afterCursorValues []string,
) ([]*Record, error) {
	var whereParts []string
	var args []any

	// collection = ? is always param 1
	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	args = append(args, collection)

	nextPlaceholder := 2

	// Keyset cursor condition
	if len(afterCursorValues) == 2 {
		afterSortVal := afterCursorValues[0]
		afterURI := afterCursorValues[1]
		sortField := keysetSortFieldName(sort)

		// Determine the sort field expression and comparison operator
		sortFieldExpr := r.keysetSortFieldExpr(sortField)

		// DESC uses <, ASC uses >
		var cmp string
		if sort == nil || sort.Direction == "DESC" {
			cmp = "<"
		} else {
			cmp = ">"
		}

		p1 := r.db.Placeholder(nextPlaceholder)
		p2 := r.db.Placeholder(nextPlaceholder + 1)
		p3 := r.db.Placeholder(nextPlaceholder + 2)
		cursorExpr1 := r.keysetCursorValueExpr(sortField, p1)
		cursorExpr2 := r.keysetCursorValueExpr(sortField, p2)

		// Composite keyset: (sortField op afterSortVal) OR (sortField = afterSortVal AND uri op afterURI)
		whereParts = append(whereParts, fmt.Sprintf(
			"(%s %s %s OR (%s = %s AND uri %s %s))",
			sortFieldExpr, cmp, cursorExpr1,
			sortFieldExpr, cursorExpr2,
			cmp, p3,
		))
		args = append(args, afterSortVal, afterSortVal, afterURI)
		nextPlaceholder += 3
	}

	// Field filters
	filterClause, filterParams, err := r.buildFilterClause(filters, nextPlaceholder)
	if err != nil {
		return nil, fmt.Errorf("failed to build filter clause: %w", err)
	}
	if filterClause != "" {
		whereParts = append(whereParts, filterClause)
		for _, p := range filterParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
			nextPlaceholder++
		}
	}

	// DID filter
	if didClause, didParams, _ := r.buildDIDFilterClause(didFilter, nextPlaceholder); didClause != "" {
		whereParts = append(whereParts, didClause)
		for _, p := range didParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	orderBy := r.buildSortExpr(sort)
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s LIMIT %d",
		r.recordColumns(), whereClause, orderBy, limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query sorted records: %w", err)
	}
	defer rows.Close()

	return scanRecords(rows)
}

// GetByCollectionReversedWithKeysetCursor retrieves records for backward pagination
// per the Relay Connection Spec (last/before).
//
// Algorithm:
//  1. Reverse the sort direction (DESC→ASC, ASC→DESC)
//  2. Reverse the cursor comparison operator (DESC's < becomes >, ASC's > becomes <)
//  3. Fetch limit records with LIMIT
//  4. Reverse the result slice in-memory to restore the original sort order
//
// This ensures that `last N` returns the last N edges in the connection, and
// `last N, before cursor` returns the N edges immediately before the cursor.
//
// Fetches limit+1 to allow the caller to detect hasPreviousPage.
// If didFilter is non-empty, results are further filtered by DID.
func (r *RecordsRepository) GetByCollectionReversedWithKeysetCursor(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	sort *SortOption,
	limit int,
	beforeCursorValues []string,
) ([]*Record, error) {
	// Build the reversed sort option: flip direction
	var reversedSort *SortOption
	if sort == nil {
		// Default is indexed_at DESC → reverse to ASC
		reversedSort = &SortOption{Field: "indexed_at", Direction: "ASC"}
	} else {
		dir := "ASC"
		if sort.Direction == "ASC" {
			dir = "DESC"
		}
		reversedSort = &SortOption{Field: sort.Field, Direction: dir}
	}

	var whereParts []string
	var args []any

	// collection = ? is always param 1
	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	args = append(args, collection)

	nextPlaceholder := 2

	// Keyset cursor condition with reversed comparison operator.
	// For DESC original (reversed to ASC): forward DESC uses <, so reversed uses >
	// For ASC original (reversed to DESC): forward ASC uses >, so reversed uses <
	if len(beforeCursorValues) == 2 {
		beforeSortVal := beforeCursorValues[0]
		beforeURI := beforeCursorValues[1]
		sortField := keysetSortFieldName(reversedSort)

		// Determine the sort field expression using the reversed sort's field
		sortFieldExpr := r.keysetSortFieldExpr(sortField)

		// Reversed comparison: ASC reversed direction uses >, DESC reversed direction uses <
		var cmp string
		if reversedSort.Direction == "ASC" {
			cmp = ">"
		} else {
			cmp = "<"
		}

		p1 := r.db.Placeholder(nextPlaceholder)
		p2 := r.db.Placeholder(nextPlaceholder + 1)
		p3 := r.db.Placeholder(nextPlaceholder + 2)
		cursorExpr1 := r.keysetCursorValueExpr(sortField, p1)
		cursorExpr2 := r.keysetCursorValueExpr(sortField, p2)

		whereParts = append(whereParts, fmt.Sprintf(
			"(%s %s %s OR (%s = %s AND uri %s %s))",
			sortFieldExpr, cmp, cursorExpr1,
			sortFieldExpr, cursorExpr2,
			cmp, p3,
		))
		args = append(args, beforeSortVal, beforeSortVal, beforeURI)
		nextPlaceholder += 3
	}

	// Field filters
	filterClause, filterParams, err := r.buildFilterClause(filters, nextPlaceholder)
	if err != nil {
		return nil, fmt.Errorf("failed to build filter clause: %w", err)
	}
	if filterClause != "" {
		whereParts = append(whereParts, filterClause)
		for _, p := range filterParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
			nextPlaceholder++
		}
	}

	// DID filter
	if didClause, didParams, _ := r.buildDIDFilterClause(didFilter, nextPlaceholder); didClause != "" {
		whereParts = append(whereParts, didClause)
		for _, p := range didParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	orderBy := r.buildSortExpr(reversedSort)
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s LIMIT %d",
		r.recordColumns(), whereClause, orderBy, limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query reversed records: %w", err)
	}
	defer rows.Close()

	records, err := scanRecords(rows)
	if err != nil {
		return nil, err
	}

	// Reverse the result slice to restore the original sort order
	for i, j := 0, len(records)-1; i < j; i, j = i+1, j-1 {
		records[i], records[j] = records[j], records[i]
	}

	return records, nil
}

// GetByDID retrieves all records for a specific DID.
func (r *RecordsRepository) GetByDID(ctx context.Context, did string) ([]*Record, error) {
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE did = %s ORDER BY indexed_at DESC",
		r.recordColumns(), r.db.Placeholder(1))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// Delete removes a record by URI.
func (r *RecordsRepository) Delete(ctx context.Context, uri string) error {
	sqlStr := fmt.Sprintf("DELETE FROM record WHERE uri = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(uri)})
	return err
}

// DeleteAll removes all records.
func (r *RecordsRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, "DELETE FROM record", nil)
	return err
}

// GetCount returns the total number of records.
func (r *RecordsRepository) GetCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM record", nil, &count)
	return count, err
}

// GetCollectionCount returns the total record count for a collection.
func (r *RecordsRepository) GetCollectionCount(ctx context.Context, collection string) (int64, error) {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM record WHERE collection = %s", r.db.Placeholder(1))
	var count int64
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(collection)}, &count)
	return count, err
}

// GetCollectionCountFiltered returns the count with optional DID and field filters applied.
func (r *RecordsRepository) GetCollectionCountFiltered(
	ctx context.Context, collection string, filters []FieldFilter, didFilter DIDFilter,
) (int64, error) {
	var whereParts []string
	var params []database.Value

	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	params = append(params, database.Text(collection))

	nextPlaceholder := 2

	// Field filters
	filterClause, filterParams, err := r.buildFilterClause(filters, nextPlaceholder)
	if err != nil {
		return 0, fmt.Errorf("failed to build filter clause: %w", err)
	}
	if filterClause != "" {
		whereParts = append(whereParts, filterClause)
		params = append(params, filterParams...)
		nextPlaceholder += len(filterParams)
	}

	// DID filter
	if didClause, didParams, _ := r.buildDIDFilterClause(didFilter, nextPlaceholder); didClause != "" {
		whereParts = append(whereParts, didClause)
		params = append(params, didParams...)
	}

	whereClause := strings.Join(whereParts, " AND ")
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM record WHERE %s", whereClause)

	var count int64
	err = r.db.QueryRow(ctx, sqlStr, params, &count)
	return count, err
}

// GetCollectionStats returns statistics for all collections.
func (r *RecordsRepository) GetCollectionStats(ctx context.Context) ([]CollectionStat, error) {
	sqlStr := "SELECT collection, COUNT(*) as count FROM record GROUP BY collection ORDER BY count DESC"

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []CollectionStat
	for rows.Next() {
		var stat CollectionStat
		if err := rows.Scan(&stat.Collection, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetCollectionStatsFiltered returns statistics for specified collections.
// If collections is empty, returns stats for all collections.
func (r *RecordsRepository) GetCollectionStatsFiltered(ctx context.Context, collections []string) ([]CollectionStat, error) {
	if len(collections) == 0 {
		return r.GetCollectionStats(ctx)
	}

	placeholders := r.db.Placeholders(len(collections), 1)
	sqlStr := fmt.Sprintf("SELECT collection, COUNT(*) as count FROM record WHERE collection IN (%s) GROUP BY collection ORDER BY count DESC", placeholders)

	params := make([]database.Value, len(collections))
	for i, c := range collections {
		params[i] = database.Text(c)
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stats []CollectionStat
	for rows.Next() {
		var stat CollectionStat
		if err := rows.Scan(&stat.Collection, &stat.Count); err != nil {
			return nil, err
		}
		stats = append(stats, stat)
	}

	return stats, rows.Err()
}

// GetCollectionTimeSeries returns time series data for a collection.
// Records are grouped by date extracted from createdAt, eventDate, or indexed_at.
func (r *RecordsRepository) GetCollectionTimeSeries(ctx context.Context, collection string) (*CollectionTimeSeries, error) {
	var sqlStr string

	switch r.db.Dialect() {
	case database.PostgreSQL:
		// PostgreSQL: Extract date from JSON fields or fall back to indexed_at
		sqlStr = fmt.Sprintf(`
			SELECT 
				DATE(COALESCE(
					(json->>'createdAt')::timestamp,
					(json->>'eventDate')::timestamp,
					indexed_at
				)) as record_date,
				COUNT(*) as count
			FROM record 
			WHERE collection = %s
			GROUP BY record_date
			ORDER BY record_date`, r.db.Placeholder(1))
	default:
		// SQLite: Use json_extract for JSON fields
		sqlStr = fmt.Sprintf(`
			SELECT 
				DATE(COALESCE(
					json_extract(json, '$.createdAt'),
					json_extract(json, '$.eventDate'),
					indexed_at
				)) as record_date,
				COUNT(*) as count
			FROM record 
			WHERE collection = %s
			GROUP BY record_date
			ORDER BY record_date`, r.db.Placeholder(1))
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, collection)
	if err != nil {
		return nil, fmt.Errorf("failed to query time series: %w", err)
	}
	defer rows.Close()

	var data []TimeSeriesDataPoint
	var cumulative int64

	for rows.Next() {
		var date string
		var count int64
		if err := rows.Scan(&date, &count); err != nil {
			return nil, err
		}
		cumulative += count
		data = append(data, TimeSeriesDataPoint{
			Date:       date,
			Count:      count,
			Cumulative: cumulative,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Get total records and unique users
	var totalRecords, uniqueUsers int64
	countSQL := fmt.Sprintf("SELECT COUNT(*), COUNT(DISTINCT did) FROM record WHERE collection = %s", r.db.Placeholder(1))
	if err := r.db.QueryRow(ctx, countSQL, []database.Value{database.Text(collection)}, &totalRecords, &uniqueUsers); err != nil {
		return nil, fmt.Errorf("failed to get collection totals: %w", err)
	}

	return &CollectionTimeSeries{
		Collection:   collection,
		TotalRecords: totalRecords,
		UniqueUsers:  uniqueUsers,
		Data:         data,
	}, nil
}

// GetCIDsByURIs returns a map of URI -> CID for records that exist.
// Used for deduplication before batch insert.
func (r *RecordsRepository) GetCIDsByURIs(ctx context.Context, uris []string) (map[string]string, error) {
	if len(uris) == 0 {
		return make(map[string]string), nil
	}

	result := make(map[string]string)

	// Process in batches of 900 to avoid SQL parameter limits
	batchSize := SQLParamBatchSize
	for i := 0; i < len(uris); i += batchSize {
		end := i + batchSize
		if end > len(uris) {
			end = len(uris)
		}
		batch := uris[i:end]

		placeholders := r.db.Placeholders(len(batch), 1)
		sqlStr := fmt.Sprintf("SELECT uri, cid FROM record WHERE uri IN (%s)", placeholders)

		params := make([]database.Value, len(batch))
		for j, uri := range batch {
			params[j] = database.Text(uri)
		}

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var uri, cid string
			if err := rows.Scan(&uri, &cid); err != nil {
				rows.Close()
				return nil, err
			}
			result[uri] = cid
		}
		rows.Close()

		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// GetExistingCIDs returns a set of CIDs that already exist in the database.
// Used to detect duplicate content across different URIs.
func (r *RecordsRepository) GetExistingCIDs(ctx context.Context, cids []string) (map[string]bool, error) {
	if len(cids) == 0 {
		return make(map[string]bool), nil
	}

	result := make(map[string]bool)

	// Process in batches of 900 to avoid SQL parameter limits
	batchSize := SQLParamBatchSize
	for i := 0; i < len(cids); i += batchSize {
		end := i + batchSize
		if end > len(cids) {
			end = len(cids)
		}
		batch := cids[i:end]

		placeholders := r.db.Placeholders(len(batch), 1)
		sqlStr := fmt.Sprintf("SELECT cid FROM record WHERE cid IN (%s)", placeholders)

		params := make([]database.Value, len(batch))
		for j, cid := range batch {
			params[j] = database.Text(cid)
		}

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
		if err != nil {
			return nil, err
		}

		for rows.Next() {
			var cid string
			if err := rows.Scan(&cid); err != nil {
				rows.Close()
				return nil, err
			}
			result[cid] = true
		}
		rows.Close()

		if err := rows.Err(); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// escapeLIKE escapes special LIKE wildcard characters (%, _) in a user-provided search string.
// This prevents wildcard injection where user input could match unintended patterns.
func escapeLIKE(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, "%", `\%`)
	s = strings.ReplaceAll(s, "_", `\_`)
	return s
}

// Search performs a LIKE-based text search on record JSON content.
// On PostgreSQL, uses case-insensitive ILIKE. On SQLite, LIKE is already case-insensitive for ASCII.
// collection is optional; if empty, searches across all collections.
// Supports keyset pagination via afterTimestamp and afterURI.
func (r *RecordsRepository) Search(
	ctx context.Context,
	searchQuery string,
	collection string,
	limit int,
	afterTimestamp string,
	afterURI string,
) ([]*Record, error) {
	ctx, cancel := context.WithTimeout(ctx, SearchTimeout)
	defer cancel()

	escaped := escapeLIKE(searchQuery)
	likeValue := "%" + escaped + "%"

	var conditions []string
	var params []database.Value
	paramIdx := 1

	// JSON LIKE/ILIKE condition
	switch r.db.Dialect() {
	case database.PostgreSQL:
		conditions = append(conditions, fmt.Sprintf("json::text ILIKE %s ESCAPE '\\'", r.db.Placeholder(paramIdx)))
	default:
		conditions = append(conditions, fmt.Sprintf("json LIKE %s ESCAPE '\\'", r.db.Placeholder(paramIdx)))
	}
	params = append(params, database.Text(likeValue))
	paramIdx++

	// Optional collection filter
	if collection != "" {
		conditions = append(conditions, fmt.Sprintf("collection = %s", r.db.Placeholder(paramIdx)))
		params = append(params, database.Text(collection))
		paramIdx++
	}

	// Keyset cursor
	if afterTimestamp != "" && afterURI != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(indexed_at < %s OR (indexed_at = %s AND uri < %s))",
			r.db.Placeholder(paramIdx),
			r.db.Placeholder(paramIdx+1),
			r.db.Placeholder(paramIdx+2),
		))
		params = append(params, database.Text(afterTimestamp), database.Text(afterTimestamp), database.Text(afterURI))
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")
	sqlStr := fmt.Sprintf("SELECT %s FROM record %s ORDER BY indexed_at DESC, uri DESC LIMIT %d",
		r.recordColumns(), whereClause, limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, fmt.Errorf("failed to search records: %w", err)
	}
	defer rows.Close()

	return scanRecords(rows)
}

// Helper functions

func (r *RecordsRepository) getCIDByURI(ctx context.Context, uri string) (string, error) {
	var cid string
	err := r.db.QueryRow(ctx, fmt.Sprintf("SELECT cid FROM record WHERE uri = %s", r.db.Placeholder(1)),
		[]database.Value{database.Text(uri)}, &cid)
	return cid, err
}

func scanRecords(rows *sql.Rows) ([]*Record, error) {
	var records []*Record
	for rows.Next() {
		var rec Record
		var indexedAtStr string
		if err := rows.Scan(&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey); err != nil {
			return nil, err
		}
		// Try various timestamp formats
		rec.IndexedAt = atproto.ParseTimestamp(indexedAtStr)
		records = append(records, &rec)
	}
	return records, rows.Err()
}

// IterateAll calls the provided function for each record in the database.
// Records are processed in batches to manage memory usage.
// Returns the total number of records processed.
func (r *RecordsRepository) IterateAll(ctx context.Context, batchSize int, fn func(*Record) error) (int64, error) {
	if batchSize <= 0 {
		batchSize = DefaultIterateBatchSize
	}

	var totalProcessed int64
	var lastURI string

	for {
		// Fetch next batch ordered by URI (for stable pagination)
		var sqlStr string
		var params []database.Value

		if lastURI == "" {
			sqlStr = fmt.Sprintf("SELECT %s FROM record ORDER BY uri LIMIT %d",
				r.recordColumns(), batchSize)
			params = nil
		} else {
			sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE uri > %s ORDER BY uri LIMIT %d",
				r.recordColumns(), r.db.Placeholder(1), batchSize)
			params = []database.Value{database.Text(lastURI)}
		}

		var args []any
		if params != nil {
			args = r.db.ConvertParams(params)
		}

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, args...)
		if err != nil {
			return totalProcessed, err
		}

		records, err := scanRecords(rows)
		rows.Close()
		if err != nil {
			return totalProcessed, err
		}

		if len(records) == 0 {
			break // No more records
		}

		// Process each record
		for _, rec := range records {
			if err := fn(rec); err != nil {
				return totalProcessed, err
			}
			totalProcessed++
			lastURI = rec.URI
		}

		// If we got fewer records than batch size, we're done
		if len(records) < batchSize {
			break
		}
	}

	return totalProcessed, nil
}
