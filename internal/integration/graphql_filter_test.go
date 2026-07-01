//go:build integration

// Package integration provides end-to-end integration tests for hyperindex.
package integration

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/validation"
	"github.com/GainForest/hyperindex/internal/validationrefresh"
)

// testLexiconJSON is a minimal lexicon with scalar fields and complex fields
// used to verify presence-only filtering.
const testLexiconJSON = `{
	"lexicon": 1,
	"id": "test.collection",
	"defs": {
		"main": {
			"type": "record",
			"key": "tid",
			"record": {
				"type": "object",
				"required": ["title"],
				"properties": {
					"title": {"type": "string"},
					"score": {"type": "integer"},
					"createdAt": {"type": "string", "format": "datetime"},
					"active": {"type": "boolean"},
					"optionalField": {"type": "string"},
					"image": {"type": "blob"},
					"contributors": {"type": "array", "items": {"type": "string"}},
					"field1": {"type": "string"},
					"field2": {"type": "string"},
					"field3": {"type": "string"},
					"field4": {"type": "string"},
					"field5": {"type": "string"},
					"field6": {"type": "string"},
					"field7": {"type": "string"},
					"field8": {"type": "string"}
				}
			}
		}
	}
}`

// filterTestEnv holds the test environment for filter/sort/search tests.
type filterTestEnv struct {
	db     *testDB
	schema *graphql.Schema
	ctx    context.Context
}

// setupFilterTestEnv creates a complete test environment with DB, lexicon, records, and schema.
func setupFilterTestEnv(t *testing.T) *filterTestEnv {
	t.Helper()

	db := setupTestDB(t)
	ctx := context.Background()

	// Parse and register the test lexicon
	registry := lexicon.NewRegistry()
	lex, err := lexicon.Parse(testLexiconJSON)
	if err != nil {
		t.Fatalf("Failed to parse test lexicon: %v", err)
	}
	registry.Register(lex)

	// Insert test records with known content
	// Use different indexed_at times so ordering is deterministic
	records := []*repositories.Record{
		{
			URI:        "at://did:plc:test123/test.collection/1",
			CID:        "cid1",
			DID:        "did:plc:test123",
			Collection: "test.collection",
			JSON:       `{"title": "Test Record", "score": 7, "createdAt": "2026-01-01T10:00:00Z", "active": true}`,
			RKey:       "1",
		},
		{
			URI:        "at://did:plc:test123/test.collection/2",
			CID:        "cid2",
			DID:        "did:plc:test123",
			Collection: "test.collection",
			JSON:       `{"title": "Another Record", "score": 3, "createdAt": "2026-01-02T10:00:00Z", "active": false}`,
			RKey:       "2",
		},
		{
			URI:        "at://did:plc:other456/test.collection/1",
			CID:        "cid3",
			DID:        "did:plc:other456",
			Collection: "test.collection",
			JSON:       `{"title": "Other User Record", "score": 15, "createdAt": "2026-01-03T10:00:00Z", "active": true}`,
			RKey:       "1",
		},
		{
			URI:        "at://did:plc:test123/test.collection/3",
			CID:        "cid4",
			DID:        "did:plc:test123",
			Collection: "test.collection",
			JSON:       `{"title": "Searchable Content", "score": 8, "createdAt": "2026-01-04T10:00:00Z", "active": true}`,
			RKey:       "3",
		},
		{
			URI:        "at://did:plc:other456/test.collection/2",
			CID:        "cid5",
			DID:        "did:plc:other456",
			Collection: "test.collection",
			JSON:       `{"title": "No Optional", "score": 20, "createdAt": "2026-01-05T10:00:00Z", "active": false}`,
			RKey:       "2",
		},
	}

	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("Failed to insert test records: %v", err)
	}
	markRecordsValid(t, ctx, db, records)

	// Set deterministic indexed_at times so ordering tests are reliable
	rawDB := db.Executor.DB()
	times := []string{
		"2026-01-01T10:00:00Z",
		"2026-01-02T10:00:00Z",
		"2026-01-03T10:00:00Z",
		"2026-01-04T10:00:00Z",
		"2026-01-05T10:00:00Z",
	}
	uris := []string{
		"at://did:plc:test123/test.collection/1",
		"at://did:plc:test123/test.collection/2",
		"at://did:plc:other456/test.collection/1",
		"at://did:plc:test123/test.collection/3",
		"at://did:plc:other456/test.collection/2",
	}
	for i, uri := range uris {
		_, err := rawDB.ExecContext(ctx, `UPDATE record SET indexed_at = ? WHERE uri = ?`, times[i], uri)
		if err != nil {
			t.Fatalf("Failed to set indexed_at for %s: %v", uri, err)
		}
	}

	// Build the GraphQL schema
	builder := schema.NewBuilder(registry)
	gqlSchema, err := builder.Build()
	if err != nil {
		t.Fatalf("Failed to build GraphQL schema: %v", err)
	}

	// Create context with repositories
	repos := &resolver.Repositories{
		Records:  db.Records,
		Actors:   db.Actors,
		Lexicons: db.Lexicons,
	}
	repoCtx := resolver.WithRepositories(ctx, repos)

	return &filterTestEnv{
		db:     db,
		schema: gqlSchema,
		ctx:    repoCtx,
	}
}

// runQuery executes a GraphQL query in the filter test environment.
func (env *filterTestEnv) runQuery(query string) *graphql.Result {
	return graphql.Do(graphql.Params{
		Schema:        *env.schema,
		RequestString: query,
		Context:       env.ctx,
	})
}

// getEdges extracts the edges array from a connection result.
func getEdges(t *testing.T, result *graphql.Result, fieldName string) []interface{} {
	t.Helper()
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected data map, got %T", result.Data)
	}
	conn, ok := data[fieldName].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected connection map for %s, got %T: %v", fieldName, data[fieldName], data[fieldName])
	}
	edges, ok := conn["edges"].([]interface{})
	if !ok {
		t.Fatalf("Expected edges slice, got %T", conn["edges"])
	}
	return edges
}

// getNodeField extracts a field value from an edge node.
func getNodeField(t *testing.T, edge interface{}, field string) interface{} {
	t.Helper()
	edgeMap, ok := edge.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected edge map, got %T", edge)
	}
	node, ok := edgeMap["node"].(map[string]interface{})
	if !ok {
		t.Fatalf("Expected node map, got %T", edgeMap["node"])
	}
	return node[field]
}

func buildQuotedValues(prefix string, count int) string {
	values := make([]string, count)
	for i := range values {
		values[i] = fmt.Sprintf("%q", fmt.Sprintf("%s-%d", prefix, i))
	}
	return strings.Join(values, ", ")
}

func assertEdgeURIs(t *testing.T, edges []interface{}, wantURIs []string) {
	t.Helper()

	got := make(map[string]bool, len(edges))
	for _, edge := range edges {
		uri, ok := getNodeField(t, edge, "uri").(string)
		if !ok {
			t.Fatalf("edge uri is %T, want string", getNodeField(t, edge, "uri"))
		}
		got[uri] = true
	}

	if len(got) != len(wantURIs) {
		t.Fatalf("edge URI count = %d, want %d: %v", len(got), len(wantURIs), got)
	}
	for _, uri := range wantURIs {
		if !got[uri] {
			t.Fatalf("missing URI %s in %v", uri, got)
		}
	}
}

// TestFilterSort_FilterByStringEq tests filtering by string equality.
func markRecordsValid(t *testing.T, ctx context.Context, db *testDB, records []*repositories.Record) {
	t.Helper()
	for _, rec := range records {
		if err := db.Records.UpdateValidationStatus(ctx, rec.URI, validation.StatusValid, "", "integration-test-valid"); err != nil {
			t.Fatalf("Failed to mark %s valid: %v", rec.URI, err)
		}
	}
}

func TestTypedGraphQLUsesValidationRefreshForCertifiedProfile(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	rawLexicon, err := os.ReadFile("../../testdata/lexicons/app/certified/actor/profile.json")
	if err != nil {
		t.Fatalf("Read profile lexicon: %v", err)
	}
	registry := lexicon.NewRegistry()
	parsed, err := lexicon.ParseBytes(rawLexicon)
	if err != nil {
		t.Fatalf("ParseBytes(profile) error = %v", err)
	}
	registry.Register(parsed)
	validator, err := validation.NewValidatorFromLexiconBytes(map[string][]byte{
		"app.certified.actor.profile": rawLexicon,
	})
	if err != nil {
		t.Fatalf("NewValidatorFromLexiconBytes() error = %v", err)
	}

	validURI := "at://did:plc:valid/app.certified.actor.profile/self"
	invalidURI := "at://did:plc:invalid/app.certified.actor.profile/self"
	records := []*repositories.Record{
		{URI: validURI, CID: "cid-valid", DID: "did:plc:valid", Collection: "app.certified.actor.profile", RKey: "self", JSON: `{"displayName":"Valid Profile","createdAt":"2026-01-01T00:00:00Z"}`},
		{URI: invalidURI, CID: "cid-invalid", DID: "did:plc:invalid", Collection: "app.certified.actor.profile", RKey: "self", JSON: `{"displayName":"Wrong Created At","createdAt":123}`},
	}
	for _, rec := range records {
		if _, err := db.Records.Insert(ctx, rec.URI, rec.CID, rec.DID, rec.Collection, rec.JSON); err != nil {
			t.Fatalf("Insert(%s) error = %v", rec.URI, err)
		}
	}

	scheduler := validationrefresh.NewScheduler(db.Records, validator)
	if err := scheduler.RefreshCollection(ctx, "app.certified.actor.profile", "integration_test"); err != nil {
		t.Fatalf("RefreshCollection() error = %v", err)
	}

	invalid, err := db.Records.GetByURI(ctx, invalidURI)
	if err != nil {
		t.Fatalf("GetByURI(invalid) error = %v", err)
	}
	if invalid.ValidationStatus != validation.StatusInvalid {
		t.Fatalf("invalid status = %q, want %q", invalid.ValidationStatus, validation.StatusInvalid)
	}
	builder := schema.NewBuilder(registry)
	gqlSchema, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	repoCtx := resolver.WithRepositories(ctx, &resolver.Repositories{Records: db.Records, Actors: db.Actors, Lexicons: db.Lexicons})
	result := graphql.Do(graphql.Params{
		Schema: *gqlSchema,
		RequestString: `{
			appCertifiedActorProfile(first: 10) {
				edges { node { uri displayName } }
			}
		}`,
		Context: repoCtx,
	})
	edges := getEdges(t, result, "appCertifiedActorProfile")
	if len(edges) != 1 {
		t.Fatalf("edge count = %d, want 1", len(edges))
	}
	if got := getNodeField(t, edges[0], "uri"); got != validURI {
		t.Fatalf("uri = %v, want %s", got, validURI)
	}
	if got := getNodeField(t, edges[0], "displayName"); got != "Valid Profile" {
		t.Fatalf("displayName = %v, want Valid Profile", got)
	}

}

func TestFilterSort_FilterByStringEq(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(where: {title: {eq: "Test Record"}}) {
			edges {
				node {
					uri
					title
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	if len(edges) != 1 {
		t.Errorf("Expected 1 record, got %d", len(edges))
	}
	if len(edges) > 0 {
		uri := getNodeField(t, edges[0], "uri")
		if uri != "at://did:plc:test123/test.collection/1" {
			t.Errorf("Expected uri for 'Test Record', got %v", uri)
		}
	}
}

// TestFilterSort_FilterByStringContains tests filtering by string substring.
func TestFilterSort_FilterByStringContains(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(where: {title: {contains: "est"}}) {
			edges {
				node {
					uri
					title
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	// "Test Record" and "Searchable Content" both contain "est" (case-insensitive in SQLite)
	// Actually: "Test Record" contains "est", "Searchable Content" does not
	// Let's check: "Test" -> "est" yes, "Searchable" -> no, "Other" -> no, "Another" -> no, "No Optional" -> no
	// Only "Test Record" matches
	if len(edges) != 1 {
		t.Errorf("Expected 1 record containing 'est', got %d", len(edges))
		for _, e := range edges {
			t.Logf("  edge: %v", getNodeField(t, e, "title"))
		}
	}
}

// TestFilterSort_FilterByIntegerGtLt tests filtering by integer range.
func TestFilterSort_FilterByIntegerGtLt(t *testing.T) {
	env := setupFilterTestEnv(t)

	// score > 5 AND score < 10 → should match score=7 and score=8
	query := `{
		testCollection(where: {score: {gt: 5, lt: 10}}) {
			edges {
				node {
					uri
					score
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	if len(edges) != 2 {
		t.Errorf("Expected 2 records with score > 5 and < 10, got %d", len(edges))
		for _, e := range edges {
			t.Logf("  edge score: %v", getNodeField(t, e, "score"))
		}
	}

	// Verify scores are in range
	for _, edge := range edges {
		score := toInt(getNodeField(t, edge, "score"))
		if score <= 5 || score >= 10 {
			t.Errorf("Score %d is not in range (5, 10)", score)
		}
	}
}

// TestFilterSort_FilterByDID tests filtering by DID (author).
func TestFilterSort_FilterByDID(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(where: {did: {eq: "did:plc:test123"}}) {
			edges {
				node {
					uri
					did
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	// did:plc:test123 has 3 records
	if len(edges) != 3 {
		t.Errorf("Expected 3 records for did:plc:test123, got %d", len(edges))
	}

	for _, edge := range edges {
		did := getNodeField(t, edge, "did")
		if did != "did:plc:test123" {
			t.Errorf("Expected did:plc:test123, got %v", did)
		}
	}
}

// TestFilterSort_FilterByIsNull tests filtering by null field.
func TestFilterSort_FilterByIsNull(t *testing.T) {
	env := setupFilterTestEnv(t)

	// All 5 records don't have optionalField set, so isNull: true should return all
	query := `{
		testCollection(where: {optionalField: {isNull: true}}) {
			edges {
				node {
					uri
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	// All records have no optionalField in their JSON
	if len(edges) != 5 {
		t.Errorf("Expected 5 records with optionalField isNull, got %d", len(edges))
	}
}

func TestFilterSort_FilterByComplexPresence(t *testing.T) {
	env := setupFilterTestEnv(t)

	presenceRecords := []*repositories.Record{
		{
			URI:        "at://did:plc:presence/test.collection/missing",
			CID:        "presence-cid-missing",
			DID:        "did:plc:presence",
			Collection: "test.collection",
			JSON:       `{"title":"Presence Missing","score":1,"createdAt":"2026-02-01T10:00:00Z","active":true}`,
			RKey:       "missing",
		},
		{
			URI:        "at://did:plc:presence/test.collection/null",
			CID:        "presence-cid-null",
			DID:        "did:plc:presence",
			Collection: "test.collection",
			JSON:       `{"title":"Presence Null","score":2,"createdAt":"2026-02-02T10:00:00Z","active":true,"image":null,"contributors":null}`,
			RKey:       "null",
		},
		{
			URI:        "at://did:plc:presence/test.collection/object-array",
			CID:        "presence-cid-object-array",
			DID:        "did:plc:presence",
			Collection: "test.collection",
			JSON:       `{"title":"Presence Object Array","score":3,"createdAt":"2026-02-03T10:00:00Z","active":true,"image":{"ref":{"$link":"bafyreiimage"},"mimeType":"image/png","size":1},"contributors":["did:plc:alice"]}`,
			RKey:       "object-array",
		},
		{
			URI:        "at://did:plc:presence/test.collection/empty-object-array",
			CID:        "presence-cid-empty-object-array",
			DID:        "did:plc:presence",
			Collection: "test.collection",
			JSON:       `{"title":"Presence Empty Object Array","score":4,"createdAt":"2026-02-04T10:00:00Z","active":true,"image":{},"contributors":[]}`,
			RKey:       "empty-object-array",
		},
	}
	if err := env.db.Records.BatchInsert(env.ctx, presenceRecords); err != nil {
		t.Fatalf("Failed to insert presence test records: %v", err)
	}
	markRecordsValid(t, env.ctx, env.db, presenceRecords)

	tests := []struct {
		name     string
		where    string
		wantURIs []string
	}{
		{
			name:  "image present matches object and empty object",
			where: `did: {eq: "did:plc:presence"}, image: {isNull: false}`,
			wantURIs: []string{
				"at://did:plc:presence/test.collection/object-array",
				"at://did:plc:presence/test.collection/empty-object-array",
			},
		},
		{
			name:  "image null matches missing and explicit null",
			where: `did: {eq: "did:plc:presence"}, image: {isNull: true}`,
			wantURIs: []string{
				"at://did:plc:presence/test.collection/missing",
				"at://did:plc:presence/test.collection/null",
			},
		},
		{
			name:  "contributors present matches array and empty array",
			where: `did: {eq: "did:plc:presence"}, contributors: {isNull: false}`,
			wantURIs: []string{
				"at://did:plc:presence/test.collection/object-array",
				"at://did:plc:presence/test.collection/empty-object-array",
			},
		},
		{
			name:  "contributors null matches missing and explicit null",
			where: `did: {eq: "did:plc:presence"}, contributors: {isNull: true}`,
			wantURIs: []string{
				"at://did:plc:presence/test.collection/missing",
				"at://did:plc:presence/test.collection/null",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := fmt.Sprintf(`{
				testCollection(where: {%s}) {
					edges { node { uri } }
				}
			}`, tt.where)
			result := env.runQuery(query)
			edges := getEdges(t, result, "testCollection")
			assertEdgeURIs(t, edges, tt.wantURIs)
		})
	}
}

// TestFilterSort_SortByFieldASC tests sorting by a field in ascending order.
func TestFilterSort_SortByFieldASC(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(sortBy: title, sortDirection: ASC) {
			edges {
				node {
					title
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	if len(edges) != 5 {
		t.Fatalf("Expected 5 records, got %d", len(edges))
	}

	// Verify ascending order by title
	titles := make([]string, len(edges))
	for i, edge := range edges {
		titles[i] = fmt.Sprintf("%v", getNodeField(t, edge, "title"))
	}

	for i := 1; i < len(titles); i++ {
		if titles[i] < titles[i-1] {
			t.Errorf("Records not in ASC order: %q comes after %q", titles[i], titles[i-1])
		}
	}
}

// TestFilterSort_SortByIndexedAtDESC tests sorting by indexed_at in descending order.
func TestFilterSort_SortByIndexedAtDESC(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(sortBy: indexed_at, sortDirection: DESC) {
			edges {
				node {
					uri
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	if len(edges) != 5 {
		t.Fatalf("Expected 5 records, got %d", len(edges))
	}

	// Newest first: record 5 (2026-01-05), 4 (2026-01-04), 3 (2026-01-03), 2 (2026-01-02), 1 (2026-01-01)
	expectedURIs := []string{
		"at://did:plc:other456/test.collection/2",
		"at://did:plc:test123/test.collection/3",
		"at://did:plc:other456/test.collection/1",
		"at://did:plc:test123/test.collection/2",
		"at://did:plc:test123/test.collection/1",
	}

	for i, edge := range edges {
		uri := getNodeField(t, edge, "uri")
		if uri != expectedURIs[i] {
			t.Errorf("Edge %d: expected URI %s, got %v", i, expectedURIs[i], uri)
		}
	}
}

// TestFilterSort_SortWithPagination tests sort + cursor-based pagination.
func TestFilterSort_SortWithPagination(t *testing.T) {
	env := setupFilterTestEnv(t)

	// First page: get 2 records sorted by indexed_at ASC
	firstPageQuery := `{
		testCollection(sortBy: indexed_at, sortDirection: ASC, first: 2) {
			edges {
				cursor
				node {
					uri
				}
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}`

	result := env.runQuery(firstPageQuery)
	if len(result.Errors) > 0 {
		t.Fatalf("First page errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})

	if len(edges) != 2 {
		t.Fatalf("Expected 2 edges on first page, got %d", len(edges))
	}

	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	if !hasNextPage {
		t.Error("Expected hasNextPage to be true")
	}

	endCursor, _ := pageInfo["endCursor"].(string)
	if endCursor == "" {
		t.Fatal("Expected non-empty endCursor")
	}

	// Verify first page has oldest records (ASC order)
	firstURI := getNodeField(t, edges[0], "uri")
	if firstURI != "at://did:plc:test123/test.collection/1" {
		t.Errorf("Expected oldest record first, got %v", firstURI)
	}

	// Second page: use cursor from first page
	secondPageQuery := fmt.Sprintf(`{
		testCollection(sortBy: indexed_at, sortDirection: ASC, first: 2, after: "%s") {
			edges {
				node {
					uri
				}
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
			}
		}
	}`, endCursor)

	result2 := env.runQuery(secondPageQuery)
	if len(result2.Errors) > 0 {
		t.Fatalf("Second page errors: %v", result2.Errors)
	}

	data2 := result2.Data.(map[string]interface{})
	conn2 := data2["testCollection"].(map[string]interface{})
	edges2 := conn2["edges"].([]interface{})
	pageInfo2 := conn2["pageInfo"].(map[string]interface{})

	if len(edges2) != 2 {
		t.Fatalf("Expected 2 edges on second page, got %d", len(edges2))
	}

	hasPreviousPage, _ := pageInfo2["hasPreviousPage"].(bool)
	if !hasPreviousPage {
		t.Error("Expected hasPreviousPage to be true on second page")
	}

	// Second page should have records 3 and 4 (by indexed_at ASC)
	secondURI := getNodeField(t, edges2[0], "uri")
	if secondURI != "at://did:plc:other456/test.collection/1" {
		t.Errorf("Expected 3rd oldest record on second page, got %v", secondURI)
	}
}

// TestFilterSort_TotalCountOptIn tests that totalCount is returned when requested.
func TestFilterSort_TotalCountOptIn(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection {
			edges {
				node { uri }
			}
			totalCount
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})

	totalCount, ok := conn["totalCount"]
	if !ok {
		t.Fatal("Expected totalCount in response")
	}

	count := toInt(totalCount)
	if count != 5 {
		t.Errorf("Expected totalCount = 5, got %d", count)
	}
}

// TestFilterSort_TotalCountOmitted tests that totalCount is null when not requested.
func TestFilterSort_TotalCountOmitted(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Query WITHOUT selecting totalCount
	query := `{
		testCollection {
			edges {
				node { uri }
			}
			pageInfo {
				hasNextPage
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})

	// totalCount should not be present (or be nil) when not requested
	totalCount, exists := conn["totalCount"]
	if exists && totalCount != nil {
		t.Errorf("Expected totalCount to be nil when not requested, got %v", totalCount)
	}
}

// TestFilterSort_MaxPageSize tests that requests above the max page size are clamped.
func TestFilterSort_MaxPageSize(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Insert enough records to test clamping (we have 5, so just verify we get at most 1000).
	query := `{
		testCollection(first: 1500) {
			edges {
				node { uri }
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	// We only have 5 records, but the query should be clamped to 1000 max.
	// Since we have fewer than 1000, we get all 5.
	if len(edges) > 1000 {
		t.Errorf("Expected at most 1000 records, got %d", len(edges))
	}

	// Verify we get all 5 records (since 5 < 100)
	if len(edges) != 5 {
		t.Errorf("Expected 5 records (all available), got %d", len(edges))
	}
}

// TestFilterSort_Search tests basic text search.
func TestFilterSort_Search(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		search(query: "Searchable") {
			edges {
				node {
					uri
					collection
				}
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["search"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 1 {
		t.Errorf("Expected 1 search result for 'Searchable', got %d", len(edges))
	}

	if len(edges) > 0 {
		uri := getNodeField(t, edges[0], "uri")
		if uri != "at://did:plc:test123/test.collection/3" {
			t.Errorf("Expected URI for 'Searchable Content', got %v", uri)
		}
	}
}

// TestFilterSort_SearchWithCollectionFilter tests search with collection filter.
func TestFilterSort_SearchWithCollectionFilter(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Insert a record in a different collection that also matches the search term
	otherRecord := []*repositories.Record{
		{
			URI:        "at://did:plc:test123/other.collection/1",
			CID:        "cid_other",
			DID:        "did:plc:test123",
			Collection: "other.collection",
			JSON:       `{"content": "Searchable in other collection"}`,
			RKey:       "1",
		},
	}
	if err := env.db.Records.BatchInsert(env.ctx, otherRecord); err != nil {
		t.Fatalf("Failed to insert other collection record: %v", err)
	}

	// Search with collection filter — should only return test.collection results
	query := `{
		search(query: "Searchable", collection: "test.collection") {
			edges {
				node {
					uri
					collection
				}
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["search"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	// Should only find the test.collection record, not the other.collection one
	for _, edge := range edges {
		coll := getNodeField(t, edge, "collection")
		if coll != "test.collection" {
			t.Errorf("Expected collection 'test.collection', got %v", coll)
		}
	}

	if len(edges) != 1 {
		t.Errorf("Expected 1 result in test.collection, got %d", len(edges))
	}
}

// TestFilterSort_BackwardCompatibility tests that queries without where/sort args work as before.
func TestFilterSort_BackwardCompatibility(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Query without any where/sort args — should return all records in default order (indexed_at DESC)
	query := `{
		testCollection {
			edges {
				node {
					uri
				}
			}
			pageInfo {
				hasNextPage
				hasPreviousPage
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})

	// Should return all 5 records
	if len(edges) != 5 {
		t.Errorf("Expected 5 records, got %d", len(edges))
	}

	// Default order is indexed_at DESC (newest first)
	firstURI := getNodeField(t, edges[0], "uri")
	if firstURI != "at://did:plc:other456/test.collection/2" {
		t.Errorf("Expected newest record first (indexed_at DESC), got %v", firstURI)
	}

	// No pagination cursors used, so hasPreviousPage should be false
	hasPreviousPage, _ := pageInfo["hasPreviousPage"].(bool)
	if hasPreviousPage {
		t.Error("Expected hasPreviousPage to be false for first page")
	}

	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	if hasNextPage {
		t.Error("Expected hasNextPage to be false when all records fit on one page")
	}
}

// TestFilterSort_EmptyResults tests that filters returning no results work correctly.
func TestFilterSort_EmptyResults(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		testCollection(where: {title: {eq: "NonExistentTitle"}}) {
			edges {
				node { uri }
			}
			pageInfo {
				hasNextPage
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 0 {
		t.Errorf("Expected 0 records for non-existent title, got %d", len(edges))
	}

	pageInfo := conn["pageInfo"].(map[string]interface{})
	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	if hasNextPage {
		t.Error("Expected hasNextPage to be false for empty results")
	}
}

// TestFilterSort_SearchShortQueryError tests that search with < 3 runes returns an error.
func TestFilterSort_SearchShortQueryError(t *testing.T) {
	env := setupFilterTestEnv(t)

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "empty query",
			query: `{ search(query: "") { edges { node { uri } } } }`,
		},
		{
			name:  "single char query",
			query: `{ search(query: "a") { edges { node { uri } } } }`,
		},
		{
			name:  "two char query",
			query: `{ search(query: "ab") { edges { node { uri } } } }`,
		},
		{
			name:  "two multi-byte char query",
			query: `{ search(query: "éé") { edges { node { uri } } } }`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := env.runQuery(tt.query)
			if len(result.Errors) == 0 {
				t.Error("Expected error for short search query, got none")
			}
		})
	}
}

// TestFilterSort_TotalCountWithFilter tests totalCount with active filters.
func TestFilterSort_TotalCountWithFilter(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Filter by DID and request totalCount
	query := `{
		testCollection(where: {did: {eq: "did:plc:test123"}}) {
			edges {
				node { uri }
			}
			totalCount
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["testCollection"].(map[string]interface{})

	totalCount := toInt(conn["totalCount"])
	if totalCount != 3 {
		t.Errorf("Expected totalCount = 3 for did:plc:test123, got %d", totalCount)
	}

	edges := conn["edges"].([]interface{})
	if len(edges) != 3 {
		t.Errorf("Expected 3 edges, got %d", len(edges))
	}
}

// TestFilterSort_TotalCountAggregateOverflow tests that large filtered totalCount queries fail cleanly.
func TestFilterSort_TotalCountAggregateOverflow(t *testing.T) {
	env := setupFilterTestEnv(t)

	fields := []string{"title", "optionalField", "field1", "field2", "field3", "field4", "field5", "field6", "field7", "field8"}
	clauses := make([]string, len(fields))
	for i, field := range fields {
		clauses[i] = fmt.Sprintf("%s: {in: [%s]}", field, buildQuotedValues(field, 100))
	}

	query := fmt.Sprintf(`{
		testCollection(where: {%s}) {
			totalCount
		}
	}`, strings.Join(clauses, ", "))

	result := env.runQuery(query)
	if len(result.Errors) == 0 {
		t.Fatal("Expected GraphQL error for aggregate parameter overflow, got none")
	}

	message := result.Errors[0].Message
	if !strings.Contains(message, "sqlite query parameter count exceeds maximum allowed") {
		t.Fatalf("Expected application aggregate-limit error, got %q", message)
	}
	if strings.Contains(message, "too many SQL variables") {
		t.Fatalf("Expected application-level error, got raw SQLite error %q", message)
	}
}

// TestFilterSort_GenericRecordsQuery tests the generic records query still works.
func TestFilterSort_GenericRecordsQuery(t *testing.T) {
	env := setupFilterTestEnv(t)

	query := `{
		records(collection: "test.collection", first: 10) {
			edges {
				node {
					uri
					collection
					did
				}
			}
			pageInfo {
				hasNextPage
			}
		}
	}`

	result := env.runQuery(query)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["records"].(map[string]interface{})
	edges := conn["edges"].([]interface{})

	if len(edges) != 5 {
		t.Errorf("Expected 5 records from generic query, got %d", len(edges))
	}

	// Verify all records are from the correct collection
	for _, edge := range edges {
		coll := getNodeField(t, edge, "collection")
		if coll != "test.collection" {
			t.Errorf("Expected collection 'test.collection', got %v", coll)
		}
	}
}

// TestFilterSort_MultipleFiltersANDed tests that multiple filter fields are ANDed.
func TestFilterSort_MultipleFiltersANDed(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Filter by DID AND score > 5 — should return records from did:plc:test123 with score > 5
	// did:plc:test123 records: score=7, score=3, score=8
	// After score > 5: score=7 and score=8
	query := `{
		testCollection(where: {did: {eq: "did:plc:test123"}, score: {gt: 5}}) {
			edges {
				node {
					uri
					score
					did
				}
			}
		}
	}`

	result := env.runQuery(query)
	edges := getEdges(t, result, "testCollection")

	if len(edges) != 2 {
		t.Errorf("Expected 2 records (DID filter AND score > 5), got %d", len(edges))
		for _, e := range edges {
			t.Logf("  uri=%v score=%v did=%v", getNodeField(t, e, "uri"), getNodeField(t, e, "score"), getNodeField(t, e, "did"))
		}
	}

	for _, edge := range edges {
		did := getNodeField(t, edge, "did")
		score := toInt(getNodeField(t, edge, "score"))
		if did != "did:plc:test123" {
			t.Errorf("Expected did:plc:test123, got %v", did)
		}
		if score <= 5 {
			t.Errorf("Expected score > 5, got %d", score)
		}
	}
}

// TestFilterSort_SearchPagination tests pagination on search results.
func TestFilterSort_SearchPagination(t *testing.T) {
	env := setupFilterTestEnv(t)

	// Search for "Record" which appears in multiple titles
	// "Test Record", "Another Record", "Other User Record"
	firstPageQuery := `{
		search(query: "Record", first: 2) {
			edges {
				cursor
				node { uri }
			}
			pageInfo {
				hasNextPage
				endCursor
			}
		}
	}`

	result := env.runQuery(firstPageQuery)
	if len(result.Errors) > 0 {
		t.Fatalf("First page errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	conn := data["search"].(map[string]interface{})
	edges := conn["edges"].([]interface{})
	pageInfo := conn["pageInfo"].(map[string]interface{})

	if len(edges) != 2 {
		t.Fatalf("Expected 2 edges on first page, got %d", len(edges))
	}

	hasNextPage, _ := pageInfo["hasNextPage"].(bool)
	if !hasNextPage {
		t.Error("Expected hasNextPage to be true")
	}

	endCursor, _ := pageInfo["endCursor"].(string)
	if endCursor == "" {
		t.Fatal("Expected non-empty endCursor")
	}

	// Second page
	secondPageQuery := fmt.Sprintf(`{
		search(query: "Record", first: 2, after: "%s") {
			edges {
				node { uri }
			}
			pageInfo {
				hasNextPage
			}
		}
	}`, endCursor)

	result2 := env.runQuery(secondPageQuery)
	if len(result2.Errors) > 0 {
		t.Fatalf("Second page errors: %v", result2.Errors)
	}

	data2 := result2.Data.(map[string]interface{})
	conn2 := data2["search"].(map[string]interface{})
	edges2 := conn2["edges"].([]interface{})

	// Should have 1 remaining record
	if len(edges2) != 1 {
		t.Errorf("Expected 1 edge on second page, got %d", len(edges2))
	}
}

// TestFilterSort_IndependentTests verifies that each test case is independent.
// This test inserts its own records and verifies isolation.
func TestFilterSort_IndependentTests(t *testing.T) {
	// Each call to setupFilterTestEnv creates a fresh DB
	env1 := setupFilterTestEnv(t)
	env2 := setupFilterTestEnv(t)

	// Both environments should have exactly 5 records
	query := `{ testCollection { edges { node { uri } } } }`

	result1 := env1.runQuery(query)
	result2 := env2.runQuery(query)

	edges1 := getEdges(t, result1, "testCollection")
	edges2 := getEdges(t, result2, "testCollection")

	if len(edges1) != 5 {
		t.Errorf("env1: expected 5 records, got %d", len(edges1))
	}
	if len(edges2) != 5 {
		t.Errorf("env2: expected 5 records, got %d", len(edges2))
	}

	// Modifying env1 should not affect env2
	newRecord := []*repositories.Record{
		{
			URI:        "at://did:plc:extra/test.collection/1",
			CID:        "cid_extra",
			DID:        "did:plc:extra",
			Collection: "test.collection",
			JSON:       `{"title": "Extra Record", "score": 1}`,
			RKey:       "1",
		},
	}
	if err := env1.db.Records.BatchInsert(env1.ctx, newRecord); err != nil {
		t.Fatalf("Failed to insert extra record: %v", err)
	}
	markRecordsValid(t, env1.ctx, env1.db, newRecord)

	result1After := env1.runQuery(query)
	result2After := env2.runQuery(query)

	edges1After := getEdges(t, result1After, "testCollection")
	edges2After := getEdges(t, result2After, "testCollection")

	if len(edges1After) != 6 {
		t.Errorf("env1 after insert: expected 6 records, got %d", len(edges1After))
	}
	if len(edges2After) != 5 {
		t.Errorf("env2 after env1 insert: expected 5 records (unchanged), got %d", len(edges2After))
	}
}

// TestFilterSort_TableDriven is a table-driven test covering multiple filter scenarios.
func TestFilterSort_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		wantEdgeCount int
		wantErrors    bool
	}{
		{
			name: "no filters returns all",
			query: `{
				testCollection {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 5,
		},
		{
			name: "filter by active true",
			query: `{
				testCollection(where: {active: {eq: true}}) {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 3, // records 1, 3, 4 have active: true
		},
		{
			name: "filter by active false",
			query: `{
				testCollection(where: {active: {eq: false}}) {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 2, // records 2, 5 have active: false
		},
		{
			name: "filter by score gte 15",
			query: `{
				testCollection(where: {score: {gte: 15}}) {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 2, // score=15 and score=20
		},
		{
			name: "filter by score lte 3",
			query: `{
				testCollection(where: {score: {lte: 3}}) {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 1, // score=3
		},
		{
			name: "filter by title startsWith",
			query: `{
				testCollection(where: {title: {startsWith: "Test"}}) {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 1, // "Test Record"
		},
		{
			name: "search with valid query",
			query: `{
				search(query: "Other") {
					edges { node { uri } }
				}
			}`,
			// SQLite LIKE is case-insensitive: "Other User Record" and "Another Record" both match "Other"
			wantEdgeCount: 2,
		},
		{
			name: "search returns empty for no match",
			query: `{
				search(query: "zzznomatch") {
					edges { node { uri } }
				}
			}`,
			wantEdgeCount: 0,
		},
		{
			name:       "search with empty query returns error",
			query:      `{ search(query: "") { edges { node { uri } } } }`,
			wantErrors: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupFilterTestEnv(t)
			result := env.runQuery(tt.query)

			if tt.wantErrors {
				if len(result.Errors) == 0 {
					t.Error("Expected errors but got none")
				}
				return
			}

			if len(result.Errors) > 0 {
				t.Fatalf("Unexpected errors: %v", result.Errors)
			}

			// Determine the connection field name
			data := result.Data.(map[string]interface{})
			var edges []interface{}
			for _, v := range data {
				if conn, ok := v.(map[string]interface{}); ok {
					if e, ok := conn["edges"].([]interface{}); ok {
						edges = e
						break
					}
				}
			}

			if len(edges) != tt.wantEdgeCount {
				t.Errorf("Expected %d edges, got %d", tt.wantEdgeCount, len(edges))
				for _, e := range edges {
					if em, ok := e.(map[string]interface{}); ok {
						if node, ok := em["node"].(map[string]interface{}); ok {
							t.Logf("  uri=%v", node["uri"])
						}
					}
				}
			}
		})
	}
}

// TestFilterSort_SearchCaseInsensitive tests that search is case-insensitive.
func TestFilterSort_SearchCaseInsensitive(t *testing.T) {
	env := setupFilterTestEnv(t)

	tests := []struct {
		name  string
		query string
	}{
		{name: "lowercase", query: "searchable"},
		{name: "uppercase", query: "SEARCHABLE"},
		{name: "mixed case", query: "Searchable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gqlQuery := fmt.Sprintf(`{
				search(query: "%s") {
					edges {
						node { uri }
					}
				}
			}`, tt.query)

			result := env.runQuery(gqlQuery)
			if len(result.Errors) > 0 {
				t.Fatalf("GraphQL errors: %v", result.Errors)
			}

			data := result.Data.(map[string]interface{})
			conn := data["search"].(map[string]interface{})
			edges := conn["edges"].([]interface{})

			// SQLite LIKE is case-insensitive for ASCII characters
			if len(edges) != 1 {
				t.Errorf("Expected 1 result for case-insensitive search %q, got %d", tt.query, len(edges))
			}
		})
	}
}
