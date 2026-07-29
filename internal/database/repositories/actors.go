package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

const missingHandlePredicateSQL = "(handle IS NULL OR TRIM(handle) = '' OR handle = did)"

// Actor represents an AT Protocol user/actor.
type Actor struct {
	DID       string
	Handle    string
	IndexedAt time.Time
}

// ActorsRepository handles actor persistence.
type ActorsRepository struct {
	db database.Executor
}

// NewActorsRepository creates a new actors repository.
func NewActorsRepository(db database.Executor) *ActorsRepository {
	return &ActorsRepository{db: db}
}

// Ensure inserts an actor when it does not exist without changing identity
// metadata already stored for that DID.
func (r *ActorsRepository) Ensure(ctx context.Context, did string) error {
	p1 := r.db.Placeholder(1)

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, NULL, NOW())
			ON CONFLICT(did) DO NOTHING`, p1)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, NULL, datetime('now'))
			ON CONFLICT(did) DO NOTHING`, p1)
	}

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(did)})
	return err
}

// UpsertIdentity inserts an actor or replaces its handle with identity metadata
// received from an authoritative identity source.
func (r *ActorsRepository) UpsertIdentity(ctx context.Context, did, handle string) error {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, %s, NOW())
			ON CONFLICT(did) DO UPDATE SET
				handle = EXCLUDED.handle,
				indexed_at = NOW()`, p1, p2)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, %s, datetime('now'))
			ON CONFLICT(did) DO UPDATE SET
				handle = excluded.handle,
				indexed_at = datetime('now')`, p1, p2)
	}

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(did),
		database.Text(handle),
	})
	return err
}

// Upsert inserts or updates an actor identity. New ingestion code should use
// Ensure when it only needs to establish that an actor exists.
func (r *ActorsRepository) Upsert(ctx context.Context, did, handle string) error {
	return r.UpsertIdentity(ctx, did, handle)
}

// SetHandleIfMissing updates an actor only while its handle is still absent.
// This prevents startup reconciliation from overwriting a newer identity event.
func (r *ActorsRepository) SetHandleIfMissing(ctx context.Context, did, handle string) (bool, error) {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	sqlStr := fmt.Sprintf(`UPDATE actor
		SET handle = %s, indexed_at = %s
		WHERE did = %s AND %s`, p1, r.db.Now(), p2, missingHandlePredicateSQL)

	result, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(handle),
		database.Text(did),
	})
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

// ActorData holds DID and Handle for batch operations.
type ActorData struct {
	DID    string
	Handle string
}

// BatchUpsert inserts or updates multiple actors efficiently.
func (r *ActorsRepository) BatchUpsert(ctx context.Context, actors []ActorData) error {
	if len(actors) == 0 {
		return nil
	}

	// Process in batches to stay within SQL parameter limits
	batchSize := BatchInsertSize
	for i := 0; i < len(actors); i += batchSize {
		end := i + batchSize
		if end > len(actors) {
			end = len(actors)
		}
		batch := actors[i:end]

		if err := r.batchUpsertChunk(ctx, batch); err != nil {
			return err
		}
	}

	return nil
}

func (r *ActorsRepository) batchUpsertChunk(ctx context.Context, actors []ActorData) error {
	// Build value placeholders
	var valueSets []string
	var params []database.Value

	for i, actor := range actors {
		base := i * 2
		var valueSet string

		if r.db.Dialect() == database.PostgreSQL {
			valueSet = fmt.Sprintf("(%s, %s, NOW())",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2))
		} else {
			valueSet = fmt.Sprintf("(%s, %s, datetime('now'))",
				r.db.Placeholder(base+1),
				r.db.Placeholder(base+2))
		}
		valueSets = append(valueSets, valueSet)

		params = append(params,
			database.Text(actor.DID),
			database.Text(actor.Handle),
		)
	}

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES %s
			ON CONFLICT(did) DO UPDATE SET
				handle = CASE WHEN EXCLUDED.handle = '' THEN actor.handle ELSE EXCLUDED.handle END,
				indexed_at = NOW()`, strings.Join(valueSets, ", "))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES %s
			ON CONFLICT(did) DO UPDATE SET
				handle = CASE WHEN excluded.handle = '' THEN actor.handle ELSE excluded.handle END,
				indexed_at = datetime('now')`, strings.Join(valueSets, ", "))
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// GetByDID retrieves an actor by their DID.
func (r *ActorsRepository) GetByDID(ctx context.Context, did string) (*Actor, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf("SELECT did, COALESCE(handle, ''), indexed_at::text FROM actor WHERE did = %s",
			r.db.Placeholder(1))
	default:
		sqlStr = fmt.Sprintf("SELECT did, COALESCE(handle, ''), indexed_at FROM actor WHERE did = %s",
			r.db.Placeholder(1))
	}

	var actor Actor
	var indexedAtStr string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(did)},
		&actor.DID, &actor.Handle, &indexedAtStr)
	if err != nil {
		return nil, err
	}

	actor.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	return &actor, nil
}

// GetByDIDs retrieves actors keyed by DID. Missing actors are omitted.
func (r *ActorsRepository) GetByDIDs(ctx context.Context, dids []string) (map[string]*Actor, error) {
	actors := make(map[string]*Actor)
	if len(dids) == 0 {
		return actors, nil
	}

	params := make([]database.Value, len(dids))
	for i, did := range dids {
		params[i] = database.Text(did)
	}

	indexedAt := "indexed_at"
	if r.db.Dialect() == database.PostgreSQL {
		indexedAt = "indexed_at::text"
	}
	sqlStr := fmt.Sprintf(
		"SELECT did, COALESCE(handle, ''), %s FROM actor WHERE did IN (%s)",
		indexedAt,
		r.db.Placeholders(len(dids), 1),
	)
	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var actor Actor
		var indexedAtStr string
		if err := rows.Scan(&actor.DID, &actor.Handle, &indexedAtStr); err != nil {
			return nil, err
		}
		actor.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
		actors[actor.DID] = &actor
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return actors, nil
}

// ListDIDsMissingHandle returns a stable page of actors whose handle has not
// been populated yet.
func (r *ActorsRepository) ListDIDsMissingHandle(ctx context.Context, afterDID string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 100
	}

	sqlStr := fmt.Sprintf(`SELECT did FROM actor
		WHERE %s AND did > %s
		ORDER BY did ASC
		LIMIT %s`, missingHandlePredicateSQL, r.db.Placeholder(1), r.db.Placeholder(2))
	params := []database.Value{database.Text(afterDID), database.Int(int64(limit))}
	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var dids []string
	for rows.Next() {
		var did string
		if err := rows.Scan(&did); err != nil {
			return nil, err
		}
		dids = append(dids, did)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return dids, nil
}

// GetByHandle retrieves an actor by their handle.
func (r *ActorsRepository) GetByHandle(ctx context.Context, handle string) (*Actor, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf("SELECT did, handle, indexed_at::text FROM actor WHERE handle = %s",
			r.db.Placeholder(1))
	default:
		sqlStr = fmt.Sprintf("SELECT did, handle, indexed_at FROM actor WHERE handle = %s",
			r.db.Placeholder(1))
	}

	var actor Actor
	var indexedAtStr string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(handle)},
		&actor.DID, &actor.Handle, &indexedAtStr)
	if err != nil {
		return nil, err
	}

	actor.IndexedAt, _ = time.Parse(time.RFC3339, indexedAtStr)
	return &actor, nil
}

// GetCount returns the total number of actors.
func (r *ActorsRepository) GetCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM actor", nil, &count)
	return count, err
}

// DeleteAll removes all actors.
func (r *ActorsRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, "DELETE FROM actor", nil)
	return err
}

// DeleteByDID removes an actor by DID.
func (r *ActorsRepository) DeleteByDID(ctx context.Context, did string) error {
	sqlStr := fmt.Sprintf("DELETE FROM actor WHERE did = %s", r.db.Placeholder(1))
	if _, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(did)}); err != nil {
		return fmt.Errorf("delete actor by DID %q: %w", did, err)
	}
	return nil
}

// Exists checks if an actor exists by DID.
func (r *ActorsRepository) Exists(ctx context.Context, did string) (bool, error) {
	var count int64
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM actor WHERE did = %s", r.db.Placeholder(1))
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(did)}, &count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}
