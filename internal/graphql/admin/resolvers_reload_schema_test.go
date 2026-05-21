package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	publicgraphql "github.com/GainForest/hyperindex/internal/graphql"
)

const reloadSchemaMutation = `mutation ReloadSchema {
	reloadSchema {
		success
		lexiconCount
		reloadedAt
		error
	}
}`

type reloadSchemaResponse struct {
	Data struct {
		ReloadSchema *struct {
			Success      bool    `json:"success"`
			LexiconCount int     `json:"lexiconCount"`
			ReloadedAt   *string `json:"reloadedAt"`
			Error        *string `json:"error"`
		} `json:"reloadSchema"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func TestResolverReloadSchemaSuccess(t *testing.T) {
	resolver := NewResolver(&Repositories{}, "did:web:example.com", nil)
	reloadedAt := time.Date(2026, 5, 21, 12, 34, 56, 0, time.UTC)
	called := false
	resolver.SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		called = true
		return &publicgraphql.ReloadSchemaResult{
			Success:      true,
			LexiconCount: 42,
			ReloadedAt:   &reloadedAt,
		}, nil
	})

	result, err := resolver.ReloadSchema(context.Background())
	if err != nil {
		t.Fatalf("ReloadSchema returned error: %v", err)
	}
	if !called {
		t.Fatal("expected schema reload callback to be called")
	}
	if result["success"] != true {
		t.Fatalf("success = %v, want true", result["success"])
	}
	if result["lexiconCount"] != 42 {
		t.Fatalf("lexiconCount = %v, want 42", result["lexiconCount"])
	}
	if result["reloadedAt"] != reloadedAt.Format(time.RFC3339) {
		t.Fatalf("reloadedAt = %v, want %q", result["reloadedAt"], reloadedAt.Format(time.RFC3339))
	}
	if result["error"] != nil {
		t.Fatalf("error = %v, want nil", result["error"])
	}
}

func TestResolverReloadSchemaFailurePayloadDoesNotError(t *testing.T) {
	resolver := NewResolver(&Repositories{}, "did:web:example.com", nil)
	resolver.SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		return &publicgraphql.ReloadSchemaResult{
			Success:      false,
			LexiconCount: 7,
			Error:        "parse database lexicon \"app.example.bad\": invalid JSON",
		}, nil
	})

	result, err := resolver.ReloadSchema(context.Background())
	if err != nil {
		t.Fatalf("ReloadSchema returned unexpected resolver error: %v", err)
	}
	if result["success"] != false {
		t.Fatalf("success = %v, want false", result["success"])
	}
	if result["lexiconCount"] != 7 {
		t.Fatalf("lexiconCount = %v, want 7", result["lexiconCount"])
	}
	if result["reloadedAt"] != nil {
		t.Fatalf("reloadedAt = %v, want nil", result["reloadedAt"])
	}
	if result["error"] == nil || !strings.Contains(result["error"].(string), "app.example.bad") {
		t.Fatalf("error = %v, want message mentioning app.example.bad", result["error"])
	}
}

func TestResolverReloadSchemaMissingCallbackReturnsHelpfulError(t *testing.T) {
	resolver := NewResolver(&Repositories{}, "did:web:example.com", nil)

	result, err := resolver.ReloadSchema(context.Background())
	if err == nil {
		t.Fatal("expected missing callback to return an error")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
	assertReloadSchemaErrorContains(t, err, "schema reload callback not configured", "SetSchemaReloadCallback")
}

func TestResolverReloadSchemaCallbackErrorReturnsResolverError(t *testing.T) {
	resolver := NewResolver(&Repositories{}, "did:web:example.com", nil)
	resolver.SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		return nil, errors.New("database unavailable")
	})

	result, err := resolver.ReloadSchema(context.Background())
	if err == nil {
		t.Fatal("expected callback error to return resolver error")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
	assertReloadSchemaErrorContains(t, err, "reload public GraphQL schema", "database unavailable")
}

func TestResolverReloadSchemaNilResultReturnsResolverError(t *testing.T) {
	resolver := NewResolver(&Repositories{}, "did:web:example.com", nil)
	resolver.SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		return nil, nil
	})

	result, err := resolver.ReloadSchema(context.Background())
	if err == nil {
		t.Fatal("expected nil callback result to return resolver error")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil", result)
	}
	assertReloadSchemaErrorContains(t, err, "callback returned nil result")
}

func TestSchemaIncludesReloadSchemaMutation(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")

	mutationType := handler.Schema().MutationType()
	if mutationType == nil {
		t.Fatal("expected mutation type")
	}

	field := mutationType.Fields()["reloadSchema"]
	if field == nil {
		t.Fatal("expected reloadSchema mutation")
	}
	if field.Type.String() != "ReloadSchemaResult!" {
		t.Fatalf("reloadSchema type = %s, want ReloadSchemaResult!", field.Type.String())
	}
}

func TestReloadSchemaResultTypeFields(t *testing.T) {
	fields := ReloadSchemaResultType.Fields()
	wantTypes := map[string]string{
		"success":      "Boolean!",
		"lexiconCount": "Int!",
		"reloadedAt":   "String",
		"error":        "String",
	}
	for name, want := range wantTypes {
		field := fields[name]
		if field == nil {
			t.Fatalf("expected field %s on ReloadSchemaResult", name)
		}
		if got := field.Type.String(); got != want {
			t.Fatalf("ReloadSchemaResult.%s type = %s, want %s", name, got, want)
		}
	}
}

func TestHandlerReloadSchemaMutationSuccess(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")
	reloadedAt := time.Date(2026, 5, 21, 12, 34, 56, 0, time.UTC)
	called := 0
	handler.Resolver().SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		called++
		return &publicgraphql.ReloadSchemaResult{
			Success:      true,
			LexiconCount: 42,
			ReloadedAt:   &reloadedAt,
		}, nil
	})

	rr := executeReloadSchemaMutation(t, handler, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	payload := decodeReloadSchemaResponse(t, rr)
	if len(payload.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors: %+v", payload.Errors)
	}
	if called != 1 {
		t.Fatalf("callback calls = %d, want 1", called)
	}
	if payload.Data.ReloadSchema == nil {
		t.Fatal("expected reloadSchema payload")
	}
	if !payload.Data.ReloadSchema.Success {
		t.Fatal("expected success=true")
	}
	if payload.Data.ReloadSchema.LexiconCount != 42 {
		t.Fatalf("lexiconCount = %d, want 42", payload.Data.ReloadSchema.LexiconCount)
	}
	if payload.Data.ReloadSchema.ReloadedAt == nil || *payload.Data.ReloadSchema.ReloadedAt != reloadedAt.Format(time.RFC3339) {
		t.Fatalf("reloadedAt = %v, want %q", payload.Data.ReloadSchema.ReloadedAt, reloadedAt.Format(time.RFC3339))
	}
	if payload.Data.ReloadSchema.Error != nil {
		t.Fatalf("error = %v, want nil", *payload.Data.ReloadSchema.Error)
	}
}

func TestHandlerReloadSchemaMutationFailurePayloadDoesNotReturnGraphQLError(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")
	handler.Resolver().SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		return &publicgraphql.ReloadSchemaResult{
			Success:      false,
			LexiconCount: 7,
			Error:        "parse database lexicon \"app.example.bad\": invalid JSON",
		}, nil
	})

	rr := executeReloadSchemaMutation(t, handler, true)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	payload := decodeReloadSchemaResponse(t, rr)
	if len(payload.Errors) != 0 {
		t.Fatalf("unexpected GraphQL errors for failure payload: %+v", payload.Errors)
	}
	if payload.Data.ReloadSchema == nil {
		t.Fatal("expected reloadSchema payload")
	}
	if payload.Data.ReloadSchema.Success {
		t.Fatal("expected success=false")
	}
	if payload.Data.ReloadSchema.LexiconCount != 7 {
		t.Fatalf("lexiconCount = %d, want 7", payload.Data.ReloadSchema.LexiconCount)
	}
	if payload.Data.ReloadSchema.Error == nil || !strings.Contains(*payload.Data.ReloadSchema.Error, "app.example.bad") {
		t.Fatalf("error = %v, want message mentioning app.example.bad", payload.Data.ReloadSchema.Error)
	}
}

func TestHandlerReloadSchemaMutationMissingCallbackReturnsGraphQLError(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")

	rr := executeReloadSchemaMutation(t, handler, true)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	payload := decodeReloadSchemaResponse(t, rr)
	if len(payload.Errors) == 0 {
		t.Fatal("expected GraphQL error for missing callback")
	}
	assertReloadSchemaErrorContains(t, errors.New(payload.Errors[0].Message), "schema reload callback not configured")
	if payload.Data.ReloadSchema != nil {
		t.Fatalf("expected no reloadSchema payload, got %+v", payload.Data.ReloadSchema)
	}
}

func TestHandlerReloadSchemaMutationRequiresAdmin(t *testing.T) {
	handler := newTestAdminHandler(t, "did:plc:admin1", "super-secret-key")
	called := false
	handler.Resolver().SetSchemaReloadCallback(func(ctx context.Context) (*publicgraphql.ReloadSchemaResult, error) {
		called = true
		return &publicgraphql.ReloadSchemaResult{Success: true, LexiconCount: 1}, nil
	})

	rr := executeReloadSchemaMutation(t, handler, false)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body: %s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}
	if called {
		t.Fatal("expected unauthorized request not to call reload callback")
	}

	payload := decodeReloadSchemaResponse(t, rr)
	if len(payload.Errors) == 0 {
		t.Fatal("expected GraphQL error for unauthorized reload")
	}
	assertReloadSchemaErrorContains(t, errors.New(payload.Errors[0].Message), "admin privileges required")
}

func executeReloadSchemaMutation(t *testing.T, handler *Handler, authorized bool) *httptest.ResponseRecorder {
	t.Helper()

	bodyBytes, err := json.Marshal(map[string]interface{}{"query": reloadSchemaMutation})
	if err != nil {
		t.Fatalf("failed to encode request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/admin/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	if authorized {
		req.Header.Set("X-Admin-API-Key", "super-secret-key")
		req.Header.Set("X-User-DID", "did:plc:admin1")
	}

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func decodeReloadSchemaResponse(t *testing.T, rr *httptest.ResponseRecorder) *reloadSchemaResponse {
	t.Helper()

	var payload reloadSchemaResponse
	if err := json.NewDecoder(rr.Body).Decode(&payload); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	return &payload
}

func assertReloadSchemaErrorContains(t *testing.T, err error, parts ...string) {
	t.Helper()

	message := err.Error()
	for _, part := range parts {
		if !strings.Contains(message, part) {
			t.Fatalf("expected error %q to contain %q", message, part)
		}
	}
}
