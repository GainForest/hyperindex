//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
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
		count, ok := countsByCollection[expected.NSID]
		if !ok {
			t.Fatalf("SmokeCollectionStats: collectionStats is missing data-bearing collection %q", expected.NSID)
		}
		if count < expected.MinimumRecords {
			t.Fatalf("SmokeCollectionStats: collectionStats[%q].count = %d, want >= %d", expected.NSID, count, expected.MinimumRecords)
		}
	}
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
				Node struct {
					URI        string `json:"uri"`
					DID        string `json:"did"`
					Collection string `json:"collection"`
				} `json:"node"`
			} `json:"edges"`
		} `json:"search"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeSearch: decode response data: %v", err)
	}

	for index, edge := range payload.Search.Edges {
		if !strings.HasPrefix(edge.Node.URI, "at://") {
			t.Fatalf("SmokeSearch: search.edges[%d].node.uri = %q, want at:// prefix", index, edge.Node.URI)
		}
		if !strings.HasPrefix(edge.Node.DID, "did:") {
			t.Fatalf("SmokeSearch: search.edges[%d].node.did = %q, want did: prefix", index, edge.Node.DID)
		}
		if edge.Node.Collection == "" {
			t.Fatalf("SmokeSearch: search.edges[%d].node.collection is empty", index)
		}
	}
}
