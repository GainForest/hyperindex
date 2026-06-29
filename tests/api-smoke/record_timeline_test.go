//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const minimumRecordTimelineRecords = 20

const smokeRecordTimelineQuery = `
query SmokeRecordTimeline($where: RecordTimelineWhereInput!, $first: Int!, $after: String) {
  recordTimeline(where: $where, first: $first, after: $after) {
    edges {
      cursor
      node {
        uri
        cid
        did
        collection
        rkey
        createdAt
        indexedAt
        json
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      startCursor
      endCursor
    }
  }
}`

const smokeRecordTimelineProfileHydrationQuery = `
query SmokeRecordTimelineProfileHydration($where: RecordTimelineWhereInput!, $first: Int!) {
  recordTimeline(where: $where, first: $first) {
    edges {
      node {
        did
        collection
        certifiedProfileData {
          did
          displayName
          createdAt
        }
      }
    }
  }
}`

type recordTimelineResponse struct {
	RecordTimeline recordTimelineConnection `json:"recordTimeline"`
}

type recordTimelineConnection struct {
	Edges    []recordTimelineEdge `json:"edges"`
	PageInfo PageInfo             `json:"pageInfo"`
}

type recordTimelineEdge struct {
	Cursor string             `json:"cursor"`
	Node   recordTimelineNode `json:"node"`
}

type recordTimelineNode struct {
	URI                  string                     `json:"uri"`
	CID                  string                     `json:"cid"`
	DID                  string                     `json:"did"`
	Collection           string                     `json:"collection"`
	RKey                 string                     `json:"rkey"`
	CreatedAt            string                     `json:"createdAt"`
	IndexedAt            string                     `json:"indexedAt"`
	JSON                 map[string]any             `json:"json"`
	CertifiedProfileData *recordTimelineProfileData `json:"certifiedProfileData"`
}

type recordTimelineProfileData struct {
	DID         string `json:"did"`
	DisplayName string `json:"displayName"`
	CreatedAt   string `json:"createdAt"`
}

func TestRecordTimelineSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	assertRecordTimelineSchema(t, config)

	collections := []string{profileCollection, activityCollection}
	minimumPage := queryRecordTimelinePage(t, ctx, config, collections, nil, minimumRecordTimelineRecords, "")
	assertRecordTimelinePage(t, "minimum record sanity page", minimumPage, collections, "")
	assertRecordTimelineDescendingOrder(t, "minimum record sanity page", minimumPage)
	if len(minimumPage.Edges) < minimumRecordTimelineRecords {
		t.Fatalf("recordTimeline returned %d records for collections %v, want at least %d", len(minimumPage.Edges), collections, minimumRecordTimelineRecords)
	}

	firstPage := queryRecordTimelinePage(t, ctx, config, collections, nil, 5, "")
	firstPageURIs := assertRecordTimelinePage(t, "first page", firstPage, collections, "")
	if !firstPage.PageInfo.HasNextPage {
		t.Fatalf("recordTimeline first page hasNextPage = false, want true for collections %v with first=5", collections)
	}
	if firstPage.PageInfo.EndCursor == "" {
		t.Fatalf("recordTimeline first page endCursor is empty")
	}

	secondPage := queryRecordTimelinePage(t, ctx, config, collections, nil, 5, firstPage.PageInfo.EndCursor)
	secondPageURIs := assertRecordTimelinePage(t, "second page", secondPage, collections, firstPage.PageInfo.EndCursor)
	if !secondPage.PageInfo.HasPreviousPage {
		t.Fatalf("recordTimeline second page hasPreviousPage = false, want true")
	}
	for uri := range secondPageURIs {
		if firstPageURIs[uri] {
			t.Fatalf("recordTimeline returned duplicate URI %q across adjacent pages", uri)
		}
	}
	assertRecordTimelineDescendingOrder(t, "first and second page", firstPage, secondPage)

	selectedAuthor := firstPage.Edges[0].Node.DID
	selectedCollection := firstPage.Edges[0].Node.Collection
	authorPage := queryRecordTimelinePage(t, ctx, config, []string{selectedCollection}, []string{selectedAuthor}, 5, "")
	if len(authorPage.Edges) == 0 {
		t.Fatalf("recordTimeline author-filtered query returned no edges for author %q collection %q", selectedAuthor, selectedCollection)
	}
	for index, edge := range authorPage.Edges {
		if edge.Node.DID != selectedAuthor {
			t.Fatalf("recordTimeline author-filtered edge %d DID = %q, want %q", index, edge.Node.DID, selectedAuthor)
		}
		if edge.Node.Collection != selectedCollection {
			t.Fatalf("recordTimeline author-filtered edge %d collection = %q, want %q", index, edge.Node.Collection, selectedCollection)
		}
	}
	assertRecordTimelineDescendingOrder(t, "author-filtered page", authorPage)

	assertRecordTimelineProfileHydration(t, ctx, config)
	smokeLog("✓ recordTimeline returns at least 20 records, stable pages, author filtering, and profile hydration")
}

func queryRecordTimelinePage(t testing.TB, ctx context.Context, config smokeConfig, collections []string, authors []string, first int, after string) recordTimelineConnection {
	t.Helper()

	where := map[string]any{
		"collection": map[string]any{"in": collections},
	}
	if authors != nil {
		where["did"] = map[string]any{"in": authors}
	}
	variables := map[string]any{
		"where": where,
		"first": first,
	}
	if after != "" {
		variables["after"] = after
	}

	response := postGraphQL(t, ctx, config, "SmokeRecordTimeline", smokeRecordTimelineQuery, variables)
	var decoded recordTimelineResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("recordTimeline: decode response data: %v", err)
	}
	return decoded.RecordTimeline
}

func assertRecordTimelineDescendingOrder(t testing.TB, label string, pages ...recordTimelineConnection) {
	t.Helper()
	var previous *recordTimelineEdge
	for pageIndex, page := range pages {
		for edgeIndex, edge := range page.Edges {
			if previous != nil && !recordTimelineSortsBefore(*previous, edge) {
				t.Fatalf(
					"recordTimeline %s order regression before page %d edge %d: previous (%s, %s), current (%s, %s)",
					label,
					pageIndex,
					edgeIndex,
					previous.Node.CreatedAt,
					previous.Node.URI,
					edge.Node.CreatedAt,
					edge.Node.URI,
				)
			}
			edgeCopy := edge
			previous = &edgeCopy
		}
	}
}

func recordTimelineSortsBefore(previous, current recordTimelineEdge) bool {
	if previous.Node.CreatedAt != current.Node.CreatedAt {
		return previous.Node.CreatedAt > current.Node.CreatedAt
	}
	return previous.Node.URI > current.Node.URI
}

func assertRecordTimelinePage(t testing.TB, label string, page recordTimelineConnection, collections []string, after string) map[string]bool {
	t.Helper()
	if len(page.Edges) == 0 {
		t.Fatalf("recordTimeline %s returned no edges for collections %v after %q", label, collections, after)
	}
	allowedCollections := makeSet(collections)
	seenURIs := make(map[string]bool, len(page.Edges))
	for index, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("recordTimeline %s edge %d cursor is empty", label, index)
		}
		if edge.Node.URI == "" || edge.Node.CID == "" || edge.Node.DID == "" {
			t.Fatalf("recordTimeline %s edge %d has incomplete metadata: %+v", label, index, edge.Node)
		}
		if !allowedCollections[edge.Node.Collection] {
			t.Fatalf("recordTimeline %s edge %d collection = %q, want one of %v", label, index, edge.Node.Collection, collections)
		}
		if edge.Node.CreatedAt == "" {
			t.Fatalf("recordTimeline %s edge %d createdAt is empty for uri %q", label, index, edge.Node.URI)
		}
		if edge.Node.IndexedAt == "" {
			t.Fatalf("recordTimeline %s edge %d indexedAt is empty for uri %q", label, index, edge.Node.URI)
		}
		if len(edge.Node.JSON) == 0 {
			t.Fatalf("recordTimeline %s edge %d json is empty for uri %q", label, index, edge.Node.URI)
		}
		if seenURIs[edge.Node.URI] {
			t.Fatalf("recordTimeline %s returned duplicate URI %q", label, edge.Node.URI)
		}
		seenURIs[edge.Node.URI] = true
	}
	return seenURIs
}

func assertRecordTimelineSchema(t testing.TB, config smokeConfig) {
	t.Helper()
	schema := fetchGraphQLSchema(t, config)
	queryFields := fieldsByName(schema.QueryType.Fields)
	types := typesByName(schema.Types)

	field := requireSchemaField(t, queryFields, "recordTimeline")
	requireSchemaArgument(t, field, "where")
	requireSchemaArgument(t, field, "first")
	requireSchemaArgument(t, field, "after")

	connectionType := requireSchemaType(t, types, "RecordTimelineConnection")
	connectionFields := fieldsByName(connectionType.Fields)
	requireSchemaField(t, connectionFields, "edges")
	requireSchemaField(t, connectionFields, "pageInfo")
	if _, exists := connectionFields["totalCount"]; exists {
		t.Fatal("RecordTimelineConnection exposes totalCount, want no exact count on timeline queries")
	}

	nodeType := requireSchemaType(t, types, "RecordTimelineNode")
	nodeFields := fieldsByName(nodeType.Fields)
	for _, name := range []string{"uri", "cid", "did", "collection", "rkey", "createdAt", "indexedAt", "json", "certifiedProfileData"} {
		requireSchemaField(t, nodeFields, name)
	}
}

func assertRecordTimelineProfileHydration(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()
	response := postGraphQL(t, ctx, config, "SmokeRecordTimelineProfileHydration", smokeRecordTimelineProfileHydrationQuery, map[string]any{
		"where": map[string]any{
			"collection": map[string]any{"in": []string{profileCollection}},
		},
		"first": 1,
	})
	var decoded recordTimelineResponse
	if err := json.Unmarshal(response.Data, &decoded); err != nil {
		t.Fatalf("recordTimeline profile hydration: decode response data: %v", err)
	}
	if len(decoded.RecordTimeline.Edges) != 1 {
		t.Fatalf("recordTimeline profile hydration returned %d edges, want 1", len(decoded.RecordTimeline.Edges))
	}
	node := decoded.RecordTimeline.Edges[0].Node
	if node.Collection != profileCollection {
		t.Fatalf("recordTimeline profile hydration node collection = %q, want %q", node.Collection, profileCollection)
	}
	if node.CertifiedProfileData == nil {
		t.Fatalf("recordTimeline profile hydration returned null certifiedProfileData for profile author %q", node.DID)
	}
	if node.CertifiedProfileData.DID != node.DID {
		t.Fatalf("recordTimeline profile hydration DID = %q, want author DID %q", node.CertifiedProfileData.DID, node.DID)
	}
	if node.CertifiedProfileData.CreatedAt == "" {
		t.Fatalf("recordTimeline profile hydration createdAt is empty for author %q", node.DID)
	}
}
