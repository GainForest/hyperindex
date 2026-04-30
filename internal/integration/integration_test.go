//go:build integration

// Package integration provides end-to-end integration tests for hyperindex.
//
// Run with: go test -tags=integration -v ./internal/integration/...
package integration

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
	"github.com/GainForest/hyperindex/internal/graphql/admin"

	"github.com/graphql-go/graphql"
)

// testDB holds test database resources.
type testDB struct {
	Executor     database.Executor
	Records      *repositories.RecordsRepository
	Actors       *repositories.ActorsRepository
	Config       *repositories.ConfigRepository
	Lexicons     *repositories.LexiconsRepository
	OAuthClients *repositories.OAuthClientsRepository
}

// setupTestDB creates an in-memory SQLite database with migrations applied.
func setupTestDB(t *testing.T) *testDB {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		exec.Close()
		t.Fatalf("Failed to run migrations: %v", err)
	}

	db := &testDB{
		Executor:     exec,
		Records:      repositories.NewRecordsRepository(exec),
		Actors:       repositories.NewActorsRepository(exec),
		Config:       repositories.NewConfigRepository(exec),
		Lexicons:     repositories.NewLexiconsRepository(exec),
		OAuthClients: repositories.NewOAuthClientsRepository(exec),
	}

	t.Cleanup(func() {
		exec.Close()
	})

	return db
}

// seedTestData seeds the test database with sample data.
func (db *testDB) seedTestData(t *testing.T, ctx context.Context) {
	t.Helper()

	actors := []struct {
		did    string
		handle string
	}{
		{"did:plc:user1", "user1.bsky.social"},
		{"did:plc:user2", "user2.bsky.social"},
		{"did:plc:admin1", "admin.example.com"},
	}

	for _, a := range actors {
		if err := db.Actors.Upsert(ctx, a.did, a.handle); err != nil {
			t.Fatalf("Failed to seed actor %s: %v", a.did, err)
		}
	}

	configs := map[string]string{
		"domain_authority": "example.com",
		"relay_url":        "wss://relay.example.com",
	}

	for key, value := range configs {
		if err := db.Config.Set(ctx, key, value); err != nil {
			t.Fatalf("Failed to seed config %s: %v", key, err)
		}
	}

	records := []*repositories.Record{
		{
			URI:        "at://did:plc:user1/example.post/1",
			CID:        "bafyreiabc123",
			DID:        "did:plc:user1",
			Collection: "example.post",
			JSON:       `{"text": "Hello world", "$type": "example.post"}`,
			RKey:       "1",
		},
		{
			URI:        "at://did:plc:user1/example.post/2",
			CID:        "bafyreidef456",
			DID:        "did:plc:user1",
			Collection: "example.post",
			JSON:       `{"text": "Second post", "$type": "example.post"}`,
			RKey:       "2",
		},
		{
			URI:        "at://did:plc:user2/example.post/1",
			CID:        "bafyreighi789",
			DID:        "did:plc:user2",
			Collection: "example.post",
			JSON:       `{"text": "User 2 post", "$type": "example.post"}`,
			RKey:       "1",
		},
	}

	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("Failed to seed records: %v", err)
	}
}

// executeQuery executes a GraphQL query.
func executeQuery(schema *graphql.Schema, query string, ctx context.Context) *graphql.Result {
	return graphql.Do(graphql.Params{
		Schema:        *schema,
		RequestString: query,
		Context:       ctx,
	})
}

// ========== Records Repository Tests ==========

func TestRecords_BatchInsert(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	records := []*repositories.Record{
		{URI: "at://did:plc:test/col/1", CID: "cid1", DID: "did:plc:test", Collection: "col", JSON: `{"test":1}`, RKey: "1"},
		{URI: "at://did:plc:test/col/2", CID: "cid2", DID: "did:plc:test", Collection: "col", JSON: `{"test":2}`, RKey: "2"},
		{URI: "at://did:plc:test/col/3", CID: "cid3", DID: "did:plc:test", Collection: "col", JSON: `{"test":3}`, RKey: "3"},
	}

	err := db.Records.BatchInsert(ctx, records)
	if err != nil {
		t.Fatalf("BatchInsert failed: %v", err)
	}

	count, err := db.Records.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount failed: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected 3 records, got %d", count)
	}
}

func TestRecords_GetCIDsByURIs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	records := []*repositories.Record{
		{URI: "at://did:plc:test/col/1", CID: "cid1", DID: "did:plc:test", Collection: "col", JSON: `{}`, RKey: "1"},
		{URI: "at://did:plc:test/col/2", CID: "cid2", DID: "did:plc:test", Collection: "col", JSON: `{}`, RKey: "2"},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("BatchInsert failed: %v", err)
	}

	cidMap, err := db.Records.GetCIDsByURIs(ctx, []string{
		"at://did:plc:test/col/1",
		"at://did:plc:test/col/2",
		"at://did:plc:test/col/nonexistent",
	})
	if err != nil {
		t.Fatalf("GetCIDsByURIs failed: %v", err)
	}

	if len(cidMap) != 2 {
		t.Errorf("Expected 2 CIDs, got %d", len(cidMap))
	}
	if cidMap["at://did:plc:test/col/1"] != "cid1" {
		t.Errorf("Expected cid1, got %s", cidMap["at://did:plc:test/col/1"])
	}
}

func TestRecords_GetExistingCIDs(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	records := []*repositories.Record{
		{URI: "at://did:plc:test/col/1", CID: "existingcid1", DID: "did:plc:test", Collection: "col", JSON: `{}`, RKey: "1"},
		{URI: "at://did:plc:test/col/2", CID: "existingcid2", DID: "did:plc:test", Collection: "col", JSON: `{}`, RKey: "2"},
	}
	if err := db.Records.BatchInsert(ctx, records); err != nil {
		t.Fatalf("BatchInsert failed: %v", err)
	}

	existingSet, err := db.Records.GetExistingCIDs(ctx, []string{"existingcid1", "newcid", "existingcid2"})
	if err != nil {
		t.Fatalf("GetExistingCIDs failed: %v", err)
	}

	if len(existingSet) != 2 {
		t.Errorf("Expected 2 existing CIDs, got %d", len(existingSet))
	}
	if !existingSet["existingcid1"] {
		t.Error("Expected existingcid1 to exist")
	}
	if existingSet["newcid"] {
		t.Error("Expected newcid to NOT exist")
	}
}

func TestRecords_Deduplication(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Insert initial records
	initial := []*repositories.Record{
		{URI: "at://did:plc:test/col/1", CID: "cidA", DID: "did:plc:test", Collection: "col", JSON: `{"v":1}`, RKey: "1"},
		{URI: "at://did:plc:test/col/2", CID: "cidB", DID: "did:plc:test", Collection: "col", JSON: `{"v":2}`, RKey: "2"},
	}
	if err := db.Records.BatchInsert(ctx, initial); err != nil {
		t.Fatalf("Initial insert failed: %v", err)
	}

	// Backfill scenario
	backfillRecords := []*repositories.Record{
		// Unchanged - same URI and CID
		{URI: "at://did:plc:test/col/1", CID: "cidA", DID: "did:plc:test", Collection: "col", JSON: `{"v":1}`, RKey: "1"},
		// Updated - same URI, different CID
		{URI: "at://did:plc:test/col/2", CID: "cidBnew", DID: "did:plc:test", Collection: "col", JSON: `{"v":2.1}`, RKey: "2"},
		// New record
		{URI: "at://did:plc:test/col/3", CID: "cidC", DID: "did:plc:test", Collection: "col", JSON: `{"v":3}`, RKey: "3"},
		// New URI but duplicate CID
		{URI: "at://did:plc:other/col/1", CID: "cidA", DID: "did:plc:other", Collection: "col", JSON: `{"v":1}`, RKey: "1"},
	}

	// Get dedup info
	uris := make([]string, len(backfillRecords))
	cidSet := make(map[string]bool)
	var cids []string
	for i, r := range backfillRecords {
		uris[i] = r.URI
		if !cidSet[r.CID] {
			cids = append(cids, r.CID)
			cidSet[r.CID] = true
		}
	}

	existingByURI, _ := db.Records.GetCIDsByURIs(ctx, uris)
	existingCIDs, _ := db.Records.GetExistingCIDs(ctx, cids)

	// Filter records
	var filtered []*repositories.Record
	var skipped int
	for _, rec := range backfillRecords {
		if existingCID, ok := existingByURI[rec.URI]; ok {
			if existingCID == rec.CID {
				skipped++
				continue
			}
			if existingCIDs[rec.CID] {
				skipped++
				continue
			}
		} else {
			if existingCIDs[rec.CID] {
				skipped++
				continue
			}
		}
		filtered = append(filtered, rec)
	}

	t.Logf("Total: %d, Filtered: %d, Skipped: %d", len(backfillRecords), len(filtered), skipped)

	if skipped != 2 {
		t.Errorf("Expected 2 skipped, got %d", skipped)
	}
	if len(filtered) != 2 {
		t.Errorf("Expected 2 filtered, got %d", len(filtered))
	}

	// Insert filtered
	if len(filtered) > 0 {
		if err := db.Records.BatchInsert(ctx, filtered); err != nil {
			t.Fatalf("BatchInsert filtered failed: %v", err)
		}
	}

	count, _ := db.Records.GetCount(ctx)
	if count != 3 {
		t.Errorf("Expected 3 records after dedup, got %d", count)
	}
}

func TestRecords_LargeBatch(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create 250 records
	records := make([]*repositories.Record, 250)
	for i := 0; i < 250; i++ {
		records[i] = &repositories.Record{
			URI:        "at://did:plc:test/col/" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			CID:        "cid" + string(rune('A'+i%26)) + string(rune('0'+i/26)),
			DID:        "did:plc:test",
			Collection: "col",
			JSON:       `{}`,
			RKey:       string(rune('A'+i%26)) + string(rune('0'+i/26)),
		}
	}

	err := db.Records.BatchInsert(ctx, records)
	if err != nil {
		t.Fatalf("BatchInsert large batch failed: %v", err)
	}

	count, _ := db.Records.GetCount(ctx)
	if count != 250 {
		t.Errorf("Expected 250 records, got %d", count)
	}
}

// ========== Admin GraphQL Tests ==========

// buildAdminSchema creates the admin GraphQL schema for testing.
func buildAdminSchema(db *testDB) (*graphql.Schema, error) {
	repos := &admin.Repositories{
		Records:      db.Records,
		Actors:       db.Actors,
		Lexicons:     db.Lexicons,
		Config:       db.Config,
		OAuthClients: db.OAuthClients,
	}
	resolver := admin.NewResolver(repos, "did:plc:test-labeler", []string{"did:plc:admin1"})
	builder := admin.NewSchemaBuilder(resolver)
	return builder.Build()
}

func TestAdminGraphQL_Statistics(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	db.seedTestData(t, ctx)

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})

	query := `{
		statistics {
			recordCount
			actorCount
			lexiconCount
		}
	}`

	result := executeQuery(schema, query, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	stats := data["statistics"].(map[string]interface{})

	// GraphQL returns ints as int or int64, handle both
	recordCount := toInt(stats["recordCount"])
	actorCount := toInt(stats["actorCount"])

	if recordCount != 3 {
		t.Errorf("Expected 3 records, got %d", recordCount)
	}
	if actorCount != 3 {
		t.Errorf("Expected 3 actors, got %d", actorCount)
	}
}

// toInt converts GraphQL numeric values to int.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func TestAdminGraphQL_Settings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	db.seedTestData(t, ctx)

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})

	query := `{ settings { domainAuthority adminDids } }`

	result := executeQuery(schema, query, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	settings := data["settings"].(map[string]interface{})

	if settings["domainAuthority"] != "example.com" {
		t.Errorf("Expected 'example.com', got '%v'", settings["domainAuthority"])
	}

	adminDids, ok := settings["adminDids"].([]interface{})
	if !ok {
		t.Fatalf("Expected adminDids list, got %T", settings["adminDids"])
	}
	if len(adminDids) != 1 || adminDids[0] != "did:plc:admin1" {
		t.Fatalf("Expected env-backed adminDids, got %v", adminDids)
	}
}

func TestAdminGraphQL_SettingsUsesEnvAdminDIDsInsteadOfDB(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()
	db.seedTestData(t, ctx)

	if err := db.Config.Set(ctx, "admin_dids", "did:plc:stale-admin"); err != nil {
		t.Fatalf("Failed to seed stale admin_dids config: %v", err)
	}

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})
	result := executeQuery(schema, `{ settings { adminDids } }`, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	settings := data["settings"].(map[string]interface{})
	adminDids := settings["adminDids"].([]interface{})
	if len(adminDids) != 1 || adminDids[0] != "did:plc:admin1" {
		t.Fatalf("Expected env adminDids to win over DB value, got %v", adminDids)
	}
}

func TestAdminGraphQL_UpdateSettings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})

	mutation := `mutation {
		updateSettings(domainAuthority: "newdomain.com") {
			domainAuthority
		}
	}`

	result := executeQuery(schema, mutation, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	settings := data["updateSettings"].(map[string]interface{})

	if settings["domainAuthority"] != "newdomain.com" {
		t.Errorf("Expected 'newdomain.com', got '%v'", settings["domainAuthority"])
	}
}

func TestAdminGraphQL_RequiresAdmin(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	userCtx := admin.ContextWithAuth(ctx, "did:plc:user1", "user.bsky.social", false, []string{"did:plc:admin1"})

	mutation := `mutation {
		updateSettings(domainAuthority: "hacked.com") {
			domainAuthority
		}
	}`

	result := executeQuery(schema, mutation, userCtx)
	if len(result.Errors) == 0 {
		t.Error("Expected error for non-admin mutation")
	}
}

func TestAdminGraphQL_CurrentSession(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	userCtx := admin.ContextWithAuth(ctx, "did:plc:user1", "user.bsky.social", false, []string{"did:plc:admin1"})

	query := `{ currentSession { did handle isAdmin } }`

	result := executeQuery(schema, query, userCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	session := data["currentSession"].(map[string]interface{})

	if session["did"] != "did:plc:user1" {
		t.Errorf("Expected 'did:plc:user1', got '%v'", session["did"])
	}
	if session["isAdmin"] != false {
		t.Errorf("Expected isAdmin false")
	}
}

func TestAdminGraphQL_OAuthClients(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})

	// Create - using individual arguments, not input object
	createMutation := `mutation {
		createOAuthClient(
			clientName: "Test App"
			clientType: "public"
			redirectUris: ["https://testapp.example.com/callback"]
		) {
			clientId
			clientName
		}
	}`

	result := executeQuery(schema, createMutation, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors on create: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	createdClient := data["createOAuthClient"].(map[string]interface{})
	clientId := createdClient["clientId"].(string)

	// Query
	query := `{ oauthClients { clientId } }`
	result = executeQuery(schema, query, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors on query: %v", result.Errors)
	}

	data = result.Data.(map[string]interface{})
	clients := data["oauthClients"].([]interface{})
	if len(clients) != 1 {
		t.Errorf("Expected 1 client, got %d", len(clients))
	}

	// Delete - use the generated clientId
	deleteMutation := `mutation { deleteOAuthClient(clientId: "` + clientId + `") }`
	result = executeQuery(schema, deleteMutation, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors on delete: %v", result.Errors)
	}

	// Verify deletion
	result = executeQuery(schema, query, adminCtx)
	data = result.Data.(map[string]interface{})
	clients = data["oauthClients"].([]interface{})
	if len(clients) != 0 {
		t.Errorf("Expected 0 clients after delete, got %d", len(clients))
	}
}

func TestAdminGraphQL_PurgeActor_Success(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Seed target + control actor/records
	if err := db.Actors.Upsert(ctx, "did:plc:target", "target.bsky.social"); err != nil {
		t.Fatalf("failed to seed target actor: %v", err)
	}
	if err := db.Actors.Upsert(ctx, "did:plc:other", "other.bsky.social"); err != nil {
		t.Fatalf("failed to seed other actor: %v", err)
	}

	if _, err := db.Records.Insert(ctx,
		"at://did:plc:target/app.certified.actor.profile/1",
		"cid-target-1",
		"did:plc:target",
		"app.certified.actor.profile",
		`{"displayName":"Target"}`,
	); err != nil {
		t.Fatalf("failed to seed target record: %v", err)
	}
	if _, err := db.Records.Insert(ctx,
		"at://did:plc:other/app.certified.actor.profile/1",
		"cid-other-1",
		"did:plc:other",
		"app.certified.actor.profile",
		`{"displayName":"Other"}`,
	); err != nil {
		t.Fatalf("failed to seed other record: %v", err)
	}

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})

	mutation := `mutation {
		purgeActor(did: "did:plc:target", confirm: "PURGE")
	}`

	result := executeQuery(schema, mutation, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	purged, ok := data["purgeActor"].(bool)
	if !ok || !purged {
		t.Fatalf("purgeActor returned %v, want true", data["purgeActor"])
	}

	targetRecords, err := db.Records.GetByDID(ctx, "did:plc:target")
	if err != nil {
		t.Fatalf("GetByDID(target) error: %v", err)
	}
	if len(targetRecords) != 0 {
		t.Fatalf("expected target records purged, got %d", len(targetRecords))
	}

	_, err = db.Actors.GetByDID(ctx, "did:plc:target")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected target actor deleted (sql.ErrNoRows), got %v", err)
	}

	otherRecords, err := db.Records.GetByDID(ctx, "did:plc:other")
	if err != nil {
		t.Fatalf("GetByDID(other) error: %v", err)
	}
	if len(otherRecords) != 1 {
		t.Fatalf("expected other DID records retained, got %d", len(otherRecords))
	}

	if _, err := db.Actors.GetByDID(ctx, "did:plc:other"); err != nil {
		t.Fatalf("expected other actor retained, got error: %v", err)
	}
}

func TestAdminGraphQL_PurgeActor_RequiresAdmin(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	userCtx := admin.ContextWithAuth(ctx, "did:plc:user1", "user.bsky.social", false, []string{"did:plc:admin1"})
	mutation := `mutation {
		purgeActor(did: "did:plc:target", confirm: "PURGE")
	}`

	result := executeQuery(schema, mutation, userCtx)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for non-admin purgeActor")
	}
}

func TestAdminGraphQL_PurgeActor_InvalidConfirm(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})
	mutation := `mutation {
		purgeActor(did: "did:plc:target", confirm: "RESET")
	}`

	result := executeQuery(schema, mutation, adminCtx)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for invalid confirm value")
	}
}

func TestAdminGraphQL_PurgeActor_InvalidDID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})
	mutation := `mutation {
		purgeActor(did: "not-a-did", confirm: "PURGE")
	}`

	result := executeQuery(schema, mutation, adminCtx)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for invalid DID format")
	}
}

func TestAdminGraphQL_PurgeActorPreview(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if err := db.Actors.Upsert(ctx, "did:plc:target", "target.bsky.social"); err != nil {
		t.Fatalf("failed to seed actor: %v", err)
	}
	if _, err := db.Records.Insert(ctx,
		"at://did:plc:target/app.certified.actor.profile/1",
		"cid-target-1",
		"did:plc:target",
		"app.certified.actor.profile",
		`{"displayName":"Target"}`,
	); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	adminCtx := admin.ContextWithAuth(ctx, "did:plc:admin1", "admin.example.com", true, []string{"did:plc:admin1"})
	query := `query {
		purgeActorPreview(did: "did:plc:target") {
			did
			isValidDid
			actorExists
			recordCount
		}
	}`

	result := executeQuery(schema, query, adminCtx)
	if len(result.Errors) > 0 {
		t.Fatalf("GraphQL errors: %v", result.Errors)
	}

	data := result.Data.(map[string]interface{})
	preview := data["purgeActorPreview"].(map[string]interface{})
	if preview["did"] != "did:plc:target" {
		t.Fatalf("did = %v, want did:plc:target", preview["did"])
	}
	if preview["isValidDid"] != true {
		t.Fatalf("isValidDid = %v, want true", preview["isValidDid"])
	}
	if preview["actorExists"] != true {
		t.Fatalf("actorExists = %v, want true", preview["actorExists"])
	}
	if toInt(preview["recordCount"]) != 1 {
		t.Fatalf("recordCount = %v, want 1", preview["recordCount"])
	}
}

func TestAdminGraphQL_PurgeActorPreview_RequiresAdmin(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	schema, err := buildAdminSchema(db)
	if err != nil {
		t.Fatalf("Failed to build schema: %v", err)
	}

	userCtx := admin.ContextWithAuth(ctx, "did:plc:user1", "user.bsky.social", false, []string{"did:plc:admin1"})
	query := `query { purgeActorPreview(did: "did:plc:target") { did } }`

	result := executeQuery(schema, query, userCtx)
	if len(result.Errors) == 0 {
		t.Fatal("expected error for non-admin purgeActorPreview")
	}
}
