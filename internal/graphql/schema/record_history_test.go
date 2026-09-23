package schema

import (
	"context"
	"testing"

	"github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestRecordHistoryGraphQL(t *testing.T) {
	const uri = "at://did:plc:alice/app.gainforest.dwc.occurrence/occ1"
	schema := buildExternalLabelsTestSchema(t)
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	body1 := `{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`
	body2 := `{"scientificName":"Hirundo tahitica","identifiedBy":"Maria","previousIdentifications":"Hirundo rustica (ai:gemini-3.8-flash, 2026-09-22)"}`
	live := true
	for _, v := range []repositories.RecordVersionWrite{
		{URI: uri, CID: "cid1", DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", Action: repositories.RecordVersionCreate, JSON: &body1, Live: &live},
		{URI: uri, CID: "cid2", DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", Action: repositories.RecordVersionUpdate, JSON: &body2, Live: &live},
	} {
		if err := db.RecordVersions.Append(ctx, v); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	ctx = resolver.WithRepositories(ctx, &resolver.Repositories{Records: db.Records, RecordVersions: db.RecordVersions})

	result := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: `{ recordHistory(uri: "` + uri + `") { id cid action live observedAt value } missing: recordHistory(uri: "at://did:plc:none/x.y.z/1") { id } }`,
		Context:       ctx,
	})
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}
	data := result.Data.(map[string]interface{})
	versions := data["recordHistory"].([]interface{})
	if len(versions) != 2 {
		t.Fatalf("recordHistory length = %d, want 2", len(versions))
	}
	first := versions[0].(map[string]interface{})
	second := versions[1].(map[string]interface{})
	if first["action"] != "create" || second["action"] != "update" || second["cid"] != "cid2" || first["live"] != true {
		t.Fatalf("versions = %+v", versions)
	}
	value := second["value"].(map[string]interface{})
	if value["scientificName"] != "Hirundo tahitica" || value["previousIdentifications"] == nil {
		t.Fatalf("second value = %+v", value)
	}
	if first["observedAt"] == "" {
		t.Fatal("observedAt missing")
	}
	if missing := data["missing"].([]interface{}); len(missing) != 0 {
		t.Fatalf("unknown uri returned %v", missing)
	}

	// Paging: the second page starts after the first version's id.
	firstID := first["id"].(string)
	paged := graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: `{ recordHistory(uri: "` + uri + `", first: 1, after: "` + firstID + `") { cid } }`,
		Context:       ctx,
	})
	if len(paged.Errors) > 0 {
		t.Fatalf("paged GraphQL errors: %v", paged.Errors)
	}
	page := paged.Data.(map[string]interface{})["recordHistory"].([]interface{})
	if len(page) != 1 || page[0].(map[string]interface{})["cid"] != "cid2" {
		t.Fatalf("second page = %v, want cid2", page)
	}

	for _, bad := range []string{`first: 0`, `first: 501`, `after: "nope"`} {
		res := graphql.Do(graphql.Params{
			Schema:        *schema,
			RequestString: `{ recordHistory(uri: "` + uri + `", ` + bad + `) { id } }`,
			Context:       ctx,
		})
		if len(res.Errors) == 0 {
			t.Errorf("recordHistory(%s) should fail", bad)
		}
	}
}
