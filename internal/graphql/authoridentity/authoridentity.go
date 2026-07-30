// Package authoridentity exposes persisted AT Protocol actor identity metadata
// through generated GraphQL record types.
package authoridentity

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/resolver"
)

const (
	// SourceKey stores pre-hydrated author identity metadata on GraphQL source maps.
	SourceKey = "__authorIdentity"

	// DIDDeprecationReason directs consumers to the structured author field while
	// preserving existing DID fields during the API transition.
	DIDDeprecationReason = "Use author.did instead."
)

// Type is the shared GraphQL representation of an AT Protocol record author.
var Type = graphql.NewObject(graphql.ObjectConfig{
	Name:        "ActorIdentity",
	Description: "AT Protocol identity metadata for a record author.",
	Fields: graphql.Fields{
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Decentralized identifier of the record author.",
		},
		"handle": &graphql.Field{
			Type:        graphql.String,
			Description: "Current verified AT Protocol handle, or null when unavailable or invalid.",
		},
	},
})

// Field creates the non-null author field shared by record GraphQL types. List
// resolvers attach batched identity data under SourceKey; the fallback lookup
// supports single records and subscription events without changing their source
// shape.
func Field() *graphql.Field {
	return &graphql.Field{
		Type:        graphql.NewNonNull(Type),
		Description: "Identity metadata for this record's author.",
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			source, ok := p.Source.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("author expected record source to be a map, got %T", p.Source)
			}
			if attached, exists := source[SourceKey]; exists {
				return attached, nil
			}

			did, _ := source["did"].(string)
			if did == "" {
				return nil, fmt.Errorf("author could not resolve because the record source has no DID")
			}

			repos := resolver.GetRepositories(p.Context)
			if repos == nil || repos.Actors == nil {
				return NewSource(did, ""), nil
			}
			actor, err := repos.Actors.GetByDID(p.Context, did)
			if errors.Is(err, sql.ErrNoRows) {
				return NewSource(did, ""), nil
			}
			if err != nil {
				return nil, fmt.Errorf("resolve author identity for %s: %w", did, err)
			}
			return NewSource(did, actor.Handle), nil
		},
	}
}

// NewSource builds the GraphQL source map for an author. Tap's handle.invalid
// sentinel is persisted so reconciliation does not repeatedly retry it, but is
// intentionally exposed to API consumers as null.
func NewSource(did, handle string) map[string]interface{} {
	return map[string]interface{}{
		"did":    did,
		"handle": PublicHandle(handle),
	}
}

// PublicHandle returns a handle suitable for GraphQL output.
func PublicHandle(handle string) interface{} {
	handle = strings.TrimSpace(handle)
	if handle == "" || syntax.Handle(handle).IsInvalidHandle() {
		return nil
	}
	return handle
}
