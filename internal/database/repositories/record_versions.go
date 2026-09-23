package repositories

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// Record version actions stored in record_version.action.
const (
	// RecordVersionBaseline is the version that was current when history was
	// switched on for a collection (seeded from the record table).
	RecordVersionBaseline = "baseline"
	RecordVersionCreate   = "create"
	RecordVersionUpdate   = "update"
	RecordVersionDelete   = "delete"

	// MaxRecordHistoryPageSize bounds one recordHistory response.
	MaxRecordHistoryPageSize = 500
)

// RecordVersion is one observed version of a record, or a delete tombstone.
type RecordVersion struct {
	ID         int64
	URI        string
	CID        string
	DID        string
	Collection string
	Action     string
	// JSON is the record body; nil for delete tombstones.
	JSON *string
	// Live reports whether Tap delivered the event from the live stream
	// (false for resync/backfill deliveries); nil for baseline rows.
	Live       *bool
	ObservedAt time.Time
}

// RecordVersionWrite is a version to append.
type RecordVersionWrite struct {
	URI        string
	CID        string
	DID        string
	Collection string
	Action     string
	JSON       *string
	Live       *bool
}

// CollectionMatcher selects which collections keep version history. Entries
// are exact NSIDs or `prefix.*` patterns (e.g. `app.gainforest.dwc.*`).
type CollectionMatcher struct {
	exact    map[string]struct{}
	prefixes []string
}

// NewCollectionMatcher parses a comma-separated collection list.
func NewCollectionMatcher(list string) CollectionMatcher {
	m := CollectionMatcher{exact: map[string]struct{}{}}
	for _, raw := range strings.Split(list, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if strings.HasSuffix(entry, ".*") {
			m.prefixes = append(m.prefixes, strings.TrimSuffix(entry, "*"))
			continue
		}
		m.exact[entry] = struct{}{}
	}
	return m
}

// Empty reports whether history is switched off.
func (m CollectionMatcher) Empty() bool {
	return len(m.exact) == 0 && len(m.prefixes) == 0
}

// Matches reports whether a collection keeps version history.
func (m CollectionMatcher) Matches(collection string) bool {
	if _, ok := m.exact[collection]; ok {
		return true
	}
	for _, prefix := range m.prefixes {
		if strings.HasPrefix(collection, prefix) {
			return true
		}
	}
	return false
}

// sqlPredicate renders the matcher as a WHERE predicate on `column`, with
// placeholders starting after `base`.
func (m CollectionMatcher) sqlPredicate(db database.Executor, column string, base int) (string, []database.Value) {
	var parts []string
	var params []database.Value
	for collection := range m.exact {
		params = append(params, database.Text(collection))
		parts = append(parts, fmt.Sprintf("%s = %s", column, db.Placeholder(base+len(params))))
	}
	for _, prefix := range m.prefixes {
		params = append(params, database.Text(escapeLike(prefix)+"%"))
		parts = append(parts, fmt.Sprintf("%s LIKE %s ESCAPE '\\'", column, db.Placeholder(base+len(params))))
	}
	if len(parts) == 0 {
		return "1 = 0", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", params
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// RecordVersionsRepository stores and reads record version history.
type RecordVersionsRepository struct {
	db database.Executor
}

// NewRecordVersionsRepository creates a record version repository.
func NewRecordVersionsRepository(db database.Executor) *RecordVersionsRepository {
	return &RecordVersionsRepository{db: db}
}

// versionKey is the dedupe identity of a version: its CID; for Tap events
// without a CID, a hash of the body; for tombstones, the deleted CID.
func versionKey(v RecordVersionWrite) string {
	if v.Action == RecordVersionDelete {
		return "delete:" + v.CID
	}
	if v.CID != "" {
		return v.CID
	}
	body := ""
	if v.JSON != nil {
		body = *v.JSON
	}
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Append records one version. A version already stored for the URI (same
// version key) is left untouched, so redelivered and resynced events,
// including repeated deletes, are no-ops.
func (r *RecordVersionsRepository) Append(ctx context.Context, v RecordVersionWrite) error {
	if v.URI == "" || v.DID == "" || v.Collection == "" {
		return fmt.Errorf("record version requires uri, did and collection")
	}
	switch v.Action {
	case RecordVersionCreate, RecordVersionUpdate, RecordVersionDelete:
	default:
		return fmt.Errorf("unsupported record version action %q", v.Action)
	}
	jsonPlaceholder := r.db.Placeholder(7)
	if r.db.Dialect() == database.PostgreSQL {
		jsonPlaceholder += "::jsonb"
	}
	sqlStr := fmt.Sprintf(`INSERT INTO record_version (uri, cid, version_key, did, collection, action, json, live)
		VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
		ON CONFLICT DO NOTHING`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3), r.db.Placeholder(4), r.db.Placeholder(5),
		r.db.Placeholder(6), jsonPlaceholder, r.db.Placeholder(8))
	live := database.Value(database.Null())
	if v.Live != nil {
		live = database.Bool(*v.Live)
	}
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(v.URI),
		database.Text(v.CID),
		database.Text(versionKey(v)),
		database.Text(v.DID),
		database.Text(v.Collection),
		database.Text(v.Action),
		database.NullableText(v.JSON),
		live,
	})
	if err != nil {
		return fmt.Errorf("append record version for %s: %w", v.URI, err)
	}
	return nil
}

// SeedBaseline adds a `baseline` version for every current record in a
// matched collection that has no history yet, stamped with the record's
// indexed_at. Idempotent: records that already have versions are skipped, so
// running it on every startup only seeds newly opted-in collections.
func (r *RecordVersionsRepository) SeedBaseline(ctx context.Context, matcher CollectionMatcher) (int64, error) {
	if matcher.Empty() {
		return 0, nil
	}
	predicate, params := matcher.sqlPredicate(r.db, "rec.collection", 0)
	observedAt := "rec.indexed_at"
	if r.db.Dialect() == database.SQLite {
		observedAt = "strftime('%Y-%m-%dT%H:%M:%SZ', rec.indexed_at)"
	}
	sqlStr := fmt.Sprintf(`INSERT INTO record_version (uri, cid, version_key, did, collection, action, json, live, observed_at)
		SELECT rec.uri, rec.cid, CASE WHEN rec.cid = '' THEN 'baseline' ELSE rec.cid END, rec.did, rec.collection, '%s', rec.json, NULL, %s
		FROM record rec
		WHERE %s
		  AND NOT EXISTS (SELECT 1 FROM record_version v WHERE v.uri = rec.uri)
		ON CONFLICT DO NOTHING`, RecordVersionBaseline, observedAt, predicate)
	result, err := r.db.Exec(ctx, sqlStr, params)
	if err != nil {
		return 0, fmt.Errorf("seed record version baseline: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count seeded record versions: %w", err)
	}
	return inserted, nil
}

// ListByURI returns up to `limit` of a record's versions oldest first,
// starting after version id `afterID` (0 for the first page). Callers page by
// passing the last returned id.
func (r *RecordVersionsRepository) ListByURI(ctx context.Context, uri string, afterID int64, limit int) ([]RecordVersion, error) {
	if limit <= 0 || limit > MaxRecordHistoryPageSize {
		return nil, fmt.Errorf("record history page size must be between 1 and %d", MaxRecordHistoryPageSize)
	}
	jsonExpr, liveExpr, observedExpr := "json", "live", "observed_at"
	if r.db.Dialect() == database.PostgreSQL {
		jsonExpr, liveExpr, observedExpr = "json::text", "CASE WHEN live IS NULL THEN NULL WHEN live THEN 1 ELSE 0 END", "observed_at::text"
	}
	sqlStr := fmt.Sprintf(`SELECT id, uri, cid, did, collection, action, %s, %s, %s
		FROM record_version
		WHERE uri = %s AND id > %s
		ORDER BY id ASC
		LIMIT %d`, jsonExpr, liveExpr, observedExpr, r.db.Placeholder(1), r.db.Placeholder(2), limit)
	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(uri), database.Int(afterID)})...)
	if err != nil {
		return nil, fmt.Errorf("list record versions for %s: %w", uri, err)
	}
	defer func() { _ = rows.Close() }()

	versions := make([]RecordVersion, 0)
	for rows.Next() {
		var (
			v          RecordVersion
			jsonText   sql.NullString
			live       sql.NullInt64
			observedAt string
		)
		if err := rows.Scan(&v.ID, &v.URI, &v.CID, &v.DID, &v.Collection, &v.Action, &jsonText, &live, &observedAt); err != nil {
			return nil, fmt.Errorf("scan record version: %w", err)
		}
		if jsonText.Valid {
			text := jsonText.String
			v.JSON = &text
		}
		if live.Valid {
			value := live.Int64 != 0
			v.Live = &value
		}
		parsed, err := parseDBTime(observedAt)
		if err != nil {
			return nil, fmt.Errorf("parse record_version.observed_at: %w", err)
		}
		v.ObservedAt = parsed
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate record versions: %w", err)
	}
	return versions, nil
}
