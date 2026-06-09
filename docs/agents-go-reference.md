# Go conventions reference

This document preserves the general Go guidance that previously lived in `AGENTS.md`.
It is intentionally not the main agent instruction file, but it can be useful as a fallback reference.

## Imports

Group imports in this order with blank lines between:

```go
import (
    "context"           // 1. Standard library
    "fmt"

    "github.com/go-chi/chi/v5"  // 2. External packages

    "github.com/GainForest/hyperindex/internal/database"  // 3. Internal packages
)
```

## Package documentation

Every package should have a doc comment:

```go
// Package config handles application configuration loading from environment variables.
package config
```

## Naming conventions

- **Packages:** lowercase, single word (`lexicon`, `oauth`, `backfill`)
- **Files:** lowercase with underscores (`did_resolver.go`, `indexing_activity.go`)
- **Types:** PascalCase (`Executor`, `RecordFetcher`, `WhereClause`)
- **Interfaces:** Noun or -er suffix (`Executor`, `Fetcher`, `Resolver`)
- **Constants:** PascalCase exported, camelCase private
- **Acronyms:** All caps (`URI`, `DID`, `HTTP`, `JSON`)

## Error handling

Wrap errors with context:

```go
if err != nil {
    return fmt.Errorf("failed to query records: %w", err)
}
```

For typed errors:

```go
type DBError struct {
    Code    string
    Message string
    Cause   error
}

func (e *DBError) Error() string { return e.Message }
func (e *DBError) Unwrap() error { return e.Cause }
```

## Context

Pass context as the first parameter:

```go
func (r *RecordsRepository) GetByURI(ctx context.Context, uri string) (*Record, error)
```

## Repository pattern

Database access goes through repositories under `internal/database/repositories/`:

```go
type RecordsRepository struct {
    db database.Executor
}

func NewRecordsRepository(db database.Executor) *RecordsRepository {
    return &RecordsRepository{db: db}
}

func (r *RecordsRepository) GetByURI(ctx context.Context, uri string) (*Record, error) {
    sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri = %s", r.recordColumns(), r.db.Placeholder(1))
    // ...
}
```

## Testing

Use table-driven tests:

```go
func TestParseLexicon(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Lexicon
        wantErr bool
    }{
        {name: "simple record", input: `{"lexicon":1}`, want: &Lexicon{}},
        {name: "invalid json", input: `{`, wantErr: true},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseLexicon(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

## Logging

Use structured logging with `log/slog`:

```go
slog.Info("Starting backfill", "collections", collections, "count", len(repos))
slog.Warn("Failed to resolve DID", "did", did, "error", err)
slog.Error("Database connection failed", "error", err)
```
