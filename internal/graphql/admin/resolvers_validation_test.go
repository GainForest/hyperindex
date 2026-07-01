package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
	"github.com/GainForest/hyperindex/internal/validationrefresh"
)

const validationGateTestLexicon = `{
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

func TestDeleteLexiconMarksCollectionUnknownSchema(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	if err := db.Lexicons.Upsert(ctx, "com.example.record", validationGateTestLexicon); err != nil {
		t.Fatalf("Upsert lexicon error = %v", err)
	}
	uri := "at://did:plc:test/com.example.record/one"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(ctx, uri, validation.StatusValid, "", "old-hash"); err != nil {
		t.Fatalf("UpdateValidationStatus error = %v", err)
	}

	registry := lexicon.NewRegistry()
	parsed, err := lexicon.ParseBytes([]byte(validationGateTestLexicon))
	if err != nil {
		t.Fatalf("ParseBytes error = %v", err)
	}
	registry.Register(parsed)
	validator := validation.NewValidator(registry, map[string]string{"com.example.record": "old-hash"})
	resolver := newValidationTestResolver(db)
	resolver.SetValidationRefresh(registry, validator, validationrefresh.NewScheduler(db.Records, validator))

	ok, err := resolver.DeleteLexicon(ctx, "com.example.record")
	if err != nil {
		t.Fatalf("DeleteLexicon error = %v", err)
	}
	if !ok {
		t.Fatal("DeleteLexicon ok = false, want true")
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusUnknownSchema {
		t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, validation.StatusUnknownSchema)
	}
	if rec.ValidationError != "lexicon removed for collection" {
		t.Fatalf("ValidationError = %q", rec.ValidationError)
	}
	if rec.LexiconHash != "" {
		t.Fatalf("LexiconHash = %q, want empty", rec.LexiconHash)
	}
	if rec.ValidatedAt == nil {
		t.Fatal("ValidatedAt is nil, want timestamp")
	}
	if _, ok := validator.LexiconHash("com.example.record"); ok {
		t.Fatal("validator still has lexicon hash after delete")
	}
	if _, ok := registry.GetRecordDef("com.example.record"); ok {
		t.Fatal("registry still has record definition after delete")
	}
}

func TestUploadLexiconsUpdatesValidationAndSchedulesRefresh(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	uri := "at://did:plc:test/com.example.record/one"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}

	registry := lexicon.NewRegistry()
	validator := validation.NewValidator(registry, nil)
	resolver := newValidationTestResolver(db)
	resolver.SetValidationRefresh(registry, validator, validationrefresh.NewScheduler(db.Records, validator))

	count, err := resolver.UploadLexicons(ctx, zippedLexiconBase64(t, "com/example/record.json", validationGateTestLexicon))
	if err != nil {
		t.Fatalf("UploadLexicons error = %v", err)
	}
	if count != 1 {
		t.Fatalf("UploadLexicons count = %d, want 1", count)
	}
	wantHash := validation.HashLexiconJSON([]byte(validationGateTestLexicon))
	if gotHash, ok := validator.LexiconHash("com.example.record"); !ok || gotHash != wantHash {
		t.Fatalf("validator hash = %q, %v; want %q, true", gotHash, ok, wantHash)
	}
	if _, ok := registry.GetRecordDef("com.example.record"); !ok {
		t.Fatal("registry missing uploaded record definition")
	}

	waitForRecordStatus(t, db.Records, uri, validation.StatusValid)
}

func TestRegisterLexiconUpdatesValidationAndSchedulesRefresh(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	uri := "at://did:plc:test/com.example.record/one"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}

	previousResolver := newLexiconResolver
	newLexiconResolver = func() lexiconResolver {
		return fakeLexiconResolver{schema: []byte(validationGateTestLexicon)}
	}
	t.Cleanup(func() { newLexiconResolver = previousResolver })

	registry := lexicon.NewRegistry()
	validator := validation.NewValidator(registry, nil)
	resolver := newValidationTestResolver(db)
	resolver.SetValidationRefresh(registry, validator, validationrefresh.NewScheduler(db.Records, validator))

	result, err := resolver.RegisterLexicon(ctx, "com.example.record")
	if err != nil {
		t.Fatalf("RegisterLexicon error = %v", err)
	}
	if result["id"] != "com.example.record" {
		t.Fatalf("registered id = %v, want com.example.record", result["id"])
	}
	wantHash := validation.HashLexiconJSON([]byte(validationGateTestLexicon))
	if gotHash, ok := validator.LexiconHash("com.example.record"); !ok || gotHash != wantHash {
		t.Fatalf("validator hash = %q, %v; want %q, true", gotHash, ok, wantHash)
	}
	if _, ok := registry.GetRecordDef("com.example.record"); !ok {
		t.Fatal("registry missing registered record definition")
	}

	waitForRecordStatus(t, db.Records, uri, validation.StatusValid)
}

func newValidationTestResolver(db *testutil.TestDB) *Resolver {
	return NewResolver(&Repositories{
		Records:      db.Records,
		Actors:       db.Actors,
		Lexicons:     db.Lexicons,
		Config:       db.Config,
		OAuthClients: db.OAuthClients,
		Activity:     db.Activity,
	}, "did:web:example.com", nil)
}

func zippedLexiconBase64(t *testing.T, name string, body string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip Create error = %v", err)
	}
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("zip Write error = %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close error = %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func waitForRecordStatus(t *testing.T, repo *repositories.RecordsRepository, uri string, want validation.Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rec, err := repo.GetByURI(context.Background(), uri)
		if err != nil {
			t.Fatalf("GetByURI error = %v", err)
		}
		if rec.ValidationStatus == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	rec, err := repo.GetByURI(context.Background(), uri)
	if err != nil {
		t.Fatalf("GetByURI final error = %v", err)
	}
	t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, want)
}

type fakeLexiconResolver struct {
	schema []byte
}

func (r fakeLexiconResolver) ResolveLexicon(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error) {
	return &lexicon.ResolvedLexicon{
		NSID:   nsid,
		DID:    "did:plc:resolver",
		PDSUrl: "https://pds.example",
		Schema: r.schema,
	}, nil
}
