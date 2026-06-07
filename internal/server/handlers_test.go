package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// TestHandleDPoPNonce
// ---------------------------------------------------------------------------

func TestHandleDPoPNonce(t *testing.T) {
	t.Run("GET returns 200 with nonce header and body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/oauth/dpop/nonce", nil)
		rec := httptest.NewRecorder()

		HandleDPoPNonce(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		headerNonce := rec.Header().Get("DPoP-Nonce")
		if headerNonce == "" {
			t.Fatal("expected non-empty DPoP-Nonce header")
		}

		bodyNonce := rec.Body.String()
		if bodyNonce == "" {
			t.Fatal("expected non-empty response body")
		}

		if headerNonce != bodyNonce {
			t.Fatalf("header nonce %q != body nonce %q", headerNonce, bodyNonce)
		}
	})

	t.Run("POST returns 200 with nonce header and body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/oauth/dpop/nonce", nil)
		rec := httptest.NewRecorder()

		HandleDPoPNonce(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		headerNonce := rec.Header().Get("DPoP-Nonce")
		if headerNonce == "" {
			t.Fatal("expected non-empty DPoP-Nonce header")
		}

		bodyNonce := rec.Body.String()
		if bodyNonce == "" {
			t.Fatal("expected non-empty response body")
		}

		if headerNonce != bodyNonce {
			t.Fatalf("header nonce %q != body nonce %q", headerNonce, bodyNonce)
		}
	})

	t.Run("consecutive calls produce different nonces", func(t *testing.T) {
		nonces := make(map[string]bool)
		const iterations = 10

		for i := 0; i < iterations; i++ {
			req := httptest.NewRequest(http.MethodGet, "/oauth/dpop/nonce", nil)
			rec := httptest.NewRecorder()

			HandleDPoPNonce(rec, req)

			nonce := rec.Body.String()
			if nonces[nonce] {
				t.Fatalf("duplicate nonce produced on iteration %d: %q", i, nonce)
			}
			nonces[nonce] = true
		}

		if len(nonces) != iterations {
			t.Fatalf("expected %d unique nonces, got %d", iterations, len(nonces))
		}
	})

	t.Run("PUT returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/oauth/dpop/nonce", nil)
		rec := httptest.NewRecorder()

		HandleDPoPNonce(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", rec.Code)
		}

		allow := rec.Header().Get("Allow")
		if allow != "GET, POST" {
			t.Fatalf("expected Allow header %q, got %q", "GET, POST", allow)
		}
	})
}

// ---------------------------------------------------------------------------
// TestHandleClientMetadata
// ---------------------------------------------------------------------------

func TestHandleClientMetadata(t *testing.T) {
	baseCfg := ClientMetadataConfig{
		ExternalBaseURL: "https://example.com",
		ClientName:      "Test App",
	}

	t.Run("GET returns 200 with correct JSON structure", func(t *testing.T) {
		handler := HandleClientMetadata(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		ct := rec.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Fatalf("expected Content-Type application/json, got %q", ct)
		}

		var resp ClientMetadataResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		// client_id = base URL + /oauth-client-metadata.json
		wantClientID := "https://example.com/oauth-client-metadata.json"
		if resp.ClientID != wantClientID {
			t.Errorf("client_id = %q, want %q", resp.ClientID, wantClientID)
		}

		if resp.ClientName != "Test App" {
			t.Errorf("client_name = %q, want %q", resp.ClientName, "Test App")
		}

		// redirect_uris
		if len(resp.RedirectURIs) != 1 {
			t.Fatalf("expected 1 redirect URI, got %d", len(resp.RedirectURIs))
		}
		wantRedirect := "https://example.com/oauth/callback"
		if resp.RedirectURIs[0] != wantRedirect {
			t.Errorf("redirect_uris[0] = %q, want %q", resp.RedirectURIs[0], wantRedirect)
		}

		// grant_types
		wantGrants := []string{"authorization_code", "refresh_token"}
		if len(resp.GrantTypes) != len(wantGrants) {
			t.Fatalf("grant_types length = %d, want %d", len(resp.GrantTypes), len(wantGrants))
		}
		for i, g := range wantGrants {
			if resp.GrantTypes[i] != g {
				t.Errorf("grant_types[%d] = %q, want %q", i, resp.GrantTypes[i], g)
			}
		}

		// response_types
		if len(resp.ResponseTypes) != 1 || resp.ResponseTypes[0] != "code" {
			t.Errorf("response_types = %v, want [code]", resp.ResponseTypes)
		}

		// Default scope
		if resp.Scope != "atproto" {
			t.Errorf("scope = %q, want %q", resp.Scope, "atproto")
		}

		// Fixed fields
		if resp.TokenEndpointAuthMethod != "private_key_jwt" {
			t.Errorf("token_endpoint_auth_method = %q, want %q", resp.TokenEndpointAuthMethod, "private_key_jwt")
		}
		if resp.TokenEndpointAuthSigningAlg != "ES256" {
			t.Errorf("token_endpoint_auth_signing_alg = %q, want %q", resp.TokenEndpointAuthSigningAlg, "ES256")
		}
		if resp.ApplicationType != "web" {
			t.Errorf("application_type = %q, want %q", resp.ApplicationType, "web")
		}
		if !resp.DPoPBoundAccessTokens {
			t.Error("dpop_bound_access_tokens = false, want true")
		}

		// Default JWKS URI when none configured
		if resp.JWKSUri == nil {
			t.Fatal("expected jwks_uri to be set")
		}
		wantJWKS := "https://example.com/oauth/jwks"
		if *resp.JWKSUri != wantJWKS {
			t.Errorf("jwks_uri = %q, want %q", *resp.JWKSUri, wantJWKS)
		}
	})

	t.Run("Cache-Control header is set", func(t *testing.T) {
		handler := HandleClientMetadata(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		cc := rec.Header().Get("Cache-Control")
		if cc == "" {
			t.Fatal("expected Cache-Control header to be set")
		}
		if !strings.Contains(cc, "max-age=") {
			t.Errorf("Cache-Control %q does not contain max-age", cc)
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		handler := HandleClientMetadata(baseCfg)
		req := httptest.NewRequest(http.MethodPost, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", rec.Code)
		}

		allow := rec.Header().Get("Allow")
		if allow != http.MethodGet {
			t.Errorf("Allow header = %q, want %q", allow, http.MethodGet)
		}
	})

	t.Run("optional fields included when configured", func(t *testing.T) {
		cfg := ClientMetadataConfig{
			ExternalBaseURL: "https://example.com",
			ClientName:      "Test App",
			ClientURI:       "https://example.com",
			LogoURI:         "https://example.com/logo.png",
			TOSUri:          "https://example.com/tos",
			PolicyURI:       "https://example.com/privacy",
			Scope:           "atproto transition:generic",
		}
		handler := HandleClientMetadata(cfg)
		req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var resp ClientMetadataResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.ClientURI == nil || *resp.ClientURI != cfg.ClientURI {
			t.Errorf("client_uri = %v, want %q", resp.ClientURI, cfg.ClientURI)
		}
		if resp.LogoURI == nil || *resp.LogoURI != cfg.LogoURI {
			t.Errorf("logo_uri = %v, want %q", resp.LogoURI, cfg.LogoURI)
		}
		if resp.TOSUri == nil || *resp.TOSUri != cfg.TOSUri {
			t.Errorf("tos_uri = %v, want %q", resp.TOSUri, cfg.TOSUri)
		}
		if resp.PolicyURI == nil || *resp.PolicyURI != cfg.PolicyURI {
			t.Errorf("policy_uri = %v, want %q", resp.PolicyURI, cfg.PolicyURI)
		}
		if resp.Scope != cfg.Scope {
			t.Errorf("scope = %q, want %q", resp.Scope, cfg.Scope)
		}
	})

	t.Run("optional fields omitted when empty", func(t *testing.T) {
		handler := HandleClientMetadata(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Decode into raw map to check field presence
		var raw map[string]json.RawMessage
		if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		for _, field := range []string{"client_uri", "logo_uri", "tos_uri", "policy_uri"} {
			if _, exists := raw[field]; exists {
				t.Errorf("expected field %q to be omitted, but it was present", field)
			}
		}
	})

	t.Run("custom scope overrides default", func(t *testing.T) {
		cfg := baseCfg
		cfg.Scope = "atproto transition:generic"
		handler := HandleClientMetadata(cfg)
		req := httptest.NewRequest(http.MethodGet, "/oauth-client-metadata.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		var resp ClientMetadataResponse
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.Scope != "atproto transition:generic" {
			t.Errorf("scope = %q, want %q", resp.Scope, "atproto transition:generic")
		}
	})
}

// ---------------------------------------------------------------------------
// TestHandleGraphiQL
// ---------------------------------------------------------------------------

func TestHandleGraphiQL(t *testing.T) {
	baseCfg := GraphiQLConfig{
		Endpoint: "/graphql",
		Title:    "Hyperindex GraphiQL",
	}

	t.Run("GET returns 200 with text/html content type", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d", rec.Code)
		}

		ct := rec.Header().Get("Content-Type")
		if !strings.HasPrefix(ct, "text/html") {
			t.Fatalf("expected Content-Type text/html, got %q", ct)
		}
	})

	t.Run("body contains endpoint URL", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "/graphql") {
			t.Error("response body does not contain endpoint URL /graphql")
		}
	})

	t.Run("body contains title", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "Hyperindex GraphiQL") {
			t.Error("response body does not contain title")
		}
	})

	t.Run("body contains official GraphiQL explorer plugin", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "@graphiql/plugin-explorer") {
			t.Error("response body does not load the official GraphiQL explorer plugin")
		}
		if !strings.Contains(body, "explorerPlugin()") {
			t.Error("response body does not wire the GraphiQL explorer plugin")
		}
		if !strings.Contains(body, "visiblePlugin: 'GraphiQL Explorer'") {
			t.Error("response body does not open the GraphiQL explorer plugin")
		}
		if strings.Contains(body, "Schema Builder") {
			t.Error("response body should not contain the custom Schema Builder panel")
		}
	})

	t.Run("subscription endpoint included when configured", func(t *testing.T) {
		cfg := GraphiQLConfig{
			Endpoint:             "/graphql",
			SubscriptionEndpoint: "ws://localhost:8080/graphql/ws",
			Title:                "Test",
		}
		handler := HandleGraphiQL(cfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "ws://localhost:8080/graphql/ws") {
			t.Error("response body does not contain subscription endpoint")
		}
		if !strings.Contains(body, "subscriptionUrl") {
			t.Error("response body does not contain subscriptionUrl config key")
		}
	})

	t.Run("subscription endpoint absent when not configured", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if strings.Contains(body, "subscriptionUrl") {
			t.Error("response body should not contain subscriptionUrl when not configured")
		}
	})

	t.Run("POST returns 405", func(t *testing.T) {
		handler := HandleGraphiQL(baseCfg)
		req := httptest.NewRequest(http.MethodPost, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", rec.Code)
		}
	})

	t.Run("default title used when not configured", func(t *testing.T) {
		cfg := GraphiQLConfig{
			Endpoint: "/graphql",
		}
		handler := HandleGraphiQL(cfg)
		req := httptest.NewRequest(http.MethodGet, "/graphiql", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body := rec.Body.String()
		if !strings.Contains(body, "<title>GraphiQL</title>") {
			t.Error("response body does not contain default title")
		}
	})
}

// ---------------------------------------------------------------------------
// TestConnectDatabase
// ---------------------------------------------------------------------------

func TestConnectDatabase(t *testing.T) {
	t.Run("sqlite memory URL succeeds", func(t *testing.T) {
		executor, err := ConnectDatabase("sqlite::memory:")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		defer executor.Close()

		if executor == nil {
			t.Fatal("expected non-nil executor")
		}
	})

	t.Run("invalid sqlite path returns error", func(t *testing.T) {
		// A path to a non-existent deeply nested directory should fail.
		_, err := ConnectDatabase("sqlite:/no/such/directory/that/exists/test.db")
		if err == nil {
			t.Fatal("expected error for invalid SQLite path")
		}
	})
}
