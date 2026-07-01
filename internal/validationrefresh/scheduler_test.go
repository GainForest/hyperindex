package validationrefresh_test

import (
	"context"
	"testing"

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
	if _, err := db.Records.Insert(ctx, validURI, "cid-valid", "did:plc:test", "com.example.record", `{"name":"ok"}`); err != nil {
		t.Fatalf("Insert(valid) error = %v", err)
	}
	if _, err := db.Records.Insert(ctx, invalidURI, "cid-invalid", "did:plc:test", "com.example.record", `{"name":123}`); err != nil {
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
