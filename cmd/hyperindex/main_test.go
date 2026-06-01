package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/buildinfo"
	"github.com/GainForest/hyperindex/internal/config"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
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
		externalLabels: db.ExternalLabels,
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
