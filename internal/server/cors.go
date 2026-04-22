// Package server provides HTTP handlers and middleware for the Hypergoat server.
package server

import (
	"net/http"
	"strings"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	// AllowedOrigins is a list of origins that are allowed to make cross-origin requests.
	// If empty, defaults to "*" (all origins allowed — suitable for development only).
	AllowedOrigins []string

	// AllowedHeaders is the list of request headers allowed in CORS requests.
	// "Content-Type" and "Authorization" are always included.
	AllowedHeaders []string

	// AllowAdminAPIKeyAuth controls whether X-User-DID is included in allowed headers.
	AllowAdminAPIKeyAuth bool
}

// CORSMiddleware returns an HTTP middleware that handles CORS headers and preflight requests.
// It uses the configured allowed origins instead of hardcoding "*".
func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	// Build allowed origins set for O(1) lookup
	allowedSet := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		allowedSet[strings.TrimSpace(origin)] = true
	}
	allowAll := len(cfg.AllowedOrigins) == 0

	// Build allowed headers
	headers := []string{"Content-Type", "Authorization", "DPoP"}
	headers = append(headers, cfg.AllowedHeaders...)
	if cfg.AllowAdminAPIKeyAuth {
		headers = append(headers, "X-User-DID")
	}
	allowedHeaders := strings.Join(headers, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && allowedSet[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
