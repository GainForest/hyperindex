package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/oauth"
	"github.com/GainForest/hyperindex/internal/testutil"
)

// helper to get a *string from a literal
func strPtr(s string) *string { return &s }

// makeTestClient returns a simple oauth.Client with the given ID.
func makeTestClient(id string) *oauth.Client {
	now := time.Now().Unix()
	return &oauth.Client{
		ClientID:                id,
		ClientName:              "Test Client " + id,
		RedirectURIs:            []string{"https://example.com/callback"},
		GrantTypes:              []oauth.GrantType{oauth.GrantAuthorizationCode, oauth.GrantRefreshToken},
		ResponseTypes:           []oauth.ResponseType{oauth.ResponseCode},
		Scope:                   strPtr("read write"),
		TokenEndpointAuthMethod: oauth.AuthNone,
		ClientType:              oauth.ClientPublic,
		CreatedAt:               now,
		UpdatedAt:               now,
		Metadata:                "{}",
		AccessTokenExpiration:   3600,
		RefreshTokenExpiration:  86400,
		RequireRedirectExact:    true,
	}
}

// ---------------------------------------------------------------------------
// OAuthClientsRepository
// ---------------------------------------------------------------------------

func TestOAuthClientsRepository_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.OAuthClients
	ctx := context.Background()

	client := makeTestClient("test-client-1")

	// Insert
	if err := repo.Insert(ctx, client); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Get
	got, err := repo.Get(ctx, client.ClientID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want client")
	}
	if got.ClientID != client.ClientID {
		t.Errorf("ClientID = %q, want %q", got.ClientID, client.ClientID)
	}
	if got.ClientName != client.ClientName {
		t.Errorf("ClientName = %q, want %q", got.ClientName, client.ClientName)
	}
	if len(got.RedirectURIs) != 1 || got.RedirectURIs[0] != "https://example.com/callback" {
		t.Errorf("RedirectURIs = %v, want [https://example.com/callback]", got.RedirectURIs)
	}
	if len(got.GrantTypes) != 2 {
		t.Errorf("GrantTypes len = %d, want 2", len(got.GrantTypes))
	}
	if !got.RequireRedirectExact {
		t.Error("RequireRedirectExact = false, want true")
	}

	// Insert a second client
	client2 := makeTestClient("test-client-2")
	if err := repo.Insert(ctx, client2); err != nil {
		t.Fatalf("Insert(client2) error = %v", err)
	}

	// GetAll (excludes admin)
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	if len(all) != 2 {
		t.Errorf("GetAll() returned %d clients, want 2", len(all))
	}

	// GetCount (excludes admin)
	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("GetCount() = %d, want 2", count)
	}

	// Update
	client.ClientName = "Updated Client"
	client.RequireRedirectExact = false
	client.UpdatedAt = time.Now().Unix()
	if err := repo.Update(ctx, client); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, err = repo.Get(ctx, client.ClientID)
	if err != nil {
		t.Fatalf("Get() after Update error = %v", err)
	}
	if got.ClientName != "Updated Client" {
		t.Errorf("ClientName after Update = %q, want %q", got.ClientName, "Updated Client")
	}
	if got.RequireRedirectExact {
		t.Error("RequireRedirectExact after Update = true, want false")
	}

	// Delete
	if err := repo.Delete(ctx, client.ClientID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, err = repo.Get(ctx, client.ClientID)
	if err != nil {
		t.Fatalf("Get() after Delete error = %v", err)
	}
	if got != nil {
		t.Error("Get() after Delete returned non-nil, want nil")
	}

	// Count should be 1 now
	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 1 {
		t.Errorf("GetCount() after Delete = %d, want 1", count)
	}
}

func TestOAuthClientsRepository_EnsureAdminClient(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := db.OAuthClients
	ctx := context.Background()

	// First call creates the admin client
	if err := repo.EnsureAdminClient(ctx); err != nil {
		t.Fatalf("EnsureAdminClient() first call error = %v", err)
	}

	admin, err := repo.Get(ctx, "admin")
	if err != nil {
		t.Fatalf("Get(admin) error = %v", err)
	}
	if admin == nil {
		t.Fatal("admin client not found after EnsureAdminClient")
	}
	if admin.ClientName != "Admin UI" {
		t.Errorf("admin ClientName = %q, want %q", admin.ClientName, "Admin UI")
	}

	// Second call is idempotent
	if err := repo.EnsureAdminClient(ctx); err != nil {
		t.Fatalf("EnsureAdminClient() second call error = %v", err)
	}

	admin2, err := repo.Get(ctx, "admin")
	if err != nil {
		t.Fatalf("Get(admin) after second call error = %v", err)
	}
	if admin2.CreatedAt != admin.CreatedAt {
		t.Error("EnsureAdminClient() modified existing admin client")
	}

	// Admin should not appear in GetAll
	all, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}
	for _, c := range all {
		if c.ClientID == "admin" {
			t.Error("GetAll() returned admin client, should be excluded")
		}
	}
}

// ---------------------------------------------------------------------------
// OAuthAccessTokensRepository
// ---------------------------------------------------------------------------

func TestOAuthAccessTokensRepository_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repositories.NewOAuthAccessTokensRepository(db.Executor)
	clientsRepo := db.OAuthClients
	ctx := context.Background()

	// Insert a client first (FK dependency)
	client := makeTestClient("at-client")
	if err := clientsRepo.Insert(ctx, client); err != nil {
		t.Fatalf("Insert client error = %v", err)
	}

	now := time.Now().Unix()
	userID := "did:plc:testuser1"
	token := &oauth.AccessToken{
		Token:     "access-token-abc123",
		TokenType: oauth.TokenBearer,
		ClientID:  "at-client",
		UserID:    &userID,
		Scope:     strPtr("read write"),
		CreatedAt: now,
		ExpiresAt: now + 3600,
		Revoked:   false,
	}

	// Insert
	if err := repo.Insert(ctx, token); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Get
	got, err := repo.Get(ctx, "access-token-abc123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want token")
	}
	if got.Token != token.Token {
		t.Errorf("Token = %q, want %q", got.Token, token.Token)
	}
	if got.TokenType != oauth.TokenBearer {
		t.Errorf("TokenType = %q, want %q", got.TokenType, oauth.TokenBearer)
	}
	if got.ClientID != "at-client" {
		t.Errorf("ClientID = %q, want %q", got.ClientID, "at-client")
	}
	if got.UserID == nil || *got.UserID != userID {
		t.Errorf("UserID = %v, want %q", got.UserID, userID)
	}
	if got.Revoked {
		t.Error("Revoked = true, want false")
	}

	// Get non-existent
	missing, err := repo.Get(ctx, "nonexistent-token")
	if err != nil {
		t.Fatalf("Get(nonexistent) error = %v", err)
	}
	if missing != nil {
		t.Error("Get(nonexistent) returned non-nil, want nil")
	}

	// Insert second token for same user
	token2 := &oauth.AccessToken{
		Token:     "access-token-def456",
		TokenType: oauth.TokenDPoP,
		ClientID:  "at-client",
		UserID:    &userID,
		Scope:     strPtr("read"),
		CreatedAt: now + 1,
		ExpiresAt: now + 3600,
		Revoked:   false,
		DPoPJKT:   strPtr("jkt-thumbprint-xyz"),
	}
	if err := repo.Insert(ctx, token2); err != nil {
		t.Fatalf("Insert(token2) error = %v", err)
	}

	// GetByUserID
	userTokens, err := repo.GetByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v", err)
	}
	if len(userTokens) != 2 {
		t.Errorf("GetByUserID() returned %d tokens, want 2", len(userTokens))
	}

	// Revoke single token
	if err := repo.Revoke(ctx, "access-token-abc123"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	got, err = repo.Get(ctx, "access-token-abc123")
	if err != nil {
		t.Fatalf("Get() after Revoke error = %v", err)
	}
	if !got.Revoked {
		t.Error("Revoked = false after Revoke(), want true")
	}
	// Second token should still be active
	got2, _ := repo.Get(ctx, "access-token-def456")
	if got2.Revoked {
		t.Error("token2 Revoked = true, should still be false")
	}

	// RevokeByUserID
	if err := repo.RevokeByUserID(ctx, userID); err != nil {
		t.Fatalf("RevokeByUserID() error = %v", err)
	}
	got2, _ = repo.Get(ctx, "access-token-def456")
	if !got2.Revoked {
		t.Error("token2 Revoked = false after RevokeByUserID(), want true")
	}

	// DeleteExpired: insert an already-expired token, then delete
	expiredToken := &oauth.AccessToken{
		Token:     "access-token-expired",
		TokenType: oauth.TokenBearer,
		ClientID:  "at-client",
		UserID:    &userID,
		CreatedAt: now - 7200,
		ExpiresAt: now - 3600, // expired 1 hour ago
		Revoked:   false,
	}
	if err := repo.Insert(ctx, expiredToken); err != nil {
		t.Fatalf("Insert(expired) error = %v", err)
	}
	if err := repo.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	got, err = repo.Get(ctx, "access-token-expired")
	if err != nil {
		t.Fatalf("Get(expired) after DeleteExpired error = %v", err)
	}
	if got != nil {
		t.Error("expired token still exists after DeleteExpired")
	}
	// Non-expired tokens should still exist
	got, _ = repo.Get(ctx, "access-token-abc123")
	if got == nil {
		t.Error("non-expired token was deleted by DeleteExpired")
	}
}

// ---------------------------------------------------------------------------
// OAuthRefreshTokensRepository
// ---------------------------------------------------------------------------

func TestOAuthRefreshTokensRepository_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repositories.NewOAuthRefreshTokensRepository(db.Executor)
	atRepo := repositories.NewOAuthAccessTokensRepository(db.Executor)
	clientsRepo := db.OAuthClients
	ctx := context.Background()

	// Setup: client + access token
	client := makeTestClient("rt-client")
	if err := clientsRepo.Insert(ctx, client); err != nil {
		t.Fatalf("Insert client error = %v", err)
	}

	now := time.Now().Unix()
	userID := "did:plc:testuser1"
	accessToken := &oauth.AccessToken{
		Token:     "at-for-refresh-1",
		TokenType: oauth.TokenBearer,
		ClientID:  "rt-client",
		UserID:    &userID,
		CreatedAt: now,
		ExpiresAt: now + 3600,
	}
	if err := atRepo.Insert(ctx, accessToken); err != nil {
		t.Fatalf("Insert access token error = %v", err)
	}

	expiresAt := now + 86400
	refreshToken := &oauth.RefreshToken{
		Token:       "refresh-token-abc123",
		AccessToken: "at-for-refresh-1",
		ClientID:    "rt-client",
		UserID:      userID,
		Scope:       strPtr("read write"),
		CreatedAt:   now,
		ExpiresAt:   &expiresAt,
		Revoked:     false,
	}

	// Insert
	if err := repo.Insert(ctx, refreshToken); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Get
	got, err := repo.Get(ctx, "refresh-token-abc123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want token")
	}
	if got.Token != refreshToken.Token {
		t.Errorf("Token = %q, want %q", got.Token, refreshToken.Token)
	}
	if got.AccessToken != "at-for-refresh-1" {
		t.Errorf("AccessToken = %q, want %q", got.AccessToken, "at-for-refresh-1")
	}
	if got.UserID != userID {
		t.Errorf("UserID = %q, want %q", got.UserID, userID)
	}
	if got.ExpiresAt == nil || *got.ExpiresAt != expiresAt {
		t.Errorf("ExpiresAt = %v, want %d", got.ExpiresAt, expiresAt)
	}

	// GetByAccessToken
	got, err = repo.GetByAccessToken(ctx, "at-for-refresh-1")
	if err != nil {
		t.Fatalf("GetByAccessToken() error = %v", err)
	}
	if got == nil {
		t.Fatal("GetByAccessToken() returned nil")
	}
	if got.Token != "refresh-token-abc123" {
		t.Errorf("GetByAccessToken().Token = %q, want %q", got.Token, "refresh-token-abc123")
	}

	// Revoke
	if err := repo.Revoke(ctx, "refresh-token-abc123"); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	got, _ = repo.Get(ctx, "refresh-token-abc123")
	if !got.Revoked {
		t.Error("Revoked = false after Revoke(), want true")
	}

	// Insert a second token, then RevokeByUserID
	accessToken2 := &oauth.AccessToken{
		Token:     "at-for-refresh-2",
		TokenType: oauth.TokenBearer,
		ClientID:  "rt-client",
		UserID:    &userID,
		CreatedAt: now + 1,
		ExpiresAt: now + 3600,
	}
	if err := atRepo.Insert(ctx, accessToken2); err != nil {
		t.Fatalf("Insert access token 2 error = %v", err)
	}

	refreshToken2 := &oauth.RefreshToken{
		Token:       "refresh-token-def456",
		AccessToken: "at-for-refresh-2",
		ClientID:    "rt-client",
		UserID:      userID,
		CreatedAt:   now + 1,
		ExpiresAt:   &expiresAt,
		Revoked:     false,
	}
	if err := repo.Insert(ctx, refreshToken2); err != nil {
		t.Fatalf("Insert(token2) error = %v", err)
	}

	if err := repo.RevokeByUserID(ctx, userID); err != nil {
		t.Fatalf("RevokeByUserID() error = %v", err)
	}
	got, _ = repo.Get(ctx, "refresh-token-def456")
	if !got.Revoked {
		t.Error("token2 Revoked = false after RevokeByUserID(), want true")
	}

	// DeleteExpired: insert an already-expired token, then delete
	expiredAt := now - 3600
	expiredRefresh := &oauth.RefreshToken{
		Token:       "refresh-token-expired",
		AccessToken: "at-for-refresh-2",
		ClientID:    "rt-client",
		UserID:      userID,
		CreatedAt:   now - 7200,
		ExpiresAt:   &expiredAt,
	}
	if err := repo.Insert(ctx, expiredRefresh); err != nil {
		t.Fatalf("Insert(expired) error = %v", err)
	}
	if err := repo.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	got, _ = repo.Get(ctx, "refresh-token-expired")
	if got != nil {
		t.Error("expired refresh token still exists after DeleteExpired")
	}
	// Non-expired tokens should still exist
	got, _ = repo.Get(ctx, "refresh-token-abc123")
	if got == nil {
		t.Error("non-expired refresh token was deleted by DeleteExpired")
	}
}

// ---------------------------------------------------------------------------
// OAuthAuthorizationCodesRepository
// ---------------------------------------------------------------------------

func TestOAuthAuthorizationCodesRepository_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repositories.NewOAuthAuthorizationCodesRepository(db.Executor)
	clientsRepo := db.OAuthClients
	ctx := context.Background()

	// Setup: client
	client := makeTestClient("ac-client")
	if err := clientsRepo.Insert(ctx, client); err != nil {
		t.Fatalf("Insert client error = %v", err)
	}

	now := time.Now().Unix()
	code := &oauth.AuthorizationCode{
		Code:                "authcode-abc123",
		ClientID:            "ac-client",
		UserID:              "did:plc:testuser1",
		RedirectURI:         "https://example.com/callback",
		Scope:               strPtr("read write"),
		CodeChallenge:       strPtr("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"),
		CodeChallengeMethod: strPtr("S256"),
		Nonce:               strPtr("nonce123"),
		CreatedAt:           now,
		ExpiresAt:           now + 600, // 10 minutes
		Used:                false,
	}

	// Insert
	if err := repo.Insert(ctx, code); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Get
	got, err := repo.Get(ctx, "authcode-abc123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want code")
	}
	if got.Code != code.Code {
		t.Errorf("Code = %q, want %q", got.Code, code.Code)
	}
	if got.ClientID != "ac-client" {
		t.Errorf("ClientID = %q, want %q", got.ClientID, "ac-client")
	}
	if got.UserID != "did:plc:testuser1" {
		t.Errorf("UserID = %q, want %q", got.UserID, "did:plc:testuser1")
	}
	if got.RedirectURI != "https://example.com/callback" {
		t.Errorf("RedirectURI = %q, want %q", got.RedirectURI, "https://example.com/callback")
	}
	if got.CodeChallenge == nil || *got.CodeChallenge != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Errorf("CodeChallenge = %v, want E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM", got.CodeChallenge)
	}
	if got.CodeChallengeMethod == nil || *got.CodeChallengeMethod != "S256" {
		t.Errorf("CodeChallengeMethod = %v, want S256", got.CodeChallengeMethod)
	}
	if got.Nonce == nil || *got.Nonce != "nonce123" {
		t.Errorf("Nonce = %v, want nonce123", got.Nonce)
	}
	if got.Used {
		t.Error("Used = true, want false")
	}

	// Get non-existent
	missing, err := repo.Get(ctx, "nonexistent-code")
	if err != nil {
		t.Fatalf("Get(nonexistent) error = %v", err)
	}
	if missing != nil {
		t.Error("Get(nonexistent) returned non-nil, want nil")
	}

	// MarkUsed
	if err := repo.MarkUsed(ctx, "authcode-abc123"); err != nil {
		t.Fatalf("MarkUsed() error = %v", err)
	}
	got, _ = repo.Get(ctx, "authcode-abc123")
	if !got.Used {
		t.Error("Used = false after MarkUsed(), want true")
	}

	// DeleteExpired: insert an expired code, then clean up
	expiredCode := &oauth.AuthorizationCode{
		Code:        "authcode-expired",
		ClientID:    "ac-client",
		UserID:      "did:plc:testuser1",
		RedirectURI: "https://example.com/callback",
		CreatedAt:   now - 1200,
		ExpiresAt:   now - 600, // expired 10 minutes ago
		Used:        false,
	}
	if err := repo.Insert(ctx, expiredCode); err != nil {
		t.Fatalf("Insert(expired) error = %v", err)
	}
	if err := repo.DeleteExpired(ctx, now); err != nil {
		t.Fatalf("DeleteExpired() error = %v", err)
	}
	got, _ = repo.Get(ctx, "authcode-expired")
	if got != nil {
		t.Error("expired code still exists after DeleteExpired")
	}
	// Non-expired code should remain
	got, _ = repo.Get(ctx, "authcode-abc123")
	if got == nil {
		t.Error("non-expired code was deleted by DeleteExpired")
	}
}

// ---------------------------------------------------------------------------
// OAuthDPoPJTIRepository
// ---------------------------------------------------------------------------

func TestOAuthDPoPJTIRepository_CRUD(t *testing.T) {
	db := testutil.SetupTestDB(t)
	repo := repositories.NewOAuthDPoPJTIRepository(db.Executor)
	ctx := context.Background()

	now := time.Now().Unix()

	jti := &oauth.DPoPJTI{
		JTI:       "unique-jti-abc123",
		CreatedAt: now,
	}

	// Insert
	if err := repo.Insert(ctx, jti); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Exists - should be true
	exists, err := repo.Exists(ctx, "unique-jti-abc123")
	if err != nil {
		t.Fatalf("Exists() error = %v", err)
	}
	if !exists {
		t.Error("Exists() = false, want true for inserted JTI")
	}

	// Exists - non-existing
	exists, err = repo.Exists(ctx, "nonexistent-jti")
	if err != nil {
		t.Fatalf("Exists(nonexistent) error = %v", err)
	}
	if exists {
		t.Error("Exists(nonexistent) = true, want false")
	}

	// Get
	got, err := repo.Get(ctx, "unique-jti-abc123")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want JTI")
	}
	if got.JTI != "unique-jti-abc123" {
		t.Errorf("JTI = %q, want %q", got.JTI, "unique-jti-abc123")
	}
	if got.CreatedAt != now {
		t.Errorf("CreatedAt = %d, want %d", got.CreatedAt, now)
	}

	// DeleteOlderThan: insert an old JTI, then clean up
	oldJTI := &oauth.DPoPJTI{
		JTI:       "old-jti-def456",
		CreatedAt: now - 7200, // 2 hours ago
	}
	if err := repo.Insert(ctx, oldJTI); err != nil {
		t.Fatalf("Insert(old) error = %v", err)
	}

	// Verify it exists before cleanup
	exists, _ = repo.Exists(ctx, "old-jti-def456")
	if !exists {
		t.Fatal("old JTI should exist before cleanup")
	}

	// Delete entries older than 1 hour ago
	if err := repo.DeleteOlderThan(ctx, now-3600); err != nil {
		t.Fatalf("DeleteOlderThan() error = %v", err)
	}

	// Old JTI should be gone
	exists, _ = repo.Exists(ctx, "old-jti-def456")
	if exists {
		t.Error("old JTI still exists after DeleteOlderThan")
	}

	// Recent JTI should remain
	exists, _ = repo.Exists(ctx, "unique-jti-abc123")
	if !exists {
		t.Error("recent JTI was deleted by DeleteOlderThan")
	}

	// Delete specific
	if err := repo.Delete(ctx, "unique-jti-abc123"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	exists, _ = repo.Exists(ctx, "unique-jti-abc123")
	if exists {
		t.Error("JTI still exists after Delete")
	}
}
