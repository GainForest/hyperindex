package graphql

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	graphqlgo "github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestPublicSchemaManagerInitialLoadFromDBLexicons(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	dir := t.TempDir()

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	manager := newTestPublicSchemaManager(db, dir)

	if manager.Schema() != nil {
		t.Fatal("expected no active schema before first reload")
	}
	if manager.LexiconCount() != 0 {
		t.Fatalf("expected zero lexicons before first reload, got %d", manager.LexiconCount())
	}

	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("reload failed: %s", result.Error)
	}
	if result.LexiconCount != 1 {
		t.Fatalf("expected result lexicon count 1, got %d", result.LexiconCount)
	}
	if result.ReloadedAt == nil {
		t.Fatal("expected successful reload to include reloadedAt")
	}

	snapshot := manager.Snapshot()
	if snapshot.Schema == nil {
		t.Fatal("expected active schema after reload")
	}
	if snapshot.SourceCounts.Filesystem != 0 || snapshot.SourceCounts.Database != 1 || snapshot.SourceCounts.Registered != 1 {
		t.Fatalf("unexpected source counts: %+v", snapshot.SourceCounts)
	}
	assertQueryField(t, snapshot.Schema, lexicon.ToFieldName("app.test.alpha"))
}

func TestPublicSchemaManagerInitialLoadFromFilesystemLexicons(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	dir := t.TempDir()
	manager := newTestPublicSchemaManager(db, dir)

	path := filepath.Join(dir, "app.test.filesystem.json")
	if err := os.WriteFile(path, []byte(testLexiconJSON("app.test.filesystem", "body")), 0o644); err != nil {
		t.Fatalf("failed to write filesystem lexicon: %v", err)
	}

	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("reload failed: %s", result.Error)
	}
	if result.LexiconCount != 1 {
		t.Fatalf("expected result lexicon count 1, got %d", result.LexiconCount)
	}

	snapshot := manager.Snapshot()
	if snapshot.SourceCounts.Filesystem != 1 || snapshot.SourceCounts.Database != 0 || snapshot.SourceCounts.Registered != 1 {
		t.Fatalf("unexpected source counts: %+v", snapshot.SourceCounts)
	}
	assertQueryField(t, snapshot.Schema, lexicon.ToFieldName("app.test.filesystem"))
}

func TestPublicSchemaManagerReloadRequiredForNewDBLexicon(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	manager := newTestPublicSchemaManager(db, t.TempDir())

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("initial reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("initial reload failed: %s", result.Error)
	}

	activeBefore := manager.Schema()
	assertQueryField(t, activeBefore, lexicon.ToFieldName("app.test.alpha"))

	upsertLexicon(ctx, t, db, "app.test.beta", "summary")
	assertNoQueryField(t, manager.Schema(), lexicon.ToFieldName("app.test.beta"))

	result, err = manager.Reload(ctx)
	if err != nil {
		t.Fatalf("second reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("second reload failed: %s", result.Error)
	}
	if result.LexiconCount != 2 {
		t.Fatalf("expected result lexicon count 2, got %d", result.LexiconCount)
	}
	if manager.Schema() == activeBefore {
		t.Fatal("expected successful reload to swap in a new schema snapshot")
	}
	assertQueryField(t, manager.Schema(), lexicon.ToFieldName("app.test.beta"))
}

func TestPublicSchemaManagerMalformedFilesystemLexiconPreservesPreviousSchema(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	dir := t.TempDir()
	manager := newTestPublicSchemaManager(db, dir)

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("initial reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("initial reload failed: %s", result.Error)
	}
	activeBefore := manager.Schema()

	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"lexicon":1,`), 0o644); err != nil {
		t.Fatalf("failed to write malformed filesystem lexicon: %v", err)
	}

	result, err = manager.Reload(ctx)
	if err != nil {
		t.Fatalf("failed reload returned unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected reload to fail for malformed filesystem lexicon")
	}
	if result.LexiconCount != 1 {
		t.Fatalf("expected failed reload to report active lexicon count 1, got %d", result.LexiconCount)
	}
	if !strings.Contains(result.Error, "filesystem lexicon") || !strings.Contains(result.Error, "bad.json") {
		t.Fatalf("expected actionable filesystem error mentioning bad.json, got %q", result.Error)
	}
	if manager.Schema() != activeBefore {
		t.Fatal("expected failed reload to keep previous schema pointer active")
	}
	assertQueryField(t, manager.Schema(), lexicon.ToFieldName("app.test.alpha"))
}

func TestPublicSchemaManagerMalformedDatabaseLexiconPreservesPreviousSchema(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	manager := newTestPublicSchemaManager(db, t.TempDir())

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("initial reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("initial reload failed: %s", result.Error)
	}
	activeBefore := manager.Schema()

	if err := db.Lexicons.Upsert(ctx, "app.test.bad", `{"lexicon":1,`); err != nil {
		t.Fatalf("failed to insert malformed database lexicon: %v", err)
	}

	result, err = manager.Reload(ctx)
	if err != nil {
		t.Fatalf("failed reload returned unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected reload to fail for malformed database lexicon")
	}
	if result.LexiconCount != 1 {
		t.Fatalf("expected failed reload to report active lexicon count 1, got %d", result.LexiconCount)
	}
	if !strings.Contains(result.Error, "database lexicon") || !strings.Contains(result.Error, "app.test.bad") {
		t.Fatalf("expected actionable database error mentioning app.test.bad, got %q", result.Error)
	}
	if manager.Schema() != activeBefore {
		t.Fatal("expected failed reload to keep previous schema pointer active")
	}
	assertQueryField(t, manager.Schema(), lexicon.ToFieldName("app.test.alpha"))
}

func TestPublicSchemaManagerDatabaseIDMismatchPreservesPreviousSchema(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	manager := newTestPublicSchemaManager(db, t.TempDir())

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("initial reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("initial reload failed: %s", result.Error)
	}
	activeBefore := manager.Schema()

	if err := db.Lexicons.Upsert(ctx, "app.test.bad", testLexiconJSON("app.test.other", "body")); err != nil {
		t.Fatalf("failed to insert mismatched database lexicon: %v", err)
	}

	result, err = manager.Reload(ctx)
	if err != nil {
		t.Fatalf("failed reload returned unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected reload to fail for mismatched database lexicon id")
	}
	if result.LexiconCount != 1 {
		t.Fatalf("expected failed reload to report active lexicon count 1, got %d", result.LexiconCount)
	}
	if !strings.Contains(result.Error, "database lexicon") || !strings.Contains(result.Error, "app.test.bad") || !strings.Contains(result.Error, "app.test.other") {
		t.Fatalf("expected actionable ID mismatch error mentioning both IDs, got %q", result.Error)
	}
	if manager.Schema() != activeBefore {
		t.Fatal("expected failed reload to keep previous schema pointer active")
	}
	assertQueryField(t, manager.Schema(), lexicon.ToFieldName("app.test.alpha"))
}

func TestPublicSchemaManagerInitialNoSchemaState(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	dir := t.TempDir()
	manager := newTestPublicSchemaManager(db, dir)

	if snapshot := manager.Snapshot(); snapshot.Schema != nil || snapshot.LexiconCount != 0 {
		t.Fatalf("expected zero-value snapshot before reload, got %+v", snapshot)
	}
	if manager.Schema() != nil {
		t.Fatal("expected nil schema before reload")
	}
	if manager.LexiconCount() != 0 {
		t.Fatalf("expected active count 0 before reload, got %d", manager.LexiconCount())
	}

	badPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(badPath, []byte(`{"lexicon":1,`), 0o644); err != nil {
		t.Fatalf("failed to write malformed filesystem lexicon: %v", err)
	}

	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("failed initial reload returned unexpected error: %v", err)
	}
	if result.Success {
		t.Fatal("expected initial reload to fail for malformed filesystem lexicon")
	}
	if result.LexiconCount != 0 {
		t.Fatalf("expected failed initial reload to report active lexicon count 0, got %d", result.LexiconCount)
	}
	if result.Error == "" {
		t.Fatal("expected failed reload to include an actionable error")
	}
	if manager.Schema() != nil {
		t.Fatal("expected no active schema after failed initial reload")
	}
}

func TestPublicSchemaManagerMissingLexiconRepositoryReturnsError(t *testing.T) {
	ctx := context.Background()
	manager := NewPublicSchemaManager(PublicSchemaManagerConfig{LexiconDir: t.TempDir()}, &resolver.Repositories{})

	result, err := manager.Reload(ctx)
	if err == nil {
		t.Fatal("expected missing lexicon repository to return an error")
	}
	if result != nil {
		t.Fatalf("expected nil result for wiring error, got %+v", result)
	}
	if !strings.Contains(err.Error(), "lexicon repository is not configured") {
		t.Fatalf("expected missing repository error, got %q", err.Error())
	}
}

func newTestPublicSchemaManager(db *testutil.TestDB, lexiconDir string) *PublicSchemaManager {
	repos := &resolver.Repositories{Lexicons: db.Lexicons}
	return NewPublicSchemaManager(PublicSchemaManagerConfig{LexiconDir: lexiconDir}, repos)
}

func upsertLexicon(ctx context.Context, t *testing.T, db *testutil.TestDB, id, fieldName string) {
	t.Helper()
	if err := db.Lexicons.Upsert(ctx, id, testLexiconJSON(id, fieldName)); err != nil {
		t.Fatalf("failed to upsert lexicon %s: %v", id, err)
	}
}

func testLexiconJSON(id, fieldName string) string {
	return fmt.Sprintf(`{
		"lexicon": 1,
		"id": %q,
		"defs": {
			"main": {
				"type": "record",
				"key": "tid",
				"record": {
					"type": "object",
					"required": [%q],
					"properties": {
						%q: {"type": "string"}
					}
				}
			}
		}
	}`, id, fieldName, fieldName)
}

func assertQueryField(t *testing.T, schema *graphqlgo.Schema, fieldName string) {
	t.Helper()
	if schema == nil {
		t.Fatalf("expected schema to contain query field %q, got nil schema", fieldName)
	}
	if schema.QueryType().Fields()[fieldName] == nil {
		t.Fatalf("expected query field %q to exist", fieldName)
	}
}

func assertNoQueryField(t *testing.T, schema *graphqlgo.Schema, fieldName string) {
	t.Helper()
	if schema == nil {
		t.Fatalf("expected schema while checking missing query field %q, got nil", fieldName)
	}
	if schema.QueryType().Fields()[fieldName] != nil {
		t.Fatalf("expected query field %q to be absent", fieldName)
	}
}
