// Package schema provides nested GraphQL where-filter generation and extraction helpers.
package schema

import (
	"fmt"
	"strings"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/types"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

const nestedFilterMaxLexiconDepth = 3

func (b *Builder) filterInputForTopLevelProperty(contextLexiconID, fieldName string, prop lexicon.Property) *graphql.InputObject {
	if input := types.FilterInputForLexiconType(prop.Type, prop.Format); input != nil {
		return input
	}

	return b.nestedFilterInputForProperty(contextLexiconID, lexicon.ToTypeName(contextLexiconID)+inputNamePart(fieldName), prop, 1, false)
}

func (b *Builder) nestedFilterInputForProperty(contextLexiconID, typeName string, prop lexicon.Property, depth int, insideArrayAny bool) *graphql.InputObject {
	if input := types.ExactFilterInputForLexiconType(prop.Type, prop.Format); input != nil {
		return input
	}

	switch prop.Type {
	case lexicon.TypeArray:
		return b.nestedFilterInputForArray(contextLexiconID, typeName, prop.Items, depth, insideArrayAny)
	case lexicon.TypeRef:
		return b.nestedFilterInputForRef(contextLexiconID, typeName, prop.Ref, depth, insideArrayAny)
	case lexicon.TypeUnion:
		return b.nestedFilterInputForUnion(contextLexiconID, typeName, prop.Refs, depth, insideArrayAny)
	case lexicon.TypeObject:
		// Inline object properties are not represented in lexicon.Property today, so
		// only presence can be generated safely.
		return b.presenceOnlyNestedInput(typeName)
	default:
		return types.FilterInputForLexiconProperty(prop.Type, prop.Format)
	}
}

func (b *Builder) nestedFilterInputForArray(contextLexiconID, typeName string, items *lexicon.ArrayItems, depth int, insideArrayAny bool) *graphql.InputObject {
	if items == nil {
		return types.PresenceFilterInput
	}

	inputName := typeName + "ArrayFilterInput"
	if existing := b.nestedWhereInputTypes[inputName]; existing != nil {
		return existing
	}

	fields := graphql.InputObjectConfigFieldMap{
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "True matches missing or null arrays; false matches present arrays.",
		},
	}

	// Repository extraction currently supports one array any scope. Once this input
	// is already inside another array any item, keep nested array fields presence-only
	// instead of advertising a second any filter that would fail at execution time.
	if !insideArrayAny {
		itemInput := b.nestedFilterInputForArrayItems(contextLexiconID, typeName+"Item", items, depth, true)
		if itemInput != nil {
			fields["any"] = &graphql.InputObjectFieldConfig{
				Type:        itemInput,
				Description: "Keep records where at least one array item matches this filter.",
			}
		}
	}

	input := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:        inputName,
		Description: "Filter conditions for an array field, including any-item matching when supported.",
		Fields:      fields,
	})
	b.nestedWhereInputTypes[inputName] = input
	return input
}

func (b *Builder) nestedFilterInputForArrayItems(contextLexiconID, typeName string, items *lexicon.ArrayItems, depth int, insideArrayAny bool) *graphql.InputObject {
	prop := lexicon.Property{Type: items.Type, Ref: items.Ref, Refs: items.Refs}
	return b.nestedFilterInputForProperty(contextLexiconID, typeName, prop, depth, insideArrayAny)
}

func (b *Builder) nestedFilterInputForRef(contextLexiconID, typeName, ref string, depth int, insideArrayAny bool) *graphql.InputObject {
	resolved, ok := b.registry.ResolveRef(ref, contextLexiconID)
	if !ok {
		return b.presenceOnlyNestedInput(typeName)
	}

	switch def := resolved.(type) {
	case *lexicon.ObjectDef:
		return b.nestedFilterInputForObjectDef(contextLexiconID, typeName, def, depth, insideArrayAny)
	case *lexicon.RecordDef:
		return b.nestedFilterInputForRecordDef(contextLexiconID, typeName, def, depth, insideArrayAny)
	default:
		return b.presenceOnlyNestedInput(typeName)
	}
}

func (b *Builder) nestedFilterInputForUnion(contextLexiconID, typeName string, refs []string, depth int, insideArrayAny bool) *graphql.InputObject {
	inputName := typeName + "UnionFilterInput"
	if existing := b.nestedWhereInputTypes[inputName]; existing != nil {
		return existing
	}

	fields := presenceNestedFields()
	conflicts := map[string]bool{}
	for _, ref := range refs {
		resolved, ok := b.registry.ResolveRef(ref, contextLexiconID)
		if !ok {
			continue
		}
		var memberFields graphql.InputObjectConfigFieldMap
		switch def := resolved.(type) {
		case *lexicon.ObjectDef:
			memberFields = b.nestedFilterFieldsForObjectDef(contextLexiconID, typeName+inputNamePart(ref), def, depth, insideArrayAny)
		case *lexicon.RecordDef:
			memberFields = b.nestedFilterFieldsForRecordDef(contextLexiconID, typeName+inputNamePart(ref), def, depth, insideArrayAny)
		}
		mergeNestedFilterFields(fields, memberFields, conflicts)
	}

	input := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:        inputName,
		Description: "Filter conditions for a union field. Shared member fields are exposed structurally.",
		Fields:      fields,
	})
	b.nestedWhereInputTypes[inputName] = input
	return input
}

func (b *Builder) nestedFilterInputForObjectDef(contextLexiconID, typeName string, def *lexicon.ObjectDef, depth int, insideArrayAny bool) *graphql.InputObject {
	inputName := typeName + "ObjectFilterInput"
	if existing := b.nestedWhereInputTypes[inputName]; existing != nil {
		return existing
	}

	fields := b.nestedFilterFieldsForObjectDef(contextLexiconID, typeName, def, depth, insideArrayAny)
	input := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:        inputName,
		Description: "Filter conditions for a nested object field.",
		Fields:      fields,
	})
	b.nestedWhereInputTypes[inputName] = input
	return input
}

func (b *Builder) nestedFilterInputForRecordDef(contextLexiconID, typeName string, def *lexicon.RecordDef, depth int, insideArrayAny bool) *graphql.InputObject {
	inputName := typeName + "ObjectFilterInput"
	if existing := b.nestedWhereInputTypes[inputName]; existing != nil {
		return existing
	}

	fields := b.nestedFilterFieldsForRecordDef(contextLexiconID, typeName, def, depth, insideArrayAny)
	input := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:        inputName,
		Description: "Filter conditions for a nested record-shaped field.",
		Fields:      fields,
	})
	b.nestedWhereInputTypes[inputName] = input
	return input
}

func (b *Builder) nestedFilterFieldsForObjectDef(contextLexiconID, typeName string, def *lexicon.ObjectDef, depth int, insideArrayAny bool) graphql.InputObjectConfigFieldMap {
	return b.nestedFilterFieldsForProperties(contextLexiconID, typeName, def.Properties, depth, insideArrayAny)
}

func (b *Builder) nestedFilterFieldsForRecordDef(contextLexiconID, typeName string, def *lexicon.RecordDef, depth int, insideArrayAny bool) graphql.InputObjectConfigFieldMap {
	return b.nestedFilterFieldsForProperties(contextLexiconID, typeName, def.Properties, depth, insideArrayAny)
}

func (b *Builder) nestedFilterFieldsForProperties(contextLexiconID, typeName string, properties []lexicon.PropertyEntry, depth int, insideArrayAny bool) graphql.InputObjectConfigFieldMap {
	fields := presenceNestedFields()
	for _, entry := range properties {
		nextDepth := depth + 1
		if nextDepth > nestedFilterMaxLexiconDepth {
			continue
		}
		input := b.nestedFilterInputForProperty(contextLexiconID, typeName+inputNamePart(entry.Name), entry.Property, nextDepth, insideArrayAny)
		if input == nil {
			continue
		}
		fields[entry.Name] = &graphql.InputObjectFieldConfig{
			Type:        input,
			Description: fmt.Sprintf("Filter by nested %s", entry.Name),
		}
	}
	return fields
}

func (b *Builder) presenceOnlyNestedInput(typeName string) *graphql.InputObject {
	inputName := typeName + "PresenceFilterInput"
	if existing := b.nestedWhereInputTypes[inputName]; existing != nil {
		return existing
	}
	input := graphql.NewInputObject(graphql.InputObjectConfig{
		Name:        inputName,
		Description: "Filter conditions for checking whether a nested field is missing/null or present.",
		Fields:      presenceNestedFields(),
	})
	b.nestedWhereInputTypes[inputName] = input
	return input
}

func presenceNestedFields() graphql.InputObjectConfigFieldMap {
	return graphql.InputObjectConfigFieldMap{
		"isNull": &graphql.InputObjectFieldConfig{
			Type:        graphql.Boolean,
			Description: "True matches missing or null fields; false matches present and non-null fields.",
		},
	}
}

func mergeNestedFilterFields(dst, src graphql.InputObjectConfigFieldMap, conflicts map[string]bool) {
	for name, field := range src {
		if name == "isNull" || conflicts[name] {
			continue
		}
		if existing, ok := dst[name]; ok && existing.Type.String() != field.Type.String() {
			delete(dst, name)
			conflicts[name] = true
			continue
		}
		dst[name] = field
	}
}

func inputNamePart(value string) string {
	var result strings.Builder
	capitalizeNext := true
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			if capitalizeNext && r >= 'a' && r <= 'z' {
				r -= 'a' - 'A'
			}
			result.WriteRune(r)
			capitalizeNext = false
			continue
		}
		capitalizeNext = true
	}
	if result.Len() == 0 {
		return "Field"
	}
	return result.String()
}

func extractNestedPropertyFilters(
	contextLexiconID string,
	registry *lexicon.Registry,
	fieldName string,
	prop lexicon.Property,
	filterMap map[string]interface{},
) ([]repositories.FieldFilter, error) {
	return extractNestedPropertyFiltersAtPath(contextLexiconID, registry, fieldName, prop, filterMap, []string{fieldName}, nil, 1)
}

func extractNestedPropertyFiltersAtPath(
	contextLexiconID string,
	registry *lexicon.Registry,
	fieldName string,
	prop lexicon.Property,
	filterMap map[string]interface{},
	jsonPath []string,
	arrayPath []string,
	depth int,
) ([]repositories.FieldFilter, error) {
	var filters []repositories.FieldFilter

	if val, ok := filterMap["isNull"]; ok && val != nil {
		filters = append(filters, repositories.FieldFilter{
			Field:     fieldName,
			Path:      jsonPath,
			ArrayPath: arrayPath,
			Operator:  "isNull",
			Value:     val,
			FieldType: prop.Type,
		})
	}

	if scalarType := scalarFieldType(prop); scalarType != "" {
		for _, op := range []string{"eq", "in"} {
			val, ok := filterMap[op]
			if !ok || val == nil {
				continue
			}
			filters = append(filters, repositories.FieldFilter{
				Field:     fieldName,
				Path:      jsonPath,
				ArrayPath: arrayPath,
				Operator:  op,
				Value:     val,
				FieldType: scalarType,
			})
		}
		return filters, nil
	}

	if depth >= nestedFilterMaxLexiconDepth && prop.Type != lexicon.TypeArray {
		return filters, nil
	}

	switch prop.Type {
	case lexicon.TypeArray:
		anyVal, ok := filterMap["any"].(map[string]interface{})
		if !ok || anyVal == nil || prop.Items == nil {
			return filters, nil
		}
		if len(arrayPath) > 0 {
			return nil, fmt.Errorf("nested array any filters inside another array any are not supported for field %q at path %q", fieldName, strings.Join(jsonPath, "."))
		}
		itemProp := lexicon.Property{Type: prop.Items.Type, Ref: prop.Items.Ref, Refs: prop.Items.Refs}
		anyFilters, err := extractNestedPropertyFiltersAtPath(contextLexiconID, registry, fieldName, itemProp, anyVal, nil, jsonPath, depth)
		if err != nil {
			return nil, err
		}
		filters = append(filters, anyFilters...)
	case lexicon.TypeRef:
		childFilters, err := extractNestedRefFilters(contextLexiconID, registry, fieldName, prop.Ref, filterMap, jsonPath, arrayPath, depth)
		if err != nil {
			return nil, err
		}
		filters = append(filters, childFilters...)
	case lexicon.TypeUnion:
		childFilters, err := extractNestedUnionFilters(contextLexiconID, registry, fieldName, prop.Refs, filterMap, jsonPath, arrayPath, depth)
		if err != nil {
			return nil, err
		}
		filters = append(filters, childFilters...)
	}

	return filters, nil
}

func extractNestedRefFilters(
	contextLexiconID string,
	registry *lexicon.Registry,
	fieldName string,
	ref string,
	filterMap map[string]interface{},
	jsonPath []string,
	arrayPath []string,
	depth int,
) ([]repositories.FieldFilter, error) {
	resolved, ok := registry.ResolveRef(ref, contextLexiconID)
	if !ok {
		return nil, nil
	}

	switch def := resolved.(type) {
	case *lexicon.ObjectDef:
		return extractNestedObjectFilters(contextLexiconID, registry, fieldName, def.Properties, filterMap, jsonPath, arrayPath, depth)
	case *lexicon.RecordDef:
		return extractNestedObjectFilters(contextLexiconID, registry, fieldName, def.Properties, filterMap, jsonPath, arrayPath, depth)
	default:
		return nil, nil
	}
}

func extractNestedUnionFilters(
	contextLexiconID string,
	registry *lexicon.Registry,
	fieldName string,
	refs []string,
	filterMap map[string]interface{},
	jsonPath []string,
	arrayPath []string,
	depth int,
) ([]repositories.FieldFilter, error) {
	var filters []repositories.FieldFilter
	for nestedField, nestedVal := range filterMap {
		if nestedField == "isNull" || nestedVal == nil {
			continue
		}
		nestedMap, ok := nestedVal.(map[string]interface{})
		if !ok || nestedMap == nil {
			continue
		}

		prop, ok := findUnionNestedProperty(contextLexiconID, registry, refs, nestedField)
		if !ok {
			continue
		}
		if depth+1 > nestedFilterMaxLexiconDepth {
			continue
		}
		childFilters, err := extractNestedPropertyFiltersAtPath(contextLexiconID, registry, fieldName, prop, nestedMap, appendPath(jsonPath, nestedField), arrayPath, depth+1)
		if err != nil {
			return nil, err
		}
		filters = append(filters, childFilters...)
	}
	return filters, nil
}

func extractNestedObjectFilters(
	contextLexiconID string,
	registry *lexicon.Registry,
	fieldName string,
	properties []lexicon.PropertyEntry,
	filterMap map[string]interface{},
	jsonPath []string,
	arrayPath []string,
	depth int,
) ([]repositories.FieldFilter, error) {
	var filters []repositories.FieldFilter
	for nestedField, nestedVal := range filterMap {
		if nestedField == "isNull" || nestedVal == nil {
			continue
		}
		nestedMap, ok := nestedVal.(map[string]interface{})
		if !ok || nestedMap == nil {
			continue
		}
		prop, ok := findPropertyEntry(properties, nestedField)
		if !ok {
			continue
		}
		if depth+1 > nestedFilterMaxLexiconDepth {
			continue
		}
		childFilters, err := extractNestedPropertyFiltersAtPath(contextLexiconID, registry, fieldName, prop, nestedMap, appendPath(jsonPath, nestedField), arrayPath, depth+1)
		if err != nil {
			return nil, err
		}
		filters = append(filters, childFilters...)
	}
	return filters, nil
}

func findUnionNestedProperty(contextLexiconID string, registry *lexicon.Registry, refs []string, fieldName string) (lexicon.Property, bool) {
	var found lexicon.Property
	foundOK := false
	for _, ref := range refs {
		resolved, ok := registry.ResolveRef(ref, contextLexiconID)
		if !ok {
			continue
		}
		var properties []lexicon.PropertyEntry
		switch def := resolved.(type) {
		case *lexicon.ObjectDef:
			properties = def.Properties
		case *lexicon.RecordDef:
			properties = def.Properties
		default:
			continue
		}
		prop, ok := findPropertyEntry(properties, fieldName)
		if !ok {
			continue
		}
		if foundOK && nestedPropertyFilterType(found) != nestedPropertyFilterType(prop) {
			return lexicon.Property{}, false
		}
		found = prop
		foundOK = true
	}
	return found, foundOK
}

func findPropertyEntry(properties []lexicon.PropertyEntry, fieldName string) (lexicon.Property, bool) {
	for _, entry := range properties {
		if entry.Name == fieldName {
			return entry.Property, true
		}
	}
	return lexicon.Property{}, false
}

func nestedPropertyFilterType(prop lexicon.Property) string {
	if input := types.ExactFilterInputForLexiconType(prop.Type, prop.Format); input != nil {
		return input.Name()
	}
	return prop.Type
}

func scalarFieldType(prop lexicon.Property) string {
	if types.ExactFilterInputForLexiconType(prop.Type, prop.Format) == nil {
		return ""
	}
	if prop.Format == lexicon.FormatDatetime {
		return "datetime"
	}
	return prop.Type
}

func appendPath(path []string, field string) []string {
	next := make([]string, 0, len(path)+1)
	next = append(next, path...)
	next = append(next, field)
	return next
}
