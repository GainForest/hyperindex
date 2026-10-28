package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
)

// TestSortedRecordsQueryCreatedAtColumnAvoidsSortPostgres guards the fix for
// slow sortBy: createdAt reads: validated createdAt-sorted queries must walk an
// index in order and stop at LIMIT instead of sorting every valid row.
func TestSortedRecordsQueryCreatedAtColumnAvoidsSortPostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()

	// Enough rows that a full scan plus sort is clearly more expensive than an
	// ordered index scan, mirroring a large collection in production.
	if _, err := exec.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json, record_created_at, validation_status)
		SELECT
			'at://did:plc:plan/' || c || '/' || g,
			'cid', 'did:plc:plan', c,
			jsonb_build_object('createdAt', to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), 'type', CASE WHEN g % 10 = 0 THEN 'project' ELSE 'other' END),
			ts, 'valid'
		FROM generate_series(1, 20000) AS g,
			LATERAL (SELECT timestamptz '2024-01-01' + g * interval '1 minute' AS ts) t,
			unnest(ARRAY['com.example.plan', 'com.example.other']) AS c`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, "ANALYZE record"); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	desc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "DESC"}
	asc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "ASC"}
	rangeFilters := []FieldFilter{
		{Field: SortFieldRecordCreatedAt, Operator: "gte", Value: "2024-01-02T00:00:00.000Z", Target: FieldFilterTargetColumn},
		{Field: SortFieldRecordCreatedAt, Operator: "lte", Value: "2024-01-10T00:00:00.000Z", Target: FieldFilterTargetColumn},
	}
	typeFilter := []FieldFilter{{Field: "type", Operator: "in", Value: []interface{}{"project", "Project"}}}

	tests := []struct {
		name     string
		filters  []FieldFilter
		sort     *SortOption
		limit    int
		after    []string
		wantSort bool
	}{
		{name: "desc limit 2", sort: desc, limit: 2},
		{name: "desc limit 1001", sort: desc, limit: 1001},
		{name: "asc limit 2", sort: asc, limit: 2},
		{name: "desc after cursor", sort: desc, limit: 2, after: []string{"2024-01-05T00:00:00.000Z", "at://did:plc:plan/com.example.plan/5000"}},
		{name: "range filter", sort: desc, limit: 1001, filters: rangeFilters},
		{name: "json filter", sort: desc, limit: 2, filters: typeFilter},
		// Control: the JSON sort this change replaces still needs a Sort node,
		// proving the assertion can detect the regression.
		{name: "json createdAt control", sort: &SortOption{Field: "createdAt", Direction: "DESC"}, limit: 2, wantSort: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlStr, args, err := repo.sortedRecordsQuery("com.example.plan", tt.filters, DIDFilter{}, ExternalLabelFilterSet{}, tt.sort, tt.limit, tt.after, true)
			if err != nil {
				t.Fatalf("sortedRecordsQuery() error = %v", err)
			}
			var planJSON string
			if err := exec.DB().QueryRowContext(ctx, "EXPLAIN (FORMAT JSON) "+sqlStr, args...).Scan(&planJSON); err != nil {
				t.Fatalf("EXPLAIN error = %v\nSQL: %s", err, sqlStr)
			}
			if got := planHasNodeType(t, planJSON, "Sort"); got != tt.wantSort {
				t.Fatalf("plan has Sort node = %v, want %v\nSQL: %s\nplan: %s", got, tt.wantSort, sqlStr, planJSON)
			}
		})
	}
}

func planHasNodeType(t *testing.T, planJSON, nodeType string) bool {
	t.Helper()
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(planJSON), &plans); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	var walk func(node map[string]any) bool
	walk = func(node map[string]any) bool {
		if node["Node Type"] == nodeType {
			return true
		}
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			if m, ok := child.(map[string]any); ok && walk(m) {
				return true
			}
		}
		return false
	}
	for _, p := range plans {
		if walk(p.Plan) {
			return true
		}
	}
	return false
}

func newCreatedAtPlanPostgresExecutor(t *testing.T) *postgres.Executor {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	parsed, err := url.Parse(databaseURL)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		t.Skip("query plan test requires DATABASE_URL pointing at PostgreSQL")
	}
	name := strings.ToLower(strings.TrimPrefix(parsed.Path, "/"))
	if name != "test" && !strings.HasSuffix(name, "_test") && !strings.HasSuffix(name, "-test") {
		t.Skip("query plan test requires a database named test or ending with _test/-test")
	}

	ctx := context.Background()
	admin, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("admin executor: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })

	schema := fmt.Sprintf("hyperindex_created_at_plan_test_%d", time.Now().UnixNano())
	if _, err := admin.DB().ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.DB().ExecContext(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})

	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	exec, err := postgres.NewExecutor(parsed.String())
	if err != nil {
		t.Fatalf("schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return exec
}
