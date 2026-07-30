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

// LexiconWrite contains one saved Lexicon document to insert or replace.
type LexiconWrite struct {
	ID   string
	JSON string
}

// LexiconMutation describes one validated saved-Lexicon set change.
type LexiconMutation struct {
	Upserts   []LexiconWrite
	DeleteIDs []string
}

// LexiconMutationBuilder chooses a mutation from the current locked database
// state. Returning an error aborts the transaction without changing the set.
type LexiconMutationBuilder func(current []*Lexicon) (LexiconMutation, error)

// LexiconSetValidator validates the prospective database set after the mutation
// has been applied inside the transaction but before it is committed.
type LexiconSetValidator func(prospective []*Lexicon) error

// LexiconsRepository handles lexicon persistence.
type LexiconsRepository struct {
	db database.Executor
}

// NewLexiconsRepository creates a new lexicons repository.
func NewLexiconsRepository(db database.Executor) *LexiconsRepository {
	return &LexiconsRepository{db: db}
}

// Upsert inserts or updates a Lexicon.
func (r *LexiconsRepository) Upsert(ctx context.Context, id, jsonData string) error {
	_, err := r.db.Exec(ctx, r.upsertSQL(), []database.Value{
		database.Text(id),
		database.Text(jsonData),
		database.Text(jsonData),
	})
	return err
}

// UpsertMany inserts or updates all supplied Lexicons in one transaction.
func (r *LexiconsRepository) UpsertMany(ctx context.Context, lexicons []LexiconWrite) error {
	if len(lexicons) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin Lexicon batch transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, lexicon := range lexicons {
		params := r.db.ConvertParams([]database.Value{
			database.Text(lexicon.ID),
			database.Text(lexicon.JSON),
			database.Text(lexicon.JSON),
		})
		if _, err := tx.ExecContext(ctx, r.upsertSQL(), params...); err != nil {
			return fmt.Errorf("save Lexicon %s: %w", lexicon.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Lexicon batch transaction: %w", err)
	}
	return nil
}

// MutateValidated serializes a prospective-set read/check/write operation at
// the database boundary. PostgreSQL uses a transaction-scoped advisory lock so
// independent Hyperindex replicas coordinate; SQLite acquires its write lock
// before reading the current set.
func (r *LexiconsRepository) MutateValidated(ctx context.Context, build LexiconMutationBuilder, validate LexiconSetValidator) error {
	// Keep PostgreSQL at READ COMMITTED so a transaction that waited for the
	// advisory lock reads the state committed by the previous lock holder.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin validated Lexicon mutation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if r.db.Dialect() == database.PostgreSQL {
		const lexiconMutationLockID int64 = 4_869_502_884_694_896_248
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", lexiconMutationLockID); err != nil {
			return fmt.Errorf("acquire PostgreSQL Lexicon mutation lock: %w", err)
		}
	} else {
		// A write statement upgrades SQLite's deferred transaction before the
		// prospective read. Even for an empty table, SQLite acquires the writer
		// lock before executing the statement.
		if _, err := tx.ExecContext(ctx, "UPDATE lexicon SET raw_json = raw_json WHERE 0"); err != nil {
			return fmt.Errorf("acquire SQLite Lexicon mutation lock: %w", err)
		}
	}

	current, err := r.getAllTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("load current Lexicons in mutation: %w", err)
	}
	mutation, err := build(current)
	if err != nil {
		return err
	}
	for _, id := range mutation.DeleteIDs {
		query := fmt.Sprintf("DELETE FROM lexicon WHERE id = %s", r.db.Placeholder(1))
		if _, err := tx.ExecContext(ctx, query, r.db.ConvertParams([]database.Value{database.Text(id)})...); err != nil {
			return fmt.Errorf("delete Lexicon %s in mutation: %w", id, err)
		}
	}
	for _, lexicon := range mutation.Upserts {
		params := r.db.ConvertParams([]database.Value{
			database.Text(lexicon.ID),
			database.Text(lexicon.JSON),
			database.Text(lexicon.JSON),
		})
		if _, err := tx.ExecContext(ctx, r.upsertSQL(), params...); err != nil {
			return fmt.Errorf("save Lexicon %s in mutation: %w", lexicon.ID, err)
		}
	}

	prospective, err := r.getAllTx(ctx, tx)
	if err != nil {
		return fmt.Errorf("load prospective Lexicons in mutation: %w", err)
	}
	if err := validate(prospective); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit validated Lexicon mutation: %w", err)
	}
	return nil
}

func (r *LexiconsRepository) lexiconSelectFields() string {
	createdAt := "created_at"
	if r.db.Dialect() == database.PostgreSQL {
		createdAt = `to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')`
	}
	return "id, raw_json, " + createdAt
}

func parseLexiconCreatedAt(id, value string) (time.Time, error) {
	layouts := []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"}
	var parseErr error
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, value)
		if err == nil {
			return parsed, nil
		}
		parseErr = err
	}
	return time.Time{}, fmt.Errorf("parse Lexicon %s created_at %q: %w", id, value, parseErr)
}

func (r *LexiconsRepository) getAllTx(ctx context.Context, tx *sql.Tx) ([]*Lexicon, error) {
	query := fmt.Sprintf("SELECT %s FROM lexicon ORDER BY id", r.lexiconSelectFields())
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lexicons []*Lexicon
	for rows.Next() {
		var lex Lexicon
		var createdAt string
		if err := rows.Scan(&lex.ID, &lex.JSON, &createdAt); err != nil {
			return nil, err
		}
		lex.CreatedAt, err = parseLexiconCreatedAt(lex.ID, createdAt)
		if err != nil {
			return nil, err
		}
		lexicons = append(lexicons, &lex)
	}
	return lexicons, rows.Err()
}

func (r *LexiconsRepository) upsertSQL() string {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	p3 := r.db.Placeholder(3)
	if r.db.Dialect() == database.PostgreSQL {
		return fmt.Sprintf(`INSERT INTO lexicon (id, json, raw_json)
			VALUES (%s, %s::jsonb, %s)
			ON CONFLICT(id) DO UPDATE SET
				json = EXCLUDED.json,
				raw_json = EXCLUDED.raw_json`, p1, p2, p3)
	}
	return fmt.Sprintf(`INSERT INTO lexicon (id, json, raw_json)
		VALUES (%s, %s, %s)
		ON CONFLICT(id) DO UPDATE SET
			json = excluded.json,
			raw_json = excluded.raw_json`, p1, p2, p3)
}

// GetByID retrieves a lexicon by its ID.
func (r *LexiconsRepository) GetByID(ctx context.Context, id string) (*Lexicon, error) {
	sqlStr := fmt.Sprintf("SELECT %s FROM lexicon WHERE id = %s", r.lexiconSelectFields(), r.db.Placeholder(1))

	var lex Lexicon
	var createdAtStr string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(id)},
		&lex.ID, &lex.JSON, &createdAtStr)
	if err != nil {
		return nil, err
	}

	lex.CreatedAt, err = parseLexiconCreatedAt(lex.ID, createdAtStr)
	if err != nil {
		return nil, err
	}
	return &lex, nil
}

// GetAll retrieves all lexicons.
func (r *LexiconsRepository) GetAll(ctx context.Context) ([]*Lexicon, error) {
	query := fmt.Sprintf("SELECT %s FROM lexicon ORDER BY id", r.lexiconSelectFields())
	rows, err := r.db.DB().QueryContext(ctx, query)
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
		lex.CreatedAt, err = parseLexiconCreatedAt(lex.ID, createdAtStr)
		if err != nil {
			return nil, err
		}
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
