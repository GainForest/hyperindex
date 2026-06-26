//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type authorLabelActivityClaimsConnection struct {
	TotalCount int                            `json:"totalCount"`
	Edges      []authorLabelActivityClaimEdge `json:"edges"`
	PageInfo   PageInfo                       `json:"pageInfo"`
}

type authorLabelActivityClaimEdge struct {
	Cursor string                       `json:"cursor"`
	Node   authorLabelActivityClaimNode `json:"node"`
}

type authorLabelActivityClaimNode struct {
	URI  string `json:"uri"`
	CID  string `json:"cid"`
	DID  string `json:"did"`
	RKey string `json:"rkey"`
}

func (e authorLabelActivityClaimsExpectation) resolvedSourceDID() (string, error) {
	if e.SourceDIDEnv != "" {
		if sourceDID := strings.TrimSpace(os.Getenv(e.SourceDIDEnv)); sourceDID != "" {
			if !strings.HasPrefix(sourceDID, "did:") {
				return "", fmt.Errorf("%s = %q, want a DID starting with did prefix", e.SourceDIDEnv, sourceDID)
			}
			return sourceDID, nil
		}
		if e.SourceDID == "" {
			return "", fmt.Errorf("%s is unset and authorLabelActivityClaims.sourceDID is empty", e.SourceDIDEnv)
		}
	}

	sourceDID := strings.TrimSpace(e.SourceDID)
	if sourceDID == "" {
		return "", fmt.Errorf("authorLabelActivityClaims.sourceDID is required")
	}
	if !strings.HasPrefix(sourceDID, "did:") {
		return "", fmt.Errorf("authorLabelActivityClaims.sourceDID = %q, want a DID starting with did prefix", sourceDID)
	}
	return sourceDID, nil
}

func TestAuthorLabelActivityClaimsSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	expectation := config.expectations.AuthorLabelActivityClaims
	if !expectation.configured() {
		t.Skip("authorLabelActivityClaims is not configured in the smoke expectations file")
	}

	sourceDID, err := expectation.resolvedSourceDID()
	if err != nil {
		t.Fatalf("author label activity claims smoke test source DID: %v", err)
	}

	typedField := config.expectations.TypedQueryFields[externalLabelActivityClaimsCollection]
	if typedField == "" {
		t.Fatalf("author label activity claims smoke test requires typedQueryFields[%q]", externalLabelActivityClaimsCollection)
	}

	ctx := context.Background()
	baseline := queryAuthorLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, "", nil)
	if len(baseline.Edges) == 0 {
		t.Fatalf("author label baseline query returned no %s records", externalLabelActivityClaimsCollection)
	}

	for _, label := range expectation.Labels {
		label := label
		t.Run("has_"+label.Value, func(t *testing.T) {
			filter := authorLabelFilterSpec{
				Kind:      "hasEq",
				SourceDID: sourceDID,
				Value:     label.Value,
			}
			page := queryAuthorLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, "", &filter)
			if page.TotalCount < label.MinimumRecords {
				t.Fatalf("author label count failed for source DID %q and value %q: totalCount = %d, want at least %d %s records", sourceDID, label.Value, page.TotalCount, label.MinimumRecords, externalLabelActivityClaimsCollection)
			}
			nodes := assertAuthorLabelActivityClaimsPage(t, "authorLabels.has first page", page, expectation.PageSize, "", sourceDID)
			assertAuthorDIDsHaveAnyLabel(t, ctx, config, sourceDID, []string{label.Value}, nodes)
			smokeLog("✓ author label has %q activity claims from %s returned at least %d records", label.Value, sourceDID, label.MinimumRecords)
		})
	}

	t.Run("none_"+expectation.NoneValue, func(t *testing.T) {
		filter := authorLabelFilterSpec{
			Kind:      "noneEq",
			SourceDID: sourceDID,
			Value:     expectation.NoneValue,
		}
		page := queryAuthorLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, "", &filter)
		nodes := assertAuthorLabelActivityClaimsPage(t, "authorLabels.none first page", page, expectation.PageSize, "", sourceDID)
		assertAuthorDIDsHaveNoLabel(t, ctx, config, sourceDID, expectation.NoneValue, nodes)
		smokeLog("✓ author label none %q activity claims from %s exclude matching authors", expectation.NoneValue, sourceDID)
	})

	t.Run("has_in_pagination", func(t *testing.T) {
		filter := authorLabelFilterSpec{
			Kind:      "hasIn",
			SourceDID: sourceDID,
			Values:    expectation.MultipleHasValues,
		}
		firstPage := queryAuthorLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, "", &filter)
		if firstPage.TotalCount < expectation.MultipleHasMinimumRecords {
			t.Fatalf("author label count failed for source DID %q and values %v: totalCount = %d, want at least %d %s records", sourceDID, expectation.MultipleHasValues, firstPage.TotalCount, expectation.MultipleHasMinimumRecords, externalLabelActivityClaimsCollection)
		}
		pageOneNodes := assertAuthorLabelActivityClaimsPage(t, "authorLabels.has in first page", firstPage, expectation.PageSize, "", sourceDID)
		assertAuthorDIDsHaveAnyLabel(t, ctx, config, sourceDID, expectation.MultipleHasValues, pageOneNodes)
		if !firstPage.PageInfo.HasNextPage {
			t.Fatalf("author label pagination failed for source DID %q and values %v: first page hasNextPage = false, want true (totalCount=%d pageSize=%d endCursor=%q)", sourceDID, expectation.MultipleHasValues, firstPage.TotalCount, expectation.PageSize, firstPage.PageInfo.EndCursor)
		}
		if firstPage.PageInfo.EndCursor == "" {
			t.Fatalf("author label pagination failed for source DID %q and values %v: first page endCursor is empty", sourceDID, expectation.MultipleHasValues)
		}

		secondPage := queryAuthorLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, firstPage.PageInfo.EndCursor, &filter)
		pageTwoNodes := assertAuthorLabelActivityClaimsPage(t, "authorLabels.has in second page", secondPage, expectation.PageSize, firstPage.PageInfo.EndCursor, sourceDID)
		assertAuthorDIDsHaveAnyLabel(t, ctx, config, sourceDID, expectation.MultipleHasValues, pageTwoNodes)
		if !secondPage.PageInfo.HasPreviousPage {
			t.Fatalf("author label pagination failed for source DID %q and values %v: second page hasPreviousPage = false, want true (after=%q)", sourceDID, expectation.MultipleHasValues, firstPage.PageInfo.EndCursor)
		}
		for uri := range pageTwoNodes {
			if pageOneNodes[uri] != "" {
				t.Fatalf("author label pagination returned duplicate URI %q across adjacent pages (firstPageEndCursor=%q secondPageEndCursor=%q)", uri, firstPage.PageInfo.EndCursor, secondPage.PageInfo.EndCursor)
			}
		}

		smokeLog("✓ author label has-in activity claims from %s have at least %d records and paginate", sourceDID, expectation.MultipleHasMinimumRecords)
	})
}

type authorLabelFilterSpec struct {
	Kind      string
	SourceDID string
	Value     string
	Values    []string
}

func queryAuthorLabelActivityClaimsPage(t testing.TB, ctx context.Context, config smokeConfig, typedField string, first int, after string, filter *authorLabelFilterSpec) authorLabelActivityClaimsConnection {
	t.Helper()

	variables := map[string]any{"first": first}
	if after != "" {
		variables["after"] = after
	}

	variableDefs := "$first: Int!, $after: String"
	whereClause := ""
	if filter != nil {
		variables["sourceDID"] = filter.SourceDID
		switch filter.Kind {
		case "hasEq":
			variables["value"] = filter.Value
			variableDefs += ", $sourceDID: String!, $value: String!"
			whereClause = "where: { authorLabels: { has: { src: { eq: $sourceDID }, val: { eq: $value } } } }"
		case "noneEq":
			variables["value"] = filter.Value
			variableDefs += ", $sourceDID: String!, $value: String!"
			whereClause = "where: { authorLabels: { none: { src: { eq: $sourceDID }, val: { eq: $value } } } }"
		case "hasIn":
			variables["values"] = filter.Values
			variableDefs += ", $sourceDID: String!, $values: [String!]!"
			whereClause = "where: { authorLabels: { has: { src: { eq: $sourceDID }, val: { in: $values } } } }"
		default:
			t.Fatalf("unknown author label filter kind %q", filter.Kind)
		}
	}

	query := fmt.Sprintf(`
		query AuthorLabelActivityClaimsSmoke(%s) {
			%s(
				first: $first
				after: $after
				%s
			) {
				totalCount
				edges {
					cursor
					node {
						uri
						cid
						did
						rkey
					}
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					startCursor
					endCursor
				}
			}
		}
	`, variableDefs, typedField, whereClause)

	response := postGraphQL(t, ctx, config, "AuthorLabelActivityClaimsSmoke", query, variables)

	var decoded map[string]authorLabelActivityClaimsConnection
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("author label activity claims: decode response after %q: %v", after, err)
	}

	connection, ok := decoded[typedField]
	if !ok {
		t.Fatalf("author label activity claims: response missing typed field %q after %q", typedField, after)
	}
	return connection
}

func assertAuthorLabelActivityClaimsPage(t testing.TB, pageName string, page authorLabelActivityClaimsConnection, expectedEdges int, after string, sourceDID string) map[string]string {
	t.Helper()

	if len(page.Edges) != expectedEdges {
		t.Fatalf("author label activity claims %s size failed for source DID %q: returned %d edges, want %d (after=%q totalCount=%d endCursor=%q sampleURI=%q)", pageName, sourceDID, len(page.Edges), expectedEdges, after, page.TotalCount, page.PageInfo.EndCursor, sampleAuthorLabelActivityClaimURI(page))
	}

	nodesByURI := make(map[string]string, len(page.Edges))
	for edgeIndex, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("author label activity claims %s cursor failed for source DID %q: edge %d cursor is empty (after=%q uri=%q)", pageName, sourceDID, edgeIndex, after, edge.Node.URI)
		}
		assertAuthorLabelActivityClaimNode(t, pageName, edgeIndex, edge.Node)
		if nodesByURI[edge.Node.URI] != "" {
			t.Fatalf("author label activity claims %s duplicate URI failed for source DID %q: duplicate URI %q", pageName, sourceDID, edge.Node.URI)
		}
		nodesByURI[edge.Node.URI] = edge.Node.DID
	}

	return nodesByURI
}

func sampleAuthorLabelActivityClaimURI(page authorLabelActivityClaimsConnection) string {
	if len(page.Edges) == 0 {
		return ""
	}
	return page.Edges[0].Node.URI
}

func assertAuthorLabelActivityClaimNode(t testing.TB, pageName string, edgeIndex int, node authorLabelActivityClaimNode) {
	t.Helper()

	location := fmt.Sprintf("%s edge %d uri=%q did=%q", pageName, edgeIndex, node.URI, node.DID)
	if !strings.HasPrefix(node.URI, "at://") {
		t.Fatalf("author label activity claim node %s: uri want at:// prefix", location)
	}
	if !strings.Contains(node.URI, "/"+externalLabelActivityClaimsCollection+"/") {
		t.Fatalf("author label activity claim node %s: uri want to contain collection segment %q", location, "/"+externalLabelActivityClaimsCollection+"/")
	}
	if !strings.HasPrefix(node.DID, "did:") {
		t.Fatalf("author label activity claim node %s: did want did: prefix", location)
	}
	if node.CID == "" {
		t.Fatalf("author label activity claim node %s: cid is empty", location)
	}
	if node.RKey == "" {
		t.Fatalf("author label activity claim node %s: rkey is empty", location)
	}
}

func assertAuthorDIDsHaveAnyLabel(t testing.TB, ctx context.Context, config smokeConfig, sourceDID string, values []string, nodesByURI map[string]string) {
	t.Helper()

	for uri, did := range nodesByURI {
		labels := queryRootExternalLabelsForAuthor(t, ctx, config, did, sourceDID, values)
		matched := false
		for _, label := range labels {
			if label.Src == sourceDID && label.URI == did && label.CID == nil && !label.Neg && stringInSlice(label.Val, values) {
				matched = true
				break
			}
		}
		if !matched {
			t.Fatalf("author label lookup for record %q author %q source DID %q values %v returned no active matching DID-subject label; labels=%s", uri, did, sourceDID, values, formatSmokeExternalLabels(labels))
		}
	}
}

func assertAuthorDIDsHaveNoLabel(t testing.TB, ctx context.Context, config smokeConfig, sourceDID string, value string, nodesByURI map[string]string) {
	t.Helper()

	for uri, did := range nodesByURI {
		labels := queryRootExternalLabelsForAuthor(t, ctx, config, did, sourceDID, []string{value})
		for _, label := range labels {
			if label.Src == sourceDID && label.URI == did && label.CID == nil && label.Val == value && !label.Neg {
				t.Fatalf("author label none filter returned record %q by author %q with active %q DID-subject label; labels=%s", uri, did, value, formatSmokeExternalLabels(labels))
			}
		}
	}
}

func queryRootExternalLabelsForAuthor(t testing.TB, ctx context.Context, config smokeConfig, did string, sourceDID string, values []string) []smokeExternalLabel {
	t.Helper()

	response := postGraphQL(t, ctx, config, "AuthorLabelsRootSmoke", `
		query AuthorLabelsRootSmoke($did: String!, $sourceDID: String!, $values: [String!]!) {
			externalLabels(subjects: [$did], sources: [$sourceDID], values: $values) {
				src
				uri
				cid
				val
				neg
				cts
				exp
				ver
			}
		}
	`, map[string]any{
		"did":       did,
		"sourceDID": sourceDID,
		"values":    values,
	})

	var decoded struct {
		ExternalLabels []smokeExternalLabel `json:"externalLabels"`
	}
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("author root externalLabels lookup: decode response for did %q source DID %q values %v: %v", did, sourceDID, values, err)
	}
	return decoded.ExternalLabels
}

func stringInSlice(value string, values []string) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
