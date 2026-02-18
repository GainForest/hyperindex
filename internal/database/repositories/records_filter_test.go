// Package repositories contains data access layer implementations.
package repositories

import (
	"context"
	"strings"
	"testing"

	"github.com/GainForest/hypergoat/internal/database"
	"github.com/GainForest/hypergoat/internal/database/sqlite"
)

// newTestRepo creates a RecordsRepository backed by an in-memory SQLite executor for unit tests.
func newTestRepo(t *testing.T) *RecordsRepository {
	t.Helper()
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create sqlite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })
	return NewRecordsRepository(exec)
}

func TestBuildFilterClause_EmptyFilters(t *testing.T) {
	repo := newTestRepo(t)
	clause, params, _ := repo.buildFilterClause(nil, 1)
	if clause != "" {
		t.Errorf("empty filters: clause = %q, want empty string", clause)
	}
	if params != nil {
		t.Errorf("empty filters: params = %v, want nil", params)
	}

	clause2, params2, _ := repo.buildFilterClause([]FieldFilter{}, 1)
	if clause2 != "" {
		t.Errorf("empty slice: clause = %q, want empty string", clause2)
	}
	if params2 != nil {
		t.Errorf("empty slice: params = %v, want nil", params2)
	}
}

func TestBuildFilterClause_Operators(t *testing.T) {
	repo := newTestRepo(t)

	tests := []struct {
		name         string
		filter       FieldFilter
		wantContains string // substring expected in the clause
		wantParams   int    // number of params expected
	}{
		{
			name:         "eq operator",
			filter:       FieldFilter{Field: "title", Operator: "eq", Value: "hello", FieldType: "string"},
			wantContains: "= ?",
			wantParams:   1,
		},
		{
			name:         "neq operator",
			filter:       FieldFilter{Field: "title", Operator: "neq", Value: "hello", FieldType: "string"},
			wantContains: "!= ?",
			wantParams:   1,
		},
		{
			name:         "gt operator",
			filter:       FieldFilter{Field: "score", Operator: "gt", Value: 5, FieldType: "integer"},
			wantContains: "> ?",
			wantParams:   1,
		},
		{
			name:         "lt operator",
			filter:       FieldFilter{Field: "score", Operator: "lt", Value: 10, FieldType: "integer"},
			wantContains: "< ?",
			wantParams:   1,
		},
		{
			name:         "gte operator",
			filter:       FieldFilter{Field: "score", Operator: "gte", Value: 5, FieldType: "number"},
			wantContains: ">= ?",
			wantParams:   1,
		},
		{
			name:         "lte operator",
			filter:       FieldFilter{Field: "score", Operator: "lte", Value: 10, FieldType: "number"},
			wantContains: "<= ?",
			wantParams:   1,
		},
		{
			name:         "contains operator wraps value in percent",
			filter:       FieldFilter{Field: "body", Operator: "contains", Value: "world", FieldType: "string"},
			wantContains: "LIKE ?",
			wantParams:   1,
		},
		{
			name:         "startsWith operator appends percent",
			filter:       FieldFilter{Field: "body", Operator: "startsWith", Value: "hello", FieldType: "string"},
			wantContains: "LIKE ?",
			wantParams:   1,
		},
		{
			name:         "isNull true generates IS NULL",
			filter:       FieldFilter{Field: "deletedAt", Operator: "isNull", Value: true, FieldType: "string"},
			wantContains: "IS NULL",
			wantParams:   0,
		},
		{
			name:         "isNull false generates IS NOT NULL",
			filter:       FieldFilter{Field: "deletedAt", Operator: "isNull", Value: false, FieldType: "string"},
			wantContains: "IS NOT NULL",
			wantParams:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, params, _ := repo.buildFilterClause([]FieldFilter{tt.filter}, 1)
			if clause == "" {
				t.Fatalf("clause is empty, want non-empty")
			}
			if !strings.Contains(clause, tt.wantContains) {
				t.Errorf("clause = %q, want to contain %q", clause, tt.wantContains)
			}
			if len(params) != tt.wantParams {
				t.Errorf("params count = %d, want %d", len(params), tt.wantParams)
			}
		})
	}
}

func TestBuildFilterClause_ContainsWrapsValue(t *testing.T) {
	repo := newTestRepo(t)
	filters := []FieldFilter{
		{Field: "body", Operator: "contains", Value: "world", FieldType: "string"},
	}
	_, params, _ := repo.buildFilterClause(filters, 1)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	tv, ok := params[0].(database.TextValue)
	if !ok {
		t.Fatalf("param is not TextValue, got %T", params[0])
	}
	if string(tv) != "%world%" {
		t.Errorf("contains param = %q, want %%world%%", string(tv))
	}
}

func TestBuildFilterClause_StartsWithAppendsPercent(t *testing.T) {
	repo := newTestRepo(t)
	filters := []FieldFilter{
		{Field: "body", Operator: "startsWith", Value: "hello", FieldType: "string"},
	}
	_, params, _ := repo.buildFilterClause(filters, 1)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	tv, ok := params[0].(database.TextValue)
	if !ok {
		t.Fatalf("param is not TextValue, got %T", params[0])
	}
	if string(tv) != "hello%" {
		t.Errorf("startsWith param = %q, want hello%%", string(tv))
	}
}

func TestBuildFilterClause_InOperator(t *testing.T) {
	repo := newTestRepo(t)
	filters := []FieldFilter{
		{Field: "status", Operator: "in", Value: []interface{}{"active", "pending", "closed"}, FieldType: "string"},
	}
	clause, params, _ := repo.buildFilterClause(filters, 1)
	if !strings.Contains(clause, "IN (") {
		t.Errorf("clause = %q, want to contain IN (", clause)
	}
	if len(params) != 3 {
		t.Errorf("params count = %d, want 3", len(params))
	}
}

func TestBuildFilterClause_NumericCast(t *testing.T) {
	repo := newTestRepo(t)

	t.Run("integer type uses CAST AS REAL in SQLite", func(t *testing.T) {
		filters := []FieldFilter{
			{Field: "score", Operator: "gt", Value: 5, FieldType: "integer"},
		}
		clause, _, _ := repo.buildFilterClause(filters, 1)
		if !strings.Contains(clause, "CAST(") {
			t.Errorf("integer filter clause = %q, want CAST(...)", clause)
		}
		if !strings.Contains(clause, "AS REAL") {
			t.Errorf("integer filter clause = %q, want AS REAL", clause)
		}
	})

	t.Run("number type uses CAST AS REAL in SQLite", func(t *testing.T) {
		filters := []FieldFilter{
			{Field: "price", Operator: "lte", Value: 99.99, FieldType: "number"},
		}
		clause, _, _ := repo.buildFilterClause(filters, 1)
		if !strings.Contains(clause, "CAST(") {
			t.Errorf("number filter clause = %q, want CAST(...)", clause)
		}
		if !strings.Contains(clause, "AS REAL") {
			t.Errorf("number filter clause = %q, want AS REAL", clause)
		}
	})

	t.Run("string type does not use CAST", func(t *testing.T) {
		filters := []FieldFilter{
			{Field: "title", Operator: "eq", Value: "hello", FieldType: "string"},
		}
		clause, _, _ := repo.buildFilterClause(filters, 1)
		if strings.Contains(clause, "CAST(") {
			t.Errorf("string filter clause = %q, should not contain CAST", clause)
		}
	})
}

func TestBuildFilterClause_MultipleFilters(t *testing.T) {
	repo := newTestRepo(t)
	filters := []FieldFilter{
		{Field: "title", Operator: "eq", Value: "hello", FieldType: "string"},
		{Field: "score", Operator: "gt", Value: 5, FieldType: "integer"},
		{Field: "deletedAt", Operator: "isNull", Value: true, FieldType: "string"},
	}
	clause, params, _ := repo.buildFilterClause(filters, 1)

	// Should be joined with AND
	parts := strings.Split(clause, " AND ")
	if len(parts) != 3 {
		t.Errorf("expected 3 AND-joined conditions, got %d in clause: %q", len(parts), clause)
	}
	// Two params: eq and gt (isNull has no param)
	if len(params) != 2 {
		t.Errorf("params count = %d, want 2", len(params))
	}
}

// newSortTestRepo creates a RecordsRepository with a fresh in-memory SQLite DB and the record table.
// Returns the repo and a helper function for running raw SQL (e.g., to set indexed_at).
func newSortTestRepo(t *testing.T) (*RecordsRepository, func(query string, args ...any)) {
	t.Helper()
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })
	rawDB := exec.DB()
	_, err = rawDB.ExecContext(context.Background(), `
		CREATE TABLE IF NOT EXISTS record (
			uri TEXT PRIMARY KEY,
			cid TEXT NOT NULL,
			did TEXT NOT NULL,
			collection TEXT NOT NULL,
			json TEXT NOT NULL DEFAULT '{}',
			indexed_at TEXT NOT NULL DEFAULT (datetime('now')),
			rkey TEXT NOT NULL DEFAULT ''
		)`)
	if err != nil {
		t.Fatalf("failed to create record table: %v", err)
	}
	execFn := func(query string, args ...any) {
		_, _ = rawDB.ExecContext(context.Background(), query, args...)
	}
	return NewRecordsRepository(exec), execFn
}

func insertSortRecord(t *testing.T, repo *RecordsRepository, uri, cid, did, collection, jsonData string) {
	t.Helper()
	_, err := repo.Insert(context.Background(), uri, cid, did, collection, jsonData)
	if err != nil {
		t.Fatalf("failed to insert record %s: %v", uri, err)
	}
}

func TestBuildSortExpr_NilSortOption(t *testing.T) {
	repo := newTestRepo(t)
	expr := repo.buildSortExpr(nil)
	want := "indexed_at DESC, uri DESC"
	if expr != want {
		t.Errorf("buildSortExpr(nil) = %q, want %q", expr, want)
	}
}

func TestBuildSortExpr_IndexedAtASC(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "indexed_at", Direction: "ASC"}
	expr := repo.buildSortExpr(sort)
	want := "indexed_at ASC, uri ASC"
	if expr != want {
		t.Errorf("buildSortExpr(indexed_at ASC) = %q, want %q", expr, want)
	}
}

func TestBuildSortExpr_IndexedAtDESC(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "indexed_at", Direction: "DESC"}
	expr := repo.buildSortExpr(sort)
	want := "indexed_at DESC, uri DESC"
	if expr != want {
		t.Errorf("buildSortExpr(indexed_at DESC) = %q, want %q", expr, want)
	}
}

func TestBuildSortExpr_URIField(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "uri", Direction: "ASC"}
	expr := repo.buildSortExpr(sort)
	// uri is the sort field itself — no tiebreaker appended
	if !strings.Contains(expr, "uri ASC") {
		t.Errorf("buildSortExpr(uri ASC) = %q, want to contain 'uri ASC'", expr)
	}
	// Should NOT have a second uri reference (no tiebreaker)
	if strings.Count(expr, "uri") > 1 {
		t.Errorf("buildSortExpr(uri ASC) = %q, should not have duplicate uri", expr)
	}
}

func TestBuildSortExpr_JSONField(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "createdAt", Direction: "DESC"}
	expr := repo.buildSortExpr(sort)
	// Should use JSONExtract (json_extract for SQLite)
	if !strings.Contains(expr, "json_extract") && !strings.Contains(expr, "->>'") {
		t.Errorf("buildSortExpr(createdAt DESC) = %q, want JSONExtract expression", expr)
	}
	if !strings.Contains(expr, "DESC") {
		t.Errorf("buildSortExpr(createdAt DESC) = %q, want DESC", expr)
	}
	// Should have uri tiebreaker
	if !strings.Contains(expr, "uri DESC") {
		t.Errorf("buildSortExpr(createdAt DESC) = %q, want uri DESC tiebreaker", expr)
	}
}

func TestBuildSortExpr_DirectColumnDID(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "did", Direction: "ASC"}
	expr := repo.buildSortExpr(sort)
	if !strings.Contains(expr, "did ASC") {
		t.Errorf("buildSortExpr(did ASC) = %q, want 'did ASC'", expr)
	}
	if !strings.Contains(expr, "uri ASC") {
		t.Errorf("buildSortExpr(did ASC) = %q, want 'uri ASC' tiebreaker", expr)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_DefaultSort(t *testing.T) {
	repo, execSQL := newSortTestRepo(t)
	ctx := context.Background()

	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"val":"a"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test/col/r1'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"val":"b"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test/col/r2'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"val":"c"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test/col/r3'`)

	// nil sort → indexed_at DESC (newest first)
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	// Newest first: r3, r2, r1
	if records[0].URI != "at://did:plc:test/col/r3" {
		t.Errorf("records[0].URI = %q, want r3", records[0].URI)
	}
	if records[1].URI != "at://did:plc:test/col/r2" {
		t.Errorf("records[1].URI = %q, want r2", records[1].URI)
	}
	if records[2].URI != "at://did:plc:test/col/r1" {
		t.Errorf("records[2].URI = %q, want r1", records[2].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_IndexedAtASC(t *testing.T) {
	repo, execSQL := newSortTestRepo(t)
	ctx := context.Background()

	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"val":"a"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test/col/r1'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"val":"b"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test/col/r2'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"val":"c"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test/col/r3'`)

	sort := &SortOption{Field: "indexed_at", Direction: "ASC"}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, DIDFilter{}, sort, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	// Oldest first: r1, r2, r3
	if records[0].URI != "at://did:plc:test/col/r1" {
		t.Errorf("records[0].URI = %q, want r1", records[0].URI)
	}
	if records[2].URI != "at://did:plc:test/col/r3" {
		t.Errorf("records[2].URI = %q, want r3", records[2].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_JSONFieldSort(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	// Insert records with different createdAt values in JSON
	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"createdAt":"2026-01-15T10:00:00Z"}`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"createdAt":"2026-01-15T12:00:00Z"}`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"createdAt":"2026-01-15T11:00:00Z"}`)

	// Sort by JSON field createdAt DESC
	sort := &SortOption{Field: "createdAt", Direction: "DESC"}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, DIDFilter{}, sort, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	// DESC: r2 (12:00), r3 (11:00), r1 (10:00)
	if records[0].URI != "at://did:plc:test/col/r2" {
		t.Errorf("records[0].URI = %q, want r2 (newest createdAt)", records[0].URI)
	}
	if records[1].URI != "at://did:plc:test/col/r3" {
		t.Errorf("records[1].URI = %q, want r3", records[1].URI)
	}
	if records[2].URI != "at://did:plc:test/col/r1" {
		t.Errorf("records[2].URI = %q, want r1 (oldest createdAt)", records[2].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_KeysetCursorASC(t *testing.T) {
	repo, execSQL := newSortTestRepo(t)
	ctx := context.Background()

	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"val":"a"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test/col/r1'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"val":"b"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test/col/r2'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"val":"c"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test/col/r3'`)

	// ASC sort: r1, r2, r3. Cursor after r1 → should return r2, r3
	sort := &SortOption{Field: "indexed_at", Direction: "ASC"}
	cursor := []string{"2026-01-15T10:00:00Z", "at://did:plc:test/col/r1"}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, DIDFilter{}, sort, 10, cursor)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].URI != "at://did:plc:test/col/r2" {
		t.Errorf("records[0].URI = %q, want r2", records[0].URI)
	}
	if records[1].URI != "at://did:plc:test/col/r3" {
		t.Errorf("records[1].URI = %q, want r3", records[1].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_KeysetCursorDESC(t *testing.T) {
	repo, execSQL := newSortTestRepo(t)
	ctx := context.Background()

	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"val":"a"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test/col/r1'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"val":"b"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test/col/r2'`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"val":"c"}`)
	execSQL(`UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test/col/r3'`)

	// DESC sort: r3, r2, r1. Cursor after r3 → should return r2, r1
	sort := &SortOption{Field: "indexed_at", Direction: "DESC"}
	cursor := []string{"2026-01-15T12:00:00Z", "at://did:plc:test/col/r3"}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, DIDFilter{}, sort, 10, cursor)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].URI != "at://did:plc:test/col/r2" {
		t.Errorf("records[0].URI = %q, want r2", records[0].URI)
	}
	if records[1].URI != "at://did:plc:test/col/r1" {
		t.Errorf("records[1].URI = %q, want r1", records[1].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_SortAndFilters(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	// Insert records: some with tag "go", some without
	insertSortRecord(t, repo, "at://did:plc:test/col/r1", "cid1", "did:plc:test", "col", `{"tag":"go","createdAt":"2026-01-15T10:00:00Z"}`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r2", "cid2", "did:plc:test", "col", `{"tag":"rust","createdAt":"2026-01-15T11:00:00Z"}`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r3", "cid3", "did:plc:test", "col", `{"tag":"go","createdAt":"2026-01-15T12:00:00Z"}`)
	insertSortRecord(t, repo, "at://did:plc:test/col/r4", "cid4", "did:plc:test", "col", `{"tag":"go","createdAt":"2026-01-15T09:00:00Z"}`)

	// Filter by tag=go, sort by createdAt ASC
	filters := []FieldFilter{
		{Field: "tag", Operator: "eq", Value: "go", FieldType: "string"},
	}
	sort := &SortOption{Field: "createdAt", Direction: "ASC"}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", filters, DIDFilter{}, sort, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	// Should return r4, r1, r3 (tag=go, sorted by createdAt ASC)
	if len(records) != 3 {
		t.Fatalf("got %d records, want 3", len(records))
	}
	if records[0].URI != "at://did:plc:test/col/r4" {
		t.Errorf("records[0].URI = %q, want r4 (09:00)", records[0].URI)
	}
	if records[1].URI != "at://did:plc:test/col/r1" {
		t.Errorf("records[1].URI = %q, want r1 (10:00)", records[1].URI)
	}
	if records[2].URI != "at://did:plc:test/col/r3" {
		t.Errorf("records[2].URI = %q, want r3 (12:00)", records[2].URI)
	}

	// Verify r2 (tag=rust) is excluded
	for _, rec := range records {
		if rec.URI == "at://did:plc:test/col/r2" {
			t.Error("r2 (tag=rust) should not be in results")
		}
	}
}

func TestBuildFilterClause_LIKEEscape(t *testing.T) {
	repo := newTestRepo(t)

	tests := []struct {
		name       string
		operator   string
		value      string
		wantParam  string // expected SQL parameter value
		wantEscape bool   // clause must contain ESCAPE
	}{
		{
			name:       "contains with percent is escaped",
			operator:   "contains",
			value:      "100%",
			wantParam:  `%100\%%`,
			wantEscape: true,
		},
		{
			name:       "contains with underscore is escaped",
			operator:   "contains",
			value:      "test_value",
			wantParam:  `%test\_value%`,
			wantEscape: true,
		},
		{
			name:       "contains with backslash is escaped",
			operator:   "contains",
			value:      `path\to`,
			wantParam:  `%path\\to%`,
			wantEscape: true,
		},
		{
			name:       "contains with no special chars is unchanged",
			operator:   "contains",
			value:      "hello",
			wantParam:  "%hello%",
			wantEscape: true,
		},
		{
			name:       "startsWith with percent is escaped",
			operator:   "startsWith",
			value:      "100%",
			wantParam:  `100\%%`,
			wantEscape: true,
		},
		{
			name:       "startsWith with underscore is escaped",
			operator:   "startsWith",
			value:      "test_",
			wantParam:  `test\_%`,
			wantEscape: true,
		},
		{
			name:       "startsWith with backslash is escaped",
			operator:   "startsWith",
			value:      `C:\Users`,
			wantParam:  `C:\\Users%`,
			wantEscape: true,
		},
		{
			name:       "startsWith with no special chars is unchanged",
			operator:   "startsWith",
			value:      "hello",
			wantParam:  "hello%",
			wantEscape: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := []FieldFilter{
				{Field: "title", Operator: tt.operator, Value: tt.value, FieldType: "string"},
			}
			clause, params, err := repo.buildFilterClause(filters, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if clause == "" {
				t.Fatalf("clause is empty")
			}

			// Verify ESCAPE clause is present
			if tt.wantEscape && !strings.Contains(clause, "ESCAPE") {
				t.Errorf("clause = %q, want to contain ESCAPE", clause)
			}

			// Verify the parameter value is correctly escaped
			if len(params) != 1 {
				t.Fatalf("expected 1 param, got %d", len(params))
			}
			tv, ok := params[0].(database.TextValue)
			if !ok {
				t.Fatalf("param is not TextValue, got %T", params[0])
			}
			if string(tv) != tt.wantParam {
				t.Errorf("param = %q, want %q", string(tv), tt.wantParam)
			}
		})
	}
}

// TestDIDFilter_IsEmpty verifies the IsEmpty helper.
func TestDIDFilter_IsEmpty(t *testing.T) {
	tests := []struct {
		name      string
		filter    DIDFilter
		wantEmpty bool
	}{
		{name: "zero value is empty", filter: DIDFilter{}, wantEmpty: true},
		{name: "EQ set is not empty", filter: DIDFilter{EQ: "did:plc:abc"}, wantEmpty: false},
		{name: "IN set is not empty", filter: DIDFilter{IN: []string{"did:plc:abc"}}, wantEmpty: false},
		{name: "both set is not empty", filter: DIDFilter{EQ: "did:plc:abc", IN: []string{"did:plc:def"}}, wantEmpty: false},
		{name: "empty EQ and nil IN is empty", filter: DIDFilter{EQ: "", IN: nil}, wantEmpty: true},
		{name: "empty EQ and empty IN is empty", filter: DIDFilter{EQ: "", IN: []string{}}, wantEmpty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.IsEmpty(); got != tt.wantEmpty {
				t.Errorf("IsEmpty() = %v, want %v", got, tt.wantEmpty)
			}
		})
	}
}

// TestBuildDIDFilterClause verifies the SQL clause generation for DIDFilter.
func TestBuildDIDFilterClause(t *testing.T) {
	repo := newTestRepo(t)

	tests := []struct {
		name         string
		filter       DIDFilter
		wantClause   string // expected substring in clause (empty means empty clause)
		wantParams   int
		wantConsumed int
	}{
		{
			name:         "empty filter returns empty clause",
			filter:       DIDFilter{},
			wantClause:   "",
			wantParams:   0,
			wantConsumed: 0,
		},
		{
			name:         "EQ filter generates did = ?",
			filter:       DIDFilter{EQ: "did:plc:abc"},
			wantClause:   "did = ?",
			wantParams:   1,
			wantConsumed: 1,
		},
		{
			name:         "IN filter generates did IN (?)",
			filter:       DIDFilter{IN: []string{"did:plc:abc", "did:plc:def"}},
			wantClause:   "did IN (?,?)",
			wantParams:   2,
			wantConsumed: 2,
		},
		{
			name:         "empty IN list is treated as empty filter (no clause)",
			filter:       DIDFilter{IN: []string{}},
			wantClause:   "",
			wantParams:   0,
			wantConsumed: 0,
		},
		{
			name:         "EQ takes precedence over IN when both set",
			filter:       DIDFilter{EQ: "did:plc:abc", IN: []string{"did:plc:def"}},
			wantClause:   "did = ?",
			wantParams:   1,
			wantConsumed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, params, consumed := repo.buildDIDFilterClause(tt.filter, 1)

			if tt.wantClause == "" {
				if clause != "" {
					t.Errorf("clause = %q, want empty", clause)
				}
				return
			}

			// Normalize placeholders for comparison (SQLite uses ?)
			if !strings.Contains(clause, strings.Split(tt.wantClause, "?")[0]) {
				t.Errorf("clause = %q, want to contain %q", clause, tt.wantClause)
			}
			if len(params) != tt.wantParams {
				t.Errorf("params count = %d, want %d", len(params), tt.wantParams)
			}
			if consumed != tt.wantConsumed {
				t.Errorf("consumed = %d, want %d", consumed, tt.wantConsumed)
			}
		})
	}
}

// TestGetByCollectionSortedWithKeysetCursor_DIDFilterIN verifies that the DID "in"
// filter correctly returns records from multiple DIDs.
func TestGetByCollectionSortedWithKeysetCursor_DIDFilterIN(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	// Insert records from 3 different DIDs
	insertSortRecord(t, repo, "at://did:plc:alice/col/r1", "cid1", "did:plc:alice", "col", `{}`)
	insertSortRecord(t, repo, "at://did:plc:bob/col/r2", "cid2", "did:plc:bob", "col", `{}`)
	insertSortRecord(t, repo, "at://did:plc:carol/col/r3", "cid3", "did:plc:carol", "col", `{}`)

	// Filter by DID in [alice, bob] — should return 2 records
	didFilter := DIDFilter{IN: []string{"did:plc:alice", "did:plc:bob"}}
	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, didFilter, nil, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	for _, rec := range records {
		if rec.DID != "did:plc:alice" && rec.DID != "did:plc:bob" {
			t.Errorf("unexpected DID %q, want alice or bob", rec.DID)
		}
	}

	// Filter by DID eq alice — should return 1 record
	didFilterEQ := DIDFilter{EQ: "did:plc:alice"}
	records2, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", nil, didFilterEQ, nil, 10, nil)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(records2) != 1 {
		t.Fatalf("got %d records, want 1", len(records2))
	}
	if records2[0].DID != "did:plc:alice" {
		t.Errorf("DID = %q, want did:plc:alice", records2[0].DID)
	}
}

func TestBuildFilterClause_INLimit(t *testing.T) {
	repo := newTestRepo(t)

	tests := []struct {
		name     string
		values   []interface{}
		wantErr  bool
		wantCond string // expected condition substring (when no error)
	}{
		{
			name:     "0 values returns 1 = 0",
			values:   []interface{}{},
			wantErr:  false,
			wantCond: "1 = 0",
		},
		{
			name:     "1 value succeeds",
			values:   []interface{}{"a"},
			wantErr:  false,
			wantCond: "IN (",
		},
		{
			name: "100 values (boundary) succeeds",
			values: func() []interface{} {
				vals := make([]interface{}, 100)
				for i := range vals {
					vals[i] = i
				}
				return vals
			}(),
			wantErr:  false,
			wantCond: "IN (",
		},
		{
			name: "101 values (over limit) returns error",
			values: func() []interface{} {
				vals := make([]interface{}, 101)
				for i := range vals {
					vals[i] = i
				}
				return vals
			}(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filters := []FieldFilter{
				{Field: "status", Operator: "in", Value: tt.values, FieldType: "string"},
			}
			clause, _, err := repo.buildFilterClause(filters, 1)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil (clause=%q)", clause)
				} else if !strings.Contains(err.Error(), "exceeds maximum") {
					t.Errorf("error = %q, want to contain \"exceeds maximum\"", err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantCond != "" && !strings.Contains(clause, tt.wantCond) {
				t.Errorf("clause = %q, want to contain %q", clause, tt.wantCond)
			}
		})
	}
}
