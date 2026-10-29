//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// requiredCreatedAtLexiconJSON declares createdAt as a required datetime, which
// lets validated typed queries read createdAt from record_created_at.
const requiredCreatedAtLexiconJSON = `{
	"lexicon": 1,
	"id": "test.timeline",
	"defs": {
		"main": {
			"type": "record",
			"key": "tid",
			"record": {
				"type": "object",
				"required": ["createdAt"],
				"properties": {
					"title": {"type": "string"},
					"createdAt": {"type": "string", "format": "datetime"}
				}
			}
		}
	}
}`

const (
	timelineURI1 = "at://did:plc:t/test.timeline/1"
	timelineURI2 = "at://did:plc:t/test.timeline/2"
	timelineURI3 = "at://did:plc:t/test.timeline/3"
)

func setupCreatedAtColumnEnv(t *testing.T) *filterTestEnv {
	t.Helper()
	db := setupTestDB(t)
	ctx := context.Background()

	registry := lexicon.NewRegistry()
	lex, err := lexicon.Parse(requiredCreatedAtLexiconJSON)
	if err != nil {
		t.Fatalf("parse lexicon: %v", err)
	}
	registry.Register(lex)

	records := []*repositories.Record{
		{URI: timelineURI1, CID: "c1", DID: "did:plc:t", Collection: "test.timeline", RKey: "1", JSON: `{"title":"one","createdAt":"2026-01-01T10:00:00Z"}`},
		// Sub-millisecond precision: the column stores 10:00:00.000Z.
		{URI: timelineURI2, CID: "c2", DID: "did:plc:t", Collection: "test.timeline", RKey: "2", JSON: `{"title":"two","createdAt":"2026-01-02T10:00:00.000500Z"}`},
		// Non-UTC offset: the column stores 2026-01-03T10:00:00.000Z.
		{URI: timelineURI3, CID: "c3", DID: "did:plc:t", Collection: "test.timeline", RKey: "3", JSON: `{"title":"three","createdAt":"2026-01-03T12:00:00+02:00"}`},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("insert records: %v", err)
	}
	markRecordsValid(t, ctx, db, records)

	gqlSchema, err := schema.NewBuilder(registry).Build()
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	repoCtx := resolver.WithRepositories(ctx, &resolver.Repositories{Records: db.Records, Actors: db.Actors, Lexicons: db.Lexicons})
	return &filterTestEnv{db: db, schema: gqlSchema, ctx: repoCtx}
}

func timelineConnection(t *testing.T, result *graphql.Result) (uris []string, endCursor string) {
	t.Helper()
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}
	conn := result.Data.(map[string]interface{})["testTimeline"].(map[string]interface{})
	for _, edge := range conn["edges"].([]interface{}) {
		uris = append(uris, getNodeField(t, edge, "uri").(string))
	}
	endCursor, _ = conn["pageInfo"].(map[string]interface{})["endCursor"].(string)
	return uris, endCursor
}

func legacyCursor(values ...string) string {
	raw, _ := json.Marshal(values)
	return base64.URLEncoding.EncodeToString(raw)
}

func TestCreatedAtColumn_SortPagination(t *testing.T) {
	env := setupCreatedAtColumnEnv(t)
	page := func(after string) string {
		afterArg := ""
		if after != "" {
			afterArg = fmt.Sprintf(`, after: %q`, after)
		}
		return fmt.Sprintf(`{ testTimeline(sortBy: createdAt, sortDirection: DESC, first: 2%s) {
			edges { node { uri } } pageInfo { endCursor } } }`, afterArg)
	}

	first, cursor := timelineConnection(t, env.runQuery(page("")))
	assertStrings(t, "first page", first, []string{timelineURI3, timelineURI2})
	second, _ := timelineConnection(t, env.runQuery(page(cursor)))
	assertStrings(t, "second page", second, []string{timelineURI1})

	// A cursor issued before this change carries the raw JSON createdAt,
	// including sub-millisecond precision. It must not repeat record 2.
	legacy, _ := timelineConnection(t, env.runQuery(page(legacyCursor("2026-01-02T10:00:00.000500Z", timelineURI2))))
	assertStrings(t, "legacy cursor page", legacy, []string{timelineURI1})

	bad := env.runQuery(page(legacyCursor("not-a-date", timelineURI2)))
	if len(bad.Errors) == 0 || !strings.Contains(bad.Errors[0].Message, "restart pagination") {
		t.Fatalf("invalid createdAt cursor errors = %v, want restart pagination guidance", bad.Errors)
	}
}

func TestCreatedAtColumn_WhereFilters(t *testing.T) {
	env := setupCreatedAtColumnEnv(t)
	query := func(where string) string {
		return fmt.Sprintf(`{ testTimeline(sortBy: createdAt, sortDirection: ASC, where: %s) {
			edges { node { uri } } pageInfo { endCursor } } }`, where)
	}

	// Full datetimes use the column; the +02:00 bound equals 10:00Z.
	got, _ := timelineConnection(t, env.runQuery(query(`{createdAt: {gte: "2026-01-02T12:00:00+02:00", lte: "2026-01-03T10:00:00Z"}}`)))
	assertStrings(t, "column range", got, []string{timelineURI2, timelineURI3})

	// Date-only values keep the existing JSON string comparison.
	got, _ = timelineConnection(t, env.runQuery(query(`{createdAt: {gte: "2026-01-02"}}`)))
	assertStrings(t, "date-only range", got, []string{timelineURI2, timelineURI3})
}

// optionalCreatedAtLexiconJSON declares createdAt as an optional datetime, so
// records can be valid without one.
const optionalCreatedAtLexiconJSON = `{
	"lexicon": 1,
	"id": "test.sparse",
	"defs": {
		"main": {
			"type": "record",
			"key": "tid",
			"record": {
				"type": "object",
				"properties": {
					"title": {"type": "string"},
					"createdAt": {"type": "string", "format": "datetime"}
				}
			}
		}
	}
}`

// sparseURIs maps record names to URIs. s3 has no createdAt and s4 has one that
// is not a datetime, so neither gets a record_created_at.
var sparseURIs = map[string]string{
	"s1": "at://did:plc:t/test.sparse/s1",
	"s2": "at://did:plc:t/test.sparse/s2",
	"s3": "at://did:plc:t/test.sparse/s3",
	"s4": "at://did:plc:t/test.sparse/s4",
}

func setupSparseCreatedAtEnv(t *testing.T, requireValid bool) *filterTestEnv {
	t.Helper()
	db := setupTestDB(t)
	ctx := context.Background()

	registry := lexicon.NewRegistry()
	lex, err := lexicon.Parse(optionalCreatedAtLexiconJSON)
	if err != nil {
		t.Fatalf("parse lexicon: %v", err)
	}
	registry.Register(lex)

	records := []*repositories.Record{
		{URI: sparseURIs["s1"], CID: "c1", DID: "did:plc:t", Collection: "test.sparse", RKey: "s1", JSON: `{"title":"one","createdAt":"2026-03-01T00:00:00Z"}`},
		{URI: sparseURIs["s2"], CID: "c2", DID: "did:plc:t", Collection: "test.sparse", RKey: "s2", JSON: `{"title":"two","createdAt":"2026-03-02T00:00:00Z"}`},
		{URI: sparseURIs["s3"], CID: "c3", DID: "did:plc:t", Collection: "test.sparse", RKey: "s3", JSON: `{"title":"three"}`},
		{URI: sparseURIs["s4"], CID: "c4", DID: "did:plc:t", Collection: "test.sparse", RKey: "s4", JSON: `{"title":"four","createdAt":"not-a-date"}`},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("insert records: %v", err)
	}
	if requireValid {
		markRecordsValid(t, ctx, db, records)
	}

	gqlSchema, err := schema.NewBuilder(registry, schema.WithTypedRecordsRequireValid(requireValid)).Build()
	if err != nil {
		t.Fatalf("build schema: %v", err)
	}
	repoCtx := resolver.WithRepositories(ctx, &resolver.Repositories{Records: db.Records, Actors: db.Actors, Lexicons: db.Lexicons})
	return &filterTestEnv{db: db, schema: gqlSchema, ctx: repoCtx}
}

func sparseConnection(t *testing.T, result *graphql.Result) (uris []string, startCursor, endCursor string) {
	t.Helper()
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}
	conn := result.Data.(map[string]interface{})["testSparse"].(map[string]interface{})
	for _, edge := range conn["edges"].([]interface{}) {
		uris = append(uris, getNodeField(t, edge, "uri").(string))
	}
	pageInfo := conn["pageInfo"].(map[string]interface{})
	startCursor, _ = pageInfo["startCursor"].(string)
	endCursor, _ = pageInfo["endCursor"].(string)
	return uris, startCursor, endCursor
}

// Whenever the Lexicon declares createdAt as a datetime, sorting uses
// record_created_at, required or not and validated or not. Records without a
// parseable createdAt stay in the connection as the oldest, and Relay cursors
// taken on them keep paging in both directions.
func TestCreatedAtColumn_OptionalCreatedAtKeepsUndatedRecords(t *testing.T) {
	for _, mode := range []struct {
		name         string
		requireValid bool
	}{
		{"validated reads", true},
		{"unvalidated reads", false},
	} {
		t.Run(mode.name, func(t *testing.T) {
			env := setupSparseCreatedAtEnv(t, mode.requireValid)
			s1, s2, s3, s4 := sparseURIs["s1"], sparseURIs["s2"], sparseURIs["s3"], sparseURIs["s4"]

			tests := []struct {
				direction string
				want      []string
			}{
				// Undated records are the oldest: last when DESC, first when ASC,
				// ordered by uri in the sort direction.
				{"DESC", []string{s2, s1, s4, s3}},
				{"ASC", []string{s3, s4, s1, s2}},
			}
			for _, tt := range tests {
				t.Run(tt.direction, func(t *testing.T) {
					for _, pageSize := range []int{1, 3} {
						var forward []string
						after := ""
						for i := 0; i < 10; i++ {
							afterArg := ""
							if after != "" {
								afterArg = fmt.Sprintf(`, after: %q`, after)
							}
							page, _, end := sparseConnection(t, env.runQuery(fmt.Sprintf(`{ testSparse(sortBy: createdAt, sortDirection: %s, first: %d%s) {
								edges { node { uri } } pageInfo { startCursor endCursor } } }`, tt.direction, pageSize, afterArg)))
							if len(page) == 0 {
								break
							}
							forward = append(forward, page...)
							after = end
						}
						assertStrings(t, fmt.Sprintf("forward pages of %d", pageSize), forward, tt.want)

						var backward []string
						before := ""
						for i := 0; i < 10; i++ {
							beforeArg := ""
							if before != "" {
								beforeArg = fmt.Sprintf(`, before: %q`, before)
							}
							page, start, _ := sparseConnection(t, env.runQuery(fmt.Sprintf(`{ testSparse(sortBy: createdAt, sortDirection: %s, last: %d%s) {
								edges { node { uri } } pageInfo { startCursor endCursor } } }`, tt.direction, pageSize, beforeArg)))
							if len(page) == 0 {
								break
							}
							backward = append(append([]string{}, page...), backward...)
							before = start
						}
						assertStrings(t, fmt.Sprintf("backward pages of %d", pageSize), backward, tt.want)
					}
				})
			}
		})
	}
}

// A cursor from before this change that sat on a record with no createdAt
// carried an empty value. It keeps working as a cursor on the undated rows.
func TestCreatedAtColumn_EmptyCursorValueContinuesAmongUndatedRecords(t *testing.T) {
	env := setupSparseCreatedAtEnv(t, true)
	got, _, _ := sparseConnection(t, env.runQuery(fmt.Sprintf(`{ testSparse(sortBy: createdAt, sortDirection: DESC, first: 5, after: %q) {
		edges { node { uri } } pageInfo { startCursor endCursor } } }`, legacyCursor("", sparseURIs["s4"]))))
	assertStrings(t, "after undated s4", got, []string{sparseURIs["s3"]})
}

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
