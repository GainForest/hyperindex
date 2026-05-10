// Package server contains HTTP handlers for the hyperindex server.
// OAuth client metadata endpoint for AT Protocol authentication.
package server

import (
	"encoding/json"
	"net/http"
)

// ClientMetadataResponse is the OAuth client metadata response.
// This describes this server as an OAuth client to AT Protocol PDS servers.
type ClientMetadataResponse struct {
	ClientID                    string      `json:"client_id"`
	ClientName                  string      `json:"client_name"`
	ClientURI                   *string     `json:"client_uri,omitempty"`
	LogoURI                     *string     `json:"logo_uri,omitempty"`
	TOSUri                      *string     `json:"tos_uri,omitempty"`
	PolicyURI                   *string     `json:"policy_uri,omitempty"`
	RedirectURIs                []string    `json:"redirect_uris"`
	GrantTypes                  []string    `json:"grant_types"`
	ResponseTypes               []string    `json:"response_types"`
	Scope                       string      `json:"scope"`
	TokenEndpointAuthMethod     string      `json:"token_endpoint_auth_method"`
	TokenEndpointAuthSigningAlg string      `json:"token_endpoint_auth_signing_alg"`
	SubjectType                 string      `json:"subject_type"`
	ApplicationType             string      `json:"application_type"`
	DPoPBoundAccessTokens       bool        `json:"dpop_bound_access_tokens"`
	JWKS                        interface{} `json:"jwks,omitempty"`
	JWKSUri                     *string     `json:"jwks_uri,omitempty"`
}

// ClientMetadataConfig contains configuration for the client metadata endpoint.
type ClientMetadataConfig struct {
	// ExternalBaseURL is the public-facing URL of this server.
	ExternalBaseURL string
	// ClientName is the display name for this OAuth client.
	ClientName string
	// ClientURI is the homepage URL for this client (optional).
	ClientURI string
	// LogoURI is the URL to a logo image (optional).
	LogoURI string
	// TOSUri is the URL to the terms of service (optional).
	TOSUri string
	// PolicyURI is the URL to the privacy policy (optional).
	PolicyURI string
	// Scope is the default scope requested by this client.
	Scope string
	// JWKS is the inline JSON Web Key Set (optional, mutually exclusive with JWKSUri).
	JWKS interface{}
	// JWKSUri is the URL to the JWKS endpoint (optional).
	JWKSUri string
}

// HandleClientMetadata handles GET /oauth-client-metadata.json.
// This endpoint serves the OAuth client metadata for this server, which PDS servers
// fetch to understand how to authenticate this AppView as an OAuth client.
func HandleClientMetadata(cfg ClientMetadataConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Client ID is the URL to this metadata document
		clientID := cfg.ExternalBaseURL + "/oauth-client-metadata.json"

		// Default scope if not configured
		scope := cfg.Scope
		if scope == "" {
			scope = "atproto"
		}

		// Build redirect URIs - callback endpoint
		redirectURIs := []string{
			cfg.ExternalBaseURL + "/oauth/callback",
		}

		metadata := ClientMetadataResponse{
			ClientID:                    clientID,
			ClientName:                  cfg.ClientName,
			RedirectURIs:                redirectURIs,
			GrantTypes:                  []string{"authorization_code", "refresh_token"},
			ResponseTypes:               []string{"code"},
			Scope:                       scope,
			TokenEndpointAuthMethod:     "private_key_jwt",
			TokenEndpointAuthSigningAlg: "ES256",
			SubjectType:                 "public",
			ApplicationType:             "web",
			DPoPBoundAccessTokens:       true,
		}

		// Optional fields
		if cfg.ClientURI != "" {
			metadata.ClientURI = &cfg.ClientURI
		}
		if cfg.LogoURI != "" {
			metadata.LogoURI = &cfg.LogoURI
		}
		if cfg.TOSUri != "" {
			metadata.TOSUri = &cfg.TOSUri
		}
		if cfg.PolicyURI != "" {
			metadata.PolicyURI = &cfg.PolicyURI
		}

		// JWKS or JWKS URI (mutually exclusive)
		switch {
		case cfg.JWKS != nil:
			metadata.JWKS = cfg.JWKS
		case cfg.JWKSUri != "":
			metadata.JWKSUri = &cfg.JWKSUri
		default:
			// Default to our JWKS endpoint
			jwksURI := cfg.ExternalBaseURL + "/oauth/jwks"
			metadata.JWKSUri = &jwksURI
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
		_ = json.NewEncoder(w).Encode(metadata)
	}
}
