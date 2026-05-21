package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

const (
	externalLabelLastErrorMaxLen = 4096
	labelSubjectKeySeparator     = "\x00"
)

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

// LabelSubject identifies a subject whose external labels should be queried.
// URI is an AT-URI for record subjects or a DID for account subjects. CID is
// optional and restricts record-label matches to labels that apply to that CID.
type LabelSubject struct {
	URI string
	CID string
}

// Key returns the stable map key used for labels fetched for this subject.
// URI-only subjects use the URI directly; CID-specific subjects include the CID
// so the same record URI can be queried for multiple CIDs without collisions.
func (s LabelSubject) Key() string {
	if s.CID == "" {
		return s.URI
	}
	return s.URI + labelSubjectKeySeparator + s.CID
}

// ExternalLabelFilter restricts external label lookups by source, value, and
// active-label state. Empty Sources or Values means that dimension is not
// restricted. Set ActiveOnly to true for the public API's default current-label
// semantics.
type ExternalLabelFilter struct {
	Sources    []string
	Values     []string
	ActiveOnly bool
}

// ExternalLabelsRepository handles external labeler event persistence and lookup.
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

// GetBySubjects retrieves external labels for the requested subjects in one
// query. The returned map is keyed by LabelSubject.Key(), which preserves CID
// distinctions when the same URI is requested for multiple record versions.
func (r *ExternalLabelsRepository) GetBySubjects(ctx context.Context, subjects []LabelSubject, filter ExternalLabelFilter) (map[string][]ExternalLabel, error) {
	normalizedSubjects, err := normalizeLabelSubjects(subjects)
	if err != nil {
		return nil, err
	}
	if err := validateExternalLabelFilter(filter); err != nil {
		return nil, err
	}

	labelsBySubject := make(map[string][]ExternalLabel, len(normalizedSubjects))
	for _, subject := range normalizedSubjects {
		labelsBySubject[subject.Key()] = nil
	}
	if len(normalizedSubjects) == 0 {
		return labelsBySubject, nil
	}

	var params []database.Value
	requestedSubjectSQL := buildRequestedSubjectSQL(r.db, normalizedSubjects, &params)
	conditions := []string{`(rs.cid IS NULL OR el.cid IS NULL OR el.cid = rs.cid)`}
	placeholderIdx := len(params) + 1

	if len(filter.Sources) > 0 {
		placeholders := r.db.Placeholders(len(filter.Sources), placeholderIdx)
		conditions = append(conditions, fmt.Sprintf("el.src IN (%s)", placeholders))
		for _, source := range filter.Sources {
			params = append(params, database.Text(source))
			placeholderIdx++
		}
	}

	if len(filter.Values) > 0 {
		placeholders := r.db.Placeholders(len(filter.Values), placeholderIdx)
		conditions = append(conditions, fmt.Sprintf("el.val IN (%s)", placeholders))
		for _, value := range filter.Values {
			params = append(params, database.Text(value))
			placeholderIdx++
		}
	}

	if filter.ActiveOnly {
		conditions = append(conditions, externalLabelActivePredicate(r.db, "el"))
	}

	orderByCts := externalLabelTimestampExpr(r.db, "el.cts")
	sqlStr := fmt.Sprintf(`WITH requested_subject(subject_key, uri, cid) AS (
		%s
	)
	SELECT rs.subject_key, %s
	FROM requested_subject rs
	JOIN external_label el ON el.uri = rs.uri
	WHERE %s
	ORDER BY rs.subject_key ASC, %s DESC, el.id DESC`,
		requestedSubjectSQL,
		r.externalLabelColumnsForAlias("el"),
		strings.Join(conditions, "\n\t\tAND "),
		orderByCts,
	)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, r.db.ConvertParams(params)...)
	if err != nil {
		return nil, fmt.Errorf("get external labels by subjects: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var subjectKey string
		label, err := scanExternalLabelRow(rows, &subjectKey)
		if err != nil {
			return nil, err
		}
		labelsBySubject[subjectKey] = append(labelsBySubject[subjectKey], label)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return labelsBySubject, nil
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
	return r.externalLabelColumnsForAlias("")
}

func (r *ExternalLabelsRepository) externalLabelColumnsForAlias(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}

	if r.db.Dialect() == database.PostgreSQL {
		return fmt.Sprintf(`%sid, %ssubscription_url, %sseq, %slabel_index, %ssrc, %suri, %scid, %sval, CASE WHEN %sneg THEN 1 ELSE 0 END AS neg, %scts, %sexp, %ssig, %sver, COALESCE(%sraw_json::text, ''), %sreceived_at::text`,
			prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix)
	}
	return fmt.Sprintf(`%sid, %ssubscription_url, %sseq, %slabel_index, %ssrc, %suri, %scid, %sval, %sneg, %scts, %sexp, %ssig, %sver, COALESCE(%sraw_json, ''), %sreceived_at`,
		prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix, prefix)
}

func buildRequestedSubjectSQL(exec database.Executor, subjects []LabelSubject, params *[]database.Value) string {
	selects := make([]string, 0, len(subjects))
	for _, subject := range subjects {
		base := len(*params) + 1
		selects = append(selects, fmt.Sprintf(
			"SELECT CAST(%s AS TEXT) AS subject_key, CAST(%s AS TEXT) AS uri, CAST(%s AS TEXT) AS cid",
			exec.Placeholder(base),
			exec.Placeholder(base+1),
			exec.Placeholder(base+2),
		))
		*params = append(*params,
			database.Text(subject.Key()),
			database.Text(subject.URI),
			nullableNonEmptyText(subject.CID),
		)
	}
	return strings.Join(selects, "\n\t\tUNION ALL ")
}

func externalLabelActivePredicate(exec database.Executor, alias string) string {
	negFalse := "0"
	if exec.Dialect() == database.PostgreSQL {
		negFalse = "false"
	}

	currentCts := externalLabelTimestampExpr(exec, alias+".cts")
	newerCts := externalLabelTimestampExpr(exec, "newer.cts")
	expiryPredicate := externalLabelExpiryPredicate(exec, alias)

	return fmt.Sprintf(`(
			%s.neg = %s
			AND (%s.exp IS NULL OR %s)
			AND NOT EXISTS (
				SELECT 1
				FROM external_label newer
				WHERE newer.src = %s.src
					AND newer.uri = %s.uri
					AND newer.val = %s.val
					AND (
						%s > %s
						OR (%s = %s AND newer.id > %s.id)
					)
			)
		)`,
		alias,
		negFalse,
		alias,
		expiryPredicate,
		alias,
		alias,
		alias,
		newerCts,
		currentCts,
		newerCts,
		currentCts,
		alias,
	)
}

func externalLabelExpiryPredicate(exec database.Executor, alias string) string {
	if exec.Dialect() == database.PostgreSQL {
		return fmt.Sprintf("(%s.exp)::timestamptz > NOW()", alias)
	}
	return fmt.Sprintf("julianday(%s.exp) > julianday('now')", alias)
}

func externalLabelTimestampExpr(exec database.Executor, expression string) string {
	if exec.Dialect() == database.PostgreSQL {
		return fmt.Sprintf("(%s)::timestamptz", expression)
	}
	return fmt.Sprintf("julianday(%s)", expression)
}

func normalizeLabelSubjects(subjects []LabelSubject) ([]LabelSubject, error) {
	if len(subjects) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(subjects))
	normalized := make([]LabelSubject, 0, len(subjects))
	for i, subject := range subjects {
		if err := validateExternalLabelString(fmt.Sprintf("external label subject %d URI", i), subject.URI); err != nil {
			return nil, err
		}
		if subject.CID != "" {
			if err := validateExternalLabelString(fmt.Sprintf("external label subject %d CID", i), subject.CID); err != nil {
				return nil, err
			}
		}

		key := subject.Key()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, subject)
	}

	return normalized, nil
}

func validateExternalLabelFilter(filter ExternalLabelFilter) error {
	if len(filter.Sources) > MaxINListSize {
		return fmt.Errorf("external label source filter exceeds maximum of %d values", MaxINListSize)
	}
	for i, source := range filter.Sources {
		if err := validateExternalLabelString(fmt.Sprintf("external label source filter %d", i), source); err != nil {
			return err
		}
	}

	if len(filter.Values) > MaxINListSize {
		return fmt.Errorf("external label value filter exceeds maximum of %d values", MaxINListSize)
	}
	for i, value := range filter.Values {
		if err := validateExternalLabelString(fmt.Sprintf("external label value filter %d", i), value); err != nil {
			return err
		}
	}

	return nil
}

func validateExternalLabelString(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be empty", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not have leading or trailing whitespace", name)
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must not contain NUL bytes", name)
	}
	return nil
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
		label, err := scanExternalLabelRow(rows)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return labels, nil
}

type externalLabelScanner interface {
	Scan(dest ...any) error
}

func scanExternalLabelRow(scanner externalLabelScanner, prefixDest ...any) (ExternalLabel, error) {
	var label ExternalLabel
	var cid, exp, sig sql.NullString
	var ver sql.NullInt64
	var neg int
	var receivedAt string

	dest := make([]any, 0, len(prefixDest)+15)
	dest = append(dest, prefixDest...)
	dest = append(dest,
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
	)

	if err := scanner.Scan(dest...); err != nil {
		return ExternalLabel{}, err
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
		return ExternalLabel{}, fmt.Errorf("parse external_label.received_at: %w", err)
	}
	label.ReceivedAt = parsedReceivedAt

	return label, nil
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
