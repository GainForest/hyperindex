//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode"
)

const smokeSchemaQuery = `
query SmokeSchema {
  __schema {
    queryType {
      fields {
        name
        args {
          name
        }
        type {
          kind
          name
          ofType {
            kind
            name
            ofType {
              kind
              name
              ofType {
                kind
                name
              }
            }
          }
        }
      }
    }
    types {
      kind
      name
      fields {
        name
      }
    }
  }
}`

const smokeTypenameQuery = `
query SmokeTypename {
  __typename
}`

type typenameQueryResponse struct {
	Typename string `json:"__typename"`
}

type schemaQueryResponse struct {
	Schema graphQLSchema `json:"__schema"`
}

type graphQLSchema struct {
	QueryType schemaObject `json:"queryType"`
	Types     []schemaType `json:"types"`
}

type schemaObject struct {
	Fields []schemaField `json:"fields"`
}

type schemaType struct {
	Kind   string        `json:"kind"`
	Name   string        `json:"name"`
	Fields []schemaField `json:"fields"`
}

type schemaField struct {
	Name string           `json:"name"`
	Args []schemaArgument `json:"args"`
	Type schemaTypeRef    `json:"type"`
}

type schemaArgument struct {
	Name string `json:"name"`
}

type schemaTypeRef struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	OfType *schemaTypeRef `json:"ofType"`
}

func TestGraphQLTypename(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeTypename", smokeTypenameQuery, nil)

	var payload typenameQueryResponse
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeTypename: decode response data: %v", err)
	}
	if payload.Typename == "" {
		t.Fatal("SmokeTypename: data.__typename is empty, want a non-empty string")
	}
}

func TestSchemaExposesExpectedTypedCollections(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)
	types := typesByName(schema.Types)

	for nsid, queryFieldName := range config.expectations.TypedQueryFields {
		nsid := nsid
		queryFieldName := queryFieldName
		t.Run(nsid, func(t *testing.T) {
			t.Logf("schema typed field check nsid=%q expectedField=%q", nsid, queryFieldName)
			collectionField := requireSchemaFieldForNSID(t, queryFields, queryFieldName, nsid)
			requireSchemaArgument(t, collectionField, "first")

			connectionTypeName := namedTypeName(collectionField.Type)
			connectionType := requireSchemaType(t, types, connectionTypeName)
			requireSchemaField(t, fieldsByName(connectionType.Fields), "edges")
			requireSchemaField(t, fieldsByName(connectionType.Fields), "pageInfo")

			byURIField := requireSchemaFieldForNSID(t, queryFields, queryFieldName+"ByUri", nsid)
			requireSchemaArgument(t, byURIField, "uri")
		})
	}
}

func TestSchemaExcludesNonRecordLexiconsFromTypedCollectionQueries(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)

	for _, nsid := range config.expectations.NonRecordNSIDs {
		nsid := nsid
		t.Run(nsid, func(t *testing.T) {
			queryFieldName := queryFieldNameFromNSID(nsid)
			if _, ok := queryFields[queryFieldName]; ok {
				t.Fatalf("schema exposes non-record lexicon %q as query field %q", nsid, queryFieldName)
			}
			if _, ok := queryFields[queryFieldName+"ByUri"]; ok {
				t.Fatalf("schema exposes non-record lexicon %q as ByUri query field %q", nsid, queryFieldName+"ByUri")
			}
		})
	}
}

func TestSchemaExposesPublicSmokeQueryFields(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)

	recordsField := requireSchemaField(t, queryFields, "records")
	requireSchemaArgument(t, recordsField, "collection")
	requireSchemaArgument(t, recordsField, "first")

	searchField := requireSchemaField(t, queryFields, "search")
	requireSchemaArgument(t, searchField, "query")
	requireSchemaArgument(t, searchField, "first")

	requireSchemaField(t, queryFields, "collectionStats")
}

func fetchGraphQLSchema(t testing.TB, config smokeConfig) graphQLSchema {
	t.Helper()

	response := postGraphQL(t, context.Background(), config, "SmokeSchema", smokeSchemaQuery, nil)

	var payload schemaQueryResponse
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeSchema: decode response data: %v", err)
	}

	return payload.Schema
}

func fieldsByName(fields []schemaField) map[string]schemaField {
	indexed := make(map[string]schemaField, len(fields))
	for _, field := range fields {
		indexed[field.Name] = field
	}
	return indexed
}

func typesByName(types []schemaType) map[string]schemaType {
	indexed := make(map[string]schemaType, len(types))
	for _, typ := range types {
		if typ.Name != "" {
			indexed[typ.Name] = typ
		}
	}
	return indexed
}

func requireSchemaField(t testing.TB, fields map[string]schemaField, name string) schemaField {
	t.Helper()

	field, ok := fields[name]
	if !ok {
		t.Fatalf("schema is missing field %q", name)
	}
	return field
}

func requireSchemaFieldForNSID(t testing.TB, fields map[string]schemaField, name string, nsid string) schemaField {
	t.Helper()

	field, ok := fields[name]
	if !ok {
		t.Fatalf("schema is missing field %q for NSID %q", name, nsid)
	}
	return field
}

func requireSchemaType(t testing.TB, types map[string]schemaType, name string) schemaType {
	t.Helper()

	typ, ok := types[name]
	if !ok {
		t.Fatalf("schema is missing type %q", name)
	}
	return typ
}

func requireSchemaArgument(t testing.TB, field schemaField, name string) {
	t.Helper()

	for _, argument := range field.Args {
		if argument.Name == name {
			return
		}
	}
	t.Fatalf("schema field %q is missing argument %q", field.Name, name)
}

func namedTypeName(typeRef schemaTypeRef) string {
	for typeRef.OfType != nil {
		typeRef = *typeRef.OfType
	}
	return typeRef.Name
}

func queryFieldNameFromNSID(nsid string) string {
	parts := strings.Split(nsid, ".")
	for index := 1; index < len(parts); index++ {
		parts[index] = upperFirstRune(parts[index])
	}
	return strings.Join(parts, "")
}

func upperFirstRune(value string) string {
	for index, first := range value {
		return string(unicode.ToUpper(first)) + value[index+len(string(first)):]
	}
	return value
}
