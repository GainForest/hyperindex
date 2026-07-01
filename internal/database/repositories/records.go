// Package repositories contains data access layer implementations.
package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/validation"
)

// Batch size constants for SQL operations.
const (
	// BatchInsertSize is the number of records per INSERT batch (6 params each = 600 SQL params).
	BatchInsertSize = 100

	// SQLParamBatchSize is the batch size for IN-clause queries, kept under SQLite's 999 param limit.
	SQLParamBatchSize = 900

	// SQLiteAggregateParameterLimit is SQLite's hard per-query parameter limit.
	SQLiteAggregateParameterLimit = 999

	// DefaultIterateBatchSize is the default batch size for IterateAll when none specified.
	DefaultIterateBatchSize = 1000

	// SearchTimeout is the maximum duration for a search query.
	SearchTimeout = 10 * time.Second

	// MaxINListSize is the maximum number of values allowed in a single IN filter clause.
	// It does not, by itself, guarantee the overall query stays within SQLite's
	// aggregate parameter limit when combined with other bound arguments.
	MaxINListSize = 100

	// MaxRecordTimelineAuthors is the largest author DID set accepted by the
	// record timeline API. SQLite timeline queries bind the set as JSON to stay
	// below SQLite's aggregate parameter limit while preserving one global page.
	MaxRecordTimelineAuthors = 1000

	// MaxRecordTimelineCollections is the largest explicit collection set accepted
	// by record timeline queries. Callers must choose collections so timeline
	// queries do not scan every indexed collection by default.
	MaxRecordTimelineCollections = 25

	// MaxFilterConditions is the maximum number of individual filter conditions allowed per query.
	// The DID filter does not count toward this cap.
	MaxFilterConditions = 20
)

// ErrSQLiteAggregateParameterLimit indicates a query exceeded SQLite's bound parameter limit.
var ErrSQLiteAggregateParameterLimit = errors.New("sqlite query parameter count exceeds maximum allowed")

// Record represents an AT Protocol record stored in the database.
type Record struct {
	URI              string
	CID              string
	DID              string
	Collection       string
	JSON             string
	IndexedAt        time.Time
	RKey             string
	ValidationStatus validation.Status
	ValidationError  string
	ValidatedAt      *time.Time
	LexiconHash      string
}

// RecordTimelineCursor identifies a position in the creation-time record
// timeline. CreatedAt must use the repository's normalized UTC timestamp format
// so SQLite text comparisons and PostgreSQL timestamptz comparisons behave the
// same way.
type RecordTimelineCursor struct {
	CreatedAt string
	URI       string
}

// RecordTimelineRecord is a current AT Protocol record returned by the generic
// creation-time timeline query. RecordCreatedAt is the materialized top-level
// record JSON createdAt timestamp used for ordering and cursor generation.
type RecordTimelineRecord struct {
	Record
	RecordCreatedAt time.Time
}

// FieldFilterTarget identifies where a record field filter reads its value
// from. Use JSON for lexicon-defined record properties, Column only for
// generated metadata filters that intentionally target record table columns, and
// explicit collection filter extensions for hand-authored product queries that
// are not expressible as same-record lexicon JSON paths.
type FieldFilterTarget string

const (
	// FieldFilterTargetJSON reads from the record JSON payload. It is also the
	// default when FieldFilter.Target is empty, which keeps older callers on the
	// safe lexicon-property path.
	FieldFilterTargetJSON FieldFilterTarget = "json"

	// FieldFilterTargetColumn reads from a whitelisted metadata column on the
	// record table. Use it only for generated metadata filters such as uri.
	FieldFilterTargetColumn FieldFilterTarget = "column"

	// FieldFilterTargetContributorDID is a narrow compatibility filter for
	// org.hypercerts.claim.activity contributor lookups. It matches inline
	// contributor DID strings and contributorInformation strongRefs whose target
	// record has a matching identifier.
	FieldFilterTargetContributorDID FieldFilterTarget = "contributor_did"

	// FieldFilterTargetBadgeAwardBadgeType is a collection filter extension for
	// app.certified.badge.award. It reads the award's badge strongRef URI and
	// filters against the referenced app.certified.badge.definition badgeType.
	FieldFilterTargetBadgeAwardBadgeType FieldFilterTarget = "badge_award_badge_type"
)

// FieldFilter represents a single condition on a filterable record field.
type FieldFilter struct {
	Field     string            // Field name for diagnostics. JSON targets use a top-level JSON property unless Path is set; column targets use a whitelisted metadata column.
	Path      []string          // Optional JSON path from the record root for nested filters. Empty means Field is the JSON path.
	ArrayPath []string          // Optional JSON path to an array field whose elements should be searched with any-semantics. Filters with the same ArrayPath are correlated against the same array element; Path is evaluated relative to that element.
	Operator  string            // One of: "eq", "neq", "gt", "lt", "gte", "lte", "in", "contains", "startsWith", "isNull"
	Value     interface{}       // The comparison value. For "in", must be []interface{}. For "isNull", must be bool.
	FieldType string            // Lexicon type used for SQL casting. Numeric types are cast; complex types are presence-filtered with isNull.
	Target    FieldFilterTarget // Where to read Field from. Empty means FieldFilterTargetJSON.
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

// ExternalLabelStringFilter represents one string condition on an external
// label column such as src or val. It supports the same string operators exposed
// by GraphQL StringFilterInput where they are meaningful for label metadata.
type ExternalLabelStringFilter struct {
	Operator string      // One of: "eq", "neq", "in", "contains", "startsWith", "isNull".
	Value    interface{} // For "in", use []interface{} or []string. Other operators expect a scalar string.
}

// ExternalLabelPredicate describes labels that should exist or not exist for a
// record. Source filters apply to external_label.src, value filters apply to
// external_label.val, and ActiveOnly applies ATProto current-label semantics.
type ExternalLabelPredicate struct {
	Sources    []ExternalLabelStringFilter
	Values     []ExternalLabelStringFilter
	ActiveOnly bool
}

// ExternalLabelRecordFilter applies external-label existence predicates to one
// bound label subject in record queries. Has keeps records with a matching
// label. None excludes records with a matching label. When both are set, both
// conditions must hold.
type ExternalLabelRecordFilter struct {
	Has  *ExternalLabelPredicate
	None *ExternalLabelPredicate
}

// IsEmpty reports whether no external-label conditions are configured.
func (f ExternalLabelRecordFilter) IsEmpty() bool {
	return f.Has == nil && f.None == nil
}

type externalLabelSubject int

const (
	externalLabelSubjectRecord externalLabelSubject = iota
	externalLabelSubjectAuthor
)

// ExternalLabelFilterSet groups external-label predicates by subject binding.
// Record powers where.externalLabels; Author powers where.authorLabels.
type ExternalLabelFilterSet struct {
	Record ExternalLabelRecordFilter
	Author ExternalLabelRecordFilter
}

// IsEmpty reports whether no external-label conditions are configured for any
// subject binding.
func (f ExternalLabelFilterSet) IsEmpty() bool {
	return f.Record.IsEmpty() && f.Author.IsEmpty()
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

func recordCreatedAtValue(recordJSON string) database.Value {
	createdAt, ok := atproto.NormalizeRecordCreatedAt(recordJSON)
	if !ok {
		return database.NullValue{}
	}
	return database.TimestamptzValue(createdAt)
}

// validateSQLiteAggregateParameterCount ensures SQLite queries stay within the
// hard per-query parameter limit. Non-SQLite dialects are not capped here.
func (r *RecordsRepository) validateSQLiteAggregateParameterCount(paramCount int) error {
	if r.db.Dialect() != database.SQLite {
		return nil
	}

	if paramCount > SQLiteAggregateParameterLimit {
		return fmt.Errorf("%w: %d exceeds maximum allowed %d", ErrSQLiteAggregateParameterLimit, paramCount, SQLiteAggregateParameterLimit)
	}

	return nil
}

// recordColumns returns the columns to select based on dialect.
func (r *RecordsRepository) recordColumns() string {
	switch r.db.Dialect() {
	case database.PostgreSQL:
		return "uri, cid, did, collection, json::text, indexed_at::text, rkey, validation_status, validation_error, validated_at::text, lexicon_hash"
	default:
		return "uri, cid, did, collection, json, indexed_at, rkey, validation_status, validation_error, validated_at, lexicon_hash"
	}
}

// Insert inserts or updates a record in the database.
// Skips if the CID already exists (content unchanged).
func (r *RecordsRepository) Insert(ctx context.Context, uri, cid, did, collection, jsonData string) (InsertResult, error) {
	createdAtValue := recordCreatedAtValue(jsonData)

	// Check if URI exists with same CID.
	existingCID, existingHasRecordCreatedAt, err := r.getCIDAndRecordCreatedAtByURI(ctx, uri)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return Skipped, err
	}

	// Only skip if both the existing and incoming CID are non-empty and match.
	// If cid == "" (omitted by Tap for some events), always proceed with the
	// upsert so new records aren't silently dropped by the "" == "" comparison.
	if cid != "" && existingCID == cid {
		if !existingHasRecordCreatedAt {
			if err := r.fillMissingRecordCreatedAt(ctx, uri, createdAtValue); err != nil {
				return Skipped, err
			}
		}
		return Skipped, nil // Content unchanged
	}

	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	p3 := r.db.Placeholder(3)
	p4 := r.db.Placeholder(4)
	p5 := r.db.Placeholder(5)
	p6 := r.db.Placeholder(6)

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json, record_created_at)
			VALUES (%s, %s, %s, %s, %s::jsonb, %s::timestamptz)
			ON CONFLICT(uri) DO UPDATE SET
				cid = EXCLUDED.cid,
				json = EXCLUDED.json,
				indexed_at = NOW(),
				record_created_at = COALESCE(record.record_created_at, EXCLUDED.record_created_at)`, p1, p2, p3, p4, p5, p6)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json, record_created_at)
			VALUES (%s, %s, %s, %s, %s, %s)
			ON CONFLICT(uri) DO UPDATE SET
				cid = excluded.cid,
				json = excluded.json,
				indexed_at = datetime('now'),
				record_created_at = COALESCE(record.record_created_at, excluded.record_created_at)`, p1, p2, p3, p4, p5, p6)
	}

	_, err = r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(uri),
		database.Text(cid),
		database.Text(did),
		database.Text(collection),
		database.Text(jsonData),
		createdAtValue,
	})
	if err != nil {
		return Skipped, err
	}

	return Inserted, nil
}

// UpdateValidationStatus records the local lexicon validation result for a raw record.
func (r *RecordsRepository) UpdateValidationStatus(ctx context.Context, uri string, status validation.Status, validationError, lexiconHash string) error {
	validationErrorValue := nullableTextValue(validationError)
	lexiconHashValue := nullableTextValue(lexiconHash)

	validatedAtExpr := "datetime('now')"
	if r.db.Dialect() == database.PostgreSQL {
		validatedAtExpr = "NOW()"
	}

	sqlStr := fmt.Sprintf(`UPDATE record
		SET validation_status = %s,
			validation_error = %s,
			validated_at = %s,
			lexicon_hash = %s
		WHERE uri = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2), validatedAtExpr, r.db.Placeholder(3), r.db.Placeholder(4))

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(string(status)),
		validationErrorValue,
		lexiconHashValue,
		database.Text(uri),
	})
	return err
}

// MarkCollectionUnknownSchema marks every record in a collection as hidden from
// typed GraphQL because Hyperindex has no saved lexicon to validate it against.
func (r *RecordsRepository) MarkCollectionUnknownSchema(ctx context.Context, collection, reason string) error {
	validatedAtExpr := "datetime('now')"
	if r.db.Dialect() == database.PostgreSQL {
		validatedAtExpr = "NOW()"
	}

	sqlStr := fmt.Sprintf(`UPDATE record
		SET validation_status = %s,
			validation_error = %s,
			validated_at = %s,
			lexicon_hash = NULL
		WHERE collection = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2), validatedAtExpr, r.db.Placeholder(3))

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(string(validation.StatusUnknownSchema)),
		database.Text(reason),
		database.Text(collection),
	})
	return err
}

// ListRecordsNeedingValidation returns a stable URI-ordered batch of records
// whose saved validation metadata is missing or stale for the current lexicon.
func (r *RecordsRepository) ListRecordsNeedingValidation(ctx context.Context, collection, currentLexiconHash, afterURI string, limit int) ([]*Record, error) {
	if limit <= 0 {
		limit = DefaultIterateBatchSize
	}

	sqlStr := fmt.Sprintf(`SELECT %s FROM record
		WHERE collection = %s
		  AND (validation_status != %s OR lexicon_hash IS NULL OR lexicon_hash != %s)
		  AND uri > %s
		ORDER BY uri
		LIMIT %d`,
		r.recordColumns(), r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4), limit)

	params := []database.Value{
		database.Text(collection),
		database.Text(string(validation.StatusValid)),
		database.Text(currentLexiconHash),
		database.Text(afterURI),
	}
	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

func nullableTextValue(value string) database.Value {
	if value == "" {
		return database.Null()
	}
	return database.Text(value)
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
		base := i * 6
		var valueSet string

		if r.db.Dialect() == database.PostgreSQL {
			valueSet = fmt.Sprintf("(%s, %s, %s, %s, %s::jsonb, %s::timestamptz)",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2),
				r.db.Placeholder(base+3),
				r.db.Placeholder(base+4),
				r.db.Placeholder(base+5),
				r.db.Placeholder(base+6))
		} else {
			valueSet = fmt.Sprintf("(%s, %s, %s, %s, %s, %s)",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2),
				r.db.Placeholder(base+3),
				r.db.Placeholder(base+4),
				r.db.Placeholder(base+5),
				r.db.Placeholder(base+6))
		}
		valueSets = append(valueSets, valueSet)

		args = append(args, rec.URI, rec.CID, rec.DID, rec.Collection, rec.JSON, r.db.ConvertParams([]database.Value{recordCreatedAtValue(rec.JSON)})[0])
	}

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json, record_created_at)
			VALUES %s
			ON CONFLICT(uri) DO UPDATE SET
				cid = EXCLUDED.cid,
				json = EXCLUDED.json,
				indexed_at = NOW(),
				record_created_at = COALESCE(record.record_created_at, EXCLUDED.record_created_at)`, strings.Join(valueSets, ", "))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json, record_created_at)
			VALUES %s
			ON CONFLICT(uri) DO UPDATE SET
				cid = excluded.cid,
				json = excluded.json,
				indexed_at = datetime('now'),
				record_created_at = COALESCE(record.record_created_at, excluded.record_created_at)`, strings.Join(valueSets, ", "))
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
	var validationStatus string
	var validationError, validatedAtStr, lexiconHash sql.NullString
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(uri)},
		&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey,
		&validationStatus, &validationError, &validatedAtStr, &lexiconHash)
	if err != nil {
		return nil, err
	}

	rec.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	rec.ValidationStatus = validation.Status(validationStatus)
	applyRecordValidationNulls(&rec, validationError, validatedAtStr, lexiconHash)
	return &rec, nil
}

// GetValidByURI retrieves a typed-visible record by URI. It returns sql.ErrNoRows
// when the raw record exists but is hidden by validation status or collection mismatch.
func (r *RecordsRepository) GetValidByURI(ctx context.Context, uri, collection string) (*Record, error) {
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri = %s AND collection = %s AND validation_status = %s",
		r.recordColumns(), r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))

	var rec Record
	var indexedAtStr string
	var validationStatus string
	var validationError, validatedAtStr, lexiconHash sql.NullString
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{
		database.Text(uri),
		database.Text(collection),
		database.Text(string(validation.StatusValid)),
	},
		&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey,
		&validationStatus, &validationError, &validatedAtStr, &lexiconHash)
	if err != nil {
		return nil, err
	}

	rec.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	rec.ValidationStatus = validation.Status(validationStatus)
	applyRecordValidationNulls(&rec, validationError, validatedAtStr, lexiconHash)
	return &rec, nil
}

// GetByURIs retrieves multiple records by their URIs.
func (r *RecordsRepository) GetByURIs(ctx context.Context, uris []string) ([]*Record, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	records := make([]*Record, 0, len(uris))
	for start := 0; start < len(uris); start += SQLParamBatchSize {
		end := start + SQLParamBatchSize
		if end > len(uris) {
			end = len(uris)
		}
		batch := uris[start:end]

		placeholders := r.db.Placeholders(len(batch), 1)
		sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri IN (%s)",
			r.recordColumns(), placeholders)

		params := make([]database.Value, len(batch))
		for i, uri := range batch {
			params[i] = database.Text(uri)
		}

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
		if err != nil {
			return nil, err
		}
		batchRecords, scanErr := scanRecords(rows)
		closeErr := rows.Close()
		if scanErr != nil {
			return nil, scanErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		records = append(records, batchRecords...)
	}

	return records, nil
}

// GetRecordTimeline returns one creation-time page of current records across
// explicit collections and an optional author set. Nil authors means no author
// filter; an empty author slice returns no rows. Rows are ordered by
// record_created_at DESC, uri DESC and use the same tuple for keyset cursors.
func (r *RecordsRepository) GetRecordTimeline(ctx context.Context, authors, collections []string, limit int, after *RecordTimelineCursor) ([]*RecordTimelineRecord, error) {
	if limit <= 0 || len(collections) == 0 {
		return nil, nil
	}
	if authors != nil && len(authors) == 0 {
		return nil, nil
	}

	var conditions []string
	var params []database.Value
	nextPlaceholder := 1

	conditions = append(conditions, "record_created_at IS NOT NULL")
	if r.db.Dialect() == database.SQLite {
		conditions = append(conditions, "json_valid(json)")
	}

	collectionCondition, collectionParams, consumed, err := r.timelineSetCondition("collection", collections, nextPlaceholder)
	if err != nil {
		return nil, err
	}
	conditions = append(conditions, collectionCondition)
	params = append(params, collectionParams...)
	nextPlaceholder += consumed

	if authors != nil {
		authorCondition, authorParams, consumed, err := r.timelineSetCondition("did", authors, nextPlaceholder)
		if err != nil {
			return nil, err
		}
		conditions = append(conditions, authorCondition)
		params = append(params, authorParams...)
		nextPlaceholder += consumed
	}

	if after != nil {
		createdAtPlaceholder := r.recordCreatedAtCursorValueExpr(r.db.Placeholder(nextPlaceholder))
		createdAtAgainPlaceholder := r.recordCreatedAtCursorValueExpr(r.db.Placeholder(nextPlaceholder + 1))
		uriPlaceholder := r.db.Placeholder(nextPlaceholder + 2)
		conditions = append(conditions, fmt.Sprintf("(record_created_at < %s OR (record_created_at = %s AND uri < %s))", createdAtPlaceholder, createdAtAgainPlaceholder, uriPlaceholder))
		params = append(params, database.TimestamptzValue(after.CreatedAt), database.TimestamptzValue(after.CreatedAt), database.Text(after.URI))
		nextPlaceholder += 3
	}

	limitPlaceholder := r.db.Placeholder(nextPlaceholder)
	params = append(params, database.IntValue(int64(limit)))

	sqlStr := fmt.Sprintf(`SELECT %s
		FROM record
		WHERE %s
		ORDER BY record_created_at DESC, uri DESC
		LIMIT %s`, r.recordTimelineColumns(), strings.Join(conditions, " AND "), limitPlaceholder)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, fmt.Errorf("failed to query record timeline: %w", err)
	}
	defer rows.Close()

	return scanRecordTimelineRecords(rows)
}

func (r *RecordsRepository) recordTimelineColumns() string {
	switch r.db.Dialect() {
	case database.PostgreSQL:
		return "uri, cid, did, collection, json::text, indexed_at::text, rkey, record_created_at::text"
	default:
		return "uri, cid, did, collection, json, indexed_at, rkey, record_created_at"
	}
}

func (r *RecordsRepository) recordCreatedAtCursorValueExpr(placeholder string) string {
	if r.db.Dialect() == database.PostgreSQL {
		return fmt.Sprintf("%s::timestamptz", placeholder)
	}
	return placeholder
}

func (r *RecordsRepository) timelineSetCondition(column string, values []string, startPlaceholder int) (string, []database.Value, int, error) {
	if len(values) == 0 {
		return "", nil, 0, fmt.Errorf("%s set must not be empty", column)
	}

	if r.db.Dialect() == database.SQLite {
		encoded, err := json.Marshal(values)
		if err != nil {
			return "", nil, 0, fmt.Errorf("failed to encode %s set: %w", column, err)
		}
		return fmt.Sprintf("%s IN (SELECT value FROM json_each(%s))", column, r.db.Placeholder(startPlaceholder)), []database.Value{database.Text(string(encoded))}, 1, nil
	}

	placeholders := r.db.Placeholders(len(values), startPlaceholder)
	params := make([]database.Value, len(values))
	for i, value := range values {
		params[i] = database.Text(value)
	}
	return fmt.Sprintf("%s IN (%s)", column, placeholders), params, len(values), nil
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
	indexedAtExpr := r.normalizedIndextAtExpr()

	if afterTimestamp == "" {
		// No cursor - get first page, ordered by indexed_at DESC (newest first)
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s ORDER BY %s DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), indexedAtExpr, limit)
		args = []any{collection}
	} else {
		// With cursor - get records older than the cursor timestamp
		// Using indexed_at < cursor for "load more" (older posts)
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s AND %s < %s ORDER BY %s DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), indexedAtExpr, r.keysetCursorValueExpr("indexed_at", r.db.Placeholder(2)), indexedAtExpr, limit)
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
	indexedAtExpr := r.normalizedIndextAtExpr()

	if afterTimestamp == "" && afterURI == "" {
		// No cursor - get first page
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s ORDER BY %s DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1), indexedAtExpr, limit)
		args = []any{collection}
	} else {
		// Keyset pagination: get records that sort after (afterTimestamp, afterURI)
		// ORDER BY indexed_at DESC, uri DESC means "after" = less than
		sqlStr = fmt.Sprintf("SELECT %s FROM record WHERE collection = %s AND (%s < %s OR (%s = %s AND uri < %s)) ORDER BY %s DESC, uri DESC LIMIT %d",
			r.recordColumns(), r.db.Placeholder(1),
			indexedAtExpr, r.keysetCursorValueExpr("indexed_at", r.db.Placeholder(2)),
			indexedAtExpr, r.keysetCursorValueExpr("indexed_at", r.db.Placeholder(3)),
			r.db.Placeholder(4), indexedAtExpr, limit)
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

	processedArrayGroups := make(map[string]bool)
	for i, f := range filters {
		if isArrayAnyFilter(f) {
			groupKey := arrayFilterGroupKey(f)
			if processedArrayGroups[groupKey] {
				continue
			}
			processedArrayGroups[groupKey] = true

			arrayFilters := make([]FieldFilter, 0, len(filters)-i)
			for _, candidate := range filters[i:] {
				if arrayFilterGroupKey(candidate) == groupKey {
					arrayFilters = append(arrayFilters, candidate)
				}
			}

			condition, conditionParams, consumed, err := r.buildArrayAnyFilterCondition(arrayFilters, placeholderIdx, i)
			if err != nil {
				return "", nil, err
			}
			conditions = append(conditions, condition)
			params = append(params, conditionParams...)
			placeholderIdx += consumed
			continue
		}

		condition, conditionParams, consumed, err := r.buildFieldFilterCondition(f, placeholderIdx, i)
		if err != nil {
			return "", nil, err
		}
		conditions = append(conditions, condition)
		params = append(params, conditionParams...)
		placeholderIdx += consumed
	}

	return strings.Join(conditions, " AND "), params, nil
}

func isArrayAnyFilter(f FieldFilter) bool {
	return (f.Target == "" || f.Target == FieldFilterTargetJSON) && len(f.ArrayPath) > 0
}

func arrayFilterGroupKey(f FieldFilter) string {
	if !isArrayAnyFilter(f) {
		return ""
	}
	return strings.Join(f.ArrayPath, "\x00")
}

func (r *RecordsRepository) buildFieldFilterCondition(f FieldFilter, placeholderIdx, filterIndex int) (string, []database.Value, int, error) {
	switch f.Target {
	case FieldFilterTargetContributorDID:
		return r.buildContributorDIDFilterCondition(f, placeholderIdx)
	case FieldFilterTargetBadgeAwardBadgeType:
		return r.buildBadgeAwardBadgeTypeFilterCondition(f, placeholderIdx)
	case "", FieldFilterTargetJSON:
		if len(f.ArrayPath) > 0 {
			return r.buildArrayAnyFilterCondition([]FieldFilter{f}, placeholderIdx, filterIndex)
		}
	}

	extract, err := r.filterFieldExpr(f)
	if err != nil {
		return "", nil, 0, err
	}
	return r.buildScalarFilterCondition(extract, f, placeholderIdx)
}

func (r *RecordsRepository) buildScalarFilterCondition(extract string, f FieldFilter, placeholderIdx int) (string, []database.Value, int, error) {
	// Wrap numeric types in a CAST for proper comparison.
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
		return fmt.Sprintf("%s = %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "neq":
		return fmt.Sprintf("%s != %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "gt":
		return fmt.Sprintf("%s > %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "lt":
		return fmt.Sprintf("%s < %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "gte":
		return fmt.Sprintf("%s >= %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "lte":
		return fmt.Sprintf("%s <= %s", extract, r.db.Placeholder(placeholderIdx)), []database.Value{toDBValue(f.Value)}, 1, nil
	case "contains":
		likeOp := "LIKE"
		if r.db.Dialect() == database.PostgreSQL {
			likeOp = "ILIKE"
		}
		val := fmt.Sprintf("%%%s%%", escapeLIKE(fmt.Sprintf("%v", f.Value)))
		return fmt.Sprintf("%s %s %s ESCAPE '\\'", extract, likeOp, r.db.Placeholder(placeholderIdx)), []database.Value{database.Text(val)}, 1, nil
	case "startsWith":
		likeOp := "LIKE"
		if r.db.Dialect() == database.PostgreSQL {
			likeOp = "ILIKE"
		}
		val := fmt.Sprintf("%s%%", escapeLIKE(fmt.Sprintf("%v", f.Value)))
		return fmt.Sprintf("%s %s %s ESCAPE '\\'", extract, likeOp, r.db.Placeholder(placeholderIdx)), []database.Value{database.Text(val)}, 1, nil
	case "isNull":
		isNull, ok := f.Value.(bool)
		if !ok {
			return "", nil, 0, fmt.Errorf("isNull filter on field %q must be a boolean, got %T", f.Field, f.Value)
		}
		if isNull {
			return fmt.Sprintf("%s IS NULL", extract), nil, 0, nil
		}
		return fmt.Sprintf("%s IS NOT NULL", extract), nil, 0, nil
	case "in":
		inVals, ok := f.Value.([]interface{})
		if !ok {
			return "", nil, 0, fmt.Errorf("IN filter on field %q must be a list, got %T", f.Field, f.Value)
		}
		if len(inVals) == 0 {
			return "1 = 0", nil, 0, nil
		}
		if len(inVals) > MaxINListSize {
			return "", nil, 0, fmt.Errorf("IN filter on field %q exceeds maximum of %d values", f.Field, MaxINListSize)
		}
		placeholders := r.db.Placeholders(len(inVals), placeholderIdx)
		params := make([]database.Value, 0, len(inVals))
		for _, v := range inVals {
			params = append(params, toDBValue(v))
		}
		return fmt.Sprintf("%s IN (%s)", extract, placeholders), params, len(params), nil
	default:
		return "", nil, 0, fmt.Errorf("unsupported filter operator %q for field %q", f.Operator, f.Field)
	}
}

func (r *RecordsRepository) buildArrayAnyFilterCondition(filters []FieldFilter, placeholderIdx, filterIndex int) (string, []database.Value, int, error) {
	if len(filters) == 0 {
		return "", nil, 0, fmt.Errorf("array any filter requires at least one condition")
	}

	first := filters[0]
	alias := fmt.Sprintf("nested_filter_%d", filterIndex)
	arrayExpr := r.jsonValuePathExpr("record.json", first.ArrayPath)
	innerConditions := make([]string, 0, len(filters))
	params := make([]database.Value, 0, len(filters))
	consumed := 0

	for _, f := range filters {
		if !isArrayAnyFilter(f) || !sameStringSlice(f.ArrayPath, first.ArrayPath) {
			return "", nil, 0, fmt.Errorf("array any filters must share one JSON array path")
		}

		innerExpr := r.jsonTextPathExpr(alias+".value", f.Path)
		innerCondition, innerParams, innerConsumed, err := r.buildScalarFilterCondition(innerExpr, f, placeholderIdx+consumed)
		if err != nil {
			return "", nil, 0, err
		}
		innerConditions = append(innerConditions, innerCondition)
		params = append(params, innerParams...)
		consumed += innerConsumed
	}
	innerCondition := strings.Join(innerConditions, " AND ")

	switch r.db.Dialect() {
	case database.PostgreSQL:
		safeArrayExpr := fmt.Sprintf("CASE WHEN jsonb_typeof(%s) = 'array' THEN %s ELSE '[]'::jsonb END", arrayExpr, arrayExpr)
		return fmt.Sprintf("EXISTS (SELECT 1 FROM jsonb_array_elements(%s) AS %s(value) WHERE %s)", safeArrayExpr, alias, innerCondition), params, consumed, nil
	default:
		arrayTypeExpr := r.sqliteJSONTypePathExpr("record.json", first.ArrayPath)
		safeArrayExpr := fmt.Sprintf("CASE WHEN %s = 'array' THEN %s ELSE '[]' END", arrayTypeExpr, arrayExpr)
		return fmt.Sprintf("%s = 'array' AND EXISTS (SELECT 1 FROM json_each(%s) AS %s WHERE %s)", arrayTypeExpr, safeArrayExpr, alias, innerCondition), params, consumed, nil
	}
}

func sameStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *RecordsRepository) jsonTextPathExpr(column string, path []string) string {
	if len(path) == 0 {
		if r.db.Dialect() == database.PostgreSQL {
			return fmt.Sprintf("%s #>> '{}'", column)
		}
		return column
	}
	if r.db.Dialect() != database.PostgreSQL {
		return fmt.Sprintf("CASE WHEN json_valid(%s) THEN %s ELSE NULL END", column, r.db.JSONExtractPath(column, path))
	}
	return r.db.JSONExtractPath(column, path)
}

func (r *RecordsRepository) sqliteJSONTypePathExpr(column string, path []string) string {
	_ = r.db.JSONExtractPath(column, path)
	return fmt.Sprintf("json_type(%s, '%s')", column, sqliteJSONPath(path))
}

func sqliteJSONPath(path []string) string {
	if len(path) == 0 {
		return "$"
	}
	return "$." + strings.Join(path, ".")
}

func (r *RecordsRepository) jsonValuePathExpr(column string, path []string) string {
	if len(path) == 0 {
		return column
	}

	// Reuse the executor's path validation before building dialect-specific JSON
	// value expressions.
	_ = r.db.JSONExtractPath(column, path)

	if r.db.Dialect() != database.PostgreSQL {
		return r.db.JSONExtractPath(column, path)
	}

	var sb strings.Builder
	sb.WriteString(column)
	for _, segment := range path {
		sb.WriteString("->'")
		sb.WriteString(segment)
		sb.WriteString("'")
	}
	return sb.String()
}

func (r *RecordsRepository) buildBadgeAwardBadgeTypeFilterCondition(f FieldFilter, placeholderIdx int) (string, []database.Value, int, error) {
	badgeTypeExpr := r.badgeAwardBadgeTypeExpr()
	return r.buildScalarFilterCondition(badgeTypeExpr, f, placeholderIdx)
}

func (r *RecordsRepository) badgeAwardBadgeTypeExpr() string {
	badgeURIExpr := r.jsonTextPathExpr("record.json", []string{"badge", "uri"})
	badgeTypeExpr := r.jsonTextPathExpr("badge_definition.json", []string{"badgeType"})
	return fmt.Sprintf(`(
		SELECT %s
		FROM record badge_definition
		WHERE badge_definition.uri = %s
			AND badge_definition.collection = 'app.certified.badge.definition'
		LIMIT 1
	)`, badgeTypeExpr, badgeURIExpr)
}

func (r *RecordsRepository) buildContributorDIDFilterCondition(f FieldFilter, placeholderIdx int) (string, []database.Value, int, error) {
	values, err := contributorDIDFilterValues(f)
	if err != nil {
		return "", nil, 0, err
	}
	if len(values) == 0 {
		return "1 = 0", nil, 0, nil
	}
	if len(values) > MaxINListSize {
		return "", nil, 0, fmt.Errorf("contributor DID filter exceeds maximum of %d values", MaxINListSize)
	}

	switch r.db.Dialect() {
	case database.PostgreSQL:
		params := contributorDIDParams(values, 2)
		placeholders := r.db.Placeholders(len(values), placeholderIdx)
		refPlaceholders := r.db.Placeholders(len(values), placeholderIdx+len(values))
		return fmt.Sprintf(`EXISTS (
			SELECT 1 FROM jsonb_array_elements(CASE WHEN jsonb_typeof(record.json->'contributors') = 'array' THEN record.json->'contributors' ELSE '[]'::jsonb END) AS contributor(value)
			WHERE contributor.value #>> '{}' IN (%[1]s)
				OR contributor.value->>'identity' IN (%[1]s)
				OR contributor.value->'contributorIdentity'->>'identity' IN (%[1]s)
				OR contributor.value->'contributorIdentity'->>'did' IN (%[1]s)
				OR EXISTS (
					SELECT 1 FROM record contributor_info
					WHERE contributor_info.uri = contributor.value->'contributorIdentity'->>'uri'
						AND contributor_info.collection = 'org.hypercerts.claim.contributorInformation'
						AND contributor_info.json->>'identifier' IN (%[2]s)
				)
		)`, placeholders, refPlaceholders), params, len(params), nil
	default:
		params := contributorDIDParams(values, 5)
		barePlaceholders := r.db.Placeholders(len(values), placeholderIdx)
		directIdentityPlaceholders := r.db.Placeholders(len(values), placeholderIdx+len(values))
		identityPlaceholders := r.db.Placeholders(len(values), placeholderIdx+(len(values)*2))
		didPlaceholders := r.db.Placeholders(len(values), placeholderIdx+(len(values)*3))
		refPlaceholders := r.db.Placeholders(len(values), placeholderIdx+(len(values)*4))
		directIdentityExpr := r.jsonTextPathExpr("contributor.value", []string{"identity"})
		identityExpr := r.jsonTextPathExpr("contributor.value", []string{"contributorIdentity", "identity"})
		didExpr := r.jsonTextPathExpr("contributor.value", []string{"contributorIdentity", "did"})
		contributorInfoURIExpr := r.jsonTextPathExpr("contributor.value", []string{"contributorIdentity", "uri"})
		contributorsExpr := r.jsonValuePathExpr("record.json", []string{"contributors"})
		contributorsTypeExpr := r.sqliteJSONTypePathExpr("record.json", []string{"contributors"})
		safeContributorsExpr := fmt.Sprintf("CASE WHEN %s = 'array' THEN %s ELSE '[]' END", contributorsTypeExpr, contributorsExpr)
		return fmt.Sprintf(`EXISTS (
			SELECT 1 FROM json_each(%[10]s) AS contributor
			WHERE contributor.value IN (%[1]s)
				OR %[2]s IN (%[3]s)
				OR %[4]s IN (%[5]s)
				OR %[6]s IN (%[7]s)
				OR EXISTS (
					SELECT 1 FROM record contributor_info
					WHERE contributor_info.uri = %[8]s
						AND contributor_info.collection = 'org.hypercerts.claim.contributorInformation'
						AND json_extract(contributor_info.json, '$.identifier') IN (%[9]s)
				)
		)`, barePlaceholders, directIdentityExpr, directIdentityPlaceholders, identityExpr, identityPlaceholders, didExpr, didPlaceholders, contributorInfoURIExpr, refPlaceholders, safeContributorsExpr), params, len(params), nil
	}
}

func contributorDIDParams(values []string, copies int) []database.Value {
	params := make([]database.Value, 0, len(values)*copies)
	for range copies {
		for _, value := range values {
			params = append(params, database.Text(value))
		}
	}
	return params
}

func contributorDIDFilterValues(f FieldFilter) ([]string, error) {
	switch f.Operator {
	case "eq":
		value, ok := f.Value.(string)
		if !ok {
			return nil, fmt.Errorf("contributor DID eq filter must be a string, got %T", f.Value)
		}
		if value == "" {
			return nil, nil
		}
		return []string{value}, nil
	case "in":
		values, ok := f.Value.([]interface{})
		if !ok {
			return nil, fmt.Errorf("contributor DID in filter must be a list, got %T", f.Value)
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("contributor DID in filter values must be strings, got %T", value)
			}
			if text != "" {
				out = append(out, text)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported contributor DID filter operator %q", f.Operator)
	}
}

// metadataFilterColumns is the set of record table columns that generated
// metadata filters may read directly. Keep this narrower than directSortColumns:
// sort fields and where-filter fields have different collision rules.
var metadataFilterColumns = map[string]bool{
	"uri": true,
}

// filterFieldExpr returns the SQL expression used for a filterable field. The
// filter target decides whether Field is a JSON property or a record table
// column; callers must not infer that from the field name alone.
func (r *RecordsRepository) filterFieldExpr(f FieldFilter) (string, error) {
	switch f.Target {
	case "", FieldFilterTargetJSON:
		if len(f.Path) > 0 {
			return r.db.JSONExtractPath("json", f.Path), nil
		}
		return r.db.JSONExtract("json", f.Field), nil
	case FieldFilterTargetColumn:
		if !metadataFilterColumns[f.Field] {
			return "", fmt.Errorf("unsupported metadata filter column %q: expose it as a generated metadata filter before using FieldFilterTargetColumn", f.Field)
		}
		return f.Field, nil
	default:
		return "", fmt.Errorf("unsupported filter target %q for field %q: use FieldFilterTargetJSON or FieldFilterTargetColumn", f.Target, f.Field)
	}
}

// buildDIDFilterClause builds a SQL WHERE clause fragment for a DIDFilter.
// startPlaceholder is the 1-based index of the first placeholder to use.
// Returns the clause string (without leading "AND"), the parameter values, the
// number of placeholders consumed, and any validation error.
// Returns empty string and nil params if the filter is empty.
func (r *RecordsRepository) buildDIDFilterClause(f DIDFilter, startPlaceholder int) (string, []database.Value, int, error) {
	if f.IsEmpty() {
		return "", nil, 0, nil
	}

	// EQ takes precedence over IN when both are set.
	if f.EQ != "" {
		clause := fmt.Sprintf("did = %s", r.db.Placeholder(startPlaceholder))
		return clause, []database.Value{database.Text(f.EQ)}, 1, nil
	}

	// IN list
	if len(f.IN) == 0 {
		return "", nil, 0, nil
	}
	if len(f.IN) > MaxINListSize {
		return "", nil, 0, fmt.Errorf("DID filter IN list exceeds maximum of %d values", MaxINListSize)
	}
	placeholders := r.db.Placeholders(len(f.IN), startPlaceholder)
	clause := fmt.Sprintf("did IN (%s)", placeholders)
	params := make([]database.Value, len(f.IN))
	for i, did := range f.IN {
		params[i] = database.Text(did)
	}
	return clause, params, len(f.IN), nil
}

func (r *RecordsRepository) buildExternalLabelFilterSetClause(filter ExternalLabelFilterSet, startPlaceholder int) (string, []database.Value, error) {
	if filter.IsEmpty() {
		return "", nil, nil
	}

	var clauses []string
	var params []database.Value
	placeholderIdx := startPlaceholder

	for _, subjectFilter := range []struct {
		subject externalLabelSubject
		alias   string
		filter  ExternalLabelRecordFilter
	}{
		{subject: externalLabelSubjectRecord, alias: "el_record", filter: filter.Record},
		{subject: externalLabelSubjectAuthor, alias: "el_author", filter: filter.Author},
	} {
		clause, clauseParams, consumed, err := r.buildExternalLabelSubjectFilterClause(subjectFilter.subject, subjectFilter.alias, subjectFilter.filter, placeholderIdx)
		if err != nil {
			return "", nil, err
		}
		if clause == "" {
			continue
		}
		clauses = append(clauses, clause)
		params = append(params, clauseParams...)
		placeholderIdx += consumed
	}

	return strings.Join(clauses, " AND "), params, nil
}

func (r *RecordsRepository) buildExternalLabelSubjectFilterClause(subject externalLabelSubject, aliasPrefix string, filter ExternalLabelRecordFilter, startPlaceholder int) (string, []database.Value, int, error) {
	if filter.IsEmpty() {
		return "", nil, 0, nil
	}

	var clauses []string
	var params []database.Value
	placeholderIdx := startPlaceholder

	if filter.Has != nil {
		clause, clauseParams, consumed, err := r.buildExternalLabelExistsSubquery(aliasPrefix+"_has", subject, *filter.Has, placeholderIdx)
		if err != nil {
			return "", nil, 0, err
		}
		clauses = append(clauses, fmt.Sprintf("EXISTS (%s)", clause))
		params = append(params, clauseParams...)
		placeholderIdx += consumed
	}

	if filter.None != nil {
		clause, clauseParams, consumed, err := r.buildExternalLabelExistsSubquery(aliasPrefix+"_none", subject, *filter.None, placeholderIdx)
		if err != nil {
			return "", nil, 0, err
		}
		clauses = append(clauses, fmt.Sprintf("NOT EXISTS (%s)", clause))
		params = append(params, clauseParams...)
		placeholderIdx += consumed
	}

	return strings.Join(clauses, " AND "), params, placeholderIdx - startPlaceholder, nil
}

func (r *RecordsRepository) buildExternalLabelExistsSubquery(alias string, subject externalLabelSubject, predicate ExternalLabelPredicate, startPlaceholder int) (string, []database.Value, int, error) {
	conditions, err := externalLabelSubjectConditions(alias, subject)
	if err != nil {
		return "", nil, 0, err
	}
	var params []database.Value
	placeholderIdx := startPlaceholder

	sourceConditions, sourceParams, consumed, err := r.buildExternalLabelStringFilterConditions(alias+".src", predicate.Sources, placeholderIdx)
	if err != nil {
		return "", nil, 0, err
	}
	conditions = append(conditions, sourceConditions...)
	params = append(params, sourceParams...)
	placeholderIdx += consumed

	valueConditions, valueParams, consumed, err := r.buildExternalLabelStringFilterConditions(alias+".val", predicate.Values, placeholderIdx)
	if err != nil {
		return "", nil, 0, err
	}
	conditions = append(conditions, valueConditions...)
	params = append(params, valueParams...)
	placeholderIdx += consumed

	if predicate.ActiveOnly {
		conditions = append(conditions, externalLabelActivePredicate(r.db, alias))
	}

	return fmt.Sprintf("SELECT 1 FROM external_label %s WHERE %s", alias, strings.Join(conditions, " AND ")), params, placeholderIdx - startPlaceholder, nil
}

func externalLabelSubjectConditions(alias string, subject externalLabelSubject) ([]string, error) {
	switch subject {
	case externalLabelSubjectRecord:
		return []string{
			fmt.Sprintf("%s.uri = record.uri", alias),
			fmt.Sprintf("(%s.cid IS NULL OR %s.cid = record.cid)", alias, alias),
		}, nil
	case externalLabelSubjectAuthor:
		return []string{
			fmt.Sprintf("%s.uri = record.did", alias),
			fmt.Sprintf("%s.cid IS NULL", alias),
		}, nil
	default:
		return nil, fmt.Errorf("unknown external label subject %d", subject)
	}
}

func (r *RecordsRepository) buildExternalLabelStringFilterConditions(column string, filters []ExternalLabelStringFilter, startPlaceholder int) ([]string, []database.Value, int, error) {
	if len(filters) == 0 {
		return nil, nil, 0, nil
	}

	var conditions []string
	var params []database.Value
	placeholderIdx := startPlaceholder
	for _, filter := range filters {
		switch filter.Operator {
		case "eq":
			value, err := externalLabelStringFilterValue(filter.Value)
			if err != nil {
				return nil, nil, 0, err
			}
			conditions = append(conditions, fmt.Sprintf("%s = %s", column, r.db.Placeholder(placeholderIdx)))
			params = append(params, database.Text(value))
			placeholderIdx++
		case "neq":
			value, err := externalLabelStringFilterValue(filter.Value)
			if err != nil {
				return nil, nil, 0, err
			}
			conditions = append(conditions, fmt.Sprintf("%s != %s", column, r.db.Placeholder(placeholderIdx)))
			params = append(params, database.Text(value))
			placeholderIdx++
		case "in":
			values, err := externalLabelStringFilterList(filter.Value)
			if err != nil {
				return nil, nil, 0, err
			}
			if len(values) == 0 {
				conditions = append(conditions, "1 = 0")
				continue
			}
			if len(values) > MaxINListSize {
				return nil, nil, 0, fmt.Errorf("external label filter IN list exceeds maximum of %d values", MaxINListSize)
			}
			conditions = append(conditions, fmt.Sprintf("%s IN (%s)", column, r.db.Placeholders(len(values), placeholderIdx)))
			for _, value := range values {
				params = append(params, database.Text(value))
				placeholderIdx++
			}
		case "contains":
			value, err := externalLabelStringFilterValue(filter.Value)
			if err != nil {
				return nil, nil, 0, err
			}
			likeOp := "LIKE"
			if r.db.Dialect() == database.PostgreSQL {
				likeOp = "ILIKE"
			}
			conditions = append(conditions, fmt.Sprintf("%s %s %s ESCAPE '\\'", column, likeOp, r.db.Placeholder(placeholderIdx)))
			params = append(params, database.Text("%"+escapeLIKE(value)+"%"))
			placeholderIdx++
		case "startsWith":
			value, err := externalLabelStringFilterValue(filter.Value)
			if err != nil {
				return nil, nil, 0, err
			}
			likeOp := "LIKE"
			if r.db.Dialect() == database.PostgreSQL {
				likeOp = "ILIKE"
			}
			conditions = append(conditions, fmt.Sprintf("%s %s %s ESCAPE '\\'", column, likeOp, r.db.Placeholder(placeholderIdx)))
			params = append(params, database.Text(escapeLIKE(value)+"%"))
			placeholderIdx++
		case "isNull":
			isNull, _ := filter.Value.(bool)
			if isNull {
				conditions = append(conditions, fmt.Sprintf("%s IS NULL", column))
			} else {
				conditions = append(conditions, fmt.Sprintf("%s IS NOT NULL", column))
			}
		default:
			return nil, nil, 0, fmt.Errorf("unsupported external label filter operator %q", filter.Operator)
		}
	}

	return conditions, params, placeholderIdx - startPlaceholder, nil
}

func externalLabelStringFilterValue(value interface{}) (string, error) {
	valueString, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("external label string filter value must be a string, got %T", value)
	}
	return valueString, nil
}

func externalLabelStringFilterList(value interface{}) ([]string, error) {
	switch typedValue := value.(type) {
	case []interface{}:
		values := make([]string, 0, len(typedValue))
		for _, item := range typedValue {
			valueString, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("external label IN filter value must contain strings, got %T", item)
			}
			values = append(values, valueString)
		}
		return values, nil
	case []string:
		return typedValue, nil
	default:
		return nil, fmt.Errorf("external label IN filter value must be a string list, got %T", value)
	}
}

func (r *RecordsRepository) appendConvertedParams(args []any, params []database.Value) []any {
	for _, p := range params {
		args = append(args, r.db.ConvertParams([]database.Value{p})[0])
	}
	return args
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
	indexedAtExpr := r.normalizedIndextAtExpr()

	// Keyset cursor condition
	if afterTimestamp != "" && afterURI != "" {
		p2 := r.db.Placeholder(nextPlaceholder)
		p3 := r.db.Placeholder(nextPlaceholder + 1)
		p4 := r.db.Placeholder(nextPlaceholder + 2)
		whereParts = append(whereParts, fmt.Sprintf("(%s < %s OR (%s = %s AND uri < %s))",
			indexedAtExpr, r.keysetCursorValueExpr("indexed_at", p2),
			indexedAtExpr, r.keysetCursorValueExpr("indexed_at", p3), p4))
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
	if didClause, didParams, _, err := r.buildDIDFilterClause(didFilter, nextPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to build DID filter clause: %w", err)
	} else if didClause != "" {
		whereParts = append(whereParts, didClause)
		for _, p := range didParams {
			args = append(args, r.db.ConvertParams([]database.Value{p})[0])
		}
	}

	whereClause := strings.Join(whereParts, " AND ")
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s DESC, uri DESC LIMIT %d",
		r.recordColumns(), whereClause, indexedAtExpr, limit)

	if err := r.validateSQLiteAggregateParameterCount(len(args)); err != nil {
		return nil, err
	}

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

// normalizedIndextAtExpr returns the record-side indexed_at expression used for
// ordering and keyset comparisons. SQLite stores indexed_at as TEXT and may
// contain mixed formats (e.g. "YYYY-MM-DD HH:MM:SS" and RFC3339), so values are
// normalized to a canonical sortable UTC representation.
func (r *RecordsRepository) normalizedIndextAtExpr() string {
	switch r.db.Dialect() {
	case database.PostgreSQL:
		return "indexed_at"
	default:
		return "strftime('%Y-%m-%dT%H:%M:%fZ', indexed_at)"
	}
}

// keysetSortFieldExpr returns the SQL expression used on the record side for
// keyset comparisons.
func (r *RecordsRepository) keysetSortFieldExpr(sortField string) string {
	if sortField == "indexed_at" {
		return r.normalizedIndextAtExpr()
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
		return fmt.Sprintf("%s DESC, uri DESC", r.normalizedIndextAtExpr())
	}

	dir := sort.Direction
	if dir != "ASC" && dir != "DESC" {
		dir = "DESC"
	}

	var fieldExpr string
	switch {
	case sort.Field == "indexed_at":
		fieldExpr = r.normalizedIndextAtExpr()
	case directSortColumns[sort.Field]:
		fieldExpr = sort.Field
	default:
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
	return r.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{}, sort, limit, afterCursorValues)
}

// GetByCollectionSortedWithKeysetCursorAndExternalLabels retrieves records for a
// collection with JSON field filters, DID filters, record-level external-label
// filters, a configurable sort order, and keyset-based pagination.
func (r *RecordsRepository) GetByCollectionSortedWithKeysetCursorAndExternalLabels(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilter ExternalLabelRecordFilter,
	sort *SortOption,
	limit int,
	afterCursorValues []string,
) ([]*Record, error) {
	return r.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{Record: externalLabelFilter}, sort, limit, afterCursorValues)
}

// GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters retrieves records
// for a collection with JSON field filters, DID filters, record-level and
// author-level external-label filters, a configurable sort order, and
// keyset-based pagination.
func (r *RecordsRepository) GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	afterCursorValues []string,
) ([]*Record, error) {
	return r.getByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, sort, limit, afterCursorValues, false)
}

// GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters retrieves
// records for typed GraphQL collection connections, excluding raw records that
// are not valid against the saved lexicon used to generate the typed schema.
func (r *RecordsRepository) GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	afterCursorValues []string,
) ([]*Record, error) {
	return r.getByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, sort, limit, afterCursorValues, true)
}

func (r *RecordsRepository) getByCollectionSortedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	afterCursorValues []string,
	validOnly bool,
) ([]*Record, error) {
	var whereParts []string
	var args []any

	// collection = ? is always param 1
	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	args = append(args, collection)

	nextPlaceholder := 2
	if validOnly {
		whereParts = append(whereParts, fmt.Sprintf("validation_status = %s", r.db.Placeholder(nextPlaceholder)))
		args = append(args, string(validation.StatusValid))
		nextPlaceholder++
	}

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
	if didClause, didParams, consumed, err := r.buildDIDFilterClause(didFilter, nextPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to build DID filter clause: %w", err)
	} else if didClause != "" {
		whereParts = append(whereParts, didClause)
		args = r.appendConvertedParams(args, didParams)
		nextPlaceholder += consumed
	}

	// External label filters
	if labelClause, labelParams, err := r.buildExternalLabelFilterSetClause(externalLabelFilters, nextPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to build external label filter clause: %w", err)
	} else if labelClause != "" {
		whereParts = append(whereParts, labelClause)
		args = r.appendConvertedParams(args, labelParams)
	}

	whereClause := strings.Join(whereParts, " AND ")
	orderBy := r.buildSortExpr(sort)
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s LIMIT %d",
		r.recordColumns(), whereClause, orderBy, limit)

	if err := r.validateSQLiteAggregateParameterCount(len(args)); err != nil {
		return nil, err
	}

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
	return r.GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{}, sort, limit, beforeCursorValues)
}

// GetByCollectionReversedWithKeysetCursorAndExternalLabels retrieves records for
// backward pagination while applying JSON field, DID, and record-level
// external-label filters before LIMIT so Relay pagination stays correct.
func (r *RecordsRepository) GetByCollectionReversedWithKeysetCursorAndExternalLabels(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilter ExternalLabelRecordFilter,
	sort *SortOption,
	limit int,
	beforeCursorValues []string,
) ([]*Record, error) {
	return r.GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{Record: externalLabelFilter}, sort, limit, beforeCursorValues)
}

// GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters retrieves
// records for backward pagination while applying JSON field, DID, record-level,
// and author-level external-label filters before LIMIT so Relay pagination stays
// correct.
func (r *RecordsRepository) GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	beforeCursorValues []string,
) ([]*Record, error) {
	return r.getByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, sort, limit, beforeCursorValues, false)
}

// GetValidByCollectionReversedWithKeysetCursorAndExternalLabelFilters retrieves
// backward pages for typed GraphQL collection connections, excluding records
// hidden by the validation gate.
func (r *RecordsRepository) GetValidByCollectionReversedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	beforeCursorValues []string,
) ([]*Record, error) {
	return r.getByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, sort, limit, beforeCursorValues, true)
}

func (r *RecordsRepository) getByCollectionReversedWithKeysetCursorAndExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	sort *SortOption,
	limit int,
	beforeCursorValues []string,
	validOnly bool,
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
	if validOnly {
		whereParts = append(whereParts, fmt.Sprintf("validation_status = %s", r.db.Placeholder(nextPlaceholder)))
		args = append(args, string(validation.StatusValid))
		nextPlaceholder++
	}

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
	if didClause, didParams, consumed, err := r.buildDIDFilterClause(didFilter, nextPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to build DID filter clause: %w", err)
	} else if didClause != "" {
		whereParts = append(whereParts, didClause)
		args = r.appendConvertedParams(args, didParams)
		nextPlaceholder += consumed
	}

	// External label filters
	if labelClause, labelParams, err := r.buildExternalLabelFilterSetClause(externalLabelFilters, nextPlaceholder); err != nil {
		return nil, fmt.Errorf("failed to build external label filter clause: %w", err)
	} else if labelClause != "" {
		whereParts = append(whereParts, labelClause)
		args = r.appendConvertedParams(args, labelParams)
	}

	whereClause := strings.Join(whereParts, " AND ")
	orderBy := r.buildSortExpr(reversedSort)
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE %s ORDER BY %s LIMIT %d",
		r.recordColumns(), whereClause, orderBy, limit)

	if err := r.validateSQLiteAggregateParameterCount(len(args)); err != nil {
		return nil, err
	}

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
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE did = %s ORDER BY %s DESC",
		r.recordColumns(), r.db.Placeholder(1), r.normalizedIndextAtExpr())

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

// DeleteByDID removes all records for a specific DID.
func (r *RecordsRepository) DeleteByDID(ctx context.Context, did string) error {
	sqlStr := fmt.Sprintf("DELETE FROM record WHERE did = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(did)})
	return err
}

// PurgeActorData atomically removes all records and the actor row for a DID.
func (r *RecordsRepository) PurgeActorData(ctx context.Context, did string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback is a no-op after successful commit.

	deleteRecordsSQL := fmt.Sprintf("DELETE FROM record WHERE did = %s", r.db.Placeholder(1))
	if _, err := tx.ExecContext(ctx, deleteRecordsSQL, did); err != nil {
		return fmt.Errorf("failed to delete records by did: %w", err)
	}

	deleteActorSQL := fmt.Sprintf("DELETE FROM actor WHERE did = %s", r.db.Placeholder(1))
	if _, err := tx.ExecContext(ctx, deleteActorSQL, did); err != nil {
		return fmt.Errorf("failed to delete actor by did: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
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

// GetCountByDID returns the number of records for a specific DID.
func (r *RecordsRepository) GetCountByDID(ctx context.Context, did string) (int64, error) {
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM record WHERE did = %s", r.db.Placeholder(1))
	var count int64
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(did)}, &count)
	return count, err
}

// GetCollectionCountFiltered returns the count with optional DID and field filters applied.
func (r *RecordsRepository) GetCollectionCountFiltered(
	ctx context.Context, collection string, filters []FieldFilter, didFilter DIDFilter,
) (int64, error) {
	return r.GetCollectionCountFilteredWithExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{})
}

// GetCollectionCountFilteredWithExternalLabels returns the count with optional
// DID, JSON field, and record-level external-label filters applied.
func (r *RecordsRepository) GetCollectionCountFilteredWithExternalLabels(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilter ExternalLabelRecordFilter,
) (int64, error) {
	return r.GetCollectionCountFilteredWithExternalLabelFilters(ctx, collection, filters, didFilter, ExternalLabelFilterSet{Record: externalLabelFilter})
}

// GetCollectionCountFilteredWithExternalLabelFilters returns the count with
// optional DID, JSON field, record-level external-label, and author-level
// external-label filters applied.
func (r *RecordsRepository) GetCollectionCountFilteredWithExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
) (int64, error) {
	return r.getCollectionCountFilteredWithExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, false)
}

// GetValidCollectionCountFilteredWithExternalLabelFilters returns the count used
// by typed GraphQL collection connections, excluding records hidden by the validation gate.
func (r *RecordsRepository) GetValidCollectionCountFilteredWithExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
) (int64, error) {
	return r.getCollectionCountFilteredWithExternalLabelFilters(ctx, collection, filters, didFilter, externalLabelFilters, true)
}

func (r *RecordsRepository) getCollectionCountFilteredWithExternalLabelFilters(
	ctx context.Context,
	collection string,
	filters []FieldFilter,
	didFilter DIDFilter,
	externalLabelFilters ExternalLabelFilterSet,
	validOnly bool,
) (int64, error) {
	var whereParts []string
	var params []database.Value

	whereParts = append(whereParts, fmt.Sprintf("collection = %s", r.db.Placeholder(1)))
	params = append(params, database.Text(collection))

	nextPlaceholder := 2
	if validOnly {
		whereParts = append(whereParts, fmt.Sprintf("validation_status = %s", r.db.Placeholder(nextPlaceholder)))
		params = append(params, database.Text(string(validation.StatusValid)))
		nextPlaceholder++
	}

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
	if didClause, didParams, consumed, err := r.buildDIDFilterClause(didFilter, nextPlaceholder); err != nil {
		return 0, fmt.Errorf("failed to build DID filter clause: %w", err)
	} else if didClause != "" {
		whereParts = append(whereParts, didClause)
		params = append(params, didParams...)
		nextPlaceholder += consumed
	}

	// External label filters
	if labelClause, labelParams, err := r.buildExternalLabelFilterSetClause(externalLabelFilters, nextPlaceholder); err != nil {
		return 0, fmt.Errorf("failed to build external label filter clause: %w", err)
	} else if labelClause != "" {
		whereParts = append(whereParts, labelClause)
		params = append(params, labelParams...)
	}

	whereClause := strings.Join(whereParts, " AND ")
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM record WHERE %s", whereClause)

	if err := r.validateSQLiteAggregateParameterCount(len(params)); err != nil {
		return 0, err
	}

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
		indexedAtExpr := r.normalizedIndextAtExpr()
		conditions = append(conditions, fmt.Sprintf(
			"(%s < %s OR (%s = %s AND uri < %s))",
			indexedAtExpr,
			r.keysetCursorValueExpr("indexed_at", r.db.Placeholder(paramIdx)),
			indexedAtExpr,
			r.keysetCursorValueExpr("indexed_at", r.db.Placeholder(paramIdx+1)),
			r.db.Placeholder(paramIdx+2),
		))
		params = append(params, database.Text(afterTimestamp), database.Text(afterTimestamp), database.Text(afterURI))
	}

	whereClause := "WHERE " + strings.Join(conditions, " AND ")
	sqlStr := fmt.Sprintf("SELECT %s FROM record %s ORDER BY %s DESC, uri DESC LIMIT %d",
		r.recordColumns(), whereClause, r.normalizedIndextAtExpr(), limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, fmt.Errorf("failed to search records: %w", err)
	}
	defer rows.Close()

	return scanRecords(rows)
}

// Helper functions

func (r *RecordsRepository) getCIDAndRecordCreatedAtByURI(ctx context.Context, uri string) (string, bool, error) {
	var cid string
	var recordCreatedAt sql.NullString
	recordCreatedAtExpr := "record_created_at"
	if r.db.Dialect() == database.PostgreSQL {
		recordCreatedAtExpr = "record_created_at::text"
	}
	err := r.db.QueryRow(ctx, fmt.Sprintf("SELECT cid, %s FROM record WHERE uri = %s", recordCreatedAtExpr, r.db.Placeholder(1)),
		[]database.Value{database.Text(uri)}, &cid, &recordCreatedAt)
	return cid, recordCreatedAt.Valid && recordCreatedAt.String != "", err
}

func (r *RecordsRepository) fillMissingRecordCreatedAt(ctx context.Context, uri string, value database.Value) error {
	if _, ok := value.(database.NullValue); ok {
		return nil
	}
	_, err := r.db.Exec(ctx,
		fmt.Sprintf("UPDATE record SET record_created_at = %s WHERE uri = %s AND record_created_at IS NULL", r.recordCreatedAtCursorValueExpr(r.db.Placeholder(1)), r.db.Placeholder(2)),
		[]database.Value{value, database.Text(uri)})
	return err
}

func scanRecords(rows *sql.Rows) ([]*Record, error) {
	var records []*Record
	for rows.Next() {
		var rec Record
		var indexedAtStr string
		var validationStatus string
		var validationError, validatedAtStr, lexiconHash sql.NullString
		if err := rows.Scan(&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey,
			&validationStatus, &validationError, &validatedAtStr, &lexiconHash); err != nil {
			return nil, err
		}
		// Try various timestamp formats
		rec.IndexedAt = atproto.ParseTimestamp(indexedAtStr)
		rec.ValidationStatus = validation.Status(validationStatus)
		applyRecordValidationNulls(&rec, validationError, validatedAtStr, lexiconHash)
		records = append(records, &rec)
	}
	return records, rows.Err()
}

func applyRecordValidationNulls(rec *Record, validationError, validatedAtStr, lexiconHash sql.NullString) {
	if validationError.Valid {
		rec.ValidationError = validationError.String
	}
	if lexiconHash.Valid {
		rec.LexiconHash = lexiconHash.String
	}
	if validatedAtStr.Valid {
		validatedAt := atproto.ParseTimestamp(validatedAtStr.String)
		if !validatedAt.IsZero() {
			rec.ValidatedAt = &validatedAt
		}
	}
}

func scanRecordTimelineRecords(rows *sql.Rows) ([]*RecordTimelineRecord, error) {
	var records []*RecordTimelineRecord
	for rows.Next() {
		var rec RecordTimelineRecord
		var indexedAtStr string
		var recordCreatedAtStr string
		if err := rows.Scan(&rec.URI, &rec.CID, &rec.DID, &rec.Collection, &rec.JSON, &indexedAtStr, &rec.RKey, &recordCreatedAtStr); err != nil {
			return nil, err
		}
		rec.IndexedAt = atproto.ParseTimestamp(indexedAtStr)
		rec.RecordCreatedAt = atproto.ParseTimestamp(recordCreatedAtStr)
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
