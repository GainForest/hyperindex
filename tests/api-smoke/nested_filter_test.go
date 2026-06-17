//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const smokeThreeLevelNestedFilterQuery = `
query SmokeThreeLevelNestedFilter($uri: String!) {
  orgHypercertsCollection(
    first: 1
    where: { items: { any: { itemIdentifier: { uri: { eq: $uri } } } } }
  ) {
    totalCount
    edges {
      node {
        uri
      }
    }
  }
}`

type nestedCollectionFilterResponse struct {
	OrgHypercertsCollection struct {
		TotalCount int `json:"totalCount"`
		Edges      []struct {
			Node struct {
				URI string `json:"uri"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"orgHypercertsCollection"`
}

func TestSchemaExposesThreeLevelNestedWhereFilter(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, "OrgHypercertsCollectionWhereInput")
	itemsField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "items")
	itemsInput := requireSchemaType(t, types, namedTypeName(itemsField.Type))

	anyField := requireSchemaInputField(t, inputFieldsByName(itemsInput.InputFields), "any")
	itemInput := requireSchemaType(t, types, namedTypeName(anyField.Type))

	itemIdentifierField := requireSchemaInputField(t, inputFieldsByName(itemInput.InputFields), "itemIdentifier")
	strongRefInput := requireSchemaType(t, types, namedTypeName(itemIdentifierField.Type))

	uriField := requireSchemaInputField(t, inputFieldsByName(strongRefInput.InputFields), "uri")
	if got := namedTypeName(uriField.Type); got != "ExactStringFilterInput" {
		t.Fatalf("items.any.itemIdentifier.uri filter type = %q, want ExactStringFilterInput", got)
	}

	exactStringInput := requireSchemaType(t, types, "ExactStringFilterInput")
	exactStringFields := inputFieldsByName(exactStringInput.InputFields)
	requireSchemaInputField(t, exactStringFields, "eq")
	requireSchemaInputField(t, exactStringFields, "in")
	requireSchemaInputField(t, exactStringFields, "isNull")
	if _, exists := exactStringFields["contains"]; exists {
		t.Fatal("ExactStringFilterInput exposes contains, but nested filters should only allow exact operators")
	}

	smokeLog("✓ org.hypercerts.collection exposes three-level nested itemIdentifier.uri filters")
}

func TestThreeLevelNestedWhereFilterQueryExecutes(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeThreeLevelNestedFilter", smokeThreeLevelNestedFilterQuery, map[string]any{
		"uri": "at://did:plc:example/org.hypercerts.claim.activity/nonexistent-smoke-record",
	})

	var decoded nestedCollectionFilterResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeThreeLevelNestedFilter data: %v", err)
	}
	if len(decoded.OrgHypercertsCollection.Edges) > 1 {
		t.Fatalf("nested filter query returned %d edges with first: 1, want at most 1", len(decoded.OrgHypercertsCollection.Edges))
	}

	smokeLog("✓ org.hypercerts.collection three-level nested where filter executes")
}
