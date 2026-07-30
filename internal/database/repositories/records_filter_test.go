// Package repositories contains data access layer implementations.
package repositories

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
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

func TestBuildFilterClause_InvalidOperators(t *testing.T) {
	repo := newTestRepo(t)

	t.Run("unsupported operator returns error", func(t *testing.T) {
		_, _, err := repo.buildFilterClause([]FieldFilter{
			{Field: "image", Operator: "exists", Value: true, FieldType: "object"},
		}, 1)
		if err == nil {
			t.Fatal("expected unsupported operator error, got nil")
		}
		if !strings.Contains(err.Error(), "unsupported filter operator") {
			t.Fatalf("error = %q, want unsupported operator message", err.Error())
		}
	})

	t.Run("isNull requires boolean", func(t *testing.T) {
		_, _, err := repo.buildFilterClause([]FieldFilter{
			{Field: "image", Operator: "isNull", Value: "false", FieldType: "object"},
		}, 1)
		if err == nil {
			t.Fatal("expected isNull type error, got nil")
		}
		if !strings.Contains(err.Error(), "must be a boolean") {
			t.Fatalf("error = %q, want boolean type message", err.Error())
		}
	})
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

func TestBuildFilterClause_URIUsesRecordColumn(t *testing.T) {
	repo := newTestRepo(t)
	filters := []FieldFilter{
		{Field: "uri", Operator: "eq", Value: "at://did:plc:alice/com.example.post/1", FieldType: "string", Target: FieldFilterTargetColumn},
	}

	clause, params, err := repo.buildFilterClause(filters, 1)
	if err != nil {
		t.Fatalf("buildFilterClause() error = %v", err)
	}
	if !strings.Contains(clause, "uri = ?") {
		t.Errorf("clause = %q, want record uri column comparison", clause)
	}
	if strings.Contains(clause, "json_extract") {
		t.Errorf("clause = %q, should not extract uri from record JSON", clause)
	}
	if len(params) != 1 {
		t.Fatalf("params count = %d, want 1", len(params))
	}
}

func TestBuildFilterClause_DefaultTargetUsesJSONForSortColumnNames(t *testing.T) {
	repo := newTestRepo(t)

	for _, field := range []string{"collection", "indexed_at"} {
		t.Run(field, func(t *testing.T) {
			filters := []FieldFilter{
				{Field: field, Operator: "eq", Value: "json-value", FieldType: "string"},
			}

			clause, _, err := repo.buildFilterClause(filters, 1)
			if err != nil {
				t.Fatalf("buildFilterClause() error = %v", err)
			}
			if !strings.Contains(clause, "json_extract(json, '$."+field+"')") {
				t.Fatalf("clause = %q, want JSON extraction for lexicon field %q", clause, field)
			}
			if strings.Contains(clause, field+" = ?") {
				t.Fatalf("clause = %q, should not use record table column for lexicon field %q", clause, field)
			}
		})
	}
}

func TestGetByCollectionSortedWithKeysetCursor_FilterColumnNameCollisionUsesJSON(t *testing.T) {
	tests := []struct {
		name       string
		field      string
		value      string
		targetJSON string
		otherJSON  string
	}{
		{
			name:       "collection JSON property",
			field:      "collection",
			value:      "forest",
			targetJSON: `{"collection":"forest","text":"target"}`,
			otherJSON:  `{"collection":"other","text":"other"}`,
		},
		{
			name:       "indexed_at JSON property",
			field:      "indexed_at",
			value:      "json-indexed-at",
			targetJSON: `{"indexed_at":"json-indexed-at","text":"target"}`,
			otherJSON:  `{"indexed_at":"other","text":"other"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newSortTestRepo(t)
			ctx := context.Background()

			const targetURI = "at://did:plc:alice/com.example.post/target"
			insertSortRecord(t, repo, targetURI, "cid-target", "did:plc:alice", "com.example.post", tt.targetJSON)
			insertSortRecord(t, repo, "at://did:plc:bob/com.example.post/other", "cid-other", "did:plc:bob", "com.example.post", tt.otherJSON)

			records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "com.example.post", []FieldFilter{
				{Field: tt.field, Operator: "eq", Value: tt.value, FieldType: "string"},
			}, DIDFilter{}, nil, 10, nil)
			if err != nil {
				t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
			}
			if len(records) != 1 {
				t.Fatalf("len(records) = %d, want 1", len(records))
			}
			if records[0].URI != targetURI {
				t.Fatalf("records[0].URI = %q, want %q", records[0].URI, targetURI)
			}
		})
	}
}

func TestBuildFilterClause_RejectsUnsupportedMetadataColumnTarget(t *testing.T) {
	repo := newTestRepo(t)

	_, _, err := repo.buildFilterClause([]FieldFilter{
		{Field: "collection", Operator: "eq", Value: "com.example.post", FieldType: "string", Target: FieldFilterTargetColumn},
	}, 1)
	if err == nil {
		t.Fatal("expected unsupported metadata filter column error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported metadata filter column \"collection\"") {
		t.Fatalf("error = %q, want unsupported metadata filter column message", err.Error())
	}
}

func TestGetByCollectionSortedWithKeysetCursor_URIFilterUsesRecordColumn(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	const targetURI = "at://did:plc:alice/com.example.post/target"
	insertSortRecord(t, repo, targetURI, "cid-target", "did:plc:alice", "com.example.post", `{"uri":"json-shadow","text":"target"}`)
	insertSortRecord(t, repo, "at://did:plc:bob/com.example.post/other", "cid-other", "did:plc:bob", "com.example.post", `{"uri":"`+targetURI+`","text":"other"}`)
	insertSortRecord(t, repo, "at://did:plc:alice/com.example.post/third", "cid-third", "did:plc:alice", "com.example.post", `{"text":"third"}`)

	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "com.example.post", []FieldFilter{
		{Field: "uri", Operator: "eq", Value: targetURI, FieldType: "string", Target: FieldFilterTargetColumn},
	}, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].URI != targetURI {
		t.Errorf("records[0].URI = %q, want %q", records[0].URI, targetURI)
	}

	count, err := repo.GetCollectionCountFiltered(ctx, "com.example.post", []FieldFilter{
		{Field: "uri", Operator: "in", Value: []interface{}{targetURI}, FieldType: "string", Target: FieldFilterTargetColumn},
	}, DIDFilter{})
	if err != nil {
		t.Fatalf("GetCollectionCountFiltered() error = %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_NestedArrayAnyFilter(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	const targetURI = "at://did:plc:maker/org.hypercerts.claim.activity/activity-1"
	insertSortRecord(t, repo, "at://did:plc:alice/org.hypercerts.collection/contains", "cid-contains", "did:plc:alice", "org.hypercerts.collection", `{"title":"Project","items":[{"itemIdentifier":{"uri":"`+targetURI+`","cid":"bafyactivity"}}]}`)
	insertSortRecord(t, repo, "at://did:plc:alice/org.hypercerts.collection/other", "cid-other", "did:plc:alice", "org.hypercerts.collection", `{"title":"Other","items":[{"itemIdentifier":{"uri":"at://did:plc:maker/org.hypercerts.claim.activity/other","cid":"bafyother"}}]}`)

	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "org.hypercerts.collection", []FieldFilter{
		{Field: "items", Path: []string{"itemIdentifier", "uri"}, ArrayPath: []string{"items"}, Operator: "eq", Value: targetURI, FieldType: "string"},
	}, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].URI != "at://did:plc:alice/org.hypercerts.collection/contains" {
		t.Fatalf("records[0].URI = %q, want contains record", records[0].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_NestedArrayAnyFilterKeepsPredicatesOnSameElement(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	const targetURI = "at://did:plc:maker/org.hypercerts.claim.activity/activity-1"
	const targetCID = "bafyactivity"
	insertSortRecord(t, repo, "at://did:plc:alice/org.hypercerts.collection/exact", "cid-exact", "did:plc:alice", "org.hypercerts.collection", `{"title":"Exact","items":[{"itemIdentifier":{"uri":"`+targetURI+`","cid":"`+targetCID+`"}}]}`)
	insertSortRecord(t, repo, "at://did:plc:alice/org.hypercerts.collection/split", "cid-split", "did:plc:alice", "org.hypercerts.collection", `{"title":"Split","items":[{"itemIdentifier":{"uri":"`+targetURI+`","cid":"wrong-cid"}},{"itemIdentifier":{"uri":"at://did:plc:maker/org.hypercerts.claim.activity/other","cid":"`+targetCID+`"}}]}`)

	filters := []FieldFilter{
		{Field: "items", Path: []string{"itemIdentifier", "uri"}, ArrayPath: []string{"items"}, Operator: "eq", Value: targetURI, FieldType: "string"},
		{Field: "items", Path: []string{"itemIdentifier", "cid"}, ArrayPath: []string{"items"}, Operator: "eq", Value: targetCID, FieldType: "string"},
	}
	clause, params, err := repo.buildFilterClause(filters, 1)
	if err != nil {
		t.Fatalf("buildFilterClause() error = %v", err)
	}
	if got := strings.Count(clause, "EXISTS (SELECT 1 FROM json_each"); got != 1 {
		t.Fatalf("array filters should share one EXISTS clause, got %d in %q", got, clause)
	}
	if !strings.Contains(clause, " AND ") {
		t.Fatalf("array filters should be joined inside the same EXISTS clause: %q", clause)
	}
	if len(params) != 2 {
		t.Fatalf("len(params) = %d, want 2", len(params))
	}

	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "org.hypercerts.collection", filters, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].URI != "at://did:plc:alice/org.hypercerts.collection/exact" {
		t.Fatalf("records[0].URI = %q, want exact record", records[0].URI)
	}

	count, err := repo.GetCollectionCountFiltered(ctx, "org.hypercerts.collection", filters, DIDFilter{})
	if err != nil {
		t.Fatalf("GetCollectionCountFiltered() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_NestedThreeLevelJSONPathFilter(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	insertSortRecord(t, repo, "at://did:plc:alice/com.example.nested/target", "cid-target", "did:plc:alice", "com.example.nested", `{"outer":{"middle":{"leaf":"target"}}}`)
	insertSortRecord(t, repo, "at://did:plc:alice/com.example.nested/other", "cid-other", "did:plc:alice", "com.example.nested", `{"outer":{"middle":{"leaf":"other"}}}`)
	insertSortRecord(t, repo, "at://did:plc:alice/com.example.nested/missing", "cid-missing", "did:plc:alice", "com.example.nested", `{"outer":{"middle":{}}}`)

	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "com.example.nested", []FieldFilter{
		{Field: "outer", Path: []string{"outer", "middle", "leaf"}, Operator: "eq", Value: "target", FieldType: "string"},
	}, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].URI != "at://did:plc:alice/com.example.nested/target" {
		t.Fatalf("records[0].URI = %q, want target record", records[0].URI)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_ContributorDIDCompatibilityFilter(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	const contributorURI = "at://did:plc:contributor/org.hypercerts.claim.contributorInformation/info-1"
	insertSortRecord(t, repo, contributorURI, "cid-info", "did:plc:contributor", "org.hypercerts.claim.contributorInformation", `{"identifier":"did:plc:alice"}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/inline", "cid-inline", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":[{"contributorIdentity":{"identity":"did:plc:alice"}}]}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/ref", "cid-ref", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":[{"contributorIdentity":{"uri":"`+contributorURI+`","cid":"cid-info"}}]}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/direct", "cid-direct", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":[{"identity":"did:plc:alice"}]}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/bare", "cid-bare", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":["did:plc:alice"]}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/other", "cid-other", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":[{"contributorIdentity":{"identity":"did:plc:bob"}}]}`)
	insertSortRecord(t, repo, "at://did:plc:author/org.hypercerts.claim.activity/bare-other", "cid-bare-other", "did:plc:author", "org.hypercerts.claim.activity", `{"contributors":["did:plc:bob"]}`)

	records, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "org.hypercerts.claim.activity", []FieldFilter{
		{Field: "contributorDid", Operator: "eq", Value: "did:plc:alice", FieldType: "string", Target: FieldFilterTargetContributorDID},
	}, DIDFilter{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursor() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("len(records) = %d, want 4", len(records))
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
			rkey TEXT NOT NULL DEFAULT '',
			record_created_at TEXT,
			validation_status TEXT NOT NULL DEFAULT 'unknown_schema',
			validation_error TEXT,
			validated_at TEXT,
			lexicon_hash TEXT
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
	want := "strftime('%Y-%m-%dT%H:%M:%fZ', indexed_at) DESC, uri DESC"
	if expr != want {
		t.Errorf("buildSortExpr(nil) = %q, want %q", expr, want)
	}
}

func TestBuildSortExpr_IndexedAtASC(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "indexed_at", Direction: "ASC"}
	expr := repo.buildSortExpr(sort)
	want := "strftime('%Y-%m-%dT%H:%M:%fZ', indexed_at) ASC, uri ASC"
	if expr != want {
		t.Errorf("buildSortExpr(indexed_at ASC) = %q, want %q", expr, want)
	}
}

func TestBuildSortExpr_IndexedAtDESC(t *testing.T) {
	repo := newTestRepo(t)
	sort := &SortOption{Field: "indexed_at", Direction: "DESC"}
	expr := repo.buildSortExpr(sort)
	want := "strftime('%Y-%m-%dT%H:%M:%fZ', indexed_at) DESC, uri DESC"
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
			clause, params, consumed, err := repo.buildDIDFilterClause(tt.filter, 1)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

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

func TestBuildDIDFilterClause_INLimit(t *testing.T) {
	repo := newTestRepo(t)

	makeDIDs := func(n int) []string {
		vals := make([]string, n)
		for i := range vals {
			vals[i] = "did:plc:" + string(rune('a'+(i%26)))
		}
		return vals
	}

	tests := []struct {
		name      string
		filter    DIDFilter
		wantErr   bool
		wantCount int
	}{
		{
			name:      "boundary succeeds at max limit",
			filter:    DIDFilter{IN: makeDIDs(MaxINListSize)},
			wantErr:   false,
			wantCount: MaxINListSize,
		},
		{
			name:    "over limit returns error",
			filter:  DIDFilter{IN: makeDIDs(MaxINListSize + 1)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clause, params, consumed, err := repo.buildDIDFilterClause(tt.filter, 1)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (clause=%q)", clause)
				}
				if !strings.Contains(err.Error(), "exceeds maximum") {
					t.Fatalf("error = %q, want to contain %q", err.Error(), "exceeds maximum")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(params) != tt.wantCount {
				t.Fatalf("params count = %d, want %d", len(params), tt.wantCount)
			}
			if consumed != tt.wantCount {
				t.Fatalf("consumed = %d, want %d", consumed, tt.wantCount)
			}
			if !strings.Contains(clause, "did IN (") {
				t.Fatalf("clause = %q, want DID IN clause", clause)
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

func TestGetByCollectionSortedWithKeysetCursor_AggregateINOverflow(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	filters := make([]FieldFilter, 10)
	for i := range filters {
		values := make([]interface{}, 100)
		for j := range values {
			values[j] = "value"
		}
		filters[i] = FieldFilter{
			Field:     "field" + string(rune('a'+i)),
			Operator:  "in",
			Value:     values,
			FieldType: "string",
		}
	}

	_, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", filters, DIDFilter{}, nil, 10, nil)
	if err == nil {
		t.Fatal("expected error for aggregate parameter overflow, got nil")
	}
	if !errors.Is(err, ErrSQLiteAggregateParameterLimit) {
		t.Fatalf("error = %v, want ErrSQLiteAggregateParameterLimit", err)
	}
}

func TestGetByCollectionSortedWithKeysetCursor_AggregateINBoundary(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	filters := make([]FieldFilter, 20)
	for i := range filters {
		values := make([]interface{}, 49)
		for j := range values {
			values[j] = "value"
		}
		filters[i] = FieldFilter{
			Field:     "field" + string(rune('a'+i)),
			Operator:  "in",
			Value:     values,
			FieldType: "string",
		}
	}

	dids := make([]string, 18)
	for i := range dids {
		dids[i] = "did:" + string(rune('a'+i))
	}

	_, err := repo.GetByCollectionSortedWithKeysetCursor(ctx, "col", filters, DIDFilter{IN: dids}, nil, 10, nil)
	if err != nil {
		t.Fatalf("expected query to succeed at SQLite aggregate limit, got %v", err)
	}
}

func makeAggregateOverflowFilters() []FieldFilter {
	filters := make([]FieldFilter, 20)
	for i := range filters {
		values := make([]interface{}, 49)
		for j := range values {
			values[j] = "value"
		}
		filters[i] = FieldFilter{
			Field:     "field" + string(rune('a'+i)),
			Operator:  "in",
			Value:     values,
			FieldType: "string",
		}
	}
	return filters
}

func makeAggregateOverflowDIDs(n int) []string {
	dids := make([]string, n)
	for i := range dids {
		dids[i] = "did:" + string(rune('a'+i))
	}
	return dids
}

func TestGetByCollectionFilteredWithKeysetCursor_AggregateOverflow(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetByCollectionFilteredWithKeysetCursor(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(19)},
		10,
		"2026-01-15T12:00:00Z",
		"at://did:plc:test/col/r1",
	)
	if err == nil {
		t.Fatal("expected error for aggregate parameter overflow, got nil")
	}
	if !errors.Is(err, ErrSQLiteAggregateParameterLimit) {
		t.Fatalf("error = %v, want ErrSQLiteAggregateParameterLimit", err)
	}
}

func TestGetByCollectionFilteredWithKeysetCursor_AggregateBoundary(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetByCollectionFilteredWithKeysetCursor(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(15)},
		10,
		"2026-01-15T12:00:00Z",
		"at://did:plc:test/col/r1",
	)
	if err != nil {
		t.Fatalf("expected query to succeed at SQLite aggregate limit, got %v", err)
	}
}

func TestGetByCollectionReversedWithKeysetCursor_AggregateOverflow(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetByCollectionReversedWithKeysetCursor(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(19)},
		&SortOption{Field: "indexed_at", Direction: "DESC"},
		10,
		[]string{"2026-01-15T12:00:00Z", "at://did:plc:test/col/r1"},
	)
	if err == nil {
		t.Fatal("expected error for aggregate parameter overflow, got nil")
	}
	if !errors.Is(err, ErrSQLiteAggregateParameterLimit) {
		t.Fatalf("error = %v, want ErrSQLiteAggregateParameterLimit", err)
	}
}

func TestGetByCollectionReversedWithKeysetCursor_AggregateBoundary(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetByCollectionReversedWithKeysetCursor(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(15)},
		&SortOption{Field: "indexed_at", Direction: "DESC"},
		10,
		[]string{"2026-01-15T12:00:00Z", "at://did:plc:test/col/r1"},
	)
	if err != nil {
		t.Fatalf("expected query to succeed at SQLite aggregate limit, got %v", err)
	}
}

func TestGetCollectionCountFiltered_AggregateOverflow(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetCollectionCountFiltered(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(19)},
	)
	if err == nil {
		t.Fatal("expected error for aggregate parameter overflow, got nil")
	}
	if !errors.Is(err, ErrSQLiteAggregateParameterLimit) {
		t.Fatalf("error = %v, want ErrSQLiteAggregateParameterLimit", err)
	}
}

func TestGetCollectionCountFiltered_AggregateBoundary(t *testing.T) {
	repo, _ := newSortTestRepo(t)
	ctx := context.Background()

	_, err := repo.GetCollectionCountFiltered(
		ctx,
		"col",
		makeAggregateOverflowFilters(),
		DIDFilter{IN: makeAggregateOverflowDIDs(18)},
	)
	if err != nil {
		t.Fatalf("expected query to succeed at SQLite aggregate limit, got %v", err)
	}
}
