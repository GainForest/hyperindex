//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"testing"
)

const expectedPaginationPageSize = 10

type paginationRecordsData struct {
	Records paginationConnection `json:"records"`
}

type paginationConnection struct {
	Edges    []paginationEdge `json:"edges"`
	PageInfo PageInfo         `json:"pageInfo"`
}

type paginationEdge struct {
	Cursor string `json:"cursor"`
	Node   Record `json:"node"`
}

func TestPaginationSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	ctx := context.Background()

	for _, collection := range config.expectations.PaginationCollections {
		collection := collection
		t.Run(collection.NSID, func(t *testing.T) {
			if collection.PageSize != expectedPaginationPageSize {
				t.Fatalf("pagination collection %q pageSize = %d, want %d", collection.NSID, collection.PageSize, expectedPaginationPageSize)
			}

			firstPage := queryPaginationPage(t, ctx, config, collection.NSID, collection.PageSize, "")
			pageOneURIs := assertPaginationPage(t, collection.NSID, "first page", firstPage, collection.PageSize)
			if !firstPage.PageInfo.HasNextPage {
				t.Fatalf("pagination collection %q first page hasNextPage = false, want true", collection.NSID)
			}
			if firstPage.PageInfo.EndCursor == "" {
				t.Fatalf("pagination collection %q first page endCursor is empty", collection.NSID)
			}

			secondPage := queryPaginationPage(t, ctx, config, collection.NSID, collection.PageSize, firstPage.PageInfo.EndCursor)
			pageTwoURIs := assertPaginationPage(t, collection.NSID, "second page", secondPage, collection.PageSize)
			for uri := range pageTwoURIs {
				if pageOneURIs[uri] {
					t.Fatalf("pagination collection %q returned duplicate URI %q across adjacent pages", collection.NSID, uri)
				}
			}
		})
	}
}

func queryPaginationPage(t testing.TB, ctx context.Context, config smokeConfig, collection string, first int, after string) paginationConnection {
	t.Helper()

	variables := map[string]any{
		"collection": collection,
		"first":      first,
	}
	if after != "" {
		variables["after"] = after
	}

	response := postGraphQL(t, ctx, config, "PaginationSmoke", `
		query PaginationSmoke($collection: String!, $first: Int!, $after: String) {
			records(collection: $collection, first: $first, after: $after) {
				edges {
					cursor
					node {
						uri
						did
						collection
					}
				}
				pageInfo {
					hasNextPage
					endCursor
				}
			}
		}
	`, variables)
	if len(response.Errors) > 0 {
		t.Fatalf("pagination collection %q returned GraphQL errors: %s", collection, formatGraphQLErrors(response.Errors))
	}

	var data paginationRecordsData
	if err := json.Unmarshal(response.Data, &data); err != nil {
		t.Fatalf("pagination collection %q: decode records response: %v", collection, err)
	}

	return data.Records
}

func assertPaginationPage(t testing.TB, collection string, pageName string, page paginationConnection, expectedEdges int) map[string]bool {
	t.Helper()

	if len(page.Edges) != expectedEdges {
		t.Fatalf("pagination collection %q %s returned %d edges, want %d", collection, pageName, len(page.Edges), expectedEdges)
	}

	seenURIs := make(map[string]bool, len(page.Edges))
	for edgeIndex, edge := range page.Edges {
		if edge.Cursor == "" {
			t.Fatalf("pagination collection %q %s edge %d cursor is empty", collection, pageName, edgeIndex)
		}
		assertPaginationNode(t, collection, pageName, edgeIndex, edge.Node)
		if seenURIs[edge.Node.URI] {
			t.Fatalf("pagination collection %q %s returned duplicate URI %q", collection, pageName, edge.Node.URI)
		}
		seenURIs[edge.Node.URI] = true
	}

	return seenURIs
}

func assertPaginationNode(t testing.TB, collection string, pageName string, edgeIndex int, node Record) {
	t.Helper()

	if node.URI == "" {
		t.Fatalf("pagination collection %q %s edge %d node uri is empty", collection, pageName, edgeIndex)
	}
	if node.DID == "" {
		t.Fatalf("pagination collection %q %s edge %d node did is empty", collection, pageName, edgeIndex)
	}
	if node.Collection != collection {
		t.Fatalf("pagination collection %q %s edge %d node collection = %q, want %q", collection, pageName, edgeIndex, node.Collection, collection)
	}
}
