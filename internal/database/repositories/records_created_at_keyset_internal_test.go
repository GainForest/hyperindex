package repositories

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

func TestReadSegmentsFillsFromLaterSegmentsOnlyWhenShort(t *testing.T) {
	recordsOf := func(n int) []*Record { return make([]*Record, n) }

	tests := []struct {
		name         string
		limit        int
		segmentRows  []int // rows each segment has available
		wantRequests []int // limit requested from each segment that ran, in order
		wantRows     int
	}{
		{name: "first segment fills the page", limit: 26, segmentRows: []int{40, 5}, wantRequests: []int{26}, wantRows: 26},
		{name: "first segment exactly fills the page", limit: 26, segmentRows: []int{26, 5}, wantRequests: []int{26}, wantRows: 26},
		{name: "shortfall comes from the next segment", limit: 26, segmentRows: []int{9, 40}, wantRequests: []int{26, 17}, wantRows: 26},
		{name: "both segments run dry", limit: 26, segmentRows: []int{9, 4}, wantRequests: []int{26, 17}, wantRows: 13},
		{name: "empty first segment", limit: 26, segmentRows: []int{0, 3}, wantRequests: []int{26, 26}, wantRows: 3},
		{name: "single segment", limit: 26, segmentRows: []int{3}, wantRequests: []int{26}, wantRows: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requests []int
			got, err := readSegments(tt.limit, len(tt.segmentRows), func(segment, limit int) ([]*Record, error) {
				requests = append(requests, limit)
				return recordsOf(min(limit, tt.segmentRows[segment])), nil
			})
			if err != nil {
				t.Fatalf("readSegments() error = %v", err)
			}
			if len(got) != tt.wantRows || !slices.Equal(requests, tt.wantRequests) {
				t.Fatalf("readSegments() = %d rows with limit requests %v, want %d rows with %v", len(got), requests, tt.wantRows, tt.wantRequests)
			}
		})
	}

	t.Run("an error stops the read", func(t *testing.T) {
		boom := errors.New("boom")
		calls := 0
		_, err := readSegments(26, 2, func(segment, limit int) ([]*Record, error) {
			calls++
			return nil, boom
		})
		if !errors.Is(err, boom) || calls != 1 {
			t.Fatalf("readSegments() error = %v after %d calls, want boom after 1", err, calls)
		}
	})
}

func newCreatedAtPlanSQLiteExecutor(t *testing.T) *sqlite.Executor {
	t.Helper()
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("sqlite executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })
	if err := migrations.Run(context.Background(), exec); err != nil {
		t.Fatalf("migrations: %v", err)
	}
	return exec
}

// SQLite must bound every segment of a keyset page at the cursor, as PostgreSQL
// does, instead of scanning the collection from the head of the index and
// filtering: a row-value comparison for the dated range and an equality or
// range on record_created_at for the undated one. EXPLAIN QUERY PLAN names the
// bound in the SEARCH line, and a sort shows up as TEMP B-TREE. The rows spread
// over 50 authors; with a single author SQLite may skip-scan the author index
// instead, which is not a case worth pinning.
func TestSortedRecordsPageSegmentsAreBoundedSQLite(t *testing.T) {
	exec := newCreatedAtPlanSQLiteExecutor(t)
	repo := NewRecordsRepository(exec)
	ctx := context.Background()
	if _, err := exec.DB().ExecContext(ctx, `
		WITH RECURSIVE g(n) AS (SELECT 1 UNION ALL SELECT n + 1 FROM g WHERE n < 3000)
		INSERT INTO record (uri, cid, did, collection, json, validation_status, record_created_at)
		SELECT 'at://did:plc:u' || (n % 50) || '/com.example.plan/' || n, 'cid', 'did:plc:u' || (n % 50), 'com.example.plan', '{}', 'valid',
			CASE WHEN n % 20 = 0 THEN NULL ELSE strftime('%Y-%m-%dT%H:%M:%fZ', '2024-01-01', '+' || n || ' minutes') END
		FROM g;
		ANALYZE`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	desc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "DESC"}
	asc := &SortOption{Field: SortFieldRecordCreatedAt, Direction: "ASC"}
	dated := []string{"2024-01-02T00:00:00.000Z", "at://did:plc:u40/com.example.plan/1440"}
	undated := []string{"", "at://did:plc:u0/com.example.plan/1500"}

	tests := []struct {
		name      string
		sort      *SortOption
		after     []string
		validOnly bool
		// wantBounds is, per segment, the bound the SEARCH line must show.
		wantBounds []string
	}{
		{name: "desc after dated cursor", sort: desc, after: dated, wantBounds: []string{"(record_created_at,uri)<(?,?)", "record_created_at=?"}},
		{name: "desc after undated cursor", sort: desc, after: undated, wantBounds: []string{"record_created_at=?"}},
		{name: "asc after dated cursor", sort: asc, after: dated, wantBounds: []string{"(record_created_at,uri)>(?,?)"}},
		{name: "asc after undated cursor", sort: asc, after: undated, wantBounds: []string{"record_created_at=?", "record_created_at>?"}},
		{name: "valid desc after dated cursor", sort: desc, after: dated, validOnly: true, wantBounds: []string{"(record_created_at,uri)<(?,?)", "record_created_at=?"}},
		{name: "valid asc after undated cursor", sort: asc, after: undated, validOnly: true, wantBounds: []string{"record_created_at=?", "record_created_at>?"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, err := repo.sortedRecordsPage("com.example.plan", nil, DIDFilter{}, ExternalLabelFilterSet{}, tt.sort, tt.after, tt.validOnly)
			if err != nil {
				t.Fatalf("sortedRecordsPage() error = %v", err)
			}
			if len(page.segments) != len(tt.wantBounds) {
				t.Fatalf("page has %d segments, want %d", len(page.segments), len(tt.wantBounds))
			}
			for i := range page.segments {
				rows, err := exec.DB().QueryContext(ctx, "EXPLAIN QUERY PLAN "+page.sql(i, 26), page.segments[i].args...)
				if err != nil {
					t.Fatalf("segment %d EXPLAIN QUERY PLAN: %v", i, err)
				}
				var plan []string
				for rows.Next() {
					var id, parent, unused int
					var detail string
					if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
						t.Fatalf("scan plan: %v", err)
					}
					plan = append(plan, detail)
				}
				if err := rows.Err(); err != nil {
					t.Fatalf("read plan: %v", err)
				}
				_ = rows.Close()
				text := strings.Join(plan, "\n")
				if !strings.Contains(text, tt.wantBounds[i]) {
					t.Errorf("segment %d plan does not bound the scan with %s:\n%s", i, tt.wantBounds[i], text)
				}
				if strings.Contains(text, "TEMP B-TREE") {
					t.Errorf("segment %d plan sorts instead of walking the index:\n%s", i, text)
				}
				wantIndex := "idx_record_collection_created_at"
				if tt.validOnly {
					wantIndex += "_valid"
				}
				if !strings.Contains(text, wantIndex) {
					t.Errorf("segment %d plan does not use %s:\n%s", i, wantIndex, text)
				}
			}
		})
	}
}
