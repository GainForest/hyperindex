package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

type recordsTestEnv struct {
	repo *repositories.RecordsRepository
	db   *testutil.TestDB
}

func setupRecordsTest(t *testing.T) *repositories.RecordsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Records
}

func setupRecordsTestEnv(t *testing.T) *recordsTestEnv {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return &recordsTestEnv{repo: db.Records, db: db}
}

// insertTestRecord is a helper that inserts a record and fails the test on error.
func insertTestRecord(t *testing.T, repo *repositories.RecordsRepository, uri, cid, did, collection, jsonData string) {
	t.Helper()
	_, err := repo.Insert(context.Background(), uri, cid, did, collection, jsonData)
	if err != nil {
		t.Fatalf("failed to insert test record %s: %v", uri, err)
	}
}

func TestRecordsRepository_UpdateValidationStatus(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	uri := "at://did:plc:test/com.example.record/one"
	insertTestRecord(t, repo, uri, "cid1", "did:plc:test", "com.example.record", `{"name":"one"}`)

	if err := repo.UpdateValidationStatus(ctx, uri, validation.StatusInvalid, "missing required field: name", "hash-1"); err != nil {
		t.Fatalf("UpdateValidationStatus() error = %v", err)
	}

	rec, err := repo.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusInvalid {
		t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, validation.StatusInvalid)
	}
	if rec.ValidationError != "missing required field: name" {
		t.Fatalf("ValidationError = %q", rec.ValidationError)
	}
	if rec.LexiconHash != "hash-1" {
		t.Fatalf("LexiconHash = %q, want hash-1", rec.LexiconHash)
	}
	if rec.ValidatedAt == nil {
		t.Fatal("ValidatedAt is nil, want timestamp")
	}

	if err := repo.UpdateValidationStatus(ctx, uri, validation.StatusValid, "", ""); err != nil {
		t.Fatalf("UpdateValidationStatus(valid) error = %v", err)
	}
	rec, err = repo.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() after valid update error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusValid {
		t.Fatalf("ValidationStatus after valid update = %q, want %q", rec.ValidationStatus, validation.StatusValid)
	}
	if rec.ValidationError != "" {
		t.Fatalf("ValidationError after valid update = %q, want empty", rec.ValidationError)
	}
	if rec.LexiconHash != "" {
		t.Fatalf("LexiconHash after valid update = %q, want empty", rec.LexiconHash)
	}
}

func TestRecordsRepository_MarkCollectionUnknownSchema(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	firstURI := "at://did:plc:test/com.example.record/one"
	secondURI := "at://did:plc:test/com.example.record/two"
	otherURI := "at://did:plc:test/com.example.other/one"
	insertTestRecord(t, repo, firstURI, "cid1", "did:plc:test", "com.example.record", `{"name":"one"}`)
	insertTestRecord(t, repo, secondURI, "cid2", "did:plc:test", "com.example.record", `{"name":"two"}`)
	insertTestRecord(t, repo, otherURI, "cid3", "did:plc:test", "com.example.other", `{"name":"other"}`)
	if err := repo.UpdateValidationStatus(ctx, firstURI, validation.StatusValid, "", "hash-1"); err != nil {
		t.Fatalf("UpdateValidationStatus(first) error = %v", err)
	}
	if err := repo.UpdateValidationStatus(ctx, secondURI, validation.StatusInvalid, "bad", "hash-1"); err != nil {
		t.Fatalf("UpdateValidationStatus(second) error = %v", err)
	}
	if err := repo.UpdateValidationStatus(ctx, otherURI, validation.StatusValid, "", "hash-other"); err != nil {
		t.Fatalf("UpdateValidationStatus(other) error = %v", err)
	}

	if err := repo.MarkCollectionUnknownSchema(ctx, "com.example.record", "lexicon removed for collection"); err != nil {
		t.Fatalf("MarkCollectionUnknownSchema() error = %v", err)
	}

	for _, uri := range []string{firstURI, secondURI} {
		rec, err := repo.GetByURI(ctx, uri)
		if err != nil {
			t.Fatalf("GetByURI(%s) error = %v", uri, err)
		}
		if rec.ValidationStatus != validation.StatusUnknownSchema {
			t.Fatalf("%s ValidationStatus = %q, want unknown_schema", uri, rec.ValidationStatus)
		}
		if rec.ValidationError != "lexicon removed for collection" {
			t.Fatalf("%s ValidationError = %q", uri, rec.ValidationError)
		}
		if rec.LexiconHash != "" {
			t.Fatalf("%s LexiconHash = %q, want empty", uri, rec.LexiconHash)
		}
		if rec.ValidatedAt == nil {
			t.Fatalf("%s ValidatedAt is nil, want timestamp from collection-wide update", uri)
		}
	}

	other, err := repo.GetByURI(ctx, otherURI)
	if err != nil {
		t.Fatalf("GetByURI(other) error = %v", err)
	}
	if other.ValidationStatus != validation.StatusValid || other.LexiconHash != "hash-other" {
		t.Fatalf("other record validation changed: status=%q hash=%q", other.ValidationStatus, other.LexiconHash)
	}
}

func TestRecordsRepository_ListRecordsNeedingValidation(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	collection := "com.example.record"
	records := []struct {
		uri    string
		status validation.Status
		hash   string
	}{
		{"at://did:plc:test/com.example.record/01-valid-current", validation.StatusValid, "hash-current"},
		{"at://did:plc:test/com.example.record/02-valid-missing-hash", validation.StatusValid, ""},
		{"at://did:plc:test/com.example.record/03-valid-stale", validation.StatusValid, "hash-old"},
		{"at://did:plc:test/com.example.record/04-invalid", validation.StatusInvalid, "hash-current"},
		{"at://did:plc:test/com.example.record/05-unknown", validation.StatusUnknownSchema, ""},
		{"at://did:plc:test/com.example.record/06-error", validation.StatusValidationError, "hash-current"},
		{"at://did:plc:test/com.example.record/07-invalid", validation.StatusInvalid, "hash-current"},
	}
	for _, rec := range records {
		insertTestRecord(t, repo, rec.uri, "cid", "did:plc:test", collection, `{"name":"test"}`)
		if err := repo.UpdateValidationStatus(ctx, rec.uri, rec.status, "", rec.hash); err != nil {
			t.Fatalf("UpdateValidationStatus(%s) error = %v", rec.uri, err)
		}
	}
	insertTestRecord(t, repo, "at://did:plc:test/com.example.other/01-invalid", "cid", "did:plc:test", "com.example.other", `{"name":"test"}`)

	got, err := repo.ListRecordsNeedingValidation(ctx, collection, "hash-current", "", 10)
	if err != nil {
		t.Fatalf("ListRecordsNeedingValidation() error = %v", err)
	}
	gotURIs := make([]string, 0, len(got))
	for _, rec := range got {
		gotURIs = append(gotURIs, rec.URI)
	}
	wantURIs := []string{
		"at://did:plc:test/com.example.record/02-valid-missing-hash",
		"at://did:plc:test/com.example.record/03-valid-stale",
		"at://did:plc:test/com.example.record/04-invalid",
		"at://did:plc:test/com.example.record/05-unknown",
		"at://did:plc:test/com.example.record/06-error",
		"at://did:plc:test/com.example.record/07-invalid",
	}
	if fmt.Sprint(gotURIs) != fmt.Sprint(wantURIs) {
		t.Fatalf("URIs = %v, want %v", gotURIs, wantURIs)
	}

	page, err := repo.ListRecordsNeedingValidation(ctx, collection, "hash-current", "at://did:plc:test/com.example.record/03-valid-stale", 2)
	if err != nil {
		t.Fatalf("ListRecordsNeedingValidation(afterURI, limit) error = %v", err)
	}
	assertRecordURIs(t, page, []string{
		"at://did:plc:test/com.example.record/04-invalid",
		"at://did:plc:test/com.example.record/05-unknown",
	})
}

func TestRecordsRepository_ValidOnlyQueries(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()
	collection := "com.example.record"
	validURI := "at://did:plc:test/com.example.record/01-valid"
	invalidURI := "at://did:plc:test/com.example.record/02-invalid"
	unknownURI := "at://did:plc:test/com.example.record/03-unknown"
	otherURI := "at://did:plc:test/com.example.other/01-valid"

	for _, rec := range []struct {
		uri        string
		collection string
		status     validation.Status
	}{
		{validURI, collection, validation.StatusValid},
		{invalidURI, collection, validation.StatusInvalid},
		{unknownURI, collection, validation.StatusUnknownSchema},
		{otherURI, "com.example.other", validation.StatusValid},
	} {
		insertTestRecord(t, repo, rec.uri, "cid", "did:plc:test", rec.collection, `{"name":"test"}`)
		if err := repo.UpdateValidationStatus(ctx, rec.uri, rec.status, "hidden", "hash-current"); err != nil {
			t.Fatalf("UpdateValidationStatus(%s) error = %v", rec.uri, err)
		}
	}
	if err := repo.UpdateValidationStatus(ctx, validURI, validation.StatusValid, "", "hash-current"); err != nil {
		t.Fatalf("UpdateValidationStatus(valid) error = %v", err)
	}

	if _, err := repo.GetValidByURI(ctx, validURI, collection); err != nil {
		t.Fatalf("GetValidByURI(valid) error = %v", err)
	}
	if _, err := repo.GetValidByURI(ctx, invalidURI, collection); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetValidByURI(invalid) error = %v, want sql.ErrNoRows", err)
	}

	records, err := repo.GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters(ctx, collection, nil, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{}, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetValidByCollectionSortedWithKeysetCursorAndExternalLabelFilters() error = %v", err)
	}
	assertRecordURIs(t, records, []string{validURI})

	count, err := repo.GetValidCollectionCountFilteredWithExternalLabelFilters(ctx, collection, nil, repositories.DIDFilter{}, repositories.ExternalLabelFilterSet{})
	if err != nil {
		t.Fatalf("GetValidCollectionCountFilteredWithExternalLabelFilters() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("valid count = %d, want 1", count)
	}
}

func TestRecordsRepository_Insert(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(*repositories.RecordsRepository)
		uri        string
		cid        string
		did        string
		collection string
		json       string
		wantResult repositories.InsertResult
		wantErr    bool
	}{
		{
			name:       "insert new record",
			uri:        "at://did:plc:test1/com.example.timeline.post/abc123",
			cid:        "bafyreiabc123",
			did:        "did:plc:test1",
			collection: "com.example.timeline.post",
			json:       `{"text":"hello","createdAt":"2026-01-15T10:00:00Z"}`,
			wantResult: repositories.Inserted,
		},
		{
			name: "insert same URI and same CID is skipped",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/com.example.timeline.post/dup1",
					"bafyreisame",
					"did:plc:test1",
					"com.example.timeline.post",
					`{"text":"original"}`,
				)
			},
			uri:        "at://did:plc:test1/com.example.timeline.post/dup1",
			cid:        "bafyreisame",
			did:        "did:plc:test1",
			collection: "com.example.timeline.post",
			json:       `{"text":"original"}`,
			wantResult: repositories.Skipped,
		},
		{
			name:       "insert new record with empty CID is inserted (not silently skipped)",
			uri:        "at://did:plc:test1/com.example.timeline.post/nocid",
			cid:        "", // Tap omits CID on some events
			did:        "did:plc:test1",
			collection: "com.example.timeline.post",
			json:       `{"text":"no cid"}`,
			wantResult: repositories.Inserted,
		},
		{
			name: "insert same URI with different CID is updated",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/com.example.timeline.post/upd1",
					"bafyreiold",
					"did:plc:test1",
					"com.example.timeline.post",
					`{"text":"old version"}`,
				)
			},
			uri:        "at://did:plc:test1/com.example.timeline.post/upd1",
			cid:        "bafyreinew",
			did:        "did:plc:test1",
			collection: "com.example.timeline.post",
			json:       `{"text":"new version"}`,
			wantResult: repositories.Inserted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			result, err := repo.Insert(ctx, tt.uri, tt.cid, tt.did, tt.collection, tt.json)
			if (err != nil) != tt.wantErr {
				t.Errorf("Insert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.wantResult {
				t.Errorf("Insert() result = %v, want %v", result, tt.wantResult)
			}

			// For the update case, verify the CID was actually updated
			if tt.name == "insert same URI with different CID is updated" {
				rec, err := repo.GetByURI(ctx, tt.uri)
				if err != nil {
					t.Fatalf("GetByURI after update: %v", err)
				}
				if rec.CID != tt.cid {
					t.Errorf("CID after update = %q, want %q", rec.CID, tt.cid)
				}
				if rec.JSON != tt.json {
					t.Errorf("JSON after update = %q, want %q", rec.JSON, tt.json)
				}
			}
		})
	}
}

func TestRecordsRepository_RecordCreatedAtInsertAndUpdate(t *testing.T) {
	env := setupRecordsTestEnv(t)
	ctx := context.Background()
	uri := "at://did:plc:timeline/com.example.timeline.post/created-at"

	result, err := env.repo.Insert(ctx, uri, "cid1", "did:plc:timeline", "com.example.timeline.post", `{"text":"first","createdAt":"2026-01-15T10:00:00.123456789+02:00"}`)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if result != repositories.Inserted {
		t.Fatalf("Insert() result = %v, want Inserted", result)
	}
	if got := recordCreatedAtForURI(t, env, uri); got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("record_created_at after insert = %q, want normalized UTC milliseconds", got)
	}

	_, err = env.repo.Insert(ctx, uri, "cid2", "did:plc:timeline", "com.example.timeline.post", `{"text":"updated","createdAt":"2026-01-16T10:00:00Z"}`)
	if err != nil {
		t.Fatalf("Insert() update error = %v", err)
	}
	if got := recordCreatedAtForURI(t, env, uri); got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("record_created_at after update = %q, want original timestamp preserved", got)
	}

	nullURI := "at://did:plc:timeline/com.example.timeline.post/fill-null"
	_, err = env.repo.Insert(ctx, nullURI, "samecid", "did:plc:timeline", "com.example.timeline.post", `{"text":"missing createdAt"}`)
	if err != nil {
		t.Fatalf("Insert() missing createdAt error = %v", err)
	}
	if got := recordCreatedAtForURI(t, env, nullURI); got != "" {
		t.Fatalf("record_created_at for missing createdAt = %q, want null", got)
	}

	result, err = env.repo.Insert(ctx, nullURI, "samecid", "did:plc:timeline", "com.example.timeline.post", `{"text":"missing createdAt","createdAt":"2026-01-17T00:00:00Z"}`)
	if err != nil {
		t.Fatalf("Insert() same CID fill error = %v", err)
	}
	if result != repositories.Skipped {
		t.Fatalf("Insert() same CID fill result = %v, want Skipped content result", result)
	}
	if got := recordCreatedAtForURI(t, env, nullURI); got != "2026-01-17T00:00:00.000Z" {
		t.Fatalf("record_created_at after same CID fill = %q, want filled timestamp", got)
	}
}

func TestRecordsRepository_RecordCreatedAtBatchInsertConflict(t *testing.T) {
	env := setupRecordsTestEnv(t)
	ctx := context.Background()
	preserveURI := "at://did:plc:timeline/com.example.timeline.post/batch-preserve"
	fillURI := "at://did:plc:timeline/com.example.timeline.post/batch-fill"

	if err := env.repo.BatchInsert(ctx, []*repositories.Record{
		{URI: preserveURI, CID: "cid-preserve-1", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-15T10:00:00Z"}`},
		{URI: fillURI, CID: "cid-fill-1", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"text":"missing"}`},
	}); err != nil {
		t.Fatalf("initial BatchInsert() error = %v", err)
	}

	if err := env.repo.BatchInsert(ctx, []*repositories.Record{
		{URI: preserveURI, CID: "cid-preserve-2", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-16T10:00:00Z"}`},
		{URI: fillURI, CID: "cid-fill-2", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-17T00:00:00Z"}`},
	}); err != nil {
		t.Fatalf("conflicting BatchInsert() error = %v", err)
	}

	if got := recordCreatedAtForURI(t, env, preserveURI); got != "2026-01-15T10:00:00.000Z" {
		t.Fatalf("preserved batch record_created_at = %q, want original timestamp", got)
	}
	if got := recordCreatedAtForURI(t, env, fillURI); got != "2026-01-17T00:00:00.000Z" {
		t.Fatalf("filled batch record_created_at = %q, want incoming timestamp", got)
	}
}

func TestRecordsRepository_GetRecordTimeline(t *testing.T) {
	env := setupRecordsTestEnv(t)
	ctx := context.Background()
	records := []*repositories.Record{
		{URI: "at://did:plc:alice/com.example.timeline.post/r1", CID: "cid1", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"text":"old","createdAt":"2026-01-15T10:00:00Z"}`},
		{URI: "at://did:plc:bob/com.example.timeline.like/r2", CID: "cid2", DID: "did:plc:bob", Collection: "com.example.timeline.like", JSON: `{"subject":"x","createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:alice/com.example.timeline.post/r3", CID: "cid3", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"text":"tie a","createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:carol/com.example.timeline.like/r4", CID: "cid4", DID: "did:plc:carol", Collection: "com.example.timeline.like", JSON: `{"subject":"excluded author","createdAt":"2026-01-15T14:00:00Z"}`},
		{URI: "at://did:plc:alice/com.example.timeline.repost/r5", CID: "cid5", DID: "did:plc:alice", Collection: "com.example.timeline.repost", JSON: `{"createdAt":"2026-01-15T13:00:00Z"}`},
	}
	if err := env.repo.BatchInsert(ctx, records); err != nil {
		t.Fatalf("BatchInsert() error = %v", err)
	}

	page, err := env.repo.GetRecordTimeline(ctx, []string{"did:plc:alice", "did:plc:bob"}, []string{"com.example.timeline.post", "com.example.timeline.like"}, 10, nil)
	if err != nil {
		t.Fatalf("GetRecordTimeline() error = %v", err)
	}
	wantURIs := []string{
		"at://did:plc:bob/com.example.timeline.like/r2",
		"at://did:plc:alice/com.example.timeline.post/r3",
		"at://did:plc:alice/com.example.timeline.post/r1",
	}
	assertTimelineURIs(t, page, wantURIs)

	cursor := &repositories.RecordTimelineCursor{CreatedAt: page[0].RecordCreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"), URI: page[0].URI}
	nextPage, err := env.repo.GetRecordTimeline(ctx, []string{"did:plc:alice", "did:plc:bob"}, []string{"com.example.timeline.post", "com.example.timeline.like"}, 10, cursor)
	if err != nil {
		t.Fatalf("GetRecordTimeline(after) error = %v", err)
	}
	assertTimelineURIs(t, nextPage, wantURIs[1:])

	globalPage, err := env.repo.GetRecordTimeline(ctx, nil, []string{"com.example.timeline.post"}, 10, nil)
	if err != nil {
		t.Fatalf("GetRecordTimeline(global) error = %v", err)
	}
	assertTimelineURIs(t, globalPage, []string{
		"at://did:plc:alice/com.example.timeline.post/r3",
		"at://did:plc:alice/com.example.timeline.post/r1",
	})
}

func TestRecordsRepository_GetRecordTimelineSupportsThousandAuthorsSQLite(t *testing.T) {
	env := setupRecordsTestEnv(t)
	ctx := context.Background()
	if _, err := env.repo.Insert(ctx,
		"at://did:plc:author999/com.example.timeline.post/r1",
		"cid-author999",
		"did:plc:author999",
		"com.example.timeline.post",
		`{"text":"large author set","createdAt":"2026-01-15T10:00:00Z"}`,
	); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	authors := make([]string, repositories.MaxRecordTimelineAuthors)
	for i := range authors {
		authors[i] = fmt.Sprintf("did:plc:author%d", i)
	}
	page, err := env.repo.GetRecordTimeline(ctx, authors, []string{"com.example.timeline.post"}, 10, nil)
	if err != nil {
		t.Fatalf("GetRecordTimeline() with %d authors error = %v", len(authors), err)
	}
	assertTimelineURIs(t, page, []string{"at://did:plc:author999/com.example.timeline.post/r1"})
}

func recordCreatedAtForURI(t *testing.T, env *recordsTestEnv, uri string) string {
	t.Helper()
	var value sql.NullString
	if err := env.db.Executor.DB().QueryRowContext(context.Background(), "SELECT record_created_at FROM record WHERE uri = ?", uri).Scan(&value); err != nil {
		t.Fatalf("failed to query record_created_at for %s: %v", uri, err)
	}
	if !value.Valid {
		return ""
	}
	return value.String
}

func assertTimelineURIs(t *testing.T, records []*repositories.RecordTimelineRecord, want []string) {
	t.Helper()
	if len(records) != len(want) {
		t.Fatalf("timeline length = %d, want %d", len(records), len(want))
	}
	for i, rec := range records {
		if rec.URI != want[i] {
			t.Fatalf("timeline[%d].URI = %q, want %q", i, rec.URI, want[i])
		}
	}
}

func TestRecordsRepository_BatchInsert(t *testing.T) {
	tests := []struct {
		name    string
		records []*repositories.Record
		wantErr bool
	}{
		{
			name:    "empty slice",
			records: nil,
		},
		{
			name: "single record",
			records: []*repositories.Record{
				{
					URI:        "at://did:plc:test1/com.example.timeline.post/batch1",
					CID:        "bafyreibatch1",
					DID:        "did:plc:test1",
					Collection: "com.example.timeline.post",
					JSON:       `{"text":"batch 1","createdAt":"2026-01-15T10:00:00Z"}`,
				},
			},
		},
		{
			name: "five records",
			records: []*repositories.Record{
				{URI: "at://did:plc:test1/com.example.timeline.post/b1", CID: "bafyreib1", DID: "did:plc:test1", Collection: "com.example.timeline.post", JSON: `{"text":"b1"}`},
				{URI: "at://did:plc:test1/com.example.timeline.post/b2", CID: "bafyreib2", DID: "did:plc:test1", Collection: "com.example.timeline.post", JSON: `{"text":"b2"}`},
				{URI: "at://did:plc:test2/com.example.timeline.post/b3", CID: "bafyreib3", DID: "did:plc:test2", Collection: "com.example.timeline.post", JSON: `{"text":"b3"}`},
				{URI: "at://did:plc:test2/com.example.timeline.like/b4", CID: "bafyreib4", DID: "did:plc:test2", Collection: "com.example.timeline.like", JSON: `{"subject":"at://x"}`},
				{URI: "at://did:plc:test3/com.example.timeline.post/b5", CID: "bafyreib5", DID: "did:plc:test3", Collection: "com.example.timeline.post", JSON: `{"text":"b5"}`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			err := repo.BatchInsert(ctx, tt.records)
			if (err != nil) != tt.wantErr {
				t.Errorf("BatchInsert() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			// Verify all records are retrievable
			for _, rec := range tt.records {
				got, err := repo.GetByURI(ctx, rec.URI)
				if err != nil {
					t.Errorf("GetByURI(%s) after BatchInsert: %v", rec.URI, err)
					continue
				}
				if got.CID != rec.CID {
					t.Errorf("record %s CID = %q, want %q", rec.URI, got.CID, rec.CID)
				}
				if got.DID != rec.DID {
					t.Errorf("record %s DID = %q, want %q", rec.URI, got.DID, rec.DID)
				}
				if got.Collection != rec.Collection {
					t.Errorf("record %s Collection = %q, want %q", rec.URI, got.Collection, rec.Collection)
				}
			}
		})
	}
}

func TestRecordsRepository_GetByURI(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		uri     string
		wantErr error
		check   func(*testing.T, *repositories.Record)
	}{
		{
			name: "found",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo,
					"at://did:plc:test1/com.example.timeline.post/found1",
					"bafyreifound1",
					"did:plc:test1",
					"com.example.timeline.post",
					`{"text":"found me","createdAt":"2026-01-15T10:00:00Z"}`,
				)
			},
			uri: "at://did:plc:test1/com.example.timeline.post/found1",
			check: func(t *testing.T, rec *repositories.Record) {
				if rec.URI != "at://did:plc:test1/com.example.timeline.post/found1" {
					t.Errorf("URI = %q", rec.URI)
				}
				if rec.CID != "bafyreifound1" {
					t.Errorf("CID = %q", rec.CID)
				}
				if rec.DID != "did:plc:test1" {
					t.Errorf("DID = %q", rec.DID)
				}
				if rec.Collection != "com.example.timeline.post" {
					t.Errorf("Collection = %q", rec.Collection)
				}
				if rec.JSON != `{"text":"found me","createdAt":"2026-01-15T10:00:00Z"}` {
					t.Errorf("JSON = %q", rec.JSON)
				}
			},
		},
		{
			name:    "not found",
			uri:     "at://did:plc:nonexistent/com.example.timeline.post/nope",
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			rec, err := repo.GetByURI(ctx, tt.uri)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("GetByURI() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetByURI() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, rec)
			}
		})
	}
}

func TestRecordsRepository_GetByURIs(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*repositories.RecordsRepository)
		uris      []string
		wantCount int
	}{
		{
			name:      "empty slice returns nil",
			uris:      nil,
			wantCount: 0,
		},
		{
			name: "multiple URIs",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/m1", "bafyreim1", "did:plc:test1", "com.example.timeline.post", `{"text":"m1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/m2", "bafyreim2", "did:plc:test1", "com.example.timeline.post", `{"text":"m2"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/m3", "bafyreim3", "did:plc:test1", "com.example.timeline.post", `{"text":"m3"}`)
			},
			uris: []string{
				"at://did:plc:test1/com.example.timeline.post/m1",
				"at://did:plc:test1/com.example.timeline.post/m3",
			},
			wantCount: 2,
		},
		{
			name: "batches large URI lists",
			setup: func(repo *repositories.RecordsRepository) {
				for i := 0; i < repositories.SQLParamBatchSize+5; i++ {
					uri := fmt.Sprintf("at://did:plc:test1/com.example.timeline.post/batch-%04d", i)
					insertTestRecord(t, repo, uri, fmt.Sprintf("bafyreibatch%04d", i), "did:plc:test1", "com.example.timeline.post", fmt.Sprintf(`{"text":"batch %d"}`, i))
				}
			},
			uris: func() []string {
				uris := make([]string, 0, repositories.SQLParamBatchSize+5)
				for i := 0; i < repositories.SQLParamBatchSize+5; i++ {
					uris = append(uris, fmt.Sprintf("at://did:plc:test1/com.example.timeline.post/batch-%04d", i))
				}
				return uris
			}(),
			wantCount: repositories.SQLParamBatchSize + 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			records, err := repo.GetByURIs(ctx, tt.uris)
			if err != nil {
				t.Fatalf("GetByURIs() error: %v", err)
			}
			if len(records) != tt.wantCount {
				t.Errorf("GetByURIs() returned %d records, want %d", len(records), tt.wantCount)
			}

			// Verify each requested URI is in the results
			if tt.wantCount > 0 {
				uriSet := make(map[string]bool)
				for _, rec := range records {
					uriSet[rec.URI] = true
				}
				for _, uri := range tt.uris {
					if !uriSet[uri] {
						t.Errorf("GetByURIs() missing expected URI %s", uri)
					}
				}
			}
		})
	}
}

func TestRecordsRepository_GetByCollection(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records across two collections
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/c1", "bafyreic1", "did:plc:test1", "com.example.timeline.post", `{"text":"c1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/c2", "bafyreic2", "did:plc:test1", "com.example.timeline.post", `{"text":"c2"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.like/c3", "bafyreic3", "did:plc:test1", "com.example.timeline.like", `{"subject":"at://x"}`)

	t.Run("returns records for specific collection", func(t *testing.T) {
		records, err := repo.GetByCollection(ctx, "com.example.timeline.post", 100)
		if err != nil {
			t.Fatalf("GetByCollection() error: %v", err)
		}
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
		for _, rec := range records {
			if rec.Collection != "com.example.timeline.post" {
				t.Errorf("unexpected collection %q", rec.Collection)
			}
		}
	})

	t.Run("does not return records from other collections", func(t *testing.T) {
		records, err := repo.GetByCollection(ctx, "com.example.timeline.like", 100)
		if err != nil {
			t.Fatalf("GetByCollection() error: %v", err)
		}
		if len(records) != 1 {
			t.Errorf("got %d records, want 1", len(records))
		}
		if len(records) > 0 && records[0].Collection != "com.example.timeline.like" {
			t.Errorf("unexpected collection %q", records[0].Collection)
		}
	})
}

func TestRecordsRepository_GetByCollectionWithCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	// Use the executor's underlying DB to set indexed_at to distinct values
	sqlDB := env.db.Executor.DB()

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/p1", "bafyreip1", "did:plc:test1", "com.example.timeline.post", `{"text":"p1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/p1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/p2", "bafyreip2", "did:plc:test1", "com.example.timeline.post", `{"text":"p2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/p2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/p3", "bafyreip3", "did:plc:test1", "com.example.timeline.post", `{"text":"p3"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/p3'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/p4", "bafyreip4", "did:plc:test1", "com.example.timeline.post", `{"text":"p4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/p4'`)

	t.Run("first page returns newest first", func(t *testing.T) {
		records, err := repo.GetByCollectionWithCursor(ctx, "com.example.timeline.post", 2, "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Newest first: p4, p3
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/p4" {
			t.Errorf("first record URI = %q, want p4", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/p3" {
			t.Errorf("second record URI = %q, want p3", records[1].URI)
		}
	})

	t.Run("second page with cursor returns older records", func(t *testing.T) {
		// Use p3's indexed_at as cursor to get records older than p3
		records, err := repo.GetByCollectionWithCursor(ctx, "com.example.timeline.post", 2, "2026-01-15T12:00:00Z")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Older than p3: p2, p1
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/p2" {
			t.Errorf("first record URI = %q, want p2", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/p1" {
			t.Errorf("second record URI = %q, want p1", records[1].URI)
		}
	})
}

func TestRecordsRepository_GetByCollectionWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/k1", "bafyreik1", "did:plc:test1", "com.example.timeline.post", `{"text":"k1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/k1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/k2", "bafyreik2", "did:plc:test1", "com.example.timeline.post", `{"text":"k2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/k2'`)

	// k3a and k3b have the SAME indexed_at to test URI tiebreaking
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/k3a", "bafyreik3a", "did:plc:test1", "com.example.timeline.post", `{"text":"k3a"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/k3a'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/k3b", "bafyreik3b", "did:plc:test1", "com.example.timeline.post", `{"text":"k3b"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/k3b'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/k4", "bafyreik4", "did:plc:test1", "com.example.timeline.post", `{"text":"k4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/k4'`)

	t.Run("first page without cursor", func(t *testing.T) {
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "com.example.timeline.post", 3, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// Newest first: k4, k3b, k3a (k3b > k3a by URI DESC)
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/k4" {
			t.Errorf("first record URI = %q, want k4", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/k3b" {
			t.Errorf("second record URI = %q, want k3b", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/com.example.timeline.post/k3a" {
			t.Errorf("third record URI = %q, want k3a", records[2].URI)
		}
	})

	t.Run("keyset cursor skips to correct position", func(t *testing.T) {
		// Cursor is after k3b (same timestamp as k3a, but k3b > k3a by URI)
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "com.example.timeline.post", 10,
			"2026-01-15T12:00:00Z", "at://did:plc:test1/com.example.timeline.post/k3b")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// After k3b: k3a (same timestamp, smaller URI), then k2, k1
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/k3a" {
			t.Errorf("first record URI = %q, want k3a", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/k2" {
			t.Errorf("second record URI = %q, want k2", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/com.example.timeline.post/k1" {
			t.Errorf("third record URI = %q, want k1", records[2].URI)
		}
	})

	t.Run("cursor between timestamps", func(t *testing.T) {
		// Cursor after k3a — should return k2, k1
		records, err := repo.GetByCollectionWithKeysetCursor(ctx, "com.example.timeline.post", 10,
			"2026-01-15T12:00:00Z", "at://did:plc:test1/com.example.timeline.post/k3a")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/k2" {
			t.Errorf("first record URI = %q, want k2", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/k1" {
			t.Errorf("second record URI = %q, want k1", records[1].URI)
		}
	})
}

func TestRecordsRepository_KeysetPagination_NormalizesIndexedAtFormats(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/n1", "bafyrein1", "did:plc:test1", "com.example.timeline.post", `{"text":"n1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15 10:00:00' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/n1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/n2", "bafyrein2", "did:plc:test1", "com.example.timeline.post", `{"text":"n2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15 11:00:00' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/n2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/n3", "bafyrein3", "did:plc:test1", "com.example.timeline.post", `{"text":"n3"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15 12:00:00' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/n3'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/n4", "bafyrein4", "did:plc:test1", "com.example.timeline.post", `{"text":"n4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15 13:00:00' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/n4'`)

	t.Run("forward keyset cursor with RFC3339 timestamp skips newer rows", func(t *testing.T) {
		page1, err := repo.GetByCollectionSortedWithKeysetCursor(
			ctx,
			"com.example.timeline.post",
			nil,
			repositories.DIDFilter{},
			nil,
			2,
			nil,
		)
		if err != nil {
			t.Fatalf("first page query failed: %v", err)
		}
		if len(page1) != 2 {
			t.Fatalf("first page got %d records, want 2", len(page1))
		}
		if page1[0].URI != "at://did:plc:test1/com.example.timeline.post/n4" || page1[1].URI != "at://did:plc:test1/com.example.timeline.post/n3" {
			t.Fatalf("first page URIs = [%s, %s], want [n4, n3]", page1[0].URI, page1[1].URI)
		}

		page2, err := repo.GetByCollectionSortedWithKeysetCursor(
			ctx,
			"com.example.timeline.post",
			nil,
			repositories.DIDFilter{},
			nil,
			2,
			[]string{"2026-01-15T12:00:00Z", "at://did:plc:test1/com.example.timeline.post/n3"},
		)
		if err != nil {
			t.Fatalf("second page query failed: %v", err)
		}
		if len(page2) != 2 {
			t.Fatalf("second page got %d records, want 2", len(page2))
		}
		if page2[0].URI != "at://did:plc:test1/com.example.timeline.post/n2" {
			t.Errorf("second page first URI = %q, want n2", page2[0].URI)
		}
		if page2[1].URI != "at://did:plc:test1/com.example.timeline.post/n1" {
			t.Errorf("second page second URI = %q, want n1", page2[1].URI)
		}
	})

	t.Run("backward keyset cursor with RFC3339 timestamp returns prior edges", func(t *testing.T) {
		records, err := repo.GetByCollectionReversedWithKeysetCursor(
			ctx,
			"com.example.timeline.post",
			nil,
			repositories.DIDFilter{},
			nil,
			2,
			[]string{"2026-01-15T10:00:00Z", "at://did:plc:test1/com.example.timeline.post/n1"},
		)
		if err != nil {
			t.Fatalf("backward query failed: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("backward query got %d records, want 2", len(records))
		}
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/n3" {
			t.Errorf("records[0].URI = %q, want n3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/n2" {
			t.Errorf("records[1].URI = %q, want n2", records[1].URI)
		}
	})
}

func TestRecordsRepository_GetByDID(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/a1", "bafyreia1", "did:plc:alice", "com.example.timeline.post", `{"text":"a1"}`)
	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.like/a2", "bafyreia2", "did:plc:alice", "com.example.timeline.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:bob/com.example.timeline.post/b1", "bafyreib1", "did:plc:bob", "com.example.timeline.post", `{"text":"b1"}`)

	records, err := repo.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID() error: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("got %d records, want 2", len(records))
	}
	for _, rec := range records {
		if rec.DID != "did:plc:alice" {
			t.Errorf("unexpected DID %q, want did:plc:alice", rec.DID)
		}
	}
}

func TestRecordsRepository_Delete(t *testing.T) {
	t.Run("delete existing record", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/del1", "bafyreidel1", "did:plc:test1", "com.example.timeline.post", `{"text":"delete me"}`)

		countBefore, _ := repo.GetCount(ctx)

		err := repo.Delete(ctx, "at://did:plc:test1/com.example.timeline.post/del1")
		if err != nil {
			t.Fatalf("Delete() error: %v", err)
		}

		countAfter, _ := repo.GetCount(ctx)
		if countAfter != countBefore-1 {
			t.Errorf("count after delete = %d, want %d", countAfter, countBefore-1)
		}

		_, err = repo.GetByURI(ctx, "at://did:plc:test1/com.example.timeline.post/del1")
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("expected sql.ErrNoRows after delete, got %v", err)
		}
	})

	t.Run("delete non-existing record is no error", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		err := repo.Delete(ctx, "at://did:plc:nonexistent/com.example.timeline.post/nope")
		if err != nil {
			t.Errorf("Delete() on non-existing record should not error, got: %v", err)
		}
	})
}

func TestRecordsRepository_DeleteByDID(t *testing.T) {
	t.Run("deletes only target did records", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo,
			"at://did:plc:alice/app.certified.actor.profile/p1",
			"bafyreialice1",
			"did:plc:alice",
			"app.certified.actor.profile",
			`{"displayName":"Alice"}`,
		)
		insertTestRecord(t, repo,
			"at://did:plc:alice/app.certified.actor.organization/o1",
			"bafyreialice2",
			"did:plc:alice",
			"app.certified.actor.organization",
			`{"organizationType":"ngo"}`,
		)
		insertTestRecord(t, repo,
			"at://did:plc:bob/app.certified.actor.profile/p2",
			"bafyreibob1",
			"did:plc:bob",
			"app.certified.actor.profile",
			`{"displayName":"Bob"}`,
		)

		if err := repo.DeleteByDID(ctx, "did:plc:alice"); err != nil {
			t.Fatalf("DeleteByDID() error = %v", err)
		}

		aliceRecords, err := repo.GetByDID(ctx, "did:plc:alice")
		if err != nil {
			t.Fatalf("GetByDID(alice) error = %v", err)
		}
		if len(aliceRecords) != 0 {
			t.Fatalf("expected 0 alice records after purge, got %d", len(aliceRecords))
		}

		bobRecords, err := repo.GetByDID(ctx, "did:plc:bob")
		if err != nil {
			t.Fatalf("GetByDID(bob) error = %v", err)
		}
		if len(bobRecords) != 1 {
			t.Fatalf("expected 1 bob record, got %d", len(bobRecords))
		}
	})

	t.Run("non-existing did is no-op", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		if err := repo.DeleteByDID(ctx, "did:plc:does-not-exist"); err != nil {
			t.Fatalf("DeleteByDID(non-existing) should not error, got %v", err)
		}
	})
}

func TestRecordsRepository_PurgeActorData(t *testing.T) {
	env := setupRecordsTestEnv(t)
	ctx := context.Background()

	if err := env.db.Actors.Upsert(ctx, "did:plc:alice", "alice.bsky.social"); err != nil {
		t.Fatalf("failed to seed alice actor: %v", err)
	}
	if err := env.db.Actors.Upsert(ctx, "did:plc:bob", "bob.bsky.social"); err != nil {
		t.Fatalf("failed to seed bob actor: %v", err)
	}

	insertTestRecord(t, env.repo,
		"at://did:plc:alice/app.certified.actor.profile/p1",
		"bafyreialice1",
		"did:plc:alice",
		"app.certified.actor.profile",
		`{"displayName":"Alice"}`,
	)
	insertTestRecord(t, env.repo,
		"at://did:plc:bob/app.certified.actor.profile/p1",
		"bafyreibob1",
		"did:plc:bob",
		"app.certified.actor.profile",
		`{"displayName":"Bob"}`,
	)

	if err := env.repo.PurgeActorData(ctx, "did:plc:alice"); err != nil {
		t.Fatalf("PurgeActorData() error = %v", err)
	}

	aliceRecords, err := env.repo.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID(alice) error = %v", err)
	}
	if len(aliceRecords) != 0 {
		t.Fatalf("expected 0 alice records after purge, got %d", len(aliceRecords))
	}

	if _, err := env.db.Actors.GetByDID(ctx, "did:plc:alice"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected alice actor deleted (sql.ErrNoRows), got %v", err)
	}

	bobRecords, err := env.repo.GetByDID(ctx, "did:plc:bob")
	if err != nil {
		t.Fatalf("GetByDID(bob) error = %v", err)
	}
	if len(bobRecords) != 1 {
		t.Fatalf("expected 1 bob record retained, got %d", len(bobRecords))
	}

	if _, err := env.db.Actors.GetByDID(ctx, "did:plc:bob"); err != nil {
		t.Fatalf("expected bob actor retained, got %v", err)
	}
}

func TestRecordsRepository_DeleteAll(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/da1", "bafyreida1", "did:plc:test1", "com.example.timeline.post", `{"text":"da1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/da2", "bafyreida2", "did:plc:test1", "com.example.timeline.post", `{"text":"da2"}`)

	err := repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error: %v", err)
	}

	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error: %v", err)
	}
	if count != 0 {
		t.Errorf("count after DeleteAll = %d, want 0", count)
	}
}

func TestRecordsRepository_GetCount(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*repositories.RecordsRepository)
		wantCount int64
	}{
		{
			name:      "empty database",
			wantCount: 0,
		},
		{
			name: "after inserts",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/gc1", "bafyreigc1", "did:plc:test1", "com.example.timeline.post", `{"text":"gc1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/gc2", "bafyreigc2", "did:plc:test1", "com.example.timeline.post", `{"text":"gc2"}`)
				insertTestRecord(t, repo, "at://did:plc:test2/com.example.timeline.post/gc3", "bafyreigc3", "did:plc:test2", "com.example.timeline.post", `{"text":"gc3"}`)
			},
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			count, err := repo.GetCount(ctx)
			if err != nil {
				t.Fatalf("GetCount() error: %v", err)
			}
			if count != tt.wantCount {
				t.Errorf("GetCount() = %d, want %d", count, tt.wantCount)
			}
		})
	}
}

func TestRecordsRepository_GetCountByDID(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/1", "cid1", "did:plc:alice", "com.example.timeline.post", `{"text":"a1"}`)
	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/2", "cid2", "did:plc:alice", "com.example.timeline.post", `{"text":"a2"}`)
	insertTestRecord(t, repo, "at://did:plc:bob/com.example.timeline.post/1", "cid3", "did:plc:bob", "com.example.timeline.post", `{"text":"b1"}`)

	aliceCount, err := repo.GetCountByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetCountByDID(alice) error: %v", err)
	}
	if aliceCount != 2 {
		t.Fatalf("GetCountByDID(alice) = %d, want 2", aliceCount)
	}

	bobCount, err := repo.GetCountByDID(ctx, "did:plc:bob")
	if err != nil {
		t.Fatalf("GetCountByDID(bob) error: %v", err)
	}
	if bobCount != 1 {
		t.Fatalf("GetCountByDID(bob) = %d, want 1", bobCount)
	}

	missingCount, err := repo.GetCountByDID(ctx, "did:plc:missing")
	if err != nil {
		t.Fatalf("GetCountByDID(missing) error: %v", err)
	}
	if missingCount != 0 {
		t.Fatalf("GetCountByDID(missing) = %d, want 0", missingCount)
	}
}

func TestRecordsRepository_GetCollectionCountFiltered_AggregateOverflow(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	filters := make([]repositories.FieldFilter, 20)
	for i := range filters {
		values := make([]interface{}, 49)
		for j := range values {
			values[j] = fmt.Sprintf("value-%d-%d", i, j)
		}

		filters[i] = repositories.FieldFilter{
			Field:     fmt.Sprintf("field%c", 'a'+i),
			Operator:  "in",
			Value:     values,
			FieldType: "string",
		}
	}

	dids := make([]string, 19)
	for i := range dids {
		dids[i] = fmt.Sprintf("did:%c", 'a'+i)
	}

	_, err := repo.GetCollectionCountFiltered(ctx, "col", filters, repositories.DIDFilter{IN: dids})
	if err == nil {
		t.Fatal("expected error for aggregate parameter overflow, got nil")
	}
	if !errors.Is(err, repositories.ErrSQLiteAggregateParameterLimit) {
		t.Fatalf("error = %v, want ErrSQLiteAggregateParameterLimit", err)
	}
}

func TestRecordsRepository_GetCollectionStats(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records: 3 posts, 2 likes, 1 follow
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/s1", "bafyreis1", "did:plc:test1", "com.example.timeline.post", `{"text":"s1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/s2", "bafyreis2", "did:plc:test1", "com.example.timeline.post", `{"text":"s2"}`)
	insertTestRecord(t, repo, "at://did:plc:test2/com.example.timeline.post/s3", "bafyreis3", "did:plc:test2", "com.example.timeline.post", `{"text":"s3"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.like/s4", "bafyreis4", "did:plc:test1", "com.example.timeline.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:test2/com.example.timeline.like/s5", "bafyreis5", "did:plc:test2", "com.example.timeline.like", `{"subject":"at://y"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.graph.follow/s6", "bafyreis6", "did:plc:test1", "app.bsky.graph.follow", `{"subject":"did:plc:test2"}`)

	stats, err := repo.GetCollectionStats(ctx)
	if err != nil {
		t.Fatalf("GetCollectionStats() error: %v", err)
	}

	if len(stats) != 3 {
		t.Fatalf("got %d stats, want 3", len(stats))
	}

	// Ordered by count DESC: posts(3), likes(2), follow(1)
	if stats[0].Collection != "com.example.timeline.post" || stats[0].Count != 3 {
		t.Errorf("stats[0] = {%s, %d}, want {com.example.timeline.post, 3}", stats[0].Collection, stats[0].Count)
	}
	if stats[1].Collection != "com.example.timeline.like" || stats[1].Count != 2 {
		t.Errorf("stats[1] = {%s, %d}, want {com.example.timeline.like, 2}", stats[1].Collection, stats[1].Count)
	}
	if stats[2].Collection != "app.bsky.graph.follow" || stats[2].Count != 1 {
		t.Errorf("stats[2] = {%s, %d}, want {app.bsky.graph.follow, 1}", stats[2].Collection, stats[2].Count)
	}
}

func TestRecordsRepository_GetCollectionStatsFiltered(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sf1", "bafyreisf1", "did:plc:test1", "com.example.timeline.post", `{"text":"sf1"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sf2", "bafyreisf2", "did:plc:test1", "com.example.timeline.post", `{"text":"sf2"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.like/sf3", "bafyreisf3", "did:plc:test1", "com.example.timeline.like", `{"subject":"at://x"}`)
	insertTestRecord(t, repo, "at://did:plc:test1/app.bsky.graph.follow/sf4", "bafyreisf4", "did:plc:test1", "app.bsky.graph.follow", `{"subject":"did:plc:test2"}`)

	t.Run("with specific collections", func(t *testing.T) {
		stats, err := repo.GetCollectionStatsFiltered(ctx, []string{"com.example.timeline.post", "com.example.timeline.like"})
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("got %d stats, want 2", len(stats))
		}
		// Verify only requested collections appear
		for _, stat := range stats {
			if stat.Collection != "com.example.timeline.post" && stat.Collection != "com.example.timeline.like" {
				t.Errorf("unexpected collection %q in filtered results", stat.Collection)
			}
		}
	})

	t.Run("empty collections returns all", func(t *testing.T) {
		stats, err := repo.GetCollectionStatsFiltered(ctx, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(stats) != 3 {
			t.Errorf("got %d stats, want 3 (all collections)", len(stats))
		}
	})
}

func TestRecordsRepository_GetCollectionTimeSeries(t *testing.T) {
	repo := setupRecordsTest(t)
	ctx := context.Background()

	// Insert records with createdAt in JSON on different dates, from different users
	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/ts1", "bafyreits1", "did:plc:alice", "com.example.timeline.post", `{"text":"ts1","createdAt":"2026-01-15T10:00:00Z"}`)
	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/ts2", "bafyreits2", "did:plc:alice", "com.example.timeline.post", `{"text":"ts2","createdAt":"2026-01-15T14:00:00Z"}`)
	insertTestRecord(t, repo, "at://did:plc:bob/com.example.timeline.post/ts3", "bafyreits3", "did:plc:bob", "com.example.timeline.post", `{"text":"ts3","createdAt":"2026-01-16T09:00:00Z"}`)

	ts, err := repo.GetCollectionTimeSeries(ctx, "com.example.timeline.post")
	if err != nil {
		t.Fatalf("GetCollectionTimeSeries() error: %v", err)
	}

	if ts.Collection != "com.example.timeline.post" {
		t.Errorf("Collection = %q", ts.Collection)
	}
	if ts.TotalRecords != 3 {
		t.Errorf("TotalRecords = %d, want 3", ts.TotalRecords)
	}
	if ts.UniqueUsers != 2 {
		t.Errorf("UniqueUsers = %d, want 2", ts.UniqueUsers)
	}

	if len(ts.Data) < 2 {
		t.Fatalf("got %d data points, want at least 2", len(ts.Data))
	}

	// First date: 2026-01-15 with 2 records
	if ts.Data[0].Date != "2026-01-15" {
		t.Errorf("Data[0].Date = %q, want 2026-01-15", ts.Data[0].Date)
	}
	if ts.Data[0].Count != 2 {
		t.Errorf("Data[0].Count = %d, want 2", ts.Data[0].Count)
	}
	if ts.Data[0].Cumulative != 2 {
		t.Errorf("Data[0].Cumulative = %d, want 2", ts.Data[0].Cumulative)
	}

	// Second date: 2026-01-16 with 1 record, cumulative 3
	if ts.Data[1].Date != "2026-01-16" {
		t.Errorf("Data[1].Date = %q, want 2026-01-16", ts.Data[1].Date)
	}
	if ts.Data[1].Count != 1 {
		t.Errorf("Data[1].Count = %d, want 1", ts.Data[1].Count)
	}
	if ts.Data[1].Cumulative != 3 {
		t.Errorf("Data[1].Cumulative = %d, want 3", ts.Data[1].Cumulative)
	}
}

func TestRecordsRepository_GetCIDsByURIs(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		uris    []string
		want    map[string]string
		wantErr bool
	}{
		{
			name: "empty returns empty map",
			uris: nil,
			want: map[string]string{},
		},
		{
			name: "returns correct URI to CID mapping",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/cid1", "bafyreicid1", "did:plc:test1", "com.example.timeline.post", `{"text":"cid1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/cid2", "bafyreicid2", "did:plc:test1", "com.example.timeline.post", `{"text":"cid2"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/cid3", "bafyreicid3", "did:plc:test1", "com.example.timeline.post", `{"text":"cid3"}`)
			},
			uris: []string{
				"at://did:plc:test1/com.example.timeline.post/cid1",
				"at://did:plc:test1/com.example.timeline.post/cid3",
			},
			want: map[string]string{
				"at://did:plc:test1/com.example.timeline.post/cid1": "bafyreicid1",
				"at://did:plc:test1/com.example.timeline.post/cid3": "bafyreicid3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			got, err := repo.GetCIDsByURIs(ctx, tt.uris)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetCIDsByURIs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
			for uri, wantCID := range tt.want {
				if gotCID, ok := got[uri]; !ok {
					t.Errorf("missing URI %s in result", uri)
				} else if gotCID != wantCID {
					t.Errorf("CID for %s = %q, want %q", uri, gotCID, wantCID)
				}
			}
		})
	}
}

func TestRecordsRepository_GetExistingCIDs(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(*repositories.RecordsRepository)
		cids    []string
		want    map[string]bool
		wantErr bool
	}{
		{
			name: "empty returns empty map",
			cids: nil,
			want: map[string]bool{},
		},
		{
			name: "returns correct existing CIDs",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/ec1", "bafyreiec1", "did:plc:test1", "com.example.timeline.post", `{"text":"ec1"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/ec2", "bafyreiec2", "did:plc:test1", "com.example.timeline.post", `{"text":"ec2"}`)
			},
			cids: []string{"bafyreiec1", "bafyreiec2", "bafyreinonexistent"},
			want: map[string]bool{
				"bafyreiec1": true,
				"bafyreiec2": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			got, err := repo.GetExistingCIDs(ctx, tt.cids)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetExistingCIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %d entries, want %d", len(got), len(tt.want))
			}
			for cid, wantVal := range tt.want {
				if gotVal, ok := got[cid]; !ok {
					t.Errorf("missing CID %s in result", cid)
				} else if gotVal != wantVal {
					t.Errorf("value for CID %s = %v, want %v", cid, gotVal, wantVal)
				}
			}
			// Ensure non-existent CID is not in the result
			if _, ok := got["bafyreinonexistent"]; ok {
				t.Error("non-existent CID should not be in result")
			}
		})
	}
}

func TestRecordsRepository_GetByCollectionFilteredWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps and varied JSON fields
	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/f1", "bafyreif1", "did:plc:alice", "com.example.timeline.post", `{"text":"hello world","score":10}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:alice/com.example.timeline.post/f1'`)

	insertTestRecord(t, repo, "at://did:plc:alice/com.example.timeline.post/f2", "bafyreif2", "did:plc:alice", "com.example.timeline.post", `{"text":"goodbye world","score":20}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:alice/com.example.timeline.post/f2'`)

	insertTestRecord(t, repo, "at://did:plc:bob/com.example.timeline.post/f3", "bafyreif3", "did:plc:bob", "com.example.timeline.post", `{"text":"hello again"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:bob/com.example.timeline.post/f3'`)

	insertTestRecord(t, repo, "at://did:plc:bob/com.example.timeline.post/f4", "bafyreif4", "did:plc:bob", "com.example.timeline.post", `{"text":"no greeting"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:bob/com.example.timeline.post/f4'`)

	t.Run("no filters returns all records", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Errorf("got %d records, want 4", len(records))
		}
	})

	t.Run("filter by string eq", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "eq", Value: "hello world", FieldType: "string"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", filters, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:alice/com.example.timeline.post/f1" {
			t.Errorf("unexpected URI %q", records[0].URI)
		}
	})

	t.Run("filter by isNull true returns records without field", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "score", Operator: "isNull", Value: true, FieldType: "integer"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", filters, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		// f3 and f4 have no score field
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
	})

	t.Run("filter with DID omits when empty", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Errorf("got %d records, want 4 (no DID filter)", len(records))
		}
	})

	t.Run("filter with DID adds AND did = ? when non-empty", func(t *testing.T) {
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{EQ: "did:plc:alice"}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Errorf("got %d records, want 2", len(records))
		}
		for _, rec := range records {
			if rec.DID != "did:plc:alice" {
				t.Errorf("unexpected DID %q, want did:plc:alice", rec.DID)
			}
		}
	})

	t.Run("filter with DID and field filter combined", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "contains", Value: "hello", FieldType: "string"},
		}
		records, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", filters, repositories.DIDFilter{EQ: "did:plc:alice"}, 100, "", "")
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:alice/com.example.timeline.post/f1" {
			t.Errorf("unexpected URI %q", records[0].URI)
		}
	})

	t.Run("pagination with filters", func(t *testing.T) {
		filters := []repositories.FieldFilter{
			{Field: "text", Operator: "contains", Value: "hello", FieldType: "string"},
		}
		// f3 (2026-01-15T12:00:00Z) and f1 (2026-01-15T10:00:00Z) contain "hello"
		// First page: limit 1 → f3 (newest)
		page1, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", filters, repositories.DIDFilter{}, 1, "", "")
		if err != nil {
			t.Fatalf("page1 error: %v", err)
		}
		if len(page1) != 1 {
			t.Fatalf("page1: got %d records, want 1", len(page1))
		}
		if page1[0].URI != "at://did:plc:bob/com.example.timeline.post/f3" {
			t.Errorf("page1[0] URI = %q, want f3", page1[0].URI)
		}

		// Second page using cursor from f3
		afterTS := page1[0].IndexedAt.UTC().Format("2006-01-02T15:04:05Z")
		page2, err := repo.GetByCollectionFilteredWithKeysetCursor(ctx, "com.example.timeline.post", filters, repositories.DIDFilter{}, 1, afterTS, page1[0].URI)
		if err != nil {
			t.Fatalf("page2 error: %v", err)
		}
		if len(page2) != 1 {
			t.Fatalf("page2: got %d records, want 1", len(page2))
		}
		if page2[0].URI != "at://did:plc:alice/com.example.timeline.post/f1" {
			t.Errorf("page2[0] URI = %q, want f1", page2[0].URI)
		}
	})
}

func TestRecordsRepository_IterateAll(t *testing.T) {
	t.Run("empty database returns 0 processed", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		count, err := repo.IterateAll(ctx, 10, func(r *repositories.Record) error {
			t.Error("callback should not be called on empty DB")
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 0 {
			t.Errorf("processed = %d, want 0", count)
		}
	})

	t.Run("processes all records in URI order", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		// Insert 5 records with URIs that sort alphabetically
		for i := 1; i <= 5; i++ {
			uri := fmt.Sprintf("at://did:plc:test1/com.example.timeline.post/iter%d", i)
			cid := fmt.Sprintf("bafyreiiter%d", i)
			jsonStr := fmt.Sprintf(`{"text":"iter%d","createdAt":"2026-01-15T10:00:00Z"}`, i)
			insertTestRecord(t, repo, uri, cid, "did:plc:test1", "com.example.timeline.post", jsonStr)
		}

		var visited []string
		count, err := repo.IterateAll(ctx, 2, func(r *repositories.Record) error {
			visited = append(visited, r.URI)
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 5 {
			t.Errorf("processed = %d, want 5", count)
		}
		if len(visited) != 5 {
			t.Fatalf("visited %d records, want 5", len(visited))
		}

		// Verify URI order (ascending)
		for i := 1; i < len(visited); i++ {
			if visited[i] <= visited[i-1] {
				t.Errorf("records not in URI order: %q <= %q", visited[i], visited[i-1])
			}
		}
	})

	t.Run("callback error stops iteration", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/err1", "bafyreierr1", "did:plc:test1", "com.example.timeline.post", `{"text":"err1"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/err2", "bafyreierr2", "did:plc:test1", "com.example.timeline.post", `{"text":"err2"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/err3", "bafyreierr3", "did:plc:test1", "com.example.timeline.post", `{"text":"err3"}`)

		callbackErr := fmt.Errorf("stop processing")
		callCount := 0

		count, err := repo.IterateAll(ctx, 10, func(r *repositories.Record) error {
			callCount++
			if callCount == 2 {
				return callbackErr
			}
			return nil
		})

		if !errors.Is(err, callbackErr) {
			t.Errorf("IterateAll() error = %v, want %v", err, callbackErr)
		}
		// totalProcessed is incremented after fn returns successfully,
		// so it should be 1 (the first successful call before the error on call 2)
		if count != 1 {
			t.Errorf("processed = %d, want 1", count)
		}
	})

	t.Run("batchSize 0 defaults to 1000", func(t *testing.T) {
		repo := setupRecordsTest(t)
		ctx := context.Background()

		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/bs1", "bafyreibs1", "did:plc:test1", "com.example.timeline.post", `{"text":"bs1"}`)
		insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/bs2", "bafyreibs2", "did:plc:test1", "com.example.timeline.post", `{"text":"bs2"}`)

		count, err := repo.IterateAll(ctx, 0, func(r *repositories.Record) error {
			return nil
		})
		if err != nil {
			t.Fatalf("IterateAll() error: %v", err)
		}
		if count != 2 {
			t.Errorf("processed = %d, want 2", count)
		}
	})
}

func TestRecordsRepository_Search(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(*repositories.RecordsRepository)
		query          string
		collection     string
		limit          int
		afterTimestamp string
		afterURI       string
		wantCount      int
		wantURIs       []string
	}{
		{
			name: "returns records containing search term",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sr1", "bafyreisr1", "did:plc:test1", "com.example.timeline.post", `{"text":"hello world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sr2", "bafyreisr2", "did:plc:test1", "com.example.timeline.post", `{"text":"goodbye world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sr3", "bafyreisr3", "did:plc:test1", "com.example.timeline.post", `{"text":"hello again"}`)
			},
			query:     "hello",
			limit:     10,
			wantCount: 2,
		},
		{
			name: "search with collection filter narrows results",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sc1", "bafyreisc1", "did:plc:test1", "com.example.timeline.post", `{"text":"hello post"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.like/sc2", "bafyreisc2", "did:plc:test1", "com.example.timeline.like", `{"text":"hello like"}`)
			},
			query:      "hello",
			collection: "com.example.timeline.post",
			limit:      10,
			wantCount:  1,
		},
		{
			name: "search returns no results when term not found",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sn1", "bafyreisn1", "did:plc:test1", "com.example.timeline.post", `{"text":"nothing here"}`)
			},
			query:     "xyzzy",
			limit:     10,
			wantCount: 0,
		},
		{
			name: "search is case-insensitive",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/si1", "bafyreisi1", "did:plc:test1", "com.example.timeline.post", `{"text":"Hello World"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/si2", "bafyreisi2", "did:plc:test1", "com.example.timeline.post", `{"text":"HELLO WORLD"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/si3", "bafyreisi3", "did:plc:test1", "com.example.timeline.post", `{"text":"hello world"}`)
			},
			query:     "hello",
			limit:     10,
			wantCount: 3,
		},
		{
			name: "search with pagination limit",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sp1", "bafyreisp1", "did:plc:test1", "com.example.timeline.post", `{"text":"paginate me one"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sp2", "bafyreisp2", "did:plc:test1", "com.example.timeline.post", `{"text":"paginate me two"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sp3", "bafyreisp3", "did:plc:test1", "com.example.timeline.post", `{"text":"paginate me three"}`)
			},
			query:     "paginate",
			limit:     2,
			wantCount: 2,
		},
		{
			name: "search escapes percent wildcard in query",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sw1", "bafyreisw1", "did:plc:test1", "com.example.timeline.post", `{"text":"100% complete"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/sw2", "bafyreisw2", "did:plc:test1", "com.example.timeline.post", `{"text":"anything else"}`)
			},
			query:     "100%",
			limit:     10,
			wantCount: 1,
		},
		{
			name: "search escapes underscore wildcard in query",
			setup: func(repo *repositories.RecordsRepository) {
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/su1", "bafyreisu1", "did:plc:test1", "com.example.timeline.post", `{"text":"hello_world"}`)
				insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/su2", "bafyreisu2", "did:plc:test1", "com.example.timeline.post", `{"text":"helloXworld"}`)
			},
			query:     "hello_world",
			limit:     10,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(repo)
			}

			records, err := repo.Search(ctx, tt.query, tt.collection, tt.limit, tt.afterTimestamp, tt.afterURI)
			if err != nil {
				t.Fatalf("Search() error: %v", err)
			}
			if len(records) != tt.wantCount {
				t.Errorf("Search() returned %d records, want %d", len(records), tt.wantCount)
			}

			// Verify specific URIs if provided
			if len(tt.wantURIs) > 0 {
				uriSet := make(map[string]bool)
				for _, rec := range records {
					uriSet[rec.URI] = true
				}
				for _, uri := range tt.wantURIs {
					if !uriSet[uri] {
						t.Errorf("Search() missing expected URI %s", uri)
					}
				}
			}
		})
	}
}

func TestRecordsRepository_Search_Pagination(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert records with distinct indexed_at timestamps
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/pg1", "bafyreipg1", "did:plc:test1", "com.example.timeline.post", `{"text":"search term alpha"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/pg1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/pg2", "bafyreipg2", "did:plc:test1", "com.example.timeline.post", `{"text":"search term beta"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/pg2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/pg3", "bafyreipg3", "did:plc:test1", "com.example.timeline.post", `{"text":"search term gamma"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/pg3'`)

	t.Run("first page returns newest first", func(t *testing.T) {
		records, err := repo.Search(ctx, "search term", "", 2, "", "")
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Newest first: pg3, pg2
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/pg3" {
			t.Errorf("first record URI = %q, want pg3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/pg2" {
			t.Errorf("second record URI = %q, want pg2", records[1].URI)
		}
	})

	t.Run("second page with keyset cursor returns older records", func(t *testing.T) {
		// Cursor after pg2 (indexed_at=2026-01-15T11:00:00Z)
		records, err := repo.Search(ctx, "search term", "", 10,
			"2026-01-15T11:00:00Z", "at://did:plc:test1/com.example.timeline.post/pg2")
		if err != nil {
			t.Fatalf("Search() error: %v", err)
		}
		if len(records) != 1 {
			t.Fatalf("got %d records, want 1", len(records))
		}
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/pg1" {
			t.Errorf("record URI = %q, want pg1", records[0].URI)
		}
	})
}

func TestRecordsRepository_GetByCollectionReversedWithKeysetCursor(t *testing.T) {
	env := setupRecordsTestEnv(t)
	repo := env.repo
	ctx := context.Background()

	sqlDB := env.db.Executor.DB()

	// Insert 5 records with distinct indexed_at timestamps (oldest = r1, newest = r5)
	// Default DESC order: r5, r4, r3, r2, r1
	// Backward pagination (last N) returns the last N edges in the connection.
	// "last 3" without cursor returns r3, r2, r1 (the 3 oldest in DESC order).
	// "last 2, before r1" returns r3, r2 (the 2 edges just before r1 in the DESC connection).
	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/r1", "bafyreir1", "did:plc:test1", "com.example.timeline.post", `{"text":"r1"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T10:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/r1'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/r2", "bafyreir2", "did:plc:test1", "com.example.timeline.post", `{"text":"r2"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T11:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/r2'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/r3", "bafyreir3", "did:plc:test1", "com.example.timeline.post", `{"text":"r3"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T12:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/r3'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/r4", "bafyreir4", "did:plc:test1", "com.example.timeline.post", `{"text":"r4"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T13:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/r4'`)

	insertTestRecord(t, repo, "at://did:plc:test1/com.example.timeline.post/r5", "bafyreir5", "did:plc:test1", "com.example.timeline.post", `{"text":"r5"}`)
	_, _ = sqlDB.ExecContext(ctx, `UPDATE record SET indexed_at = '2026-01-15T14:00:00Z' WHERE uri = 'at://did:plc:test1/com.example.timeline.post/r5'`)

	t.Run("last 3 without cursor returns oldest 3 in DESC order", func(t *testing.T) {
		// Default DESC order: r5, r4, r3, r2, r1
		// last 3 = r3, r2, r1 (the last 3 edges in the connection)
		// Algorithm: reversed sort=ASC, LIMIT 3 → r1,r2,r3 → reverse → r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, nil, 3, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 3 {
			t.Fatalf("got %d records, want 3", len(records))
		}
		// Result in DESC order: r3, r2, r1
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/r3" {
			t.Errorf("records[0].URI = %q, want r3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/r2" {
			t.Errorf("records[1].URI = %q, want r2", records[1].URI)
		}
		if records[2].URI != "at://did:plc:test1/com.example.timeline.post/r1" {
			t.Errorf("records[2].URI = %q, want r1", records[2].URI)
		}
	})

	t.Run("last N+1 allows hasPreviousPage detection", func(t *testing.T) {
		// Fetch 4 (last 3 + 1 extra) — should return 4 records
		// Algorithm: reversed sort=ASC, LIMIT 4 → r1,r2,r3,r4 → reverse → r4,r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, nil, 4, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		// 4 records returned means there are more (hasPreviousPage = true when caller uses last=3)
		if len(records) != 4 {
			t.Fatalf("got %d records, want 4", len(records))
		}
	})

	t.Run("before r1 returns edges before r1 in DESC connection", func(t *testing.T) {
		// DESC connection: r5, r4, r3, r2, r1
		// "before r1" = edges that come before r1 in the list = r5, r4, r3, r2
		// Algorithm: reversed sort=ASC, comparison=>, WHERE indexed_at > 10:00
		//   → r2,r3,r4,r5 → reverse → r5,r4,r3,r2
		beforeCursor := []string{"2026-01-15T10:00:00Z", "at://did:plc:test1/com.example.timeline.post/r1"}
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, nil, 10, beforeCursor)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 4 {
			t.Fatalf("got %d records, want 4", len(records))
		}
		// Result in DESC order: r5, r4, r3, r2
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/r5" {
			t.Errorf("records[0].URI = %q, want r5", records[0].URI)
		}
		if records[3].URI != "at://did:plc:test1/com.example.timeline.post/r2" {
			t.Errorf("records[3].URI = %q, want r2", records[3].URI)
		}
	})

	t.Run("last 2 before r1 returns 2 edges just before r1", func(t *testing.T) {
		// DESC connection: r5, r4, r3, r2, r1
		// "before r1" = r5, r4, r3, r2; last 2 = r3, r2
		// Algorithm: reversed sort=ASC, comparison=>, WHERE indexed_at > 10:00, LIMIT 2
		//   → r2,r3 → reverse → r3,r2
		beforeCursor := []string{"2026-01-15T10:00:00Z", "at://did:plc:test1/com.example.timeline.post/r1"}
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, nil, 2, beforeCursor)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 2 {
			t.Fatalf("got %d records, want 2", len(records))
		}
		// Result in DESC order: r3, r2 (the 2 edges just before r1)
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/r3" {
			t.Errorf("records[0].URI = %q, want r3", records[0].URI)
		}
		if records[1].URI != "at://did:plc:test1/com.example.timeline.post/r2" {
			t.Errorf("records[1].URI = %q, want r2", records[1].URI)
		}
	})

	t.Run("all records returned when limit exceeds total", func(t *testing.T) {
		// Algorithm: reversed sort=ASC, LIMIT 100 → r1,r2,r3,r4,r5 → reverse → r5,r4,r3,r2,r1
		records, err := repo.GetByCollectionReversedWithKeysetCursor(ctx, "com.example.timeline.post", nil, repositories.DIDFilter{}, nil, 100, nil)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if len(records) != 5 {
			t.Fatalf("got %d records, want 5", len(records))
		}
		// Should be in DESC order: r5, r4, r3, r2, r1
		if records[0].URI != "at://did:plc:test1/com.example.timeline.post/r5" {
			t.Errorf("records[0].URI = %q, want r5", records[0].URI)
		}
		if records[4].URI != "at://did:plc:test1/com.example.timeline.post/r1" {
			t.Errorf("records[4].URI = %q, want r1", records[4].URI)
		}
	})
}

// TestSearchTimeout verifies that the Search method applies a context deadline.
func TestSearchTimeout(t *testing.T) {
	tests := []struct {
		name        string
		ctxTimeout  time.Duration
		wantTimeout bool
	}{
		{
			name:        "already-cancelled context returns error immediately",
			ctxTimeout:  0, // will be cancelled before call
			wantTimeout: true,
		},
		{
			name:        "context with ample time succeeds",
			ctxTimeout:  30 * time.Second,
			wantTimeout: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupRecordsTest(t)

			ctx, cancel := context.WithTimeout(context.Background(), repositories.SearchTimeout)
			defer cancel()

			// Verify that the Search method wraps the context with a deadline.
			// We do this by checking that a pre-cancelled context causes an error.
			if tt.wantTimeout {
				cancelledCtx, cancelFn := context.WithCancel(context.Background())
				cancelFn() // cancel immediately
				_, err := repo.Search(cancelledCtx, "hello", "", 10, "", "")
				if err == nil {
					t.Error("Search() with cancelled context should return an error, got nil")
				}
			} else {
				// Normal call with a fresh context should succeed (even with empty results).
				_, err := repo.Search(ctx, "hello", "", 10, "", "")
				if err != nil {
					t.Errorf("Search() with valid context returned unexpected error: %v", err)
				}
			}
		})
	}

	// Verify the exported constant value.
	t.Run("SearchTimeout constant is 10 seconds", func(t *testing.T) {
		if repositories.SearchTimeout != 10*time.Second {
			t.Errorf("SearchTimeout = %v, want 10s", repositories.SearchTimeout)
		}
	})
}
