//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

const (
	certifiedGraphFollowCollection = "app.certified.graph.follow"
	certifiedGraphFollowTypedField = "appCertifiedGraphFollow"
	certifiedGraphFollowPageSize   = 5
)

type certifiedGraphFollowData struct {
	AppCertifiedGraphFollow certifiedGraphFollowConnection `json:"appCertifiedGraphFollow"`
}

type certifiedGraphFollowConnection struct {
	Edges      []certifiedGraphFollowEdge `json:"edges"`
	PageInfo   PageInfo                   `json:"pageInfo"`
	TotalCount int                        `json:"totalCount"`
}

type certifiedGraphFollowEdge struct {
	Cursor string                     `json:"cursor"`
	Node   certifiedGraphFollowRecord `json:"node"`
}

type certifiedGraphFollowRecord struct {
	URI       string `json:"uri"`
	CID       string `json:"cid"`
	DID       string `json:"did"`
	RKey      string `json:"rkey"`
	Subject   string `json:"subject"`
	CreatedAt string `json:"createdAt"`
}

type certifiedGraphFollowPageRequest struct {
	first         int
	after         string
	where         map[string]any
	sortBy        string
	sortDirection string
}

func TestCertifiedGraphFollowSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	typedField, ok := config.expectations.TypedQueryFields[certifiedGraphFollowCollection]
	if !ok {
		t.Fatalf("smoke expectations are missing typed field for %q", certifiedGraphFollowCollection)
	}
	if typedField != certifiedGraphFollowTypedField {
		t.Fatalf("smoke expectations typed field for %q = %q, want %q", certifiedGraphFollowCollection, typedField, certifiedGraphFollowTypedField)
	}

	minimumRecords := minimumExpectedRecordsForCollection(t, config, certifiedGraphFollowCollection)
	firstPage := queryCertifiedGraphFollowPage(t, ctx, config, certifiedGraphFollowPageRequest{
		first:         certifiedGraphFollowPageSize,
		sortBy:        "indexed_at",
		sortDirection: "DESC",
	})
	firstPageURIs := assertCertifiedGraphFollowPage(t, "first page", firstPage, certifiedGraphFollowPageSize)
	if firstPage.TotalCount < minimumRecords {
		t.Fatalf("%s totalCount = %d, want at least %d", certifiedGraphFollowTypedField, firstPage.TotalCount, minimumRecords)
	}
	if !firstPage.PageInfo.HasNextPage {
		t.Fatalf("%s first page hasNextPage = false, want true", certifiedGraphFollowTypedField)
	}
	if firstPage.PageInfo.EndCursor == "" {
		t.Fatalf("%s first page endCursor is empty", certifiedGraphFollowTypedField)
	}

	secondPage := queryCertifiedGraphFollowPage(t, ctx, config, certifiedGraphFollowPageRequest{
		first:         certifiedGraphFollowPageSize,
		after:         firstPage.PageInfo.EndCursor,
		sortBy:        "indexed_at",
		sortDirection: "DESC",
	})
	secondPageURIs := assertCertifiedGraphFollowPage(t, "second page", secondPage, certifiedGraphFollowPageSize)
	for uri := range secondPageURIs {
		if firstPageURIs[uri] {
			t.Fatalf("%s pagination returned duplicate URI %q across adjacent pages", certifiedGraphFollowTypedField, uri)
		}
	}

	sample := firstPage.Edges[0].Node
	assertCertifiedGraphFollowFilter(t, ctx, config, "did eq filter", map[string]any{
		"did": map[string]any{"eq": sample.DID},
	}, func(record certifiedGraphFollowRecord) bool {
		return record.DID == sample.DID
	})
	assertCertifiedGraphFollowFilter(t, ctx, config, "subject eq filter", map[string]any{
		"subject": map[string]any{"eq": sample.Subject},
	}, func(record certifiedGraphFollowRecord) bool {
		return record.Subject == sample.Subject
	})

	combinedFilterPage := queryCertifiedGraphFollowPage(t, ctx, config, certifiedGraphFollowPageRequest{
		first: certifiedGraphFollowPageSize,
		where: map[string]any{
			"did":     map[string]any{"eq": sample.DID},
			"subject": map[string]any{"eq": sample.Subject},
		},
	})
	assertCertifiedGraphFollowPageAtLeast(t, "did+subject filter", combinedFilterPage, 1)
	assertCertifiedGraphFollowContainsURI(t, "did+subject filter", combinedFilterPage, sample.URI)
	for _, edge := range combinedFilterPage.Edges {
		if edge.Node.DID != sample.DID || edge.Node.Subject != sample.Subject {
			t.Fatalf("%s did+subject filter returned uri=%q did=%q subject=%q, want did=%q subject=%q", certifiedGraphFollowTypedField, edge.Node.URI, edge.Node.DID, edge.Node.Subject, sample.DID, sample.Subject)
		}
	}

	subjectSortedPage := queryCertifiedGraphFollowPage(t, ctx, config, certifiedGraphFollowPageRequest{
		first:         certifiedGraphFollowPageSize,
		sortBy:        "subject",
		sortDirection: "ASC",
	})
	assertCertifiedGraphFollowPage(t, "subject sorted page", subjectSortedPage, certifiedGraphFollowPageSize)
	assertCertifiedGraphFollowSubjectsSorted(t, subjectSortedPage)

	typedRecord := fetchTypedRecordByURI(t, config, typedField, sample.URI)
	if typedRecord == nil {
		t.Fatalf("%sByUri(%q) returned null", typedField, sample.URI)
	}
	assertMatchingRecordMetadata(t, typedField+"ByUri", Record{
		URI:  sample.URI,
		CID:  sample.CID,
		DID:  sample.DID,
		RKey: sample.RKey,
	}, *typedRecord)

	smokeLog("✓ %s pagination, filters, sort, and ByUri work", certifiedGraphFollowCollection)
}

func queryCertifiedGraphFollowPage(t testing.TB, ctx context.Context, config smokeConfig, request certifiedGraphFollowPageRequest) certifiedGraphFollowConnection {
	t.Helper()

	variables := map[string]any{
		"first": request.first,
	}
	if request.after != "" {
		variables["after"] = request.after
	}
	if request.where != nil {
		variables["where"] = request.where
	}
	if request.sortBy != "" {
		variables["sortBy"] = request.sortBy
	}
	if request.sortDirection != "" {
		variables["sortDirection"] = request.sortDirection
	}

	response := postGraphQL(t, ctx, config, "CertifiedGraphFollowSmoke", `
		query CertifiedGraphFollowSmoke(
			$first: Int!
			$after: String
			$where: AppCertifiedGraphFollowWhereInput
			$sortBy: AppCertifiedGraphFollowSortField
			$sortDirection: SortDirection
		) {
			appCertifiedGraphFollow(first: $first, after: $after, where: $where, sortBy: $sortBy, sortDirection: $sortDirection) {
				totalCount
				edges {
					cursor
					node {
						uri
						cid
						did
						rkey
						subject
						createdAt
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
	`, variables)

	var data certifiedGraphFollowData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatalf("%s: decode response data: %v", certifiedGraphFollowTypedField, err)
	}

	return data.AppCertifiedGraphFollow
}

func assertCertifiedGraphFollowFilter(t testing.TB, ctx context.Context, config smokeConfig, name string, where map[string]any, matches func(certifiedGraphFollowRecord) bool) {
	t.Helper()

	page := queryCertifiedGraphFollowPage(t, ctx, config, certifiedGraphFollowPageRequest{
		first: certifiedGraphFollowPageSize,
		where: where,
	})
	assertCertifiedGraphFollowPageAtLeast(t, name, page, 1)
	for _, edge := range page.Edges {
		if !matches(edge.Node) {
			t.Fatalf("%s %s returned non-matching record: %+v", certifiedGraphFollowTypedField, name, edge.Node)
		}
	}
}

func assertCertifiedGraphFollowPage(t testing.TB, pageName string, page certifiedGraphFollowConnection, expectedEdges int) map[string]bool {
	t.Helper()

	if len(page.Edges) != expectedEdges {
		t.Fatalf("%s %s returned %d edges, want %d", certifiedGraphFollowTypedField, pageName, len(page.Edges), expectedEdges)
	}
	return assertCertifiedGraphFollowPageAtLeast(t, pageName, page, expectedEdges)
}

func assertCertifiedGraphFollowPageAtLeast(t testing.TB, pageName string, page certifiedGraphFollowConnection, minimumEdges int) map[string]bool {
	t.Helper()

	if len(page.Edges) < minimumEdges {
		t.Fatalf("%s %s returned %d edges, want at least %d", certifiedGraphFollowTypedField, pageName, len(page.Edges), minimumEdges)
	}

	seenURIs := make(map[string]bool, len(page.Edges))
	for edgeIndex, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("%s %s edge %d cursor is empty", certifiedGraphFollowTypedField, pageName, edgeIndex)
		}
		assertCertifiedGraphFollowRecord(t, pageName, edgeIndex, edge.Node)
		if seenURIs[edge.Node.URI] {
			t.Fatalf("%s %s returned duplicate URI %q", certifiedGraphFollowTypedField, pageName, edge.Node.URI)
		}
		seenURIs[edge.Node.URI] = true
	}

	return seenURIs
}

func assertCertifiedGraphFollowRecord(t testing.TB, pageName string, edgeIndex int, record certifiedGraphFollowRecord) {
	t.Helper()

	location := fmt.Sprintf("%s edge %d uri=%q", pageName, edgeIndex, record.URI)
	if !strings.HasPrefix(record.URI, "at://") {
		t.Fatalf("%s %s: uri want at:// prefix", certifiedGraphFollowTypedField, location)
	}
	if !strings.Contains(record.URI, "/"+certifiedGraphFollowCollection+"/") {
		t.Fatalf("%s %s: uri want collection segment %q", certifiedGraphFollowTypedField, location, "/"+certifiedGraphFollowCollection+"/")
	}
	if !strings.HasPrefix(record.DID, "did:") {
		t.Fatalf("%s %s: did = %q, want did: prefix", certifiedGraphFollowTypedField, location, record.DID)
	}
	if record.CID == "" {
		t.Fatalf("%s %s: cid is empty", certifiedGraphFollowTypedField, location)
	}
	if record.RKey == "" {
		t.Fatalf("%s %s: rkey is empty", certifiedGraphFollowTypedField, location)
	}
	if !strings.HasPrefix(record.Subject, "did:") {
		t.Fatalf("%s %s: subject = %q, want did: prefix", certifiedGraphFollowTypedField, location, record.Subject)
	}
	if _, err := time.Parse(time.RFC3339Nano, record.CreatedAt); err != nil {
		t.Fatalf("%s %s: createdAt = %q, want RFC3339 timestamp: %v", certifiedGraphFollowTypedField, location, record.CreatedAt, err)
	}
}

func assertCertifiedGraphFollowContainsURI(t testing.TB, pageName string, page certifiedGraphFollowConnection, uri string) {
	t.Helper()

	for _, edge := range page.Edges {
		if edge.Node.URI == uri {
			return
		}
	}
	t.Fatalf("%s %s did not include expected URI %q", certifiedGraphFollowTypedField, pageName, uri)
}

func assertCertifiedGraphFollowSubjectsSorted(t testing.TB, page certifiedGraphFollowConnection) {
	t.Helper()

	for i := 1; i < len(page.Edges); i++ {
		previous := page.Edges[i-1].Node.Subject
		current := page.Edges[i].Node.Subject
		if previous > current {
			t.Fatalf("%s subject sort order failed at edge %d: %q > %q", certifiedGraphFollowTypedField, i, previous, current)
		}
	}
}

func minimumExpectedRecordsForCollection(t testing.TB, config smokeConfig, collection string) int {
	t.Helper()

	for _, expectation := range config.expectations.DataBearingCollections {
		if expectation.NSID == collection {
			return expectation.MinimumRecords
		}
	}
	t.Fatalf("smoke expectations are missing data-bearing collection %q", collection)
	return 0
}
