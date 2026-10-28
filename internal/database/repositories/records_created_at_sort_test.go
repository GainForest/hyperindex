package repositories_test

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/validation"
)

const createdAtSortCollection = "com.example.createdat"

// seedCreatedAtSortRecords inserts valid records whose createdAt values cover
// timezone offsets, sub-millisecond precision, a same-instant tie, an edit that
// changes createdAt, an invalid row and a valid row without createdAt.
func seedCreatedAtSortRecords(t *testing.T, repo *repositories.RecordsRepository) map[string]string {
	t.Helper()
	ctx := context.Background()
	uris := map[string]string{
		"newest":   "at://did:plc:a/" + createdAtSortCollection + "/newest",
		"tieA":     "at://did:plc:a/" + createdAtSortCollection + "/tie-a",
		"tieB":     "at://did:plc:a/" + createdAtSortCollection + "/tie-b",
		"edited":   "at://did:plc:a/" + createdAtSortCollection + "/edited",
		"invalid":  "at://did:plc:a/" + createdAtSortCollection + "/invalid",
		"noCreate": "at://did:plc:a/" + createdAtSortCollection + "/no-created-at",
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

func TestRecordsRepository_SortByRecordCreatedAtColumn(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	uris := seedCreatedAtSortRecords(t, repo)
	desc := &repositories.SortOption{Field: repositories.SortFieldRecordCreatedAt, Direction: "DESC"}

	// Edited keeps its first-seen createdAt (Jan 1), so it sorts last even
	// though its JSON now says Jan 9. Same-instant rows tie-break by uri DESC.
	// The row without createdAt and the invalid row are excluded.
	wantOrder := []string{uris["newest"], uris["tieB"], uris["tieA"], uris["edited"]}

	var got []string
	var cursor []string
	for page := 0; page < 5; page++ {
		records, err := repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, createdAtSortCollection, nil, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, desc, 2, cursor)
		if err != nil {
			t.Fatalf("sorted page %d error = %v", page, err)
		}
		if len(records) == 0 {
			break
		}
		for _, rec := range records {
			if rec.RecordCreatedAt == "" {
				t.Fatalf("record %s has empty RecordCreatedAt; cursors would be unusable", rec.URI)
			}
			got = append(got, rec.URI)
		}
		last := records[len(records)-1]
		cursor = []string{last.RecordCreatedAt, last.URI}
	}
	assertURIOrder(t, "forward DESC pages", got, wantOrder)

	first, err := repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, createdAtSortCollection, nil, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, desc, 1, nil)
	if err != nil {
		t.Fatalf("first record error = %v", err)
	}
	if first[0].RecordCreatedAt != "2026-01-03T00:00:00.123Z" {
		t.Fatalf("RecordCreatedAt = %q, want normalized UTC millisecond value", first[0].RecordCreatedAt)
	}

	// Backward pagination: last 2 before the tieA cursor.
	before := []string{"2026-01-02T10:00:00.000Z", uris["tieA"]}
	records, err := repo.GetValidByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, createdAtSortCollection, nil, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, desc, 2, before)
	if err != nil {
		t.Fatalf("reversed error = %v", err)
	}
	assertURIOrder(t, "backward page", recordURIs(records), []string{uris["newest"], uris["tieB"]})
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
