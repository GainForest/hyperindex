// Package sqlite provides a SQLite implementation of the database Executor interface.
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite" // Pure Go SQLite driver

	"github.com/GainForest/hyperindex/internal/database"
)

// validJSONFieldName matches safe JSON field names to prevent SQL injection.
var validJSONFieldName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Executor implements database.Executor for SQLite.
type Executor struct {
	db *sql.DB
}

// NewExecutor creates a new SQLite executor from a database URL.
// URL format: "sqlite:path/to/file.db" or "sqlite::memory:"
func NewExecutor(databaseURL string) (*Executor, error) {
	// Parse the URL to get the file path
	path := strings.TrimPrefix(databaseURL, "sqlite:")
	if path == "" {
		path = ":memory:"
	}

	// Open the database
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, database.ConnectionError("failed to open SQLite database", err)
	}

	// Configure connection pool for SQLite
	db.SetMaxOpenConns(1) // SQLite doesn't handle concurrent writes well
	db.SetMaxIdleConns(1)

	// Enable foreign keys and WAL mode for better performance
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		_ = db.Close()
		return nil, database.ConnectionError("failed to enable foreign keys", err)
	}

	// Enable WAL mode for better concurrent read performance (skip for :memory:)
	if path != ":memory:" {
		if _, err := db.Exec("PRAGMA journal_mode = WAL"); err != nil {
			_ = db.Close()
			return nil, database.ConnectionError("failed to enable WAL mode", err)
		}
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, database.ConnectionError("failed to ping SQLite database", err)
	}

	return &Executor{db: db}, nil
}

// QueryRow executes a query expected to return at most one row.
func (e *Executor) QueryRow(ctx context.Context, sqlStr string, params []database.Value, dest ...any) error {
	args := convertParams(params)
	row := e.db.QueryRowContext(ctx, sqlStr, args...)
	if err := row.Scan(dest...); err != nil {
		if err == sql.ErrNoRows {
			return err
		}
		return database.QueryError("failed to scan row", err)
	}
	return nil
}

// BeginTx starts a new database transaction.
func (e *Executor) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return e.db.BeginTx(ctx, opts)
}

// ConvertParams converts []database.Value to []any for direct *sql.DB usage.
func (e *Executor) ConvertParams(params []database.Value) []any {
	return convertParams(params)
}

// Exec executes a statement without returning results.
func (e *Executor) Exec(ctx context.Context, sqlStr string, params []database.Value) (sql.Result, error) {
	args := convertParams(params)
	result, err := e.db.ExecContext(ctx, sqlStr, args...)
	if err != nil {
		// Check for constraint violations
		if strings.Contains(err.Error(), "UNIQUE constraint") ||
			strings.Contains(err.Error(), "FOREIGN KEY constraint") {
			return nil, database.ConstraintError("constraint violation", err)
		}
		return nil, database.QueryError("failed to execute statement", err)
	}
	return result, nil
}

// Dialect returns SQLite.
func (e *Executor) Dialect() database.Dialect {
	return database.SQLite
}

// Placeholder returns "?" for all parameters (SQLite ignores index).
func (e *Executor) Placeholder(index int) string {
	return "?"
}

// Placeholders returns a comma-separated list of "?" placeholders.
func (e *Executor) Placeholders(count, startIndex int) string {
	if count <= 0 {
		return ""
	}
	placeholders := make([]string, count)
	for i := 0; i < count; i++ {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ", ")
}

// JSONExtract generates SQLite JSON extraction SQL.
// The field parameter is validated to prevent SQL injection.
func (e *Executor) JSONExtract(column, field string) string {
	if !validJSONFieldName.MatchString(field) {
		panic(fmt.Sprintf("sqlite: invalid JSON field name: %q (must match ^[a-zA-Z_][a-zA-Z0-9_]*$)", field))
	}
	return fmt.Sprintf("json_extract(%s, '$.%s')", column, field)
}

// JSONExtractPath generates SQLite JSON path extraction SQL.
// All path segments are validated to prevent SQL injection.
func (e *Executor) JSONExtractPath(column string, path []string) string {
	for _, p := range path {
		if !validJSONFieldName.MatchString(p) {
			panic(fmt.Sprintf("sqlite: invalid JSON path segment: %q (must match ^[a-zA-Z_][a-zA-Z0-9_]*$)", p))
		}
	}
	jsonPath := "$." + strings.Join(path, ".")
	return fmt.Sprintf("json_extract(%s, '%s')", column, jsonPath)
}

// Now returns SQLite's current timestamp function.
func (e *Executor) Now() string {
	return "datetime('now')"
}

// Close closes the database connection.
func (e *Executor) Close() error {
	return e.db.Close()
}

// DB returns the underlying *sql.DB.
func (e *Executor) DB() *sql.DB {
	return e.db
}

// convertParams converts database.Value slice to []any for sql.DB methods.
func convertParams(params []database.Value) []any {
	if len(params) == 0 {
		return nil
	}

	args := make([]any, len(params))
	for i, param := range params {
		switch v := param.(type) {
		case database.TextValue:
			args[i] = string(v)
		case database.IntValue:
			args[i] = int64(v)
		case database.FloatValue:
			args[i] = float64(v)
		case database.BoolValue:
			// SQLite uses integers for booleans
			if bool(v) {
				args[i] = 1
			} else {
				args[i] = 0
			}
		case database.NullValue:
			args[i] = nil
		case database.BlobValue:
			args[i] = []byte(v)
		case database.TimestamptzValue:
			args[i] = string(v)
		default:
			args[i] = param
		}
	}
	return args
}
