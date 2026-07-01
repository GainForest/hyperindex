package backfill

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

type fakeRecordValidator struct {
	results map[string]validation.Result
}

func (v fakeRecordValidator) ValidateRecord(collection string, rkey string, rawJSON []byte) validation.Result {
	if result, ok := v.results[rkey]; ok {
		return result
	}
	return validation.Result{Status: validation.StatusValid, LexiconHash: "hash-current"}
}

func (v fakeRecordValidator) LexiconHash(collection string) (string, bool) {
	return "hash-current", true
}

func TestBackfillerBatchValidationClassifiesStoredRecords(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	validator := fakeRecordValidator{results: map[string]validation.Result{
		"invalid": {
			Status:      validation.StatusInvalid,
			Error:       "missing required field: name",
			LexiconHash: "hash-current",
		},
		"unknown": {
			Status: validation.StatusUnknownSchema,
			Error:  "no saved lexicon for collection com.example.record",
		},
	}}
	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, nil, validator)
	defer backfiller.Close()

	records := []*repositories.Record{
		{
			URI:        "at://did:plc:test/com.example.record/invalid",
			CID:        "cid-invalid",
			DID:        "did:plc:test",
			Collection: "com.example.record",
			JSON:       `{"$type":"com.example.record"}`,
			RKey:       "invalid",
		},
		{
			URI:        "at://did:plc:test/com.example.record/unknown",
			CID:        "cid-unknown",
			DID:        "did:plc:test",
			Collection: "com.example.record",
			JSON:       `{"$type":"com.example.record","name":"unknown"}`,
			RKey:       "unknown",
		},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("BatchInsert() error = %v", err)
	}

	backfiller.updateValidationMetadata(ctx, records)

	assertValidationMetadata(t, db.Records, records[0].URI, validation.StatusInvalid, "missing required field: name", "hash-current")
	assertValidationMetadata(t, db.Records, records[1].URI, validation.StatusUnknownSchema, "no saved lexicon for collection com.example.record", "")
}

func TestBackfillerLegacyPathClassifiesInsertedRecords(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	validator := fakeRecordValidator{results: map[string]validation.Result{
		"invalid": {
			Status:      validation.StatusInvalid,
			Error:       "missing required field: name",
			LexiconHash: "hash-current",
		},
		"unknown": {
			Status: validation.StatusUnknownSchema,
			Error:  "no saved lexicon for collection com.example.record",
		},
	}}
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/xrpc/com.atproto.repo.listRecords" {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("repo"); got != "did:plc:test" {
			t.Fatalf("repo query = %q, want did:plc:test", got)
		}
		if got := r.URL.Query().Get("collection"); got != "com.example.record" {
			t.Fatalf("collection query = %q, want com.example.record", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ListRecordsResponse{Records: []ListRecordsRecord{
			{
				URI:   "at://did:plc:test/com.example.record/invalid",
				CID:   "cid-invalid",
				Value: json.RawMessage(`{"$type":"com.example.record"}`),
			},
			{
				URI:   "at://did:plc:test/com.example.record/unknown",
				CID:   "cid-unknown",
				Value: json.RawMessage(`{"$type":"com.example.record","name":"unknown"}`),
			},
		}})
	}))
	defer pds.Close()

	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, nil, validator)
	defer backfiller.Close()

	inserted, err := backfiller.backfillActorLegacy(ctx, &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: pds.URL})
	if err != nil {
		t.Fatalf("backfillActorLegacy() error = %v", err)
	}
	if inserted != 2 {
		t.Fatalf("inserted = %d, want 2", inserted)
	}

	assertValidationMetadata(t, db.Records, "at://did:plc:test/com.example.record/invalid", validation.StatusInvalid, "missing required field: name", "hash-current")
	assertValidationMetadata(t, db.Records, "at://did:plc:test/com.example.record/unknown", validation.StatusUnknownSchema, "no saved lexicon for collection com.example.record", "")
}

func assertValidationMetadata(t *testing.T, records *repositories.RecordsRepository, uri string, wantStatus validation.Status, wantError string, wantHash string) {
	t.Helper()
	rec, err := records.GetByURI(context.Background(), uri)
	if err != nil {
		t.Fatalf("GetByURI(%s) error = %v", uri, err)
	}
	if rec.ValidationStatus != wantStatus {
		t.Fatalf("%s ValidationStatus = %q, want %q", uri, rec.ValidationStatus, wantStatus)
	}
	if rec.ValidationError != wantError {
		t.Fatalf("%s ValidationError = %q, want %q", uri, rec.ValidationError, wantError)
	}
	if rec.LexiconHash != wantHash {
		t.Fatalf("%s LexiconHash = %q, want %q", uri, rec.LexiconHash, wantHash)
	}
	if rec.ValidatedAt == nil {
		t.Fatalf("%s ValidatedAt is nil, want validation timestamp", uri)
	}
}
