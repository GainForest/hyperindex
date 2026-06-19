// Package graphql provides GraphQL schema building and HTTP handling.
package graphql

import (
	"encoding/json"
	"net/http"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// Handler handles GraphQL requests.
type Handler struct {
	schema *graphql.Schema
	repos  *resolver.Repositories
}

// NewHandler creates a new GraphQL handler from a lexicon registry and repositories.
func NewHandler(registry *lexicon.Registry, repos *resolver.Repositories) (*Handler, error) {
	builder := schema.NewBuilder(registry)
	s, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return &Handler{schema: s, repos: repos}, nil
}

// ServeHTTP handles GraphQL HTTP requests.
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
		// Variables from query string would need to be parsed from JSON
	} else {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	// Inject repositories into context
	ctx := resolver.WithRepositories(r.Context(), h.repos)

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
		w.WriteHeader(http.StatusBadRequest)
	}
	_ = json.NewEncoder(w).Encode(result)
}

// Schema returns the underlying GraphQL schema.
func (h *Handler) Schema() *graphql.Schema {
	return h.schema
}
