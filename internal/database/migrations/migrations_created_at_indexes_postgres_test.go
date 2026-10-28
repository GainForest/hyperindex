package migrations_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
)

// newIsolatedPostgresExecutor returns an executor on a fresh schema of the
// PostgreSQL test database, or skips when DATABASE_URL is not a safe one.
func newIsolatedPostgresExecutor(t *testing.T) (*postgres.Executor, string) {
	t.Helper()
	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL migration test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}
	ctx := context.Background()
	admin, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("create postgres admin executor: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schemaName := fmt.Sprintf("hyperindex_created_at_idx_test_%d", time.Now().UnixNano())
	quoted := quotePostgresIdentifier(schemaName)
	if _, err := admin.DB().ExecContext(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatalf("create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.DB().ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+quoted+" CASCADE")
	})

	schemaURL, err := postgresURLWithSearchPath(databaseURL, schemaName)
	if err != nil {
		t.Fatalf("build postgres schema URL: %v", err)
	}
	exec, err := postgres.NewExecutor(schemaURL)
	if err != nil {
		t.Fatalf("create postgres schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })
	return exec, schemaName
}

func postgresIndexDefinition(ctx context.Context, t *testing.T, exec *postgres.Executor, name string) string {
	t.Helper()
	var definition string
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT pg_get_indexdef(indexrelid) FROM pg_index WHERE indexrelid = to_regclass($1)`, name,
	).Scan(&definition); err != nil {
		t.Fatalf("read definition of index %s: %v", name, err)
	}
	return definition
}

func postgresMigrationApplied(ctx context.Context, t *testing.T, exec *postgres.Executor, version string) bool {
	t.Helper()
	var applied bool
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version).Scan(&applied); err != nil {
		t.Fatalf("check migration %s: %v", version, err)
	}
	return applied
}

// Migrations 018 and 019 add the full createdAt indexes and drop the hand-made
// stopgap with CREATE/DROP INDEX CONCURRENTLY. PostgreSQL rejects those inside a
// transaction, so applying them at all proves they ran through the runner's
// no-transaction path. The partial timeline indexes from 010 and 017 stay,
// because readers outside Hyperindex still order NULLS FIRST.
func TestMigrations_018And019AddFullIndexesAndDropStopgapConcurrentlyPostgres(t *testing.T) {
	exec, schemaName := newIsolatedPostgresExecutor(t)
	ctx := context.Background()

	if err := migrations.RunThrough(ctx, exec, "017"); err != nil {
		t.Fatalf("RunThrough(017) error = %v", err)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_timeline_valid")
	if _, err := exec.DB().ExecContext(ctx,
		`CREATE INDEX idx_record_collection_json_created ON record (collection, (json->>'createdAt') DESC, uri DESC)`); err != nil {
		t.Fatalf("create stopgap index: %v", err)
	}

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	assertPostgresIndexValid(ctx, t, exec, "idx_record_collection_created_at")
	if def := postgresIndexDefinition(ctx, t, exec, "idx_record_collection_created_at"); !strings.Contains(def, "record_created_at DESC NULLS LAST") || strings.Contains(def, "WHERE") {
		t.Fatalf("idx_record_collection_created_at must be a full NULLS LAST index, got:\n%s", def)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_collection_created_at_valid")
	if def := postgresIndexDefinition(ctx, t, exec, "idx_record_collection_created_at_valid"); !strings.Contains(def, "record_created_at DESC NULLS LAST") ||
		!strings.Contains(def, "validation_status = 'valid'") || strings.Contains(def, "IS NOT NULL") {
		t.Fatalf("idx_record_collection_created_at_valid must be valid-only and keep undated rows, got:\n%s", def)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_author_collection_created_at")
	if def := postgresIndexDefinition(ctx, t, exec, "idx_record_author_collection_created_at"); !strings.Contains(def, "(did, collection, record_created_at DESC NULLS LAST, uri DESC)") || strings.Contains(def, "WHERE") {
		t.Fatalf("idx_record_author_collection_created_at must be a full NULLS LAST index, got:\n%s", def)
	}
	assertPostgresIndexNotExists(ctx, t, exec, schemaName, "idx_record_collection_json_created")
	// The partial indexes are retained for outside readers that order NULLS FIRST.
	for _, kept := range []string{"idx_record_timeline_collection_created", "idx_record_timeline_valid", "idx_record_timeline_author_collection_created"} {
		assertPostgresIndexExists(ctx, t, exec, schemaName, kept)
		assertPostgresIndexValid(ctx, t, exec, kept)
	}
	for _, version := range []string{"018", "019"} {
		if !postgresMigrationApplied(ctx, t, exec, version) {
			t.Fatalf("migration %s was not recorded", version)
		}
	}
}

func TestMigrations_018And019RollBackAndReapplyPostgres(t *testing.T) {
	exec, schemaName := newIsolatedPostgresExecutor(t)
	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, version := range []string{"019", "018"} {
		if err := migrations.Rollback(ctx, exec); err != nil {
			t.Fatalf("Rollback(%s) error = %v", version, err)
		}
		if postgresMigrationApplied(ctx, t, exec, version) {
			t.Fatalf("migration %s still recorded after rollback", version)
		}
	}
	for _, removed := range []string{"idx_record_collection_created_at", "idx_record_collection_created_at_valid", "idx_record_author_collection_created_at"} {
		assertPostgresIndexNotExists(ctx, t, exec, schemaName, removed)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_timeline_valid")

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("re-Run() error = %v", err)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_collection_created_at")
	assertPostgresIndexValid(ctx, t, exec, "idx_record_timeline_valid")
}

// An interrupted CREATE INDEX CONCURRENTLY leaves an invalid index under the
// target name, which IF NOT EXISTS would accept. The runner must stop with the
// recovery steps, leave the version unrecorded, and succeed once the operator
// drops the leftover index.
func TestMigrations_018RefusesInvalidLeftoverIndexPostgres(t *testing.T) {
	exec, _ := newIsolatedPostgresExecutor(t)
	ctx := context.Background()
	if err := migrations.RunThrough(ctx, exec, "017"); err != nil {
		t.Fatalf("RunThrough(017) error = %v", err)
	}

	// Two rows in one collection make a unique concurrent build fail, which
	// leaves an invalid index exactly as an interrupted build would.
	if _, err := exec.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json) VALUES
			('at://did:plc:test/com.example.post/a', 'cid1', 'did:plc:test', 'com.example.post', '{}'::jsonb),
			('at://did:plc:test/com.example.post/b', 'cid2', 'did:plc:test', 'com.example.post', '{}'::jsonb)`); err != nil {
		t.Fatalf("insert records: %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, `CREATE UNIQUE INDEX CONCURRENTLY idx_record_collection_created_at ON record (collection)`); err == nil {
		t.Fatal("unique concurrent build unexpectedly succeeded; cannot create an invalid index")
	}

	err := migrations.Run(ctx, exec)
	if err == nil || !strings.Contains(err.Error(), "idx_record_collection_created_at exists but is invalid") ||
		!strings.Contains(err.Error(), "DROP INDEX CONCURRENTLY idx_record_collection_created_at;") {
		t.Fatalf("Run() error = %v, want invalid index error with recovery command", err)
	}
	if postgresMigrationApplied(ctx, t, exec, "018") {
		t.Fatal("migration 018 recorded as applied despite an invalid index")
	}

	if _, err := exec.DB().ExecContext(ctx, `DROP INDEX CONCURRENTLY idx_record_collection_created_at`); err != nil {
		t.Fatalf("drop invalid index: %v", err)
	}
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() after dropping the invalid index error = %v", err)
	}
	assertPostgresIndexValid(ctx, t, exec, "idx_record_collection_created_at")
}

// An index an operator built out of band, the way 017's notes describe, is
// kept instead of rebuilt, so large deployments can pre-build it.
func TestMigrations_018KeepsPrebuiltIndexPostgres(t *testing.T) {
	exec, _ := newIsolatedPostgresExecutor(t)
	ctx := context.Background()
	if err := migrations.RunThrough(ctx, exec, "017"); err != nil {
		t.Fatalf("RunThrough(017) error = %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, `CREATE INDEX CONCURRENTLY idx_record_collection_created_at
		ON record (collection, record_created_at DESC NULLS LAST, uri DESC)`); err != nil {
		t.Fatalf("prebuild index: %v", err)
	}
	oid := func() (id uint32) {
		t.Helper()
		if err := exec.DB().QueryRowContext(ctx, `SELECT to_regclass('idx_record_collection_created_at')::oid`).Scan(&id); err != nil {
			t.Fatalf("read index oid: %v", err)
		}
		return id
	}
	prebuilt := oid()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() over a prebuilt index error = %v", err)
	}
	if kept := oid(); kept != prebuilt {
		t.Fatalf("migration 018 rebuilt the prebuilt index (oid %d -> %d)", prebuilt, kept)
	}
}
