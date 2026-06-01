package types //nolint:revive // package name is descriptive within graphql context

import (
	"fmt"
	"log/slog"
	"reflect"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/graphql/certifiedprofiles"
	"github.com/GainForest/hyperindex/internal/graphql/externallabels"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// ReservedRecordFields are field names injected as metadata and must not be overwritten by lexicon properties.
var ReservedRecordFields = map[string]bool{
	"uri":                  true,
	"cid":                  true,
	"did":                  true,
	"rkey":                 true,
	"externalLabels":       true,
	"certifiedProfileData": true,
}

// ObjectBuilder builds GraphQL object types from lexicon definitions.
type ObjectBuilder struct {
	mapper   *Mapper
	registry *lexicon.Registry
}

// NewObjectBuilder creates a new object builder.
func NewObjectBuilder(mapper *Mapper, registry *lexicon.Registry) *ObjectBuilder {
	return &ObjectBuilder{
		mapper:   mapper,
		registry: registry,
	}
}

// BuildObjectType builds a GraphQL object type from an ObjectDef.
// The ref is the fully-qualified reference (e.g., "org.hypercerts.defs#uri").
func (b *ObjectBuilder) BuildObjectType(ref string, def *lexicon.ObjectDef) *graphql.Object {
	// Check cache first
	if t, ok := b.mapper.GetObjectType(ref); ok {
		return t
	}

	// Generate GraphQL type name from ref
	typeName := refToTypeName(ref)

	// Create the object type with a thunk to handle circular references
	obj := graphql.NewObject(graphql.ObjectConfig{
		Name:        typeName,
		Description: fmt.Sprintf("Object type for %s", ref),
		Fields: graphql.FieldsThunk(func() graphql.Fields {
			return b.buildFields(ref, def)
		}),
	})

	// Cache before building fields (for circular refs)
	b.mapper.SetObjectType(ref, obj)

	return obj
}

// BuildRecordType builds a GraphQL object type from a RecordDef.
// The lexiconID is the NSID (e.g., "org.hypercerts.claim.activity").
func (b *ObjectBuilder) BuildRecordType(lexiconID string, def *lexicon.RecordDef) *graphql.Object {
	// Check cache first
	if t, ok := b.mapper.GetObjectType(lexiconID); ok {
		return t
	}

	typeName := lexicon.ToTypeName(lexiconID)

	obj := graphql.NewObject(graphql.ObjectConfig{
		Name:        typeName,
		Description: fmt.Sprintf("Record type for %s", lexiconID),
		Fields: graphql.FieldsThunk(func() graphql.Fields {
			return b.buildRecordFields(lexiconID, def)
		}),
	})

	b.mapper.SetObjectType(lexiconID, obj)

	return obj
}

// buildFields builds GraphQL fields from ObjectDef properties.
func (b *ObjectBuilder) buildFields(contextRef string, def *lexicon.ObjectDef) graphql.Fields {
	fields := graphql.Fields{}

	// Extract lexicon ID from context ref for resolving local refs
	contextLexiconID := lexicon.IDFromRef(contextRef)

	for _, entry := range def.Properties {
		field := b.buildField(contextLexiconID, entry.Name, &entry.Property, def.IsRequired(entry.Name))
		if field != nil {
			fields[entry.Name] = field
		}
	}

	if len(fields) == 0 {
		fields["empty"] = &graphql.Field{
			Type:        graphql.Boolean,
			Description: "Placeholder field for empty object definitions",
		}
	}

	return fields
}

// buildRecordFields builds GraphQL fields from RecordDef properties.
func (b *ObjectBuilder) buildRecordFields(lexiconID string, def *lexicon.RecordDef) graphql.Fields {
	fields := graphql.Fields{
		// Standard record fields
		"uri": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "AT-URI of this record",
		},
		"cid": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "CID of this record version",
		},
		"did": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "DID of the record author",
		},
		"rkey": &graphql.Field{
			Type:        graphql.NewNonNull(graphql.String),
			Description: "Record key (last segment of AT-URI)",
		},
		"externalLabels": externallabels.Field(),
	}
	if profileType, ok := b.mapper.GetObjectType(certifiedprofiles.CollectionID); ok {
		fields["certifiedProfileData"] = certifiedprofiles.Field(profileType)
	}

	// Build required set for quick lookup
	requiredSet := make(map[string]bool)
	for _, prop := range def.Properties {
		if prop.Property.Required {
			requiredSet[prop.Name] = true
		}
	}

	for _, entry := range def.Properties {
		if ReservedRecordFields[entry.Name] {
			slog.Warn("Skipping lexicon property that collides with reserved field name",
				"lexicon", lexiconID, "property", entry.Name)
			continue
		}
		field := b.buildField(lexiconID, entry.Name, &entry.Property, requiredSet[entry.Name])
		if field != nil {
			fields[entry.Name] = field
		}
	}

	return fields
}

// buildField builds a single GraphQL field from a property.
func (b *ObjectBuilder) buildField(contextLexiconID, name string, prop *lexicon.Property, required bool) *graphql.Field {
	var fieldType graphql.Output

	switch prop.Type {
	case lexicon.TypeRef:
		fieldType = b.resolveRefType(contextLexiconID, prop.Ref)
	case lexicon.TypeUnion:
		fieldType = b.buildUnionType(contextLexiconID, name, prop.Refs)
	case lexicon.TypeArray:
		itemType := b.resolveArrayItemType(contextLexiconID, prop.Items)
		fieldType = graphql.NewList(graphql.NewNonNull(itemType))
	default:
		fieldType = b.mapper.MapPrimitiveType(prop.Type, prop.Format)
	}

	if fieldType == nil {
		// Fallback to String for unknown types
		fieldType = graphql.String
	}

	if required {
		fieldType = graphql.NewNonNull(fieldType)
	}

	field := &graphql.Field{
		Type:        fieldType,
		Description: prop.Description,
	}

	if prop.Type == lexicon.TypeCIDLink {
		field.Resolve = resolveCIDLinkField(name)
	}
	if prop.Type == lexicon.TypeArray && prop.Items != nil && prop.Items.Type == lexicon.TypeCIDLink {
		field.Resolve = resolveCIDLinkArrayField(name)
	}

	return field
}

func resolveCIDLinkField(name string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		value := sourceMapValue(p.Source, name)
		if value == nil {
			return nil, nil
		}

		cid, ok := extractCIDLinkString(value)
		if !ok {
			return nil, nil
		}

		return cid, nil
	}
}

func resolveCIDLinkArrayField(name string) graphql.FieldResolveFn {
	return func(p graphql.ResolveParams) (interface{}, error) {
		value := sourceMapValue(p.Source, name)
		if value == nil {
			return nil, nil
		}

		items := reflect.ValueOf(value)
		if items.Kind() != reflect.Slice && items.Kind() != reflect.Array {
			return nil, nil
		}

		cids := make([]any, items.Len())
		for i := 0; i < items.Len(); i++ {
			item := items.Index(i).Interface()
			if item == nil {
				continue
			}

			cid, ok := extractCIDLinkString(item)
			if ok {
				cids[i] = cid
			}
		}

		return cids, nil
	}
}

func sourceMapValue(source any, name string) any {
	sourceMap, ok := source.(map[string]any)
	if !ok {
		return nil
	}

	return sourceMap[name]
}

// resolveRefType resolves a ref to a GraphQL type.
func (b *ObjectBuilder) resolveRefType(contextLexiconID, ref string) graphql.Output {
	if ref == "" {
		return graphql.String
	}

	// Resolve local refs
	fullRef := ref
	if lexicon.IsLocalRef(ref) {
		fullRef = lexicon.ResolveLocalRef(ref, contextLexiconID)
	}

	// Check if already built
	if t, ok := b.mapper.GetObjectType(fullRef); ok {
		return t
	}

	// Try to resolve from registry
	resolved, ok := b.registry.ResolveRef(ref, contextLexiconID)
	if !ok {
		// Unknown ref - return String as fallback
		return graphql.String
	}

	// Build the type based on what we resolved
	switch def := resolved.(type) {
	case *lexicon.ObjectDef:
		return b.BuildObjectType(fullRef, def)
	case *lexicon.RecordDef:
		resolvedLexiconID := lexicon.IDFromRef(fullRef)
		return b.BuildRecordType(resolvedLexiconID, def)
	default:
		return graphql.String
	}
}

// buildUnionType builds a GraphQL union type from refs.
func (b *ObjectBuilder) buildUnionType(contextLexiconID, fieldName string, refs []string) graphql.Output {
	if len(refs) == 0 {
		return graphql.String
	}

	// Handle string-type refs (primitive unions)
	// These are refs to primitive types like "contributorIdentity" which is just a string
	var objectTypes []*graphql.Object
	hasPrimitives := false

	for _, ref := range refs {
		fullRef := ref
		if lexicon.IsLocalRef(ref) {
			fullRef = lexicon.ResolveLocalRef(ref, contextLexiconID)
		}

		resolved, ok := b.registry.ResolveRef(ref, contextLexiconID)
		if !ok {
			// Check if it's a primitive type ref (like #contributorIdentity -> string)
			hasPrimitives = true
			continue
		}

		switch def := resolved.(type) {
		case *lexicon.ObjectDef:
			objType := b.BuildObjectType(fullRef, def)
			objectTypes = append(objectTypes, objType)
		case *lexicon.RecordDef:
			resolvedLexiconID := lexicon.IDFromRef(fullRef)
			objType := b.BuildRecordType(resolvedLexiconID, def)
			objectTypes = append(objectTypes, objType)
		default:
			hasPrimitives = true
		}
	}

	// If we only have primitives, return JSON scalar
	if len(objectTypes) == 0 {
		return JSONScalar
	}

	// If we have a mix, use JSON as fallback for now
	// (proper handling would need interface types)
	if hasPrimitives {
		return JSONScalar
	}

	// Create union name from context and field
	unionName := lexicon.ToTypeName(contextLexiconID) + capitalizeFirst(fieldName) + "Union"

	// Check if union already exists
	if u, ok := b.mapper.GetUnionType(unionName); ok {
		return u
	}

	// Build union type
	union := graphql.NewUnion(graphql.UnionConfig{
		Name:        unionName,
		Description: fmt.Sprintf("Union type for %s.%s", contextLexiconID, fieldName),
		Types:       objectTypes,
		ResolveType: func(p graphql.ResolveTypeParams) *graphql.Object {
			// Resolve based on $type field in the data
			data, ok := p.Value.(map[string]interface{})
			if !ok {
				if len(objectTypes) > 0 {
					return objectTypes[0]
				}
				return nil
			}
			typeVal, hasType := data["$type"].(string)
			if hasType {
				// Find matching object type
				for _, objType := range objectTypes {
					// Match by type name
					if refToTypeName(typeVal) == objType.Name() {
						return objType
					}
				}
			}
			// Default to first type
			if len(objectTypes) > 0 {
				return objectTypes[0]
			}
			return nil
		},
	})

	b.mapper.SetUnionType(unionName, union)
	return union
}

// resolveArrayItemType resolves the item type for an array.
func (b *ObjectBuilder) resolveArrayItemType(contextLexiconID string, items *lexicon.ArrayItems) graphql.Output {
	if items == nil {
		return graphql.String
	}

	switch items.Type {
	case lexicon.TypeRef:
		return b.resolveRefType(contextLexiconID, items.Ref)
	case lexicon.TypeUnion:
		return b.buildUnionType(contextLexiconID, "items", items.Refs)
	default:
		return b.mapper.MapPrimitiveType(items.Type, "")
	}
}

// refToTypeName converts a ref to a GraphQL type name.
func refToTypeName(ref string) string {
	lexiconID, defName, ok := lexicon.ParseRef(ref)
	if !ok || defName == "" {
		return lexicon.ToTypeName(ref)
	}
	// For refs like "org.hypercerts.defs#uri", create "OrgHypercertsDefsUri"
	return lexicon.ToTypeName(lexiconID) + capitalizeFirst(defName)
}

// capitalizeFirst capitalizes the first letter of a string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-32) + s[1:]
}
