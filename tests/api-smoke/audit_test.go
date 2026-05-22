//go:build api_smoke

package apismoke

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

const maxSmokeAuditShapeEdges = 20

const smokeAuditRecordEventsQuery = `
query SmokeAuditRecordEvents($first: Int!, $after: String, $where: AuditRecordEventWhere, $orderBy: AuditRecordEventOrder) {
  auditRecordEvents(first: $first, after: $after, where: $where, orderBy: $orderBy) {
    edges {
      cursor
      node {
        id
        receivedAt
        live
        rev
        did
        collection
        rkey
        uri
        action
        cid
        record
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      startCursor
      endCursor
    }
    totalCount
  }
}`

const smokeAuditCurrentProjectionQueryTemplate = `
query SmokeAuditCurrentProjection($uri: String!) {
  record: %sByUri(uri: $uri) {
    uri
  }
}`

type auditRecordEventsQueryResponse struct {
	AuditRecordEvents auditRecordEventConnection `json:"auditRecordEvents"`
}

type auditRecordEventConnection struct {
	Edges      []auditRecordEventEdge `json:"edges"`
	PageInfo   PageInfo               `json:"pageInfo"`
	TotalCount *int                   `json:"totalCount"`
}

type auditRecordEventEdge struct {
	Cursor string           `json:"cursor"`
	Node   auditRecordEvent `json:"node"`
}

type auditRecordEvent struct {
	ID         string         `json:"id"`
	ReceivedAt string         `json:"receivedAt"`
	Live       bool           `json:"live"`
	Rev        string         `json:"rev"`
	DID        string         `json:"did"`
	Collection string         `json:"collection"`
	RKey       string         `json:"rkey"`
	URI        string         `json:"uri"`
	Action     string         `json:"action"`
	CID        *string        `json:"cid"`
	Record     map[string]any `json:"record"`
}

type auditRecordEventQueryOptions struct {
	First          int
	After          string
	Where          map[string]any
	OrderDirection string
}

func TestAuditRecordEventsSmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	if !config.audit.enabled {
		return
	}

	first := config.audit.minEvents
	if first > maxSmokeAuditShapeEdges {
		first = maxSmokeAuditShapeEdges
	}
	connection := queryAuditRecordEvents(t, context.Background(), config, auditRecordEventQueryOptions{
		First:          first,
		OrderDirection: "DESC",
	})

	if connection.TotalCount == nil {
		t.Fatal("SmokeAuditRecordEvents: totalCount is null, want an integer. Confirm the target deployment includes the audit totalCount fix.")
	}
	if *connection.TotalCount < config.audit.minEvents {
		t.Fatalf("SmokeAuditRecordEvents: totalCount = %d, want at least %d. Confirm the target deployment has TAP_ENABLED=true and AUDIT_ENABLED=true, then wait for Tap to ingest audit rows.", *connection.TotalCount, config.audit.minEvents)
	}

	edges := connection.Edges
	if len(edges) < first {
		t.Fatalf("SmokeAuditRecordEvents: auditRecordEvents returned %d edges, want %d. Confirm the target deployment has TAP_ENABLED=true and AUDIT_ENABLED=true, then wait for Tap to ingest audit rows.", len(edges), first)
	}

	for edgeIndex, edge := range edges {
		if edge.Cursor == "" {
			t.Fatalf("SmokeAuditRecordEvents: edge %d cursor is empty", edgeIndex)
		}
		assertAuditRecordEventShape(t, edgeIndex, edge.Node)
	}

	smokeLog("✓ auditRecordEvents has at least %d events (totalCount=%d)", config.audit.minEvents, *connection.TotalCount)
}

func TestAppendOnlyAuditE2ESmoke(t *testing.T) {
	config := loadSmokeConfig(t)
	if !config.audit.enabled {
		return
	}

	ctx := context.Background()
	firstPage := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{First: 5, OrderDirection: "ASC"})
	assertAuditConnectionShape(t, "append-only first page", firstPage)
	if *firstPage.TotalCount < 2 {
		t.Fatalf("append-only audit E2E requires at least 2 audit rows for cursor pagination, got totalCount=%d", *firstPage.TotalCount)
	}

	assertAuditPagination(t, ctx, config)
	assertAuditDescendingOrder(t, ctx, config)
	assertAuditTotalCountIgnoresCursor(t, ctx, config, firstPage.Edges[0])
	assertAuditEmptyFilter(t, ctx, config)

	sample := firstPage.Edges[0].Node
	assertAuditFilters(t, ctx, config, sample)
	assertAuditReceivedAtRangeFilters(t, ctx, config, sample)
	assertAuditCreateRowsIncludeRecordBody(t, ctx, config)
	assertOptionalAuditActionRows(t, ctx, config, "UPDATE")
	assertOptionalAuditActionRows(t, ctx, config, "DELETE")
	assertOptionalAuditLifecycleTrail(t, ctx, config)

	smokeLog("✓ append-only audit E2E checks passed")
}

func queryAuditRecordEvents(t testing.TB, ctx context.Context, config smokeConfig, options auditRecordEventQueryOptions) auditRecordEventConnection {
	t.Helper()

	if options.First <= 0 {
		options.First = 1
	}
	direction := options.OrderDirection
	if direction == "" {
		direction = "DESC"
	}

	variables := map[string]any{
		"first": options.First,
		"orderBy": map[string]any{
			"field":     "ID",
			"direction": direction,
		},
	}
	if options.After != "" {
		variables["after"] = options.After
	}
	if options.Where != nil {
		variables["where"] = options.Where
	}

	response := postGraphQL(t, ctx, config, "SmokeAuditRecordEvents", smokeAuditRecordEventsQuery, variables)
	var payload auditRecordEventsQueryResponse
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeAuditRecordEvents: decode response data: %v", err)
	}
	return payload.AuditRecordEvents
}

func assertAuditConnectionShape(t testing.TB, label string, connection auditRecordEventConnection) {
	t.Helper()

	if connection.TotalCount == nil {
		t.Fatalf("%s: totalCount is null, want an integer", label)
	}
	if len(connection.Edges) == 0 {
		t.Fatalf("%s: edges is empty", label)
	}
	if *connection.TotalCount < len(connection.Edges) {
		t.Fatalf("%s: totalCount = %d, want at least edge count %d", label, *connection.TotalCount, len(connection.Edges))
	}
	if connection.PageInfo.StartCursor == "" {
		t.Fatalf("%s: pageInfo.startCursor is empty", label)
	}
	if connection.PageInfo.EndCursor == "" {
		t.Fatalf("%s: pageInfo.endCursor is empty", label)
	}

	for edgeIndex, edge := range connection.Edges {
		if edge.Cursor == "" {
			t.Fatalf("%s: edge %d cursor is empty", label, edgeIndex)
		}
		assertAuditRecordEventShape(t, edgeIndex, edge.Node)
	}
}

func assertAuditPagination(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	firstPage := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{First: 1, OrderDirection: "ASC"})
	assertAuditConnectionShape(t, "append-only pagination first page", firstPage)
	secondPage := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		After:          firstPage.PageInfo.EndCursor,
		OrderDirection: "ASC",
	})
	assertAuditConnectionShape(t, "append-only pagination second page", secondPage)

	firstIDs := auditConnectionIDs(t, firstPage)
	secondIDs := auditConnectionIDs(t, secondPage)
	if !isStrictlyIncreasing(firstIDs) {
		t.Fatalf("append-only first page ids = %v, want strictly increasing ids for ASC order", firstIDs)
	}
	if !isStrictlyIncreasing(secondIDs) {
		t.Fatalf("append-only second page ids = %v, want strictly increasing ids for ASC order", secondIDs)
	}
	if firstIDs[len(firstIDs)-1] >= secondIDs[0] {
		t.Fatalf("append-only pagination overlap/regression: first page ids = %v, second page ids = %v", firstIDs, secondIDs)
	}
	if !secondPage.PageInfo.HasPreviousPage {
		t.Fatalf("append-only second page hasPreviousPage = false, want true (after=%q)", firstPage.PageInfo.EndCursor)
	}

	smokeLog("✓ auditRecordEvents cursor pagination works")
}

func assertAuditDescendingOrder(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{First: 5, OrderDirection: "DESC"})
	assertAuditConnectionShape(t, "append-only desc page", connection)
	ids := auditConnectionIDs(t, connection)
	if !isStrictlyDecreasing(ids) {
		t.Fatalf("append-only DESC ids = %v, want strictly decreasing ids", ids)
	}
}

func assertAuditTotalCountIgnoresCursor(t testing.TB, ctx context.Context, config smokeConfig, edge auditRecordEventEdge) {
	t.Helper()

	id, ok := auditGraphQLIntID(t, edge.Node.ID)
	if !ok {
		smokeLog("Skipping audit id totalCount cursor check because id %q exceeds GraphQL Int range", edge.Node.ID)
		return
	}

	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		After:          edge.Cursor,
		Where:          map[string]any{"id": map[string]any{"eq": id}},
		OrderDirection: "ASC",
	})
	if connection.TotalCount == nil || *connection.TotalCount != 1 {
		t.Fatalf("audit totalCount with id+after = %v, want 1", connection.TotalCount)
	}
	if len(connection.Edges) != 0 {
		t.Fatalf("audit id+after page returned %d edges, want 0 because cursor excludes the only matching row", len(connection.Edges))
	}
}

func assertAuditEmptyFilter(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	missingURI := "at://did:example:missing/app.missing.collection/nope"
	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		Where:          map[string]any{"uri": map[string]any{"eq": missingURI}},
		OrderDirection: "ASC",
	})
	if connection.TotalCount == nil || *connection.TotalCount != 0 {
		t.Fatalf("audit missing URI totalCount = %v, want 0", connection.TotalCount)
	}
	if len(connection.Edges) != 0 {
		t.Fatalf("audit missing URI returned %d edges, want 0", len(connection.Edges))
	}
}

func assertAuditFilters(t testing.TB, ctx context.Context, config smokeConfig, sample auditRecordEvent) {
	t.Helper()

	if id, ok := auditGraphQLIntID(t, sample.ID); ok {
		assertAuditWhereFilter(t, ctx, config, "id", map[string]any{"id": map[string]any{"eq": id}}, func(event auditRecordEvent) bool {
			return event.ID == sample.ID
		})
	}
	assertAuditWhereFilter(t, ctx, config, "uri", map[string]any{"uri": map[string]any{"eq": sample.URI}}, func(event auditRecordEvent) bool {
		return event.URI == sample.URI
	})
	assertAuditWhereFilter(t, ctx, config, "did", map[string]any{"did": map[string]any{"eq": sample.DID}}, func(event auditRecordEvent) bool {
		return event.DID == sample.DID
	})
	assertAuditWhereFilter(t, ctx, config, "collection", map[string]any{"collection": map[string]any{"eq": sample.Collection}}, func(event auditRecordEvent) bool {
		return event.Collection == sample.Collection
	})
	assertAuditWhereFilter(t, ctx, config, "rkey", map[string]any{"rkey": map[string]any{"eq": sample.RKey}}, func(event auditRecordEvent) bool {
		return event.RKey == sample.RKey
	})
	assertAuditWhereFilter(t, ctx, config, "action", map[string]any{"action": map[string]any{"eq": sample.Action}}, func(event auditRecordEvent) bool {
		return event.Action == sample.Action
	})
	assertAuditWhereFilter(t, ctx, config, "live", map[string]any{"live": map[string]any{"eq": sample.Live}}, func(event auditRecordEvent) bool {
		return event.Live == sample.Live
	})
	assertAuditWhereFilter(t, ctx, config, "rev", map[string]any{"rev": map[string]any{"eq": sample.Rev}}, func(event auditRecordEvent) bool {
		return event.Rev == sample.Rev
	})
	assertAuditWhereFilter(t, ctx, config, "receivedAt.eq", map[string]any{"receivedAt": map[string]any{"eq": sample.ReceivedAt}}, func(event auditRecordEvent) bool {
		return event.ReceivedAt == sample.ReceivedAt
	})
	if sample.CID != nil {
		assertAuditWhereFilter(t, ctx, config, "cid", map[string]any{"cid": map[string]any{"eq": *sample.CID}}, func(event auditRecordEvent) bool {
			return event.CID != nil && *event.CID == *sample.CID
		})
	} else {
		assertAuditWhereFilter(t, ctx, config, "empty cid", map[string]any{"cid": map[string]any{"eq": ""}}, func(event auditRecordEvent) bool {
			return event.CID == nil || *event.CID == ""
		})
	}

	smokeLog("✓ auditRecordEvents filters work")
}

func assertAuditWhereFilter(t testing.TB, ctx context.Context, config smokeConfig, name string, where map[string]any, matches func(auditRecordEvent) bool) {
	t.Helper()

	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          5,
		Where:          where,
		OrderDirection: "ASC",
	})
	if connection.TotalCount == nil || *connection.TotalCount == 0 {
		t.Fatalf("audit where.%s totalCount = %v, want > 0", name, connection.TotalCount)
	}
	if len(connection.Edges) == 0 {
		t.Fatalf("audit where.%s returned no edges despite totalCount=%d", name, *connection.TotalCount)
	}
	for edgeIndex, edge := range connection.Edges {
		if !matches(edge.Node) {
			t.Fatalf("audit where.%s edge %d node = %+v, does not match filter", name, edgeIndex, edge.Node)
		}
	}
}

func assertAuditReceivedAtRangeFilters(t testing.TB, ctx context.Context, config smokeConfig, oldest auditRecordEvent) {
	t.Helper()

	latestConnection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{First: 1, OrderDirection: "DESC"})
	latest := latestConnection.Edges[0].Node
	if oldest.ReceivedAt == latest.ReceivedAt {
		smokeLog("Skipping audit receivedAt range checks: sampled oldest and latest rows have the same timestamp %q", oldest.ReceivedAt)
		return
	}

	gtConnection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		Where:          map[string]any{"receivedAt": map[string]any{"gt": oldest.ReceivedAt}},
		OrderDirection: "ASC",
	})
	if gtConnection.TotalCount == nil || *gtConnection.TotalCount == 0 {
		t.Fatalf("audit receivedAt.gt totalCount = %v, want > 0", gtConnection.TotalCount)
	}

	ltConnection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		Where:          map[string]any{"receivedAt": map[string]any{"lt": latest.ReceivedAt}},
		OrderDirection: "ASC",
	})
	if ltConnection.TotalCount == nil || *ltConnection.TotalCount == 0 {
		t.Fatalf("audit receivedAt.lt totalCount = %v, want > 0", ltConnection.TotalCount)
	}
}

func assertAuditCreateRowsIncludeRecordBody(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          10,
		Where:          map[string]any{"action": map[string]any{"eq": "CREATE"}},
		OrderDirection: "ASC",
	})
	if connection.TotalCount == nil || *connection.TotalCount == 0 {
		t.Fatal("audit CREATE totalCount = 0, want at least one create row")
	}
	for _, edge := range connection.Edges {
		if len(edge.Node.Record) > 0 {
			smokeLog("✓ audit create/update rows expose decoded record JSON")
			return
		}
	}
	t.Fatalf("audit CREATE rows returned no decoded record JSON in first %d edges", len(connection.Edges))
}

func assertOptionalAuditActionRows(t testing.TB, ctx context.Context, config smokeConfig, action string) {
	t.Helper()

	connection := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          1,
		Where:          map[string]any{"action": map[string]any{"eq": action}},
		OrderDirection: "ASC",
	})
	if connection.TotalCount == nil || *connection.TotalCount == 0 {
		smokeLog("Skipping optional audit %s check: no matching rows observed", action)
		return
	}
	if len(connection.Edges) != 1 {
		t.Fatalf("audit %s returned %d edges, want 1", action, len(connection.Edges))
	}
	row := connection.Edges[0].Node
	if row.Action != action {
		t.Fatalf("audit %s row action = %q, want %q", action, row.Action, action)
	}
	if action == "DELETE" && (row.CID != nil || row.Record != nil) {
		t.Fatalf("audit DELETE row id=%s cid=%v record=%v, want null cid and record", row.ID, row.CID, row.Record)
	}
}

func assertOptionalAuditLifecycleTrail(t testing.TB, ctx context.Context, config smokeConfig) {
	t.Helper()

	deletes := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
		First:          5,
		Where:          map[string]any{"action": map[string]any{"eq": "DELETE"}},
		OrderDirection: "DESC",
	})
	if deletes.TotalCount == nil || *deletes.TotalCount == 0 {
		smokeLog("Skipping optional audit lifecycle check: no DELETE rows observed")
		return
	}

	for _, deletedEdge := range deletes.Edges {
		deleted := deletedEdge.Node
		trail := queryAuditRecordEvents(t, ctx, config, auditRecordEventQueryOptions{
			First:          20,
			Where:          map[string]any{"uri": map[string]any{"eq": deleted.URI}},
			OrderDirection: "ASC",
		})
		actions := auditConnectionActions(trail)
		if containsAuditAction(actions, "CREATE") && containsAuditAction(actions, "DELETE") {
			if !actionAppearsBefore(actions, "CREATE", "DELETE") {
				t.Fatalf("audit lifecycle uri=%q actions=%v, want CREATE before DELETE", deleted.URI, actions)
			}
			last := trail.Edges[len(trail.Edges)-1].Node
			if last.Action == "DELETE" && (last.CID != nil || last.Record != nil) {
				t.Fatalf("audit lifecycle delete row id=%s cid=%v record=%v, want null cid and record", last.ID, last.CID, last.Record)
			}
			assertDeletedCurrentProjectionIfTypedFieldKnown(t, ctx, config, deleted)
			smokeLog("✓ audit lifecycle trail preserves append-only history for %s", deleted.URI)
			return
		}
	}

	smokeLog("Skipping optional audit lifecycle check: DELETE rows exist but no complete CREATE→DELETE trail was observed in sampled rows")
}

func assertDeletedCurrentProjectionIfTypedFieldKnown(t testing.TB, ctx context.Context, config smokeConfig, deleted auditRecordEvent) {
	t.Helper()

	fieldName, ok := config.expectations.TypedQueryFields[deleted.Collection]
	if !ok {
		smokeLog("Skipping deleted projection check for %s: no typed query field configured", deleted.Collection)
		return
	}

	query := fmt.Sprintf(smokeAuditCurrentProjectionQueryTemplate, fieldName)
	response := postGraphQL(t, ctx, config, "SmokeAuditCurrentProjection", query, map[string]any{"uri": deleted.URI})
	var payload struct {
		Record *struct {
			URI string `json:"uri"`
		} `json:"record"`
	}
	if err := json.Unmarshal(response.Data, &payload); err != nil {
		t.Fatalf("SmokeAuditCurrentProjection: decode response data: %v", err)
	}
	if payload.Record != nil {
		t.Fatalf("deleted audit row uri=%q still has current-state projection %+v", deleted.URI, payload.Record)
	}
}

func assertAuditRecordEventShape(t testing.TB, edgeIndex int, event auditRecordEvent) {
	t.Helper()

	location := fmt.Sprintf("auditRecordEvents edge %d id=%q uri=%q", edgeIndex, event.ID, event.URI)
	if event.ID == "" {
		t.Fatalf("%s: id is empty", location)
	}
	if _, err := strconv.ParseInt(event.ID, 10, 64); err != nil {
		t.Fatalf("%s: id is not a base-10 integer: %v", location, err)
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
	if !strings.HasSuffix(event.URI, "/"+event.RKey) {
		t.Fatalf("%s: uri does not end with rkey segment %q", location, "/"+event.RKey)
	}
	if !validAuditRecordAction(event.Action) {
		t.Fatalf("%s: action = %q, want CREATE, UPDATE, or DELETE", location, event.Action)
	}
}

func auditConnectionIDs(t testing.TB, connection auditRecordEventConnection) []int64 {
	t.Helper()

	ids := make([]int64, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		id, err := strconv.ParseInt(edge.Node.ID, 10, 64)
		if err != nil {
			t.Fatalf("audit id %q is not a base-10 integer: %v", edge.Node.ID, err)
		}
		ids = append(ids, id)
	}
	return ids
}

func auditGraphQLIntID(t testing.TB, id string) (int, bool) {
	t.Helper()

	parsed, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		t.Fatalf("audit id %q is not a base-10 integer: %v", id, err)
	}
	if parsed > 1<<31-1 {
		return 0, false
	}
	return int(parsed), true
}

func auditConnectionActions(connection auditRecordEventConnection) []string {
	actions := make([]string, 0, len(connection.Edges))
	for _, edge := range connection.Edges {
		actions = append(actions, edge.Node.Action)
	}
	return actions
}

func isStrictlyIncreasing(values []int64) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] >= values[index] {
			return false
		}
	}
	return true
}

func isStrictlyDecreasing(values []int64) bool {
	for index := 1; index < len(values); index++ {
		if values[index-1] <= values[index] {
			return false
		}
	}
	return true
}

func containsAuditAction(actions []string, want string) bool {
	for _, action := range actions {
		if action == want {
			return true
		}
	}
	return false
}

func actionAppearsBefore(actions []string, before string, after string) bool {
	beforeIndex := -1
	afterIndex := -1
	for index, action := range actions {
		switch action {
		case before:
			if beforeIndex == -1 {
				beforeIndex = index
			}
		case after:
			afterIndex = index
		}
	}
	return beforeIndex >= 0 && afterIndex >= 0 && beforeIndex < afterIndex
}

func validAuditRecordAction(action string) bool {
	switch action {
	case "CREATE", "UPDATE", "DELETE":
		return true
	default:
		return false
	}
}
