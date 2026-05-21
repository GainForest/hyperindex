package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestResolverUploadLexiconsRejectsMalformedJSONBeforePersistence(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)

	count, err := resolver.UploadLexicons(ctx, zipBase64(t, map[string]string{
		"bad.json": `{"lexicon":1,`,
	}))
	if err == nil {
		t.Fatal("expected malformed upload JSON to fail")
	}
	if count != 0 {
		t.Fatalf("expected upload count 0, got %d", count)
	}
	assertErrorContains(t, err, "bad.json", "did not store any lexicons")
	assertLexiconCount(ctx, t, db, 0)
}

func TestResolverUploadLexiconsRejectsParserInvalidDocumentBeforePersistence(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)

	count, err := resolver.UploadLexicons(ctx, zipBase64(t, map[string]string{
		"invalid.json": `{"lexicon":1,"id":"app.test.invalid","defs":{"main":{"type":"query"}}}`,
	}))
	if err == nil {
		t.Fatal("expected parser-invalid upload lexicon to fail")
	}
	if count != 0 {
		t.Fatalf("expected upload count 0, got %d", count)
	}
	assertErrorContains(t, err, "invalid.json", "unsupported main definition type", "did not store any lexicons")
	assertLexiconCount(ctx, t, db, 0)
}

func TestResolverUploadLexiconsValidatesWholeZIPBeforeWriting(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)

	seedJSON := validAdminLexiconJSON("app.test.seed", "text")
	if err := db.Lexicons.Upsert(ctx, "app.test.seed", seedJSON); err != nil {
		t.Fatalf("failed to seed lexicon: %v", err)
	}

	count, err := resolver.UploadLexicons(ctx, zipBase64Entries(t, []zipEntry{
		{name: "valid.json", contents: validAdminLexiconJSON("app.test.valid", "body")},
		{name: "invalid.json", contents: `{"lexicon":1,"defs":{}}`},
	}))
	if err == nil {
		t.Fatal("expected mixed upload ZIP to fail")
	}
	if count != 0 {
		t.Fatalf("expected upload count 0, got %d", count)
	}
	assertErrorContains(t, err, "invalid.json", "did not store any lexicons")
	assertLexiconCount(ctx, t, db, 1)
	assertLexiconExists(ctx, t, db, "app.test.seed", true)
	assertLexiconExists(ctx, t, db, "app.test.valid", false)
}

func TestResolverUploadLexiconsStoresValidDocuments(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	callbackCalls := 0
	resolver.SetLexiconChangeCallback(func(collections []string) error {
		callbackCalls++
		if len(collections) != 2 {
			t.Fatalf("expected callback collections length 2, got %d", len(collections))
		}
		return nil
	})

	count, err := resolver.UploadLexicons(ctx, zipBase64(t, map[string]string{
		"one.json": validAdminLexiconJSON("app.test.one", "text"),
		"two.json": validAdminLexiconJSON("app.test.two", "summary"),
	}))
	if err != nil {
		t.Fatalf("expected valid upload to succeed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected upload count 2, got %d", count)
	}
	assertLexiconCount(ctx, t, db, 2)
	assertLexiconExists(ctx, t, db, "app.test.one", true)
	assertLexiconExists(ctx, t, db, "app.test.two", true)
	if callbackCalls != 1 {
		t.Fatalf("expected lexicon change callback once, got %d", callbackCalls)
	}
}

func TestResolverRegisterLexiconRejectsMalformedResolvedSchemaBeforePersistence(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	resolver.resolveLexicon = fakeLexiconResolver(`{"lexicon":1,`, "did:plc:test")

	lex, err := resolver.RegisterLexicon(ctx, "app.test.bad")
	if err == nil {
		t.Fatal("expected malformed resolved schema to fail")
	}
	if lex != nil {
		t.Fatalf("expected nil lexicon result, got %+v", lex)
	}
	assertErrorContains(t, err, "app.test.bad", "did not store lexicon")
	assertLexiconCount(ctx, t, db, 0)
}

func TestResolverRegisterLexiconRejectsParserInvalidDocumentBeforePersistence(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	resolver.resolveLexicon = fakeLexiconResolver(`{"lexicon":1,"id":"app.test.invalid","defs":{"main":{"type":"query"}}}`, "did:plc:test")

	seedJSON := validAdminLexiconJSON("app.test.seed", "text")
	if err := db.Lexicons.Upsert(ctx, "app.test.seed", seedJSON); err != nil {
		t.Fatalf("failed to seed lexicon: %v", err)
	}

	lex, err := resolver.RegisterLexicon(ctx, "app.test.invalid")
	if err == nil {
		t.Fatal("expected parser-invalid resolved schema to fail")
	}
	if lex != nil {
		t.Fatalf("expected nil lexicon result, got %+v", lex)
	}
	assertErrorContains(t, err, "app.test.invalid", "unsupported main definition type", "did not store lexicon")
	assertLexiconCount(ctx, t, db, 1)
	assertLexiconExists(ctx, t, db, "app.test.seed", true)
	assertLexiconExists(ctx, t, db, "app.test.invalid", false)
}

func TestResolverRegisterLexiconRejectsIDMismatchBeforePersistence(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	resolver.resolveLexicon = fakeLexiconResolver(validAdminLexiconJSON("app.test.other", "body"), "did:plc:test")

	lex, err := resolver.RegisterLexicon(ctx, "app.test.expected")
	if err == nil {
		t.Fatal("expected resolved schema ID mismatch to fail")
	}
	if lex != nil {
		t.Fatalf("expected nil lexicon result, got %+v", lex)
	}
	assertErrorContains(t, err, "app.test.expected", "app.test.other", "did not store lexicon")
	assertLexiconCount(ctx, t, db, 0)
	assertLexiconExists(ctx, t, db, "app.test.expected", false)
	assertLexiconExists(ctx, t, db, "app.test.other", false)
}

func TestResolverRegisterLexiconTrimsRequestedNSIDBeforeResolving(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	resolver.resolveLexicon = func(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error) {
		if nsid != "app.test.trimmed" {
			t.Fatalf("expected resolver to receive trimmed NSID, got %q", nsid)
		}
		return &lexicon.ResolvedLexicon{
			NSID:   nsid,
			DID:    "did:plc:test",
			PDSUrl: "https://pds.example.com",
			Schema: json.RawMessage(validAdminLexiconJSON("app.test.trimmed", "body")),
		}, nil
	}

	lex, err := resolver.RegisterLexicon(ctx, " \tapp.test.trimmed\n")
	if err != nil {
		t.Fatalf("expected trimmed NSID to register: %v", err)
	}
	if lex["id"] != "app.test.trimmed" {
		t.Fatalf("expected registered id app.test.trimmed, got %v", lex["id"])
	}
	assertLexiconCount(ctx, t, db, 1)
	assertLexiconExists(ctx, t, db, "app.test.trimmed", true)
}

func TestResolverRegisterLexiconRejectsMalformedNSIDBeforeResolving(t *testing.T) {
	ctx := context.Background()

	for _, nsid := range []string{"app..test", ".bad.test", "bad.test.", "app.test"} {
		t.Run(nsid, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			resolver := newLexiconTestResolver(db)
			resolverCalled := false
			resolver.resolveLexicon = func(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error) {
				resolverCalled = true
				return nil, fmt.Errorf("resolver should not be called for invalid NSID")
			}

			lex, err := resolver.RegisterLexicon(ctx, nsid)
			if err == nil {
				t.Fatal("expected malformed NSID to fail before resolution")
			}
			if lex != nil {
				t.Fatalf("expected nil lexicon result, got %+v", lex)
			}
			if resolverCalled {
				t.Fatal("expected malformed NSID to fail before DNS/PDS resolution")
			}
			assertErrorContains(t, err, "dotted identifier", "no empty segments")
			assertLexiconCount(ctx, t, db, 0)
		})
	}
}

func TestResolverRegisterLexiconStoresValidResolvedSchema(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	resolver := newLexiconTestResolver(db)
	resolver.resolveLexicon = fakeLexiconResolver(validAdminLexiconJSON("app.test.valid", "body"), "did:plc:test")
	callbackCalls := 0
	resolver.SetLexiconChangeCallback(func(collections []string) error {
		callbackCalls++
		if len(collections) != 1 || collections[0] != "app.test.valid" {
			t.Fatalf("unexpected callback collections: %v", collections)
		}
		return nil
	})

	lex, err := resolver.RegisterLexicon(ctx, "app.test.valid")
	if err != nil {
		t.Fatalf("expected valid resolved schema to register: %v", err)
	}
	if lex["id"] != "app.test.valid" {
		t.Fatalf("expected registered id app.test.valid, got %v", lex["id"])
	}
	if lex["did"] != "did:plc:test" {
		t.Fatalf("expected DID did:plc:test, got %v", lex["did"])
	}
	assertLexiconCount(ctx, t, db, 1)
	assertLexiconExists(ctx, t, db, "app.test.valid", true)
	if callbackCalls != 1 {
		t.Fatalf("expected lexicon change callback once, got %d", callbackCalls)
	}
}

func newLexiconTestResolver(db *testutil.TestDB) *Resolver {
	return NewResolver(&Repositories{Lexicons: db.Lexicons}, "did:plc:test-labeler", nil)
}

func fakeLexiconResolver(schemaJSON string, did string) lexiconResolveFunc {
	return func(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error) {
		return &lexicon.ResolvedLexicon{
			NSID:   nsid,
			DID:    did,
			PDSUrl: "https://pds.example.com",
			Schema: json.RawMessage(schemaJSON),
		}, nil
	}
}

type zipEntry struct {
	name     string
	contents string
}

func zipBase64(t *testing.T, files map[string]string) string {
	t.Helper()

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]zipEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, zipEntry{name: name, contents: files[name]})
	}
	return zipBase64Entries(t, entries)
}

func zipBase64Entries(t *testing.T, entries []zipEntry) string {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, entry := range entries {
		writer, err := zw.Create(entry.name)
		if err != nil {
			t.Fatalf("failed to create zip entry %s: %v", entry.name, err)
		}
		if _, err := writer.Write([]byte(entry.contents)); err != nil {
			t.Fatalf("failed to write zip entry %s: %v", entry.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("failed to close zip: %v", err)
	}

	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func validAdminLexiconJSON(id, fieldName string) string {
	return fmt.Sprintf(`{
		"lexicon": 1,
		"id": %q,
		"defs": {
			"main": {
				"type": "record",
				"key": "tid",
				"record": {
					"type": "object",
					"required": [%q],
					"properties": {
						%q: {"type": "string"}
					}
				}
			}
		}
	}`, id, fieldName, fieldName)
}

func assertLexiconCount(ctx context.Context, t *testing.T, db *testutil.TestDB, want int64) {
	t.Helper()

	got, err := db.Lexicons.GetCount(ctx)
	if err != nil {
		t.Fatalf("failed to count lexicons: %v", err)
	}
	if got != want {
		t.Fatalf("lexicon count = %d, want %d", got, want)
	}
}

func assertLexiconExists(ctx context.Context, t *testing.T, db *testutil.TestDB, id string, want bool) {
	t.Helper()

	got, err := db.Lexicons.Exists(ctx, id)
	if err != nil {
		t.Fatalf("failed to check lexicon %s existence: %v", id, err)
	}
	if got != want {
		t.Fatalf("lexicon %s exists = %v, want %v", id, got, want)
	}
}

func assertErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	message := err.Error()
	for _, part := range parts {
		if !strings.Contains(message, part) {
			t.Fatalf("expected error %q to contain %q", message, part)
		}
	}
}
