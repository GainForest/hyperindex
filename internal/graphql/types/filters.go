// Package types provides GraphQL type mapping and building utilities.
// This file defines shared filter InputObject types for field-level filtering,
// reused across all collection query types.
package types

import (
	"github.com/graphql-go/graphql"
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

// FilterInputForLexiconType maps a lexicon property type and format to the
// appropriate GraphQL filter InputObject. Returns nil for non-filterable types.
func FilterInputForLexiconType(lexiconType, format string) *graphql.InputObject {
	switch lexiconType {
	case "string":
		if format == "datetime" {
			return DateTimeFilterInput
		}
		return StringFilterInput
	case "integer":
		return IntFilterInput
	case "number":
		return FloatFilterInput
	case "boolean":
		return BooleanFilterInput
	default:
		// blob, bytes, unknown, ref, union, array, object, record — not filterable
		return nil
	}
}
