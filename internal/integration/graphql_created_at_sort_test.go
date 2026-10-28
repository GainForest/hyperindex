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

func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}
