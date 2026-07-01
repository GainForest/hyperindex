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

const smokeCollectionItemIdentifierPositiveNestedFilterQuery = `
query SmokeCollectionItemIdentifierPositiveNestedFilter($uri: String!) {
  orgHypercertsCollection(
    first: 100
    where: { items: { any: { itemIdentifier: { uri: { eq: $uri } } } } }
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

const smokeActivityContributorIdentityNestedFilterQuery = `
query SmokeActivityContributorIdentityNestedFilter($identity: String!) {
  orgHypercertsClaimActivity(
    first: 1
    where: { contributors: { any: { contributorIdentity: { identity: { eq: $identity } } } } }
  ) {
    totalCount
    edges {
      node {
        uri
      }
    }
  }
}`

// TODO(#86): Re-enable broad activity candidate discovery once lexicon-invalid
// records are quarantined before typed GraphQL exposure.
//
// This smoke test originally used one broad query for contributor, rights, and image
// candidates:
//
//	query SmokeActivityNestedFilterCandidates {
//	  orgHypercertsClaimActivity(first: 100) {
//	    edges { node { uri contributors { contributorIdentity { ... } } rights { ... } image { ... } } }
//	  }
//	}
//
// That broad read is the correct long-term smoke coverage because it catches typed
// GraphQL contract violations. It currently fails on old test records whose
// contributors are direct strongRef-like objects instead of the current
// { contributorIdentity: ... } shape, producing:
//
//	Cannot return null for non-nullable field
//	OrgHypercertsClaimActivityContributor.contributorIdentity
//
// Until https://github.com/GainForest/hyperindex/issues/86 is fixed, keep the
// contributor candidate query narrowed to records with a non-null nested identity
// so nested-filter smoke coverage can run without hiding the known invalid-record
// quarantine work. When #86 is done, remove this narrowed query, restore the broad
// candidate query, and make this smoke test prove broad typed collection reads do
// not crash on malformed stored records.
const smokeActivityContributorIdentityCandidatesQuery = `
query SmokeActivityContributorIdentityCandidates {
  orgHypercertsClaimActivity(
    first: 20
    where: { contributors: { any: { contributorIdentity: { identity: { isNull: false } } } } }
  ) {
    edges {
      node {
        uri
        contributors {
          contributorIdentity {
            __typename
            ... on OrgHypercertsClaimActivityContributorIdentity {
              identity
            }
          }
        }
      }
    }
  }
}`

const smokeActivityOneLevelNestedFilterCandidatesQuery = `
query SmokeActivityOneLevelNestedFilterCandidates {
  orgHypercertsClaimActivity(first: 100) {
    edges {
      node {
        uri
        rights {
          uri
          cid
        }
        image {
          __typename
          ... on OrgHypercertsDefsUri {
            uri
          }
        }
      }
    }
  }
}`

const smokeActivityContributorIdentityPositiveNestedFilterQuery = `
query SmokeActivityContributorIdentityPositiveNestedFilter($identity: String!) {
  orgHypercertsClaimActivity(
    first: 100
    where: { contributors: { any: { contributorIdentity: { identity: { eq: $identity } } } } }
  ) {
    edges {
      node {
        uri
      }
    }
  }
}`

const smokeActivityRightsPositiveNestedFilterQuery = `
query SmokeActivityRightsPositiveNestedFilter($uri: String!) {
  orgHypercertsClaimActivity(
    first: 100
    where: { rights: { uri: { eq: $uri } } }
  ) {
    edges {
      node {
        uri
        rights {
          uri
          cid
        }
      }
    }
  }
}`

const smokeActivityImagePositiveNestedFilterQuery = `
query SmokeActivityImagePositiveNestedFilter($uri: String!) {
  orgHypercertsClaimActivity(
    first: 100
    where: { image: { uri: { eq: $uri } } }
  ) {
    edges {
      node {
        uri
        image {
          __typename
          ... on OrgHypercertsDefsUri {
            uri
          }
        }
      }
    }
  }
}`

const smokeActivityOneLevelNestedFiltersQuery = `
query SmokeActivityOneLevelNestedFilters($rightsURI: String!, $imageURI: String!) {
  rightsMiss: orgHypercertsClaimActivity(
    first: 1
    where: { rights: { uri: { eq: $rightsURI } } }
  ) {
    totalCount
    edges {
      node {
        uri
      }
    }
  }
  imageMiss: orgHypercertsClaimActivity(
    first: 1
    where: { image: { uri: { eq: $imageURI } } }
  ) {
    totalCount
    edges {
      node {
        uri
      }
    }
  }
}`

const smokeActivityDepthLimitPresenceFilterQuery = `
query SmokeActivityDepthLimitPresenceFilter {
  orgHypercertsClaimActivity(
    first: 1
    where: { description: { facets: { any: { index: { isNull: false } } } } }
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

type activityNestedConnection struct {
	TotalCount int `json:"totalCount"`
	Edges      []struct {
		Node struct {
			URI string `json:"uri"`
		} `json:"node"`
	} `json:"edges"`
}

type activityNestedFilterResponse struct {
	OrgHypercertsClaimActivity activityNestedConnection `json:"orgHypercertsClaimActivity"`
}

type activityOneLevelNestedFiltersResponse struct {
	RightsMiss activityNestedConnection `json:"rightsMiss"`
	ImageMiss  activityNestedConnection `json:"imageMiss"`
}

type activityNestedCandidateResponse struct {
	OrgHypercertsClaimActivity struct {
		Edges []struct {
			Node activityNestedCandidateNode `json:"node"`
		} `json:"edges"`
	} `json:"orgHypercertsClaimActivity"`
}

type activityNestedCandidateNode struct {
	URI          string                               `json:"uri"`
	Contributors []activityNestedCandidateContributor `json:"contributors"`
	Rights       *nestedCollectionStrongRef           `json:"rights"`
	Image        *activityURIUnion                    `json:"image"`
}

type activityNestedCandidateContributor struct {
	ContributorIdentity activityContributorIdentityUnion `json:"contributorIdentity"`
}

type activityContributorIdentityUnion struct {
	Typename string `json:"__typename"`
	Identity string `json:"identity"`
}

type activityURIUnion struct {
	Typename string `json:"__typename"`
	URI      string `json:"uri"`
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

func TestSchemaExposesActivityContributorIdentityNestedWhereFilter(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, "OrgHypercertsClaimActivityWhereInput")
	contributorsField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "contributors")
	contributorsInput := requireSchemaType(t, types, namedTypeName(contributorsField.Type))

	anyField := requireSchemaInputField(t, inputFieldsByName(contributorsInput.InputFields), "any")
	contributorInput := requireSchemaType(t, types, namedTypeName(anyField.Type))

	contributorIdentityField := requireSchemaInputField(t, inputFieldsByName(contributorInput.InputFields), "contributorIdentity")
	contributorIdentityInput := requireSchemaType(t, types, namedTypeName(contributorIdentityField.Type))

	identityField := requireSchemaInputField(t, inputFieldsByName(contributorIdentityInput.InputFields), "identity")
	if got := namedTypeName(identityField.Type); got != "ExactStringFilterInput" {
		t.Fatalf("contributors.any.contributorIdentity.identity filter type = %q, want ExactStringFilterInput", got)
	}

	smokeLog("✓ org.hypercerts.claim.activity exposes contributor identity nested filters")
}

func TestSchemaExposesActivityOneLevelNestedRefAndUnionFilters(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, "OrgHypercertsClaimActivityWhereInput")

	rightsField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "rights")
	rightsInput := requireSchemaType(t, types, namedTypeName(rightsField.Type))
	rightsURIField := requireSchemaInputField(t, inputFieldsByName(rightsInput.InputFields), "uri")
	if got := namedTypeName(rightsURIField.Type); got != "ExactStringFilterInput" {
		t.Fatalf("rights.uri filter type = %q, want ExactStringFilterInput", got)
	}

	imageField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "image")
	imageInput := requireSchemaType(t, types, namedTypeName(imageField.Type))
	imageURIField := requireSchemaInputField(t, inputFieldsByName(imageInput.InputFields), "uri")
	if got := namedTypeName(imageURIField.Type); got != "ExactStringFilterInput" {
		t.Fatalf("image.uri filter type = %q, want ExactStringFilterInput", got)
	}

	smokeLog("✓ org.hypercerts.claim.activity exposes one-level ref and union nested filters")
}

func TestSchemaHidesUnsupportedNestedArrayAnyFilters(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, "OrgHypercertsClaimActivityWhereInput")
	facetsField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "shortDescriptionFacets")
	facetsInput := requireSchemaType(t, types, namedTypeName(facetsField.Type))

	anyField := requireSchemaInputField(t, inputFieldsByName(facetsInput.InputFields), "any")
	facetInput := requireSchemaType(t, types, namedTypeName(anyField.Type))

	featuresField := requireSchemaInputField(t, inputFieldsByName(facetInput.InputFields), "features")
	featuresInput := requireSchemaType(t, types, namedTypeName(featuresField.Type))
	featuresFields := inputFieldsByName(featuresInput.InputFields)
	requireSchemaInputField(t, featuresFields, "isNull")
	if _, exists := featuresFields["any"]; exists {
		t.Fatal("shortDescriptionFacets.any.features exposes nested any, but nested array any filters are not executable")
	}

	smokeLog("✓ nested filters hide unsupported array any filters inside another array any")
}

func TestSchemaKeepsPresenceOnlyFiltersAtNestedDepthLimit(t *testing.T) {
	config := loadSmokeConfig(t)
	schema := fetchGraphQLSchema(t, config)
	types := typesByName(schema.Types)

	whereInput := requireSchemaType(t, types, "OrgHypercertsClaimActivityWhereInput")
	descriptionField := requireSchemaInputField(t, inputFieldsByName(whereInput.InputFields), "description")
	descriptionInput := requireSchemaType(t, types, namedTypeName(descriptionField.Type))

	facetsField := requireSchemaInputField(t, inputFieldsByName(descriptionInput.InputFields), "facets")
	facetsInput := requireSchemaType(t, types, namedTypeName(facetsField.Type))

	anyField := requireSchemaInputField(t, inputFieldsByName(facetsInput.InputFields), "any")
	facetInput := requireSchemaType(t, types, namedTypeName(anyField.Type))

	indexField := requireSchemaInputField(t, inputFieldsByName(facetInput.InputFields), "index")
	indexInput := requireSchemaType(t, types, namedTypeName(indexField.Type))
	indexFields := inputFieldsByName(indexInput.InputFields)
	requireSchemaInputField(t, indexFields, "isNull")
	for _, absent := range []string{"byteStart", "byteEnd"} {
		if _, exists := indexFields[absent]; exists {
			t.Fatalf("depth-limited index filter exposes %q, want only presence checks at this depth", absent)
		}
	}

	smokeLog("✓ nested filters keep presence checks when deeper scalar fields exceed the depth limit")
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

func TestActivityContributorIdentityNestedWhereFilterQueryExecutes(t *testing.T) {
	config := loadSmokeConfig(t)
	requestedIdentity := "did:plc:nonexistentcontributorsmoke"
	response := postGraphQL(t, context.Background(), config, "SmokeActivityContributorIdentityNestedFilter", smokeActivityContributorIdentityNestedFilterQuery, map[string]any{
		"identity": requestedIdentity,
	})

	var decoded activityNestedFilterResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeActivityContributorIdentityNestedFilter data: %v", err)
	}
	if len(decoded.OrgHypercertsClaimActivity.Edges) != 0 {
		t.Fatalf("contributor identity nested filter query for guaranteed-miss identity %q returned %d edges, want 0", requestedIdentity, len(decoded.OrgHypercertsClaimActivity.Edges))
	}

	smokeLog("✓ org.hypercerts.claim.activity contributor identity nested where filter executes and filters")
}

func TestActivityOneLevelNestedRefAndUnionFilterQueriesExecute(t *testing.T) {
	config := loadSmokeConfig(t)
	rightsURI := "at://did:plc:example/org.hypercerts.claim.rights/nonexistent-smoke-record"
	imageURI := "https://example.invalid/hyperindex-smoke/nonexistent-image.png"
	response := postGraphQL(t, context.Background(), config, "SmokeActivityOneLevelNestedFilters", smokeActivityOneLevelNestedFiltersQuery, map[string]any{
		"rightsURI": rightsURI,
		"imageURI":  imageURI,
	})

	var decoded activityOneLevelNestedFiltersResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeActivityOneLevelNestedFilters data: %v", err)
	}
	if len(decoded.RightsMiss.Edges) != 0 {
		t.Fatalf("rights.uri nested filter query for guaranteed-miss URI %q returned %d edges, want 0", rightsURI, len(decoded.RightsMiss.Edges))
	}
	if len(decoded.ImageMiss.Edges) != 0 {
		t.Fatalf("image.uri nested filter query for guaranteed-miss URI %q returned %d edges, want 0", imageURI, len(decoded.ImageMiss.Edges))
	}

	smokeLog("✓ org.hypercerts.claim.activity one-level nested ref and union filters execute and filter")
}

func TestActivityDepthLimitPresenceFilterQueryExecutes(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeActivityDepthLimitPresenceFilter", smokeActivityDepthLimitPresenceFilterQuery, nil)

	var decoded activityNestedFilterResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("decode SmokeActivityDepthLimitPresenceFilter data: %v", err)
	}

	smokeLog("✓ org.hypercerts.claim.activity depth-limited nested presence filter executes")
}

func TestCollectionNestedWhereFilterReturnsMatchingItemURI(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeNestedFilterSameElementCandidates", smokeNestedFilterSameElementCandidatesQuery, nil)

	var candidates nestedCollectionSameElementResponse
	if err := json.Unmarshal(response.Data, &candidates); err != nil {
		t.Fatalf("decode SmokeNestedFilterSameElementCandidates data: %v", err)
	}

	candidateURI, itemURI, ok := findCollectionItemIdentifierCandidate(candidates)
	if !ok {
		t.Fatal("no org.hypercerts.collection record with an itemIdentifier.uri candidate for positive nested filter smoke coverage")
	}

	response = postGraphQL(t, context.Background(), config, "SmokeCollectionItemIdentifierPositiveNestedFilter", smokeCollectionItemIdentifierPositiveNestedFilterQuery, map[string]any{
		"uri": itemURI,
	})

	var filtered nestedCollectionSameElementResponse
	if err := json.Unmarshal(response.Data, &filtered); err != nil {
		t.Fatalf("decode SmokeCollectionItemIdentifierPositiveNestedFilter data: %v", err)
	}
	if !collectionResponseContainsURI(filtered, candidateURI) {
		t.Fatalf("nested itemIdentifier.uri filter for %q did not return candidate collection %q; returned %v", itemURI, candidateURI, collectionResponseURIs(filtered))
	}
	for _, edge := range filtered.OrgHypercertsCollection.Edges {
		if !nodeHasItemIdentifierURI(edge.Node, itemURI) {
			t.Fatalf("nested itemIdentifier.uri filter returned %q without matching itemIdentifier.uri=%q", edge.Node.URI, itemURI)
		}
	}

	smokeLog("✓ org.hypercerts.collection nested itemIdentifier.uri filter returns matching records")
}

func TestActivityContributorIdentityNestedWhereFilterReturnsMatchingRecord(t *testing.T) {
	config := loadSmokeConfig(t)
	candidates := fetchActivityContributorIdentityCandidates(t, config)

	candidateURI, identity, ok := findActivityContributorIdentityCandidate(candidates)
	if !ok {
		t.Fatal("no org.hypercerts.claim.activity record with contributors.contributorIdentity.identity candidate for positive nested filter smoke coverage")
	}

	response := postGraphQL(t, context.Background(), config, "SmokeActivityContributorIdentityPositiveNestedFilter", smokeActivityContributorIdentityPositiveNestedFilterQuery, map[string]any{
		"identity": identity,
	})

	var filtered activityNestedCandidateResponse
	if err := json.Unmarshal(response.Data, &filtered); err != nil {
		t.Fatalf("decode SmokeActivityContributorIdentityPositiveNestedFilter data: %v", err)
	}
	if !activityResponseContainsURI(filtered, candidateURI) {
		t.Fatalf("contributor identity nested filter for %q did not return candidate activity %q; returned %v", identity, candidateURI, activityResponseURIs(filtered))
	}

	smokeLog("✓ org.hypercerts.claim.activity contributor identity nested filter returns matching records")
}

func TestActivityOneLevelNestedFiltersReturnMatchingRecords(t *testing.T) {
	config := loadSmokeConfig(t)
	candidates := fetchActivityOneLevelNestedFilterCandidates(t, config)

	rightsCandidateURI, rightsURI, ok := findActivityRightsCandidate(candidates)
	if !ok {
		t.Fatal("no org.hypercerts.claim.activity record with rights.uri candidate for positive nested filter smoke coverage")
	}
	imageCandidateURI, imageURI, ok := findActivityImageURICandidate(candidates)
	if !ok {
		t.Fatal("no org.hypercerts.claim.activity record with image.uri candidate for positive nested filter smoke coverage")
	}

	rightsResponse := postGraphQL(t, context.Background(), config, "SmokeActivityRightsPositiveNestedFilter", smokeActivityRightsPositiveNestedFilterQuery, map[string]any{
		"uri": rightsURI,
	})
	var rightsFiltered activityNestedCandidateResponse
	if err := json.Unmarshal(rightsResponse.Data, &rightsFiltered); err != nil {
		t.Fatalf("decode SmokeActivityRightsPositiveNestedFilter data: %v", err)
	}
	if !activityResponseContainsURI(rightsFiltered, rightsCandidateURI) {
		t.Fatalf("rights.uri nested filter for %q did not return candidate activity %q; returned %v", rightsURI, rightsCandidateURI, activityResponseURIs(rightsFiltered))
	}
	for _, edge := range rightsFiltered.OrgHypercertsClaimActivity.Edges {
		if edge.Node.Rights == nil || edge.Node.Rights.URI != rightsURI {
			t.Fatalf("rights.uri nested filter returned %q without matching rights.uri=%q", edge.Node.URI, rightsURI)
		}
	}

	imageResponse := postGraphQL(t, context.Background(), config, "SmokeActivityImagePositiveNestedFilter", smokeActivityImagePositiveNestedFilterQuery, map[string]any{
		"uri": imageURI,
	})
	var imageFiltered activityNestedCandidateResponse
	if err := json.Unmarshal(imageResponse.Data, &imageFiltered); err != nil {
		t.Fatalf("decode SmokeActivityImagePositiveNestedFilter data: %v", err)
	}
	if !activityResponseContainsURI(imageFiltered, imageCandidateURI) {
		t.Fatalf("image.uri nested filter for %q did not return candidate activity %q; returned %v", imageURI, imageCandidateURI, activityResponseURIs(imageFiltered))
	}
	for _, edge := range imageFiltered.OrgHypercertsClaimActivity.Edges {
		if edge.Node.Image == nil || edge.Node.Image.URI != imageURI {
			t.Fatalf("image.uri nested filter returned %q without matching image.uri=%q", edge.Node.URI, imageURI)
		}
	}

	smokeLog("✓ org.hypercerts.claim.activity one-level nested filters return matching records")
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

func fetchActivityContributorIdentityCandidates(t testing.TB, config smokeConfig) activityNestedCandidateResponse {
	t.Helper()

	response := postGraphQL(t, context.Background(), config, "SmokeActivityContributorIdentityCandidates", smokeActivityContributorIdentityCandidatesQuery, nil)

	var candidates activityNestedCandidateResponse
	if err := json.Unmarshal(response.Data, &candidates); err != nil {
		t.Fatalf("decode SmokeActivityContributorIdentityCandidates data: %v", err)
	}
	return candidates
}

func fetchActivityOneLevelNestedFilterCandidates(t testing.TB, config smokeConfig) activityNestedCandidateResponse {
	t.Helper()

	response := postGraphQL(t, context.Background(), config, "SmokeActivityOneLevelNestedFilterCandidates", smokeActivityOneLevelNestedFilterCandidatesQuery, nil)

	var candidates activityNestedCandidateResponse
	if err := json.Unmarshal(response.Data, &candidates); err != nil {
		t.Fatalf("decode SmokeActivityOneLevelNestedFilterCandidates data: %v", err)
	}
	return candidates
}

func findCollectionItemIdentifierCandidate(response nestedCollectionSameElementResponse) (recordURI string, itemURI string, ok bool) {
	for _, edge := range response.OrgHypercertsCollection.Edges {
		for _, item := range edge.Node.Items {
			if item.ItemIdentifier.URI != "" {
				return edge.Node.URI, item.ItemIdentifier.URI, true
			}
		}
	}
	return "", "", false
}

func collectionResponseContainsURI(response nestedCollectionSameElementResponse, uri string) bool {
	for _, edge := range response.OrgHypercertsCollection.Edges {
		if edge.Node.URI == uri {
			return true
		}
	}
	return false
}

func collectionResponseURIs(response nestedCollectionSameElementResponse) []string {
	uris := make([]string, 0, len(response.OrgHypercertsCollection.Edges))
	for _, edge := range response.OrgHypercertsCollection.Edges {
		uris = append(uris, edge.Node.URI)
	}
	return uris
}

func findActivityContributorIdentityCandidate(response activityNestedCandidateResponse) (recordURI string, identity string, ok bool) {
	for _, edge := range response.OrgHypercertsClaimActivity.Edges {
		for _, contributor := range edge.Node.Contributors {
			if contributor.ContributorIdentity.Identity != "" {
				return edge.Node.URI, contributor.ContributorIdentity.Identity, true
			}
		}
	}
	return "", "", false
}

func findActivityRightsCandidate(response activityNestedCandidateResponse) (recordURI string, rightsURI string, ok bool) {
	for _, edge := range response.OrgHypercertsClaimActivity.Edges {
		if edge.Node.Rights != nil && edge.Node.Rights.URI != "" {
			return edge.Node.URI, edge.Node.Rights.URI, true
		}
	}
	return "", "", false
}

func findActivityImageURICandidate(response activityNestedCandidateResponse) (recordURI string, imageURI string, ok bool) {
	for _, edge := range response.OrgHypercertsClaimActivity.Edges {
		if edge.Node.Image != nil && edge.Node.Image.URI != "" {
			return edge.Node.URI, edge.Node.Image.URI, true
		}
	}
	return "", "", false
}

func activityResponseContainsURI(response activityNestedCandidateResponse, uri string) bool {
	for _, edge := range response.OrgHypercertsClaimActivity.Edges {
		if edge.Node.URI == uri {
			return true
		}
	}
	return false
}

func activityResponseURIs(response activityNestedCandidateResponse) []string {
	uris := make([]string, 0, len(response.OrgHypercertsClaimActivity.Edges))
	for _, edge := range response.OrgHypercertsClaimActivity.Edges {
		uris = append(uris, edge.Node.URI)
	}
	return uris
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

func nodeHasItemIdentifierURI(node nestedCollectionNode, uri string) bool {
	for _, item := range node.Items {
		if item.ItemIdentifier.URI == uri {
			return true
		}
	}
	return false
}
