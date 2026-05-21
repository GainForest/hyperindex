package graphql

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	graphqlgo "github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/testutil"
)

// createMinimalSchema creates a minimal GraphQL schema for testing
func createMinimalSchema() (*graphqlgo.Schema, error) {
	queryType := graphqlgo.NewObject(graphqlgo.ObjectConfig{
		Name: "Query",
		Fields: graphqlgo.Fields{
			"ping": &graphqlgo.Field{
				Type: graphqlgo.String,
				Resolve: func(p graphqlgo.ResolveParams) (interface{}, error) {
					return "pong", nil
				},
			},
		},
	})

	schema, err := graphqlgo.NewSchema(graphqlgo.SchemaConfig{
		Query: queryType,
	})
	if err != nil {
		return nil, err
	}
	return &schema, nil
}

func newStaticTestHandler(t *testing.T, schema *graphqlgo.Schema, repos *resolver.Repositories) *Handler {
	t.Helper()

	handler, err := NewHandlerWithSchemaProvider(staticSchemaProvider{schema: schema}, repos)
	if err != nil {
		t.Fatalf("failed to create test handler: %v", err)
	}
	return handler
}

func TestHandler_ServeHTTP_NoCORSInHandler(t *testing.T) {
	// CORS is handled by the router-level CORSMiddleware, not the handler.
	// Verify the handler does NOT set CORS headers directly.
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	t.Run("handler does not set CORS headers", func(t *testing.T) {
		body := map[string]interface{}{"query": "{ ping }"}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("handler should not set Access-Control-Allow-Origin (CORS is middleware's job)")
		}
	})
}

func TestHandler_ServeHTTP_POST(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	t.Run("valid POST request", func(t *testing.T) {
		body := map[string]interface{}{
			"query": "{ ping }",
		}
		bodyBytes, _ := json.Marshal(body)

		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		data, ok := result["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected data object in response")
		}

		if data["ping"] != "pong" {
			t.Errorf("expected ping to be 'pong', got %v", data["ping"])
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader([]byte("not json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})
}

func TestHandler_ServeHTTP_GET(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	t.Run("GET request with query parameter", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/graphql?query={ping}", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		data, ok := result["data"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected data object in response")
		}

		if data["ping"] != "pong" {
			t.Errorf("expected ping to be 'pong', got %v", data["ping"])
		}
	})
}

func TestHandler_Schema(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	if handler.Schema() != schema {
		t.Error("Schema() did not return the expected schema")
	}
}

func TestHandler_ServeHTTP_ContentType(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	body := map[string]interface{}{
		"query": "{ ping }",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got %q", contentType)
	}
}

func TestHandler_ServeHTTP_GraphQLError(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := newStaticTestHandler(t, schema, nil)

	// Query for a field that doesn't exist
	body := map[string]interface{}{
		"query": "{ nonexistent }",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// GraphQL errors should return 400
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result["errors"] == nil {
		t.Error("expected errors in response")
	}
}

func TestHandler_ServeHTTP_WithRepositories(t *testing.T) {
	// Create a schema that accesses repositories from context
	queryType := graphqlgo.NewObject(graphqlgo.ObjectConfig{
		Name: "Query",
		Fields: graphqlgo.Fields{
			"hasRepos": &graphqlgo.Field{
				Type: graphqlgo.Boolean,
				Resolve: func(p graphqlgo.ResolveParams) (interface{}, error) {
					repos := resolver.GetRepositories(p.Context)
					return repos != nil, nil
				},
			},
		},
	})

	schema, err := graphqlgo.NewSchema(graphqlgo.SchemaConfig{
		Query: queryType,
	})
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Create handler with non-nil repos (even though they're empty)
	repos := &resolver.Repositories{}
	handler := newStaticTestHandler(t, &schema, repos)

	body := map[string]interface{}{
		"query": "{ hasRepos }",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object in response, got %v", result)
	}

	if data["hasRepos"] != true {
		t.Errorf("expected hasRepos to be true, got %v", data["hasRepos"])
	}
}

func TestHandler_ServeHTTP_NoActiveSchemaReturnsServiceUnavailable(t *testing.T) {
	handler := newStaticTestHandler(t, nil, nil)

	body := map[string]interface{}{"query": "{ ping }"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", contentType)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	errorsValue, ok := result["errors"].([]interface{})
	if !ok || len(errorsValue) != 1 {
		t.Fatalf("expected one GraphQL error, got %#v", result["errors"])
	}
	firstError, ok := errorsValue[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected GraphQL error object, got %#v", errorsValue[0])
	}
	message, _ := firstError["message"].(string)
	if !strings.Contains(message, "public GraphQL schema is unavailable") || !strings.Contains(message, "reloadSchema") {
		t.Fatalf("expected actionable no-schema message, got %q", message)
	}
	extensions, ok := firstError["extensions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected schema-unavailable error extensions, got %#v", firstError["extensions"])
	}
	if extensions["code"] != "SCHEMA_UNAVAILABLE" || extensions["httpStatus"] != float64(http.StatusServiceUnavailable) {
		t.Fatalf("unexpected schema-unavailable extensions: %#v", extensions)
	}
}

func TestHandler_ServeHTTP_UsesLatestSchemaAfterProviderReload(t *testing.T) {
	ctx := context.Background()
	db := testutil.SetupTestDB(t)
	repos := &resolver.Repositories{Lexicons: db.Lexicons}
	manager := NewPublicSchemaManager(PublicSchemaManagerConfig{LexiconDir: t.TempDir()}, repos)

	upsertLexicon(ctx, t, db, "app.test.alpha", "text")
	result, err := manager.Reload(ctx)
	if err != nil {
		t.Fatalf("initial reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("initial reload failed: %s", result.Error)
	}

	handler, err := NewHandlerWithSchemaProvider(manager, repos)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	assertHTTPQueryField(t, handler, lexicon.ToFieldName("app.test.alpha"), true)
	assertHTTPQueryField(t, handler, lexicon.ToFieldName("app.test.beta"), false)

	upsertLexicon(ctx, t, db, "app.test.beta", "summary")
	result, err = manager.Reload(ctx)
	if err != nil {
		t.Fatalf("second reload returned error: %v", err)
	}
	if !result.Success {
		t.Fatalf("second reload failed: %s", result.Error)
	}

	assertHTTPQueryField(t, handler, lexicon.ToFieldName("app.test.beta"), true)
}

func assertHTTPQueryField(t *testing.T, handler *Handler, fieldName string, want bool) {
	t.Helper()

	body := map[string]interface{}{"query": `{ __type(name: "Query") { fields { name } } }`}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("introspection status = %d, want %d; body: %s", w.Code, http.StatusOK, w.Body.String())
	}

	var response struct {
		Data struct {
			Type struct {
				Fields []struct {
					Name string `json:"name"`
				} `json:"fields"`
			} `json:"__type"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to decode introspection response: %v", err)
	}

	for _, field := range response.Data.Type.Fields {
		if field.Name == fieldName {
			if !want {
				t.Fatalf("query field %q exists, want absent", fieldName)
			}
			return
		}
	}
	if want {
		t.Fatalf("query field %q is absent, want present", fieldName)
	}
}
