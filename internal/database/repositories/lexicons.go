package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
)

// Lexicon represents an AT Protocol lexicon schema.
type Lexicon struct {
	ID        string
	JSON      string
	CreatedAt time.Time
}

// LexiconsRepository handles lexicon persistence.
type LexiconsRepository struct {
	db database.Executor
}

// NewLexiconsRepository creates a new lexicons repository.
func NewLexiconsRepository(db database.Executor) *LexiconsRepository {
	return &LexiconsRepository{db: db}
}

// Upsert inserts or updates a lexicon.
func (r *LexiconsRepository) Upsert(ctx context.Context, id, jsonData string) error {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO lexicon (id, json)
			VALUES (%s, %s::jsonb)
			ON CONFLICT(id) DO UPDATE SET
				json = EXCLUDED.json`, p1, p2)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO lexicon (id, json)
			VALUES (%s, %s)
			ON CONFLICT(id) DO UPDATE SET
				json = excluded.json`, p1, p2)
	}

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(id),
		database.Text(jsonData),
	})
	return err
}

// GetByID retrieves a lexicon by its ID.
func (r *LexiconsRepository) GetByID(ctx context.Context, id string) (*Lexicon, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf("SELECT id, json::text, created_at::text FROM lexicon WHERE id = %s",
			r.db.Placeholder(1))
	default:
		sqlStr = fmt.Sprintf("SELECT id, json, created_at FROM lexicon WHERE id = %s",
			r.db.Placeholder(1))
	}

	var lex Lexicon
	var createdAtStr string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(id)},
		&lex.ID, &lex.JSON, &createdAtStr)
	if err != nil {
		return nil, err
	}

	lex.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
	return &lex, nil
}

// GetAll retrieves all lexicons.
func (r *LexiconsRepository) GetAll(ctx context.Context) ([]*Lexicon, error) {
	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = "SELECT id, json::text, created_at::text FROM lexicon ORDER BY id"
	default:
		sqlStr = "SELECT id, json, created_at FROM lexicon ORDER BY id"
	}

	rows, err := r.db.DB().QueryContext(ctx, sqlStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lexicons []*Lexicon
	for rows.Next() {
		var lex Lexicon
		var createdAtStr string
		if err := rows.Scan(&lex.ID, &lex.JSON, &createdAtStr); err != nil {
			return nil, err
		}
		lex.CreatedAt, _ = time.Parse(time.RFC3339, createdAtStr)
		lexicons = append(lexicons, &lex)
	}

	return lexicons, rows.Err()
}

// Delete removes a lexicon by ID.
func (r *LexiconsRepository) Delete(ctx context.Context, id string) error {
	sqlStr := fmt.Sprintf("DELETE FROM lexicon WHERE id = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(id)})
	return err
}

// DeleteAll removes all lexicons.
func (r *LexiconsRepository) DeleteAll(ctx context.Context) error {
	_, err := r.db.Exec(ctx, "DELETE FROM lexicon", nil)
	return err
}

// GetCount returns the total number of lexicons.
func (r *LexiconsRepository) GetCount(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM lexicon", nil, &count)
	return count, err
}

// Exists checks if a lexicon exists.
func (r *LexiconsRepository) Exists(ctx context.Context, id string) (bool, error) {
	var count int64
	sqlStr := fmt.Sprintf("SELECT COUNT(*) FROM lexicon WHERE id = %s", r.db.Placeholder(1))
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(id)}, &count)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}
