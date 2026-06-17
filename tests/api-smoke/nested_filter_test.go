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

const smokeNestedFilterSameElementCandidatesQuery = `
query SmokeNestedFilterSameElementCandidates {
  orgHypercertsCollection(first: 100) {
    edges {
      node {
        uri
        items {
          itemIdentifier {
            uri
            cid
          }
        }
      }
    }
  }
}`

const smokeNestedFilterSameElementQuery = `
query SmokeNestedFilterSameElement($uri: String!, $cid: String!) {
  orgHypercertsCollection(
    first: 100
    where: { items: { any: { itemIdentifier: { uri: { eq: $uri }, cid: { eq: $cid } } } } }
  ) {
    edges {
      node {
        uri
        items {
          itemIdentifier {
            uri
            cid
          }
        }
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

type nestedCollectionStrongRef struct {
	URI string `json:"uri"`
	CID string `json:"cid"`
}

type nestedCollectionItem struct {
	ItemIdentifier nestedCollectionStrongRef `json:"itemIdentifier"`
}

type nestedCollectionNode struct {
	URI   string                 `json:"uri"`
	Items []nestedCollectionItem `json:"items"`
}

type nestedCollectionSameElementResponse struct {
	OrgHypercertsCollection struct {
		Edges []struct {
			Node nestedCollectionNode `json:"node"`
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
	requestedURI := "at://did:plc:example/org.hypercerts.claim.activity/nonexistent-smoke-record"
	response := postGraphQL(t, context.Background(), config, "SmokeThreeLevelNestedFilter", smokeThreeLevelNestedFilterQuery, map[string]any{
		"uri": requestedURI,
	})

	var decoded nestedCollectionFilterResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeThreeLevelNestedFilter data: %v", err)
	}
	if len(decoded.OrgHypercertsCollection.Edges) != 0 {
		t.Fatalf("nested filter query for guaranteed-miss URI %q returned %d edges, want 0", requestedURI, len(decoded.OrgHypercertsCollection.Edges))
	}

	smokeLog("✓ org.hypercerts.collection three-level nested where filter executes and filters")
}

func TestNestedWhereAnyPredicatesMatchSameArrayElement(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeNestedFilterSameElementCandidates", smokeNestedFilterSameElementCandidatesQuery, nil)

	var candidates nestedCollectionSameElementResponse
	if err := json.Unmarshal(response.Data, &candidates); err != nil {
		t.Fatalf("decode SmokeNestedFilterSameElementCandidates data: %v", err)
	}

	candidateURI, requestedURI, requestedCID, ok := findMismatchedItemIdentifierCandidate(candidates)
	if !ok {
		t.Skip("no org.hypercerts.collection record with two itemIdentifier values suitable for same-element nested filter smoke coverage")
	}

	response = postGraphQL(t, context.Background(), config, "SmokeNestedFilterSameElement", smokeNestedFilterSameElementQuery, map[string]any{
		"uri": requestedURI,
		"cid": requestedCID,
	})

	var filtered nestedCollectionSameElementResponse
	if err := json.Unmarshal(response.Data, &filtered); err != nil {
		t.Fatalf("decode SmokeNestedFilterSameElement data: %v", err)
	}
	for _, edge := range filtered.OrgHypercertsCollection.Edges {
		if !nodeHasItemIdentifierPair(edge.Node, requestedURI, requestedCID) {
			t.Fatalf("nested any filter returned %q for itemIdentifier.uri=%q and itemIdentifier.cid=%q, but no single item has both values", edge.Node.URI, requestedURI, requestedCID)
		}
	}

	smokeLog("✓ org.hypercerts.collection nested any keeps uri/cid predicates on the same item (candidate %s)", candidateURI)
}

func findMismatchedItemIdentifierCandidate(response nestedCollectionSameElementResponse) (recordURI string, requestedURI string, requestedCID string, ok bool) {
	for _, edge := range response.OrgHypercertsCollection.Edges {
		refs := make([]nestedCollectionStrongRef, 0, len(edge.Node.Items))
		for _, item := range edge.Node.Items {
			ref := item.ItemIdentifier
			if ref.URI != "" && ref.CID != "" {
				refs = append(refs, ref)
			}
		}
		for i, uriRef := range refs {
			for j, cidRef := range refs {
				if i == j {
					continue
				}
				if !nodeHasItemIdentifierPair(edge.Node, uriRef.URI, cidRef.CID) {
					return edge.Node.URI, uriRef.URI, cidRef.CID, true
				}
			}
		}
	}
	return "", "", "", false
}

func nodeHasItemIdentifierPair(node nestedCollectionNode, uri string, cid string) bool {
	for _, item := range node.Items {
		if item.ItemIdentifier.URI == uri && item.ItemIdentifier.CID == cid {
			return true
		}
	}
	return false
}
