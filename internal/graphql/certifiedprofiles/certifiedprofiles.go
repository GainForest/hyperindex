// Package certifiedprofiles contains GraphQL helpers for exposing Certified profile
// records as virtual metadata on all record nodes.
package certifiedprofiles

import "github.com/graphql-go/graphql"

const (
	// CollectionID is the ATProto collection used for Certified account profiles.
	CollectionID = "app.certified.actor.profile"

	// RKey is the well-known record key for Certified account profile records.
	RKey = "self"

	// SourceKey is the internal source-map key used to attach a Certified profile
	// record node to arbitrary record nodes before GraphQL resolves the public
	// certifiedProfileData field.
	SourceKey = "__certifiedProfileData"
)

// Field creates the virtual certifiedProfileData field injected into generated
// record types and GenericRecord. The resolver reads profile data pre-attached
// by parent resolvers so record lists can hydrate profiles in one batch.
func Field(profileType *graphql.Object) *graphql.Field {
	return &graphql.Field{
		Type:        profileType,
		Description: "Certified profile record for this record's author, including profile external labels when selected.",
		Resolve: func(p graphql.ResolveParams) (interface{}, error) {
			return ResolveFromSource(p.Source), nil
		},
	}
}

// ResolveFromSource returns the pre-attached Certified profile node for a record.
func ResolveFromSource(source interface{}) interface{} {
	sourceMap, ok := source.(map[string]interface{})
	if !ok {
		return nil
	}
	return sourceMap[SourceKey]
}
