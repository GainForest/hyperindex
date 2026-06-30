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
      inputFields {
        name
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
	Kind        string             `json:"kind"`
	Name        string             `json:"name"`
	Fields      []schemaField      `json:"fields"`
	InputFields []schemaInputField `json:"inputFields"`
}

type schemaField struct {
	Name string           `json:"name"`
	Args []schemaArgument `json:"args"`
	Type schemaTypeRef    `json:"type"`
}

type schemaArgument struct {
	Name string        `json:"name"`
	Type schemaTypeRef `json:"type"`
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

	smokeLog("✓ GraphQL endpoint is responding")
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

func TestSchemaExposesURIWhereFilter(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)
	types := typesByName(schema.Types)

	uriFilter := requireSchemaType(t, types, "URIFilterInput")
	if uriFilter.Kind != "INPUT_OBJECT" {
		t.Fatalf("URIFilterInput kind = %q, want INPUT_OBJECT", uriFilter.Kind)
	}
	uriFilterFields := inputFieldsByName(uriFilter.InputFields)
	requireSchemaInputField(t, uriFilterFields, "eq")
	requireSchemaInputField(t, uriFilterFields, "in")
	for _, absent := range []string{"contains", "startsWith", "neq", "isNull"} {
		if _, exists := uriFilterFields[absent]; exists {
			t.Fatalf("URIFilterInput exposes unsupported field %q", absent)
		}
	}

	for nsid, queryFieldName := range config.expectations.TypedQueryFields {
		nsid := nsid
		queryFieldName := queryFieldName
		t.Run(nsid, func(t *testing.T) {
			collectionField := requireSchemaFieldForNSID(t, queryFields, queryFieldName, nsid)
			whereArg := requireSchemaArgument(t, collectionField, "where")
			whereInputName := namedTypeName(whereArg.Type)
			whereInput := requireSchemaType(t, types, whereInputName)
			uriField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "uri")
			if got := namedTypeName(uriField.Type); got != "URIFilterInput" {
				t.Fatalf("%s.uri filter type = %q, want URIFilterInput", whereInputName, got)
			}
		})
	}

	smokeLog("✓ Typed collection schemas expose uri where filters")
}

func TestSchemaExposesAuthorLabelWhereFilter(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)
	types := typesByName(schema.Types)

	for nsid, queryFieldName := range config.expectations.TypedQueryFields {
		nsid := nsid
		queryFieldName := queryFieldName
		t.Run(nsid, func(t *testing.T) {
			collectionField := requireSchemaFieldForNSID(t, queryFields, queryFieldName, nsid)
			whereArg := requireSchemaArgument(t, collectionField, "where")
			whereInputName := namedTypeName(whereArg.Type)
			whereInput := requireSchemaType(t, types, whereInputName)
			whereFields := inputFieldsByName(whereInput.InputFields)

			externalLabelsField := requireSchemaInputField(t, whereFields, "externalLabels")
			if got := namedTypeName(externalLabelsField.Type); got != "ExternalLabelWhereInput" {
				t.Fatalf("%s.externalLabels filter type = %q, want ExternalLabelWhereInput", whereInputName, got)
			}
			authorLabelsField := requireSchemaInputField(t, whereFields, "authorLabels")
			if got := namedTypeName(authorLabelsField.Type); got != "ExternalLabelWhereInput" {
				t.Fatalf("%s.authorLabels filter type = %q, want ExternalLabelWhereInput", whereInputName, got)
			}

			recordType := requireSchemaType(t, types, typeNameFromNSID(nsid))
			recordFields := fieldsByName(recordType.Fields)
			requireSchemaField(t, recordFields, "externalLabels")
			if _, exists := recordFields["authorLabels"]; exists {
				t.Fatalf("record type %s exposes authorLabels field; authorLabels should only be a where filter", recordType.Name)
			}
		})
	}

	smokeLog("✓ Typed collection schemas expose authorLabels where filters")
}

func TestSchemaExposesBadgeAwardBadgeTypeFilter(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	awardWhereInput := requireSchemaType(t, types, "AppCertifiedBadgeAwardWhereInput")
	badgeTypeField := requireSchemaInputField(t, inputFieldsByName(awardWhereInput.InputFields), "badgeType")
	if got := namedTypeName(badgeTypeField.Type); got != "StringFilterInput" {
		t.Fatalf("AppCertifiedBadgeAwardWhereInput.badgeType filter type = %q, want StringFilterInput", got)
	}

	definitionWhereInput := requireSchemaType(t, types, "AppCertifiedBadgeDefinitionWhereInput")
	definitionBadgeTypeField := requireSchemaInputField(t, inputFieldsByName(definitionWhereInput.InputFields), "badgeType")
	if got := namedTypeName(definitionBadgeTypeField.Type); got != "StringFilterInput" {
		t.Fatalf("AppCertifiedBadgeDefinitionWhereInput.badgeType filter type = %q, want StringFilterInput", got)
	}

	smokeLog("✓ Badge award schemas expose badgeType where filters")
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

	if config.expectations.EndorsementClosure.configured() {
		endorsementClosureField := requireSchemaField(t, queryFields, "endorsementClosure")
		whereArg := requireSchemaArgument(t, endorsementClosureField, "where")
		requireSchemaArgument(t, endorsementClosureField, "first")
		requireSchemaArgument(t, endorsementClosureField, "after")

		whereInput := requireSchemaType(t, typesByName(schema.Types), namedTypeName(whereArg.Type))
		didField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "did")
		if got := namedTypeName(didField.Type); got != "EndorsementClosureDIDFilterInput" {
			t.Fatalf("EndorsementClosureWhereInput.did type = %q, want EndorsementClosureDIDFilterInput", got)
		}
		types := typesByName(schema.Types)
		didFilter := requireSchemaType(t, types, "EndorsementClosureDIDFilterInput")
		didFilterFields := inputFieldsByName(didFilter.InputFields)
		requireSchemaInputField(t, didFilterFields, "eq")
		if _, exists := didFilterFields["in"]; exists {
			t.Fatal("EndorsementClosureDIDFilterInput exposes unsupported field in")
		}

		accountType := requireSchemaType(t, types, "EndorsementAccount")
		accountFields := fieldsByName(accountType.Fields)
		requireSchemaField(t, accountFields, "certifiedProfileData")
		requireSchemaField(t, accountFields, "viaAccounts")
		if _, exists := accountFields["via"]; exists {
			t.Fatal("EndorsementAccount exposes removed field via")
		}
		viaAccountType := requireSchemaType(t, types, "EndorsementViaAccount")
		viaAccountFields := fieldsByName(viaAccountType.Fields)
		requireSchemaField(t, viaAccountFields, "did")
		requireSchemaField(t, viaAccountFields, "certifiedProfileData")
	}

	smokeLog("✓ Public schema has expected GraphQL query fields")
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
		t.Fatalf("schema is missing field %q for NSID %q; this usually means the lexicon was not loaded when the backend started or the deployment expectations are out of date", name, nsid)
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

func requireSchemaArgument(t testing.TB, field schemaField, name string) schemaArgument {
	t.Helper()

	for _, argument := range field.Args {
		if argument.Name == name {
			return argument
		}
	}
	t.Fatalf("schema field %q is missing argument %q", field.Name, name)
	return schemaArgument{}
}

func requireSchemaInputField(t testing.TB, fields map[string]schemaInputField, name string) schemaInputField {
	t.Helper()

	field, ok := fields[name]
	if !ok {
		t.Fatalf("schema input is missing field %q", name)
	}
	return field
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

func typeNameFromNSID(nsid string) string {
	parts := strings.Split(nsid, ".")
	for index := range parts {
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
