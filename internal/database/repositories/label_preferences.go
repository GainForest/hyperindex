package repositories

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// LabelPreference represents a user's preference for a specific label.
type LabelPreference struct {
	DID        string
	LabelVal   string
	Visibility LabelVisibility
	CreatedAt  time.Time
}

// LabelPreferencesRepository handles label preference persistence.
type LabelPreferencesRepository struct {
	db database.Executor
}

// NewLabelPreferencesRepository creates a new label preferences repository.
func NewLabelPreferencesRepository(db database.Executor) *LabelPreferencesRepository {
	return &LabelPreferencesRepository{db: db}
}

// GetByDID retrieves all label preferences for a user.
func (r *LabelPreferencesRepository) GetByDID(ctx context.Context, did string) ([]LabelPreference, error) {
	sqlStr := fmt.Sprintf(`SELECT did, label_val, visibility, created_at 
		FROM actor_label_preference 
		WHERE did = %s 
		ORDER BY label_val`, r.db.Placeholder(1))

	rows, err := r.db.DB().QueryContext(ctx, sqlStr, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanLabelPreferences(rows)
}

// Get retrieves a specific label preference for a user.
func (r *LabelPreferencesRepository) Get(ctx context.Context, did, labelVal string) (*LabelPreference, error) {
	sqlStr := fmt.Sprintf(`SELECT did, label_val, visibility, created_at 
		FROM actor_label_preference 
		WHERE did = %s AND label_val = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2))

	var pref LabelPreference
	var createdAtStr string

	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(did), database.Text(labelVal)},
		&pref.DID, &pref.LabelVal, &pref.Visibility, &createdAtStr)
	if err != nil {
		return nil, err
	}

	pref.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	if pref.CreatedAt.IsZero() {
		pref.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
	}

	return &pref, nil
}

// Set creates or updates a label preference.
func (r *LabelPreferencesRepository) Set(ctx context.Context, did, labelVal string, visibility LabelVisibility) (*LabelPreference, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO actor_label_preference (did, label_val, visibility)
			VALUES (%s, %s, %s)
			ON CONFLICT (did, label_val) DO UPDATE SET
				visibility = EXCLUDED.visibility,
				created_at = NOW()`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO actor_label_preference (did, label_val, visibility)
			VALUES (%s, %s, %s)
			ON CONFLICT (did, label_val) DO UPDATE SET
				visibility = excluded.visibility,
				created_at = datetime('now')`,
			r.db.Placeholder(1), r.db.Placeholder(2), r.db.Placeholder(3))
	}

	params := []database.Value{
		database.Text(did),
		database.Text(labelVal),
		database.Text(string(visibility)),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	if err != nil {
		return nil, err
	}

	return r.Get(ctx, did, labelVal)
}

// Delete removes a label preference (resets to default).
func (r *LabelPreferencesRepository) Delete(ctx context.Context, did, labelVal string) error {
	sqlStr := fmt.Sprintf(`DELETE FROM actor_label_preference 
		WHERE did = %s AND label_val = %s`,
		r.db.Placeholder(1), r.db.Placeholder(2))

	params := []database.Value{
		database.Text(did),
		database.Text(labelVal),
	}

	_, err := r.db.Exec(ctx, sqlStr, params)
	return err
}

// DeleteByDID removes all label preferences for a user.
func (r *LabelPreferencesRepository) DeleteByDID(ctx context.Context, did string) error {
	sqlStr := fmt.Sprintf("DELETE FROM actor_label_preference WHERE did = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(did)})
	return err
}

// Helper function to scan label preferences from rows
func scanLabelPreferences(rows *sql.Rows) ([]LabelPreference, error) {
	var preferences []LabelPreference
	for rows.Next() {
		var pref LabelPreference
		var createdAtStr string

		if err := rows.Scan(&pref.DID, &pref.LabelVal, &pref.Visibility, &createdAtStr); err != nil {
			return nil, err
		}

		pref.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		if pref.CreatedAt.IsZero() {
			pref.CreatedAt, _ = time.Parse("2006-01-02 15:04:05", createdAtStr)
		}
		preferences = append(preferences, pref)
	}

	return preferences, rows.Err()
}
