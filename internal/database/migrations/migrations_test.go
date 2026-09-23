package migrations_test

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
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

// newTestExecutor creates an in-memory SQLite executor for testing.
func newTestExecutor(t *testing.T) *sqlite.Executor {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create SQLite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	return exec
}

func TestMigrations_Run(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Verify key tables exist by querying sqlite_master.
	expectedTables := []string{
		"record",
		"actor",
		"config",
		"lexicon",
		"indexing_activity",
		"label",
		"report",
		"label_definition",
		"actor_label_preference",
		"label_subscription_state",
		"external_label",
	}

	for _, table := range expectedTables {
		var name string
		err := exec.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist, but got error: %v", table, err)
		}
	}

	var oldActivityTableCount int
	if err := exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='jetstream_activity'",
	).Scan(&oldActivityTableCount); err != nil {
		t.Fatalf("failed to check old activity table name: %v", err)
	}
	if oldActivityTableCount != 0 {
		t.Errorf("old activity table count = %d, want 0 after indexing_activity rename", oldActivityTableCount)
	}

	expectedIndexes := []string{
		"idx_indexing_activity_timestamp",
		"idx_indexing_activity_rkey",
		"idx_external_label_active_lookup",
		"idx_record_timeline_author_collection_created",
		"idx_record_timeline_collection_created",
		"idx_record_collection_validation",
		"idx_record_collection_lexicon_hash",
		"idx_record_collection_uri",
	}

	for _, index := range expectedIndexes {
		var name string
		err := exec.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", index,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected index %q to exist, but got error: %v", index, err)
		}
	}

	assertSQLiteRecordValidationColumns(ctx, t, exec)
	assertSQLiteLexiconRawJSONColumn(ctx, t, exec)
	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO lexicon (id, json) VALUES (?, ?)`, "com.example.oldwriter", `{}`); err == nil {
		t.Fatal("SQLite old-writer Lexicon insert without raw_json succeeded, want NOT NULL failure")
	}

	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO record (uri, cid, did, collection, json) VALUES (?, ?, ?, ?, ?)`,
		"at://did:plc:test/com.example.record/default", "cid", "did:plc:test", "com.example.record", `{"name":"default"}`); err != nil {
		t.Fatalf("failed to insert record for validation metadata default check: %v", err)
	}
	var defaultStatus string
	var validationError, validatedAt, lexiconHash sql.NullString
	if err := exec.DB().QueryRowContext(ctx, `SELECT validation_status, validation_error, validated_at, lexicon_hash FROM record WHERE uri = ?`,
		"at://did:plc:test/com.example.record/default").Scan(&defaultStatus, &validationError, &validatedAt, &lexiconHash); err != nil {
		t.Fatalf("failed to query validation metadata defaults: %v", err)
	}
	if defaultStatus != "unknown_schema" {
		t.Fatalf("validation_status default = %q, want unknown_schema", defaultStatus)
	}
	if validationError.Valid || validatedAt.Valid || lexiconHash.Valid {
		t.Fatalf("validation metadata nullable defaults = error:%v validatedAt:%v hash:%v, want all null", validationError.Valid, validatedAt.Valid, lexiconHash.Valid)
	}

	var removalMigrationApplied int
	if err := exec.DB().QueryRowContext(ctx,
		"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = '011')",
	).Scan(&removalMigrationApplied); err != nil {
		t.Fatalf("failed to check migration 011: %v", err)
	}
	if removalMigrationApplied != 1 {
		t.Fatal("expected migration 011 to be applied")
	}
}

func TestMigrations_BackfillsRecordCreatedAtSQLite(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	_, err := exec.DB().ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE lexicon (
			id TEXT PRIMARY KEY NOT NULL,
			json TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		);
		CREATE TABLE record (
			uri TEXT PRIMARY KEY NOT NULL,
			cid TEXT NOT NULL,
			did TEXT NOT NULL,
			collection TEXT NOT NULL,
			json TEXT NOT NULL,
			indexed_at TEXT NOT NULL DEFAULT (datetime('now')),
			rkey TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO lexicon (id, json) VALUES
			('com.example.saved', '{"lexicon": 1, "id": "com.example.saved", "defs": {}}');
		INSERT INTO record (uri, cid, did, collection, json, rkey) VALUES
			('at://did:plc:test/com.example.timeline.post/parseable', 'cid1', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T10:00:00.123+02:00"}', 'parseable'),
			('at://did:plc:test/com.example.timeline.post/nanos', 'cid5', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T10:00:00.123999999Z"}', 'nanos'),
			('at://did:plc:test/com.example.timeline.post/missing', 'cid2', 'did:plc:test', 'com.example.timeline.post', '{"text":"missing"}', 'missing'),
			('at://did:plc:test/com.example.timeline.post/alternate', 'cid3', 'did:plc:test', 'com.example.timeline.post', '{"timestamp":"2026-01-15T10:00:00Z"}', 'alternate'),
			('at://did:plc:test/com.example.timeline.post/malformed', 'cid4', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"not-a-time"}', 'malformed'),
			('at://did:plc:test/com.example.timeline.post/out-of-range', 'cid6', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T24:00:00Z"}', 'out-of-range'),
			('at://did:plc:test/com.example.timeline.post/invalid-json', 'cid7', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":', 'invalid-json');
	`)
	if err != nil {
		t.Fatalf("failed to set up pre-010 schema: %v", err)
	}
	for i := 1; i <= 9; i++ {
		version := fmt.Sprintf("%03d", i)
		if _, err := exec.DB().ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
			t.Fatalf("failed to mark migration %s applied: %v", version, err)
		}
	}

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	var storedLexiconJSON, rawLexiconJSON string
	if err := exec.DB().QueryRowContext(ctx, "SELECT json, raw_json FROM lexicon WHERE id = ?", "com.example.saved").Scan(&storedLexiconJSON, &rawLexiconJSON); err != nil {
		t.Fatalf("query migrated Lexicon: %v", err)
	}
	if rawLexiconJSON != storedLexiconJSON {
		t.Fatalf("migrated Lexicon raw_json = %q, want original json %q", rawLexiconJSON, storedLexiconJSON)
	}

	got := sqliteRecordCreatedAt(t, exec, "at://did:plc:test/com.example.timeline.post/parseable")
	if got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("parseable record_created_at = %q, want normalized UTC timestamp", got)
	}
	got = sqliteRecordCreatedAt(t, exec, "at://did:plc:test/com.example.timeline.post/nanos")
	if got != "2026-01-15T10:00:00.123Z" {
		t.Fatalf("nanosecond record_created_at = %q, want truncated millisecond timestamp", got)
	}
	for _, uri := range []string{
		"at://did:plc:test/com.example.timeline.post/missing",
		"at://did:plc:test/com.example.timeline.post/alternate",
		"at://did:plc:test/com.example.timeline.post/malformed",
		"at://did:plc:test/com.example.timeline.post/out-of-range",
		"at://did:plc:test/com.example.timeline.post/invalid-json",
	} {
		if got := sqliteRecordCreatedAt(t, exec, uri); got != "" {
			t.Fatalf("%s record_created_at = %q, want null", uri, got)
		}
	}
}

func sqliteRecordCreatedAt(t *testing.T, exec *sqlite.Executor, uri string) string {
	t.Helper()
	var value sql.NullString
	if err := exec.DB().QueryRowContext(context.Background(), "SELECT record_created_at FROM record WHERE uri = ?", uri).Scan(&value); err != nil {
		t.Fatalf("failed to query record_created_at for %s: %v", uri, err)
	}
	if !value.Valid {
		return ""
	}
	return value.String
}

func TestMigrations_RunPostgresRenamesIndexingActivity(t *testing.T) {
	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL migration rename test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	ctx := context.Background()
	adminExec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres admin executor: %v", err)
	}
	t.Cleanup(func() { _ = adminExec.Close() })

	schemaName := fmt.Sprintf("hyperindex_migration_test_%d", time.Now().UnixNano())
	quotedSchemaName := quotePostgresIdentifier(schemaName)
	if _, err := adminExec.DB().ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchemaName)); err != nil {
		t.Fatalf("failed to create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminExec.DB().ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchemaName))
	})

	schemaURL, err := postgresURLWithSearchPath(databaseURL, schemaName)
	if err != nil {
		t.Fatalf("failed to build postgres schema URL: %v", err)
	}

	exec, err := postgres.NewExecutor(schemaURL)
	if err != nil {
		t.Fatalf("failed to create postgres schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	assertPostgresTableCount(ctx, t, exec, schemaName, "indexing_activity", 1)
	assertPostgresTableCount(ctx, t, exec, schemaName, "jetstream_activity", 0)
	assertPostgresIndexExists(ctx, t, exec, schemaName, "indexing_activity_pkey")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_indexing_activity_timestamp")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_indexing_activity_rkey")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_collection_validation")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_collection_lexicon_hash")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_collection_uri")
	assertPostgresSequenceExists(ctx, t, exec, schemaName, "indexing_activity_id_seq")

	assertPostgresRecordValidationColumns(ctx, t, exec, schemaName)
	assertPostgresLexiconRawJSONColumn(ctx, t, exec, schemaName)
	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO lexicon (id, json) VALUES ($1, $2::jsonb)`, "com.example.oldwriter", `{}`); err == nil {
		t.Fatal("PostgreSQL old-writer Lexicon insert without raw_json succeeded, want NOT NULL failure")
	}
	const formattedLexicon = "{\n  \"lexicon\": 1,\n  \"id\": \"com.example.formatted\",\n  \"defs\": {}\n}\n"
	lexiconsRepo := repositories.NewLexiconsRepository(exec)
	if err := lexiconsRepo.Upsert(ctx, "com.example.formatted", formattedLexicon); err != nil {
		t.Fatalf("failed to save formatted postgres Lexicon: %v", err)
	}
	savedLexicon, err := lexiconsRepo.GetByID(ctx, "com.example.formatted")
	if err != nil {
		t.Fatalf("failed to read formatted postgres Lexicon: %v", err)
	}
	if savedLexicon.JSON != formattedLexicon {
		t.Fatalf("postgres raw Lexicon JSON = %q, want exact bytes %q", savedLexicon.JSON, formattedLexicon)
	}
	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO record (uri, cid, did, collection, json) VALUES ($1, $2, $3, $4, $5::jsonb)`,
		"at://did:plc:test/com.example.record/default", "cid", "did:plc:test", "com.example.record", `{"name":"default"}`); err != nil {
		t.Fatalf("failed to insert postgres record for validation metadata default check: %v", err)
	}
	var defaultStatus string
	var validationError, validatedAt, lexiconHash sql.NullString
	if err := exec.DB().QueryRowContext(ctx, `SELECT validation_status, validation_error, validated_at::text, lexicon_hash FROM record WHERE uri = $1`,
		"at://did:plc:test/com.example.record/default").Scan(&defaultStatus, &validationError, &validatedAt, &lexiconHash); err != nil {
		t.Fatalf("failed to query postgres validation metadata defaults: %v", err)
	}
	if defaultStatus != "unknown_schema" {
		t.Fatalf("postgres validation_status default = %q, want unknown_schema", defaultStatus)
	}
	if validationError.Valid || validatedAt.Valid || lexiconHash.Valid {
		t.Fatalf("postgres validation metadata nullable defaults = error:%v validatedAt:%v hash:%v, want all null", validationError.Valid, validatedAt.Valid, lexiconHash.Valid)
	}
	assertPostgresIndexNotExists(ctx, t, exec, schemaName, "idx_record_json_gin")

	for _, version := range []string{"016", "015", "014", "013", "012", "011"} {
		if err := migrations.Rollback(ctx, exec); err != nil {
			t.Fatalf("Rollback(%s) returned error: %v", version, err)
		}
	}
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_json_gin")
}

func TestMigrations_BackfillsRecordCreatedAtPostgres(t *testing.T) {
	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL record_created_at backfill test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	ctx := context.Background()
	adminExec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres admin executor: %v", err)
	}
	t.Cleanup(func() { _ = adminExec.Close() })

	schemaName := fmt.Sprintf("hyperindex_record_created_at_test_%d", time.Now().UnixNano())
	quotedSchemaName := quotePostgresIdentifier(schemaName)
	if _, err := adminExec.DB().ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchemaName)); err != nil {
		t.Fatalf("failed to create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminExec.DB().ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchemaName))
	})

	schemaURL, err := postgresURLWithSearchPath(databaseURL, schemaName)
	if err != nil {
		t.Fatalf("failed to build postgres schema URL: %v", err)
	}

	exec, err := postgres.NewExecutor(schemaURL)
	if err != nil {
		t.Fatalf("failed to create postgres schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	_, err = exec.DB().ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE TABLE lexicon (
			id TEXT PRIMARY KEY NOT NULL,
			json JSONB NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		);
		CREATE TABLE record (
			uri TEXT PRIMARY KEY NOT NULL,
			cid TEXT NOT NULL,
			did TEXT NOT NULL,
			collection TEXT NOT NULL,
			json JSONB NOT NULL,
			indexed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
			rkey TEXT NOT NULL DEFAULT ''
		);
		INSERT INTO lexicon (id, json) VALUES
			('com.example.saved', '{"lexicon": 1, "id": "com.example.saved", "defs": {}}'::jsonb);
		INSERT INTO record (uri, cid, did, collection, json, rkey) VALUES
			('at://did:plc:test/com.example.timeline.post/parseable', 'cid1', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T10:00:00.123+02:00"}'::jsonb, 'parseable'),
			('at://did:plc:test/com.example.timeline.post/nanos', 'cid5', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T10:00:00.123999999Z"}'::jsonb, 'nanos'),
			('at://did:plc:test/com.example.timeline.post/malformed', 'cid4', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"not-a-time"}'::jsonb, 'malformed'),
			('at://did:plc:test/com.example.timeline.post/out-of-range', 'cid6', 'did:plc:test', 'com.example.timeline.post', '{"createdAt":"2026-01-15T24:00:00Z"}'::jsonb, 'out-of-range');
	`)
	if err != nil {
		t.Fatalf("failed to set up pre-010 postgres schema: %v", err)
	}
	for i := 1; i <= 9; i++ {
		version := fmt.Sprintf("%03d", i)
		if _, err := exec.DB().ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			t.Fatalf("failed to mark migration %s applied: %v", version, err)
		}
	}

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	var storedLexiconJSON, rawLexiconJSON string
	if err := exec.DB().QueryRowContext(ctx, "SELECT json::text, raw_json FROM lexicon WHERE id = $1", "com.example.saved").Scan(&storedLexiconJSON, &rawLexiconJSON); err != nil {
		t.Fatalf("query migrated postgres Lexicon: %v", err)
	}
	if rawLexiconJSON != storedLexiconJSON {
		t.Fatalf("migrated postgres Lexicon raw_json = %q, want normalized json::text %q", rawLexiconJSON, storedLexiconJSON)
	}

	got := postgresRecordCreatedAt(t, exec, "at://did:plc:test/com.example.timeline.post/parseable")
	if got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("parseable record_created_at = %q, want normalized UTC timestamp", got)
	}
	got = postgresRecordCreatedAt(t, exec, "at://did:plc:test/com.example.timeline.post/nanos")
	if got != "2026-01-15T10:00:00.123Z" {
		t.Fatalf("nanosecond record_created_at = %q, want truncated millisecond timestamp", got)
	}
	for _, uri := range []string{
		"at://did:plc:test/com.example.timeline.post/malformed",
		"at://did:plc:test/com.example.timeline.post/out-of-range",
	} {
		if got := postgresRecordCreatedAt(t, exec, uri); got != "" {
			t.Fatalf("%s record_created_at = %q, want null", uri, got)
		}
	}
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_timeline_author_collection_created")
	assertPostgresIndexExists(ctx, t, exec, schemaName, "idx_record_timeline_collection_created")
}

func postgresRecordCreatedAt(t *testing.T, exec *postgres.Executor, uri string) string {
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

func TestMigrations_RunIdempotent(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("first Run() returned error: %v", err)
	}

	// Running a second time should be a no-op (all migrations already applied).
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("second Run() returned error: %v", err)
	}

	// Verify tables still present after second run.
	var count int
	err := exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='record'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record table, got %d", count)
	}
}

func TestMigrations_Rollback(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	// Apply all migrations first.
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Count applied migrations before rollback.
	var countBefore int
	err := exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&countBefore)
	if err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}

	if countBefore == 0 {
		t.Fatal("expected at least one applied migration before rollback")
	}

	// Rollback the last migration.
	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback() returned error: %v", err)
	}

	// Verify one fewer migration is recorded.
	var countAfter int
	err = exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&countAfter)
	if err != nil {
		t.Fatalf("failed to count migrations after rollback: %v", err)
	}

	if countAfter != countBefore-1 {
		t.Errorf("expected %d migrations after rollback, got %d", countBefore-1, countAfter)
	}
}

// rollbackRecordVersionMigration undoes 016 and 015 so tests written against
// 014 as the newest migration keep exercising 014 and 013.
func rollbackRecordVersionMigration(ctx context.Context, t *testing.T, exec *sqlite.Executor) {
	t.Helper()
	for _, version := range []string{"016", "015"} {
		if err := migrations.Rollback(ctx, exec); err != nil {
			t.Fatalf("Rollback(%s) error = %v", version, err)
		}
	}
}

func TestMigrations_RecordVersionUpAndDown(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !sqliteIndexExists(t, exec, "idx_record_version_uri_key") || !migrationVersionExists(t, exec, "015") {
		t.Fatal("migration 015 did not create record_version")
	}
	if !sqliteIndexExists(t, exec, "idx_record_version_did") || !migrationVersionExists(t, exec, "016") {
		t.Fatal("migration 016 did not create the record_version did index")
	}
	rollbackRecordVersionMigration(ctx, t, exec)
	if sqliteIndexExists(t, exec, "idx_record_version_uri_key") || migrationVersionExists(t, exec, "015") {
		t.Fatal("migration 015 schema/version remained after rollback")
	}
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("re-Run() error = %v", err)
	}
	if !migrationVersionExists(t, exec, "015") {
		t.Fatal("migration 015 was not re-applied")
	}
}

func TestMigrations_Rollback014Then013PreservesLexiconData(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rollbackRecordVersionMigration(ctx, t, exec)
	const raw = `{"lexicon":1,"id":"com.example.saved","defs":{}}`
	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO lexicon (id, json, raw_json) VALUES (?, ?, ?)`, "com.example.saved", raw, raw); err != nil {
		t.Fatalf("insert Lexicon before downgrade: %v", err)
	}

	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback(014) error = %v", err)
	}
	if sqliteIndexExists(t, exec, "idx_record_collection_uri") || migrationVersionExists(t, exec, "014") {
		t.Fatal("migration 014 schema/version remained after rollback")
	}
	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback(013) error = %v", err)
	}
	if sqliteColumnExists(t, exec, "lexicon", "raw_json") || migrationVersionExists(t, exec, "013") {
		t.Fatal("migration 013 schema/version remained after rollback")
	}
	var saved string
	if err := exec.DB().QueryRowContext(ctx, `SELECT json FROM lexicon WHERE id = ?`, "com.example.saved").Scan(&saved); err != nil {
		t.Fatalf("query downgraded Lexicon: %v", err)
	}
	if saved != raw {
		t.Fatalf("downgraded Lexicon json = %q, want %q", saved, raw)
	}
}

func TestMigrations_ApplyAndVersionRecordRollbackTogether(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rollbackRecordVersionMigration(ctx, t, exec)
	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback(014) error = %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, `
		CREATE TRIGGER fail_migration_014_record
		BEFORE INSERT ON schema_migrations
		WHEN NEW.version = '014'
		BEGIN SELECT RAISE(ABORT, 'forced version record failure'); END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := migrations.Run(ctx, exec)
	if err == nil || !strings.Contains(err.Error(), "forced version record failure") {
		t.Fatalf("Run() error = %v, want forced version record failure", err)
	}
	if sqliteIndexExists(t, exec, "idx_record_collection_uri") {
		t.Fatal("migration 014 index survived failed atomic apply")
	}
	if migrationVersionExists(t, exec, "014") {
		t.Fatal("migration 014 version was recorded after failed atomic apply")
	}
}

func TestMigrations_DownAndVersionDeleteRollbackTogether(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rollbackRecordVersionMigration(ctx, t, exec)
	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback(014) error = %v", err)
	}
	const raw = `{"lexicon":1,"id":"com.example.rollback","defs":{}}`
	if _, err := exec.DB().ExecContext(ctx, `INSERT INTO lexicon (id, json, raw_json) VALUES (?, ?, ?)`, "com.example.rollback", raw, raw); err != nil {
		t.Fatalf("insert Lexicon before failed rollback: %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, `
		CREATE TRIGGER fail_migration_013_delete
		BEFORE DELETE ON schema_migrations
		WHEN OLD.version = '013'
		BEGIN SELECT RAISE(ABORT, 'forced version delete failure'); END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := migrations.Rollback(ctx, exec)
	if err == nil || !strings.Contains(err.Error(), "forced version delete failure") {
		t.Fatalf("Rollback(013) error = %v, want forced version delete failure", err)
	}
	if !sqliteColumnExists(t, exec, "lexicon", "raw_json") {
		t.Fatal("raw_json column was not restored after failed atomic rollback")
	}
	if !migrationVersionExists(t, exec, "013") {
		t.Fatal("migration 013 version was removed after failed atomic rollback")
	}
	var saved string
	if err := exec.DB().QueryRowContext(ctx, `SELECT raw_json FROM lexicon WHERE id = ?`, "com.example.rollback").Scan(&saved); err != nil {
		t.Fatalf("query Lexicon after failed rollback: %v", err)
	}
	if saved != raw {
		t.Fatalf("raw_json after failed rollback = %q, want %q", saved, raw)
	}
}

func sqliteIndexExists(t *testing.T, exec *sqlite.Executor, name string) bool {
	t.Helper()
	var count int
	if err := exec.DB().QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count); err != nil {
		t.Fatalf("check SQLite index %s: %v", name, err)
	}
	return count == 1
}

func sqliteColumnExists(t *testing.T, exec *sqlite.Executor, table, column string) bool {
	t.Helper()
	rows, err := exec.DB().Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("inspect SQLite table %s: %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan SQLite table %s: %v", table, err)
		}
		if name == column {
			return true
		}
	}
	return false
}

func migrationVersionExists(t *testing.T, exec *sqlite.Executor, version string) bool {
	t.Helper()
	var count int
	if err := exec.DB().QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
		t.Fatalf("check migration version %s: %v", version, err)
	}
	return count == 1
}

func assertSQLiteRecordValidationColumns(ctx context.Context, t *testing.T, exec *sqlite.Executor) {
	t.Helper()

	rows, err := exec.DB().QueryContext(ctx, "PRAGMA table_info(record)")
	if err != nil {
		t.Fatalf("failed to inspect sqlite record columns: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]struct {
		columnType string
		notNull    int
	})
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue sql.NullString
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("failed to scan sqlite column metadata: %v", err)
		}
		columns[name] = struct {
			columnType string
			notNull    int
		}{columnType: columnType, notNull: notNull}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate sqlite column metadata: %v", err)
	}

	for _, column := range []string{"validation_status", "validation_error", "validated_at", "lexicon_hash"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("sqlite record column %q missing", column)
		}
	}
	if columns["validation_status"].notNull != 1 {
		t.Fatalf("sqlite validation_status is nullable, want NOT NULL")
	}
}

func assertPostgresRecordValidationColumns(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName string) {
	t.Helper()

	rows, err := exec.DB().QueryContext(ctx, `
		SELECT column_name, is_nullable
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'record'
		  AND column_name IN ('validation_status', 'validation_error', 'validated_at', 'lexicon_hash')`, schemaName)
	if err != nil {
		t.Fatalf("failed to inspect postgres record validation columns: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var name, nullable string
		if err := rows.Scan(&name, &nullable); err != nil {
			t.Fatalf("failed to scan postgres column metadata: %v", err)
		}
		columns[name] = nullable
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate postgres column metadata: %v", err)
	}

	for _, column := range []string{"validation_status", "validation_error", "validated_at", "lexicon_hash"} {
		if _, ok := columns[column]; !ok {
			t.Fatalf("postgres record column %q missing", column)
		}
	}
	if columns["validation_status"] != "NO" {
		t.Fatalf("postgres validation_status is nullable, want NOT NULL")
	}
}

func assertSQLiteLexiconRawJSONColumn(ctx context.Context, t *testing.T, exec *sqlite.Executor) {
	t.Helper()

	rows, err := exec.DB().QueryContext(ctx, "PRAGMA table_info(lexicon)")
	if err != nil {
		t.Fatalf("failed to inspect sqlite lexicon columns: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("failed to scan sqlite lexicon column metadata: %v", err)
		}
		if name == "raw_json" {
			if columnType != "TEXT" || notNull != 1 {
				t.Fatalf("sqlite lexicon.raw_json type/not-null = %s/%d, want TEXT/1", columnType, notNull)
			}
			if defaultValue.Valid {
				t.Fatalf("sqlite lexicon.raw_json default = %q, want no default", defaultValue.String)
			}
			return
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("failed to iterate sqlite lexicon column metadata: %v", err)
	}
	t.Fatal("sqlite lexicon.raw_json column missing")
}

func assertPostgresLexiconRawJSONColumn(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName string) {
	t.Helper()

	var dataType, nullable string
	var columnDefault sql.NullString
	if err := exec.DB().QueryRowContext(ctx, `
		SELECT data_type, is_nullable, column_default
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'lexicon' AND column_name = 'raw_json'`, schemaName,
	).Scan(&dataType, &nullable, &columnDefault); err != nil {
		t.Fatalf("failed to inspect postgres lexicon.raw_json: %v", err)
	}
	if dataType != "text" || nullable != "NO" {
		t.Fatalf("postgres lexicon.raw_json type/nullable = %s/%s, want text/NO", dataType, nullable)
	}
	if columnDefault.Valid {
		t.Fatalf("postgres lexicon.raw_json default = %q, want no default", columnDefault.String)
	}
}

func safePostgresTestDatabaseURL(t *testing.T) (string, bool) {
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
	if !isSafePostgresTestDatabaseName(databaseName) {
		return "", false
	}

	return databaseURL, true
}

func isSafePostgresTestDatabaseName(databaseName string) bool {
	name := strings.ToLower(strings.TrimSpace(databaseName))
	return name == "test" || strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "-test")
}

func postgresURLWithSearchPath(databaseURL, schemaName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}

	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func quotePostgresIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func assertPostgresTableCount(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName, tableName string, want int) {
	t.Helper()

	var count int
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = $1 AND table_name = $2`,
		schemaName,
		tableName,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count postgres table %q: %v", tableName, err)
	}
	if count != want {
		t.Errorf("postgres table %q count = %d, want %d", tableName, count, want)
	}
}

func assertPostgresIndexExists(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName, indexName string) {
	t.Helper()

	var count int
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = $1 AND indexname = $2`,
		schemaName,
		indexName,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count postgres index %q: %v", indexName, err)
	}
	if count != 1 {
		t.Errorf("postgres index %q count = %d, want 1", indexName, count)
	}
}

func assertPostgresIndexNotExists(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName, indexName string) {
	t.Helper()

	var count int
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE schemaname = $1 AND indexname = $2`,
		schemaName,
		indexName,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count postgres index %q: %v", indexName, err)
	}
	if count != 0 {
		t.Errorf("postgres index %q count = %d, want 0", indexName, count)
	}
}

func assertPostgresSequenceExists(ctx context.Context, t *testing.T, exec *postgres.Executor, schemaName, sequenceName string) {
	t.Helper()

	var count int
	if err := exec.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.sequences WHERE sequence_schema = $1 AND sequence_name = $2`,
		schemaName,
		sequenceName,
	).Scan(&count); err != nil {
		t.Fatalf("failed to count postgres sequence %q: %v", sequenceName, err)
	}
	if count != 1 {
		t.Errorf("postgres sequence %q count = %d, want 1", sequenceName, count)
	}
}
