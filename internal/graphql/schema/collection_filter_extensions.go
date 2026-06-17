package schema

import (
	"fmt"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/types"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// collectionFilterExtension describes a hand-authored where-filter field for a
// specific collection. These extensions are deliberately separate from generated
// lexicon filters: they are exact-NSID allowlist entries and may use
// repository-level targets that are not expressible as same-record lexicon JSON
// paths.
type collectionFilterExtension struct {
	Collection  string
	Field       string
	Input       *graphql.InputObject
	Description string
	FieldType   string
	Target      repositories.FieldFilterTarget
}

var collectionFilterExtensions = []collectionFilterExtension{
	{
		Collection:  "org.hypercerts.claim.activity",
		Field:       "contributorDid",
		Input:       types.DIDFilterInput,
		Description: "Compatibility filter for activities whose contributors include this DID inline or through an org.hypercerts.claim.contributorInformation strongRef.",
		FieldType:   lexicon.TypeString,
		Target:      repositories.FieldFilterTargetContributorDID,
	},
	{
		Collection:  "app.certified.badge.award",
		Field:       "badgeType",
		Input:       types.StringFilterInput,
		Description: "Derived filter for awards whose referenced app.certified.badge.definition has this badgeType.",
		FieldType:   lexicon.TypeString,
		Target:      repositories.FieldFilterTargetBadgeAwardBadgeType,
	},
}

func addCollectionFilterExtensionFields(collection string, fields graphql.InputObjectConfigFieldMap) error {
	for _, extension := range collectionFilterExtensionsForCollection(collection) {
		if _, exists := fields[extension.Field]; exists {
			return fmt.Errorf("collection filter extension %s.%s conflicts with an existing where filter field; remove the extension or handle the lexicon-defined field explicitly", collection, extension.Field)
		}
		fields[extension.Field] = &graphql.InputObjectFieldConfig{
			Type:        extension.Input,
			Description: extension.Description,
		}
	}
	return nil
}

func collectionFilterExtensionsForCollection(collection string) []collectionFilterExtension {
	extensions := make([]collectionFilterExtension, 0)
	for _, extension := range collectionFilterExtensions {
		if extension.Collection == collection {
			extensions = append(extensions, extension)
		}
	}
	return extensions
}

func collectionFilterExtensionForField(collection, field string) (collectionFilterExtension, bool) {
	for _, extension := range collectionFilterExtensions {
		if extension.Collection == collection && extension.Field == field {
			return extension, true
		}
	}
	return collectionFilterExtension{}, false
}

func extractCollectionFilterExtensionFilters(extension collectionFilterExtension, filterMap map[string]interface{}) []repositories.FieldFilter {
	filters := make([]repositories.FieldFilter, 0, len(filterMap))
	for op, val := range filterMap {
		if val == nil {
			continue
		}
		filters = append(filters, repositories.FieldFilter{
			Field:     extension.Field,
			Operator:  op,
			Value:     val,
			FieldType: extension.FieldType,
			Target:    extension.Target,
		})
	}
	return filters
}
