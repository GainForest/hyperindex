package graphql

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	graphqlgo "github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
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

func TestHandler_ServeHTTP_NoCORSInHandler(t *testing.T) {
	// CORS is handled by route-level CORSMiddleware, not the handler.
	// Verify the handler does NOT set CORS headers directly.
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := &Handler{schema: schema, repos: nil}

	t.Run("handler does not set CORS headers", func(t *testing.T) {
		body := map[string]interface{}{"query": "{ ping }"}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Error("handler should not set Access-Control-Allow-Origin (CORS is route middleware's job)")
		}
	})
}

func TestHandler_ServeHTTP_POST(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := &Handler{schema: schema, repos: nil}

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

	handler := &Handler{schema: schema, repos: nil}

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

	handler := &Handler{schema: schema, repos: nil}

	if handler.Schema() != schema {
		t.Error("Schema() did not return the expected schema")
	}
}

func TestHandler_ServeHTTP_ContentType(t *testing.T) {
	schema, err := createMinimalSchema()
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	handler := &Handler{schema: schema, repos: nil}

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

	handler := &Handler{schema: schema, repos: nil}

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
	handler := &Handler{schema: &schema, repos: repos}

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
