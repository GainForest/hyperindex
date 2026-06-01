package repositories

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

func TestRecordsRepository_PresenceFilterSemanticsSQLite(t *testing.T) {
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("failed to run sqlite migrations: %v", err)
	}

	runPresenceFilterSemantics(t, exec)
}

func TestRecordsRepository_PresenceFilterSemanticsPostgres(t *testing.T) {
	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL presence semantics test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	exec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("failed to run postgres migrations: %v", err)
	}

	runPresenceFilterSemantics(t, exec)
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

func TestSafePostgresTestDatabaseURL(t *testing.T) {
	tests := []struct {
		name        string
		databaseURL string
		wantOK      bool
	}{
		{name: "postgres exact test database", databaseURL: "postgres://user:pass@localhost:5432/test?sslmode=disable", wantOK: true},
		{name: "postgresql underscore test suffix", databaseURL: "postgresql://user:pass@localhost:5432/hyperindex_test?sslmode=disable", wantOK: true},
		{name: "postgres dash test suffix", databaseURL: "postgres://user:pass@localhost:5432/hyperindex-test?sslmode=disable", wantOK: true},
		{name: "database name containing test is not enough", databaseURL: "postgres://user:pass@localhost:5432/contest?sslmode=disable"},
		{name: "database name ending with latest is not enough", databaseURL: "postgres://user:pass@localhost:5432/latest?sslmode=disable"},
		{name: "database name with test prefix is not enough", databaseURL: "postgres://user:pass@localhost:5432/testdata?sslmode=disable"},
		{name: "non postgres URL is ignored", databaseURL: "sqlite::memory:"},
		{name: "empty URL is ignored"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)

			gotURL, gotOK := safePostgresTestDatabaseURL(t)
			if gotOK != tt.wantOK {
				t.Fatalf("safePostgresTestDatabaseURL() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotOK && gotURL != tt.databaseURL {
				t.Fatalf("safePostgresTestDatabaseURL() URL = %q, want %q", gotURL, tt.databaseURL)
			}
		})
	}
}

func runPresenceFilterSemantics(t *testing.T, exec database.Executor) {
	t.Helper()

	ctx := context.Background()
	repo := NewRecordsRepository(exec)
	collection := fmt.Sprintf("com.example.presence.%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = exec.Exec(ctx,
			fmt.Sprintf("DELETE FROM record WHERE collection = %s", exec.Placeholder(1)),
			[]database.Value{database.Text(collection)},
		)
	})

	records := []*Record{
		presenceRecord(collection, "missing", `{}`),
		presenceRecord(collection, "json-null", `{"image":null,"contributors":null}`),
		presenceRecord(collection, "object-array", `{"image":{"type":"photo"},"contributors":["did:plc:alice"]}`),
		presenceRecord(collection, "empty-object-array", `{"image":{},"contributors":[]}`),
		presenceRecord(collection, "empty-string", `{"image":"","contributors":""}`),
	}
	if err := repo.BatchInsert(ctx, records); err != nil {
		t.Fatalf("failed to insert presence test records: %v", err)
	}

	assertPresenceFilter(t, repo, collection,
		FieldFilter{Field: "image", Operator: "isNull", Value: true, FieldType: "object"},
		[]string{"missing", "json-null"},
	)
	assertPresenceFilter(t, repo, collection,
		FieldFilter{Field: "image", Operator: "isNull", Value: false, FieldType: "object"},
		[]string{"object-array", "empty-object-array", "empty-string"},
	)
	assertPresenceFilter(t, repo, collection,
		FieldFilter{Field: "contributors", Operator: "isNull", Value: true, FieldType: "array"},
		[]string{"missing", "json-null"},
	)
	assertPresenceFilter(t, repo, collection,
		FieldFilter{Field: "contributors", Operator: "isNull", Value: false, FieldType: "array"},
		[]string{"object-array", "empty-object-array", "empty-string"},
	)
}

func presenceRecord(collection, rkey, jsonData string) *Record {
	return &Record{
		URI:        fmt.Sprintf("at://did:plc:presence/%s/%s", collection, rkey),
		CID:        "cid-" + rkey,
		DID:        "did:plc:presence",
		Collection: collection,
		JSON:       jsonData,
		RKey:       rkey,
	}
}

func assertPresenceFilter(t *testing.T, repo *RecordsRepository, collection string, filter FieldFilter, wantRKeys []string) {
	t.Helper()

	ctx := context.Background()
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, collection, []FieldFilter{filter}, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor(%+v) failed: %v", filter, err)
	}

	gotRKeys := make([]string, 0, len(records))
	for _, record := range records {
		gotRKeys = append(gotRKeys, record.RKey)
	}
	sort.Strings(gotRKeys)
	want := append([]string(nil), wantRKeys...)
	sort.Strings(want)
	if strings.Join(gotRKeys, ",") != strings.Join(want, ",") {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor(%+v) rkeys = %v, want %v", filter, gotRKeys, want)
	}

	count, err := repo.GetCollectionCountFiltered(ctx, collection, []FieldFilter{filter}, DIDFilter{})
	if err != nil {
		t.Fatalf("GetCollectionCountFiltered(%+v) failed: %v", filter, err)
	}
	if count != int64(len(wantRKeys)) {
		t.Fatalf("GetCollectionCountFiltered(%+v) = %d, want %d", filter, count, len(wantRKeys))
	}
}
