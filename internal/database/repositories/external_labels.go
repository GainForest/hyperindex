package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

const externalLabelLastErrorMaxLen = 4096

// LabelSubscriptionState tracks cursor state for a labeler websocket subscription.
type LabelSubscriptionState struct {
	URL             string
	LabelerDID      *string
	LastSeq         int64
	LastConnectedAt *time.Time
	LastEventAt     *time.Time
	LastError       *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ExternalLabel is a raw label received from an external ATProto labeler stream.
type ExternalLabel struct {
	ID              int64
	SubscriptionURL string
	Seq             int64
	LabelIndex      int64
	Src             string
	URI             string
	CID             *string
	Val             string
	Neg             bool
	Cts             string
	Exp             *string
	Sig             *string
	Ver             *int64
	RawJSON         string
	ReceivedAt      time.Time
}

// ExternalLabelInput is the repository-independent input for storing external labels.
type ExternalLabelInput struct {
	LabelIndex int64
	Src        string
	URI        string
	CID        *string
	Val        string
	Neg        bool
	Cts        string
	Exp        *string
	Sig        *string
	Ver        *int64
	RawJSON    string
}

// ExternalLabelsRepository handles external labeler event persistence.
type ExternalLabelsRepository struct {
	db database.Executor
}

// NewExternalLabelsRepository creates a new external labels repository.
func NewExternalLabelsRepository(db database.Executor) *ExternalLabelsRepository {
	return &ExternalLabelsRepository{db: db}
}

// EnsureState creates a subscription state row if it does not already exist.
func (r *ExternalLabelsRepository) EnsureState(ctx context.Context, subscriptionURL string) (*LabelSubscriptionState, error) {
	if err := validateSubscriptionURL(subscriptionURL); err != nil {
		return nil, err
	}

	sqlStr := fmt.Sprintf(`INSERT INTO label_subscription_state (url)
		VALUES (%s)
		ON CONFLICT(url) DO NOTHING`, r.db.Placeholder(1))
	if _, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(subscriptionURL)}); err != nil {
		return nil, err
	}

	return r.GetState(ctx, subscriptionURL)
}

// GetState retrieves subscription state by URL.
func (r *ExternalLabelsRepository) GetState(ctx context.Context, subscriptionURL string) (*LabelSubscriptionState, error) {
	sqlStr := fmt.Sprintf(`SELECT %s
		FROM label_subscription_state
		WHERE url = %s`, r.stateColumns(), r.db.Placeholder(1))

	var state LabelSubscriptionState
	var labelerDID, lastConnectedAt, lastEventAt, lastError sql.NullString
	var createdAt, updatedAt string

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(subscriptionURL)},
		&state.URL,
		&labelerDID,
		&state.LastSeq,
		&lastConnectedAt,
		&lastEventAt,
		&lastError,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if labelerDID.Valid {
		state.LabelerDID = &labelerDID.String
	}
	if lastError.Valid {
		state.LastError = &lastError.String
	}

	parsedCreatedAt, err := parseDBTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse label_subscription_state.created_at: %w", err)
	}
	state.CreatedAt = parsedCreatedAt

	parsedUpdatedAt, err := parseDBTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse label_subscription_state.updated_at: %w", err)
	}
	state.UpdatedAt = parsedUpdatedAt

	if lastConnectedAt.Valid {
		parsed, err := parseDBTime(lastConnectedAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse label_subscription_state.last_connected_at: %w", err)
		}
		state.LastConnectedAt = &parsed
	}
	if lastEventAt.Valid {
		parsed, err := parseDBTime(lastEventAt.String)
		if err != nil {
			return nil, fmt.Errorf("parse label_subscription_state.last_event_at: %w", err)
		}
		state.LastEventAt = &parsed
	}

	return &state, nil
}

// UpdateConnected records a successful websocket connection attempt.
func (r *ExternalLabelsRepository) UpdateConnected(ctx context.Context, subscriptionURL string) error {
	if _, err := r.EnsureState(ctx, subscriptionURL); err != nil {
		return err
	}

	sqlStr := fmt.Sprintf(`UPDATE label_subscription_state
		SET last_connected_at = %s,
			last_error = NULL,
			updated_at = %s
		WHERE url = %s`, r.db.Now(), r.db.Now(), r.db.Placeholder(1))

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(subscriptionURL)})
	return err
}

// UpdateError records the latest subscription error.
func (r *ExternalLabelsRepository) UpdateError(ctx context.Context, subscriptionURL, errText string) error {
	if _, err := r.EnsureState(ctx, subscriptionURL); err != nil {
		return err
	}

	normalizedErr := normalizeLastError(errText)
	sqlStr := fmt.Sprintf(`UPDATE label_subscription_state
		SET last_error = %s,
			updated_at = %s
		WHERE url = %s`, r.db.Placeholder(1), r.db.Now(), r.db.Placeholder(2))

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(normalizedErr),
		database.Text(subscriptionURL),
	})
	return err
}

// PersistEvent stores labels and advances the subscription cursor in one transaction.
func (r *ExternalLabelsRepository) PersistEvent(ctx context.Context, subscriptionURL string, seq int64, labels []ExternalLabelInput) error {
	if err := validateSubscriptionURL(subscriptionURL); err != nil {
		return err
	}
	if seq < 0 {
		return fmt.Errorf("external label event seq must be non-negative: %d", seq)
	}
	if err := validateExternalLabelInputs(labels); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin external label event transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit succeeds

	if err := r.ensureStateTx(ctx, tx, subscriptionURL); err != nil {
		return err
	}

	for i := range labels {
		if err := r.insertExternalLabelTx(ctx, tx, subscriptionURL, seq, labels[i]); err != nil {
			return err
		}
	}

	if err := r.updateLastSeqTx(ctx, tx, subscriptionURL, seq); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit external label event transaction: %w", err)
	}
	return nil
}

// UpdateLastSeq advances a subscription cursor without inserting labels.
func (r *ExternalLabelsRepository) UpdateLastSeq(ctx context.Context, subscriptionURL string, seq int64) error {
	if err := validateSubscriptionURL(subscriptionURL); err != nil {
		return err
	}
	if seq < 0 {
		return fmt.Errorf("external label event seq must be non-negative: %d", seq)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin external label cursor transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit succeeds

	if err := r.ensureStateTx(ctx, tx, subscriptionURL); err != nil {
		return err
	}
	if err := r.updateLastSeqTx(ctx, tx, subscriptionURL, seq); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit external label cursor transaction: %w", err)
	}
	return nil
}

// ListLabelsByEvent returns labels stored for a subscription event, ordered by label index.
func (r *ExternalLabelsRepository) ListLabelsByEvent(ctx context.Context, subscriptionURL string, seq int64) ([]ExternalLabel, error) {
	sqlStr := fmt.Sprintf(`SELECT %s
		FROM external_label
		WHERE subscription_url = %s AND seq = %s
		ORDER BY label_index ASC`, r.externalLabelColumns(), r.db.Placeholder(1), r.db.Placeholder(2))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, subscriptionURL, seq)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanExternalLabels(rows)
}

// CountLabels returns the number of external labels for a subscription URL.
func (r *ExternalLabelsRepository) CountLabels(ctx context.Context, subscriptionURL string) (int64, error) {
	sqlStr := fmt.Sprintf(`SELECT COUNT(*) FROM external_label WHERE subscription_url = %s`, r.db.Placeholder(1))
	var count int64
	if err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(subscriptionURL)}, &count); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *ExternalLabelsRepository) ensureStateTx(ctx context.Context, tx *sql.Tx, subscriptionURL string) error {
	sqlStr := fmt.Sprintf(`INSERT INTO label_subscription_state (url)
		VALUES (%s)
		ON CONFLICT(url) DO NOTHING`, r.db.Placeholder(1))
	_, err := tx.ExecContext(ctx, sqlStr, subscriptionURL)
	if err != nil {
		return fmt.Errorf("ensure label subscription state: %w", err)
	}
	return nil
}

func (r *ExternalLabelsRepository) insertExternalLabelTx(ctx context.Context, tx *sql.Tx, subscriptionURL string, seq int64, label ExternalLabelInput) error {
	p := func(idx int) string { return r.db.Placeholder(idx) }

	rawJSONPlaceholder := p(13)
	if r.db.Dialect() == database.PostgreSQL {
		rawJSONPlaceholder += "::jsonb"
	}

	sqlStr := fmt.Sprintf(`INSERT INTO external_label (
			subscription_url, seq, label_index, src, uri, cid, val, neg, cts, exp, sig, ver, raw_json
		) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
		ON CONFLICT(subscription_url, seq, label_index) DO NOTHING`,
		p(1), p(2), p(3), p(4), p(5), p(6), p(7), p(8), p(9), p(10), p(11), p(12), rawJSONPlaceholder)

	params := []database.Value{
		database.Text(subscriptionURL),
		database.Int(seq),
		database.Int(label.LabelIndex),
		database.Text(label.Src),
		database.Text(label.URI),
		database.NullableText(label.CID),
		database.Text(label.Val),
		database.Bool(label.Neg),
		database.Text(label.Cts),
		database.NullableText(label.Exp),
		database.NullableText(label.Sig),
		database.NullableInt(label.Ver),
		nullableNonEmptyText(label.RawJSON),
	}

	if _, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams(params)...); err != nil {
		return fmt.Errorf("insert external label seq=%d index=%d: %w", seq, label.LabelIndex, err)
	}
	return nil
}

func (r *ExternalLabelsRepository) updateLastSeqTx(ctx context.Context, tx *sql.Tx, subscriptionURL string, seq int64) error {
	sqlStr := fmt.Sprintf(`UPDATE label_subscription_state
		SET last_seq = CASE WHEN last_seq < %s THEN %s ELSE last_seq END,
			last_event_at = CASE WHEN last_seq < %s THEN %s ELSE last_event_at END,
			last_error = CASE WHEN last_seq < %s THEN NULL ELSE last_error END,
			updated_at = %s
		WHERE url = %s`,
		r.db.Placeholder(1),
		r.db.Placeholder(2),
		r.db.Placeholder(3),
		r.db.Now(),
		r.db.Placeholder(4),
		r.db.Now(),
		r.db.Placeholder(5),
	)

	params := []database.Value{
		database.Int(seq),
		database.Int(seq),
		database.Int(seq),
		database.Int(seq),
		database.Text(subscriptionURL),
	}
	if _, err := tx.ExecContext(ctx, sqlStr, r.db.ConvertParams(params)...); err != nil {
		return fmt.Errorf("update external label cursor: %w", err)
	}
	return nil
}

func (r *ExternalLabelsRepository) stateColumns() string {
	if r.db.Dialect() == database.PostgreSQL {
		return `url, labeler_did, last_seq, last_connected_at::text, last_event_at::text, last_error, created_at::text, updated_at::text`
	}
	return `url, labeler_did, last_seq, last_connected_at, last_event_at, last_error, created_at, updated_at`
}

func (r *ExternalLabelsRepository) externalLabelColumns() string {
	if r.db.Dialect() == database.PostgreSQL {
		return `id, subscription_url, seq, label_index, src, uri, cid, val, CASE WHEN neg THEN 1 ELSE 0 END AS neg, cts, exp, sig, ver, COALESCE(raw_json::text, ''), received_at::text`
	}
	return `id, subscription_url, seq, label_index, src, uri, cid, val, neg, cts, exp, sig, ver, COALESCE(raw_json, ''), received_at`
}

func validateSubscriptionURL(subscriptionURL string) error {
	if strings.TrimSpace(subscriptionURL) == "" {
		return fmt.Errorf("label subscription URL must not be empty")
	}
	return nil
}

func validateExternalLabelInputs(labels []ExternalLabelInput) error {
	for i, label := range labels {
		if label.LabelIndex < 0 {
			return fmt.Errorf("external label %d has negative label_index %d", i, label.LabelIndex)
		}
		if strings.TrimSpace(label.Src) == "" {
			return fmt.Errorf("external label %d missing src", i)
		}
		if strings.TrimSpace(label.URI) == "" {
			return fmt.Errorf("external label %d missing uri", i)
		}
		if strings.TrimSpace(label.Val) == "" {
			return fmt.Errorf("external label %d missing val", i)
		}
		if _, err := time.Parse(time.RFC3339Nano, label.Cts); err != nil {
			return fmt.Errorf("external label %d has invalid cts %q: %w", i, label.Cts, err)
		}
		if label.Exp != nil {
			if _, err := time.Parse(time.RFC3339Nano, *label.Exp); err != nil {
				return fmt.Errorf("external label %d has invalid exp %q: %w", i, *label.Exp, err)
			}
		}
	}
	return nil
}

func nullableNonEmptyText(s string) database.Value {
	if s == "" {
		return database.Null()
	}
	return database.Text(s)
}

func normalizeLastError(errText string) string {
	errText = strings.TrimSpace(strings.ToValidUTF8(errText, "�"))
	if errText == "" {
		return "unknown error"
	}
	if len(errText) > externalLabelLastErrorMaxLen {
		return errText[:externalLabelLastErrorMaxLen]
	}
	return errText
}

func scanExternalLabels(rows *sql.Rows) ([]ExternalLabel, error) {
	var labels []ExternalLabel
	for rows.Next() {
		var label ExternalLabel
		var cid, exp, sig sql.NullString
		var ver sql.NullInt64
		var neg int
		var receivedAt string

		if err := rows.Scan(
			&label.ID,
			&label.SubscriptionURL,
			&label.Seq,
			&label.LabelIndex,
			&label.Src,
			&label.URI,
			&cid,
			&label.Val,
			&neg,
			&label.Cts,
			&exp,
			&sig,
			&ver,
			&label.RawJSON,
			&receivedAt,
		); err != nil {
			return nil, err
		}

		label.Neg = neg != 0
		if cid.Valid {
			label.CID = &cid.String
		}
		if exp.Valid {
			label.Exp = &exp.String
		}
		if sig.Valid {
			label.Sig = &sig.String
		}
		if ver.Valid {
			label.Ver = &ver.Int64
		}

		parsedReceivedAt, err := parseDBTime(receivedAt)
		if err != nil {
			return nil, fmt.Errorf("parse external_label.received_at: %w", err)
		}
		label.ReceivedAt = parsedReceivedAt

		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return labels, nil
}

func parseDBTime(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	layoutsWithZone := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07",
		"2006-01-02 15:04:05.999999Z07:00",
		"2006-01-02 15:04:05.999999-07",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05-07",
	}
	for _, layout := range layoutsWithZone {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), nil
		}
	}

	layoutsWithoutZone := []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05.999999",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layoutsWithoutZone {
		if parsed, err := time.ParseInLocation(layout, value, time.UTC); err == nil {
			return parsed.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported timestamp %q", value)
}
