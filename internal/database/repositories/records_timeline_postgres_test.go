package repositories_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/repositories"
)

func TestRecordsRepository_RecordTimelinePostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()

	preserveURI := "at://did:plc:timeline/com.example.timeline.post/postgres-preserve"
	if _, err := repo.Insert(ctx, preserveURI, "cid-preserve-1", "did:plc:timeline", "com.example.timeline.post", `{"createdAt":"2026-01-15T10:00:00.123456789+02:00"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := repo.Insert(ctx, preserveURI, "cid-preserve-2", "did:plc:timeline", "com.example.timeline.post", `{"createdAt":"2026-01-16T10:00:00Z"}`); err != nil {
		t.Fatalf("Insert() update error = %v", err)
	}
	if got := postgresRepositoryRecordCreatedAt(t, exec, preserveURI); got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("postgres preserved record_created_at = %q, want original normalized timestamp", got)
	}

	fillURI := "at://did:plc:timeline/com.example.timeline.post/postgres-fill"
	if err := repo.BatchInsert(ctx, []*repositories.Record{
		{URI: fillURI, CID: "cid-fill-1", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"text":"missing"}`},
	}); err != nil {
		t.Fatalf("initial BatchInsert() error = %v", err)
	}
	if err := repo.BatchInsert(ctx, []*repositories.Record{
		{URI: fillURI, CID: "cid-fill-2", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-17T00:00:00Z"}`},
	}); err != nil {
		t.Fatalf("conflicting BatchInsert() error = %v", err)
	}
	if got := postgresRepositoryRecordCreatedAt(t, exec, fillURI); got != "2026-01-17T00:00:00.000Z" {
		t.Fatalf("postgres filled batch record_created_at = %q, want incoming timestamp", got)
	}

	records := []*repositories.Record{
		{URI: "at://did:plc:alice/com.example.timeline.post/r1", CID: "cid1", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-15T10:00:00Z"}`},
		{URI: "at://did:plc:bob/com.example.timeline.like/r2", CID: "cid2", DID: "did:plc:bob", Collection: "com.example.timeline.like", JSON: `{"createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:alice/com.example.timeline.post/r3", CID: "cid3", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:carol/com.example.timeline.like/r4", CID: "cid4", DID: "did:plc:carol", Collection: "com.example.timeline.like", JSON: `{"createdAt":"2026-01-15T14:00:00Z"}`},
	}
	if err := repo.BatchInsert(ctx, records); err != nil {
		t.Fatalf("timeline BatchInsert() error = %v", err)
	}

	page, err := repo.GetRecordTimeline(ctx, []string{"did:plc:alice", "did:plc:bob"}, []string{"com.example.timeline.post", "com.example.timeline.like"}, 10, nil)
	if err != nil {
		t.Fatalf("GetRecordTimeline() error = %v", err)
	}
	assertTimelineURIs(t, page, []string{
		"at://did:plc:bob/com.example.timeline.like/r2",
		"at://did:plc:alice/com.example.timeline.post/r3",
		"at://did:plc:alice/com.example.timeline.post/r1",
	})
}

func newPostgresRecordsTestExecutor(t *testing.T) *postgres.Executor {
	t.Helper()
	databaseURL, ok := safePostgresRecordsTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL record timeline test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	ctx := context.Background()
	adminExec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres admin executor: %v", err)
	}
	t.Cleanup(func() { _ = adminExec.Close() })

	schemaName := fmt.Sprintf("hyperindex_records_timeline_test_%d", time.Now().UnixNano())
	quotedSchemaName := quotePostgresRecordsIdentifier(schemaName)
	if _, err := adminExec.DB().ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchemaName)); err != nil {
		t.Fatalf("failed to create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminExec.DB().ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchemaName))
	})

	schemaURL, err := postgresRecordsURLWithSearchPath(databaseURL, schemaName)
	if err != nil {
		t.Fatalf("failed to build postgres schema URL: %v", err)
	}

	exec, err := postgres.NewExecutor(schemaURL)
	if err != nil {
		t.Fatalf("failed to create postgres schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("failed to run postgres migrations: %v", err)
	}
	return exec
}

func safePostgresRecordsTestDatabaseURL(t *testing.T) (string, bool) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return "", false
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("DATABASE_URL is not a valid URL: %v", err)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	name := strings.ToLower(strings.TrimSpace(databaseName))
	return databaseURL, name == "test" || strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "-test")
}

func postgresRecordsURLWithSearchPath(databaseURL, schemaName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func quotePostgresRecordsIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func postgresRepositoryRecordCreatedAt(t *testing.T, exec *postgres.Executor, uri string) string {
	t.Helper()
	var value sql.NullString
	if err := exec.DB().QueryRowContext(context.Background(), `
		SELECT to_char(record_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM record
		WHERE uri = $1`, uri,
	).Scan(&value); err != nil {
		t.Fatalf("failed to query postgres record_created_at for %s: %v", uri, err)
	}
	if !value.Valid {
		return ""
	}
	return value.String
}
