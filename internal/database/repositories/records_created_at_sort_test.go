package repositories_test

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

const createdAtSortCollection = "com.example.createdat"

// createdAtSortRepositories returns a SQLite repository and, when DATABASE_URL
// points at a safe PostgreSQL test database, an isolated PostgreSQL one.
func createdAtSortRepositories(t *testing.T) map[string]*repositories.RecordsRepository {
	t.Helper()
	repos := map[string]*repositories.RecordsRepository{"sqlite": setupRecordsTest(t)}
	if _, ok := safePostgresRecordsTestDatabaseURL(t); ok {
		repos["postgres"] = repositories.NewRecordsRepository(newPostgresRecordsTestExecutor(t))
	}
	return repos
}

// createdAtSortEnv is a records repository with the external-label repository
// that shares its database.
type createdAtSortEnv struct {
	records *repositories.RecordsRepository
	labels  *repositories.ExternalLabelsRepository
}

// createdAtSortEnvs is createdAtSortRepositories plus the label repository.
func createdAtSortEnvs(t *testing.T) map[string]createdAtSortEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	envs := map[string]createdAtSortEnv{"sqlite": {records: db.Records, labels: db.ExternalLabels}}
	if _, ok := safePostgresRecordsTestDatabaseURL(t); ok {
		exec := newPostgresRecordsTestExecutor(t)
		envs["postgres"] = createdAtSortEnv{records: repositories.NewRecordsRepository(exec), labels: repositories.NewExternalLabelsRepository(exec)}
	}
	return envs
}

// seedCreatedAtSortRecords inserts records whose createdAt values cover
// timezone offsets, sub-millisecond precision, a same-instant tie, an edit that
// changes createdAt, an invalid row, and two rows with no record_created_at: one
// without createdAt and one whose createdAt is not a datetime.
func seedCreatedAtSortRecords(t *testing.T, repo *repositories.RecordsRepository) map[string]string {
	t.Helper()
	ctx := context.Background()
	uris := map[string]string{
		"newest":    "at://did:plc:a/" + createdAtSortCollection + "/newest",
		"tieA":      "at://did:plc:a/" + createdAtSortCollection + "/tie-a",
		"tieB":      "at://did:plc:a/" + createdAtSortCollection + "/tie-b",
		"edited":    "at://did:plc:a/" + createdAtSortCollection + "/edited",
		"invalid":   "at://did:plc:a/" + createdAtSortCollection + "/invalid",
		"noCreate":  "at://did:plc:a/" + createdAtSortCollection + "/no-created-at",
		"malformed": "at://did:plc:a/" + createdAtSortCollection + "/malformed",
	}
	writes := []struct {
		uri, cid, json string
		status         validation.Status
	}{
		{uris["newest"], "c1", `{"createdAt":"2026-01-03T00:00:00.123456Z"}`, validation.StatusValid},
		{uris["tieA"], "c1", `{"createdAt":"2026-01-02T12:00:00+02:00"}`, validation.StatusValid},
		{uris["tieB"], "c1", `{"createdAt":"2026-01-02T10:00:00Z"}`, validation.StatusValid},
		{uris["edited"], "c1", `{"createdAt":"2026-01-01T00:00:00Z"}`, validation.StatusValid},
		{uris["edited"], "c2", `{"createdAt":"2026-01-09T00:00:00Z"}`, validation.StatusValid},
		{uris["invalid"], "c1", `{"createdAt":"2026-01-08T00:00:00Z"}`, validation.StatusInvalid},
		{uris["noCreate"], "c1", `{"name":"x"}`, validation.StatusValid},
		{uris["malformed"], "c1", `{"createdAt":"not-a-date"}`, validation.StatusValid},
	}
	for _, w := range writes {
		if _, err := repo.Insert(ctx, w.uri, w.cid, "did:plc:a", createdAtSortCollection, w.json); err != nil {
			t.Fatalf("Insert(%s) error = %v", w.uri, err)
		}
		if err := repo.UpdateValidationStatus(ctx, w.uri, w.status, "", "hash"); err != nil {
			t.Fatalf("UpdateValidationStatus(%s) error = %v", w.uri, err)
		}
	}
	return uris
}

type createdAtSortReader struct {
	repo      *repositories.RecordsRepository
	validOnly bool
	filters   []repositories.FieldFilter
	didFilter repositories.DIDFilter
	labels    repositories.ExternalLabelFilterSet
	sort      *repositories.SortOption
}

func (r createdAtSortReader) forward(t *testing.T, limit int, after []string) []*repositories.Record {
	t.Helper()
	ctx := context.Background()
	get := r.repo.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters
	if r.validOnly {
		get = r.repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters
	}
	records, err := get(ctx, createdAtSortCollection, r.filters, r.didFilter, r.labels, r.sort, limit, after)
	if err != nil {
		t.Fatalf("forward page error = %v", err)
	}
	// A page that crosses the dated/undated boundary asks the second statement
	// for only the shortfall; passing the full limit would return up to twice
	// the rows while still looking ordered.
	if len(records) > limit {
		t.Fatalf("forward page returned %d rows, want at most %d", len(records), limit)
	}
	return records
}

func (r createdAtSortReader) backward(t *testing.T, limit int, before []string) []*repositories.Record {
	t.Helper()
	ctx := context.Background()
	get := r.repo.GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters
	if r.validOnly {
		get = r.repo.GetValidByCollectionReversedWithKeysetCursorAndExternalLabelFilters
	}
	records, err := get(ctx, createdAtSortCollection, r.filters, r.didFilter, r.labels, r.sort, limit, before)
	if err != nil {
		t.Fatalf("backward page error = %v", err)
	}
	// A page that crosses the dated/undated boundary asks the second statement
	// for only the shortfall; passing the full limit would return up to twice
	// the rows while still looking ordered.
	if len(records) > limit {
		t.Fatalf("backward page returned %d rows, want at most %d", len(records), limit)
	}
	return records
}

// pagesForward follows end cursors the way a Relay client does.
func (r createdAtSortReader) pagesForward(t *testing.T, pageSize int) []string {
	t.Helper()
	var got, cursor []string
	for i := 0; i < 50; i++ {
		page := r.forward(t, pageSize, cursor)
		if len(page) == 0 {
			return got
		}
		got = append(got, recordURIs(page)...)
		last := page[len(page)-1]
		cursor = []string{last.RecordCreatedAt, last.URI}
	}
	t.Fatal("forward pagination did not terminate")
	return nil
}

// pagesBackward walks from the end of the connection with last/before and
// returns the records in connection order.
func (r createdAtSortReader) pagesBackward(t *testing.T, pageSize int) []string {
	t.Helper()
	var got, before []string
	for i := 0; i < 50; i++ {
		page := r.backward(t, pageSize, before)
		if len(page) == 0 {
			return got
		}
		got = append(recordURIs(page), got...)
		first := page[0]
		before = []string{first.RecordCreatedAt, first.URI}
	}
	t.Fatal("backward pagination did not terminate")
	return nil
}

// Undated rows are kept and sort as the oldest: last when newest-first, first
// when oldest-first, ordered among themselves by uri in the sort direction.
// Paging with every page size puts the page boundary on each side of the NULL
// boundary, so a cursor sits on a dated row, on an undated row, and on the last
// dated row before the undated ones.
func TestRecordsRepository_SortByRecordCreatedAtColumn(t *testing.T) {
	for dialect, repo := range createdAtSortRepositories(t) {
		t.Run(dialect, func(t *testing.T) {
			uris := seedCreatedAtSortRecords(t, repo)

			// Edited keeps its first-seen createdAt (Jan 1) even though its JSON now
			// says Jan 9. tie-a and tie-b are the same instant, split by uri.
			tests := []struct {
				name      string
				validOnly bool
				direction string
				want      []string
			}{
				{"valid DESC", true, "DESC", []string{uris["newest"], uris["tieB"], uris["tieA"], uris["edited"], uris["noCreate"], uris["malformed"]}},
				{"valid ASC", true, "ASC", []string{uris["malformed"], uris["noCreate"], uris["edited"], uris["tieA"], uris["tieB"], uris["newest"]}},
				{"unvalidated DESC", false, "DESC", []string{uris["invalid"], uris["newest"], uris["tieB"], uris["tieA"], uris["edited"], uris["noCreate"], uris["malformed"]}},
				{"unvalidated ASC", false, "ASC", []string{uris["malformed"], uris["noCreate"], uris["edited"], uris["tieA"], uris["tieB"], uris["newest"], uris["invalid"]}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					reader := createdAtSortReader{
						repo:      repo,
						validOnly: tt.validOnly,
						sort:      &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: tt.direction},
					}
					assertURIOrder(t, "first page", recordURIs(reader.forward(t, len(tt.want)+5, nil)), tt.want)
					for pageSize := 1; pageSize <= 4; pageSize++ {
						assertURIOrder(t, fmt.Sprintf("forward pages of %d", pageSize), reader.pagesForward(t, pageSize), tt.want)
						assertURIOrder(t, fmt.Sprintf("backward pages of %d", pageSize), reader.pagesBackward(t, pageSize), tt.want)
					}
				})
			}
		})
	}
}

// The cursor value must round-trip: normalized UTC milliseconds for dated rows,
// and empty for rows without record_created_at.
func TestRecordsRepository_RecordCreatedAtSortCursorValues(t *testing.T) {
	for dialect, repo := range createdAtSortRepositories(t) {
		t.Run(dialect, func(t *testing.T) {
			uris := seedCreatedAtSortRecords(t, repo)
			reader := createdAtSortReader{repo: repo, validOnly: true, sort: &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: "DESC"}}

			got := map[string]string{}
			for _, rec := range reader.forward(t, 10, nil) {
				got[rec.URI] = rec.RecordCreatedAt
			}
			want := map[string]string{
				uris["newest"]:    "2026-01-03T00:00:00.123Z",
				uris["tieA"]:      "2026-01-02T10:00:00.000Z",
				uris["edited"]:    "2026-01-01T00:00:00.000Z",
				uris["noCreate"]:  "",
				uris["malformed"]: "",
			}
			for uri, wantValue := range want {
				if gotValue, ok := got[uri]; !ok || gotValue != wantValue {
					t.Errorf("RecordCreatedAt(%s) = %q (present=%v), want %q", uri, gotValue, ok, wantValue)
				}
			}
		})
	}
}

// A DID filter composes with the NULL-aware keyset condition: each author's
// undated rows stay in their own list, after that author's dated rows.
func TestRecordsRepository_RecordCreatedAtSortWithDIDFilter(t *testing.T) {
	for dialect, repo := range createdAtSortRepositories(t) {
		t.Run(dialect, func(t *testing.T) {
			ctx := context.Background()
			for _, w := range []struct{ did, rkey, json string }{
				{"did:plc:a", "a-new", `{"createdAt":"2026-02-02T00:00:00Z"}`},
				{"did:plc:a", "a-none", `{"name":"a"}`},
				{"did:plc:b", "b-new", `{"createdAt":"2026-02-03T00:00:00Z"}`},
				{"did:plc:b", "b-none", `{"name":"b"}`},
				{"did:plc:a", "a-old", `{"createdAt":"2026-02-01T00:00:00Z"}`},
			} {
				uri := "at://" + w.did + "/" + createdAtSortCollection + "/" + w.rkey
				if _, err := repo.Insert(ctx, uri, "c1", w.did, createdAtSortCollection, w.json); err != nil {
					t.Fatalf("Insert(%s) error = %v", uri, err)
				}
			}
			uri := func(did, rkey string) string { return "at://" + did + "/" + createdAtSortCollection + "/" + rkey }

			reader := createdAtSortReader{
				repo:      repo,
				didFilter: repositories.DIDFilter{EQ: "did:plc:a"},
				sort:      &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: "DESC"},
			}
			want := []string{uri("did:plc:a", "a-new"), uri("did:plc:a", "a-old"), uri("did:plc:a", "a-none")}
			assertURIOrder(t, "forward pages of 1", reader.pagesForward(t, 1), want)
			assertURIOrder(t, "backward pages of 1", reader.pagesBackward(t, 1), want)
		})
	}
}

// filterRecord is one row of the filter-combination data set: its author, the
// JSON kind a filter can test, the normalized createdAt ("" when the record has
// none or an unparseable one) and the external labels it carries.
type filterRecord struct {
	rkey, did, kind, createdAtJSON, createdAt string
	labels                                    []string
}

// Typed connections sorted by createdAt combine the sort with JSON filters, DID
// lists and external-label filters. Each of those adds placeholders that the
// cursor placeholders must be numbered after, and each segment of a page that
// spans dated and undated rows repeats them. This checks every combination with
// dated and undated cursors in both directions: pages of 1 to 3 rows put a
// cursor on every row and make the second segment ask for a shortfall. The
// expected order is computed in Go from the data, not from SQL.
func TestRecordsRepository_RecordCreatedAtSortCombinesWithFilters(t *testing.T) {
	const (
		didA = "did:plc:a"
		didB = "did:plc:b"
		didC = "did:plc:c"
	)
	data := []filterRecord{
		{"d1", didA, "keep", `"2026-03-01T00:00:00Z"`, "2026-03-01T00:00:00.000Z", []string{"good"}},
		{"d2", didB, "keep", `"2026-03-02T00:00:00Z"`, "2026-03-02T00:00:00.000Z", []string{"good"}},
		{"d3", didC, "skip", `"2026-03-03T00:00:00Z"`, "2026-03-03T00:00:00.000Z", nil},
		{"d4", didA, "keep", `"2026-03-04T00:00:00Z"`, "2026-03-04T00:00:00.000Z", []string{"good", "spam"}},
		{"d5", didB, "keep", `"2026-03-05T00:00:00Z"`, "2026-03-05T00:00:00.000Z", nil},
		{"d6", didA, "skip", `"2026-03-06T00:00:00Z"`, "2026-03-06T00:00:00.000Z", []string{"good"}},
		{"n1", didA, "keep", ``, "", []string{"good"}},
		{"n2", didB, "keep", `"not-a-date"`, "", nil},
		{"n3", didC, "skip", ``, "", nil},
		{"n4", didA, "keep", ``, "", []string{"good"}},
		{"n5", didB, "skip", ``, "", []string{"good"}},
		{"n6", didC, "keep", `"bad"`, "", []string{"good", "spam"}},
	}
	const labelSource = "did:plc:labeler"

	hasLabel := func(rec filterRecord, val string) bool { return slices.Contains(rec.labels, val) }
	labelFilter := repositories.ExternalLabelFilterSet{Record: repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "good"}},
			ActiveOnly: true,
		},
		None: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "spam"}},
			ActiveOnly: true,
		},
	}}
	keepFilter := []repositories.FieldFilter{{Field: "kind", Operator: "eq", Value: "keep"}}

	cases := []struct {
		name    string
		filters []repositories.FieldFilter
		did     repositories.DIDFilter
		labels  repositories.ExternalLabelFilterSet
		match   func(filterRecord) bool
	}{
		{
			"JSON filter", keepFilter,
			repositories.DIDFilter{},
			repositories.ExternalLabelFilterSet{},
			func(r filterRecord) bool { return r.kind == "keep" },
		},
		{
			"external label has and none", nil,
			repositories.DIDFilter{},
			labelFilter,
			func(r filterRecord) bool { return hasLabel(r, "good") && !hasLabel(r, "spam") },
		},
		{
			"DID IN list", nil,
			repositories.DIDFilter{IN: []string{didA, didB}},
			repositories.ExternalLabelFilterSet{},
			func(r filterRecord) bool { return r.did == didA || r.did == didB },
		},
		{
			"JSON filter, DID IN list and labels together", keepFilter,
			repositories.DIDFilter{IN: []string{didA, didB, didC}},
			labelFilter,
			func(r filterRecord) bool {
				return r.kind == "keep" && hasLabel(r, "good") && !hasLabel(r, "spam")
			},
		},
	}

	for dialect, env := range createdAtSortEnvs(t) {
		t.Run(dialect, func(t *testing.T) {
			ctx := context.Background()
			uriOf := func(rec filterRecord) string {
				return "at://" + rec.did + "/" + createdAtSortCollection + "/" + rec.rkey
			}
			var inputs []repositories.ExternalLabelInput
			for _, rec := range data {
				body := `{"kind":"` + rec.kind + `"`
				if rec.createdAtJSON != "" {
					body += `,"createdAt":` + rec.createdAtJSON
				}
				body += `}`
				if _, err := env.records.Insert(ctx, uriOf(rec), "cid-"+rec.rkey, rec.did, createdAtSortCollection, body); err != nil {
					t.Fatalf("Insert(%s) error = %v", rec.rkey, err)
				}
				if err := env.records.UpdateValidationStatus(ctx, uriOf(rec), validation.StatusValid, "", "hash"); err != nil {
					t.Fatalf("UpdateValidationStatus(%s) error = %v", rec.rkey, err)
				}
				for _, val := range rec.labels {
					inputs = append(inputs, repositories.ExternalLabelInput{
						LabelIndex: int64(len(inputs)), Src: labelSource, URI: uriOf(rec), Val: val, Cts: "2026-03-10T00:00:00Z", RawJSON: `{}`,
					})
				}
			}
			if err := env.labels.PersistEvent(ctx, "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels", 1, inputs); err != nil {
				t.Fatalf("PersistEvent() error = %v", err)
			}

			expected := func(match func(filterRecord) bool, direction string) []string {
				var picked []filterRecord
				for _, rec := range data {
					if match(rec) {
						picked = append(picked, rec)
					}
				}
				// Oldest first: undated rows (empty createdAt) are the oldest.
				slices.SortFunc(picked, func(a, b filterRecord) int {
					return cmp.Or(strings.Compare(a.createdAt, b.createdAt), strings.Compare(uriOf(a), uriOf(b)))
				})
				if direction == "DESC" {
					slices.Reverse(picked)
				}
				uris := make([]string, len(picked))
				for i, rec := range picked {
					uris[i] = uriOf(rec)
				}
				return uris
			}

			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					var dated, undated int
					for _, rec := range data {
						if tc.match(rec) {
							if rec.createdAt == "" {
								undated++
							} else {
								dated++
							}
						}
					}
					if dated < 2 || undated < 2 {
						t.Fatalf("case matches %d dated and %d undated rows; it must span the boundary", dated, undated)
					}

					for _, validOnly := range []bool{false, true} {
						for _, direction := range []string{"DESC", "ASC"} {
							reader := createdAtSortReader{
								repo: env.records, validOnly: validOnly,
								filters: tc.filters, didFilter: tc.did, labels: tc.labels,
								sort: &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: direction},
							}
							want := expected(tc.match, direction)
							for pageSize := 1; pageSize <= 3; pageSize++ {
								label := fmt.Sprintf("validOnly=%v %s pages of %d", validOnly, direction, pageSize)
								assertURIOrder(t, "forward "+label, reader.pagesForward(t, pageSize), want)
								assertURIOrder(t, "backward "+label, reader.pagesBackward(t, pageSize), want)
							}
						}
					}
				})
			}
		})
	}
}

func TestRecordsRepository_RecordCreatedAtColumnFilters(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	uris := seedCreatedAtSortRecords(t, repo)
	desc := &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: "DESC"}
	column := func(op string, value interface{}) repositories.FieldFilter {
		return repositories.FieldFilter{Field: repositories.SortFieldRecordCreatedAt, Operator: op, Value: value, FieldType: "datetime", Target: repositories.FieldFilterTargetColumn}
	}

	tests := []struct {
		name    string
		filters []repositories.FieldFilter
		want    []string
	}{
		{"range", []repositories.FieldFilter{column("gte", "2026-01-02T10:00:00.000Z"), column("lt", "2026-01-03T00:00:00.000Z")}, []string{uris["tieB"], uris["tieA"]}},
		{"eq", []repositories.FieldFilter{column("eq", "2026-01-03T00:00:00.123Z")}, []string{uris["newest"]}},
		{"in", []repositories.FieldFilter{column("in", []interface{}{"2026-01-01T00:00:00.000Z", "2026-01-03T00:00:00.123Z"})}, []string{uris["newest"], uris["edited"]}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			records, err := repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, createdAtSortCollection, tt.filters, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, desc, 10, nil)
			if err != nil {
				t.Fatalf("query error = %v", err)
			}
			assertURIOrder(t, tt.name, recordURIs(records), tt.want)

			count, err := repo.GetValidCollectionCountFilteredWithExternalLabelFilters(ctx, createdAtSortCollection, tt.filters, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{})
			if err != nil {
				t.Fatalf("count error = %v", err)
			}
			if int(count) != len(tt.want) {
				t.Fatalf("count = %d, want %d", count, len(tt.want))
			}
		})
	}

	_, err := repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, createdAtSortCollection, []repositories.FieldFilter{column("contains", "2026")}, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, desc, 10, nil)
	if err == nil {
		t.Fatal("contains on record_created_at succeeded, want unsupported operator error")
	}
}

func assertURIOrder(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}
