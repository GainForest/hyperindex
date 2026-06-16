// Package types provides GraphQL type mapping and building utilities.
// This file defines shared filter InputObject types for field-level filtering,
// reused across all collection query types.
package types //nolint:revive // package name is descriptive within graphql context

import (
	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

// StringFilterInput is a GraphQL InputObject for filtering string fields.
var StringFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "StringFilterInput",
	Description: "Filter conditions for string fields",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Equal to",
		},
		"neq": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Not equal to",
		},
		"in": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
			Description: "Value is in list",
		},
		"contains": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Contains substring",
		},
		"startsWith": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Starts with prefix",
		},
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Field is null",
		},
	},
})

// IntFilterInput is a GraphQL InputObject for filtering integer fields.
var IntFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "IntFilterInput",
	Description: "Filter conditions for integer fields",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Equal to",
		},
		"neq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Not equal to",
		},
		"gt": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Greater than",
		},
		"lt": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Less than",
		},
		"gte": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Greater than or equal to",
		},
		"lte": &graphql.InputObjectFieldConfig{
			Type:        graphql.Int,
			Description: "Less than or equal to",
		},
		"in": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.Int)),
			Description: "Value is in list",
		},
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Field is null",
		},
	},
})

// FloatFilterInput is a GraphQL InputObject for filtering float/number fields.
var FloatFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "FloatFilterInput",
	Description: "Filter conditions for float fields",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Equal to",
		},
		"neq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Not equal to",
		},
		"gt": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Greater than",
		},
		"lt": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Less than",
		},
		"gte": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Greater than or equal to",
		},
		"lte": &graphql.InputObjectFieldConfig{
			Type:        graphql.Float,
			Description: "Less than or equal to",
		},
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Field is null",
		},
	},
})

// BooleanFilterInput is a GraphQL InputObject for filtering boolean fields.
var BooleanFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "BooleanFilterInput",
	Description: "Filter conditions for boolean fields",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Equal to",
		},
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Field is null",
		},
	},
})

// DateTimeFilterInput is a GraphQL InputObject for filtering datetime fields.
var DateTimeFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "DateTimeFilterInput",
	Description: "Filter conditions for datetime fields",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Equal to",
		},
		"neq": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Not equal to",
		},
		"gt": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Greater than",
		},
		"lt": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Less than",
		},
		"gte": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Greater than or equal to",
		},
		"lte": &graphql.InputObjectFieldConfig{
			Type:        DateTimeScalar,
			Description: "Less than or equal to",
		},
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "Field is null",
		},
	},
})

// DIDFilterInput is a restricted GraphQL InputObject for filtering DID fields.
// DID is a column-level filter (not a JSON field), so only eq and in are meaningful.
// Operators like contains or startsWith are not supported for DIDs.
var DIDFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "DIDFilterInput",
	Description: "Filter conditions for DID fields (column-level). Only eq and in are supported.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Equals",
		},
		"in": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
			Description: "In list",
		},
	},
})

// URIFilterInput is a restricted GraphQL InputObject for filtering AT-URI
// metadata fields. URI filters are column-level filters, so exact matching and
// batched lookup are supported while substring matching is intentionally not.
var URIFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "URIFilterInput",
	Description: "Filter conditions for AT-URI metadata fields. Only eq and in are supported.",
	Fields: graphql.InputObjectConfigFieldMap{
		"eq": &graphql.InputObjectFieldConfig{
			Type:        graphql.String,
			Description: "Equals",
		},
		"in": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewList(graphql.NewNonNull(graphql.String)),
			Description: "In list",
		},
	},
})

// PresenceFilterInput is a GraphQL InputObject for checking whether a top-level
// JSON field is missing/null or present. Use it for complex lexicon properties
// where Hyperindex exposes presence checks but not nested value filtering.
var PresenceFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
	Name:        "PresenceFilterInput",
	Description: "Filter conditions for checking whether a top-level JSON field is missing/null or present.",
	Fields: graphql.InputObjectConfigFieldMap{
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.NewNonNull(graphql.Boolean),
			Description: "True matches missing or null fields; false matches present and non-null fields.",
		},
	},
})

// FilterInputForLexiconType maps scalar lexicon property types and formats to
// their GraphQL filter InputObject. It returns nil for complex types; use
// FilterInputForLexiconProperty when building collection where inputs that
// should expose presence-only filters for complex top-level properties.
func FilterInputForLexiconType(lexiconType, format string) *graphql.InputObject {
	switch lexiconType {
	case lexicon.TypeString:
		if format == lexicon.FormatDatetime {
			return DateTimeFilterInput
		}
		return StringFilterInput
	case lexicon.TypeInteger:
		return IntFilterInput
	case "number":
		return FloatFilterInput
	case lexicon.TypeBoolean:
		return BooleanFilterInput
	default:
		return nil
	}
}

// FilterInputForLexiconProperty maps a top-level lexicon property type and
// format to the GraphQL filter InputObject used by generated collection where
// inputs. Scalar fields keep their typed filter inputs; complex fields get
// PresenceFilterInput so clients can check whether the field is missing/null or
// present without nested JSON filtering.
func FilterInputForLexiconProperty(lexiconType, format string) *graphql.InputObject {
	if input := FilterInputForLexiconType(lexiconType, format); input != nil {
		return input
	}

	switch lexiconType {
	case lexicon.TypeArray,
		lexicon.TypeRef,
		lexicon.TypeUnion,
		lexicon.TypeObject,
		lexicon.TypeBlob,
		lexicon.TypeBytes,
		lexicon.TypeUnknown,
		lexicon.TypeCIDLink:
		return PresenceFilterInput
	default:
		return nil
	}
}
