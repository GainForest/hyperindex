package validationrefresh_test

import (
	"context"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"

	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
	"github.com/GainForest/hyperindex/internal/validationrefresh"
)

const testLexiconJSON = `{
  "lexicon": 1,
  "id": "com.example.record",
  "defs": {
    "main": {
      "type": "record",
      "key": "tid",
      "record": {
        "type": "object",
        "required": ["name"],
        "properties": {
          "name": {"type": "string"}
        }
      }
    }
  }
}`

type countingValidator struct {
	calls int
}

func (v *countingValidator) ValidateRecord(collection string, rkey string, rawJSON []byte) validation.Result {
	v.calls++
	return validation.Result{Status: validation.StatusValid, LexiconHash: "hash-current"}
}

func (v *countingValidator) LexiconHash(collection string) (string, bool) {
	if collection != "com.example.record" {
		return "", false
	}
	return "hash-current", true
}

type blockingValidator struct {
	started chan struct{}
	resume  chan struct{}
}

func (v *blockingValidator) ValidateRecord(string, string, []byte) validation.Result {
	close(v.started)
	<-v.resume
	return validation.Result{Status: validation.StatusValid, LexiconHash: "hash-current"}
}

func (v *blockingValidator) LexiconHash(collection string) (string, bool) {
	return "hash-current", collection == "com.example.record"
}

func TestRefreshCollectionDoesNotOverwriteConcurrentIngestionMetadata(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	validator := &blockingValidator{started: make(chan struct{}), resume: make(chan struct{})}
	scheduler := validationrefresh.NewScheduler(db.Records, validator)
	uri := "at://did:plc:test/com.example.record/concurrent"
	if _, err := db.Records.Insert(ctx, uri, "cid-old", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"old"}`); err != nil {
		t.Fatalf("Insert(old) error = %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- scheduler.RefreshCollection(ctx, "com.example.record", "test interleaving")
	}()
	select {
	case <-validator.started:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not reach validation")
	}
	resumed := false
	defer func() {
		if !resumed {
			close(validator.resume)
		}
	}()

	const replacementJSON = `{"$type":"com.example.record","name":123}`
	if err := db.Records.BatchUpsertWithValidation(ctx, []repositories.RecordWrite{{
		URI: uri, CID: "cid-new", DID: "did:plc:test", Collection: "com.example.record", RKey: "concurrent", JSON: replacementJSON,
		ValidationStatus: validation.StatusInvalid, ValidationError: "replacement is invalid", LexiconHash: "hash-current",
	}}); err != nil {
		t.Fatalf("BatchUpsertWithValidation(replacement) error = %v", err)
	}
	close(validator.resume)
	resumed = true

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("RefreshCollection() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("refresh did not complete")
	}

	stored, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if stored.CID != "cid-new" || stored.JSON != replacementJSON {
		t.Fatalf("current content overwritten: CID=%q JSON=%s", stored.CID, stored.JSON)
	}
	if stored.ValidationStatus != validation.StatusInvalid || stored.ValidationError != "replacement is invalid" || stored.LexiconHash != "hash-current" {
		t.Fatalf("current validation metadata overwritten: status=%q error=%q hash=%q", stored.ValidationStatus, stored.ValidationError, stored.LexiconHash)
	}
}

func TestRefreshCollectionSkipsCurrentHashInvalidRows(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	validator := &countingValidator{}
	scheduler := validationrefresh.NewScheduler(db.Records, validator)
	uri := "at://did:plc:test/com.example.record/current-invalid"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"name":123}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(ctx, uri, validation.StatusInvalid, "bad", "hash-current"); err != nil {
		t.Fatalf("UpdateValidationStatus() error = %v", err)
	}
	if err := scheduler.RefreshCollection(ctx, "com.example.record", "test"); err != nil {
		t.Fatalf("RefreshCollection() error = %v", err)
	}
	if validator.calls != 0 {
		t.Fatalf("ValidateRecord calls = %d, want 0 for current-hash invalid row", validator.calls)
	}
}

func TestRefreshCollectionsMarksAbsentCollectionsUnknown(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	validator := &countingValidator{}
	scheduler := validationrefresh.NewScheduler(db.Records, validator)
	removedURI := "at://did:plc:test/com.example.removed/one"
	if _, err := db.Records.Insert(ctx, removedURI, "cid", "did:plc:test", "com.example.removed", `{"name":"old"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(ctx, removedURI, validation.StatusValid, "", "old-hash"); err != nil {
		t.Fatalf("UpdateValidationStatus() error = %v", err)
	}
	if err := scheduler.RefreshCollections(ctx, []string{"com.example.record"}, "test"); err != nil {
		t.Fatalf("RefreshCollections() error = %v", err)
	}
	stored, err := db.Records.GetByURI(ctx, removedURI)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if stored.ValidationStatus != validation.StatusUnknownSchema || stored.LexiconHash != "" || stored.ValidatedAt == nil {
		t.Fatalf("removed collection metadata = status:%q hash:%q at:%v", stored.ValidationStatus, stored.LexiconHash, stored.ValidatedAt)
	}
}

func TestRefreshCollectionClassifiesRecords(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	validator, err := validation.NewValidatorFromLexiconBytes(map[string][]byte{
		"com.example.record": []byte(testLexiconJSON),
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}
	scheduler := validationrefresh.NewScheduler(db.Records, validator)

	validURI := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	invalidURI := "at://did:plc:test/com.example.record/3jui7kd54zh3z"
	if _, err := db.Records.Insert(ctx, validURI, "cid-valid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"ok"}`); err != nil {
		t.Fatalf("Insert(valid) error = %v", err)
	}
	if _, err := db.Records.Insert(ctx, invalidURI, "cid-invalid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":123}`); err != nil {
		t.Fatalf("Insert(invalid) error = %v", err)
	}

	if err := scheduler.RefreshCollection(ctx, "com.example.record", "test"); err != nil {
		t.Fatalf("RefreshCollection() error = %v", err)
	}

	valid, err := db.Records.GetByURI(ctx, validURI)
	if err != nil {
		t.Fatalf("GetByURI(valid) error = %v", err)
	}
	if valid.ValidationStatus != validation.StatusValid {
		t.Fatalf("valid status = %q, want %q", valid.ValidationStatus, validation.StatusValid)
	}
	if valid.LexiconHash == "" || valid.ValidatedAt == nil {
		t.Fatalf("valid metadata missing: hash=%q validatedAt=%v", valid.LexiconHash, valid.ValidatedAt)
	}

	invalid, err := db.Records.GetByURI(ctx, invalidURI)
	if err != nil {
		t.Fatalf("GetByURI(invalid) error = %v", err)
	}
	if invalid.ValidationStatus != validation.StatusInvalid {
		t.Fatalf("invalid status = %q, want %q", invalid.ValidationStatus, validation.StatusInvalid)
	}
	if invalid.ValidationError == "" {
		t.Fatal("invalid ValidationError is empty")
	}
	if invalid.LexiconHash != valid.LexiconHash {
		t.Fatalf("invalid hash = %q, want %q", invalid.LexiconHash, valid.LexiconHash)
	}
}

func TestRefreshCollectionMarksUnknownWhenHashMissing(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	validator, err := validation.NewValidatorFromLexiconBytes(nil)
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}
	scheduler := validationrefresh.NewScheduler(db.Records, validator)
	uri := "at://did:plc:test/com.example.unknown/one"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.unknown", `{"name":"ok"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	if err := scheduler.RefreshCollection(ctx, "com.example.unknown", "test"); err != nil {
		t.Fatalf("RefreshCollection() error = %v", err)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusUnknownSchema {
		t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, validation.StatusUnknownSchema)
	}
	if rec.ValidationError == "" || rec.LexiconHash != "" || rec.ValidatedAt == nil {
		t.Fatalf("unexpected unknown metadata: error=%q hash=%q validatedAt=%v", rec.ValidationError, rec.LexiconHash, rec.ValidatedAt)
	}
}
