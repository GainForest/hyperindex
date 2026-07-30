//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

func TestCollectionStatsSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeCollectionStats", `
		query SmokeCollectionStats {
			collectionStats {
				collection
				count
			}
		}
	`, nil)

	var payload struct {
		CollectionStats []struct {
			Collection string `json:"collection"`
			Count      int    `json:"count"`
		} `json:"collectionStats"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeCollectionStats: decode response data: %v", err)
	}
	if len(payload.CollectionStats) == 0 {
		t.Fatal("SmokeCollectionStats: collectionStats is empty, want at least one collection")
	}

	countsByCollection := make(map[string]int, len(payload.CollectionStats))
	for index, item := range payload.CollectionStats {
		if item.Collection == "" {
			t.Fatalf("SmokeCollectionStats: collectionStats[%d].collection is empty", index)
		}
		if item.Count < 0 {
			t.Fatalf("SmokeCollectionStats: collectionStats[%d].count = %d, want >= 0", index, item.Count)
		}
		countsByCollection[item.Collection] = item.Count
	}

	for _, expected := range config.expectations.DataBearingCollections {
		rawCount, ok := countsByCollection[expected.NSID]
		if !ok {
			t.Fatalf("SmokeCollectionStats: collectionStats is missing data-bearing collection %q; available collections: %s", expected.NSID, formatAvailableCollections(countsByCollection))
		}

		typedField := config.expectations.TypedQueryFields[expected.NSID]
		typedCount := fetchTypedCollectionCount(t, config, typedField)
		if typedCount < expected.MinimumRecords {
			t.Fatalf("SmokeCollectionStats: typed collection %q has %d valid records, want at least %d", expected.NSID, typedCount, expected.MinimumRecords)
		}
		smokeLog("✓ %s has at least %d valid typed records (%d raw at stats snapshot)", expected.NSID, expected.MinimumRecords, rawCount)
	}
}

func fetchTypedCollectionCount(t testing.TB, config smokeConfig, typedField string) int {
	t.Helper()

	query := fmt.Sprintf(`
query SmokeTypedCollectionCount {
  %s(first: 1) {
    totalCount
  }
}`, typedField)
	response := postGraphQL(t, context.Background(), config, "SmokeTypedCollectionCount", query, nil)

	var payload map[string]struct {
		TotalCount int `json:"totalCount"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeTypedCollectionCount: decode response data for %s: %v", typedField, err)
	}
	connection, ok := payload[typedField]
	if !ok {
		t.Fatalf("SmokeTypedCollectionCount: response is missing typed field %q", typedField)
	}
	return connection.TotalCount
}

func TestSearchSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	response := postGraphQL(t, context.Background(), config, "SmokeSearch", `
		query SmokeSearch($query: String!, $first: Int!) {
			search(query: $query, first: $first) {
				edges {
					node {
						uri
						did
						collection
						validationStatus
						validationError
						validatedAt
						lexiconHash
					}
				}
			}
		}
	`, map[string]any{
		"query": config.expectations.Search.Query,
		"first": config.expectations.Search.First,
	})

	var payload struct {
		Search struct {
			Edges []struct {
				Node Record `json:"node"`
			} `json:"edges"`
		} `json:"search"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeSearch: decode response data for query %q: %v", config.expectations.Search.Query, err)
	}
	if len(payload.Search.Edges) == 0 {
		t.Fatalf("SmokeSearch: query %q returned no search.edges, want at least one edge for first=%d", config.expectations.Search.Query, config.expectations.Search.First)
	}

	for index, edge := range payload.Search.Edges {
		if !strings.HasPrefix(edge.Node.URI, "at://") {
			t.Fatalf("SmokeSearch: query %q search.edges[%d].node.uri = %q, want at:// prefix", config.expectations.Search.Query, index, edge.Node.URI)
		}
		if !strings.HasPrefix(edge.Node.DID, "did:") {
			t.Fatalf("SmokeSearch: query %q search.edges[%d].node.did = %q, want did: prefix", config.expectations.Search.Query, index, edge.Node.DID)
		}
		if edge.Node.Collection == "" {
			t.Fatalf("SmokeSearch: query %q search.edges[%d].node.collection is empty", config.expectations.Search.Query, index)
		}
		location := fmt.Sprintf("search query %q edge %d uri=%q", config.expectations.Search.Query, index, edge.Node.URI)
		assertGenericRecordValidationMetadata(t, location, edge.Node)
	}

	smokeLog("✓ Search responds with validation metadata")
}

func formatAvailableCollections(countsByCollection map[string]int) string {
	collections := make([]string, 0, len(countsByCollection))
	for collection := range countsByCollection {
		collections = append(collections, collection)
	}
	sort.Strings(collections)
	return strings.Join(collections, ",")
}
