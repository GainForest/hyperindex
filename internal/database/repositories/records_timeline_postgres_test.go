package repositories_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/validation"
)

func TestRecordsRepository_ValidationMetadataPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()
	collection := "com.example.record"
	validURI := "at://did:plc:test/com.example.record/01-valid"
	staleURI := "at://did:plc:test/com.example.record/02-stale"
	invalidURI := "at://did:plc:test/com.example.record/03-invalid"
	otherURI := "at://did:plc:test/com.example.other/01-valid"

	for _, rec := range []struct {
		uri        string
		collection string
		status     validation.Status
		errorText  string
		hash       string
	}{
		{validURI, collection, validation.StatusValid, "", "hash-current"},
		{staleURI, collection, validation.StatusValid, "", "hash-old"},
		{invalidURI, collection, validation.StatusInvalid, "missing required field: name", "hash-current"},
		{otherURI, "com.example.other", validation.StatusValid, "", "hash-other"},
	} {
		if _, err := repo.Insert(ctx, rec.uri, "cid", "did:plc:test", rec.collection, `{"name":"test"}`); err != nil {
			t.Fatalf("Insert(%s) error = %v", rec.uri, err)
		}
		if err := repo.UpdateValidationStatus(ctx, rec.uri, rec.status, rec.errorText, rec.hash); err != nil {
			t.Fatalf("UpdateValidationStatus(%s) error = %v", rec.uri, err)
		}
	}

	invalid, err := repo.GetByURI(ctx, invalidURI)
	if err != nil {
		t.Fatalf("GetByURI(invalid) error = %v", err)
	}
	if invalid.ValidationStatus != validation.StatusInvalid || invalid.ValidationError == "" || invalid.LexiconHash != "hash-current" || invalid.ValidatedAt == nil {
		t.Fatalf("invalid record metadata = status:%q error:%q hash:%q validatedAt:%v", invalid.ValidationStatus, invalid.ValidationError, invalid.LexiconHash, invalid.ValidatedAt)
	}

	needingValidation, err := repo.ListRecordsNeedingValidation(ctx, collection, "hash-current", "", 10)
	if err != nil {
		t.Fatalf("ListRecordsNeedingValidation() error = %v", err)
	}
	assertRecordURIs(t, needingValidation, []string{staleURI})

	if err := repo.MarkCollectionUnknownSchema(ctx, collection, "lexicon removed for collection"); err != nil {
		t.Fatalf("MarkCollectionUnknownSchema() error = %v", err)
	}
	for _, uri := range []string{validURI, staleURI, invalidURI} {
		rec, err := repo.GetByURI(ctx, uri)
		if err != nil {
			t.Fatalf("GetByURI(%s) error = %v", uri, err)
		}
		if rec.ValidationStatus != validation.StatusUnknownSchema || rec.ValidationError != "lexicon removed for collection" || rec.LexiconHash != "" || rec.ValidatedAt == nil {
			t.Fatalf("%s metadata = status:%q error:%q hash:%q validatedAt:%v", uri, rec.ValidationStatus, rec.ValidationError, rec.LexiconHash, rec.ValidatedAt)
		}
	}
	other, err := repo.GetByURI(ctx, otherURI)
	if err != nil {
		t.Fatalf("GetByURI(other) error = %v", err)
	}
	if other.ValidationStatus != validation.StatusValid || other.LexiconHash != "hash-other" {
		t.Fatalf("other collection metadata changed: status=%q hash=%q", other.ValidationStatus, other.LexiconHash)
	}
}

func TestRecordsRepository_UpdateValidationStatusIfUnchangedPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()
	const collection = "com.example.record"
	const observedJSON = `{"name":"observed"}`

	t.Run("matching URI CID and JSON updates metadata", func(t *testing.T) {
		uri := "at://did:plc:test/com.example.record/conditional-match"
		if _, err := repo.Insert(ctx, uri, "cid-observed", "did:plc:test", collection, observedJSON); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		observed, err := repo.GetByURI(ctx, uri)
		if err != nil {
			t.Fatalf("GetByURI() error = %v", err)
		}
		updated, err := repo.UpdateValidationStatusIfUnchanged(ctx, observed, validation.StatusInvalid, "observed invalid", "hash-observed")
		if err != nil || !updated {
			t.Fatalf("conditional update = %v, %v; want true, nil", updated, err)
		}
		assertPostgresConditionalValidationMetadata(t, repo, uri, "cid-observed", observedJSON, validation.StatusInvalid, "observed invalid", "hash-observed")
	})

	t.Run("changed CID preserves current metadata", func(t *testing.T) {
		uri := "at://did:plc:test/com.example.record/conditional-cid"
		if _, err := repo.Insert(ctx, uri, "cid-observed", "did:plc:test", collection, observedJSON); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		observed, err := repo.GetByURI(ctx, uri)
		if err != nil {
			t.Fatalf("GetByURI() error = %v", err)
		}
		if err := repo.BatchUpsertWithValidation(ctx, []repositories.RecordWrite{{
			URI: uri, CID: "cid-current", DID: "did:plc:test", Collection: collection, RKey: "conditional-cid", JSON: observedJSON,
			ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
		}}); err != nil {
			t.Fatalf("BatchUpsertWithValidation() error = %v", err)
		}
		updated, err := repo.UpdateValidationStatusIfUnchanged(ctx, observed, validation.StatusInvalid, "stale invalid", "hash-stale")
		if err != nil || updated {
			t.Fatalf("conditional update after CID replacement = %v, %v; want false, nil", updated, err)
		}
		assertPostgresConditionalValidationMetadata(t, repo, uri, "cid-current", observedJSON, validation.StatusValid, "", "hash-current")
	})

	t.Run("changed JSON preserves current metadata", func(t *testing.T) {
		uri := "at://did:plc:test/com.example.record/conditional-json"
		if _, err := repo.Insert(ctx, uri, "cid-observed", "did:plc:test", collection, observedJSON); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		observed, err := repo.GetByURI(ctx, uri)
		if err != nil {
			t.Fatalf("GetByURI() error = %v", err)
		}
		const currentJSON = `{"name":"current"}`
		if err := repo.BatchUpsertWithValidation(ctx, []repositories.RecordWrite{{
			URI: uri, CID: "cid-observed", DID: "did:plc:test", Collection: collection, RKey: "conditional-json", JSON: currentJSON,
			ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
		}}); err != nil {
			t.Fatalf("BatchUpsertWithValidation() error = %v", err)
		}
		updated, err := repo.UpdateValidationStatusIfUnchanged(ctx, observed, validation.StatusInvalid, "stale invalid", "hash-stale")
		if err != nil || updated {
			t.Fatalf("conditional update after JSON replacement = %v, %v; want false, nil", updated, err)
		}
		assertPostgresConditionalValidationMetadata(t, repo, uri, "cid-observed", currentJSON, validation.StatusValid, "", "hash-current")
	})
}

func assertPostgresConditionalValidationMetadata(t *testing.T, repo *repositories.RecordsRepository, uri, cid, rawJSON string, status validation.Status, validationError, lexiconHash string) {
	t.Helper()
	stored, err := repo.GetByURI(context.Background(), uri)
	if err != nil {
		t.Fatalf("GetByURI(%s) error = %v", uri, err)
	}
	if stored.CID != cid || stored.ValidationStatus != status || stored.ValidationError != validationError || stored.LexiconHash != lexiconHash || stored.ValidatedAt == nil {
		t.Fatalf("record = cid:%q json:%s status:%q error:%q hash:%q at:%v", stored.CID, stored.JSON, stored.ValidationStatus, stored.ValidationError, stored.LexiconHash, stored.ValidatedAt)
	}
	var gotJSON, wantJSON interface{}
	if err := json.Unmarshal([]byte(stored.JSON), &gotJSON); err != nil {
		t.Fatalf("stored JSON is invalid: %v", err)
	}
	if err := json.Unmarshal([]byte(rawJSON), &wantJSON); err != nil {
		t.Fatalf("expected JSON is invalid: %v", err)
	}
	if !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("stored JSON = %s, want semantic value %s", stored.JSON, rawJSON)
	}
}

func TestRecordsRepository_BackfillBatchDuplicateURIsPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()

	t.Run("identical duplicate within chunk", func(t *testing.T) {
		first := repositories.RecordWrite{
			URI: "at://did:plc:duplicate/com.example.duplicate/within", CID: "cid-within", DID: "did:plc:duplicate", Collection: "com.example.duplicate", RKey: "within", JSON: `{"name":"first"}`,
			ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
		}
		duplicate := first
		duplicate.JSON = "{\n  \"name\": \"same CID\"\n}"
		result, err := repo.BatchUpsertWithValidationForBackfill(ctx, first.DID, []repositories.RecordWrite{first, duplicate})
		if err != nil || len(result.ChangedIndices) != 1 || result.ChangedIndices[0] != 0 || result.Skipped != 0 {
			t.Fatalf("duplicate result=%+v error=%v, want [0]/0 nil", result, err)
		}
		assertPostgresRecordJSON(t, repo, first.URI, first.JSON)
	})

	t.Run("identical duplicate across chunk boundary", func(t *testing.T) {
		const did = "did:plc:duplicate-boundary"
		const collection = "com.example.duplicateboundary"
		first := repositories.RecordWrite{
			URI: "at://did:plc:duplicate-boundary/com.example.duplicateboundary/first", CID: "cid-first", DID: did, Collection: collection, RKey: "first", JSON: `{"name":"first"}`,
			ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
		}
		writes := []repositories.RecordWrite{first}
		for i := 1; i < repositories.ValidationBatchUpsertSize; i++ {
			writes = append(writes, repositories.RecordWrite{
				URI: fmt.Sprintf("at://%s/%s/item-%03d", did, collection, i), CID: fmt.Sprintf("cid-%03d", i), DID: did, Collection: collection, RKey: fmt.Sprintf("item-%03d", i), JSON: `{"name":"item"}`,
				ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
			})
		}
		duplicate := first
		duplicate.JSON = `{"name":"same CID"}`
		writes = append(writes, duplicate)
		result, err := repo.BatchUpsertWithValidationForBackfill(ctx, did, writes)
		if err != nil || len(result.ChangedIndices) != repositories.ValidationBatchUpsertSize || result.Skipped != 0 {
			t.Fatalf("boundary duplicate result=%+v error=%v", result, err)
		}
		count, err := repo.GetCollectionCount(ctx, collection)
		if err != nil || count != int64(repositories.ValidationBatchUpsertSize) {
			t.Fatalf("boundary collection count=%d error=%v, want %d nil", count, err, repositories.ValidationBatchUpsertSize)
		}
	})

	t.Run("conflicting duplicate writes nothing", func(t *testing.T) {
		first := repositories.RecordWrite{
			URI: "at://did:plc:duplicate-conflict/com.example.duplicate/conflict", CID: "cid-first", DID: "did:plc:duplicate-conflict", Collection: "com.example.duplicate", RKey: "conflict", JSON: `{"name":"first"}`,
			ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
		}
		conflict := first
		conflict.CID = "cid-second"
		result, err := repo.BatchUpsertWithValidationForBackfill(ctx, first.DID, []repositories.RecordWrite{first, conflict})
		if err == nil || !strings.Contains(err.Error(), first.URI) || !strings.Contains(err.Error(), "CID differs") {
			t.Fatalf("conflicting duplicate result=%+v error=%v, want explicit conflict", result, err)
		}
		if _, err := repo.GetByURI(ctx, first.URI); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("GetByURI() after conflict error=%v, want sql.ErrNoRows", err)
		}
	})
}

func TestRecordsRepository_BackfillBatchSameCIDRepairPreservesContentPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()
	const uri = "at://did:plc:repair/com.example.record/one"
	const storedJSON = `{"name":"stored"}`
	if _, err := repo.Insert(ctx, uri, "cid-same", "did:plc:repair", "com.example.record", storedJSON); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := exec.DB().ExecContext(ctx, "UPDATE record SET indexed_at = $1::timestamptz WHERE uri = $2", "2026-01-15T10:00:00.123Z", uri); err != nil {
		t.Fatalf("set indexed_at: %v", err)
	}
	before, err := repo.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI(before) error = %v", err)
	}
	write := repositories.RecordWrite{
		URI: uri, CID: "cid-same", DID: "did:plc:repair", Collection: "com.example.record", RKey: "one", JSON: `{"name":"incoming"}`,
		ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
	}
	result, err := repo.BatchUpsertWithValidationForBackfill(ctx, write.DID, []repositories.RecordWrite{write})
	if err != nil || len(result.ChangedIndices) != 0 || result.Skipped != 1 {
		t.Fatalf("repair result=%+v error=%v, want none/1 nil", result, err)
	}
	after, err := repo.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI(after) error = %v", err)
	}
	if !after.IndexedAt.Equal(before.IndexedAt) || after.ValidationStatus != validation.StatusValid || after.LexiconHash != "hash-current" || after.ValidatedAt == nil {
		t.Fatalf("repair changed indexed/content metadata: before=%s after=%s status=%q hash=%q at=%v", before.IndexedAt, after.IndexedAt, after.ValidationStatus, after.LexiconHash, after.ValidatedAt)
	}
	assertPostgresRecordJSON(t, repo, uri, storedJSON)
}

func assertPostgresRecordJSON(t *testing.T, repo *repositories.RecordsRepository, uri, wantJSON string) {
	t.Helper()
	stored, err := repo.GetByURI(context.Background(), uri)
	if err != nil {
		t.Fatalf("GetByURI(%s) error = %v", uri, err)
	}
	var got, want interface{}
	if err := json.Unmarshal([]byte(stored.JSON), &got); err != nil {
		t.Fatalf("stored JSON invalid: %v", err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("wanted JSON invalid: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stored JSON=%s, want semantic value %s", stored.JSON, wantJSON)
	}
}

func TestRecordsRepository_BackfillBatchConstrainedPoolPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	exec.DB().SetMaxOpenConns(1)
	exec.DB().SetMaxIdleConns(1)
	repo := repositories.NewRecordsRepository(exec)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const operationCount = 4
	start := make(chan struct{})
	errs := make(chan error, operationCount)
	var wg sync.WaitGroup
	for i := 0; i < operationCount; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			did := fmt.Sprintf("did:plc:pool-%d", index)
			uri := fmt.Sprintf("at://%s/com.example.record/one", did)
			result, err := repo.BatchUpsertWithValidationForBackfill(ctx, did, []repositories.RecordWrite{{
				URI: uri, CID: fmt.Sprintf("cid-%d", index), DID: did, Collection: "com.example.record", RKey: "one", JSON: `{"name":"pool"}`,
				ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
			}})
			if err == nil && (len(result.ChangedIndices) != 1 || result.ChangedIndices[0] != 0 || result.Skipped != 0) {
				err = fmt.Errorf("unexpected result: %+v", result)
			}
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("constrained-pool backfill error = %v", err)
		}
	}
}

func TestRecordsRepository_BackfillBatchAdvisoryLockCancellationPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	exec.DB().SetMaxOpenConns(2)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()
	did := "did:plc:advisory-blocked"
	uri := "at://did:plc:advisory-blocked/com.example.record/one"

	holder, err := exec.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTx(holder) error = %v", err)
	}
	defer func() { _ = holder.Rollback() }()
	if _, err := holder.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", postgresBackfillLockKey(did)); err != nil {
		t.Fatalf("acquire holder advisory lock: %v", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	result, err := repo.BatchUpsertWithValidationForBackfill(waitCtx, did, []repositories.RecordWrite{{
		URI: uri, CID: "cid", DID: did, Collection: "com.example.record", RKey: "one", JSON: `{"name":"blocked"}`,
		ValidationStatus: validation.StatusValid, LexiconHash: "hash-current",
	}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked advisory result=%+v error=%v, want context deadline", result, err)
	}
	if len(result.ChangedIndices) != 0 || result.Skipped != 0 {
		t.Fatalf("blocked advisory result = %+v, want zero", result)
	}
	if err := holder.Rollback(); err != nil {
		t.Fatalf("Rollback(holder) error = %v", err)
	}
	if _, err := repo.GetByURI(ctx, uri); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("GetByURI() after advisory cancellation error = %v, want sql.ErrNoRows", err)
	}
}

func postgresBackfillLockKey(did string) int64 {
	hasher := fnv.New64a()
	_, _ = hasher.Write([]byte(did))
	return int64(hasher.Sum64())
}

func TestRecordsRepository_ValidOnlyQueriesPostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()
	collection := "com.example.record"
	validURI := "at://did:plc:test/com.example.record/01-valid"
	invalidURI := "at://did:plc:test/com.example.record/02-invalid"
	otherURI := "at://did:plc:test/com.example.other/01-valid"

	for _, rec := range []struct {
		uri        string
		collection string
		status     validation.Status
	}{
		{validURI, collection, validation.StatusValid},
		{invalidURI, collection, validation.StatusInvalid},
		{otherURI, "com.example.other", validation.StatusValid},
	} {
		if _, err := repo.Insert(ctx, rec.uri, "cid", "did:plc:test", rec.collection, `{"name":"test"}`); err != nil {
			t.Fatalf("Insert(%s) error = %v", rec.uri, err)
		}
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

func TestRecordsRepository_RecordTimelinePostgres(t *testing.T) {
	exec := newPostgresRecordsTestExecutor(t)
	repo := repositories.NewRecordsRepository(exec)
	ctx := context.Background()

	preserveURI := "at://did:plc:timeline/com.example.timeline.post/postgres-preserve"
	if _, err := repo.Insert(ctx, preserveURI, "cid-preserve-1", "did:plc:timeline", "com.example.timeline.post", `{"createdAt":"2026-01-15T10:00:00.123456789+02:00"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if _, err := repo.Insert(ctx, preserveURI, "cid-preserve-2", "did:plc:timeline", "com.example.timeline.post", `{"createdAt":"2026-01-16T10:00:00Z"}`); err != nil {
		t.Fatalf("Insert() update error = %v", err)
	}
	if got := postgresRepositoryRecordCreatedAt(t, exec, preserveURI); got != "2026-01-15T08:00:00.123Z" {
		t.Fatalf("postgres preserved record_created_at = %q, want original normalized timestamp", got)
	}

	fillURI := "at://did:plc:timeline/com.example.timeline.post/postgres-fill"
	if err := repo.BatchInsert(ctx, []*repositories.Record{
		{URI: fillURI, CID: "cid-fill-1", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"text":"missing"}`},
	}); err != nil {
		t.Fatalf("initial BatchInsert() error = %v", err)
	}
	if err := repo.BatchInsert(ctx, []*repositories.Record{
		{URI: fillURI, CID: "cid-fill-2", DID: "did:plc:timeline", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-17T00:00:00Z"}`},
	}); err != nil {
		t.Fatalf("conflicting BatchInsert() error = %v", err)
	}
	if got := postgresRepositoryRecordCreatedAt(t, exec, fillURI); got != "2026-01-17T00:00:00.000Z" {
		t.Fatalf("postgres filled batch record_created_at = %q, want incoming timestamp", got)
	}

	records := []*repositories.Record{
		{URI: "at://did:plc:alice/com.example.timeline.post/r1", CID: "cid1", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-15T10:00:00Z"}`},
		{URI: "at://did:plc:bob/com.example.timeline.like/r2", CID: "cid2", DID: "did:plc:bob", Collection: "com.example.timeline.like", JSON: `{"createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:alice/com.example.timeline.post/r3", CID: "cid3", DID: "did:plc:alice", Collection: "com.example.timeline.post", JSON: `{"createdAt":"2026-01-15T12:00:00Z"}`},
		{URI: "at://did:plc:carol/com.example.timeline.like/r4", CID: "cid4", DID: "did:plc:carol", Collection: "com.example.timeline.like", JSON: `{"createdAt":"2026-01-15T14:00:00Z"}`},
	}
	if err := repo.BatchInsert(ctx, records); err != nil {
		t.Fatalf("timeline BatchInsert() error = %v", err)
	}

	page, err := repo.GetRecordTimeline(ctx, []string{"did:plc:alice", "did:plc:bob"}, []string{"com.example.timeline.post", "com.example.timeline.like"}, 10, nil)
	if err != nil {
		t.Fatalf("GetRecordTimeline() error = %v", err)
	}
	assertTimelineURIs(t, page, []string{
		"at://did:plc:bob/com.example.timeline.like/r2",
		"at://did:plc:alice/com.example.timeline.post/r3",
		"at://did:plc:alice/com.example.timeline.post/r1",
	})
}

func newPostgresRecordsTestExecutor(t *testing.T) *postgres.Executor {
	t.Helper()
	databaseURL, ok := safePostgresRecordsTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL record timeline test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	ctx := context.Background()
	adminExec, err := postgres.NewExecutor(databaseURL)
	if err != nil {
		t.Fatalf("failed to create postgres admin executor: %v", err)
	}
	t.Cleanup(func() { _ = adminExec.Close() })

	schemaName := fmt.Sprintf("hyperindex_records_timeline_test_%d", time.Now().UnixNano())
	quotedSchemaName := quotePostgresRecordsIdentifier(schemaName)
	if _, err := adminExec.DB().ExecContext(ctx, fmt.Sprintf("CREATE SCHEMA %s", quotedSchemaName)); err != nil {
		t.Fatalf("failed to create postgres test schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = adminExec.DB().ExecContext(context.Background(), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", quotedSchemaName))
	})

	schemaURL, err := postgresRecordsURLWithSearchPath(databaseURL, schemaName)
	if err != nil {
		t.Fatalf("failed to build postgres schema URL: %v", err)
	}

	exec, err := postgres.NewExecutor(schemaURL)
	if err != nil {
		t.Fatalf("failed to create postgres schema executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("failed to run postgres migrations: %v", err)
	}
	return exec
}

func safePostgresRecordsTestDatabaseURL(t *testing.T) (string, bool) {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return "", false
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatalf("DATABASE_URL is not a valid URL: %v", err)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	name := strings.ToLower(strings.TrimSpace(databaseName))
	return databaseURL, name == "test" || strings.HasSuffix(name, "_test") || strings.HasSuffix(name, "-test")
}

func postgresRecordsURLWithSearchPath(databaseURL, schemaName string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func quotePostgresRecordsIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func postgresRepositoryRecordCreatedAt(t *testing.T, exec *postgres.Executor, uri string) string {
	t.Helper()
	var value sql.NullString
	if err := exec.DB().QueryRowContext(context.Background(), `
		SELECT to_char(record_created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.MS"Z"')
		FROM record
		WHERE uri = $1`, uri,
	).Scan(&value); err != nil {
		t.Fatalf("failed to query postgres record_created_at for %s: %v", uri, err)
	}
	if !value.Valid {
		return ""
	}
	return value.String
}
