package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// Label represents an applied label on a record or account.
type Label struct {
	ID  int64
	Src string  // DID of the labeler
	URI string  // Subject URI (at:// or did:)
	CID *string // Optional CID for version-specific label
	Val string  // Label value (e.g., 'porn', '!takedown')
	Neg bool    // True if this is a negation (retraction)
	Cts time.Time
	Exp *time.Time // Optional expiration
}

// PaginatedLabels holds paginated label results.
type PaginatedLabels struct {
	Labels      []Label
	HasNextPage bool
	TotalCount  int64
}

// LabelsRepository handles label persistence.
type LabelsRepository struct {
	db database.Executor
}

// NewLabelsRepository creates a new labels repository.
func NewLabelsRepository(db database.Executor) *LabelsRepository {
	return &LabelsRepository{db: db}
}

// Insert creates a new label.
func (r *LabelsRepository) Insert(ctx context.Context, src, uri string, cid *string, val string, exp *time.Time) (*Label, error) {
	var sqlStr string
	var expStr *string
	if exp != nil {
		s := exp.Format(time.RFC3339)
		expStr = &s
	}

	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO label (src, uri, cid, val, exp)
			VALUES (%s, %s, %s, %s, %s)
			RETURNING id, src, uri, cid, val, neg, cts, exp`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3),
			r.db.Placeholder(4), r.db.Placeholder(5))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO label (src, uri, cid, val, exp)
			VALUES (%s, %s, %s, %s, %s)`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3),
			r.db.Placeholder(4), r.db.Placeholder(5))
	}

	params := []database.Value{
		database.Text(src),
		database.Text(uri),
		database.NullableText(cid),
		database.Text(val),
		database.NullableText(expStr),
	}

	if r.db.Dialect() == database.PostgreSQL {
		var label Label
		var ctsStr string
		var cidNull, expNull sql.NullString
		var neg int
		err := r.db.QueryRow(ctx, sqlStr, params,
			&label.ID, &label.Src, &label.URI, &cidNull, &label.Val, &neg, &ctsStr, &expNull)
		if err != nil {
			return nil, err
		}
		label.Neg = neg != 0
		label.Cts, _ = time.Parse(time.RFC3339, ctsStr)
		if cidNull.Valid {
			label.CID = &cidNull.String
		}
		if expNull.Valid {
			t, _ := time.Parse(time.RFC3339, expNull.String)
			label.Exp = &t
		}
		return &label, nil
	}

	result, err := r.db.Exec(ctx, sqlStr, params)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()

	return r.GetByID(ctx, id)
}

// InsertNegation creates a negation (retraction) label.
func (r *LabelsRepository) InsertNegation(ctx context.Context, src, uri, val string) (*Label, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO label (src, uri, val, neg)
			VALUES (%s, %s, %s, 1)
			RETURNING id, src, uri, cid, val, neg, cts, exp`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO label (src, uri, val, neg)
			VALUES (%s, %s, %s, 1)`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))
	}

	params := []database.Value{
		database.Text(src),
		database.Text(uri),
		database.Text(val),
	}

	if r.db.Dialect() == database.PostgreSQL {
		var label Label
		var ctsStr string
		var cidNull, expNull sql.NullString
		var neg int
		err := r.db.QueryRow(ctx, sqlStr, params,
			&label.ID, &label.Src, &label.URI, &cidNull, &label.Val, &neg, &ctsStr, &expNull)
		if err != nil {
			return nil, err
		}
		label.Neg = neg != 0
		label.Cts, _ = time.Parse(time.RFC3339, ctsStr)
		if cidNull.Valid {
			label.CID = &cidNull.String
		}
		return &label, nil
	}

	result, err := r.db.Exec(ctx, sqlStr, params)
	if err != nil {
		return nil, err
	}
	id, _ := result.LastInsertId()

	return r.GetByID(ctx, id)
}

// GetByID retrieves a label by ID.
func (r *LabelsRepository) GetByID(ctx context.Context, id int64) (*Label, error) {
	sqlStr := fmt.Sprintf(`SELECT id, src, uri, cid, val, neg, cts, exp
		FROM label WHERE id = %s`, r.db.Placeholder(1))

	var label Label
	var ctsStr string
	var cidNull, expNull sql.NullString
	var neg int

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Int(id)},
		&label.ID, &label.Src, &label.URI, &cidNull, &label.Val, &neg, &ctsStr, &expNull)
	if err != nil {
		return nil, err
	}

	label.Neg = neg != 0
	label.Cts, _ = time.Parse(time.RFC3339, ctsStr)
	if label.Cts.IsZero() {
		label.Cts, _ = time.Parse("2006-01-02 15:04:05", ctsStr)
	}
	if cidNull.Valid {
		label.CID = &cidNull.String
	}
	if expNull.Valid {
		t, _ := time.Parse(time.RFC3339, expNull.String)
		if t.IsZero() {
			t, _ = time.Parse("2006-01-02 15:04:05", expNull.String)
		}
		label.Exp = &t
	}

	return &label, nil
}

// GetByURIs retrieves active (non-negated) labels for a list of URIs.
func (r *LabelsRepository) GetByURIs(ctx context.Context, uris []string) ([]Label, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	placeholders := r.db.Placeholders(len(uris), 1)
	// Get only labels that haven't been negated
	sqlStr := fmt.Sprintf(`SELECT l.id, l.src, l.uri, l.cid, l.val, l.neg, l.cts, l.exp
		FROM label l
		WHERE l.uri IN (%s) AND l.neg = 0
		AND NOT EXISTS (
			SELECT 1 FROM label neg 
			WHERE neg.uri = l.uri AND neg.val = l.val AND neg.neg = 1 AND neg.id > l.id
		)
		ORDER BY l.cts DESC`, placeholders)

	params := make([]any, len(uris))
	for i, uri := range uris {
		params[i] = uri
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLabels(rows)
}

// GetPaginated retrieves labels with optional filters and pagination.
func (r *LabelsRepository) GetPaginated(ctx context.Context, uriFilter, valFilter *string, first int, afterID *int64) (*PaginatedLabels, error) {
	// Build WHERE clause
	var conditions []string
	var params []any
	paramIdx := 1

	if uriFilter != nil {
		conditions = append(conditions, fmt.Sprintf("uri = %s", r.db.Placeholder(paramIdx)))
		params = append(params, *uriFilter)
		paramIdx++
	}

	if valFilter != nil {
		conditions = append(conditions, fmt.Sprintf("val = %s", r.db.Placeholder(paramIdx)))
		params = append(params, *valFilter)
		paramIdx++
	}

	if afterID != nil {
		conditions = append(conditions, fmt.Sprintf("id < %s", r.db.Placeholder(paramIdx)))
		params = append(params, *afterID)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM label %s", whereClause)
	var totalCount int64
	if err := r.db.DB().QueryRowContext(ctx, countSQL, params...).Scan(&totalCount); err != nil {
		return nil, err
	}

	// Get labels
	sqlStr := fmt.Sprintf(`SELECT id, src, uri, cid, val, neg, cts, exp
		FROM label %s
		ORDER BY id DESC
		LIMIT %d`, whereClause, first+1)

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels, err := scanLabels(rows)
	if err != nil {
		return nil, err
	}

	hasNextPage := len(labels) > first
	if hasNextPage {
		labels = labels[:first]
	}

	return &PaginatedLabels{
		Labels:      labels,
		HasNextPage: hasNextPage,
		TotalCount:  totalCount,
	}, nil
}

// HasTakedown checks if a URI has an active takedown label.
func (r *LabelsRepository) HasTakedown(ctx context.Context, uri string) (bool, error) {
	sqlStr := fmt.Sprintf(`SELECT COUNT(*) FROM label 
		WHERE uri = %s AND val = '!takedown' AND neg = 0
		AND NOT EXISTS (
			SELECT 1 FROM label neg 
			WHERE neg.uri = label.uri AND neg.val = '!takedown' AND neg.neg = 1 AND neg.id > label.id
		)`, r.db.Placeholder(1))

	var count int64
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(uri)}, &count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetTakedownURIs returns URIs that have active takedown labels from a list.
func (r *LabelsRepository) GetTakedownURIs(ctx context.Context, uris []string) ([]string, error) {
	if len(uris) == 0 {
		return nil, nil
	}

	placeholders := r.db.Placeholders(len(uris), 1)
	sqlStr := fmt.Sprintf(`SELECT DISTINCT l.uri FROM label l
		WHERE l.uri IN (%s) AND l.val = '!takedown' AND l.neg = 0
		AND NOT EXISTS (
			SELECT 1 FROM label neg 
			WHERE neg.uri = l.uri AND neg.val = '!takedown' AND neg.neg = 1 AND neg.id > l.id
		)`, placeholders)

	params := make([]any, len(uris))
	for i, uri := range uris {
		params[i] = uri
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []string
	for rows.Next() {
		var uri string
		if err := rows.Scan(&uri); err != nil {
			return nil, err
		}
		result = append(result, uri)
	}

	return result, rows.Err()
}

// DeleteAll removes all labels.
func (r *LabelsRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, "DELETE FROM label", nil)
	return err
}

// IsValidSubjectURI validates an AT Protocol subject URI format.
func IsValidSubjectURI(uri string) bool {
	return strings.HasPrefix(uri, "at://") || strings.HasPrefix(uri, "did:")
}

// Helper function to scan labels from rows
func scanLabels(rows *sql.Rows) ([]Label, error) {
	var labels []Label
	for rows.Next() {
		var label Label
		var ctsStr string
		var cidNull, expNull sql.NullString
		var neg int

		if err := rows.Scan(&label.ID, &label.Src, &label.URI, &cidNull, &label.Val, &neg, &ctsStr, &expNull); err != nil {
			return nil, err
		}

		label.Neg = neg != 0
		label.Cts, _ = time.Parse(time.RFC3339, ctsStr)
		if label.Cts.IsZero() {
			label.Cts, _ = time.Parse("2006-01-02 15:04:05", ctsStr)
		}
		if cidNull.Valid {
			label.CID = &cidNull.String
		}
		if expNull.Valid {
			t, _ := time.Parse(time.RFC3339, expNull.String)
			if t.IsZero() {
				t, _ = time.Parse("2006-01-02 15:04:05", expNull.String)
			}
			label.Exp = &t
		}
		labels = append(labels, label)
	}

	return labels, rows.Err()
}
