package admin

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/logsafe"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// Handler handles admin GraphQL requests with authentication.
type Handler struct {
	schema      *graphql.Schema
	resolver    *Resolver
	middleware  *oauth.AuthMiddleware
	adminDIDs   []string
	adminAPIKey string
}

// NewHandler creates a new admin GraphQL handler.
// adminAPIKey gates when the X-User-DID header may be trusted for authentication.
func NewHandler(repos *Repositories, middleware *oauth.AuthMiddleware, domainDID, adminAPIKey string, adminDIDs []string) (*Handler, error) {
	resolver := NewResolver(repos, domainDID, adminDIDs)

	builder := NewSchemaBuilder(resolver)
	schema, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return &Handler{
		schema:      schema,
		resolver:    resolver,
		middleware:  middleware,
		adminDIDs:   adminDIDs,
		adminAPIKey: adminAPIKey,
	}, nil
}

func isValidAdminAPIKey(providedKey, expectedKey string) bool {
	providedKey = strings.TrimSpace(providedKey)
	expectedKey = strings.TrimSpace(expectedKey)

	if expectedKey == "" {
		return false
	}
	if providedKey == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(providedKey), []byte(expectedKey)) == 1
}

func logSafeMutationVariables(variables map[string]interface{}) map[string]interface{} {
	if variables == nil {
		return nil
	}

	redacted := make(map[string]interface{}, len(variables))
	for key, value := range variables {
		if isURLVariableName(key) {
			redacted[key] = logSafeURLVariableValue(value)
			continue
		}

		redacted[key] = value
	}

	return redacted
}

func isURLVariableName(name string) bool {
	lowerName := strings.ToLower(name)
	return lowerName == "url" || strings.HasSuffix(lowerName, "url") || strings.HasSuffix(lowerName, "urls")
}

func logSafeURLVariableValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case string:
		return logsafe.URL(typed)
	case []string:
		return logsafe.URLs(typed)
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for i, item := range typed {
			if rawURL, ok := item.(string); ok {
				redacted[i] = logsafe.URL(rawURL)
				continue
			}

			redacted[i] = item
		}
		return redacted
	default:
		return value
	}
}

// ServeHTTP handles admin GraphQL HTTP requests.
// CORS is handled by route-level middleware; not duplicated here.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse the request
	var params struct {
		Query         string                 `json:"query"`
		OperationName string                 `json:"operationName"`
		Variables     map[string]interface{} `json:"variables"`
	}

	if r.Method == "GET" {
		params.Query = r.URL.Query().Get("query")
		params.OperationName = r.URL.Query().Get("operationName")
	} else {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	// Log mutation requests
	if strings.Contains(params.Query, "mutation") {
		slog.Info("[admin] Mutation request", "operation", params.OperationName, "variables", logSafeMutationVariables(params.Variables))
	}

	// Get authentication info from context (set by middleware) or X-User-DID header
	ctx := r.Context()
	userDID := oauth.UserIDFromContext(ctx)

	// Only trust X-User-DID when the request presents a valid admin API key.
	if userDID == "" {
		proxiedUserDID := r.Header.Get("X-User-DID")
		switch {
		case proxiedUserDID == "":
		case isValidAdminAPIKey(r.Header.Get("X-Admin-API-Key"), h.adminAPIKey):
			userDID = proxiedUserDID
			slog.Warn("[admin] Auth via X-User-DID admin API key",
				"did", userDID,
				"remote_addr", r.RemoteAddr)
		default:
			slog.Warn("[admin] Ignoring X-User-DID without valid admin API key",
				"remote_addr", r.RemoteAddr)
		}
	}
	handle := "" // Would need to resolve from DID

	isAdmin := isAdminDID(userDID, h.adminDIDs)

	// Debug logging for auth
	if userDID != "" {
		slog.Info("[admin] Authenticated request", "userDID", userDID, "isAdmin", isAdmin)
	}

	// Inject auth info into context
	ctx = ContextWithAuth(ctx, userDID, handle, isAdmin, h.adminDIDs)

	// Execute the query
	result := graphql.Do(graphql.Params{
		Schema:         *h.schema,
		RequestString:  params.Query,
		OperationName:  params.OperationName,
		VariableValues: params.Variables,
		Context:        ctx,
	})

	// Write response
	w.Header().Set("Content-Type", "application/json")
	if len(result.Errors) > 0 {
		// Log errors for debugging
		for _, err := range result.Errors {
			slog.Debug("GraphQL error", "error", err.Message, "path", err.Path)
		}
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// Schema returns the underlying GraphQL schema.
func (h *Handler) Schema() *graphql.Schema {
	return h.schema
}

// Resolver returns the admin resolver.
func (h *Handler) Resolver() *Resolver {
	return h.resolver
}

// RequireAuth returns a middleware-wrapped handler that requires authentication.
func (h *Handler) RequireAuth() http.Handler {
	return h.middleware.RequireAuth(h)
}

// OptionalAuth returns a middleware-wrapped handler that allows optional authentication.
func (h *Handler) OptionalAuth() http.Handler {
	return h.middleware.OptionalAuth(h)
}
