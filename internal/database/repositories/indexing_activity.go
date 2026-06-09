package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// ActivityEntry represents an indexing activity log entry.
type ActivityEntry struct {
	ID           int64
	Timestamp    time.Time
	Operation    string
	Collection   string
	DID          string
	RKey         *string
	Status       string
	ErrorMessage *string
	EventJSON    string
}

// ActivityBucket represents aggregated activity data for a time bucket.
type ActivityBucket struct {
	Timestamp time.Time
	Total     int64
	Creates   int64
	Updates   int64
	Deletes   int64
}

// IndexingActivityRepository handles indexing activity persistence.
type IndexingActivityRepository struct {
	db database.Executor
}

// NewIndexingActivityRepository creates a new indexing activity repository.
func NewIndexingActivityRepository(db database.Executor) *IndexingActivityRepository {
	return &IndexingActivityRepository{db: db}
}

// LogActivity logs a new indexing activity entry with pending status and returns the ID.
// The eventJSON argument must be valid JSON; empty input and literal JSON null are stored as `{}` for events without a record payload.
func (r *IndexingActivityRepository) LogActivity(
	ctx context.Context,
	timestamp time.Time,
	operation, collection, did, rkey, eventJSON string,
) (int64, error) {
	return r.LogActivityWithStatus(ctx, timestamp, operation, collection, did, rkey, eventJSON, "pending")
}

// LogActivityWithStatus logs a new indexing activity entry with a custom status and returns the ID.
// The eventJSON argument must be valid JSON; empty input and literal JSON null are stored as `{}` for events without a record payload.
func (r *IndexingActivityRepository) LogActivityWithStatus(
	ctx context.Context,
	timestamp time.Time,
	operation, collection, did, rkey, eventJSON, status string,
) (int64, error) {
	var sqlStr string
	var timestampStr string

	// Always store in UTC for consistency
	utcTime := timestamp.UTC()

	normalizedEventJSON, err := normalizeActivityEventJSON(eventJSON)
	if err != nil {
		return 0, err
	}

	switch r.db.Dialect() {
	case database.PostgreSQL:
		timestampStr = utcTime.Format(time.RFC3339)
		sqlStr = fmt.Sprintf(`INSERT INTO indexing_activity 
			(timestamp, operation, collection, did, rkey, status, event_json)
			VALUES (%s, %s, %s, %s, %s, %s, %s)
			RETURNING id`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3),
			r.db.Placeholder(4), r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7))
	default:
		timestampStr = utcTime.Format("2006-01-02 15:04:05")
		sqlStr = fmt.Sprintf(`INSERT INTO indexing_activity 
			(timestamp, operation, collection, did, rkey, status, event_json)
			VALUES (%s, %s, %s, %s, %s, %s, %s)`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3),
			r.db.Placeholder(4), r.db.Placeholder(5), r.db.Placeholder(6), r.db.Placeholder(7))
	}

	params := []database.Value{
		database.Text(timestampStr),
		database.Text(operation),
		database.Text(collection),
		database.Text(did),
		database.Text(rkey),
		database.Text(status),
		database.Text(normalizedEventJSON),
	}

	if r.db.Dialect() == database.PostgreSQL {
		var id int64
		err := r.db.QueryRow(ctx, sqlStr, params, &id)
		return id, err
	}

	result, err := r.db.Exec(ctx, sqlStr, params)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateStatus updates the status and optional error message of an activity entry.
func (r *IndexingActivityRepository) UpdateStatus(
	ctx context.Context,
	id int64,
	status string,
	errorMessage *string,
) error {
	sqlStr := fmt.Sprintf(`UPDATE indexing_activity 
		SET status = %s, error_message = %s 
		WHERE id = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))

	params := []database.Value{
		database.Text(status),
		database.NullableText(errorMessage),
		database.Int(id),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// GetRecentActivity returns activity entries from the last N hours.
func (r *IndexingActivityRepository) GetRecentActivity(ctx context.Context, hours int) ([]ActivityEntry, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`SELECT id, timestamp, operation, collection, did, rkey, status, error_message, event_json
			FROM indexing_activity
			WHERE timestamp >= NOW() - INTERVAL '%d hours'
			ORDER BY timestamp DESC
			LIMIT 1000`, hours)
	default:
		sqlStr = fmt.Sprintf(`SELECT id, timestamp, operation, collection, did, rkey, status, error_message, event_json
			FROM indexing_activity
			WHERE timestamp >= datetime('now', '-%d hours')
			ORDER BY timestamp DESC
			LIMIT 1000`, hours)
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanActivityEntries(rows)
}

// CleanupOldActivity deletes activity entries older than the specified hours.
func (r *IndexingActivityRepository) CleanupOldActivity(ctx context.Context, hours int) error {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`DELETE FROM indexing_activity 
			WHERE timestamp < NOW() - INTERVAL '%d hours'`, hours)
	default:
		sqlStr = fmt.Sprintf(`DELETE FROM indexing_activity 
			WHERE timestamp < datetime('now', '-%d hours')`, hours)
	}

	_, err := r.db.Exec(ctx, sqlStr, nil)
	return err
}

// GetActivityBuckets returns aggregated activity data for the specified time range.
func (r *IndexingActivityRepository) GetActivityBuckets(ctx context.Context, timeRange string) ([]ActivityBucket, error) {
	var sqlStr string

	switch timeRange {
	case "ONE_HOUR":
		sqlStr = r.buildBucketQuery(1, 5)
	case "THREE_HOURS":
		sqlStr = r.buildBucketQuery(3, 15)
	case "SIX_HOURS":
		sqlStr = r.buildBucketQuery(6, 30)
	case "ONE_DAY":
		sqlStr = r.buildBucketQuery(24, 60)
	case "SEVEN_DAYS":
		sqlStr = r.buildBucketQuery(168, 1440)
	default:
		sqlStr = r.buildBucketQuery(1, 5)
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var buckets []ActivityBucket
	for rows.Next() {
		var bucket ActivityBucket
		var timestampStr string

		if err := rows.Scan(&timestampStr, &bucket.Total, &bucket.Creates, &bucket.Updates, &bucket.Deletes); err != nil {
			return nil, err
		}

		bucket.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if bucket.Timestamp.IsZero() {
			bucket.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		}
		buckets = append(buckets, bucket)
	}

	return buckets, rows.Err()
}

func (r *IndexingActivityRepository) buildBucketQuery(hours, minutes int) string {
	switch r.db.Dialect() {
	case database.PostgreSQL:
		return fmt.Sprintf(`SELECT 
			date_trunc('hour', timestamp) + 
				INTERVAL '%d minutes' * FLOOR(EXTRACT(MINUTE FROM timestamp) / %d) as bucket,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE operation = 'create') as creates,
			COUNT(*) FILTER (WHERE operation = 'update') as updates,
			COUNT(*) FILTER (WHERE operation = 'delete') as deletes
		FROM indexing_activity
		WHERE timestamp >= NOW() - INTERVAL '%d hours'
		GROUP BY bucket
		ORDER BY bucket ASC`, minutes, minutes, hours)
	default:
		// SQLite version
		return fmt.Sprintf(`SELECT 
			strftime('%%Y-%%m-%%d %%H:', timestamp) || 
				printf('%%02d', (CAST(strftime('%%M', timestamp) AS INTEGER) / %d) * %d) || ':00' as bucket,
			COUNT(*) as total,
			SUM(CASE WHEN operation = 'create' THEN 1 ELSE 0 END) as creates,
			SUM(CASE WHEN operation = 'update' THEN 1 ELSE 0 END) as updates,
			SUM(CASE WHEN operation = 'delete' THEN 1 ELSE 0 END) as deletes
		FROM indexing_activity
		WHERE timestamp >= datetime('now', '-%d hours')
		GROUP BY bucket
		ORDER BY bucket ASC`, minutes, minutes, hours)
	}
}

// DeleteAll removes all activity entries.
func (r *IndexingActivityRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, "DELETE FROM indexing_activity", nil)
	return err
}

// GetCount returns the total number of activity entries.
func (r *IndexingActivityRepository) GetCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM indexing_activity", nil, &count)
	return count, err
}

func normalizeActivityEventJSON(eventJSON string) (string, error) {
	trimmed := strings.TrimSpace(eventJSON)
	if trimmed == "" || trimmed == "null" {
		return "{}", nil
	}
	if !json.Valid([]byte(trimmed)) {
		return "", fmt.Errorf("activity event_json must be valid JSON, got %q; pass a valid JSON payload or an empty string for delete events", trimmed)
	}
	return trimmed, nil
}

// scanActivityEntries scans indexing activity entries from rows.
func scanActivityEntries(rows *sql.Rows) ([]ActivityEntry, error) {
	var entries []ActivityEntry
	for rows.Next() {
		var entry ActivityEntry
		var timestampStr string
		var rkey sql.NullString
		var errorMessage sql.NullString

		if err := rows.Scan(&entry.ID, &timestampStr, &entry.Operation, &entry.Collection,
			&entry.DID, &rkey, &entry.Status, &errorMessage, &entry.EventJSON); err != nil {
			return nil, err
		}

		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		if entry.Timestamp.IsZero() {
			entry.Timestamp, _ = time.Parse("2006-01-02 15:04:05", timestampStr)
		}
		if rkey.Valid {
			entry.RKey = &rkey.String
		}
		if errorMessage.Valid {
			entry.ErrorMessage = &errorMessage.String
		}
		entries = append(entries, entry)
	}

	return entries, rows.Err()
}
