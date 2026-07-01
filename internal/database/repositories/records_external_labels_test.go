package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

const externalLabelRecordFilterCollection = "app.example.labelled"

func TestRecordsRepositoryExternalLabelFilterHasNoneAndSource(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	recordA := insertExternalLabelFilterRecord(t, db, "a", "cid-a", "did:plc:alice", "2025-01-02T03:04:01Z")
	recordB := insertExternalLabelFilterRecord(t, db, "b", "cid-b", "did:plc:bob", "2025-01-02T03:04:02Z")
	recordC := insertExternalLabelFilterRecord(t, db, "c", "cid-c", "did:plc:carol", "2025-01-02T03:04:03Z")
	recordD := insertExternalLabelFilterRecord(t, db, "d", "cid-d", "did:plc:dana", "2025-01-02T03:04:04Z")

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler-a", URI: recordA, Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler-b", URI: recordB, Val: "high-quality", Cts: "2025-01-02T03:05:02Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler-a", URI: recordC, Val: "spam", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
	})

	highQuality := repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
			ActiveOnly: true,
		},
	}
	records, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, highQuality, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabels(has high-quality) error = %v", err)
	}
	assertRecordURISet(t, records, []string{recordA, recordB})

	labelerAHighQuality := repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Sources:    []repositories.ExternalLabelStringFilter{{Operator: "in", Value: []interface{}{"did:plc:labeler-a"}}},
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
			ActiveOnly: true,
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, labelerAHighQuality, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabels(has source+value) error = %v", err)
	}
	assertRecordURISet(t, records, []string{recordA})

	noSpam := repositories.ExternalLabelRecordFilter{
		None: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "spam"}},
			ActiveOnly: true,
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, noSpam, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabels(none spam) error = %v", err)
	}
	assertRecordURISet(t, records, []string{recordA, recordB, recordD})

	count, err := db.Records.GetCollectionCountFilteredWithExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, highQuality)
	if err != nil {
		t.Fatalf("GetCollectionCountFilteredWithExternalLabels() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("filtered count = %d, want 2", count)
	}
}

func TestRecordsRepositoryExternalLabelFilterActiveSemantics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	activeRecord := insertExternalLabelFilterRecord(t, db, "active", "cid-active", "did:plc:active", "2025-01-02T03:04:01Z")
	negatedRecord := insertExternalLabelFilterRecord(t, db, "negated", "cid-negated", "did:plc:negated", "2025-01-02T03:04:02Z")
	expiredRecord := insertExternalLabelFilterRecord(t, db, "expired", "cid-expired", "did:plc:expired", "2025-01-02T03:04:03Z")
	cidMismatchRecord := insertExternalLabelFilterRecord(t, db, "cid-mismatch", "cid-current", "did:plc:cid", "2025-01-02T03:04:04Z")
	expiredAt := "2000-01-02T03:04:05Z"
	wrongCID := "cid-old"

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: activeRecord, Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler", URI: negatedRecord, Val: "high-quality", Cts: "2025-01-02T03:05:02Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler", URI: expiredRecord, Val: "high-quality", Cts: "2025-01-02T03:05:03Z", Exp: &expiredAt, RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:labeler", URI: cidMismatchRecord, CID: &wrongCID, Val: "high-quality", Cts: "2025-01-02T03:05:04Z", RawJSON: `{}`},
	})
	persistExternalLabelEvent(t, db.ExternalLabels, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: negatedRecord, Val: "high-quality", Neg: true, Cts: "2025-01-02T03:06:02Z", RawJSON: `{}`},
	})

	activeOnly := repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
			ActiveOnly: true,
		},
	}
	records, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, activeOnly, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabels(active) error = %v", err)
	}
	assertRecordURISet(t, records, []string{activeRecord})

	historical := repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
			ActiveOnly: false,
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, historical, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabels(history) error = %v", err)
	}
	assertRecordURISet(t, records, []string{activeRecord, negatedRecord, expiredRecord})
}

func TestRecordsRepositoryExternalLabelFilterPaginationAppliesBeforeLimit(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	recordOne := insertExternalLabelFilterRecord(t, db, "one", "cid-one", "did:plc:one", "2025-01-02T03:04:01Z")
	insertExternalLabelFilterRecord(t, db, "two", "cid-two", "did:plc:two", "2025-01-02T03:04:02Z")
	recordThree := insertExternalLabelFilterRecord(t, db, "three", "cid-three", "did:plc:three", "2025-01-02T03:04:03Z")
	insertExternalLabelFilterRecord(t, db, "four", "cid-four", "did:plc:four", "2025-01-02T03:04:04Z")
	recordFive := insertExternalLabelFilterRecord(t, db, "five", "cid-five", "did:plc:five", "2025-01-02T03:04:05Z")

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:labeler", URI: recordOne, Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:labeler", URI: recordThree, Val: "high-quality", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:labeler", URI: recordFive, Val: "high-quality", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	})

	filter := repositories.ExternalLabelRecordFilter{
		Has: &repositories.ExternalLabelPredicate{
			Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
			ActiveOnly: true,
		},
	}
	pageOne, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 2, nil)
	if err != nil {
		t.Fatalf("page one query error = %v", err)
	}
	assertRecordURIs(t, pageOne, []string{recordFive, recordThree})

	cursor := []string{pageOne[1].IndexedAt.UTC().Format(time.RFC3339Nano), pageOne[1].URI}
	pageTwo, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabels(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 2, cursor)
	if err != nil {
		t.Fatalf("page two query error = %v", err)
	}
	assertRecordURIs(t, pageTwo, []string{recordOne})
}

func TestRecordsRepositoryAuthorLabelFilterHasNoneAndSubjectSeparation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	aliceRecord := insertExternalLabelFilterRecord(t, db, "alice", "cid-alice", "did:plc:alice", "2025-01-02T03:04:01Z")
	bobRecord := insertExternalLabelFilterRecord(t, db, "bob", "cid-bob", "did:plc:bob", "2025-01-02T03:04:02Z")
	caraRecord := insertExternalLabelFilterRecord(t, db, "cara", "cid-cara", "did:plc:cara", "2025-01-02T03:04:03Z")
	danaRecord := insertExternalLabelFilterRecord(t, db, "dana", "cid-dana", "did:plc:dana", "2025-01-02T03:04:04Z")
	erinRecord := insertExternalLabelFilterRecord(t, db, "erin", "cid-erin", "did:plc:erin", "2025-01-02T03:04:05Z")
	cidSpecificAccountLabel := "account-version-cid"

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:orglabeler", URI: "did:plc:alice", Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:orglabeler", URI: "did:plc:bob", Val: "likely-test", Cts: "2025-01-02T03:05:02Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:orglabeler", URI: danaRecord, Val: "high-quality", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:orglabeler", URI: "did:plc:erin", CID: &cidSpecificAccountLabel, Val: "high-quality", Cts: "2025-01-02T03:05:04Z", RawJSON: `{}`},
	})

	authorHighQuality := repositories.ExternalLabelFilterSet{
		Author: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
				ActiveOnly: true,
			},
		},
	}
	records, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, authorHighQuality, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(author has high-quality) error = %v", err)
	}
	assertRecordURISet(t, records, []string{aliceRecord})

	noLikelyTestAuthor := repositories.ExternalLabelFilterSet{
		Author: repositories.ExternalLabelRecordFilter{
			None: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "likely-test"}},
				ActiveOnly: true,
			},
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, noLikelyTestAuthor, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(author none likely-test) error = %v", err)
	}
	assertRecordURISet(t, records, []string{aliceRecord, caraRecord, danaRecord, erinRecord})
	if containsRecordURI(records, bobRecord) {
		t.Fatalf("likely-test author record %s should have been excluded", bobRecord)
	}

	recordHighQuality := repositories.ExternalLabelFilterSet{
		Record: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
				ActiveOnly: true,
			},
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, recordHighQuality, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(record has high-quality) error = %v", err)
	}
	assertRecordURISet(t, records, []string{danaRecord})
}

func TestRecordsRepositoryAuthorLabelFilterCombinesWithRecordLabels(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	aliceRecord := insertExternalLabelFilterRecord(t, db, "combined-alice", "cid-combined-alice", "did:plc:combined-alice", "2025-01-02T03:04:01Z")
	bobRecord := insertExternalLabelFilterRecord(t, db, "combined-bob", "cid-combined-bob", "did:plc:combined-bob", "2025-01-02T03:04:02Z")
	insertExternalLabelFilterRecord(t, db, "combined-cara", "cid-combined-cara", "did:plc:combined-cara", "2025-01-02T03:04:03Z")

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:activity-labeler", URI: aliceRecord, Val: "verified-impact", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:activity-labeler", URI: bobRecord, Val: "verified-impact", Cts: "2025-01-02T03:05:02Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:orglabeler", URI: "did:plc:combined-bob", Val: "likely-test", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
	})

	filter := repositories.ExternalLabelFilterSet{
		Record: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Sources:    []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "did:plc:activity-labeler"}},
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "verified-impact"}},
				ActiveOnly: true,
			},
		},
		Author: repositories.ExternalLabelRecordFilter{
			None: &repositories.ExternalLabelPredicate{
				Sources:    []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "did:plc:orglabeler"}},
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "likely-test"}},
				ActiveOnly: true,
			},
		},
	}
	records, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(combined) error = %v", err)
	}
	assertRecordURISet(t, records, []string{aliceRecord})
}

func TestRecordsRepositoryAuthorLabelFilterActiveSemantics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	activeRecord := insertExternalLabelFilterRecord(t, db, "author-active", "cid-author-active", "did:plc:author-active", "2025-01-02T03:04:01Z")
	negatedRecord := insertExternalLabelFilterRecord(t, db, "author-negated", "cid-author-negated", "did:plc:author-negated", "2025-01-02T03:04:02Z")
	expiredRecord := insertExternalLabelFilterRecord(t, db, "author-expired", "cid-author-expired", "did:plc:author-expired", "2025-01-02T03:04:03Z")
	cidSpecificRecord := insertExternalLabelFilterRecord(t, db, "author-cid-specific", "cid-author-current", "did:plc:author-cid-specific", "2025-01-02T03:04:04Z")
	expiredAt := "2000-01-02T03:04:05Z"
	accountCID := "account-label-cid"

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:orglabeler", URI: "did:plc:author-active", Val: "standard", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:orglabeler", URI: "did:plc:author-negated", Val: "standard", Cts: "2025-01-02T03:05:02Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:orglabeler", URI: "did:plc:author-expired", Val: "standard", Cts: "2025-01-02T03:05:03Z", Exp: &expiredAt, RawJSON: `{}`},
		{LabelIndex: 3, Src: "did:plc:orglabeler", URI: "did:plc:author-cid-specific", CID: &accountCID, Val: "standard", Cts: "2025-01-02T03:05:04Z", RawJSON: `{}`},
	})
	persistExternalLabelEvent(t, db.ExternalLabels, 2, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:orglabeler", URI: "did:plc:author-negated", Val: "standard", Neg: true, Cts: "2025-01-02T03:06:02Z", RawJSON: `{}`},
	})

	activeOnly := repositories.ExternalLabelFilterSet{
		Author: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "standard"}},
				ActiveOnly: true,
			},
		},
	}
	records, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, activeOnly, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(active author labels) error = %v", err)
	}
	assertRecordURISet(t, records, []string{activeRecord})

	historical := repositories.ExternalLabelFilterSet{
		Author: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "standard"}},
				ActiveOnly: false,
			},
		},
	}
	records, err = db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, historical, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(historical author labels) error = %v", err)
	}
	assertRecordURISet(t, records, []string{activeRecord, negatedRecord, expiredRecord})
	if containsRecordURI(records, cidSpecificRecord) {
		t.Fatalf("CID-specific account label should not match authorLabels for %s", cidSpecificRecord)
	}
}

func TestRecordsRepositoryAuthorLabelFilterPaginationAndCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	recordOne := insertExternalLabelFilterRecord(t, db, "author-one", "cid-author-one", "did:plc:author-one", "2025-01-02T03:04:01Z")
	insertExternalLabelFilterRecord(t, db, "author-two", "cid-author-two", "did:plc:author-two", "2025-01-02T03:04:02Z")
	recordThree := insertExternalLabelFilterRecord(t, db, "author-three", "cid-author-three", "did:plc:author-three", "2025-01-02T03:04:03Z")
	insertExternalLabelFilterRecord(t, db, "author-four", "cid-author-four", "did:plc:author-four", "2025-01-02T03:04:04Z")
	recordFive := insertExternalLabelFilterRecord(t, db, "author-five", "cid-author-five", "did:plc:author-five", "2025-01-02T03:04:05Z")

	persistExternalLabelEvent(t, db.ExternalLabels, 1, []repositories.ExternalLabelInput{
		{LabelIndex: 0, Src: "did:plc:orglabeler", URI: "did:plc:author-one", Val: "high-quality", Cts: "2025-01-02T03:05:01Z", RawJSON: `{}`},
		{LabelIndex: 1, Src: "did:plc:orglabeler", URI: "did:plc:author-three", Val: "high-quality", Cts: "2025-01-02T03:05:03Z", RawJSON: `{}`},
		{LabelIndex: 2, Src: "did:plc:orglabeler", URI: "did:plc:author-five", Val: "high-quality", Cts: "2025-01-02T03:05:05Z", RawJSON: `{}`},
	})

	filter := repositories.ExternalLabelFilterSet{
		Author: repositories.ExternalLabelRecordFilter{
			Has: &repositories.ExternalLabelPredicate{
				Values:     []repositories.ExternalLabelStringFilter{{Operator: "eq", Value: "high-quality"}},
				ActiveOnly: true,
			},
		},
	}
	pageOne, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 2, nil)
	if err != nil {
		t.Fatalf("author label first page query error = %v", err)
	}
	assertRecordURIs(t, pageOne, []string{recordFive, recordThree})

	count, err := db.Records.GetCollectionCountFilteredWithExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter)
	if err != nil {
		t.Fatalf("GetCollectionCountFilteredWithExternalLabelFilters(author labels) error = %v", err)
	}
	if count != 3 {
		t.Fatalf("author label filtered count = %d, want 3", count)
	}

	cursor := []string{pageOne[1].IndexedAt.UTC().Format(time.RFC3339Nano), pageOne[1].URI}
	pageTwo, err := db.Records.GetByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 2, cursor)
	if err != nil {
		t.Fatalf("author label second page query error = %v", err)
	}
	assertRecordURIs(t, pageTwo, []string{recordOne})

	lastPage, err := db.Records.GetByCollectionReversedWithKeysetCursorAndExternalLabelFilters(ctx, externalLabelRecordFilterCollection, nil, repositories.DIDFilter{}, filter, nil, 2, nil)
	if err != nil {
		t.Fatalf("author label backward page query error = %v", err)
	}
	assertRecordURIs(t, lastPage, []string{recordThree, recordOne})
}

func insertExternalLabelFilterRecord(t *testing.T, db *testutil.TestDB, rkey, cid, did, indexedAt string) string {
	t.Helper()
	uri := "at://" + did + "/" + externalLabelRecordFilterCollection + "/" + rkey
	insertTestRecord(t, db.Records, uri, cid, did, externalLabelRecordFilterCollection, `{"text":"`+rkey+`"}`)
	_, err := db.Executor.DB().ExecContext(context.Background(), "UPDATE record SET indexed_at = ? WHERE uri = ?", indexedAt, uri)
	if err != nil {
		t.Fatalf("failed to set indexed_at for %s: %v", uri, err)
	}
	return uri
}

func assertRecordURISet(t *testing.T, records []*repositories.Record, want []string) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("records length = %d, want %d: %v", len(records), len(want), recordURIs(records))
	}

	wantSet := make(map[string]bool, len(want))
	for _, uri := range want {
		wantSet[uri] = true
	}
	seen := make(map[string]bool, len(records))
	for _, rec := range records {
		if !wantSet[rec.URI] {
			t.Fatalf("unexpected URI %s in records %v; want %v", rec.URI, recordURIs(records), want)
		}
		if seen[rec.URI] {
			t.Fatalf("duplicate URI %s in records %v", rec.URI, recordURIs(records))
		}
		seen[rec.URI] = true
	}
	for _, uri := range want {
		if !seen[uri] {
			t.Fatalf("missing URI %s in records %v", uri, recordURIs(records))
		}
	}
}

func assertRecordURIs(t *testing.T, records []*repositories.Record, want []string) {
	t.Helper()
	got := recordURIs(records)
	if len(got) != len(want) {
		t.Fatalf("records = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("records = %v, want %v", got, want)
		}
	}
}

func containsRecordURI(records []*repositories.Record, uri string) bool {
	for _, rec := range records {
		if rec.URI == uri {
			return true
		}
	}
	return false
}

func recordURIs(records []*repositories.Record) []string {
	uris := make([]string, 0, len(records))
	for _, rec := range records {
		uris = append(uris, rec.URI)
	}
	return uris
}
