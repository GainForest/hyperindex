// Package repositories contains data access layer implementations.
package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hypergoat/internal/database"
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

// CollectionStat represents statistics for a collection.
type CollectionStat struct {
	Collection string
	Count      int64
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

	if existingCID == cid {
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
	tx, err := r.db.DB().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Rollback is a no-op if Commit succeeds

	// Process in batches of 100 (5 params per record)
	batchSize := 100
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

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, convertToAny(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
}

// GetByCollection retrieves records for a specific collection.
func (r *RecordsRepository) GetByCollection(ctx context.Context, collection string, limit int) ([]*Record, error) {
	sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE collection = %s ORDER BY indexed_at DESC LIMIT %d",
		r.recordColumns(), r.db.Placeholder(1), limit)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, collection)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRecords(rows)
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

// GetCIDsByURIs returns a map of URI -> CID for records that exist.
// Used for deduplication before batch insert.
func (r *RecordsRepository) GetCIDsByURIs(ctx context.Context, uris []string) (map[string]string, error) {
	if len(uris) == 0 {
		return make(map[string]string), nil
	}

	result := make(map[string]string)

	// Process in batches of 900 to avoid SQL parameter limits
	batchSize := 900
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

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, convertToAny(params)...)
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
	batchSize := 900
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

		rows, err := r.db.DB().QueryContext(ctx, sqlStr, convertToAny(params)...)
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
		rec.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
		records = append(records, &rec)
	}
	return records, rows.Err()
}

func convertToAny(params []database.Value) []any {
	args := make([]any, len(params))
	for i, p := range params {
		switch v := p.(type) {
		case database.TextValue:
			args[i] = string(v)
		case database.IntValue:
			args[i] = int64(v)
		default:
			args[i] = p
		}
	}
	return args
}
