// Package server contains HTTP handlers for the hyperindex server.
// OAuth client registration endpoint (RFC 7591).
package server

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// RegistrationRequest is the OAuth client registration request.
type RegistrationRequest struct {
	ClientName              string   `json:"client_name,omitempty"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	ApplicationType         string   `json:"application_type,omitempty"`
	Contacts                []string `json:"contacts,omitempty"`
	ClientURI               string   `json:"client_uri,omitempty"`
	LogoURI                 string   `json:"logo_uri,omitempty"`
	JWKS                    any      `json:"jwks,omitempty"`
	JWKSUri                 string   `json:"jwks_uri,omitempty"`
	DPoPBoundAccessTokens   bool     `json:"dpop_bound_access_tokens,omitempty"`
}

// RegistrationResponse is the OAuth client registration response.
type RegistrationResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            *string  `json:"client_secret,omitempty"`
	ClientName              string   `json:"client_name"`
	RedirectURIs            []string `json:"redirect_uris"`
	GrantTypes              []string `json:"grant_types"`
	ResponseTypes           []string `json:"response_types"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method"`
	Scope                   string   `json:"scope"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at"`
	ClientType              string   `json:"client_type"`
	ApplicationType         string   `json:"application_type,omitempty"`
}

// OAuthRegisterHandler handles client registration.
type OAuthRegisterHandler struct {
	db      database.Executor
	clients *repositories.OAuthClientsRepository
}

// NewOAuthRegisterHandler creates a new registration handler.
func NewOAuthRegisterHandler(db database.Executor) *OAuthRegisterHandler {
	return &OAuthRegisterHandler{
		db:      db,
		clients: repositories.NewOAuthClientsRepository(db),
	}
}

// HandleRegister handles POST /oauth/register.
func (h *OAuthRegisterHandler) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeRegisterError(w, http.StatusMethodNotAllowed, "invalid_request", "Method not allowed")
		return
	}

	ctx := r.Context()

	// Parse JSON request
	var req RegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeRegisterError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON: "+err.Error())
		return
	}

	// Validate redirect_uris (required)
	if len(req.RedirectURIs) == 0 {
		writeRegisterError(w, http.StatusBadRequest, "invalid_request", "redirect_uris cannot be empty")
		return
	}

	// Validate each redirect URI
	for _, uri := range req.RedirectURIs {
		if err := validateRedirectURI(uri); err != nil {
			writeRegisterError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
	}

	// Parse grant_types or use defaults
	grantTypes := parseGrantTypes(req.GrantTypes)
	if len(grantTypes) == 0 {
		grantTypes = []oauth.GrantType{oauth.GrantAuthorizationCode, oauth.GrantRefreshToken}
	}

	// Parse response_types or use defaults
	responseTypes := parseResponseTypes(req.ResponseTypes)
	if len(responseTypes) == 0 {
		responseTypes = []oauth.ResponseType{oauth.ResponseCode}
	}

	// Parse token_endpoint_auth_method or use default
	authMethod := parseAuthMethod(req.TokenEndpointAuthMethod)
	if req.TokenEndpointAuthMethod == "" {
		authMethod = oauth.AuthNone // Public client by default for AT Protocol
	}

	// Determine client type based on auth method
	clientType := oauth.ClientConfidential
	if authMethod == oauth.AuthNone {
		clientType = oauth.ClientPublic
	}

	// Generate client credentials
	clientID := generateClientID()
	var clientSecret *string
	if clientType == oauth.ClientConfidential {
		secret := generateClientSecret()
		clientSecret = &secret
	}

	// Default client name
	clientName := req.ClientName
	if clientName == "" {
		clientName = "OAuth Client"
	}

	// Default scope
	scope := req.Scope
	if scope == "" {
		scope = "atproto"
	}

	now := oauth.CurrentTimestamp()

	// Create client record
	client := &oauth.Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              clientName,
		ClientType:              clientType,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		Scope:                   &scope,
		TokenEndpointAuthMethod: authMethod,
		CreatedAt:               now,
		UpdatedAt:               now,
		AccessTokenExpiration:   3600,    // 1 hour
		RefreshTokenExpiration:  2592000, // 30 days
		RequireRedirectExact:    true,
	}

	// Store client
	if err := h.clients.Insert(ctx, client); err != nil {
		writeRegisterError(w, http.StatusInternalServerError, "server_error", "Failed to store client")
		return
	}

	// Build response - convert types back to strings
	grantTypeStrings := make([]string, len(grantTypes))
	for i, gt := range grantTypes {
		grantTypeStrings[i] = string(gt)
	}
	responseTypeStrings := make([]string, len(responseTypes))
	for i, rt := range responseTypes {
		responseTypeStrings[i] = string(rt)
	}

	resp := RegistrationResponse{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              clientName,
		RedirectURIs:            req.RedirectURIs,
		GrantTypes:              grantTypeStrings,
		ResponseTypes:           responseTypeStrings,
		TokenEndpointAuthMethod: string(authMethod),
		Scope:                   scope,
		ClientIDIssuedAt:        now,
		ClientType:              string(clientType),
		ApplicationType:         req.ApplicationType,
	}
	if resp.ApplicationType == "" {
		resp.ApplicationType = "web"
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

// validateRedirectURI validates a redirect URI per OAuth 2.0 rules.
func validateRedirectURI(uri string) error {
	parsed, err := url.Parse(uri)
	if err != nil {
		return &RegistrationError{Message: "Invalid URI: " + uri}
	}

	// Must be absolute
	if !parsed.IsAbs() {
		return &RegistrationError{Message: "redirect_uri must be absolute: " + uri}
	}

	// Must be https, unless localhost
	if parsed.Scheme != "https" {
		host := strings.ToLower(parsed.Hostname())
		if host != "localhost" && host != "127.0.0.1" && host != "[::1]" {
			return &RegistrationError{Message: "redirect_uri must use https (except for localhost): " + uri}
		}
	}

	// No fragment allowed
	if parsed.Fragment != "" {
		return &RegistrationError{Message: "redirect_uri must not contain fragment: " + uri}
	}

	return nil
}

// RegistrationError is an error during registration.
type RegistrationError struct {
	Message string
}

func (e *RegistrationError) Error() string {
	return e.Message
}

func generateClientID() string {
	return "client_" + randomString(16)
}

func generateClientSecret() string {
	return randomString(32)
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func writeRegisterError(w http.ResponseWriter, status int, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}

func parseGrantTypes(types []string) []oauth.GrantType {
	result := make([]oauth.GrantType, 0, len(types))
	for _, t := range types {
		switch t {
		case "authorization_code":
			result = append(result, oauth.GrantAuthorizationCode)
		case "refresh_token":
			result = append(result, oauth.GrantRefreshToken)
		case "client_credentials":
			result = append(result, oauth.GrantClientCredentials)
		}
	}
	return result
}

func parseResponseTypes(types []string) []oauth.ResponseType {
	result := make([]oauth.ResponseType, 0, len(types))
	for _, t := range types {
		switch t {
		case "code":
			result = append(result, oauth.ResponseCode)
		case "token":
			result = append(result, oauth.ResponseToken)
		}
	}
	return result
}

func parseAuthMethod(method string) oauth.AuthMethod {
	switch method {
	case "client_secret_basic":
		return oauth.AuthClientSecret
	case "client_secret_post":
		return oauth.AuthClientPost
	case "private_key_jwt":
		return oauth.AuthPrivateKeyJWT
	case "none":
		return oauth.AuthNone
	default:
		return oauth.AuthNone
	}
}
