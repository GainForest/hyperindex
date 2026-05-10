// Package types provides GraphQL type mapping and building utilities.
package types //nolint:revive // package name is descriptive within graphql context

import (
	"fmt"

	"github.com/graphql-go/graphql"
	"github.com/graphql-go/graphql/language/ast"

	"github.com/GainForest/hyperindex/internal/lexicon"
)

// JSONScalar is a package-level JSON scalar for use across the schema.
// It's defined once to avoid duplicate type errors.
var JSONScalar = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "JSON",
	Description: "Arbitrary JSON value",
	Serialize: func(value interface{}) interface{} {
		return value
	},
	ParseValue: func(value interface{}) interface{} {
		return value
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		if v, ok := valueAST.(*ast.StringValue); ok {
			return v.Value
		}
		return nil
	},
})

// DateTimeScalar is a package-level DateTime scalar for use across the schema.
var DateTimeScalar = graphql.NewScalar(graphql.ScalarConfig{
	Name:        "DateTime",
	Description: "ISO 8601 datetime string",
	Serialize: func(value interface{}) interface{} {
		return value
	},
	ParseValue: func(value interface{}) interface{} {
		return value
	},
	ParseLiteral: func(valueAST ast.Value) interface{} {
		if v, ok := valueAST.(*ast.StringValue); ok {
			return v.Value
		}
		return nil
	},
})

// Mapper maps lexicon types to GraphQL types.
type Mapper struct {
	// Built-in object types
	BlobType *graphql.Object

	// Cache for built object types (keyed by fully-qualified ref)
	objectTypes map[string]*graphql.Object

	// Cache for built union types
	unionTypes map[string]*graphql.Union
}

// NewMapper creates a new type mapper with initialized scalars.
func NewMapper() *Mapper {
	m := &Mapper{
		objectTypes: make(map[string]*graphql.Object),
		unionTypes:  make(map[string]*graphql.Union),
	}
	m.initBlobType()
	return m
}

// initBlobType initializes the Blob object type.
func (m *Mapper) initBlobType() {
	m.BlobType = graphql.NewObject(graphql.ObjectConfig{
		Name:        "Blob",
		Description: "AT Protocol blob reference",
		Fields: graphql.Fields{
			"ref": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "CID reference to the blob",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					blob, ok := p.Source.(map[string]any)
					if !ok {
						return nil, fmt.Errorf("blob.ref expected source to be a map[string]any, got %T", p.Source)
					}

					ref, ok := extractCIDLinkString(blob["ref"])
					if !ok {
						return nil, fmt.Errorf("blob.ref expected ref to be a CID string or {\"$link\": string} object")
					}

					return ref, nil
				},
			},
			"mimeType": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.String),
				Description: "MIME type of the blob",
			},
			"size": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Int),
				Description: "Size in bytes",
			},
		},
	})
}

// extractCIDLinkString returns the plain CID string from values encoded as either
// an already-normalized CID string or an AT Protocol CID link object.
func extractCIDLinkString(value any) (string, bool) {
	switch typedValue := value.(type) {
	case string:
		return typedValue, true
	case map[string]any:
		if len(typedValue) != 1 {
			return "", false
		}

		link, ok := typedValue["$link"].(string)
		return link, ok
	case map[string]string:
		if len(typedValue) != 1 {
			return "", false
		}

		link, ok := typedValue["$link"]
		return link, ok
	default:
		return "", false
	}
}

// MapPrimitiveType maps a lexicon primitive type to a GraphQL type.
// This handles basic types without refs.
func (m *Mapper) MapPrimitiveType(lexiconType, format string) graphql.Output {
	switch lexiconType {
	case lexicon.TypeString:
		// Check format hints
		switch format {
		case lexicon.FormatDatetime:
			return DateTimeScalar
		default:
			return graphql.String
		}
	case lexicon.TypeInteger:
		return graphql.Int
	case lexicon.TypeBoolean:
		return graphql.Boolean
	case "number":
		return graphql.Float
	case lexicon.TypeBlob:
		return m.BlobType
	case lexicon.TypeBytes:
		return graphql.String // Base64 encoded
	case lexicon.TypeCIDLink:
		return graphql.String
	case lexicon.TypeUnknown:
		return JSONScalar
	default:
		return graphql.String
	}
}

// GetObjectType returns a cached object type by ref.
func (m *Mapper) GetObjectType(ref string) (*graphql.Object, bool) {
	t, ok := m.objectTypes[ref]
	return t, ok
}

// SetObjectType caches an object type by ref.
func (m *Mapper) SetObjectType(ref string, t *graphql.Object) {
	m.objectTypes[ref] = t
}

// GetUnionType returns a cached union type by name.
func (m *Mapper) GetUnionType(name string) (*graphql.Union, bool) {
	t, ok := m.unionTypes[name]
	return t, ok
}

// SetUnionType caches a union type by name.
func (m *Mapper) SetUnionType(name string, t *graphql.Union) {
	m.unionTypes[name] = t
}

// AllObjectTypes returns all cached object types.
func (m *Mapper) AllObjectTypes() map[string]*graphql.Object {
	return m.objectTypes
}
