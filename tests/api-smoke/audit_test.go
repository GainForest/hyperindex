//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

const smokeAuditRecordEventsQuery = `
query SmokeAuditRecordEvents($first: Int!) {
  auditRecordEvents(first: $first) {
    edges {
      cursor
      node {
        id
        receivedAt
        live
        did
        collection
        rkey
        uri
        action
        cid
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`

type auditRecordEventsQueryResponse struct {
	AuditRecordEvents auditRecordEventConnection `json:"auditRecordEvents"`
}

type auditRecordEventConnection struct {
	Edges    []auditRecordEventEdge `json:"edges"`
	PageInfo PageInfo               `json:"pageInfo"`
}

type auditRecordEventEdge struct {
	Cursor string           `json:"cursor"`
	Node   auditRecordEvent `json:"node"`
}

type auditRecordEvent struct {
	ID         string `json:"id"`
	ReceivedAt string `json:"receivedAt"`
	Live       bool   `json:"live"`
	DID        string `json:"did"`
	Collection string `json:"collection"`
	RKey       string `json:"rkey"`
	URI        string `json:"uri"`
	Action     string `json:"action"`
	CID        string `json:"cid"`
}

func TestAuditRecordEventsSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	if !config.audit.enabled {
		return
	}

	response := postGraphQL(t, context.Background(), config, "SmokeAuditRecordEvents", smokeAuditRecordEventsQuery, map[string]any{
		"first": config.audit.minEvents,
	})

	var payload auditRecordEventsQueryResponse
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeAuditRecordEvents: decode response data: %v", err)
	}

	edges := payload.AuditRecordEvents.Edges
	if len(edges) < config.audit.minEvents {
		t.Fatalf("SmokeAuditRecordEvents: auditRecordEvents returned %d edges, want at least %d. Confirm the target deployment has TAP_ENABLED=true and AUDIT_ENABLED=true, then wait for Tap to ingest audit rows.", len(edges), config.audit.minEvents)
	}

	for edgeIndex, edge := range edges {
		if edge.Cursor == "" {
			t.Fatalf("SmokeAuditRecordEvents: edge %d cursor is empty", edgeIndex)
		}
		assertAuditRecordEventShape(t, edgeIndex, edge.Node)
	}

	smokeLog("✓ auditRecordEvents has at least %d events", config.audit.minEvents)
}

func assertAuditRecordEventShape(t testing.TB, edgeIndex int, event auditRecordEvent) {
	t.Helper()

	location := fmt.Sprintf("auditRecordEvents edge %d id=%q uri=%q", edgeIndex, event.ID, event.URI)
	if event.ID == "" {
		t.Fatalf("%s: id is empty", location)
	}
	if event.ReceivedAt == "" {
		t.Fatalf("%s: receivedAt is empty", location)
	}
	if !strings.HasPrefix(event.DID, "did:") {
		t.Fatalf("%s: did = %q, want did: prefix", location, event.DID)
	}
	if event.Collection == "" {
		t.Fatalf("%s: collection is empty", location)
	}
	if event.RKey == "" {
		t.Fatalf("%s: rkey is empty", location)
	}
	if !strings.HasPrefix(event.URI, "at://") {
		t.Fatalf("%s: uri = %q, want at:// prefix", location, event.URI)
	}
	if !strings.Contains(event.URI, "/"+event.Collection+"/") {
		t.Fatalf("%s: uri does not contain collection segment %q", location, "/"+event.Collection+"/")
	}
	if !validAuditRecordAction(event.Action) {
		t.Fatalf("%s: action = %q, want CREATE, UPDATE, or DELETE", location, event.Action)
	}
}

func validAuditRecordAction(action string) bool {
	switch action {
	case "CREATE", "UPDATE", "DELETE":
		return true
	default:
		return false
	}
}
