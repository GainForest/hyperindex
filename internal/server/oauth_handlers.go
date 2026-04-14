// Package server contains HTTP handlers for the hypergoat server.
// OAuth HTTP handlers for AT Protocol authentication.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GainForest/hypergoat/internal/database"
	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/oauth"
)

// OAuthHandlerConfig contains configuration for OAuth handlers.
type OAuthHandlerConfig struct {
	// ExternalBaseURL is the public-facing URL of this server.
	ExternalBaseURL string
	// ClientID is this server's OAuth client ID (typically the client metadata URL).
	ClientID string
	// CallbackURL is the OAuth callback URL for this server.
	CallbackURL string
	// SigningKey is the private key for signing client assertions (optional).
	SigningKey *oauth.DPoPKeyPair
	// Issuer is the OAuth issuer identifier.
	Issuer string
	// ScopesSupported lists the supported OAuth scopes.
	ScopesSupported []string
	// AccessTokenExpiration is the access token lifetime in seconds.
	AccessTokenExpiration int64
	// RefreshTokenExpiration is the refresh token lifetime in seconds (0 = no expiry).
	RefreshTokenExpiration int64
	// AuthorizationCodeExpiration is the auth code lifetime in seconds.
	AuthorizationCodeExpiration int64
}

// OAuthHandlers provides HTTP handlers for OAuth endpoints.
type OAuthHandlers struct {
	config      OAuthHandlerConfig
	db          database.Executor
	didResolver *oauth.DIDResolver
	bridge      *oauth.Bridge

	// Repositories
	authRequests  *repositories.OAuthAuthRequestsRepository
	atpRequests   *repositories.OAuthATPRequestsRepository
	atpSessions   *repositories.OAuthATPSessionsRepository
	authCodes     *repositories.OAuthAuthorizationCodesRepository
	accessTokens  *repositories.OAuthAccessTokensRepository
	refreshTokens *repositories.OAuthRefreshTokensRepository
	clients       *repositories.OAuthClientsRepository
	dpopJTIs      *repositories.OAuthDPoPJTIRepository
}

// NewOAuthHandlers creates a new OAuth handlers instance.
func NewOAuthHandlers(cfg OAuthHandlerConfig, db database.Executor) *OAuthHandlers {
	// Set defaults
	if cfg.AccessTokenExpiration == 0 {
		cfg.AccessTokenExpiration = 3600 // 1 hour
	}
	if cfg.RefreshTokenExpiration == 0 {
		cfg.RefreshTokenExpiration = 1209600 // 14 days
	}
	if cfg.AuthorizationCodeExpiration == 0 {
		cfg.AuthorizationCodeExpiration = 600 // 10 minutes
	}

	didResolver := oauth.NewDIDResolver()
	bridge := oauth.NewBridge(oauth.BridgeConfig{
		DIDResolver: didResolver,
		ClientID:    cfg.ClientID,
		SigningKey:  cfg.SigningKey,
		CallbackURL: cfg.CallbackURL,
	})

	return &OAuthHandlers{
		config:        cfg,
		db:            db,
		didResolver:   didResolver,
		bridge:        bridge,
		authRequests:  repositories.NewOAuthAuthRequestsRepository(db),
		atpRequests:   repositories.NewOAuthATPRequestsRepository(db),
		atpSessions:   repositories.NewOAuthATPSessionsRepository(db),
		authCodes:     repositories.NewOAuthAuthorizationCodesRepository(db),
		accessTokens:  repositories.NewOAuthAccessTokensRepository(db),
		refreshTokens: repositories.NewOAuthRefreshTokensRepository(db),
		clients:       repositories.NewOAuthClientsRepository(db),
		dpopJTIs:      repositories.NewOAuthDPoPJTIRepository(db),
	}
}

// AuthorizationServerMetadataResponse is the OAuth server metadata response.
type AuthorizationServerMetadataResponse struct {
	Issuer                             string   `json:"issuer"`
	AuthorizationEndpoint              string   `json:"authorization_endpoint"`
	TokenEndpoint                      string   `json:"token_endpoint"`
	PushedAuthorizationRequestEndpoint *string  `json:"pushed_authorization_request_endpoint,omitempty"`
	RevocationEndpoint                 *string  `json:"revocation_endpoint,omitempty"`
	JWKSUri                            *string  `json:"jwks_uri,omitempty"`
	ResponseTypesSupported             []string `json:"response_types_supported"`
	GrantTypesSupported                []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported      []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported  []string `json:"token_endpoint_auth_methods_supported"`
	DPoPSigningAlgValuesSupported      []string `json:"dpop_signing_alg_values_supported,omitempty"`
	ScopesSupported                    []string `json:"scopes_supported,omitempty"`
}

// ProtectedResourceMetadataResponse is the OAuth protected resource metadata response.
type ProtectedResourceMetadataResponse struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// HandleAuthorizationServerMetadata handles GET /.well-known/oauth-authorization-server.
func (h *OAuthHandlers) HandleAuthorizationServerMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, []string{http.MethodGet})
		return
	}

	baseURL := h.config.ExternalBaseURL

	metadata := AuthorizationServerMetadataResponse{
		Issuer:                            h.config.Issuer,
		AuthorizationEndpoint:             baseURL + "/oauth/authorize",
		TokenEndpoint:                     baseURL + "/oauth/token",
		ResponseTypesSupported:            []string{"code"},
		GrantTypesSupported:               []string{"authorization_code", "refresh_token"},
		CodeChallengeMethodsSupported:     []string{"S256"},
		TokenEndpointAuthMethodsSupported: []string{"none", "private_key_jwt"},
		DPoPSigningAlgValuesSupported:     []string{"ES256"},
		ScopesSupported:                   h.config.ScopesSupported,
	}

	// Optional endpoints
	parEndpoint := baseURL + "/oauth/par"
	metadata.PushedAuthorizationRequestEndpoint = &parEndpoint

	jwksURI := baseURL + "/oauth/jwks"
	metadata.JWKSUri = &jwksURI

	revocationEndpoint := baseURL + "/oauth/revoke"
	metadata.RevocationEndpoint = &revocationEndpoint

	h.writeJSON(w, http.StatusOK, metadata)
}

// HandleProtectedResourceMetadata handles GET /.well-known/oauth-protected-resource.
func (h *OAuthHandlers) HandleProtectedResourceMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, []string{http.MethodGet})
		return
	}

	metadata := ProtectedResourceMetadataResponse{
		Resource:               h.config.ExternalBaseURL,
		AuthorizationServers:   []string{h.config.Issuer},
		ScopesSupported:        h.config.ScopesSupported,
		BearerMethodsSupported: []string{"header"},
	}

	h.writeJSON(w, http.StatusOK, metadata)
}

// HandleAuthorize handles GET/POST /oauth/authorize.
// This initiates the OAuth authorization flow.
func (h *OAuthHandlers) HandleAuthorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, []string{http.MethodGet, http.MethodPost})
		return
	}

	ctx := r.Context()

	// Parse query parameters
	var params url.Values
	if r.Method == http.MethodGet {
		params = r.URL.Query()
	} else {
		if err := r.ParseForm(); err != nil {
			h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data")
			return
		}
		params = r.Form
	}

	// Extract required parameters
	clientID := params.Get("client_id")
	redirectURI := params.Get("redirect_uri")
	responseType := params.Get("response_type")
	state := params.Get("state")

	// Validate required parameters (before we have a valid redirect_uri, use JSON errors)
	if clientID == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}
	if redirectURI == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	// Validate client exists
	client, err := h.clients.Get(ctx, clientID)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if client == nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_client", "Client not found")
		return
	}

	// Validate redirect_uri matches client configuration
	if !h.isValidRedirectURI(redirectURI, client.RedirectURIs, client.RequireRedirectExact) {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri not registered for client")
		return
	}

	// Now we have a valid redirect_uri, use redirect errors for subsequent failures
	if responseType != "code" {
		h.redirectWithError(w, redirectURI, "unsupported_response_type", "Only 'code' response_type is supported", state)
		return
	}

	// Extract optional parameters
	scope := params.Get("scope")
	codeChallenge := params.Get("code_challenge")
	codeChallengeMethod := params.Get("code_challenge_method")
	loginHint := params.Get("login_hint")
	nonce := params.Get("nonce")

	// Validate PKCE (required for public clients in AT Protocol)
	if codeChallenge == "" {
		h.redirectWithError(w, redirectURI, "invalid_request", "code_challenge is required", state)
		return
	}
	if codeChallengeMethod != "S256" && codeChallengeMethod != "" {
		h.redirectWithError(w, redirectURI, "invalid_request", "Only S256 code_challenge_method is supported", state)
		return
	}
	if codeChallengeMethod == "" {
		codeChallengeMethod = "S256"
	}

	// login_hint is required for AT Protocol (DID or handle)
	if loginHint == "" {
		h.redirectWithError(w, redirectURI, "invalid_request", "login_hint (DID or handle) is required", state)
		return
	}

	// Resolve login_hint to DID if it's a handle
	did := loginHint
	if !strings.HasPrefix(loginHint, "did:") {
		resolvedDID, err := h.didResolver.ResolveHandleToDID(loginHint)
		if err != nil {
			h.redirectWithError(w, redirectURI, "invalid_request", "Failed to resolve handle: "+err.Error(), state)
			return
		}
		did = resolvedDID
	}

	// Resolve DID to get PDS endpoint and auth server metadata
	authServer, _, err := h.bridge.ResolveAuthServer(ctx, did)
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to resolve authorization server: "+err.Error(), state)
		return
	}

	// Generate session ID and timestamps
	sessionID, err := oauth.GenerateSessionID()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to generate session", state)
		return
	}
	now := oauth.CurrentTimestamp()
	expiresAt := oauth.ExpirationTimestamp(h.config.AuthorizationCodeExpiration)

	// Store client authorization request
	authReq := &oauth.AuthRequest{
		SessionID:           sessionID,
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               ptrString(scope),
		State:               ptrString(state),
		CodeChallenge:       ptrString(codeChallenge),
		CodeChallengeMethod: ptrString(codeChallengeMethod),
		ResponseType:        responseType,
		Nonce:               ptrString(nonce),
		LoginHint:           ptrString(loginHint),
		CreatedAt:           now,
		ExpiresAt:           expiresAt,
	}
	if err := h.authRequests.Insert(ctx, authReq); err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to store authorization request", state)
		return
	}

	// Generate ATP OAuth state
	atpOAuthState, err := oauth.GenerateState()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to generate state", state)
		return
	}

	// Generate DPoP key pair for upstream auth
	dpopKey, err := oauth.GenerateDPoPKeyPair()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to generate DPoP key", state)
		return
	}
	dpopKeyJSON, err := dpopKey.ToPrivateJSON()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to serialize DPoP key", state)
		return
	}

	signingJKT, err := dpopKey.CalculateJKT()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to compute DPoP key thumbprint", state)
		return
	}

	// Generate PKCE for ATP OAuth
	pkceVerifier, err := oauth.GenerateCodeVerifier()
	if err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to generate PKCE verifier", state)
		return
	}
	atpCodeChallenge := oauth.GenerateCodeChallenge(pkceVerifier)

	// Store ATP request for callback
	atpReq := &oauth.ATPRequest{
		OAuthState:          atpOAuthState,
		AuthorizationServer: authServer.Issuer,
		Nonce:               sessionID, // Use session ID as correlation
		PKCEVerifier:        pkceVerifier,
		SigningPublicKey:    signingJKT,
		DPoPPrivateKey:      dpopKeyJSON,
		CreatedAt:           now,
		ExpiresAt:           expiresAt,
	}
	if err := h.atpRequests.Insert(ctx, atpReq); err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to store ATP request", state)
		return
	}

	// Create ATP session
	atpSession := &oauth.ATPSession{
		SessionID:        sessionID,
		Iteration:        0,
		DID:              &did,
		SessionCreatedAt: now,
		ATPOAuthState:    atpOAuthState,
		SigningKeyJKT:    signingJKT,
		DPoPKey:          dpopKeyJSON,
	}
	if err := h.atpSessions.Insert(ctx, atpSession); err != nil {
		h.redirectWithError(w, redirectURI, "server_error", "Failed to store ATP session", state)
		return
	}

	// Build authorization URL for PDS
	// Use request scope, or fall back to client's configured scope
	atpScope := scope
	if atpScope == "" && client.Scope != nil {
		atpScope = *client.Scope
	}
	if atpScope == "" {
		atpScope = "atproto"
	}

	authParams := url.Values{
		"client_id":             {h.config.ClientID},
		"redirect_uri":          {h.config.CallbackURL},
		"response_type":         {"code"},
		"code_challenge":        {atpCodeChallenge},
		"code_challenge_method": {"S256"},
		"state":                 {atpOAuthState},
		"scope":                 {atpScope},
		"login_hint":            {did},
	}

	authURL := oauth.BuildAuthorizationURL(authServer.AuthorizationEndpoint, authParams)

	// Redirect to PDS authorization endpoint
	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles GET /oauth/callback.
// This receives the OAuth callback from the PDS authorization server.
func (h *OAuthHandlers) HandleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, []string{http.MethodGet})
		return
	}

	ctx := r.Context()

	// Extract callback parameters
	code := r.URL.Query().Get("code")
	atpState := r.URL.Query().Get("state")
	errorCode := r.URL.Query().Get("error")
	errorDescription := r.URL.Query().Get("error_description")

	// Look up ATP request by state
	atpReq, err := h.atpRequests.Get(ctx, atpState)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if atpReq == nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid or expired state")
		return
	}

	// Check if expired
	if atpReq.IsExpired() {
		_ = h.atpRequests.Delete(ctx, atpState)
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Authorization request expired")
		return
	}

	// Get the session ID from the nonce (we stored it there)
	sessionID := atpReq.Nonce

	// Look up client auth request
	authReq, err := h.authRequests.Get(ctx, sessionID)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if authReq == nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Session not found")
		return
	}

	clientState := ""
	if authReq.State != nil {
		clientState = *authReq.State
	}

	// Handle error response from PDS
	if errorCode != "" {
		h.redirectWithError(w, authReq.RedirectURI, errorCode, errorDescription, clientState)
		return
	}

	if code == "" {
		h.redirectWithError(w, authReq.RedirectURI, "invalid_request", "Missing authorization code", clientState)
		return
	}

	// Parse DPoP key
	dpopKey, err := oauth.ParseDPoPKeyPair(atpReq.DPoPPrivateKey)
	if err != nil {
		h.redirectWithError(w, authReq.RedirectURI, "server_error", "Failed to parse DPoP key", clientState)
		return
	}

	// Exchange code for tokens with PDS
	tokenResp, err := h.bridge.ExchangeCode(ctx, oauth.ExchangeCodeRequest{
		TokenEndpoint: atpReq.AuthorizationServer + "/oauth/token",
		Issuer:        atpReq.AuthorizationServer,
		Code:          code,
		CodeVerifier:  atpReq.PKCEVerifier,
		RedirectURI:   h.config.CallbackURL,
		DPoPKey:       dpopKey,
	})
	if err != nil {
		h.redirectWithError(w, authReq.RedirectURI, "server_error", "Token exchange failed: "+err.Error(), clientState)
		return
	}

	// Update ATP session with tokens
	now := oauth.CurrentTimestamp()
	expiresAt := now + tokenResp.ExpiresIn
	atpSession, err := h.atpSessions.GetLatest(ctx, sessionID)
	if err == nil && atpSession != nil {
		atpSession.DID = &tokenResp.Sub
		atpSession.AccessToken = &tokenResp.AccessToken
		atpSession.RefreshToken = &tokenResp.RefreshToken
		atpSession.AccessTokenCreatedAt = &now
		atpSession.AccessTokenExpiresAt = &expiresAt
		if tokenResp.Scope != nil {
			atpSession.AccessTokenScopes = tokenResp.Scope
		}
		atpSession.SessionExchangedAt = &now
		_ = h.atpSessions.Update(ctx, atpSession)
	}

	// Generate authorization code for client
	clientCode, err := oauth.GenerateAuthorizationCode()
	if err != nil {
		h.redirectWithError(w, authReq.RedirectURI, "server_error", "Failed to generate authorization code", clientState)
		return
	}

	codeExpiresAt := oauth.ExpirationTimestamp(h.config.AuthorizationCodeExpiration)
	authCode := &oauth.AuthorizationCode{
		Code:                clientCode,
		ClientID:            authReq.ClientID,
		UserID:              tokenResp.Sub,
		SessionID:           &sessionID,
		SessionIteration:    ptrInt64(0),
		RedirectURI:         authReq.RedirectURI,
		Scope:               authReq.Scope,
		CodeChallenge:       authReq.CodeChallenge,
		CodeChallengeMethod: authReq.CodeChallengeMethod,
		Nonce:               authReq.Nonce,
		CreatedAt:           now,
		ExpiresAt:           codeExpiresAt,
		Used:                false,
	}
	if err := h.authCodes.Insert(ctx, authCode); err != nil {
		h.redirectWithError(w, authReq.RedirectURI, "server_error", "Failed to store authorization code", clientState)
		return
	}

	// Clean up
	_ = h.atpRequests.Delete(ctx, atpState)
	_ = h.authRequests.Delete(ctx, sessionID)

	// Redirect back to client with code
	redirectURL, _ := url.Parse(authReq.RedirectURI)
	q := redirectURL.Query()
	q.Set("code", clientCode)
	if clientState != "" {
		q.Set("state", clientState)
	}
	redirectURL.RawQuery = q.Encode()

	http.Redirect(w, r, redirectURL.String(), http.StatusFound)
}

// HandleToken handles POST /oauth/token.
// This exchanges authorization codes or refresh tokens for access tokens.
func (h *OAuthHandlers) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, []string{http.MethodPost})
		return
	}

	if err := r.ParseForm(); err != nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data")
		return
	}

	grantType := r.Form.Get("grant_type")

	switch grantType {
	case "authorization_code":
		h.handleAuthorizationCodeGrant(w, r)
	case "refresh_token":
		h.handleRefreshTokenGrant(w, r)
	default:
		h.writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Unsupported grant_type: "+grantType)
	}
}

// handleAuthorizationCodeGrant handles the authorization_code grant type.
func (h *OAuthHandlers) handleAuthorizationCodeGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	codeStr := r.Form.Get("code")
	clientID := r.Form.Get("client_id")
	redirectURI := r.Form.Get("redirect_uri")
	codeVerifier := r.Form.Get("code_verifier")

	if codeStr == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code is required")
		return
	}
	if clientID == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}
	if redirectURI == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	// Get client
	client, err := h.clients.Get(ctx, clientID)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if client == nil {
		h.writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client not found")
		return
	}

	// Get authorization code
	authCode, err := h.authCodes.Get(ctx, codeStr)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if authCode == nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid authorization code")
		return
	}

	// Validate code
	if authCode.Used {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Authorization code already used")
		return
	}
	if authCode.IsExpired() {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Authorization code expired")
		return
	}
	if authCode.ClientID != clientID {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Client ID mismatch")
		return
	}
	if authCode.RedirectURI != redirectURI {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Redirect URI mismatch")
		return
	}

	// Verify PKCE
	if authCode.CodeChallenge != nil && *authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "code_verifier required for PKCE")
			return
		}
		method := "S256"
		if authCode.CodeChallengeMethod != nil {
			method = *authCode.CodeChallengeMethod
		}
		if !oauth.VerifyCodeChallenge(codeVerifier, *authCode.CodeChallenge, method) {
			h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid code verifier")
			return
		}
	}

	// Validate DPoP if present
	dpopJKT, err := h.validateDPoP(ctx, r, client)
	if err != nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_dpop_proof", err.Error())
		return
	}

	// Mark code as used
	if err := h.authCodes.MarkUsed(ctx, codeStr); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}

	// Generate tokens
	accessTokenStr, err := oauth.GenerateAccessToken()
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to generate access token")
		return
	}
	refreshTokenStr, err := oauth.GenerateRefreshToken()
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to generate refresh token")
		return
	}

	now := oauth.CurrentTimestamp()
	accessExpiresAt := now + h.config.AccessTokenExpiration

	// Determine token type
	tokenType := oauth.TokenBearer
	if dpopJKT != nil {
		tokenType = oauth.TokenDPoP
	}

	// Store access token
	accessToken := &oauth.AccessToken{
		Token:            accessTokenStr,
		TokenType:        tokenType,
		ClientID:         clientID,
		UserID:           &authCode.UserID,
		SessionID:        authCode.SessionID,
		SessionIteration: ptrInt64(0),
		Scope:            authCode.Scope,
		CreatedAt:        now,
		ExpiresAt:        accessExpiresAt,
		Revoked:          false,
		DPoPJKT:          dpopJKT,
	}
	if err := h.accessTokens.Insert(ctx, accessToken); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to store access token")
		return
	}

	// Store refresh token
	var refreshExpiresAt *int64
	if h.config.RefreshTokenExpiration > 0 {
		exp := now + h.config.RefreshTokenExpiration
		refreshExpiresAt = &exp
	}
	refreshToken := &oauth.RefreshToken{
		Token:            refreshTokenStr,
		AccessToken:      accessTokenStr,
		ClientID:         clientID,
		UserID:           authCode.UserID,
		SessionID:        authCode.SessionID,
		SessionIteration: ptrInt64(0),
		Scope:            authCode.Scope,
		CreatedAt:        now,
		ExpiresAt:        refreshExpiresAt,
		Revoked:          false,
	}
	if err := h.refreshTokens.Insert(ctx, refreshToken); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to store refresh token")
		return
	}

	// Build response
	tokenTypeStr := "Bearer"
	if dpopJKT != nil {
		tokenTypeStr = "DPoP"
	}

	resp := TokenResponse{
		AccessToken:  accessTokenStr,
		TokenType:    tokenTypeStr,
		ExpiresIn:    h.config.AccessTokenExpiration,
		RefreshToken: &refreshTokenStr,
		Scope:        authCode.Scope,
		Sub:          &authCode.UserID,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// handleRefreshTokenGrant handles the refresh_token grant type.
func (h *OAuthHandlers) handleRefreshTokenGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	refreshTokenStr := r.Form.Get("refresh_token")
	clientID := r.Form.Get("client_id")

	if refreshTokenStr == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "refresh_token is required")
		return
	}
	if clientID == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	// Get client
	client, err := h.clients.Get(ctx, clientID)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if client == nil {
		h.writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "Client not found")
		return
	}

	// Get refresh token
	oldRefreshToken, err := h.refreshTokens.Get(ctx, refreshTokenStr)
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if oldRefreshToken == nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Invalid refresh token")
		return
	}

	// Validate refresh token
	if oldRefreshToken.Revoked {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Refresh token revoked")
		return
	}
	if oldRefreshToken.IsExpired() {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Refresh token expired")
		return
	}
	if oldRefreshToken.ClientID != clientID {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_grant", "Client ID mismatch")
		return
	}

	// Validate DPoP if present
	dpopJKT, err := h.validateDPoP(ctx, r, client)
	if err != nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_dpop_proof", err.Error())
		return
	}

	// Revoke old refresh token
	if err := h.refreshTokens.Revoke(ctx, refreshTokenStr); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}

	// Generate new tokens
	newAccessTokenStr, err := oauth.GenerateAccessToken()
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to generate access token")
		return
	}
	newRefreshTokenStr, err := oauth.GenerateRefreshToken()
	if err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to generate refresh token")
		return
	}

	now := oauth.CurrentTimestamp()
	accessExpiresAt := now + h.config.AccessTokenExpiration

	tokenType := oauth.TokenBearer
	if dpopJKT != nil {
		tokenType = oauth.TokenDPoP
	}

	// Store new access token
	newAccessToken := &oauth.AccessToken{
		Token:            newAccessTokenStr,
		TokenType:        tokenType,
		ClientID:         clientID,
		UserID:           &oldRefreshToken.UserID,
		SessionID:        oldRefreshToken.SessionID,
		SessionIteration: ptrInt64(0),
		Scope:            oldRefreshToken.Scope,
		CreatedAt:        now,
		ExpiresAt:        accessExpiresAt,
		Revoked:          false,
		DPoPJKT:          dpopJKT,
	}
	if err := h.accessTokens.Insert(ctx, newAccessToken); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to store access token")
		return
	}

	// Store new refresh token
	var refreshExpiresAt *int64
	if h.config.RefreshTokenExpiration > 0 {
		exp := now + h.config.RefreshTokenExpiration
		refreshExpiresAt = &exp
	}
	newRefreshToken := &oauth.RefreshToken{
		Token:            newRefreshTokenStr,
		AccessToken:      newAccessTokenStr,
		ClientID:         clientID,
		UserID:           oldRefreshToken.UserID,
		SessionID:        oldRefreshToken.SessionID,
		SessionIteration: ptrInt64(0),
		Scope:            oldRefreshToken.Scope,
		CreatedAt:        now,
		ExpiresAt:        refreshExpiresAt,
		Revoked:          false,
	}
	if err := h.refreshTokens.Insert(ctx, newRefreshToken); err != nil {
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Failed to store refresh token")
		return
	}

	// Build response
	tokenTypeStr := "Bearer"
	if dpopJKT != nil {
		tokenTypeStr = "DPoP"
	}

	resp := TokenResponse{
		AccessToken:  newAccessTokenStr,
		TokenType:    tokenTypeStr,
		ExpiresIn:    h.config.AccessTokenExpiration,
		RefreshToken: &newRefreshTokenStr,
		Scope:        oldRefreshToken.Scope,
		Sub:          &oldRefreshToken.UserID,
	}

	h.writeJSON(w, http.StatusOK, resp)
}

// HandleJWKS handles GET /oauth/jwks.
// Returns the JSON Web Key Set for token verification.
func (h *OAuthHandlers) HandleJWKS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.writeMethodNotAllowed(w, []string{http.MethodGet})
		return
	}

	// Return empty JWKS if no signing key configured
	jwks := map[string]interface{}{
		"keys": []interface{}{},
	}

	if h.config.SigningKey != nil {
		jwk, err := h.config.SigningKey.ToJWK()
		if err != nil {
			slog.Warn("Failed to convert signing key to JWK for JWKS endpoint", "error", err)
			h.writeJSON(w, http.StatusOK, jwks)
			return
		}

		kid, err := h.config.SigningKey.CalculateJKT()
		if err != nil {
			slog.Warn("Failed to compute signing key thumbprint for JWKS endpoint", "error", err)
			h.writeJSON(w, http.StatusOK, jwks)
			return
		}

		jwks["keys"] = []interface{}{
			map[string]interface{}{
				"kty": jwk.Kty,
				"crv": jwk.Crv,
				"x":   jwk.X,
				"y":   jwk.Y,
				"use": "sig",
				"kid": kid,
			},
		}
	}

	h.writeJSON(w, http.StatusOK, jwks)
}

// HandleRevoke handles POST /oauth/revoke.
// Revokes an access token or refresh token.
func (h *OAuthHandlers) HandleRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeMethodNotAllowed(w, []string{http.MethodPost})
		return
	}

	ctx := r.Context()

	if err := r.ParseForm(); err != nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data")
		return
	}

	token := r.Form.Get("token")
	if token == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "token is required")
		return
	}

	// Try to revoke as access token
	_ = h.accessTokens.Revoke(ctx, token)

	// Try to revoke as refresh token
	_ = h.refreshTokens.Revoke(ctx, token)

	// Per RFC 7009, return 200 OK even if token not found
	w.WriteHeader(http.StatusOK)
}

// validateDPoP validates the DPoP proof if present.
// Returns the JKT if valid, nil if no DPoP, or error if invalid.
func (h *OAuthHandlers) validateDPoP(ctx context.Context, r *http.Request, client *oauth.Client) (*string, error) {
	dpopProof := r.Header.Get("DPoP")
	if dpopProof == "" {
		// DPoP not required for confidential clients
		if client.ClientType == oauth.ClientPublic {
			return nil, nil // Allow for now, but in strict mode should require DPoP
		}
		return nil, nil
	}

	// Build token endpoint URL
	tokenURL := h.config.ExternalBaseURL + "/oauth/token"

	// Verify DPoP proof
	result, err := oauth.VerifyDPoPProof(dpopProof, "POST", tokenURL, oauth.DefaultMaxDPoPAge)
	if err != nil {
		return nil, err
	}

	// Check JTI for replay
	used, err := h.dpopJTIs.Exists(ctx, result.JTI)
	if err != nil {
		return nil, err
	}
	if used {
		return nil, oauth.ErrDPoPReplay
	}

	// Record JTI
	jti := &oauth.DPoPJTI{
		JTI:       result.JTI,
		CreatedAt: result.IAT,
	}
	if err := h.dpopJTIs.Insert(ctx, jti); err != nil {
		return nil, err
	}

	return &result.JKT, nil
}

// isValidRedirectURI checks if the redirect URI is valid for the client.
func (h *OAuthHandlers) isValidRedirectURI(uri string, registeredURIs []string, requireExact bool) bool {
	for _, registered := range registeredURIs {
		if requireExact {
			if uri == registered {
				return true
			}
		} else {
			// Allow prefix match for non-exact mode
			if strings.HasPrefix(uri, registered) {
				return true
			}
		}
	}
	return false
}

// Helper methods

func (h *OAuthHandlers) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *OAuthHandlers) writeOAuthError(w http.ResponseWriter, status int, errorCode, description string) {
	resp := map[string]string{
		"error":             errorCode,
		"error_description": description,
	}
	h.writeJSON(w, status, resp)
}

func (h *OAuthHandlers) writeMethodNotAllowed(w http.ResponseWriter, allowed []string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	h.writeOAuthError(w, http.StatusMethodNotAllowed, "invalid_request", "Method not allowed")
}

func (h *OAuthHandlers) redirectWithError(w http.ResponseWriter, redirectURI, errorCode, description, state string) {
	u, err := url.Parse(redirectURI)
	if err != nil {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
		return
	}

	q := u.Query()
	q.Set("error", errorCode)
	q.Set("error_description", description)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()

	// Use a dummy request for redirect
	http.Redirect(w, &http.Request{}, u.String(), http.StatusFound)
}

// TokenResponse represents an OAuth token response.
type TokenResponse struct {
	AccessToken  string  `json:"access_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	RefreshToken *string `json:"refresh_token,omitempty"`
	Scope        *string `json:"scope,omitempty"`
	Sub          *string `json:"sub,omitempty"`
}

// RegisterRoutes registers all OAuth routes on the given mux.
func (h *OAuthHandlers) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/.well-known/oauth-authorization-server", h.HandleAuthorizationServerMetadata)
	mux.HandleFunc("/.well-known/oauth-protected-resource", h.HandleProtectedResourceMetadata)
	mux.HandleFunc("/oauth/authorize", h.HandleAuthorize)
	mux.HandleFunc("/oauth/token", h.HandleToken)
	mux.HandleFunc("/oauth/callback", h.HandleCallback)
	mux.HandleFunc("/oauth/jwks", h.HandleJWKS)
	mux.HandleFunc("/oauth/revoke", h.HandleRevoke)
}

// Helper functions

func ptrString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrInt64(i int64) *int64 {
	return &i
}

// StartCleanupWorker starts a background worker to clean up expired tokens.
func (h *OAuthHandlers) StartCleanupWorker(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				now := oauth.CurrentTimestamp()
				_ = h.authRequests.DeleteExpired(ctx, now)
				_ = h.atpRequests.DeleteExpired(ctx, now)
				_ = h.authCodes.DeleteExpired(ctx, now)
				_ = h.accessTokens.DeleteExpired(ctx, now)
				// Clean up JTIs older than 1 hour
				_ = h.dpopJTIs.DeleteOlderThan(ctx, now-3600)
			}
		}
	}()
}
