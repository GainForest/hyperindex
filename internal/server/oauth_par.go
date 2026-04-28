// Package server contains HTTP handlers for the hyperindex server.
// OAuth Pushed Authorization Request (PAR) endpoint (RFC 9126).
package server

import (
	"encoding/json"
	"net/http"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// PARResponse is the PAR response.
type PARResponse struct {
	RequestURI string `json:"request_uri"`
	ExpiresIn  int64  `json:"expires_in"`
}

// OAuthPARHandler handles Pushed Authorization Requests.
type OAuthPARHandler struct {
	db          database.Executor
	clients     *repositories.OAuthClientsRepository
	parRequests *repositories.OAuthPARRequestsRepository
}

// NewOAuthPARHandler creates a new PAR handler.
func NewOAuthPARHandler(db database.Executor) *OAuthPARHandler {
	return &OAuthPARHandler{
		db:          db,
		clients:     repositories.NewOAuthClientsRepository(db),
		parRequests: repositories.NewOAuthPARRequestsRepository(db),
	}
}

// HandlePAR handles POST /oauth/par.
func (h *OAuthPARHandler) HandlePAR(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writePARError(w, http.StatusMethodNotAllowed, "invalid_request", "Method not allowed")
		return
	}

	ctx := r.Context()

	// Parse form data
	if err := r.ParseForm(); err != nil {
		writePARError(w, http.StatusBadRequest, "invalid_request", "Failed to parse form data")
		return
	}

	// Extract client_id (required)
	clientID := r.Form.Get("client_id")
	if clientID == "" {
		writePARError(w, http.StatusBadRequest, "invalid_request", "client_id is required")
		return
	}

	// Get client from storage
	client, err := h.clients.Get(ctx, clientID)
	if err != nil {
		writePARError(w, http.StatusInternalServerError, "server_error", "Database error")
		return
	}
	if client == nil {
		writePARError(w, http.StatusUnauthorized, "invalid_client", "Client not found")
		return
	}

	// Extract required parameters
	responseType := r.Form.Get("response_type")
	if responseType == "" {
		writePARError(w, http.StatusBadRequest, "invalid_request", "response_type is required")
		return
	}

	redirectURI := r.Form.Get("redirect_uri")
	if redirectURI == "" {
		writePARError(w, http.StatusBadRequest, "invalid_request", "redirect_uri is required")
		return
	}

	// Build authorization request JSON
	authRequest := map[string]interface{}{
		"response_type": responseType,
		"client_id":     clientID,
		"redirect_uri":  redirectURI,
	}

	// Optional parameters
	if scope := r.Form.Get("scope"); scope != "" {
		authRequest["scope"] = scope
	}
	if state := r.Form.Get("state"); state != "" {
		authRequest["state"] = state
	}
	if codeChallenge := r.Form.Get("code_challenge"); codeChallenge != "" {
		authRequest["code_challenge"] = codeChallenge
	}
	if codeChallengeMethod := r.Form.Get("code_challenge_method"); codeChallengeMethod != "" {
		authRequest["code_challenge_method"] = codeChallengeMethod
	}
	if loginHint := r.Form.Get("login_hint"); loginHint != "" {
		authRequest["login_hint"] = loginHint
	}
	if nonce := r.Form.Get("nonce"); nonce != "" {
		authRequest["nonce"] = nonce
	}

	authRequestJSON, err := json.Marshal(authRequest)
	if err != nil {
		writePARError(w, http.StatusInternalServerError, "server_error", "Failed to serialize request")
		return
	}

	// Generate PAR request URI
	requestURI := generatePARRequestURI()
	now := oauth.CurrentTimestamp()
	expiresIn := int64(60) // 60 seconds
	expiresAt := now + expiresIn

	par := &oauth.PARRequest{
		RequestURI:           requestURI,
		AuthorizationRequest: string(authRequestJSON),
		ClientID:             clientID,
		CreatedAt:            now,
		ExpiresAt:            expiresAt,
		Metadata:             "{}",
	}

	// Store PAR request
	if err := h.parRequests.Insert(ctx, par); err != nil {
		writePARError(w, http.StatusInternalServerError, "server_error", "Failed to store PAR request")
		return
	}

	// Build response
	resp := PARResponse{
		RequestURI: requestURI,
		ExpiresIn:  expiresIn,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}

func generatePARRequestURI() string {
	return "urn:ietf:params:oauth:request_uri:" + randomString(32)
}

func writePARError(w http.ResponseWriter, status int, errorCode, description string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":             errorCode,
		"error_description": description,
	})
}
