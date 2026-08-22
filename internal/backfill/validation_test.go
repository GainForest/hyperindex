package backfill

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluesky-social/indigo/atproto/atdata"

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
	if err := db.Records.BatchUpsertWithValidation(ctx, backfiller.validationWrites(records)); err != nil {
		t.Fatalf("BatchUpsertWithValidation() error = %v", err)
	}

	assertValidationMetadata(t, db.Records, records[0].URI, validation.StatusInvalid, "missing required field: name", "hash-current")
	assertValidationMetadata(t, db.Records, records[1].URI, validation.StatusUnknownSchema, "no saved lexicon for collection com.example.record", "")
}

func TestBackfillerValidationWritesFailClosedWithoutValidator(t *testing.T) {
	db := testutil.SetupTestDB(t)
	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, nil)
	defer backfiller.Close()
	record := &repositories.Record{
		URI: "at://did:plc:test/com.example.record/no-validator", CID: "cid", DID: "did:plc:test",
		Collection: "com.example.record", RKey: "no-validator", JSON: `{"name":"test"}`,
	}
	writes := backfiller.validationWrites([]*repositories.Record{record})
	if len(writes) != 1 || writes[0].ValidationStatus != validation.StatusValidationError || writes[0].ValidationError == "" {
		t.Fatalf("validationWrites() = %#v, want fail-closed validation_error", writes)
	}
}

func TestBackfillBatchKeepsSameCIDAtDistinctURIs(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	if _, err := db.Records.Insert(ctx, "at://did:plc:test/com.example.record/first", "shared-cid", "did:plc:test", "com.example.record", `{"name":"same"}`); err != nil {
		t.Fatalf("Insert(existing) error = %v", err)
	}
	writes := []repositories.RecordWrite{
		{URI: "at://did:plc:test/com.example.record/first", CID: "shared-cid", DID: "did:plc:test", Collection: "com.example.record", RKey: "first", JSON: `{"name":"same"}`, ValidationStatus: validation.StatusValid, LexiconHash: "hash-current"},
		{URI: "at://did:plc:test/com.example.record/second", CID: "shared-cid", DID: "did:plc:test", Collection: "com.example.record", RKey: "second", JSON: `{"name":"same"}`, ValidationStatus: validation.StatusValid, LexiconHash: "hash-current"},
	}
	result, err := db.Records.BatchUpsertWithValidationForBackfill(ctx, "did:plc:test", writes)
	if err != nil {
		t.Fatalf("BatchUpsertWithValidationForBackfill() error = %v", err)
	}
	if len(result.ChangedIndices) != 1 || result.ChangedIndices[0] != 1 || result.Skipped != 1 {
		t.Fatalf("result = changed:%v skipped:%d, want [1]/1", result.ChangedIndices, result.Skipped)
	}
	if _, err := db.Records.GetByURI(ctx, writes[1].URI); err != nil {
		t.Fatalf("distinct URI with shared CID was not persisted: %v", err)
	}
}

func TestBackfillerCIDLookupFailureHasNoWritesCountersOrActivity(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	if _, err := db.Executor.DB().ExecContext(ctx, "ALTER TABLE record RENAME COLUMN cid TO cid_unavailable"); err != nil {
		t.Fatalf("rename CID column to force classification failure: %v", err)
	}
	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()

	uri := "at://did:plc:test/com.example.record/lookup-failure"
	inserted, err := backfiller.persistRecordBatch(ctx, "did:plc:test", []*repositories.Record{{
		URI: uri, CID: "cid", DID: "did:plc:test", Collection: "com.example.record", RKey: "lookup-failure", JSON: `{"name":"test"}`,
	}})
	if err == nil || !strings.Contains(err.Error(), "classify existing repo") || inserted != 0 {
		t.Fatalf("persistRecordBatch() inserted=%d error=%v, want lookup error and zero inserts", inserted, err)
	}
	if backfiller.stats.RecordsInserted != 0 || backfiller.stats.RecordsSkipped != 0 {
		t.Fatalf("counters changed after lookup failure: inserted=%d skipped=%d", backfiller.stats.RecordsInserted, backfiller.stats.RecordsSkipped)
	}
	var recordCount int
	if err := db.Executor.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM record").Scan(&recordCount); err != nil || recordCount != 0 {
		t.Fatalf("record count after lookup failure = %d, %v; want 0, nil", recordCount, err)
	}
	activityCount, err := db.Activity.GetCount(ctx)
	if err != nil || activityCount != 0 {
		t.Fatalf("activity after lookup failure = %d, %v; want 0, nil", activityCount, err)
	}
}

func TestBackfillerDuplicateURIProducesOneChangedCounterAndActivity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()
	uri := "at://did:plc:test/com.example.record/duplicate"
	records := []*repositories.Record{
		{URI: uri, CID: "cid-duplicate", DID: "did:plc:test", Collection: "com.example.record", RKey: "duplicate", JSON: `{"name":"first"}`},
		{URI: uri, CID: "cid-duplicate", DID: "did:plc:test", Collection: "com.example.record", RKey: "duplicate", JSON: "{\n  \"name\": \"same CID\"\n}"},
	}

	inserted, err := backfiller.persistRecordBatch(ctx, "did:plc:test", records)
	if err != nil || inserted != 1 {
		t.Fatalf("persistRecordBatch() inserted=%d error=%v, want 1 nil", inserted, err)
	}
	if backfiller.stats.RecordsInserted != 1 || backfiller.stats.RecordsSkipped != 0 {
		t.Fatalf("duplicate counters = inserted:%d skipped:%d, want 1/0", backfiller.stats.RecordsInserted, backfiller.stats.RecordsSkipped)
	}
	activityCount, err := db.Activity.GetCount(ctx)
	if err != nil || activityCount != 1 {
		t.Fatalf("duplicate activity count = %d, %v; want 1, nil", activityCount, err)
	}
	stored, err := db.Records.GetByURI(ctx, uri)
	if err != nil || stored.JSON != records[0].JSON {
		t.Fatalf("stored duplicate record = %#v, %v; want stable first content", stored, err)
	}
}

func TestConcurrentBackfillActorsLogOneCreateForSameDID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	data := &AtprotoData{DID: "did:plc:concurrent", Handle: "concurrent.example", PDS: "https://pds.example"}
	record := testCARRecord(t,
		"at://did:plc:concurrent/com.example.record/one", "cid-one", "com.example.record", "one",
		`{"$type":"com.example.record","name":"one"}`,
	)

	bothFetched := make(chan struct{})
	releaseFetch := make(chan struct{})
	var fetchCalls int32
	getRepo := func(context.Context, string, string, []string) ([]CARRecord, error) {
		if atomic.AddInt32(&fetchCalls, 1) == 2 {
			close(bothFetched)
		}
		<-releaseFetch
		return []CARRecord{record}, nil
	}

	first := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer first.Close()
	first.client = &fakeBackfillClient{data: data, getRepo: getRepo}
	second := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer second.Close()
	second.client = &fakeBackfillClient{data: data, getRepo: getRepo}

	type result struct {
		inserted int
		err      error
	}
	results := make(chan result, 2)
	go func() {
		inserted, err := first.BackfillActor(ctx, data.DID)
		results <- result{inserted: inserted, err: err}
	}()
	go func() {
		inserted, err := second.BackfillActor(ctx, data.DID)
		results <- result{inserted: inserted, err: err}
	}()
	select {
	case <-bothFetched:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent backfills did not both finish network fetches")
	}
	close(releaseFetch)

	totalInserted := 0
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("BackfillActor() error = %v", got.err)
			}
			totalInserted += got.inserted
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent backfill did not complete")
		}
	}
	if totalInserted != 1 {
		t.Fatalf("concurrent inserted total = %d, want 1", totalInserted)
	}
	activityCount, err := db.Activity.GetCount(ctx)
	if err != nil || activityCount != 1 {
		t.Fatalf("concurrent activity count = %d, %v; want 1, nil", activityCount, err)
	}
	if _, err := db.Records.GetByURI(ctx, record.URI); err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
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
	var gotPath, gotRepo, gotCollection string
	pds := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotRepo = r.URL.Query().Get("repo")
		gotCollection = r.URL.Query().Get("collection")
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
	if gotPath != "/xrpc/com.atproto.repo.listRecords" {
		t.Fatalf("unexpected request path %s", gotPath)
	}
	if gotRepo != "did:plc:test" {
		t.Fatalf("repo query = %q, want did:plc:test", gotRepo)
	}
	if gotCollection != "com.example.record" {
		t.Fatalf("collection query = %q, want com.example.record", gotCollection)
	}

	assertValidationMetadata(t, db.Records, "at://did:plc:test/com.example.record/invalid", validation.StatusInvalid, "missing required field: name", "hash-current")
	assertValidationMetadata(t, db.Records, "at://did:plc:test/com.example.record/unknown", validation.StatusUnknownSchema, "no saved lexicon for collection com.example.record", "")
}

func TestBackfillActorCARPathRepairsSameCIDValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	const uri = "at://did:plc:test/com.example.record/same-car"
	const recordJSON = `{"$type":"com.example.record","name":"repaired"}`
	if _, err := db.Records.Insert(ctx, uri, "cid-same", "did:plc:test", "com.example.record", recordJSON); err != nil {
		t.Fatalf("Insert(stale same-CID record) error = %v", err)
	}

	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{
		data:       data,
		carRecords: []CARRecord{testCARRecord(t, uri, "cid-same", "com.example.record", "same-car", recordJSON)},
	}

	inserted, err := backfiller.BackfillActor(ctx, data.DID)
	if err != nil {
		t.Fatalf("BackfillActor(CAR same-CID) error = %v", err)
	}
	if inserted != 0 {
		t.Fatalf("BackfillActor(CAR same-CID) inserted = %d, want 0 repaired", inserted)
	}
	assertValidationMetadata(t, db.Records, uri, validation.StatusValid, "", "hash-current")
	if count, err := db.Activity.GetCount(ctx); err != nil || count != 0 {
		t.Fatalf("same-CID CAR activity count = %d, %v; want 0, nil", count, err)
	}
}

func TestBackfillActorLegacyPathRepairsSameCIDValidation(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	const uri = "at://did:plc:test/com.example.record/same-legacy"
	const recordJSON = `{"$type":"com.example.record","name":"repaired"}`
	if _, err := db.Records.Insert(ctx, uri, "cid-same", "did:plc:test", "com.example.record", recordJSON); err != nil {
		t.Fatalf("Insert(stale same-CID record) error = %v", err)
	}

	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{
		data:   data,
		carErr: newCARFailure(CARFailureAvailability, errors.New("CAR unavailable")),
		listRecords: map[string][]ListRecordsRecord{
			"com.example.record": {{URI: uri, CID: "cid-same", Value: json.RawMessage(recordJSON)}},
		},
	}

	inserted, err := backfiller.BackfillActor(ctx, data.DID)
	if err != nil {
		t.Fatalf("BackfillActor(legacy same-CID) error = %v", err)
	}
	if inserted != 0 {
		t.Fatalf("BackfillActor(legacy same-CID) inserted = %d, want 0 repaired", inserted)
	}
	assertValidationMetadata(t, db.Records, uri, validation.StatusValid, "", "hash-current")
	if count, err := db.Activity.GetCount(ctx); err != nil || count != 0 {
		t.Fatalf("same-CID legacy activity count = %d, %v; want 0, nil", count, err)
	}
}

func TestBackfillActorLogsCommittedCARSiblingBeforeIntegrityError(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{data: data, carRecords: []CARRecord{
		testCARRecord(t, "at://did:plc:test/com.example.record/safe", "cid-safe", "com.example.record", "safe", `{"$type":"com.example.record","name":"safe"}`),
		{URI: "at://did:plc:test/com.example.record/broken", CID: "cid-broken", Collection: "com.example.record", RKey: "broken", Value: []byte{0xff}},
	}}

	inserted, err := backfiller.BackfillActor(ctx, data.DID)
	if err == nil || inserted != 1 {
		t.Fatalf("BackfillActor() inserted=%d error=%v, want 1 and integrity error", inserted, err)
	}
	entries, err := db.Activity.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Operation != "create" || entries[0].RKey == nil || *entries[0].RKey != "safe" {
		t.Fatalf("CAR activity entries = %#v, want one create for safe sibling", entries)
	}
}

func TestBackfillActorLogsSuccessfulLegacyFallbackRecord(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, db.Activity, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{
		data: data, carErr: newCARFailure(CARFailureAvailability, errors.New("CAR unavailable")),
		listRecords: map[string][]ListRecordsRecord{"com.example.record": {{
			URI: "at://did:plc:test/com.example.record/legacy", CID: "cid-legacy", Value: json.RawMessage(`{"$type":"com.example.record","name":"legacy"}`),
		}}},
	}

	inserted, err := backfiller.BackfillActor(ctx, data.DID)
	if err != nil || inserted != 1 {
		t.Fatalf("BackfillActor() inserted=%d error=%v, want 1 nil", inserted, err)
	}
	entries, err := db.Activity.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 || entries[0].Operation != "create" || entries[0].RKey == nil || *entries[0].RKey != "legacy" {
		t.Fatalf("legacy activity entries = %#v, want one create for legacy record", entries)
	}
}

func TestBackfillerCARBatchFailureIsSurfacedWithoutProcessedCount(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	const uri = "at://did:plc:test/com.example.record/failure"
	const recordJSON = `{"$type":"com.example.record","name":"failure"}`
	if _, err := db.Records.Insert(ctx, uri, "cid-failure", "did:plc:test", "com.example.record", recordJSON); err != nil {
		t.Fatalf("seed same-CID record: %v", err)
	}
	if _, err := db.Executor.DB().ExecContext(ctx, `CREATE TRIGGER fail_backfill_record_write BEFORE UPDATE ON record BEGIN SELECT RAISE(ABORT, 'forced repair failure'); END`); err != nil {
		t.Fatalf("create batch failure trigger: %v", err)
	}

	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, nil, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", Handle: "test.example", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{
		data:       data,
		carRecords: []CARRecord{testCARRecord(t, uri, "cid-failure", "com.example.record", "failure", recordJSON)},
	}
	count, err := backfiller.safeProcessRepo(ctx, data.PDS, data)
	if err == nil || !strings.Contains(err.Error(), "repair") {
		t.Fatalf("safeProcessRepo() count=%d error=%v, want surfaced repair error", count, err)
	}
	if got := atomic.LoadInt64(&backfiller.stats.ReposProcessed); got != 0 {
		t.Fatalf("ReposProcessed = %d, want 0 after persistence failure", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.Errors); got != 1 {
		t.Fatalf("Errors = %d, want 1 after surfaced worker failure", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.RecordsInserted); got != 0 {
		t.Fatalf("RecordsInserted = %d, want 0 after failed batch", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.RecordsSkipped); got != 0 {
		t.Fatalf("RecordsSkipped = %d, want 0 after failed batch", got)
	}
}

func TestBackfillerMalformedCARDoesNotMarkRepoProcessed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	const validURI = "at://did:plc:test/com.example.record/valid-before-malformed"
	backfiller := NewBackfiller(DefaultConfig(), db.Records, db.Actors, nil, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{data: data, carErr: newCARFailure(CARFailureIntegrity, errors.New("missing CAR block")), carRecords: []CARRecord{
		testCARRecord(t, validURI, "cid-valid", "com.example.record", "valid-before-malformed", `{"$type":"com.example.record","name":"valid"}`),
		{URI: "at://did:plc:test/com.example.record/malformed", CID: "cid-malformed", Collection: "com.example.record", RKey: "malformed", Value: []byte{0xff}},
	}}

	count, err := backfiller.safeProcessRepo(ctx, data.PDS, data)
	if err == nil || !strings.Contains(err.Error(), "convert CAR record") {
		t.Fatalf("safeProcessRepo() count=%d error=%v, want CAR conversion error", count, err)
	}
	if got := atomic.LoadInt64(&backfiller.stats.ReposProcessed); got != 0 {
		t.Fatalf("ReposProcessed = %d, want 0", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.Errors); got != 1 {
		t.Fatalf("Errors = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.RecordsInserted); got != 1 {
		t.Fatalf("RecordsInserted = %d, want 1 committed safe sibling", got)
	}
	if _, err := db.Records.GetByURI(ctx, validURI); err != nil {
		t.Fatalf("safe CAR sibling was not committed: %v", err)
	}
	if calls := atomic.LoadInt64(&backfiller.client.(*fakeBackfillClient).listCalls); calls != 0 {
		t.Fatalf("ListRecords calls = %d, want 0 for CAR integrity failure", calls)
	}
}

func TestBackfillerPartialLegacyFailureDoesNotMarkRepoProcessed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.first", "com.example.second"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, nil, fakeRecordValidator{})
	defer backfiller.Close()
	data := &AtprotoData{DID: "did:plc:test", PDS: "https://pds.example"}
	backfiller.client = &fakeBackfillClient{
		data:   data,
		carErr: newCARFailure(CARFailureAvailability, errors.New("CAR unavailable")),
		listRecords: map[string][]ListRecordsRecord{
			"com.example.second": {{
				URI: "at://did:plc:test/com.example.second/one", CID: "cid-one", Value: json.RawMessage(`{"$type":"com.example.second","name":"one"}`),
			}},
		},
		listErrors: map[string]error{"com.example.first": errors.New("malformed listRecords response")},
	}

	count, err := backfiller.safeProcessRepo(ctx, data.PDS, data)
	if err == nil || !strings.Contains(err.Error(), "com.example.first") {
		t.Fatalf("safeProcessRepo() count=%d error=%v, want first collection error", count, err)
	}
	if count != 1 {
		t.Fatalf("partial legacy count = %d, want 1 inserted before surfaced failure", count)
	}
	if got := atomic.LoadInt64(&backfiller.stats.ReposProcessed); got != 0 {
		t.Fatalf("ReposProcessed = %d, want 0", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.Errors); got != 1 {
		t.Fatalf("Errors = %d, want 1", got)
	}
	if got := atomic.LoadInt64(&backfiller.stats.RecordsInserted); got != 1 {
		t.Fatalf("RecordsInserted = %d, want 1 partial write", got)
	}
}

func TestBackfillerCARAvailabilityAndUnsupportedFallback(t *testing.T) {
	for _, kind := range []CARFailureKind{CARFailureAvailability, CARFailureUnsupported} {
		t.Run(string(kind), func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			cfg := DefaultConfig()
			cfg.Collections = []string{"com.example.record"}
			backfiller := NewBackfiller(cfg, db.Records, db.Actors, nil, fakeRecordValidator{})
			defer backfiller.Close()
			data := &AtprotoData{DID: "did:plc:test", PDS: "https://pds.example"}
			uri := "at://did:plc:test/com.example.record/fallback"
			client := &fakeBackfillClient{
				data: data, carErr: newCARFailure(kind, errors.New("CAR unavailable")),
				listRecords: map[string][]ListRecordsRecord{"com.example.record": {{URI: uri, CID: "cid", Value: json.RawMessage(`{"$type":"com.example.record","name":"fallback"}`)}}},
			}
			backfiller.client = client

			count, err := backfiller.safeProcessRepo(t.Context(), data.PDS, data)
			if err != nil || count != 1 {
				t.Fatalf("safeProcessRepo() count=%d error=%v, want 1 nil", count, err)
			}
			if atomic.LoadInt64(&backfiller.stats.ReposProcessed) != 1 || atomic.LoadInt64(&backfiller.stats.Errors) != 0 || atomic.LoadInt64(&client.listCalls) != 1 {
				t.Fatalf("fallback stats = %+v listCalls=%d", backfiller.stats, client.listCalls)
			}

			// Idempotent fallback rerun repairs/checks the same CID without another insert.
			count, err = backfiller.safeProcessRepo(t.Context(), data.PDS, data)
			if err != nil || count != 0 {
				t.Fatalf("fallback rerun count=%d error=%v, want 0 nil", count, err)
			}
			assertValidationMetadata(t, db.Records, uri, validation.StatusValid, "", "hash-current")
			if atomic.LoadInt64(&backfiller.stats.RecordsSkipped) != 1 {
				t.Fatalf("RecordsSkipped = %d, want 1 after idempotent rerun", backfiller.stats.RecordsSkipped)
			}
		})
	}
}

func TestBackfillerContextCancellationDoesNotFallback(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, nil, fakeRecordValidator{})
	defer backfiller.Close()
	client := &fakeBackfillClient{carErr: newCARFailure(CARFailureAvailability, errors.New("unavailable"))}
	backfiller.client = client
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := backfiller.safeProcessRepo(ctx, "https://pds.example", &AtprotoData{DID: "did:plc:test"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("safeProcessRepo() error = %v, want context.Canceled", err)
	}
	if atomic.LoadInt64(&client.listCalls) != 0 || atomic.LoadInt64(&backfiller.stats.Errors) != 0 {
		t.Fatalf("canceled fallback calls=%d errors=%d, want 0/0", client.listCalls, backfiller.stats.Errors)
	}
}

func TestBackfillerRunReturnsStatsAndBoundedIncompleteError(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cfg := DefaultConfig()
	cfg.Collections = []string{"com.example.record"}
	backfiller := NewBackfiller(cfg, db.Records, db.Actors, nil, fakeRecordValidator{})
	defer backfiller.Close()
	backfiller.client = &fakeBackfillClient{listReposErr: errors.New("relay unavailable")}

	stats, err := backfiller.Run(t.Context())
	if stats == nil || err == nil || !strings.Contains(err.Error(), "discover collection") {
		t.Fatalf("Run() stats=%v error=%v, want populated stats and incomplete error", stats, err)
	}
	if stats.Errors != 1 || stats.EndTime.IsZero() {
		t.Fatalf("Run() stats = %+v, want Errors=1 and EndTime", stats)
	}
}

type fakeBackfillClient struct {
	data         *AtprotoData
	carRecords   []CARRecord
	carErr       error
	getRepo      func(context.Context, string, string, []string) ([]CARRecord, error)
	listRecords  map[string][]ListRecordsRecord
	listErrors   map[string]error
	listRepos    []string
	listReposErr error
	listCalls    int64
}

func (c *fakeBackfillClient) ListReposByCollection(context.Context, string) ([]string, error) {
	return c.listRepos, c.listReposErr
}

func (c *fakeBackfillClient) ResolveDID(context.Context, string) (*AtprotoData, error) {
	return c.data, nil
}

func (c *fakeBackfillClient) GetRepo(ctx context.Context, pdsURL, did string, collections []string) ([]CARRecord, error) {
	if c.getRepo != nil {
		return c.getRepo(ctx, pdsURL, did, collections)
	}
	return c.carRecords, c.carErr
}

func (c *fakeBackfillClient) ListRecords(_ context.Context, _ string, _ string, collection string) ([]ListRecordsRecord, error) {
	atomic.AddInt64(&c.listCalls, 1)
	if err := c.listErrors[collection]; err != nil {
		return nil, err
	}
	return c.listRecords[collection], nil
}

func testCARRecord(t *testing.T, uri, cid, collection, rkey, recordJSON string) CARRecord {
	t.Helper()
	var record map[string]any
	if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
		t.Fatalf("json.Unmarshal(test CAR record) error = %v", err)
	}
	cbor, err := atdata.MarshalCBOR(record)
	if err != nil {
		t.Fatalf("atdata.MarshalCBOR(test CAR record) error = %v", err)
	}
	return CARRecord{URI: uri, CID: cid, Collection: collection, RKey: rkey, Value: cbor}
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
