// Package oauth provides AT Protocol OAuth implementation.
// AT Protocol OAuth bridge for authenticating with PDS authorization servers.
package oauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// AuthorizationServerMetadata contains OAuth metadata from an AT Protocol PDS.
type AuthorizationServerMetadata struct {
	Issuer                             string   `json:"issuer"`
	AuthorizationEndpoint              string   `json:"authorization_endpoint"`
	TokenEndpoint                      string   `json:"token_endpoint"`
	PushedAuthorizationRequestEndpoint *string  `json:"pushed_authorization_request_endpoint,omitempty"`
	DPoPSigningAlgValuesSupported      []string `json:"dpop_signing_alg_values_supported,omitempty"`
	ScopesSupported                    []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported             []string `json:"response_types_supported,omitempty"`
	GrantTypesSupported                []string `json:"grant_types_supported,omitempty"`
	CodeChallengeMethodsSupported      []string `json:"code_challenge_methods_supported,omitempty"`
	TokenEndpointAuthMethodsSupported  []string `json:"token_endpoint_auth_methods_supported,omitempty"`
	RevocationEndpoint                 *string  `json:"revocation_endpoint,omitempty"`
	IntrospectionEndpoint              *string  `json:"introspection_endpoint,omitempty"`
}

// ProtectedResourceMetadata contains metadata for a protected resource.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported,omitempty"`
	BearerMethodsSupported []string `json:"bearer_methods_supported,omitempty"`
}

// TokenResponse contains the response from a token exchange.
type TokenResponse struct {
	AccessToken  string  `json:"access_token"`
	TokenType    string  `json:"token_type"`
	ExpiresIn    int64   `json:"expires_in"`
	RefreshToken string  `json:"refresh_token"`
	Scope        *string `json:"scope,omitempty"`
	Sub          string  `json:"sub"`
}

// PARResponse contains the response from a Pushed Authorization Request.
type PARResponse struct {
	RequestURI string `json:"request_uri"`
	ExpiresIn  int64  `json:"expires_in"`
}

// BridgeError represents an error from the ATP OAuth bridge.
type BridgeError struct {
	Type    string
	Message string
	Cause   error
}

func (e *BridgeError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
}

func (e *BridgeError) Unwrap() error {
	return e.Cause
}

// Bridge error types.
const (
	ErrTypeDIDResolution   = "did_resolution_error"
	ErrTypePDSNotFound     = "pds_not_found"
	ErrTypeTokenExchange   = "token_exchange_error"
	ErrTypeHTTP            = "http_error"
	ErrTypeInvalidResponse = "invalid_response"
	ErrTypeStorage         = "storage_error"
	ErrTypeMetadataFetch   = "metadata_fetch_error"
	ErrTypePAR             = "par_error"
)

// Bridge provides AT Protocol OAuth bridge functionality.
type Bridge struct {
	httpClient  *http.Client
	didResolver *DIDResolver
	clientID    string
	signingKey  *DPoPKeyPair // For client_assertion
	callbackURL string
}

// BridgeConfig contains configuration for the bridge.
type BridgeConfig struct {
	HTTPClient  *http.Client
	DIDResolver *DIDResolver
	ClientID    string
	SigningKey  *DPoPKeyPair
	CallbackURL string
}

// NewBridge creates a new AT Protocol OAuth bridge.
func NewBridge(cfg BridgeConfig) *Bridge {
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &Bridge{
		httpClient:  client,
		didResolver: cfg.DIDResolver,
		clientID:    cfg.ClientID,
		signingKey:  cfg.SigningKey,
		callbackURL: cfg.CallbackURL,
	}
}

// FetchProtectedResourceMetadata fetches the OAuth protected resource metadata from a PDS.
func (b *Bridge) FetchProtectedResourceMetadata(ctx context.Context, pdsEndpoint string) (*ProtectedResourceMetadata, error) {
	metadataURL := strings.TrimSuffix(pdsEndpoint, "/") + "/.well-known/oauth-protected-resource"

	req, err := http.NewRequestWithContext(ctx, "GET", metadataURL, http.NoBody)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to create request", Cause: err}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to fetch protected resource metadata", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &BridgeError{
			Type:    ErrTypeMetadataFetch,
			Message: fmt.Sprintf("metadata request failed with status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var metadata ProtectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, &BridgeError{Type: ErrTypeInvalidResponse, Message: "failed to parse protected resource metadata", Cause: err}
	}

	return &metadata, nil
}

// FetchAuthorizationServerMetadata fetches the OAuth authorization server metadata.
func (b *Bridge) FetchAuthorizationServerMetadata(ctx context.Context, authServerEndpoint string) (*AuthorizationServerMetadata, error) {
	metadataURL := strings.TrimSuffix(authServerEndpoint, "/") + "/.well-known/oauth-authorization-server"

	req, err := http.NewRequestWithContext(ctx, "GET", metadataURL, http.NoBody)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to create request", Cause: err}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to fetch authorization server metadata", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &BridgeError{
			Type:    ErrTypeMetadataFetch,
			Message: fmt.Sprintf("metadata request failed with status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var metadata AuthorizationServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, &BridgeError{Type: ErrTypeInvalidResponse, Message: "failed to parse authorization server metadata", Cause: err}
	}

	return &metadata, nil
}

// ResolveAuthServer resolves the authorization server for a given DID.
// It fetches the DID document, extracts the PDS endpoint, and then fetches
// the protected resource metadata to find the authorization server.
func (b *Bridge) ResolveAuthServer(ctx context.Context, did string) (*AuthorizationServerMetadata, string, error) {
	if b.didResolver == nil {
		return nil, "", &BridgeError{Type: ErrTypeDIDResolution, Message: "DID resolver not configured"}
	}

	// Resolve the DID (note: ResolveDID doesn't take context currently)
	didDoc, err := b.didResolver.ResolveDID(did)
	if err != nil {
		return nil, "", &BridgeError{Type: ErrTypeDIDResolution, Message: "failed to resolve DID", Cause: err}
	}

	// Extract PDS endpoint
	pdsEndpoint := didDoc.GetPDSEndpoint()
	if pdsEndpoint == "" {
		return nil, "", &BridgeError{Type: ErrTypePDSNotFound, Message: "no PDS endpoint in DID document"}
	}

	// Fetch protected resource metadata
	prm, err := b.FetchProtectedResourceMetadata(ctx, pdsEndpoint)
	if err != nil {
		return nil, "", err
	}

	if len(prm.AuthorizationServers) == 0 {
		return nil, "", &BridgeError{Type: ErrTypeMetadataFetch, Message: "no authorization servers in protected resource metadata"}
	}

	// Fetch authorization server metadata
	authServerEndpoint := prm.AuthorizationServers[0]
	asm, err := b.FetchAuthorizationServerMetadata(ctx, authServerEndpoint)
	if err != nil {
		return nil, "", err
	}

	return asm, pdsEndpoint, nil
}

// ExchangeCodeRequest contains the parameters for exchanging an authorization code.
type ExchangeCodeRequest struct {
	TokenEndpoint string
	Issuer        string
	Code          string
	CodeVerifier  string
	RedirectURI   string
	DPoPKey       *DPoPKeyPair
}

// ExchangeCode exchanges an authorization code for tokens.
func (b *Bridge) ExchangeCode(ctx context.Context, req ExchangeCodeRequest) (*TokenResponse, error) {
	body := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {req.Code},
		"redirect_uri":  {req.RedirectURI},
		"client_id":     {b.clientID},
		"code_verifier": {req.CodeVerifier},
	}

	return b.fetchTokens(ctx, req.TokenEndpoint, body, req.DPoPKey, req.Issuer, nil)
}

// RefreshTokensRequest contains the parameters for refreshing tokens.
type RefreshTokensRequest struct {
	TokenEndpoint string
	Issuer        string
	RefreshToken  string
	DPoPKey       *DPoPKeyPair
}

// RefreshTokens refreshes an access token using a refresh token.
func (b *Bridge) RefreshTokens(ctx context.Context, req RefreshTokensRequest) (*TokenResponse, error) {
	body := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {req.RefreshToken},
		"client_id":     {b.clientID},
	}

	return b.fetchTokens(ctx, req.TokenEndpoint, body, req.DPoPKey, req.Issuer, nil)
}

// fetchTokens performs the token request with DPoP and optional nonce retry.
func (b *Bridge) fetchTokens(ctx context.Context, tokenURL string, body url.Values, dpopKey *DPoPKeyPair, issuer string, nonce *string) (*TokenResponse, error) {
	// Add client_assertion if signing key is configured
	if b.signingKey != nil {
		assertion, err := b.createClientAssertion(issuer)
		if err == nil {
			body.Set("client_assertion", assertion)
			body.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
		}
	}

	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(body.Encode()))
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to create request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	// Generate DPoP proof
	if dpopKey != nil {
		nonceStr := ""
		if nonce != nil {
			nonceStr = *nonce
		}
		dpopProof, err := dpopKey.GenerateDPoPProof("POST", tokenURL, "", nonceStr)
		if err != nil {
			return nil, &BridgeError{Type: ErrTypeTokenExchange, Message: "failed to generate DPoP proof", Cause: err}
		}
		httpReq.Header.Set("DPoP", dpopProof)
	}

	// Send request
	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to send token request", Cause: err}
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to read response", Cause: err}
	}

	if resp.StatusCode == http.StatusOK {
		var tokenResp TokenResponse
		if err := json.Unmarshal(respBody, &tokenResp); err != nil {
			return nil, &BridgeError{Type: ErrTypeInvalidResponse, Message: "failed to parse token response", Cause: err}
		}
		return &tokenResp, nil
	}

	// Check for DPoP nonce requirement (400 with use_dpop_nonce error)
	if resp.StatusCode == http.StatusBadRequest && nonce == nil {
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Error == "use_dpop_nonce" {
			// Get nonce from header and retry
			dpopNonce := resp.Header.Get("DPoP-Nonce")
			if dpopNonce == "" {
				return nil, &BridgeError{Type: ErrTypeTokenExchange, Message: "server requires DPoP nonce but didn't provide one"}
			}
			return b.fetchTokens(ctx, tokenURL, body, dpopKey, issuer, &dpopNonce)
		}
	}

	return nil, &BridgeError{
		Type:    ErrTypeTokenExchange,
		Message: fmt.Sprintf("token request failed with status %d: %s", resp.StatusCode, string(respBody)),
	}
}

// createClientAssertion creates a JWT client assertion for authentication.
func (b *Bridge) createClientAssertion(audience string) (string, error) {
	if b.signingKey == nil || b.signingKey.PrivateKey == nil {
		return "", errors.New("no signing key configured")
	}

	// Create a simple JWT assertion
	// In production, you'd want a proper JWT library with all RFC claims
	now := time.Now()
	jti, err := generateJTI()
	if err != nil {
		return "", err
	}

	header := map[string]interface{}{
		"alg": "ES256",
		"typ": "JWT",
	}

	kid, err := b.signingKey.CalculateJKT()
	if err != nil {
		return "", fmt.Errorf("failed to calculate signing key thumbprint: %w", err)
	}
	header["kid"] = kid

	claims := map[string]interface{}{
		"iss": b.clientID,
		"sub": b.clientID,
		"aud": audience,
		"jti": jti,
		"iat": now.Unix(),
		"exp": now.Add(5 * time.Minute).Unix(),
	}

	// This is a simplified implementation
	// For production, use github.com/golang-jwt/jwt/v5 properly
	return signJWT(header, claims, b.signingKey.PrivateKey)
}

// PushAuthorizationRequest sends a Pushed Authorization Request to the server.
func (b *Bridge) PushAuthorizationRequest(ctx context.Context, parEndpoint string, params url.Values, dpopKey *DPoPKeyPair) (*PARResponse, error) {
	// Create request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", parEndpoint, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to create request", Cause: err}
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	httpReq.Header.Set("Accept", "application/json")

	// Generate DPoP proof if key provided
	if dpopKey != nil {
		dpopProof, err := dpopKey.GenerateDPoPProof("POST", parEndpoint, "", "")
		if err != nil {
			return nil, &BridgeError{Type: ErrTypePAR, Message: "failed to generate DPoP proof", Cause: err}
		}
		httpReq.Header.Set("DPoP", dpopProof)
	}

	// Send request
	resp, err := b.httpClient.Do(httpReq)
	if err != nil {
		return nil, &BridgeError{Type: ErrTypeHTTP, Message: "failed to send PAR request", Cause: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &BridgeError{
			Type:    ErrTypePAR,
			Message: fmt.Sprintf("PAR request failed with status %d: %s", resp.StatusCode, string(body)),
		}
	}

	var parResp PARResponse
	if err := json.NewDecoder(resp.Body).Decode(&parResp); err != nil {
		return nil, &BridgeError{Type: ErrTypeInvalidResponse, Message: "failed to parse PAR response", Cause: err}
	}

	return &parResp, nil
}

// BuildAuthorizationURL builds the authorization URL for the OAuth flow.
func BuildAuthorizationURL(authEndpoint string, params url.Values) string {
	if strings.Contains(authEndpoint, "?") {
		return authEndpoint + "&" + params.Encode()
	}
	return authEndpoint + "?" + params.Encode()
}

// signJWT is a helper to sign a JWT with an ECDSA key.
// This is a simplified implementation - use jwt library in production.
func signJWT(header, claims map[string]interface{}, key interface{}) (string, error) {
	ecdsaKey, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return "", errors.New("invalid key type")
	}

	headerJSON, _ := json.Marshal(header)
	claimsJSON, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsJSON)

	signingInput := headerB64 + "." + claimsB64
	hash := sha256.Sum256([]byte(signingInput))

	r, s, err := ecdsa.Sign(rand.Reader, ecdsaKey, hash[:])
	if err != nil {
		return "", err
	}

	// Convert to fixed-size format for ES256
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	sig := make([]byte, 64)
	copy(sig[32-len(rBytes):32], rBytes)
	copy(sig[64-len(sBytes):64], sBytes)

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return headerB64 + "." + claimsB64 + "." + sigB64, nil
}
