package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"sync"
	"testing"

	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
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

func TestDeleteLexiconStagesChangeUntilRestart(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	if err := db.Lexicons.Upsert(ctx, "com.example.record", validationGateTestLexicon); err != nil {
		t.Fatalf("Upsert Lexicon error = %v", err)
	}
	uri := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}
	if err := db.Records.UpdateValidationStatus(ctx, uri, validation.StatusValid, "", "startup-hash"); err != nil {
		t.Fatalf("UpdateValidationStatus error = %v", err)
	}

	resolver := newValidationTestResolver(db)
	ok, err := resolver.DeleteLexicon(ctx, "com.example.record")
	if err != nil {
		t.Fatalf("DeleteLexicon error = %v", err)
	}
	if !ok {
		t.Fatal("DeleteLexicon ok = false, want true")
	}
	if exists, err := db.Lexicons.Exists(ctx, "com.example.record"); err != nil || exists {
		t.Fatalf("saved Lexicon exists = %v, error = %v; want false, nil", exists, err)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusValid || rec.LexiconHash != "startup-hash" {
		t.Fatalf("running validation state changed to status=%q hash=%q; want valid/startup-hash until restart", rec.ValidationStatus, rec.LexiconHash)
	}
}

func TestUploadLexiconsStagesChangeUntilRestart(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	uri := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}

	resolver := newValidationTestResolver(db)
	count, err := resolver.UploadLexicons(ctx, zippedLexiconBase64(t, "com/example/record.json", validationGateTestLexicon))
	if err != nil {
		t.Fatalf("UploadLexicons error = %v", err)
	}
	if count != 1 {
		t.Fatalf("UploadLexicons count = %d, want 1", count)
	}
	if exists, err := db.Lexicons.Exists(ctx, "com.example.record"); err != nil || !exists {
		t.Fatalf("saved Lexicon exists = %v, error = %v; want true, nil", exists, err)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusUnknownSchema {
		t.Fatalf("ValidationStatus = %q, want %q until restart", rec.ValidationStatus, validation.StatusUnknownSchema)
	}
}

func TestRegisterLexiconStagesChangeUntilRestart(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	uri := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"ok"}`); err != nil {
		t.Fatalf("Insert record error = %v", err)
	}

	previousResolver := newLexiconResolver
	newLexiconResolver = func() lexiconResolver {
		return fakeLexiconResolver{schema: []byte(validationGateTestLexicon)}
	}
	t.Cleanup(func() { newLexiconResolver = previousResolver })

	resolver := newValidationTestResolver(db)
	result, err := resolver.RegisterLexicon(ctx, "com.example.record")
	if err != nil {
		t.Fatalf("RegisterLexicon error = %v", err)
	}
	if result["id"] != "com.example.record" {
		t.Fatalf("registered id = %v, want com.example.record", result["id"])
	}
	if exists, err := db.Lexicons.Exists(ctx, "com.example.record"); err != nil || !exists {
		t.Fatalf("saved Lexicon exists = %v, error = %v; want true, nil", exists, err)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusUnknownSchema {
		t.Fatalf("ValidationStatus = %q, want %q until restart", rec.ValidationStatus, validation.StatusUnknownSchema)
	}
}

func TestUploadLexiconsAcceptsRecordKeyCompatibility(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	recordKeyLexicon := strings.Replace(validationGateTestLexicon, `"key": "tid"`, `"key": "record-key"`, 1)

	resolver := newValidationTestResolver(db)
	count, err := resolver.UploadLexicons(ctx, zippedLexiconBase64(t, "record-key.json", recordKeyLexicon))
	if err != nil {
		t.Fatalf("UploadLexicons(record-key) error = %v", err)
	}
	if count != 1 {
		t.Fatalf("UploadLexicons(record-key) count = %d, want 1", count)
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "com.example.record"); existsErr != nil || !exists {
		t.Fatalf("record-key Lexicon exists = %v, error = %v; want true, nil", exists, existsErr)
	}
}

func TestUploadLexiconsRejectsInvalidSchemaBeforeSaving(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	invalid := `{"lexicon":1,"id":"com.example.invalid","defs":{"main":{"type":"record","record":{"type":"object","properties":{}}}}}`

	resolver := newValidationTestResolver(db)
	count, err := resolver.UploadLexicons(ctx, zippedLexiconsBase64(t,
		zipLexicon{name: "valid.json", body: validationGateTestLexicon},
		zipLexicon{name: "invalid.json", body: invalid},
	))
	if err == nil || !strings.Contains(err.Error(), "record key specifier is required") {
		t.Fatalf("UploadLexicons count=%d error=%v, want Indigo schema error", count, err)
	}
	if count != 0 {
		t.Fatalf("UploadLexicons count = %d, want 0", count)
	}
	for _, id := range []string{"com.example.record", "com.example.invalid"} {
		if exists, existsErr := db.Lexicons.Exists(ctx, id); existsErr != nil || exists {
			t.Fatalf("Lexicon %s exists = %v, error = %v; want false, nil", id, exists, existsErr)
		}
	}
}

func TestUploadLexiconsValidatesProspectiveReferenceSet(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newValidationTestResolver(db)
	record := `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","properties":{"helper":{"type":"ref","ref":"com.example.helper"}}}}}}`
	helper := `{"lexicon":1,"id":"com.example.helper","defs":{"main":{"type":"object","properties":{"name":{"type":"string"}}}}}`

	count, err := resolver.UploadLexicons(ctx, zippedLexiconsBase64(t,
		zipLexicon{name: "record.json", body: record},
		zipLexicon{name: "helper.json", body: helper},
	))
	if err != nil {
		t.Fatalf("UploadLexicons(valid reference set) error = %v", err)
	}
	if count != 2 {
		t.Fatalf("UploadLexicons(valid reference set) count = %d, want 2", count)
	}

	missing := strings.Replace(record, `"id":"com.example.record"`, `"id":"com.example.record2"`, 1)
	missing = strings.ReplaceAll(missing, "com.example.helper", "com.example.missing")
	count, err = resolver.UploadLexicons(ctx, zippedLexiconBase64(t, "missing.json", missing))
	if err == nil || !strings.Contains(err.Error(), "com.example.missing") {
		t.Fatalf("UploadLexicons(missing ref) count=%d error=%v, want missing reference error", count, err)
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "com.example.record2"); existsErr != nil || exists {
		t.Fatalf("invalid prospective Lexicon exists = %v, error = %v; want false, nil", exists, existsErr)
	}
}

func TestConcurrentLexiconMutationsKeepProspectiveSetValid(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	helper := `{"lexicon":1,"id":"com.example.helper","defs":{"main":{"type":"object","properties":{"name":{"type":"string"}}}}}`
	record := `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","properties":{"helper":{"type":"ref","ref":"com.example.helper"}}}}}}`
	if err := db.Lexicons.Upsert(ctx, "com.example.helper", helper); err != nil {
		t.Fatalf("Upsert(helper) error = %v", err)
	}

	uploadResolver := newValidationTestResolver(db)
	deleteResolver := newValidationTestResolver(db)
	upload := zippedLexiconBase64(t, "record.json", record)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := uploadResolver.UploadLexicons(ctx, upload)
		results <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := deleteResolver.DeleteLexicon(ctx, "com.example.helper")
		results <- err
	}()
	close(start)
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent mutation successes = %d, want exactly one", successes)
	}

	saved, err := db.Lexicons.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	savedBytes := make(map[string][]byte, len(saved))
	for _, item := range saved {
		savedBytes[item.ID] = []byte(item.JSON)
	}
	if err := validation.CheckLexiconSet(savedBytes); err != nil {
		t.Fatalf("concurrent mutations left an invalid saved set: %v", err)
	}
}

func TestDeleteLexiconRejectsBrokenProspectiveSet(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	helper := `{"lexicon":1,"id":"com.example.helper","defs":{"main":{"type":"object","properties":{"name":{"type":"string"}}}}}`
	record := `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","properties":{"helper":{"type":"ref","ref":"com.example.helper"}}}}}}`
	for id, raw := range map[string]string{"com.example.helper": helper, "com.example.record": record} {
		if err := db.Lexicons.Upsert(ctx, id, raw); err != nil {
			t.Fatalf("Upsert(%s) error = %v", id, err)
		}
	}

	resolver := newValidationTestResolver(db)
	ok, err := resolver.DeleteLexicon(ctx, "com.example.helper")
	if err == nil || !strings.Contains(err.Error(), "would make the next startup schema invalid") {
		t.Fatalf("DeleteLexicon() ok=%v error=%v, want prospective schema error", ok, err)
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "com.example.helper"); existsErr != nil || !exists {
		t.Fatalf("dependent helper exists = %v, error = %v; want true, nil", exists, existsErr)
	}
}

func TestRegisterLexiconRejectsIndigoInvalidSchemaBeforeSaving(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	invalid := `{"lexicon":1,"id":"com.example.invalid","defs":{"main":{"type":"record","record":{"type":"object","properties":{}}}}}`

	previousResolver := newLexiconResolver
	newLexiconResolver = func() lexiconResolver {
		return fakeLexiconResolver{schema: []byte(invalid)}
	}
	t.Cleanup(func() { newLexiconResolver = previousResolver })

	resolver := newValidationTestResolver(db)
	_, err := resolver.RegisterLexicon(ctx, "com.example.invalid")
	if err == nil || !strings.Contains(err.Error(), "record key specifier is required") {
		t.Fatalf("RegisterLexicon error = %v, want Indigo schema error", err)
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "com.example.invalid"); existsErr != nil || exists {
		t.Fatalf("invalid registered Lexicon exists = %v, error = %v; want false, nil", exists, existsErr)
	}
}

func TestRegisterLexiconRejectsMismatchedIDBeforeSaving(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)

	previousResolver := newLexiconResolver
	newLexiconResolver = func() lexiconResolver {
		return fakeLexiconResolver{schema: []byte(validationGateTestLexicon)}
	}
	t.Cleanup(func() { newLexiconResolver = previousResolver })

	resolver := newValidationTestResolver(db)
	_, err := resolver.RegisterLexicon(ctx, "com.example.different")
	if err == nil || !strings.Contains(err.Error(), "lexicon ID mismatch") {
		t.Fatalf("RegisterLexicon error = %v, want ID mismatch", err)
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "com.example.different"); existsErr != nil || exists {
		t.Fatalf("mismatched Lexicon exists = %v, error = %v; want false, nil", exists, existsErr)
	}
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

type zipLexicon struct {
	name string
	body string
}

func zippedLexiconBase64(t *testing.T, name string, body string) string {
	t.Helper()
	return zippedLexiconsBase64(t, zipLexicon{name: name, body: body})
}

func zippedLexiconsBase64(t *testing.T, lexicons ...zipLexicon) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, lexicon := range lexicons {
		w, err := zw.Create(lexicon.name)
		if err != nil {
			t.Fatalf("zip Create error = %v", err)
		}
		if _, err := w.Write([]byte(lexicon.body)); err != nil {
			t.Fatalf("zip Write error = %v", err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip Close error = %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
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
