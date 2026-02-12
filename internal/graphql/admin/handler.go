package admin

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/oauth"
)

// Handler handles admin GraphQL requests with authentication.
type Handler struct {
	schema      *graphql.Schema
	resolver    *Resolver
	middleware  *oauth.AuthMiddleware
	configRepo  *repositories.ConfigRepository
	adminAPIKey string // shared secret; when set, X-User-DID is trusted if Bearer token matches
}

// NewHandler creates a new admin GraphQL handler.
// When adminAPIKey is non-empty, the X-User-DID header is trusted only if the
// request also carries a matching Authorization: Bearer <key> header.
func NewHandler(repos *Repositories, middleware *oauth.AuthMiddleware, configRepo *repositories.ConfigRepository, domainDID string, adminAPIKey string) (*Handler, error) {
	resolver := NewResolver(repos, domainDID)

	builder := NewSchemaBuilder(resolver)
	schema, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return &Handler{
		schema:      schema,
		resolver:    resolver,
		middleware:  middleware,
		configRepo:  configRepo,
		adminAPIKey: adminAPIKey,
	}, nil
}

// ServeHTTP handles admin GraphQL HTTP requests.
// CORS is handled by the router-level middleware; not duplicated here.
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
		slog.Info("[admin] Mutation request", "operation", params.OperationName, "variables", params.Variables)
	}

	// Get authentication info from context (set by middleware) or X-User-DID header
	ctx := r.Context()
	userDID := oauth.UserIDFromContext(ctx)

	// Trust X-User-DID header only when the request carries a valid admin API key.
	// This allows frontends and CLI tools to authenticate as a specific user
	// without requiring the full OAuth flow.
	if userDID == "" && h.adminAPIKey != "" {
		if h.validAPIKey(r) {
			userDID = r.Header.Get("X-User-DID")
			if userDID != "" {
				slog.Info("[admin] Auth via X-User-DID + API key",
					"did", userDID,
					"remote_addr", r.RemoteAddr)
			}
		} else if r.Header.Get("X-User-DID") != "" {
			slog.Warn("[admin] X-User-DID header rejected: missing or invalid API key",
				"remote_addr", r.RemoteAddr)
		}
	}
	handle := "" // Would need to resolve from DID

	// Get admin DIDs from config
	adminDidsStr, err := h.configRepo.Get(ctx, "admin_dids")
	if err != nil {
		slog.Warn("Failed to get admin DIDs", "error", err)
		adminDidsStr = ""
	}

	var adminDIDs []string
	if adminDidsStr != "" {
		adminDIDs = strings.Split(adminDidsStr, ",")
		for i := range adminDIDs {
			adminDIDs[i] = strings.TrimSpace(adminDIDs[i])
		}
	}

	// Check if user is admin
	isAdmin := false
	for _, adminDID := range adminDIDs {
		if adminDID == userDID {
			isAdmin = true
			break
		}
	}

	// Debug logging for auth
	if userDID != "" {
		slog.Info("[admin] Authenticated request", "userDID", userDID, "isAdmin", isAdmin)
	}

	// Inject auth info into context
	ctx = ContextWithAuth(ctx, userDID, handle, isAdmin, adminDIDs)

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

// validAPIKey checks whether the request carries a valid admin API key.
// Returns true if no API key is configured (backwards-compatible) or if the
// request's Authorization: Bearer token matches the configured key.
func (h *Handler) validAPIKey(r *http.Request) bool {
	if h.adminAPIKey == "" {
		return true // no key configured — allow (backwards-compatible)
	}

	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return subtle.ConstantTimeCompare([]byte(token), []byte(h.adminAPIKey)) == 1
}
