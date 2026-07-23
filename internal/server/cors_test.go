package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCORSMiddlewareAllowsAllOriginsWithWildcard(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{AllowedOrigins: []string{"*"}})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/graphql", nil)
	req.Header.Set("Origin", "https://example.app")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestCORSMiddlewareRestrictsSpecificOrigins(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{AllowedOrigins: []string{"https://admin.example"}})(okHandler())

	t.Run("allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/graphql", nil)
		req.Header.Set("Origin", "https://admin.example")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://admin.example" {
			t.Fatalf("Access-Control-Allow-Origin = %q, want allowed origin", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q, want Origin", got)
		}
	})

	t.Run("rejected origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/admin/graphql", nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
			t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
		}
		if got := rec.Header().Get("Vary"); got != "Origin" {
			t.Fatalf("Vary = %q, want Origin", got)
		}
	})
}

func TestCORSMiddlewareEmptyOriginsDoNotAllowCrossOrigin(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{})(okHandler())

	req := httptest.NewRequest(http.MethodPost, "/admin/graphql", nil)
	req.Header.Set("Origin", "https://example.app")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty", got)
	}
}

func TestCORSMiddlewareAdminPreflightHeaders(t *testing.T) {
	handler := CORSMiddleware(CORSConfig{
		AllowedOrigins:       []string{"https://admin.example"},
		AllowAdminAPIKeyAuth: true,
	})(okHandler())

	req := httptest.NewRequest(http.MethodOptions, "/admin/graphql", nil)
	req.Header.Set("Origin", "https://admin.example")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	allowedHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"Content-Type", "Authorization", "DPoP", "X-Admin-API-Key", "X-User-DID"} {
		if !strings.Contains(allowedHeaders, header) {
			t.Fatalf("Access-Control-Allow-Headers = %q, want it to contain %q", allowedHeaders, header)
		}
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
