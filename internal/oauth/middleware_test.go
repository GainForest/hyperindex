package oauth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockTokenStore implements AccessTokenStore for testing.
type mockTokenStore struct {
	tokens map[string]*AccessToken
	err    error
}

func (m *mockTokenStore) Get(ctx context.Context, token string) (*AccessToken, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.tokens[token], nil
}

// mockJTIStore implements JTIStore for testing.
type mockJTIStore struct {
	jtis map[string]bool
	err  error
}

func (m *mockJTIStore) Exists(ctx context.Context, jti string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.jtis[jti], nil
}

func (m *mockJTIStore) Insert(ctx context.Context, jti *DPoPJTI) error {
	if m.err != nil {
		return m.err
	}
	if m.jtis == nil {
		m.jtis = make(map[string]bool)
	}
	m.jtis[jti.JTI] = true
	return nil
}

func TestAuthMiddleware_RequireAuth_NoHeader(t *testing.T) {
	middleware := NewAuthMiddleware(&mockTokenStore{}, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_InvalidFormat(t *testing.T) {
	middleware := NewAuthMiddleware(&mockTokenStore{}, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "InvalidFormat")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_InvalidScheme(t *testing.T) {
	middleware := NewAuthMiddleware(&mockTokenStore{}, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_BearerToken_Valid(t *testing.T) {
	userID := "did:plc:test123"
	scope := "atproto transition:generic"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"valid-token": {
				Token:     "valid-token",
				TokenType: TokenBearer,
				UserID:    &userID,
				Scope:     &scope,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   nil, // Not DPoP-bound
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	var capturedUserID string
	var capturedScopes []string
	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		capturedScopes = ScopesFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if capturedUserID != userID {
		t.Errorf("expected user ID %q, got %q", userID, capturedUserID)
	}
	if len(capturedScopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(capturedScopes))
	}
}

func TestAuthMiddleware_RequireAuth_BearerToken_Expired(t *testing.T) {
	userID := "did:plc:test123"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"expired-token": {
				Token:     "expired-token",
				TokenType: TokenBearer,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(-time.Hour).Unix(), // Expired
				Revoked:   false,
				DPoPJKT:   nil,
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_BearerToken_Revoked(t *testing.T) {
	userID := "did:plc:test123"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"revoked-token": {
				Token:     "revoked-token",
				TokenType: TokenBearer,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   true, // Revoked
				DPoPJKT:   nil,
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer revoked-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_BearerToken_DPoPBound(t *testing.T) {
	userID := "did:plc:test123"
	jkt := "some-jkt"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"dpop-token": {
				Token:     "dpop-token",
				TokenType: TokenDPoP,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   &jkt, // DPoP-bound
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer dpop-token") // Using Bearer with DPoP-bound token
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_DPoP_Valid(t *testing.T) {
	// Generate a key pair for testing
	keyPair, err := GenerateDPoPKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	jkt, err := keyPair.CalculateJKT()
	if err != nil {
		t.Fatalf("failed to calculate JKT: %v", err)
	}
	userID := "did:plc:test123"

	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"dpop-access-token": {
				Token:     "dpop-access-token",
				TokenType: TokenDPoP,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   &jkt,
			},
		},
	}

	jtis := &mockJTIStore{jtis: make(map[string]bool)}
	middleware := NewAuthMiddleware(tokens, jtis, "https://example.com")

	var capturedUserID string
	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// Generate DPoP proof
	proof, err := keyPair.GenerateDPoPProof("GET", "https://example.com/protected", "", "")
	if err != nil {
		t.Fatalf("failed to generate DPoP proof: %v", err)
	}

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "DPoP dpop-access-token")
	req.Header.Set("DPoP", proof)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if capturedUserID != userID {
		t.Errorf("expected user ID %q, got %q", userID, capturedUserID)
	}
}

func TestAuthMiddleware_RequireAuth_DPoP_MissingProof(t *testing.T) {
	userID := "did:plc:test123"
	jkt := "some-jkt"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"dpop-access-token": {
				Token:     "dpop-access-token",
				TokenType: TokenDPoP,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   &jkt,
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "DPoP dpop-access-token")
	// No DPoP header
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_DPoP_KeyMismatch(t *testing.T) {
	// Generate two different key pairs
	keyPair1, _ := GenerateDPoPKeyPair()
	keyPair2, _ := GenerateDPoPKeyPair()

	jkt, err := keyPair1.CalculateJKT() // Token bound to key 1
	if err != nil {
		t.Fatalf("failed to calculate JKT: %v", err)
	}
	userID := "did:plc:test123"

	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"dpop-access-token": {
				Token:     "dpop-access-token",
				TokenType: TokenDPoP,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   &jkt,
			},
		},
	}

	jtis := &mockJTIStore{jtis: make(map[string]bool)}
	middleware := NewAuthMiddleware(tokens, jtis, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	// Generate DPoP proof with key 2 (wrong key)
	proof, _ := keyPair2.GenerateDPoPProof("GET", "https://example.com/protected", "", "")

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "DPoP dpop-access-token")
	req.Header.Set("DPoP", proof)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireAuth_DPoP_Replay(t *testing.T) {
	keyPair, _ := GenerateDPoPKeyPair()
	jkt, err := keyPair.CalculateJKT()
	if err != nil {
		t.Fatalf("failed to calculate JKT: %v", err)
	}
	userID := "did:plc:test123"

	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"dpop-access-token": {
				Token:     "dpop-access-token",
				TokenType: TokenDPoP,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   &jkt,
			},
		},
	}

	jtis := &mockJTIStore{jtis: make(map[string]bool)}
	middleware := NewAuthMiddleware(tokens, jtis, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Generate DPoP proof
	proof, _ := keyPair.GenerateDPoPProof("GET", "https://example.com/protected", "", "")

	// First request should succeed
	req1 := httptest.NewRequest("GET", "/protected", nil)
	req1.Header.Set("Authorization", "DPoP dpop-access-token")
	req1.Header.Set("DPoP", proof)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("first request: expected status 200, got %d", rec1.Code)
	}

	// Second request with same proof should fail (replay)
	req2 := httptest.NewRequest("GET", "/protected", nil)
	req2.Header.Set("Authorization", "DPoP dpop-access-token")
	req2.Header.Set("DPoP", proof) // Same proof
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusUnauthorized {
		t.Errorf("replay request: expected status 401, got %d", rec2.Code)
	}
}

func TestAuthMiddleware_OptionalAuth_NoHeader(t *testing.T) {
	middleware := NewAuthMiddleware(&mockTokenStore{}, &mockJTIStore{}, "https://example.com")

	var called bool
	var capturedUserID string
	handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/optional", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if capturedUserID != "" {
		t.Errorf("expected empty user ID, got %q", capturedUserID)
	}
}

func TestAuthMiddleware_OptionalAuth_ValidToken(t *testing.T) {
	userID := "did:plc:test123"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"valid-token": {
				Token:     "valid-token",
				TokenType: TokenBearer,
				UserID:    &userID,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   nil,
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	var capturedUserID string
	handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedUserID = UserIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/optional", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
	if capturedUserID != userID {
		t.Errorf("expected user ID %q, got %q", userID, capturedUserID)
	}
}

func TestAuthMiddleware_OptionalAuth_InvalidToken(t *testing.T) {
	middleware := NewAuthMiddleware(&mockTokenStore{}, &mockJTIStore{}, "https://example.com")

	handler := middleware.OptionalAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/optional", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rec.Code)
	}
}

func TestAuthMiddleware_RequireScope(t *testing.T) {
	userID := "did:plc:test123"
	scope := "atproto transition:generic read:profile"
	tokens := &mockTokenStore{
		tokens: map[string]*AccessToken{
			"valid-token": {
				Token:     "valid-token",
				TokenType: TokenBearer,
				UserID:    &userID,
				Scope:     &scope,
				ExpiresAt: time.Now().Add(time.Hour).Unix(),
				Revoked:   false,
				DPoPJKT:   nil,
			},
		},
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	t.Run("has required scope", func(t *testing.T) {
		var called bool
		handler := middleware.RequireAuth(
			middleware.RequireScope("atproto")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})),
		)

		req := httptest.NewRequest("GET", "/scoped", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("handler should be called")
		}
		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("missing required scope", func(t *testing.T) {
		handler := middleware.RequireAuth(
			middleware.RequireScope("admin:write")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Error("handler should not be called")
			})),
		)

		req := httptest.NewRequest("GET", "/scoped", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", rec.Code)
		}
	})

	t.Run("multiple required scopes - all present", func(t *testing.T) {
		var called bool
		handler := middleware.RequireAuth(
			middleware.RequireScope("atproto", "transition:generic")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})),
		)

		req := httptest.NewRequest("GET", "/scoped", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if !called {
			t.Error("handler should be called")
		}
	})
}

func TestAuthMiddleware_ServerError(t *testing.T) {
	tokens := &mockTokenStore{
		err: errors.New("database error"),
	}

	middleware := NewAuthMiddleware(tokens, &mockJTIStore{}, "https://example.com")

	handler := middleware.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer some-token")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}
}

func TestContextHelpers(t *testing.T) {
	t.Run("empty context", func(t *testing.T) {
		ctx := context.Background()

		if UserIDFromContext(ctx) != "" {
			t.Error("expected empty user ID")
		}
		if AccessTokenFromContext(ctx) != nil {
			t.Error("expected nil access token")
		}
		if ScopesFromContext(ctx) != nil {
			t.Error("expected nil scopes")
		}
	})

	t.Run("populated context", func(t *testing.T) {
		userID := "did:plc:test123"
		token := &AccessToken{Token: "test-token"}
		scopes := []string{"atproto", "read:profile"}

		ctx := context.Background()
		ctx = context.WithValue(ctx, UserIDKey, userID)
		ctx = context.WithValue(ctx, AccessTokenKey, token)
		ctx = context.WithValue(ctx, ScopesKey, scopes)

		if UserIDFromContext(ctx) != userID {
			t.Errorf("expected user ID %q, got %q", userID, UserIDFromContext(ctx))
		}
		if AccessTokenFromContext(ctx) != token {
			t.Error("expected access token")
		}
		if len(ScopesFromContext(ctx)) != 2 {
			t.Errorf("expected 2 scopes, got %d", len(ScopesFromContext(ctx)))
		}
	})
}

func TestAuthError_HTTPStatus(t *testing.T) {
	tests := []struct {
		err      *AuthError
		expected int
	}{
		{ErrMissingAuth, http.StatusUnauthorized},
		{ErrInvalidToken, http.StatusUnauthorized},
		{ErrTokenExpired, http.StatusUnauthorized},
		{ErrInsufficientScope, http.StatusForbidden},
		{ErrServerError, http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.err.Code, func(t *testing.T) {
			if tt.err.HTTPStatus() != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, tt.err.HTTPStatus())
			}
		})
	}
}

func TestUseJTI(t *testing.T) {
	t.Run("new JTI", func(t *testing.T) {
		store := &mockJTIStore{jtis: make(map[string]bool)}
		ok, err := UseJTI(context.Background(), store, "new-jti", time.Now().Unix())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if !ok {
			t.Error("expected ok=true for new JTI")
		}
	})

	t.Run("existing JTI", func(t *testing.T) {
		store := &mockJTIStore{jtis: map[string]bool{"existing-jti": true}}
		ok, err := UseJTI(context.Background(), store, "existing-jti", time.Now().Unix())
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if ok {
			t.Error("expected ok=false for existing JTI")
		}
	})

	t.Run("store error", func(t *testing.T) {
		store := &mockJTIStore{err: errors.New("database error")}
		_, err := UseJTI(context.Background(), store, "any-jti", time.Now().Unix())
		if err == nil {
			t.Error("expected error")
		}
	})
}
