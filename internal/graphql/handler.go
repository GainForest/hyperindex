// Package graphql provides GraphQL schema building and HTTP handling.
package graphql

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/gqlerrors"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

const (
	publicSchemaUnavailableCode    = "SCHEMA_UNAVAILABLE"
	publicSchemaUnavailableMessage = "public GraphQL schema is unavailable: no schema has loaded successfully. Fix lexicon load errors in the backend logs, then run the admin reloadSchema mutation or restart after fixing configuration."
)

// Handler handles GraphQL requests.
type Handler struct {
	schemaProvider SchemaProvider
	repos          *resolver.Repositories
}

type staticSchemaProvider struct {
	schema *graphql.Schema
}

func (p staticSchemaProvider) Schema() *graphql.Schema {
	return p.schema
}

// NewHandler creates a new GraphQL handler from a lexicon registry and repositories.
func NewHandler(registry *lexicon.Registry, repos *resolver.Repositories) (*Handler, error) {
	builder := schema.NewBuilder(registry)
	s, err := builder.Build()
	if err != nil {
		return nil, err
	}

	return NewHandlerWithSchemaProvider(staticSchemaProvider{schema: s}, repos)
}

// NewHandlerWithSchemaProvider creates a GraphQL handler that resolves the
// current public schema for each request. Use this with PublicSchemaManager when
// the live schema can be reloaded without restarting the process.
func NewHandlerWithSchemaProvider(provider SchemaProvider, repos *resolver.Repositories) (*Handler, error) {
	if provider == nil {
		return nil, fmt.Errorf("create public GraphQL handler: schema provider is nil; pass a PublicSchemaManager or static schema provider")
	}

	return &Handler{schemaProvider: provider, repos: repos}, nil
}

// ServeHTTP handles GraphQL HTTP requests.
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
		// Variables from query string would need to be parsed from JSON
	} else {
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}

	currentSchema := h.Schema()
	if currentSchema == nil {
		writeSchemaUnavailable(w)
		return
	}

	// Inject repositories into context
	ctx := resolver.WithRepositories(r.Context(), h.repos)

	// Execute the query
	result := graphql.Do(graphql.Params{
		Schema:         *currentSchema,
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

// Schema returns the current GraphQL schema from the handler's provider. It
// returns nil when no schema has loaded successfully yet.
func (h *Handler) Schema() *graphql.Schema {
	if h == nil || h.schemaProvider == nil {
		return nil
	}
	return h.schemaProvider.Schema()
}

func writeSchemaUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(&graphql.Result{
		Errors: []gqlerrors.FormattedError{{
			Message: publicSchemaUnavailableMessage,
			Extensions: map[string]interface{}{
				"code":       publicSchemaUnavailableCode,
				"httpStatus": http.StatusServiceUnavailable,
			},
		}},
	})
}
