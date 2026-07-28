package authoridentity_test

import (
	"context"
	"testing"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/authoridentity"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestPublicHandle(t *testing.T) {
	tests := []struct {
		name   string
		handle string
		want   interface{}
	}{
		{name: "verified handle", handle: "alice.example", want: "alice.example"},
		{name: "empty handle", handle: "", want: nil},
		{name: "whitespace handle", handle: "   ", want: nil},
		{name: "invalid sentinel", handle: "handle.invalid", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authoridentity.PublicHandle(tt.handle); got != tt.want {
				t.Fatalf("PublicHandle(%q) = %#v, want %#v", tt.handle, got, tt.want)
			}
		})
	}
}

func TestFieldResolvesAttachedAndPersistedIdentity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := resolver.WithRepositories(context.Background(), &resolver.Repositories{Actors: db.Actors})
	if err := db.Actors.UpsertIdentity(ctx, "did:plc:persisted", "persisted.example"); err != nil {
		t.Fatalf("UpsertIdentity() error = %v", err)
	}

	field := authoridentity.Field()
	tests := []struct {
		name   string
		source map[string]interface{}
		want   map[string]interface{}
	}{
		{
			name: "attached identity",
			source: map[string]interface{}{
				"did":                    "did:plc:attached",
				authoridentity.SourceKey: authoridentity.NewSource("did:plc:attached", "attached.example"),
			},
			want: authoridentity.NewSource("did:plc:attached", "attached.example"),
		},
		{
			name:   "repository fallback",
			source: map[string]interface{}{"did": "did:plc:persisted"},
			want:   authoridentity.NewSource("did:plc:persisted", "persisted.example"),
		},
		{
			name:   "missing actor still exposes did",
			source: map[string]interface{}{"did": "did:plc:missing"},
			want:   authoridentity.NewSource("did:plc:missing", ""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := field.Resolve(graphql.ResolveParams{Context: ctx, Source: tt.source})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			gotMap, ok := got.(map[string]interface{})
			if !ok {
				t.Fatalf("Resolve() returned %T, want map[string]interface{}", got)
			}
			if gotMap["did"] != tt.want["did"] || gotMap["handle"] != tt.want["handle"] {
				t.Fatalf("Resolve() = %#v, want %#v", gotMap, tt.want)
			}
		})
	}
}
