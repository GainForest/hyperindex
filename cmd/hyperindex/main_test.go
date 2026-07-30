package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/GainForest/hyperindex/internal/buildinfo"
	"github.com/GainForest/hyperindex/internal/config"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	graphqladmin "github.com/GainForest/hyperindex/internal/graphql/admin"
	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/GainForest/hyperindex/internal/validation"
)

func TestRootEndpointReturnsBuildInfoVersion(t *testing.T) {
	previousVersion := buildinfo.Version
	buildinfo.Version = "v9.9.9-test"
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
	})

	r := setupRouter(&config.Config{ExternalBaseURL: "https://example.com"}, &services{}, &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET / response: %v", err)
	}
	if body["version"] != buildinfo.Version {
		t.Fatalf("version = %q, want buildinfo.Version %q", body["version"], buildinfo.Version)
	}
}

func TestHealthIgnoresLabelerReadiness(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.ExternalLabels.MarkFatalCursor(context.Background(), url, "FutureCursor", "Cursor is in the future. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(url), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	if _, ok := body["labelers"]; ok {
		t.Fatalf("unexpected labelers in liveness response: %#v", body["labelers"])
	}
	if _, ok := body["labelersError"]; ok {
		t.Fatalf("unexpected labelersError in liveness response: %#v", body["labelersError"])
	}
}

func TestHealthIgnoresLabelerDiagnosticsFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.Executor.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(url), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %v, want ok", body["status"])
	}
	if _, ok := body["labelersError"]; ok {
		t.Fatalf("unexpected labelersError in liveness response: %#v", body["labelersError"])
	}
	if _, ok := body["databaseError"]; ok {
		t.Fatalf("unexpected databaseError in liveness response: %#v", body["databaseError"])
	}
}

func TestReadyReturnsOKForRetryableLabelerError(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.ExternalLabels.UpdateError(context.Background(), url, "temporary websocket timeout"); err != nil {
		t.Fatalf("UpdateError() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(url), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ready status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
}

func TestReadyReturnsUnavailableWhenDatabaseUnavailable(t *testing.T) {
	db := testutil.SetupTestDB(t)
	if err := db.Executor.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	r := setupRouter(&config.Config{ExternalBaseURL: "https://example.com"}, labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /ready response: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", body["status"])
	}
	if databaseErr, ok := body["databaseError"].(string); !ok || databaseErr == "" {
		t.Fatalf("databaseError = %#v, want non-empty string", body["databaseError"])
	}
}

func TestReadyReturnsUnavailableWhenLabelerDiagnosticsFail(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	svc := labelerTestServices(db)
	svc.externalLabels = nil

	r := setupRouter(labelerTestConfig(url), svc, &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /ready response: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", body["status"])
	}
	if labelersErr, ok := body["labelersError"].(string); !ok || labelersErr == "" {
		t.Fatalf("labelersError = %#v, want non-empty string", body["labelersError"])
	}
}

func TestReadyReturnsUnavailableForFatalLabeler(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.ExternalLabels.UpdateLastSeq(context.Background(), url, 56428); err != nil {
		t.Fatalf("UpdateLastSeq() error = %v", err)
	}
	if err := db.ExternalLabels.MarkFatalCursor(context.Background(), url, "FutureCursor", "Cursor is in the future. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(url), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want %d; body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /ready response: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Fatalf("status = %v, want not_ready", body["status"])
	}
	labelers, ok := body["labelers"].([]any)
	if !ok || len(labelers) != 1 {
		t.Fatalf("labelers = %#v, want one diagnostic", body["labelers"])
	}
	diagnostic, ok := labelers[0].(map[string]any)
	if !ok {
		t.Fatalf("labeler diagnostic = %#v, want object", labelers[0])
	}
	if diagnostic["status"] != repositories.LabelSubscriptionStatusFatal {
		t.Fatalf("labeler status = %v, want fatal", diagnostic["status"])
	}
	if diagnostic["lastErrorCode"] != "FutureCursor" {
		t.Fatalf("lastErrorCode = %v, want FutureCursor", diagnostic["lastErrorCode"])
	}
	if lastError, ok := diagnostic["lastError"].(string); !ok || !strings.HasPrefix(lastError, "FATAL_CURSOR FutureCursor:") {
		t.Fatalf("lastError = %v, want fatal cursor marker", diagnostic["lastError"])
	}
}

func TestStatsIncludesLabelerDiagnostics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	url := "wss://labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.ExternalLabels.MarkFatalCursor(context.Background(), url, "OutdatedCursor", "Cursor is outside retained history. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(url), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /stats status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /stats response: %v", err)
	}
	labelers, ok := body["labelers"].([]any)
	if !ok || len(labelers) != 1 {
		t.Fatalf("labelers = %#v, want one diagnostic", body["labelers"])
	}
	diagnostic := labelers[0].(map[string]any)
	if diagnostic["url"] != url {
		t.Fatalf("url = %v, want %s", diagnostic["url"], url)
	}
	if diagnostic["status"] != repositories.LabelSubscriptionStatusFatal {
		t.Fatalf("status = %v, want fatal", diagnostic["status"])
	}
	if diagnostic["lastErrorCode"] != "OutdatedCursor" || diagnostic["lastError"] == nil {
		t.Fatalf("diagnostic missing expected fatal marker fields: %#v", diagnostic)
	}
}

func TestStatsUsesPersistedLabelerURLOverride(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	envURL := "wss://env-labeler.example/xrpc/com.atproto.label.subscribeLabels"
	persistedURL := "wss://persisted-labeler.example/xrpc/com.atproto.label.subscribeLabels"
	if err := db.Config.Set(ctx, repositories.ConfigKeyLabelerSubscribeURLs, persistedURL); err != nil {
		t.Fatalf("Set(labeler_subscribe_urls) error = %v", err)
	}
	if err := db.ExternalLabels.MarkFatalCursor(ctx, persistedURL, "OutdatedCursor", "Cursor is outside retained history. Reset subscription cursor and replay labels."); err != nil {
		t.Fatalf("MarkFatalCursor() error = %v", err)
	}

	r := setupRouter(labelerTestConfig(envURL), labelerTestServices(db), &backgroundServices{})
	req := httptest.NewRequest(http.MethodGet, "/stats", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /stats status = %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode GET /stats response: %v", err)
	}
	labelers, ok := body["labelers"].([]any)
	if !ok || len(labelers) != 1 {
		t.Fatalf("labelers = %#v, want one diagnostic", body["labelers"])
	}
	diagnostic := labelers[0].(map[string]any)
	if diagnostic["url"] != persistedURL {
		t.Fatalf("url = %v, want persisted override %s", diagnostic["url"], persistedURL)
	}
}

func TestSetupGraphQLUsesBundledLexiconsWithoutDirectoryOrDatabaseRows(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := labelerTestServices(db)
	router := chi.NewRouter()

	collections, err := setupGraphQL(router, &config.Config{
		ExternalBaseURL: "https://example.com",
	}, svc, nil, nil)
	if err != nil {
		t.Fatalf("setupGraphQL() error = %v", err)
	}
	if !slices.Contains(collections, "org.hypercerts.claim.activity") {
		t.Fatalf("collections = %v, want bundled Hypercerts collection", collections)
	}
	if _, ok := svc.validator.LexiconHash("org.hypercerts.workscope.cel"); !ok {
		t.Fatal("startup validator is missing bundled org.hypercerts.workscope.cel")
	}

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ appCertifiedSignatureProof(first: 1) { totalCount } orgHyperboardsBoard(first: 1) { totalCount } orgHyperboardsDisplayProfile(first: 1) { totalCount } }"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var body struct {
		Errors []map[string]interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode bundled GraphQL query response: %v", err)
	}
	if len(body.Errors) > 0 {
		t.Fatalf("bundled typed GraphQL fields failed: errors=%v body=%s", body.Errors, response.Body.String())
	}
}

func TestSetupGraphQLHashesSavedLexiconsAndClassifiesBeforeServing(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	lexiconDir := t.TempDir()
	writeTestLexiconFile(t, lexiconDir, "com.example.record.json", setupGraphQLTestLexicon)

	uri := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","name":"ok"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	svc := labelerTestServices(db)
	collections, err := setupGraphQL(chi.NewRouter(), &config.Config{
		ExternalBaseURL: "https://example.com",
		LexiconDir:      lexiconDir,
	}, svc, nil, nil)
	if err != nil {
		t.Fatalf("setupGraphQL() error = %v", err)
	}
	if !slices.Contains(collections, "com.example.record") {
		t.Fatalf("collections = %v, want custom startup record collection", collections)
	}
	if !slices.Contains(collections, "org.hypercerts.claim.activity") {
		t.Fatalf("collections = %v, want bundled Hypercerts collection", collections)
	}
	wantHash := validation.HashLexiconJSON([]byte("com.example.record=" + validation.HashLexiconJSON([]byte(setupGraphQLTestLexicon))))
	if gotHash, ok := svc.validator.LexiconHash("com.example.record"); !ok || gotHash != wantHash {
		t.Fatalf("validator hash = %q, %v; want %q, true", gotHash, ok, wantHash)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusValid {
		t.Fatalf("ValidationStatus = %q, want %q", rec.ValidationStatus, validation.StatusValid)
	}
	if rec.LexiconHash != wantHash {
		t.Fatalf("LexiconHash = %q, want %q", rec.LexiconHash, wantHash)
	}
	if rec.ValidatedAt == nil {
		t.Fatal("ValidatedAt is nil, want startup classification timestamp")
	}
}

func TestSetupGraphQLSupportsRecordKeyLexicon(t *testing.T) {
	db := testutil.SetupTestDB(t)
	lexiconDir := t.TempDir()
	recordKeyLexicon := strings.Replace(setupGraphQLTestLexicon, `"key": "tid"`, `"key": "record-key"`, 1)
	writeTestLexiconFile(t, lexiconDir, "com.example.record.json", recordKeyLexicon)

	svc := labelerTestServices(db)
	collections, err := setupGraphQL(chi.NewRouter(), &config.Config{
		ExternalBaseURL: "https://example.com",
		LexiconDir:      lexiconDir,
	}, svc, nil, nil)
	if err != nil {
		t.Fatalf("setupGraphQL(record-key) error = %v", err)
	}
	if !slices.Contains(collections, "com.example.record") {
		t.Fatalf("collections = %v, want record-key collection", collections)
	}
	result := svc.validator.ValidateRecord("com.example.record", "self", []byte(`{"$type":"com.example.record","name":"ok"}`))
	if result.Status != validation.StatusValid {
		t.Fatalf("record-key validation status = %q, want valid (error %q)", result.Status, result.Error)
	}
}

func TestSetupGraphQLUsesDatabaseOverrideAndExplicitJetstreamCollections(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	lexiconDir := t.TempDir()
	filesystemLexicon := `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","required":["filesystemField"],"properties":{"filesystemField":{"type":"string"}}}}}}`
	databaseLexicon := `{"lexicon":1,"id":"com.example.record","defs":{"main":{"type":"record","key":"tid","record":{"type":"object","required":["databaseField"],"properties":{"databaseField":{"type":"string"}}}}}}`
	writeTestLexiconFile(t, lexiconDir, "com.example.record.json", filesystemLexicon)
	if err := db.Lexicons.Upsert(ctx, "com.example.record", databaseLexicon); err != nil {
		t.Fatalf("Upsert database Lexicon error = %v", err)
	}

	uri := "at://did:plc:test/com.example.record/3jui7kd54zh2y"
	if _, err := db.Records.Insert(ctx, uri, "cid", "did:plc:test", "com.example.record", `{"$type":"com.example.record","databaseField":"database wins"}`); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	svc := labelerTestServices(db)
	collections, err := setupGraphQL(chi.NewRouter(), &config.Config{
		ExternalBaseURL:      "https://example.com",
		LexiconDir:           lexiconDir,
		JetstreamCollections: "com.example.explicit,com.example.other",
	}, svc, nil, nil)
	if err != nil {
		t.Fatalf("setupGraphQL() error = %v", err)
	}
	if len(collections) != 2 || collections[0] != "com.example.explicit" || collections[1] != "com.example.other" {
		t.Fatalf("collections = %v, want explicit fixed override", collections)
	}

	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("GetByURI() error = %v", err)
	}
	if rec.ValidationStatus != validation.StatusValid {
		t.Fatalf("ValidationStatus = %q, want %q from database override (error %q)", rec.ValidationStatus, validation.StatusValid, rec.ValidationError)
	}
}

func TestSetupGraphQLAppliesStagedLexiconDeletionAfterRestart(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	if err := db.Lexicons.Upsert(ctx, "com.example.record", setupGraphQLTestLexicon); err != nil {
		t.Fatalf("Upsert database Lexicon error = %v", err)
	}
	adminResolver := graphqladmin.NewResolver(&graphqladmin.Repositories{Lexicons: db.Lexicons}, "did:web:example.com", nil)
	deleted, err := adminResolver.DeleteLexicon(ctx, "com.example.record")
	if err != nil || !deleted {
		t.Fatalf("DeleteLexicon() deleted=%v error=%v, want true nil", deleted, err)
	}

	router := chi.NewRouter()
	svc := labelerTestServices(db)
	collections, err := setupGraphQL(router, &config.Config{
		ExternalBaseURL: "https://example.com",
		LexiconDir:      t.TempDir(),
	}, svc, nil, nil)
	if err != nil {
		t.Fatalf("setupGraphQL(after deletion) error = %v", err)
	}
	if slices.Contains(collections, "com.example.record") {
		t.Fatalf("startup collections after deletion = %v, want com.example.record absent", collections)
	}
	if !slices.Contains(collections, "org.hypercerts.claim.activity") {
		t.Fatalf("startup collections after deletion = %v, want bundled collections retained", collections)
	}
	if _, ok := svc.validator.GraphQLRegistry().GetRecordDef("com.example.record"); ok {
		t.Fatal("deleted Lexicon remained in restarted GraphQL registry")
	}

	req := httptest.NewRequest(http.MethodPost, "/graphql", strings.NewReader(`{"query":"{ comExampleRecord(first: 1) { totalCount } }"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var body struct {
		Errors []map[string]interface{} `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode GraphQL response: %v", err)
	}
	if len(body.Errors) == 0 {
		t.Fatalf("deleted typed GraphQL field remained queryable; response=%s", response.Body.String())
	}
}

func TestSetupGraphQLRejectsCorruptSavedLexiconFiles(t *testing.T) {
	db := testutil.SetupTestDB(t)
	lexiconDir := t.TempDir()
	writeTestLexiconFile(t, lexiconDir, "broken.json", `{"id":"com.example.broken","defs":`)

	_, err := setupGraphQL(chi.NewRouter(), &config.Config{
		ExternalBaseURL: "https://example.com",
		LexiconDir:      lexiconDir,
	}, labelerTestServices(db), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "invalid JSON file") {
		t.Fatalf("setupGraphQL() error = %v, want corrupt file path error", err)
	}
}

const setupGraphQLTestLexicon = `{
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

func writeTestLexiconFile(t *testing.T, dir string, name string, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
}

func labelerTestConfig(url string) *config.Config {
	return &config.Config{
		ExternalBaseURL:         "https://example.com",
		LabelerSubscribeEnabled: true,
		LabelerSubscribeURLs:    url,
	}
}

func labelerTestServices(db *testutil.TestDB) *services {
	return &services{
		db:             db.Executor,
		records:        db.Records,
		actors:         db.Actors,
		lexicons:       db.Lexicons,
		config:         db.Config,
		externalLabels: db.ExternalLabels,
	}
}

func TestLoadLexiconsFromDirSkipsNonLexiconJSON(t *testing.T) {
	dir := t.TempDir()
	lexiconJSON := `{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "post.json"), []byte(lexiconJSON), 0o644); err != nil {
		t.Fatalf("write lexicon: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(`{"not":"a lexicon"}`), 0o644); err != nil {
		t.Fatalf("write metadata: %v", err)
	}

	saved, err := loadLexiconsFromDir(dir)
	if err != nil {
		t.Fatalf("loadLexiconsFromDir() error = %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("saved Lexicon count = %d, want 1", len(saved))
	}
	if got, ok := saved["app.example.post"]; !ok || string(got) != lexiconJSON {
		t.Fatalf("saved Lexicons missing exact app.example.post bytes: %#v", saved)
	}
}

func TestLoadLexiconsFromDirRejectsDuplicateIDs(t *testing.T) {
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.json")
	secondPath := filepath.Join(dir, "second.json")
	const lexiconJSON = `{"lexicon":1,"id":"app.example.post","defs":{"main":{"type":"record","key":"any","record":{"type":"object","properties":{}}}}}`
	writeTestLexiconFile(t, dir, "first.json", lexiconJSON)
	writeTestLexiconFile(t, dir, "second.json", lexiconJSON)

	_, err := loadLexiconsFromDir(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate Lexicon id app.example.post") ||
		!strings.Contains(err.Error(), firstPath) || !strings.Contains(err.Error(), secondPath) {
		t.Fatalf("loadLexiconsFromDir() error = %v, want duplicate ID and both paths", err)
	}
}

func TestApplyTapSidecarHealth(t *testing.T) {
	tests := []struct {
		name      string
		timeout   time.Duration
		healthFn  func(context.Context) error
		wantState string
	}{
		{
			name:    "sidecar healthy",
			timeout: 50 * time.Millisecond,
			healthFn: func(context.Context) error {
				return nil
			},
			wantState: "ok",
		},
		{
			name:    "sidecar returns error",
			timeout: 50 * time.Millisecond,
			healthFn: func(context.Context) error {
				return errors.New("sidecar unavailable")
			},
			wantState: "unreachable",
		},
		{
			name:    "sidecar health times out",
			timeout: 10 * time.Millisecond,
			healthFn: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			wantState: "unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tapInfo := map[string]any{}
			applyTapSidecarHealth(context.Background(), tapInfo, tt.timeout, tt.healthFn)

			gotState, ok := tapInfo["sidecar"].(string)
			if !ok {
				t.Fatalf("sidecar state missing or non-string: %#v", tapInfo["sidecar"])
			}
			if gotState != tt.wantState {
				t.Fatalf("sidecar state = %q, want %q", gotState, tt.wantState)
			}

			if _, hasErr := tapInfo["sidecar_error"]; hasErr {
				t.Fatalf("unexpected sidecar_error in stats payload")
			}
		})
	}
}
