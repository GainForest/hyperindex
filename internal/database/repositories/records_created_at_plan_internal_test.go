package repositories

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
)

// TestSortedRecordsQueryCreatedAtColumnAvoidsSortPostgres guards the fix for
// slow sortBy: createdAt reads: createdAt-sorted queries, validated or not, must
// walk an index in order and stop at LIMIT instead of sorting every row. Every
// twentieth row has no record_created_at, so the NULLS LAST ordering and the
// cursor conditions that cross the NULL boundary are what the plans cover. A
// page whose continuation spans the dated and undated rows is two statements;
// each one is checked on its own.
func TestSortedRecordsQueryCreatedAtColumnAvoidsSortPostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()

	seedCreatedAtPlanRows(t, exec)

	desc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "DESC"}
	asc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "ASC"}
	rangeFilters := []FieldFilter{
		{Field: SortFieldRecordCreatedAt, Operator: "gte", Value: "2024-01-02T00:00:00.000Z", Target: FieldFilterTargetColumn},
		{Field: SortFieldRecordCreatedAt, Operator: "lte", Value: "2024-01-10T00:00:00.000Z", Target: FieldFilterTargetColumn},
	}
	typeFilter := []FieldFilter{{Field: "type", Operator: "in", Value: []interface{}{"project", "Project"}}}

	// undated is the cursor of a row without record_created_at (g = 20).
	undated := []string{"", "at://did:plc:heavy/com.example.plan/20"}
	dated := []string{"2024-01-05T00:00:00.000Z", "at://did:plc:heavy/com.example.plan/5000"}
	tests := []struct {
		name      string
		filters   []FieldFilter
		did       DIDFilter
		sort      *SortOption
		limit     int
		after     []string
		validOnly bool
		wantIndex string
		wantSort  bool
		// wantSegments is how many statements the page is split into; 0 means 1.
		wantSegments int
		// checkSegments limits the plan checks to the first N segments; 0 checks
		// all. A page as large as the undated segment may legitimately sort the
		// whole undated segment, which holds about a thousand rows here.
		checkSegments int
		// bounded asserts the cursor is an index bound: no index scan may visit
		// more than limit+1 rows, counting rows it filtered out. maxVisited
		// raises that ceiling where another predicate legitimately discards
		// rows inside the bounded range, such as a did filter served by the
		// collection index.
		bounded    bool
		maxVisited int
	}{
		{name: "valid desc limit 2", sort: desc, limit: 2, validOnly: true, wantIndex: "idx_record_collection_created_at_valid"},
		{name: "valid desc limit 1001", sort: desc, limit: 1001, validOnly: true, wantIndex: "idx_record_collection_created_at_valid"},
		{name: "valid asc limit 2", sort: asc, limit: 2, validOnly: true, wantIndex: "idx_record_collection_created_at_valid"},
		// Deep pages: dated is ~14000 rows into the collection in both directions.
		// A page must cost about what it returns, not what precedes the cursor.
		{name: "valid desc after dated cursor", sort: desc, limit: 26, after: dated, validOnly: true, wantIndex: "idx_record_collection_created_at_valid", bounded: true, wantSegments: 2},
		{name: "valid desc after undated cursor", sort: desc, limit: 26, after: undated, validOnly: true, wantIndex: "idx_record_collection_created_at_valid", bounded: true},
		{name: "valid asc after dated cursor", sort: asc, limit: 26, after: dated, validOnly: true, wantIndex: "idx_record_collection_created_at_valid", bounded: true},
		{name: "valid asc after undated cursor", sort: asc, limit: 26, after: undated, validOnly: true, wantIndex: "idx_record_collection_created_at_valid", bounded: true, wantSegments: 2},
		{name: "valid desc limit 1001 after dated cursor", sort: desc, limit: 1001, after: dated, validOnly: true, wantIndex: "idx_record_collection_created_at_valid", bounded: true, wantSegments: 2, checkSegments: 1},
		{name: "valid range filter", sort: desc, limit: 1001, filters: rangeFilters, validOnly: true, wantIndex: "idx_record_collection_created_at_valid"},
		{name: "valid json filter", sort: desc, limit: 2, filters: typeFilter, validOnly: true, wantIndex: "idx_record_collection_created_at_valid"},
		{name: "valid did filter", sort: desc, limit: 26, did: DIDFilter{EQ: "did:plc:heavy"}, validOnly: true, wantIndex: "idx_record_author_collection_created_at"},
		{name: "valid did filter after dated cursor", sort: desc, limit: 26, did: DIDFilter{EQ: "did:plc:heavy"}, after: dated, validOnly: true, wantIndex: "idx_record_author_collection_created_at", bounded: true, maxVisited: 5 * 26, wantSegments: 2},
		{name: "unvalidated did filter ascending", sort: asc, limit: 26, did: DIDFilter{EQ: "did:plc:heavy"}, wantIndex: "idx_record_author_collection_created_at"},
		{name: "unvalidated desc limit 2", sort: desc, limit: 2, wantIndex: "idx_record_collection_created_at"},
		{name: "unvalidated desc limit 1001", sort: desc, limit: 1001, wantIndex: "idx_record_collection_created_at"},
		{name: "unvalidated desc after dated cursor", sort: desc, limit: 26, after: dated, wantIndex: "idx_record_collection_created_at", bounded: true, wantSegments: 2},
		{name: "unvalidated desc after undated cursor", sort: desc, limit: 26, after: undated, wantIndex: "idx_record_collection_created_at", bounded: true},
		{name: "unvalidated asc after dated cursor", sort: asc, limit: 26, after: dated, wantIndex: "idx_record_collection_created_at", bounded: true},
		{name: "unvalidated asc after undated cursor", sort: asc, limit: 26, after: undated, wantIndex: "idx_record_collection_created_at", bounded: true, wantSegments: 2},
		// Control: the JSON sort this change replaces still needs a Sort node,
		// proving the assertion can detect the regression.
		{name: "json createdAt control", sort: &SortOption{Field: "createdAt", Direction: "DESC"}, limit: 2, validOnly: true, wantSort: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.sortedRecordsPage("com.example.plan", tt.filters, tt.did, ExternalLabelFilterSet{}, tt.sort, tt.after, tt.validOnly)
			if err != nil {
				t.Fatalf("sortedRecordsPage() error = %v", err)
			}
			wantSegments := max(tt.wantSegments, 1)
			if len(page.segments) != wantSegments {
				t.Fatalf("page has %d segments, want %d", len(page.segments), wantSegments)
			}

			usesWanted := tt.wantIndex == ""
			checked := len(page.segments)
			if tt.checkSegments > 0 {
				checked = tt.checkSegments
			}
			for i := range checked {
				sqlStr := page.sql(i, tt.limit)
				var planJSON string
				if err := exec.DB().QueryRowContext(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+sqlStr, page.segments[i].args...).Scan(&planJSON); err != nil {
					t.Fatalf("segment %d EXPLAIN error = %v\nSQL: %s", i, err, sqlStr)
				}
				if got := planHasNodeType(t, planJSON, "Sort"); got != tt.wantSort {
					t.Fatalf("segment %d plan has Sort node = %v, want %v\nSQL: %s\nplan: %s", i, got, tt.wantSort, sqlStr, planJSON)
				}
				usesWanted = usesWanted || strings.Contains(planJSON, `"Index Name": "`+tt.wantIndex+`"`)
				if tt.bounded {
					maxVisited := tt.maxVisited
					if maxVisited == 0 {
						maxVisited = tt.limit + 1
					}
					assertIndexScansBounded(t, planJSON, maxVisited)
				}
			}
			if !usesWanted {
				t.Fatalf("no segment uses %s\nSQL: %s", tt.wantIndex, page.sql(0, tt.limit))
			}
		})
	}
}

// seedCreatedAtPlanRows inserts enough rows that a full scan plus sort is
// clearly more expensive than an ordered index scan, mirroring a large
// collection in production. com.example.plan and com.example.other each get
// 20000 valid rows: half belong to did:plc:heavy, the rest are spread over 200
// small authors. Every twentieth row has no record_created_at and no "tag"; the
// others have tag "dated". Timestamps are UTC minutes from 2024-01-01, so the
// row with g = 5760 is 2024-01-05T00:00:00Z whatever the session time zone.
func seedCreatedAtPlanRows(t *testing.T, exec *postgres.Executor) {
	t.Helper()
	ctx := context.Background()
	if _, err := exec.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json, record_created_at, validation_status)
		SELECT
			'at://' || a.did || '/' || c || '/' || g,
			'cid', a.did, c,
			jsonb_build_object('createdAt', to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), 'type', CASE WHEN g % 10 = 0 THEN 'project' ELSE 'other' END,
				'tag', CASE WHEN g % 20 = 0 THEN NULL ELSE 'dated' END),
			CASE WHEN g % 20 = 0 THEN NULL ELSE ts END, 'valid'
		FROM generate_series(1, 20000) AS g,
			LATERAL (SELECT timestamptz '2024-01-01 00:00:00+00' + g * interval '1 minute' AS ts) t,
			LATERAL (SELECT CASE WHEN g % 2 = 0 THEN 'did:plc:heavy' ELSE 'did:plc:plan' || (g % 200) END AS did) a,
			unnest(ARRAY['com.example.plan', 'com.example.other']) AS c`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, "ANALYZE record"); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}
}

// recordTimeline excludes undated rows, but its ORDER BY must still match the
// index PostgreSQL walks: collection-wide reads use the full NULLS LAST index
// that typed connections share, and author-scoped reads keep the partial author
// index, whose default NULL placement a NULLS LAST ordering would not match.
func TestRecordTimelineQueryUsesIndexesPostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()
	seedCreatedAtPlanRows(t, exec)

	after := &RecordTimelineCursor{CreatedAt: "2024-01-05T00:00:00.000Z", URI: "at://did:plc:heavy/com.example.plan/5000"}
	tests := []struct {
		name    string
		authors []string
		after   *RecordTimelineCursor
		// wantIndexes lists acceptable indexes; any one satisfies the test.
		wantIndexes []string
		// maxVisited is the most rows any index scan may visit, counting rows a
		// filter discards. Author reads may be served by either partial index.
		maxVisited int
	}{
		{name: "collection-wide first page", wantIndexes: []string{"idx_record_collection_created_at"}, maxVisited: 26},
		{name: "collection-wide after cursor", after: after, wantIndexes: []string{"idx_record_collection_created_at"}, maxVisited: 26},
		{name: "one author first page", authors: []string{"did:plc:heavy"}, wantIndexes: []string{"idx_record_timeline_author_collection_created"}, maxVisited: 26},
		// did:plc:heavy owns half the collection, so the planner may also walk the
		// retained partial collection index and discard the other half.
		{name: "one author after cursor", authors: []string{"did:plc:heavy"}, after: after, wantIndexes: []string{"idx_record_timeline_author_collection_created", "idx_record_timeline_collection_created"}, maxVisited: 5 * 26},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlStr, params, err := repo.recordTimelineQuery(tt.authors, []string{"com.example.plan"}, 25, tt.after)
			if err != nil {
				t.Fatalf("recordTimelineQuery() error = %v", err)
			}
			var planJSON string
			if err := exec.DB().QueryRowContext(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+sqlStr, exec.ConvertParams(params)...).Scan(&planJSON); err != nil {
				t.Fatalf("EXPLAIN error = %v\nSQL: %s", err, sqlStr)
			}
			if planHasNodeType(t, planJSON, "Sort") {
				t.Fatalf("timeline plan sorts instead of walking an index\nSQL: %s\nplan: %s", sqlStr, planJSON)
			}
			usesWanted := false
			for _, index := range tt.wantIndexes {
				usesWanted = usesWanted || strings.Contains(planJSON, `"Index Name": "`+index+`"`)
			}
			if !usesWanted {
				t.Fatalf("timeline plan does not use any of %v\nSQL: %s\nplan: %s", tt.wantIndexes, sqlStr, planJSON)
			}
			assertIndexScansBounded(t, planJSON, tt.maxVisited)
		})
	}
}

// Index-bound keyset pages must return exactly the rows a full sort would,
// including across the NULL boundary and through rows that share a createdAt.
// Sequential and bitmap scans are disabled so every page goes through the index
// range scans (the row-value bound, and the second statement of a page that
// spans dated and undated rows) rather than whatever the planner would pick for
// a tiny table. The expected order comes from sorting in Go, not from SQL.
func TestRecordCreatedAtPagesMatchFullOrderUnderIndexScansPostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutorWithOptions(t, "-c enable_seqscan=off -c enable_bitmapscan=off")
	repo := NewRecordsRepository(exec)
	ctx := context.Background()
	seedCreatedAtPlanRows(t, exec)
	if _, err := exec.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json, record_created_at, validation_status)
		SELECT 'at://did:plc:tie/com.example.plan/tie' || g, 'cid', 'did:plc:tie', 'com.example.plan', '{}',
			timestamptz '2024-01-05 00:00:00+00', CASE WHEN g % 3 = 0 THEN 'invalid' ELSE 'valid' END
		FROM generate_series(1, 300) AS g;
		UPDATE record SET validation_status = 'invalid' WHERE collection = 'com.example.plan' AND uri LIKE '%7';
		ANALYZE record`); err != nil {
		t.Fatalf("seed ties: %v", err)
	}

	type row struct{ uri, createdAt string }
	var all []row
	rows, err := exec.DB().QueryContext(ctx, `
		SELECT uri, COALESCE(to_char(record_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'), ''), validation_status
		FROM record WHERE collection = 'com.example.plan'`)
	if err != nil {
		t.Fatalf("read rows: %v", err)
	}
	validURIs := map[string]bool{}
	for rows.Next() {
		var r row
		var status string
		if err := rows.Scan(&r.uri, &r.createdAt, &status); err != nil {
			t.Fatal(err)
		}
		all = append(all, r)
		validURIs[r.uri] = status == "valid"
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	_ = rows.Close()

	expected := func(validOnly bool, direction string) []string {
		var picked []row
		for _, r := range all {
			if !validOnly || validURIs[r.uri] {
				picked = append(picked, r)
			}
		}
		// Oldest first: NULL (empty) is the oldest, then createdAt, then uri.
		slices.SortFunc(picked, func(a, b row) int {
			return cmp.Or(strings.Compare(a.createdAt, b.createdAt), strings.Compare(a.uri, b.uri))
		})
		if direction == "DESC" {
			slices.Reverse(picked)
		}
		uris := make([]string, len(picked))
		for i, r := range picked {
			uris[i] = r.uri
		}
		return uris
	}

	// 1001 is the largest page clients ask for; 97 puts page boundaries inside
	// the 300-row group of identical createdAt values.
	for _, pageSize := range []int{1001, 97} {
		for _, validOnly := range []bool{true, false} {
			for _, direction := range []string{"DESC", "ASC"} {
				t.Run(fmt.Sprintf("pageSize=%d/validOnly=%v/%s", pageSize, validOnly, direction), func(t *testing.T) {
					sort := &SortOption{Field: SortFieldRecordCreatedAt, Direction: direction}
					want := expected(validOnly, direction)
					if len(want) < 5*pageSize {
						t.Fatalf("seed produced %d rows, want enough for several pages", len(want))
					}
					forward := func(after []string) ([]*Record, error) {
						if validOnly {
							return repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, "com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, sort, pageSize, after)
						}
						return repo.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, "com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, sort, pageSize, after)
					}
					backward := func(before []string) ([]*Record, error) {
						if validOnly {
							return repo.GetValidByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, "com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, sort, pageSize, before)
						}
						return repo.GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, "com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, sort, pageSize, before)
					}

					var got []string
					var cursor []string
					for pages := 0; ; pages++ {
						page, err := forward(cursor)
						if err != nil {
							t.Fatalf("forward page %d: %v", pages, err)
						}
						if len(page) == 0 {
							break
						}
						for _, rec := range page {
							got = append(got, rec.URI)
						}
						last := page[len(page)-1]
						cursor = []string{last.RecordCreatedAt, last.URI}
						if pages > len(want) {
							t.Fatal("forward pagination did not terminate")
						}
					}
					if !slices.Equal(got, want) {
						t.Fatalf("forward pages differ from the full order: got %d rows, want %d; first difference at %d", len(got), len(want), firstDifference(got, want))
					}

					got, cursor = nil, nil
					for pages := 0; ; pages++ {
						page, err := backward(cursor)
						if err != nil {
							t.Fatalf("backward page %d: %v", pages, err)
						}
						if len(page) == 0 {
							break
						}
						uris := make([]string, len(page))
						for i, rec := range page {
							uris[i] = rec.URI
						}
						got = append(uris, got...)
						cursor = []string{page[0].RecordCreatedAt, page[0].URI}
						if pages > len(want) {
							t.Fatal("backward pagination did not terminate")
						}
					}
					if !slices.Equal(got, want) {
						t.Fatalf("backward pages differ from the full order: got %d rows, want %d; first difference at %d", len(got), len(want), firstDifference(got, want))
					}
				})
			}
		}
	}
}

func firstDifference(a, b []string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

func TestSortedRecordsQueryGenericPlanUsesValidIndexPostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()

	if _, err := exec.DB().ExecContext(ctx, `
		INSERT INTO record (uri, cid, did, collection, json, record_created_at, validation_status)
		SELECT 'at://did:plc:plan/com.example.plan/' || g, 'cid', 'did:plc:plan', 'com.example.plan',
			'{}'::jsonb, CASE WHEN g % 20 = 0 THEN NULL ELSE timestamptz '2024-01-01 00:00:00+00' + g * interval '1 minute' END,
			CASE WHEN g % 2 = 0 THEN 'valid' ELSE 'invalid' END
		FROM generate_series(1, 20000) AS g`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, "ANALYZE record"); err != nil {
		t.Fatalf("ANALYZE: %v", err)
	}

	// pgx caches prepared statements, and PostgreSQL may switch a cached
	// statement to a generic plan that ignores bind values. Force that plan so
	// the test catches a validation_status bind parameter, which would hide the
	// valid-only partial index from the planner.
	conn, err := exec.DB().Conn(ctx)
	if err != nil {
		t.Fatalf("Conn: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, "SET plan_cache_mode = force_generic_plan"); err != nil {
		t.Fatalf("SET plan_cache_mode: %v", err)
	}

	// The cursor cases matter most: a generic plan cannot see the cursor value,
	// so it must rely on the index bound rather than on a favourable estimate.
	desc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "DESC"}
	asc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "ASC"}
	dated := []string{"2024-01-05T00:00:00.000Z", "at://did:plc:plan/com.example.plan/5760"}
	undated := []string{"", "at://did:plc:plan/com.example.plan/6000"}
	tests := []struct {
		name  string
		sort  *SortOption
		after []string
	}{
		{name: "first page", sort: desc},
		{name: "desc after dated cursor", sort: desc, after: dated},
		{name: "desc after undated cursor", sort: desc, after: undated},
		{name: "asc after dated cursor", sort: asc, after: dated},
		{name: "asc after undated cursor", sort: asc, after: undated},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.sortedRecordsPage("com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, tt.sort, tt.after, true)
			if err != nil {
				t.Fatalf("sortedRecordsPage() error = %v", err)
			}
			for segment := range page.segments {
				sqlStr := page.sql(segment, 26)
				// EXPLAIN over the driver's extended protocol plans with the bound
				// values, so use a server-side PREPARE to get the plan a cached
				// statement would use.
				statement := fmt.Sprintf("sorted_plan_%d_%d", i, segment)
				if _, err := conn.ExecContext(ctx, "PREPARE "+statement+" AS "+sqlStr); err != nil {
					t.Fatalf("PREPARE error = %v\nSQL: %s", err, sqlStr)
				}
				literals := make([]string, 0, len(page.segments[segment].args))
				for _, arg := range page.segments[segment].args {
					literals = append(literals, "'"+strings.ReplaceAll(fmt.Sprint(arg), "'", "''")+"'")
				}
				var planJSON string
				if err := conn.QueryRowContext(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) EXECUTE "+statement+"("+strings.Join(literals, ", ")+")").Scan(&planJSON); err != nil {
					t.Fatalf("EXPLAIN EXECUTE error = %v\nSQL: %s", err, sqlStr)
				}
				if !strings.Contains(planJSON, `"Index Name": "idx_record_collection_created_at_valid"`) {
					t.Fatalf("generic plan of segment %d does not use idx_record_collection_created_at_valid\nSQL: %s\nplan: %s", segment, sqlStr, planJSON)
				}
				if planHasNodeType(t, planJSON, "Sort") {
					t.Fatalf("generic plan of segment %d sorts\nSQL: %s\nplan: %s", segment, sqlStr, planJSON)
				}
				assertIndexScansBounded(t, planJSON, 26+1)
			}
		})
	}
}

// A selective predicate on the undated rows must not slow a dated page. The
// undated segment is only read when the dated segment runs out, so a deep DESC
// page that the dated rows fill never touches it, however many undated rows the
// filter would have to walk. When the dated rows do run out, the undated segment
// asks for only the shortfall.
func TestSortedRecordsPageSkipsUndatedSegmentWhenDatedRowsFillPagePostgres(t *testing.T) {
	exec := newCreatedAtPlanPostgresExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()
	seedCreatedAtPlanRows(t, exec)
	// Only dated rows carry a tag, so the filter matches no undated row.
	tagged := []FieldFilter{{Field: "tag", Operator: "isNull", Value: false, FieldType: "string"}}
	desc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "DESC"}
	const limit = 26

	// Deep page: the cursor is ~14000 dated rows from the start of the DESC order.
	deep := []string{"2024-01-05T00:00:00.000Z", "at://did:plc:heavy/com.example.plan/5760"}
	page, err := repo.sortedRecordsPage("com.example.plan", tagged, DIDFilter{}, ExternalLabelFilterSet{}, desc, deep, true)
	if err != nil {
		t.Fatalf("sortedRecordsPage() error = %v", err)
	}
	if len(page.segments) != 2 {
		t.Fatalf("page has %d segments, want the dated segment and the undated one", len(page.segments))
	}

	// The dated segment alone is bounded.
	var datedPlan string
	if err := exec.DB().QueryRowContext(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+page.sql(0, limit), page.segments[0].args...).Scan(&datedPlan); err != nil {
		t.Fatalf("EXPLAIN dated segment: %v", err)
	}
	assertIndexScansBounded(t, datedPlan, limit+1)

	// The undated segment would be expensive: nothing among the undated rows
	// matches, so it walks all of them. This is what a Merge Append over both
	// segments paid on every page.
	var undatedPlan string
	if err := exec.DB().QueryRowContext(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+page.sql(1, limit), page.segments[1].args...).Scan(&undatedPlan); err != nil {
		t.Fatalf("EXPLAIN undated segment: %v", err)
	}
	if got := planRowsRemovedByFilter(t, undatedPlan); got < 500 {
		t.Fatalf("undated segment removed %d rows by filter, want it to walk the ~1000 undated rows; the scenario no longer shows the cost", got)
	}

	// Reading the page runs only the dated segment.
	var ran []int
	records, err := readSegments(limit, len(page.segments), func(segment, remaining int) ([]*Record, error) {
		ran = append(ran, segment)
		return repo.querySegment(ctx, page, segment, remaining, "sorted")
	})
	if err != nil {
		t.Fatalf("readSegments() error = %v", err)
	}
	if len(records) != limit || !slices.Equal(ran, []int{0}) {
		t.Fatalf("read %d rows from segments %v, want %d rows from segment 0 only", len(records), ran, limit)
	}

	// Near the oldest dated row the dated segment runs out, and the undated
	// segment is asked for just the shortfall.
	tail := []string{"2024-01-01T00:10:00.000Z", "at://did:plc:heavy/com.example.plan/10"}
	page, err = repo.sortedRecordsPage("com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, desc, tail, true)
	if err != nil {
		t.Fatalf("sortedRecordsPage(tail) error = %v", err)
	}
	var requested []int
	records, err = readSegments(limit, len(page.segments), func(segment, remaining int) ([]*Record, error) {
		requested = append(requested, segment, remaining)
		return repo.querySegment(ctx, page, segment, remaining, "sorted")
	})
	if err != nil {
		t.Fatalf("readSegments(tail) error = %v", err)
	}
	// Dated rows older than g = 10 are g = 1..9, so 9 rows come from segment 0.
	if len(records) != limit || !slices.Equal(requested, []int{0, limit, 1, limit - 9}) {
		t.Fatalf("read %d rows with (segment, limit) requests %v, want %d rows from (0, %d) then (1, %d)", len(records), requested, limit, limit, limit-9)
	}
}

func planRowsRemovedByFilter(t *testing.T, planJSON string) int {
	t.Helper()
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(planJSON), &plans); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	removed := 0
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if n, ok := node["Rows Removed by Filter"].(float64); ok {
			removed += int(n)
		}
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				walk(m)
			}
		}
	}
	for _, p := range plans {
		walk(p.Plan)
	}
	return removed
}

// assertIndexScansBounded fails unless the plan reads an index and no index
// scan visited more than maxRows rows, counting rows removed by a filter. It
// needs EXPLAIN (ANALYZE, FORMAT JSON) output. A keyset cursor that is only a
// filter makes the scan start at the head of the index and discard every row
// before the cursor, which shows up here as a large "Rows Removed by Filter".
func assertIndexScansBounded(t *testing.T, planJSON string, maxRows int) {
	t.Helper()
	var plans []struct {
		Plan map[string]any `json:"Plan"`
	}
	if err := json.Unmarshal([]byte(planJSON), &plans); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	number := func(node map[string]any, key string) int {
		value, _ := node[key].(float64)
		return int(value)
	}
	indexScans := 0
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if nodeType, _ := node["Node Type"].(string); nodeType == "Index Scan" || nodeType == "Index Only Scan" {
			indexScans++
			visited := number(node, "Actual Rows")*max(number(node, "Actual Loops"), 1) + number(node, "Rows Removed by Filter")
			if visited > maxRows {
				t.Errorf("%s on %v visited %d rows (%d returned, %d removed by filter), want at most %d; the cursor is not an index bound\nplan: %s",
					nodeType, node["Index Name"], visited, number(node, "Actual Rows"), number(node, "Rows Removed by Filter"), maxRows, planJSON)
			}
		}
		children, _ := node["Plans"].([]any)
		for _, child := range children {
			if m, ok := child.(map[string]any); ok {
				walk(m)
			}
		}
	}
	for _, p := range plans {
		walk(p.Plan)
	}
	if indexScans == 0 {
		t.Errorf("plan has no index scan\nplan: %s", planJSON)
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
	return newCreatedAtPlanPostgresExecutorWithOptions(t, "")
}

// newCreatedAtPlanPostgresExecutorWithOptions is newCreatedAtPlanPostgresExecutor
// with PostgreSQL startup options such as "-c enable_seqscan=off" applied to
// every connection of the returned executor.
func newCreatedAtPlanPostgresExecutorWithOptions(t *testing.T, options string) *postgres.Executor {
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
	if options != "" {
		query.Set("options", options)
	}
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
