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

type externalLabelActivityClaimsConnection struct {
	TotalCount int                              `json:"totalCount"`
	Edges      []externalLabelActivityClaimEdge `json:"edges"`
	PageInfo   PageInfo                         `json:"pageInfo"`
}

type externalLabelActivityClaimEdge struct {
	Cursor string                         `json:"cursor"`
	Node   externalLabelActivityClaimNode `json:"node"`
}

type externalLabelActivityClaimNode struct {
	URI            string               `json:"uri"`
	CID            string               `json:"cid"`
	DID            string               `json:"did"`
	RKey           string               `json:"rkey"`
	ExternalLabels []smokeExternalLabel `json:"externalLabels"`
}

type smokeExternalLabel struct {
	Src string  `json:"src"`
	URI string  `json:"uri"`
	CID *string `json:"cid"`
	Val string  `json:"val"`
	Neg bool    `json:"neg"`
	Cts string  `json:"cts"`
	Exp *string `json:"exp"`
	Ver *int    `json:"ver"`
}

func TestExternalLabelActivityClaimsSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	expectation := config.expectations.ExternalLabelActivityClaims
	if !expectation.configured() {
		t.Skip("externalLabelActivityClaims is not configured in the smoke expectations file")
	}

	sourceDID := strings.TrimSpace(os.Getenv(expectation.SourceDIDEnv))
	if sourceDID == "" {
		t.Skipf("%s is unset; skipping external label activity claims smoke test", expectation.SourceDIDEnv)
	}
	if !strings.HasPrefix(sourceDID, "did:") {
		t.Fatalf("%s = %q, want a DID starting with did:", expectation.SourceDIDEnv, sourceDID)
	}

	typedField := config.expectations.TypedQueryFields[externalLabelActivityClaimsCollection]
	if typedField == "" {
		t.Fatalf("external label activity claims smoke test requires typedQueryFields[%q]", externalLabelActivityClaimsCollection)
	}

	ctx := context.Background()
	for _, label := range expectation.Labels {
		label := label
		t.Run(label.Value, func(t *testing.T) {
			firstPage := queryExternalLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, "", sourceDID, label.Value)
			if firstPage.TotalCount < label.MinimumRecords {
				t.Fatalf("external label activity claims count failed for source DID %q and value %q: totalCount = %d, want at least %d %s records", sourceDID, label.Value, firstPage.TotalCount, label.MinimumRecords, externalLabelActivityClaimsCollection)
			}

			pageOneURIs := assertExternalLabelActivityClaimsPage(t, "first page", firstPage, expectation.PageSize, "", sourceDID, label.Value)
			if !firstPage.PageInfo.HasNextPage {
				t.Fatalf("external label activity claims pagination failed for source DID %q and value %q: first page hasNextPage = false, want true (totalCount=%d pageSize=%d endCursor=%q)", sourceDID, label.Value, firstPage.TotalCount, expectation.PageSize, firstPage.PageInfo.EndCursor)
			}
			if firstPage.PageInfo.EndCursor == "" {
				t.Fatalf("external label activity claims pagination failed for source DID %q and value %q: first page endCursor is empty", sourceDID, label.Value)
			}

			secondPage := queryExternalLabelActivityClaimsPage(t, ctx, config, typedField, expectation.PageSize, firstPage.PageInfo.EndCursor, sourceDID, label.Value)
			pageTwoURIs := assertExternalLabelActivityClaimsPage(t, "second page", secondPage, expectation.PageSize, firstPage.PageInfo.EndCursor, sourceDID, label.Value)
			if !secondPage.PageInfo.HasPreviousPage {
				t.Fatalf("external label activity claims pagination failed for source DID %q and value %q: second page hasPreviousPage = false, want true (after=%q)", sourceDID, label.Value, firstPage.PageInfo.EndCursor)
			}
			for uri := range pageTwoURIs {
				if pageOneURIs[uri] {
					t.Fatalf("external label activity claims pagination failed for source DID %q and value %q: duplicate URI %q across adjacent pages (firstPageEndCursor=%q secondPageEndCursor=%q)", sourceDID, label.Value, uri, firstPage.PageInfo.EndCursor, secondPage.PageInfo.EndCursor)
				}
			}

			rootLabels := queryRootExternalLabelsForSubject(t, ctx, config, firstPage.Edges[0].Node.URI, sourceDID, label.Value)
			assertExternalLabelSetContains(t, "root externalLabels lookup", rootLabels, firstPage.Edges[0].Node.URI, sourceDID, label.Value)

			smokeLog("✓ external label %q activity claims from %s have at least %d records and paginate", label.Value, sourceDID, label.MinimumRecords)
		})
	}
}

func queryExternalLabelActivityClaimsPage(t testing.TB, ctx context.Context, config smokeConfig, typedField string, first int, after string, sourceDID string, value string) externalLabelActivityClaimsConnection {
	t.Helper()

	variables := map[string]any{
		"first":     first,
		"sourceDID": sourceDID,
		"value":     value,
	}
	if after != "" {
		variables["after"] = after
	}

	query := fmt.Sprintf(`
		query ExternalLabelActivityClaimsSmoke($first: Int!, $after: String, $sourceDID: String!, $value: String!) {
			%s(
				first: $first
				after: $after
				where: { externalLabels: { has: { src: { eq: $sourceDID }, val: { eq: $value } } } }
			) {
				totalCount
				edges {
					cursor
					node {
						uri
						cid
						did
						rkey
						externalLabels(sources: [$sourceDID], values: [$value]) {
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
				}
				pageInfo {
					hasNextPage
					hasPreviousPage
					startCursor
					endCursor
				}
			}
		}
	`, typedField)

	response := postGraphQL(t, ctx, config, "ExternalLabelActivityClaimsSmoke", query, variables)

	var decoded map[string]externalLabelActivityClaimsConnection
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("external label activity claims: decode response for source DID %q value %q after %q: %v", sourceDID, value, after, err)
	}

	connection, ok := decoded[typedField]
	if !ok {
		t.Fatalf("external label activity claims: response missing typed field %q for source DID %q value %q after %q", typedField, sourceDID, value, after)
	}
	return connection
}

func assertExternalLabelActivityClaimsPage(t testing.TB, pageName string, page externalLabelActivityClaimsConnection, expectedEdges int, after string, sourceDID string, value string) map[string]bool {
	t.Helper()

	if len(page.Edges) != expectedEdges {
		t.Fatalf("external label activity claims %s size failed for source DID %q and value %q: returned %d edges, want %d (after=%q totalCount=%d endCursor=%q sampleURI=%q)", pageName, sourceDID, value, len(page.Edges), expectedEdges, after, page.TotalCount, page.PageInfo.EndCursor, sampleExternalLabelActivityClaimURI(page))
	}

	seenURIs := make(map[string]bool, len(page.Edges))
	for edgeIndex, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("external label activity claims %s cursor failed for source DID %q and value %q: edge %d cursor is empty (after=%q uri=%q)", pageName, sourceDID, value, edgeIndex, after, edge.Node.URI)
		}
		assertExternalLabelActivityClaimNode(t, pageName, edgeIndex, edge.Node)
		assertExternalLabelSetContains(t, pageName, edge.Node.ExternalLabels, edge.Node.URI, sourceDID, value)
		if seenURIs[edge.Node.URI] {
			t.Fatalf("external label activity claims %s duplicate URI failed for source DID %q and value %q: duplicate URI %q", pageName, sourceDID, value, edge.Node.URI)
		}
		seenURIs[edge.Node.URI] = true
	}

	return seenURIs
}

func sampleExternalLabelActivityClaimURI(page externalLabelActivityClaimsConnection) string {
	if len(page.Edges) == 0 {
		return ""
	}
	return page.Edges[0].Node.URI
}

func assertExternalLabelActivityClaimNode(t testing.TB, pageName string, edgeIndex int, node externalLabelActivityClaimNode) {
	t.Helper()

	location := fmt.Sprintf("%s edge %d uri=%q did=%q", pageName, edgeIndex, node.URI, node.DID)
	if !strings.HasPrefix(node.URI, "at://") {
		t.Fatalf("external label activity claim node %s: uri want at:// prefix", location)
	}
	if !strings.Contains(node.URI, "/"+externalLabelActivityClaimsCollection+"/") {
		t.Fatalf("external label activity claim node %s: uri want to contain collection segment %q", location, "/"+externalLabelActivityClaimsCollection+"/")
	}
	if !strings.HasPrefix(node.DID, "did:") {
		t.Fatalf("external label activity claim node %s: did want did: prefix", location)
	}
	if node.CID == "" {
		t.Fatalf("external label activity claim node %s: cid is empty", location)
	}
	if node.RKey == "" {
		t.Fatalf("external label activity claim node %s: rkey is empty", location)
	}
}

func queryRootExternalLabelsForSubject(t testing.TB, ctx context.Context, config smokeConfig, uri string, sourceDID string, value string) []smokeExternalLabel {
	t.Helper()

	response := postGraphQL(t, ctx, config, "ExternalLabelsRootSmoke", `
		query ExternalLabelsRootSmoke($uri: String!, $sourceDID: String!, $value: String!) {
			externalLabels(subjects: [$uri], sources: [$sourceDID], values: [$value]) {
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
		"uri":       uri,
		"sourceDID": sourceDID,
		"value":     value,
	})

	var decoded struct {
		ExternalLabels []smokeExternalLabel `json:"externalLabels"`
	}
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("root externalLabels lookup: decode response for uri %q source DID %q value %q: %v", uri, sourceDID, value, err)
	}
	return decoded.ExternalLabels
}

func assertExternalLabelSetContains(t testing.TB, location string, labels []smokeExternalLabel, recordURI string, sourceDID string, value string) {
	t.Helper()

	for _, label := range labels {
		if label.Src == sourceDID && label.URI == recordURI && label.Val == value && !label.Neg {
			return
		}
	}

	t.Fatalf("%s externalLabels missing active label for uri %q source DID %q value %q; labels=%s", location, recordURI, sourceDID, value, formatSmokeExternalLabels(labels))
}

func formatSmokeExternalLabels(labels []smokeExternalLabel) string {
	payload, err := json.Marshal(labels)
	if err != nil {
		return fmt.Sprintf("%+v", labels)
	}
	return string(payload)
}
