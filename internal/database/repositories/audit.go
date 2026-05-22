package repositories

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/GainForest/hyperindex/internal/database"
)

// DefaultAuditRecordEventPageSize is the page size used when a caller does not
// provide an explicit audit record event limit.
const DefaultAuditRecordEventPageSize = 50

// MaxAuditRecordEventPageSize is the largest audit record event page this
// repository will return in one query to protect GraphQL callers and databases
// from accidental unbounded scans.
const MaxAuditRecordEventPageSize = 1000

const auditRecordEventCursorPrefix = "audit_record_event:"

// AuditRepository stores and queries append-only Tap audit history while keeping
// the existing current-state record and actor projections in sync.
type AuditRepository struct {
	db database.Executor
}

// NewAuditRepository creates an audit repository backed by the provided database
// executor. The executor must have the audit migrations applied before ingest or
// query methods are used.
func NewAuditRepository(db database.Executor) *AuditRepository {
	return &AuditRepository{db: db}
}

// AuditTapEvent is the repository-layer projection of a Tap websocket delivery.
// It mirrors the Tap envelope fields needed for audit storage without importing
// the tap package into repositories, which would create an import cycle because
// the Tap handler already depends on repository types.
type AuditTapEvent struct {
	// ID is Tap's top-level delivery/ack id, not a semantic event id.
	ID int64 `json:"id"`
	// Type is the top-level Tap event type; supported values are "record" and "identity".
	Type string `json:"type"`
	// Record contains the decoded record payload when Type is "record".
	Record *AuditTapRecordEvent `json:"record,omitempty"`
	// Identity contains the decoded identity payload when Type is "identity".
	Identity *AuditTapIdentityEvent `json:"identity,omitempty"`
}

// IsRecord reports whether the delivery contains a valid record event payload.
func (e *AuditTapEvent) IsRecord() bool {
	return e != nil && e.Type == "record" && e.Record != nil
}

// IsIdentity reports whether the delivery contains a valid identity event payload.
func (e *AuditTapEvent) IsIdentity() bool {
	return e != nil && e.Type == "identity" && e.Identity != nil
}

// AuditTapRecordEvent contains the decoded Tap record payload needed for audit
// ingest and current-state record projection updates.
type AuditTapRecordEvent struct {
	// Live is Tap's firehose/backfill marker for the delivery.
	Live bool `json:"live"`
	// Rev is the repo revision associated with this record change. It may be empty.
	Rev string `json:"rev"`
	// DID is the repository DID that owns the record.
	DID string `json:"did"`
	// Collection is the AT Protocol collection namespace.
	Collection string `json:"collection"`
	// RKey is the record key within the collection.
	RKey string `json:"rkey"`
	// Action is the record operation: "create", "update", or "delete".
	Action string `json:"action"`
	// CID is the content identifier for create/update events when Tap provides it.
	CID string `json:"cid,omitempty"`
	// Record is the JSON record body for create/update events. It may be empty.
	Record json.RawMessage `json:"record,omitempty"`
}

// URI returns the AT-URI addressed by this Tap record event.
func (e *AuditTapRecordEvent) URI() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("at://%s/%s/%s", e.DID, e.Collection, e.RKey)
}

// AuditTapIdentityEvent contains the decoded Tap identity payload needed for
// audit ingest and current-state actor projection updates.
type AuditTapIdentityEvent struct {
	// DID is the actor DID affected by this identity event.
	DID string `json:"did"`
	// Handle is the current handle when Tap provides one.
	Handle string `json:"handle"`
	// IsActive is Tap's is_active value when IsActivePresent is true.
	IsActive bool `json:"is_active"`
	// IsActivePresent distinguishes a missing is_active field from an explicit false.
	IsActivePresent bool `json:"-"`
	// Status is Tap's identity status string, such as active, deleted, or takendown.
	Status string `json:"status"`
}

// AuditIngestResult describes the durable rows created or reused for one Tap
// audit ingest transaction.
type AuditIngestResult struct {
	// Type is the supported Tap event type that was ingested.
	Type string
	// TapDeliveryID is Tap's top-level delivery/ack id for the raw delivery.
	TapDeliveryID int64
	// RawEventID is the raw_tap_events row id inserted for this delivery.
	RawEventID int64
	// Inserted is true when the decoded record_events or identity_events row was new.
	Inserted bool
	// EventID is the decoded audit row id. For duplicates, this points at the existing row.
	EventID *int64
	// EventKey is the dedupe key used for the decoded audit row.
	EventKey *string
}

// AuditRecordEvent is one decoded append-only record audit event returned by
// FindRecordEvents.
type AuditRecordEvent struct {
	ID            int64
	EventKey      string
	Source        string
	TapDeliveryID int64
	RawEventID    int64
	ReceivedAt    string
	Live          bool
	Rev           string
	DID           string
	Collection    string
	RKey          string
	URI           string
	Action        string
	CID           *string
	Record        *string
}

// AuditRecordEventFilters contains equality and range filters supported by
// FindRecordEvents. Nil fields are ignored.
type AuditRecordEventFilters struct {
	ID               *int64
	URI              *string
	DID              *string
	Collection       *string
	RKey             *string
	Action           *string
	Live             *bool
	Rev              *string
	CID              *string
	ReceivedAt       *string
	ReceivedAtAfter  *string
	ReceivedAtBefore *string
}

// AuditRecordEventOrder controls the stable id ordering used by FindRecordEvents.
// Direction accepts "ASC" or "DESC" and defaults to "DESC" when empty.
type AuditRecordEventOrder struct {
	Direction string
}

// RecordEventFindOptions controls audit record event filtering and cursor
// pagination. Cursors are opaque values returned on AuditRecordEventEdge.
type RecordEventFindOptions struct {
	First   int
	After   string
	Where   AuditRecordEventFilters
	OrderBy AuditRecordEventOrder
}

// AuditRecordEventPage is a cursor-paginated page of audit record events.
type AuditRecordEventPage struct {
	Events          []*AuditRecordEvent
	Edges           []*AuditRecordEventEdge
	HasNextPage     bool
	HasPreviousPage bool
	StartCursor     *string
	EndCursor       *string
}

// AuditRecordEventEdge pairs an audit record event with its opaque cursor.
type AuditRecordEventEdge struct {
	Cursor string
	Node   *AuditRecordEvent
}

// IngestTapEvent appends one valid record or identity Tap delivery to the audit
// tables and applies the matching current-state projection change in the same
// database transaction.
func (r *AuditRepository) IngestTapEvent(ctx context.Context, rawPayload []byte, event *AuditTapEvent) (*AuditIngestResult, error) {
	if event == nil {
		return nil, fmt.Errorf("audit ingest requires a Tap event; got nil")
	}
	if !event.IsRecord() && !event.IsIdentity() {
		return nil, fmt.Errorf("audit ingest supports only record or identity Tap events; got type %q", event.Type)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin audit ingest transaction: %w", err)
	}
	defer tx.Rollback()

	var result *AuditIngestResult
	if event.IsRecord() {
		result, err = r.ingestRecordTapEvent(ctx, tx, rawPayload, event)
	} else {
		result, err = r.ingestIdentityTapEvent(ctx, tx, rawPayload, event)
	}
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit audit ingest transaction for Tap delivery %d: %w", event.ID, err)
	}

	return result, nil
}

// CountRecordEvents returns the total number of append-only record audit
// events matching the provided filters. Pagination cursors are intentionally not
// applied so callers can expose Relay-style totalCount separately from page
// slicing.
func (r *AuditRepository) CountRecordEvents(ctx context.Context, filters AuditRecordEventFilters) (int64, error) {
	conditions, params, err := r.buildAuditRecordEventConditions(filters)
	if err != nil {
		return 0, err
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM record_events%s", whereClause)
	var count int64
	if err := r.db.DB().QueryRowContext(ctx, sqlStr, r.db.ConvertParams(params)...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count audit record events: %w", err)
	}

	return count, nil
}

// FindRecordEvents returns a stable id-cursor page of decoded record audit
// events matching the provided filters.
func (r *AuditRepository) FindRecordEvents(ctx context.Context, opts RecordEventFindOptions) (*AuditRecordEventPage, error) {
	limit := opts.First
	if limit == 0 {
		limit = DefaultAuditRecordEventPageSize
	}
	if limit < 0 {
		return nil, fmt.Errorf("audit record event page size must be positive; got %d", opts.First)
	}
	if limit > MaxAuditRecordEventPageSize {
		return nil, fmt.Errorf("audit record event page size %d exceeds maximum %d", limit, MaxAuditRecordEventPageSize)
	}

	direction, err := normalizeAuditOrderDirection(opts.OrderBy.Direction)
	if err != nil {
		return nil, err
	}

	conditions, params, err := r.buildAuditRecordEventConditions(opts.Where)
	if err != nil {
		return nil, err
	}

	if opts.After != "" {
		afterID, err := decodeAuditRecordEventCursor(opts.After)
		if err != nil {
			return nil, err
		}
		placeholder := r.db.Placeholder(len(params) + 1)
		operator := "<"
		if direction == "ASC" {
			operator = ">"
		}
		conditions = append(conditions, fmt.Sprintf("id %s %s", operator, placeholder))
		params = append(params, database.Int(afterID))
	}

	params = append(params, database.Int(int64(limit+1)))
	limitPlaceholder := r.db.Placeholder(len(params))

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	recordColumn := "record"
	receivedAtColumn := "received_at"
	if r.db.Dialect() == database.PostgreSQL {
		recordColumn = "record::text"
		receivedAtColumn = "received_at::text"
	}

	sqlStr := fmt.Sprintf(`SELECT id, event_key, source, tap_delivery_id, raw_event_id, %s, live, rev, did, collection, rkey, uri, action, cid, %s
		FROM record_events%s
		ORDER BY id %s
		LIMIT %s`, receivedAtColumn, recordColumn, whereClause, direction, limitPlaceholder)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, fmt.Errorf("query audit record events: %w", err)
	}
	defer rows.Close()

	var events []*AuditRecordEvent
	for rows.Next() {
		event, err := scanAuditRecordEvent(rows)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit record events: %w", err)
	}

	hasNext := false
	if len(events) > limit {
		hasNext = true
		events = events[:limit]
	}

	edges := make([]*AuditRecordEventEdge, 0, len(events))
	var startCursor, endCursor *string
	for _, event := range events {
		cursor := encodeAuditRecordEventCursor(event.ID)
		edges = append(edges, &AuditRecordEventEdge{Cursor: cursor, Node: event})
		if startCursor == nil {
			value := cursor
			startCursor = &value
		}
		value := cursor
		endCursor = &value
	}

	return &AuditRecordEventPage{
		Events:          events,
		Edges:           edges,
		HasNextPage:     hasNext,
		HasPreviousPage: opts.After != "",
		StartCursor:     startCursor,
		EndCursor:       endCursor,
	}, nil
}

func (r *AuditRepository) ingestRecordTapEvent(ctx context.Context, tx *sql.Tx, rawPayload []byte, event *AuditTapEvent) (*AuditIngestResult, error) {
	record := event.Record
	rawEventID, err := r.insertRawTapEvent(ctx, tx, rawPayload, event.ID, "record")
	if err != nil {
		return nil, err
	}

	eventKey := recordEventKey(event.ID, rawPayload, event, record)
	recordEventID, inserted, err := r.insertRecordEvent(ctx, tx, rawEventID, event.ID, eventKey, record)
	if err != nil {
		return nil, err
	}

	if inserted {
		switch record.Action {
		case "create", "update":
			if len(record.Record) > 0 {
				if err := r.upsertCurrentActor(ctx, tx, record.DID, ""); err != nil {
					return nil, fmt.Errorf("upsert current actor for record event %q: %w", eventKey, err)
				}
				if err := r.upsertCurrentRecord(ctx, tx, record); err != nil {
					return nil, fmt.Errorf("upsert current record for audit event %q: %w", eventKey, err)
				}
			}
		case "delete":
			if err := r.deleteCurrentRecord(ctx, tx, record.URI()); err != nil {
				return nil, fmt.Errorf("delete current record for audit event %q: %w", eventKey, err)
			}
		default:
			return nil, fmt.Errorf("audit record event has unsupported action %q; expected create, update, or delete", record.Action)
		}
	}

	return &AuditIngestResult{
		Type:          "record",
		TapDeliveryID: event.ID,
		RawEventID:    rawEventID,
		Inserted:      inserted,
		EventID:       &recordEventID,
		EventKey:      &eventKey,
	}, nil
}

func (r *AuditRepository) ingestIdentityTapEvent(ctx context.Context, tx *sql.Tx, rawPayload []byte, event *AuditTapEvent) (*AuditIngestResult, error) {
	identity := event.Identity
	rawEventID, err := r.insertRawTapEvent(ctx, tx, rawPayload, event.ID, "identity")
	if err != nil {
		return nil, err
	}

	eventKey := identityEventKey(event.ID, identity)
	identityEventID, inserted, err := r.insertIdentityEvent(ctx, tx, rawEventID, event.ID, eventKey, identity)
	if err != nil {
		return nil, err
	}

	if inserted {
		if shouldPurgeAuditIdentity(identity) {
			if err := r.deleteCurrentRecordsByDID(ctx, tx, identity.DID); err != nil {
				return nil, fmt.Errorf("delete current records for identity audit event %q: %w", eventKey, err)
			}
			if err := r.deleteCurrentActorByDID(ctx, tx, identity.DID); err != nil {
				return nil, fmt.Errorf("delete current actor for identity audit event %q: %w", eventKey, err)
			}
		} else if err := r.upsertCurrentActor(ctx, tx, identity.DID, identity.Handle); err != nil {
			return nil, fmt.Errorf("upsert current actor for identity audit event %q: %w", eventKey, err)
		}
	}

	return &AuditIngestResult{
		Type:          "identity",
		TapDeliveryID: event.ID,
		RawEventID:    rawEventID,
		Inserted:      inserted,
		EventID:       &identityEventID,
		EventKey:      &eventKey,
	}, nil
}

func (r *AuditRepository) insertRawTapEvent(ctx context.Context, tx *sql.Tx, rawPayload []byte, deliveryID int64, eventType string) (int64, error) {
	payload := string(rawPayload)
	if payload == "" {
		payload = "{}"
	}

	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	p3 := r.db.Placeholder(3)
	params := []database.Value{database.Int(deliveryID), database.Text(eventType), database.Text(payload)}

	if r.db.Dialect() == database.PostgreSQL {
		var id int64
		sqlStr := fmt.Sprintf(`INSERT INTO raw_tap_events (tap_delivery_id, type, payload)
			VALUES (%s, %s, %s)
			RETURNING id`, p1, p2, p3)
		if err := tx.QueryRowContext(ctx, sqlStr, r.db.ConvertParams(params)...).Scan(&id); err != nil {
			return 0, fmt.Errorf("insert raw Tap audit event: %w", err)
		}
		return id, nil
	}

	sqlStr := fmt.Sprintf(`INSERT INTO raw_tap_events (tap_delivery_id, type, payload)
		VALUES (%s, %s, %s)`, p1, p2, p3)
	result, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return 0, fmt.Errorf("insert raw Tap audit event: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read raw Tap audit event id: %w", err)
	}
	return id, nil
}

func (r *AuditRepository) insertRecordEvent(ctx context.Context, tx *sql.Tx, rawEventID, deliveryID int64, eventKey string, record *AuditTapRecordEvent) (int64, bool, error) {
	recordValue := database.Value(database.Null())
	if len(record.Record) > 0 {
		recordValue = database.Text(string(record.Record))
	}
	cidValue := database.Value(database.Null())
	if record.CID != "" {
		cidValue = database.Text(record.CID)
	}

	placeholders := make([]string, 12)
	for i := range placeholders {
		placeholders[i] = r.db.Placeholder(i + 1)
	}
	if r.db.Dialect() == database.PostgreSQL {
		placeholders[11] += "::jsonb"
	}

	sqlStr := fmt.Sprintf(`INSERT INTO record_events (
			event_key, tap_delivery_id, raw_event_id, live, rev, did, collection, rkey, uri, action, cid, record
		) VALUES (%s)
		ON CONFLICT(event_key) DO NOTHING`, strings.Join(placeholders, ", "))

	params := []database.Value{
		database.Text(eventKey),
		database.Int(deliveryID),
		database.Int(rawEventID),
		database.Bool(record.Live),
		database.Text(record.Rev),
		database.Text(record.DID),
		database.Text(record.Collection),
		database.Text(record.RKey),
		database.Text(record.URI()),
		database.Text(record.Action),
		cidValue,
		recordValue,
	}
	result, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return 0, false, fmt.Errorf("insert record audit event %q: %w", eventKey, err)
	}
	inserted, err := rowsWereInserted(result)
	if err != nil {
		return 0, false, fmt.Errorf("determine record audit event insert result for %q: %w", eventKey, err)
	}

	id, err := r.findRecordEventID(ctx, tx, eventKey)
	if err != nil {
		return 0, false, err
	}
	return id, inserted, nil
}

func (r *AuditRepository) insertIdentityEvent(ctx context.Context, tx *sql.Tx, rawEventID, deliveryID int64, eventKey string, identity *AuditTapIdentityEvent) (int64, bool, error) {
	isActiveValue := database.Value(database.Null())
	if identity.IsActivePresent {
		isActiveValue = database.Bool(identity.IsActive)
	}

	p := make([]string, 7)
	for i := range p {
		p[i] = r.db.Placeholder(i + 1)
	}
	sqlStr := fmt.Sprintf(`INSERT INTO identity_events (
			event_key, tap_delivery_id, raw_event_id, did, handle, is_active, status
		) VALUES (%s)
		ON CONFLICT(event_key) DO NOTHING`, strings.Join(p, ", "))

	params := []database.Value{
		database.Text(eventKey),
		database.Int(deliveryID),
		database.Int(rawEventID),
		database.Text(identity.DID),
		database.Text(identity.Handle),
		isActiveValue,
		database.Text(identity.Status),
	}
	result, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return 0, false, fmt.Errorf("insert identity audit event %q: %w", eventKey, err)
	}
	inserted, err := rowsWereInserted(result)
	if err != nil {
		return 0, false, fmt.Errorf("determine identity audit event insert result for %q: %w", eventKey, err)
	}

	id, err := r.findIdentityEventID(ctx, tx, eventKey)
	if err != nil {
		return 0, false, err
	}
	return id, inserted, nil
}

func (r *AuditRepository) findRecordEventID(ctx context.Context, tx *sql.Tx, eventKey string) (int64, error) {
	var id int64
	sqlStr := fmt.Sprintf("SELECT id FROM record_events WHERE event_key = %s", r.db.Placeholder(1))
	if err := tx.QueryRowContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(eventKey)})...).Scan(&id); err != nil {
		return 0, fmt.Errorf("find record audit event id for %q: %w", eventKey, err)
	}
	return id, nil
}

func (r *AuditRepository) findIdentityEventID(ctx context.Context, tx *sql.Tx, eventKey string) (int64, error) {
	var id int64
	sqlStr := fmt.Sprintf("SELECT id FROM identity_events WHERE event_key = %s", r.db.Placeholder(1))
	if err := tx.QueryRowContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(eventKey)})...).Scan(&id); err != nil {
		return 0, fmt.Errorf("find identity audit event id for %q: %w", eventKey, err)
	}
	return id, nil
}

func (r *AuditRepository) upsertCurrentRecord(ctx context.Context, tx *sql.Tx, record *AuditTapRecordEvent) error {
	selectSQL := fmt.Sprintf("SELECT cid FROM record WHERE uri = %s", r.db.Placeholder(1))
	var existingCID string
	err := tx.QueryRowContext(ctx, selectSQL, r.db.ConvertParams([]database.Value{database.Text(record.URI())})...).Scan(&existingCID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read current record cid for %s: %w", record.URI(), err)
	}
	if record.CID != "" && existingCID == record.CID {
		return nil
	}

	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	p3 := r.db.Placeholder(3)
	p4 := r.db.Placeholder(4)
	p5 := r.db.Placeholder(5)

	var sqlStr string
	if r.db.Dialect() == database.PostgreSQL {
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES (%s, %s, %s, %s, %s::jsonb)
			ON CONFLICT(uri) DO UPDATE SET
				cid = EXCLUDED.cid,
				json = EXCLUDED.json,
				indexed_at = NOW()`, p1, p2, p3, p4, p5)
	} else {
		sqlStr = fmt.Sprintf(`INSERT INTO record (uri, cid, did, collection, json)
			VALUES (%s, %s, %s, %s, %s)
			ON CONFLICT(uri) DO UPDATE SET
				cid = excluded.cid,
				json = excluded.json,
				indexed_at = datetime('now')`, p1, p2, p3, p4, p5)
	}

	_, err = tx.ExecContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{
		database.Text(record.URI()),
		database.Text(record.CID),
		database.Text(record.DID),
		database.Text(record.Collection),
		database.Text(string(record.Record)),
	})...)
	return err
}

func (r *AuditRepository) deleteCurrentRecord(ctx context.Context, tx *sql.Tx, uri string) error {
	sqlStr := fmt.Sprintf("DELETE FROM record WHERE uri = %s", r.db.Placeholder(1))
	_, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(uri)})...)
	return err
}

func (r *AuditRepository) upsertCurrentActor(ctx context.Context, tx *sql.Tx, did, handle string) error {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)

	var sqlStr string
	if r.db.Dialect() == database.PostgreSQL {
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, %s, NOW())
			ON CONFLICT(did) DO UPDATE SET
				handle = EXCLUDED.handle,
				indexed_at = NOW()`, p1, p2)
	} else {
		sqlStr = fmt.Sprintf(`INSERT INTO actor (did, handle, indexed_at)
			VALUES (%s, %s, datetime('now'))
			ON CONFLICT(did) DO UPDATE SET
				handle = excluded.handle,
				indexed_at = datetime('now')`, p1, p2)
	}

	_, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{
		database.Text(did),
		database.Text(handle),
	})...)
	return err
}

func (r *AuditRepository) deleteCurrentRecordsByDID(ctx context.Context, tx *sql.Tx, did string) error {
	sqlStr := fmt.Sprintf("DELETE FROM record WHERE did = %s", r.db.Placeholder(1))
	_, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(did)})...)
	return err
}

func (r *AuditRepository) deleteCurrentActorByDID(ctx context.Context, tx *sql.Tx, did string) error {
	sqlStr := fmt.Sprintf("DELETE FROM actor WHERE did = %s", r.db.Placeholder(1))
	_, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams([]database.Value{database.Text(did)})...)
	return err
}

func (r *AuditRepository) buildAuditRecordEventConditions(filters AuditRecordEventFilters) ([]string, []database.Value, error) {
	var conditions []string
	var params []database.Value

	addEqualCondition := func(column string, value database.Value) {
		placeholder := r.db.Placeholder(len(params) + 1)
		conditions = append(conditions, fmt.Sprintf("%s = %s", column, placeholder))
		params = append(params, value)
	}
	addTimestampCondition := func(column, operator string, value string) {
		placeholder := r.db.Placeholder(len(params) + 1)
		if r.db.Dialect() == database.PostgreSQL {
			conditions = append(conditions, fmt.Sprintf("%s %s %s::timestamptz", column, operator, placeholder))
		} else {
			conditions = append(conditions, fmt.Sprintf("julianday(%s) %s julianday(%s)", column, operator, placeholder))
		}
		params = append(params, database.TimestamptzString(value))
	}

	if filters.ID != nil {
		addEqualCondition("id", database.Int(*filters.ID))
	}
	if filters.URI != nil {
		addEqualCondition("uri", database.Text(*filters.URI))
	}
	if filters.DID != nil {
		addEqualCondition("did", database.Text(*filters.DID))
	}
	if filters.Collection != nil {
		addEqualCondition("collection", database.Text(*filters.Collection))
	}
	if filters.RKey != nil {
		addEqualCondition("rkey", database.Text(*filters.RKey))
	}
	if filters.Action != nil {
		action := strings.ToLower(strings.TrimSpace(*filters.Action))
		if action != "create" && action != "update" && action != "delete" {
			return nil, nil, fmt.Errorf("unsupported audit record action filter %q; expected create, update, or delete", *filters.Action)
		}
		addEqualCondition("action", database.Text(action))
	}
	if filters.Live != nil {
		addEqualCondition("live", database.Bool(*filters.Live))
	}
	if filters.Rev != nil {
		addEqualCondition("rev", database.Text(*filters.Rev))
	}
	if filters.CID != nil {
		if *filters.CID == "" {
			conditions = append(conditions, "(cid = '' OR cid IS NULL)")
		} else {
			addEqualCondition("cid", database.Text(*filters.CID))
		}
	}
	if filters.ReceivedAt != nil {
		addTimestampCondition("received_at", "=", *filters.ReceivedAt)
	}
	if filters.ReceivedAtAfter != nil {
		addTimestampCondition("received_at", ">", *filters.ReceivedAtAfter)
	}
	if filters.ReceivedAtBefore != nil {
		addTimestampCondition("received_at", "<", *filters.ReceivedAtBefore)
	}

	return conditions, params, nil
}

func scanAuditRecordEvent(rows *sql.Rows) (*AuditRecordEvent, error) {
	var event AuditRecordEvent
	var liveRaw any
	var cid sql.NullString
	var record sql.NullString
	if err := rows.Scan(
		&event.ID,
		&event.EventKey,
		&event.Source,
		&event.TapDeliveryID,
		&event.RawEventID,
		&event.ReceivedAt,
		&liveRaw,
		&event.Rev,
		&event.DID,
		&event.Collection,
		&event.RKey,
		&event.URI,
		&event.Action,
		&cid,
		&record,
	); err != nil {
		return nil, fmt.Errorf("scan audit record event: %w", err)
	}

	live, err := boolFromDBValue(liveRaw)
	if err != nil {
		return nil, fmt.Errorf("scan audit record event live flag for id %d: %w", event.ID, err)
	}
	event.Live = live
	if cid.Valid {
		event.CID = &cid.String
	}
	if record.Valid {
		event.Record = &record.String
	}
	return &event, nil
}

func rowsWereInserted(result sql.Result) (bool, error) {
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rowsAffected > 0, nil
}

func recordEventKey(deliveryID int64, rawPayload []byte, event *AuditTapEvent, record *AuditTapRecordEvent) string {
	if record.Rev == "" {
		return fmt.Sprintf("record:fallback:%d:%x", deliveryID, sha256.Sum256(normalizedEventPayload(rawPayload, event)))
	}
	return fmt.Sprintf("record:%s:%s:%s:%s:%s:%s", record.DID, record.Rev, record.Collection, record.RKey, record.Action, record.CID)
}

func identityEventKey(deliveryID int64, identity *AuditTapIdentityEvent) string {
	return fmt.Sprintf("identity:%d:%s:%s:%s:%s", deliveryID, identity.DID, identity.Handle, identityActiveKeyPart(identity), identity.Status)
}

func identityActiveKeyPart(identity *AuditTapIdentityEvent) string {
	if !identity.IsActivePresent {
		return ""
	}
	if identity.IsActive {
		return "true"
	}
	return "false"
}

func normalizedEventPayload(rawPayload []byte, event *AuditTapEvent) []byte {
	if len(rawPayload) > 0 {
		if normalized, ok := normalizeJSON(rawPayload); ok {
			return normalized
		}
		return []byte(strings.TrimSpace(string(rawPayload)))
	}

	encoded, err := json.Marshal(event)
	if err != nil {
		return []byte(fmt.Sprintf("%+v", event))
	}
	return encoded
}

func normalizeJSON(rawPayload []byte) ([]byte, bool) {
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(string(rawPayload)))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, false
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func shouldPurgeAuditIdentity(identity *AuditTapIdentityEvent) bool {
	switch strings.ToLower(strings.TrimSpace(identity.Status)) {
	case "deleted", "deactivated", "suspended", "takendown":
		return true
	default:
		return false
	}
}

func normalizeAuditOrderDirection(direction string) (string, error) {
	if direction == "" {
		return "DESC", nil
	}
	normalized := strings.ToUpper(strings.TrimSpace(direction))
	switch normalized {
	case "ASC", "DESC":
		return normalized, nil
	default:
		return "", fmt.Errorf("unsupported audit record event order direction %q; expected ASC or DESC", direction)
	}
}

func encodeAuditRecordEventCursor(id int64) string {
	return base64.URLEncoding.EncodeToString([]byte(fmt.Sprintf("%s%d", auditRecordEventCursorPrefix, id)))
}

func decodeAuditRecordEventCursor(cursor string) (int64, error) {
	decoded, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, fmt.Errorf("invalid audit record event cursor: expected base64-encoded %q cursor: %w", auditRecordEventCursorPrefix+"<id>", err)
	}
	value := string(decoded)
	if !strings.HasPrefix(value, auditRecordEventCursorPrefix) {
		return 0, fmt.Errorf("invalid audit record event cursor: expected prefix %q", auditRecordEventCursorPrefix)
	}
	idText := strings.TrimPrefix(value, auditRecordEventCursorPrefix)
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid audit record event cursor: expected positive numeric id after prefix %q", auditRecordEventCursorPrefix)
	}
	return id, nil
}

func boolFromDBValue(value any) (bool, error) {
	switch typed := value.(type) {
	case bool:
		return typed, nil
	case int64:
		return typed != 0, nil
	case int:
		return typed != 0, nil
	case []byte:
		return parseDBBoolString(string(typed))
	case string:
		return parseDBBoolString(typed)
	default:
		return false, fmt.Errorf("unsupported bool database value %T", value)
	}
}

func parseDBBoolString(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "t", "true":
		return true, nil
	case "0", "f", "false":
		return false, nil
	default:
		return false, fmt.Errorf("unsupported bool database value %q", value)
	}
}
