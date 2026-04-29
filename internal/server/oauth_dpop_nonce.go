// Package server contains HTTP handlers for the hyperindex server.
// OAuth DPoP nonce endpoint.
package server

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

// HandleDPoPNonce handles GET/POST /oauth/dpop/nonce.
// Returns a fresh DPoP nonce for use in DPoP proofs.
func HandleDPoPNonce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate a fresh nonce
	nonce := generateDPoPNonce()

	// Return nonce in both header and body
	w.Header().Set("DPoP-Nonce", nonce)
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(nonce))
}

// generateDPoPNonce generates a cryptographically secure nonce for DPoP.
func generateDPoPNonce() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
