package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/GainForest/hyperindex/internal/oauth"
)

func TestHandleJWKS(t *testing.T) {
	t.Run("returns empty JWKS when no signing key configured", func(t *testing.T) {
		h := NewOAuthHandlers(OAuthHandlerConfig{}, nil)

		req := httptest.NewRequest(http.MethodGet, "/oauth/jwks", nil)
		rec := httptest.NewRecorder()
		h.HandleJWKS(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var got map[string][]map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		keys, ok := got["keys"]
		if !ok {
			t.Fatal("response missing keys field")
		}
		if len(keys) != 0 {
			t.Fatalf("expected 0 keys, got %d", len(keys))
		}
	})

	t.Run("returns signing key in JWKS when configured", func(t *testing.T) {
		signingKey, err := oauth.GenerateDPoPKeyPair()
		if err != nil {
			t.Fatalf("failed to generate signing key: %v", err)
		}

		h := NewOAuthHandlers(OAuthHandlerConfig{SigningKey: signingKey}, nil)

		req := httptest.NewRequest(http.MethodGet, "/oauth/jwks", nil)
		rec := httptest.NewRecorder()
		h.HandleJWKS(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var got map[string][]map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		keys := got["keys"]
		if len(keys) != 1 {
			t.Fatalf("expected 1 key, got %d", len(keys))
		}

		key := keys[0]
		for _, field := range []string{"kty", "crv", "x", "y", "kid", "use"} {
			if _, ok := key[field]; !ok {
				t.Fatalf("expected JWKS key to include field %q", field)
			}
		}
	})

	t.Run("returns server_error with request_id when signing key conversion fails", func(t *testing.T) {
		h := NewOAuthHandlers(OAuthHandlerConfig{SigningKey: &oauth.DPoPKeyPair{}}, nil)

		r := chi.NewRouter()
		r.Use(middleware.RequestID)
		r.Get("/oauth/jwks", h.HandleJWKS)

		req := httptest.NewRequest(http.MethodGet, "/oauth/jwks", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}

		var got map[string]string
		if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if got["error"] != "server_error" {
			t.Fatalf("error = %q, want %q", got["error"], "server_error")
		}
		if got["error_description"] != "Internal server error" {
			t.Fatalf("error_description = %q, want %q", got["error_description"], "Internal server error")
		}
		if got["request_id"] == "" {
			t.Fatal("request_id should be present")
		}
	})
}
