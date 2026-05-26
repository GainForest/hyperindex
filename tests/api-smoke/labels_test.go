//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const (
	smokeActivityClaimLabelSource = "did:plc:edod7rboajioq3jbyxsgeicc"
	smokeActivityClaimLabelValue  = "likely-test"
	smokeAbsentActivityLabelValue = "not-likely-test"
)

const smokeActivityClaimLabelsQuery = `
query SmokeActivityClaimLabels($first: Int!, $after: String, $value: String!, $source: String!) {
  orgHypercertsClaimActivity(
    first: $first
    after: $after
    where: { externalLabels: { has: { src: { eq: $source }, val: { eq: $value } } } }
  ) {
    edges {
      cursor
      node {
        uri
        cid
        externalLabels(sources: [$source], values: [$value]) {
          src
          uri
          cid
          val
          neg
          cts
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`

type activityClaimLabelsResponse struct {
	ActivityClaims externalLabelRecordConnection `json:"orgHypercertsClaimActivity"`
}

type externalLabelRecordConnection struct {
	Edges    []externalLabelRecordEdge `json:"edges"`
	PageInfo PageInfo                  `json:"pageInfo"`
}

type externalLabelRecordEdge struct {
	Cursor string                  `json:"cursor"`
	Node   externalLabelRecordNode `json:"node"`
}

type externalLabelRecordNode struct {
	URI            string          `json:"uri"`
	CID            string          `json:"cid"`
	ExternalLabels []externalLabel `json:"externalLabels"`
}

type externalLabel struct {
	Src string  `json:"src"`
	URI string  `json:"uri"`
	CID *string `json:"cid"`
	Val string  `json:"val"`
	Neg bool    `json:"neg"`
	CTS string  `json:"cts"`
}

func TestActivityClaimExternalLabelPaginationSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	firstPage := queryActivityClaimLabelPage(t, ctx, config, 2, "", smokeActivityClaimLabelValue)
	firstPageURIs := assertActivityClaimLabelPage(t, "first page", firstPage, 2, smokeActivityClaimLabelValue)
	if !firstPage.PageInfo.HasNextPage {
		t.Fatalf("activity claim label pagination: first page hasNextPage = false, want true for label value %q", smokeActivityClaimLabelValue)
	}
	if firstPage.PageInfo.EndCursor == "" {
		t.Fatalf("activity claim label pagination: first page endCursor is empty for label value %q", smokeActivityClaimLabelValue)
	}

	secondPage := queryActivityClaimLabelPage(t, ctx, config, 2, firstPage.PageInfo.EndCursor, smokeActivityClaimLabelValue)
	secondPageURIs := assertActivityClaimLabelPage(t, "second page", secondPage, 2, smokeActivityClaimLabelValue)
	for uri := range secondPageURIs {
		if firstPageURIs[uri] {
			t.Fatalf("activity claim label pagination returned duplicate URI %q across adjacent pages", uri)
		}
	}

	smokeLog("✓ activity claim external label querying paginates for source %s", smokeActivityClaimLabelSource)
}

func TestExternalLabelValueFiltersSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	page := queryActivityClaimLabelPage(t, ctx, config, 1, "", smokeActivityClaimLabelValue)
	uris := assertActivityClaimLabelPage(t, "subject lookup page", page, 1, smokeActivityClaimLabelValue)
	var subject string
	for uri := range uris {
		subject = uri
		break
	}

	matchingLabels := queryRootExternalLabels(t, ctx, config, subject, smokeActivityClaimLabelValue)
	if len(matchingLabels) == 0 {
		t.Fatalf("root externalLabels for subject %q and value %q returned no labels", subject, smokeActivityClaimLabelValue)
	}
	for index, label := range matchingLabels {
		assertMatchingActivityClaimLabel(t, "root externalLabels", subject, "", smokeActivityClaimLabelValue, index, label)
	}

	absentLabels := queryRootExternalLabels(t, ctx, config, subject, smokeAbsentActivityLabelValue)
	if len(absentLabels) != 0 {
		t.Fatalf("root externalLabels for subject %q and absent value %q returned %d labels, want 0", subject, smokeAbsentActivityLabelValue, len(absentLabels))
	}

	smokeLog("✓ external label value filters distinguish %q from %q", smokeActivityClaimLabelValue, smokeAbsentActivityLabelValue)
}

func queryActivityClaimLabelPage(t testing.TB, ctx context.Context, config smokeConfig, first int, after string, value string) externalLabelRecordConnection {
	t.Helper()

	variables := map[string]any{
		"first":  first,
		"source": smokeActivityClaimLabelSource,
		"value":  value,
	}
	if after != "" {
		variables["after"] = after
	}

	response := postGraphQL(t, ctx, config, "SmokeActivityClaimLabels", smokeActivityClaimLabelsQuery, variables)

	var data activityClaimLabelsResponse
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatalf("decode SmokeActivityClaimLabels data for value %q: %v", value, err)
	}

	return data.ActivityClaims
}

func assertActivityClaimLabelPage(t testing.TB, pageName string, page externalLabelRecordConnection, expectedEdges int, value string) map[string]bool {
	t.Helper()

	if len(page.Edges) != expectedEdges {
		t.Fatalf("activity claim label %s returned %d edges, want %d for value %q", pageName, len(page.Edges), expectedEdges, value)
	}

	seenURIs := make(map[string]bool, len(page.Edges))
	for edgeIndex, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("activity claim label %s edge %d cursor is empty", pageName, edgeIndex)
		}
		if edge.Node.URI == "" {
			t.Fatalf("activity claim label %s edge %d node.uri is empty", pageName, edgeIndex)
		}
		if edge.Node.CID == "" {
			t.Fatalf("activity claim label %s edge %d node.cid is empty for uri %q", pageName, edgeIndex, edge.Node.URI)
		}
		if seenURIs[edge.Node.URI] {
			t.Fatalf("activity claim label %s returned duplicate URI %q", pageName, edge.Node.URI)
		}
		seenURIs[edge.Node.URI] = true
		if len(edge.Node.ExternalLabels) == 0 {
			t.Fatalf("activity claim label %s edge %d uri %q returned no externalLabels for source %q and value %q", pageName, edgeIndex, edge.Node.URI, smokeActivityClaimLabelSource, value)
		}
		for labelIndex, label := range edge.Node.ExternalLabels {
			assertMatchingActivityClaimLabel(t, pageName, edge.Node.URI, edge.Node.CID, value, labelIndex, label)
		}
	}

	return seenURIs
}

func queryRootExternalLabels(t testing.TB, ctx context.Context, config smokeConfig, subject string, value string) []externalLabel {
	t.Helper()

	response := postGraphQL(t, ctx, config, "SmokeRootExternalLabels", `
		query SmokeRootExternalLabels($subject: String!, $source: String!, $value: String!) {
			externalLabels(subjects: [$subject], sources: [$source], values: [$value]) {
				src
				uri
				cid
				val
				neg
				cts
			}
		}
	`, map[string]any{
		"subject": subject,
		"source":  smokeActivityClaimLabelSource,
		"value":   value,
	})

	var data struct {
		ExternalLabels []externalLabel `json:"externalLabels"`
	}
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatalf("decode SmokeRootExternalLabels data for subject %q and value %q: %v", subject, value, err)
	}

	return data.ExternalLabels
}

func assertMatchingActivityClaimLabel(t testing.TB, location string, uri string, cid string, value string, labelIndex int, label externalLabel) {
	t.Helper()

	if label.Src != smokeActivityClaimLabelSource {
		t.Fatalf("%s label %d src = %q, want %q", location, labelIndex, label.Src, smokeActivityClaimLabelSource)
	}
	if label.URI != uri {
		t.Fatalf("%s label %d uri = %q, want %q", location, labelIndex, label.URI, uri)
	}
	if label.CID != nil && cid != "" && *label.CID != cid {
		t.Fatalf("%s label %d cid = %q, want null or node cid %q", location, labelIndex, *label.CID, cid)
	}
	if label.Val != value {
		t.Fatalf("%s label %d val = %q, want %q", location, labelIndex, label.Val, value)
	}
	if label.Neg {
		t.Fatalf("%s label %d neg = true, want active positive label", location, labelIndex)
	}
	if label.CTS == "" {
		t.Fatalf("%s label %d cts is empty", location, labelIndex)
	}
}
