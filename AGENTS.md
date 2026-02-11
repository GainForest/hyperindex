# AGENTS.md - Hypergoat Development Guide

Hypergoat is a Go port of Quickslice - an AT Protocol AppView server that indexes
Lexicon-defined records and exposes them via a dynamically-generated GraphQL API.

**Status:** Core functionality complete (Phases 1-7). All tests passing.

Use `bd` for task tracking. Run `bd onboard` to get started.

## Build/Test Commands

```bash
# Build
make build                     # Build binary to bin/hypergoat
go build ./...                 # Build all packages (quick check)

# Run
make run                       # Build and run server
go run ./cmd/hypergoat         # Run directly
make dev                       # Run with hot reload (requires air)

# Test - ALL TESTS
make test                      # Run all tests with race detector
go test ./...                  # Run all tests (faster, no race)

# Test - SINGLE TEST
go test -v -run TestName ./...                    # Run test by name pattern
go test -v -run TestParseLexicon ./internal/lexicon/...  # Specific package
go test -v ./internal/graphql/admin/...           # All tests in package

# Test - WITH COVERAGE
make test-coverage             # Generate coverage.html report

# Lint & Format
make lint                      # Run golangci-lint
make fmt                       # Format with go fmt + gofumpt
go fmt ./...                   # Format only

# Install dev tools
make tools                     # Install air, golangci-lint, gofumpt, migrate
```

## Code Style Guidelines

### Imports
Group imports in this order with blank lines between:
```go
import (
    "context"           // 1. Standard library
    "fmt"

    "github.com/go-chi/chi/v5"  // 2. External packages

    "github.com/GainForest/hypergoat/internal/database"  // 3. Internal packages
)
```

### Package Documentation
Every package must have a doc comment:
```go
// Package config handles application configuration loading from environment variables.
package config
```

### Naming Conventions
- **Packages:** lowercase, single word (`lexicon`, `oauth`, `backfill`)
- **Files:** lowercase with underscores (`did_resolver.go`, `jetstream_activity.go`)
- **Types:** PascalCase (`Executor`, `RecordFetcher`, `WhereClause`)
- **Interfaces:** Noun or -er suffix (`Executor`, `Fetcher`, `Resolver`)
- **Constants:** PascalCase exported, camelCase private
- **Acronyms:** All caps (`URI`, `DID`, `HTTP`, `JSON`)

### Error Handling
Always wrap errors with context:
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

### Context
Always pass context as the first parameter:
```go
func (r *RecordsRepository) GetByURI(ctx context.Context, uri string) (*Record, error)
```

### Repository Pattern
All database access goes through repositories in `internal/database/repositories/`:
```go
type RecordsRepository struct {
    db database.Executor
}

func NewRecordsRepository(db database.Executor) *RecordsRepository {
    return &RecordsRepository{db: db}
}

func (r *RecordsRepository) GetByURI(ctx context.Context, uri string) (*Record, error) {
    // Use r.db.Placeholder() for SQL parameters
    sqlStr := fmt.Sprintf("SELECT %s FROM record WHERE uri = %s", r.recordColumns(), r.db.Placeholder(1))
    // ...
}
```

### Testing
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

### Logging
Use structured logging with `log/slog`:
```go
slog.Info("Starting backfill", "collections", collections, "count", len(repos))
slog.Warn("Failed to resolve DID", "did", did, "error", err)
slog.Error("Database connection failed", "error", err)
```

## Project Structure

```
cmd/hypergoat/          # Main entry point (server initialization, routing)
internal/
  backfill/             # Historical data backfill from AT Protocol relays
  config/               # Configuration loading from environment
  database/
    migrations/         # SQL migrations (auto-run on startup)
    repositories/       # Data access layer (records, actors, lexicons, oauth, etc.)
    sqlite/             # SQLite implementation (pure Go, no CGO)
    postgres/           # PostgreSQL implementation (pgx)
  graphql/
    admin/              # Admin API (schema.go, resolvers.go, handler.go, types.go)
    schema/             # Public schema builder (dynamic from lexicons)
    resolver/           # Public resolvers and context
    query/              # Connection types (Relay spec)
    subscription/       # WebSocket subscriptions (graphql-transport-ws)
    types/              # GraphQL type mapping from lexicons
  integration/          # Integration tests
  jetstream/            # Real-time AT Protocol event consumer
  lexicon/              # Lexicon parsing, registry, NSID utilities
  oauth/                # OAuth 2.0 + DPoP + PKCE implementation
  server/               # HTTP handlers (GraphiQL, OAuth endpoints)
  workers/              # Background jobs (activity cleanup, backfill state)
docs/                   # Implementation plan and documentation
testdata/               # Test fixtures and sample lexicons
```

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below.

1. **Run quality gates** (if code changed):
   ```bash
   go build ./...
   go test ./...
   ```
2. **Commit and PUSH**:
   ```bash
   git add -A && git commit -m "feat: description"
   git push origin $branchname
   git status  # MUST show "up to date with origin"
   ```
3. **Verify** - Work is NOT complete until `git push` succeeds

**CRITICAL:** Never stop before pushing - that leaves work stranded locally.
