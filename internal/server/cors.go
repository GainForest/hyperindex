// Package server provides HTTP handlers and middleware for the Hyperindex server.
package server

import (
	"net/http"
	"strings"
)

// CORSConfig holds CORS middleware configuration for one route group.
type CORSConfig struct {
	// AllowedOrigins is the list of browser origins that may make cross-origin requests.
	// Use []string{"*"} to allow every origin. An empty list sends no CORS origin
	// headers, which keeps browser access same-origin unless another middleware adds them.
	AllowedOrigins []string

	// AllowedHeaders is the list of additional request headers allowed in CORS requests.
	// "Content-Type", "Authorization", and "DPoP" are always included.
	AllowedHeaders []string

	// AllowAdminAPIKeyAuth includes admin proxy headers in preflight responses.
	// Enable this only for admin routes that intentionally accept X-Admin-API-Key
	// and X-User-DID from browser clients.
	AllowAdminAPIKeyAuth bool
}

// CORSMiddleware returns an HTTP middleware that handles CORS headers and
// preflight requests for a single route group.
func CORSMiddleware(cfg CORSConfig) func(http.Handler) http.Handler {
	allowedSet, allowAll := normalizeAllowedOrigins(cfg.AllowedOrigins)
	allowedHeaders := strings.Join(allowedRequestHeaders(cfg), ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if allowAll {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				addVaryHeader(w.Header(), "Origin")
				if _, ok := allowedSet[origin]; ok {
					w.Header().Set("Access-Control-Allow-Origin", origin)
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", allowedHeaders)
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func normalizeAllowedOrigins(origins []string) (map[string]struct{}, bool) {
	allowedSet := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		trimmed := strings.TrimSpace(origin)
		if trimmed == "" {
			continue
		}
		if trimmed == "*" {
			return nil, true
		}
		allowedSet[trimmed] = struct{}{}
	}
	return allowedSet, false
}

func allowedRequestHeaders(cfg CORSConfig) []string {
	headers := []string{"Content-Type", "Authorization", "DPoP"}
	headers = append(headers, cfg.AllowedHeaders...)
	if cfg.AllowAdminAPIKeyAuth {
		headers = append(headers, "X-Admin-API-Key", "X-User-DID")
	}
	return uniqueHeaderNames(headers)
}

func uniqueHeaderNames(headers []string) []string {
	unique := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		trimmed := strings.TrimSpace(header)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, trimmed)
	}
	return unique
}

func addVaryHeader(header http.Header, value string) {
	current := header.Values("Vary")
	for _, entry := range current {
		for _, part := range strings.Split(entry, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	header.Add("Vary", value)
}
