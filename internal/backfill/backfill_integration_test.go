//go:build integration

// Package backfill provides integration tests for AT Protocol backfill operations.
//
// Run with: go test -tags=integration -v ./internal/backfill/...
package backfill

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/testutil"
	"github.com/bluesky-social/indigo/atproto/atdata"
)

const (
	// HypercertsActivityCollection is the collection used by the legacy integration tests.
	HypercertsActivityCollection = "org.hypercerts.claim.activity"

	// CertifiedActorProfileCollection is the default collection used for CAR backfill tests.
	CertifiedActorProfileCollection = "app.certified.actor.profile"

	// TestTimeout is the timeout for integration tests.
	TestTimeout = 2 * time.Minute
)

// TestListReposByCollection_HypercertsActivity tests discovering repos
// that have hypercerts.claim.activity records.
func TestListReposByCollection_HypercertsActivity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	client := NewClient("", "") // Use defaults

	repos, err := client.ListReposByCollection(ctx, HypercertsActivityCollection)
	if err != nil {
		t.Fatalf("ListReposByCollection failed: %v", err)
	}

	t.Logf("Found %d repos with %s records", len(repos), HypercertsActivityCollection)

	if len(repos) == 0 {
		t.Fatal("Expected to find at least one repo with hypercerts.claim.activity records")
	}

	// Log first few repos
	limit := 5
	if len(repos) < limit {
		limit = len(repos)
	}
	for i := 0; i < limit; i++ {
		t.Logf("  Repo %d: %s", i+1, repos[i])
	}
}

// TestResolveDID tests DID resolution via PLC directory.
func TestResolveDID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := NewClient("", "")

	// First, get a real DID from the collection
	repos, err := client.ListReposByCollection(ctx, HypercertsActivityCollection)
	if err != nil {
		t.Fatalf("ListReposByCollection failed: %v", err)
	}

	if len(repos) == 0 {
		t.Skip("No repos found for testing")
	}

	did := repos[0]
	t.Logf("Testing DID resolution for: %s", did)

	data, err := client.ResolveDID(ctx, did)
	if err != nil {
		t.Fatalf("ResolveDID failed: %v", err)
	}

	t.Logf("Resolved DID:")
	t.Logf("  DID: %s", data.DID)
	t.Logf("  Handle: %s", data.Handle)
	t.Logf("  PDS: %s", data.PDS)

	if data.DID != did {
		t.Errorf("DID mismatch: got %s, want %s", data.DID, did)
	}

	if data.PDS == "" {
		t.Error("Expected non-empty PDS URL")
	}
}

// TestListRecords_HypercertsActivity tests fetching actual records.
func TestListRecords_HypercertsActivity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	client := NewClient("", "")

	// Get repos
	repos, err := client.ListReposByCollection(ctx, HypercertsActivityCollection)
	if err != nil {
		t.Fatalf("ListReposByCollection failed: %v", err)
	}

	if len(repos) == 0 {
		t.Skip("No repos found for testing")
	}

	// Try to find a repo with records (some might be empty or have deleted records)
	var totalRecords int
	var successfulRepos int
	maxRepos := 10
	if len(repos) < maxRepos {
		maxRepos = len(repos)
	}

	for i := 0; i < maxRepos; i++ {
		did := repos[i]

		// Resolve DID to get PDS
		data, err := client.ResolveDID(ctx, did)
		if err != nil {
			t.Logf("Failed to resolve DID %s: %v", did, err)
			continue
		}

		// Fetch records
		records, err := client.ListRecords(ctx, data.PDS, did, HypercertsActivityCollection)
		if err != nil {
			t.Logf("Failed to list records for %s: %v", did, err)
			continue
		}

		if len(records) > 0 {
			successfulRepos++
			totalRecords += len(records)
			t.Logf("Repo %s (%s): %d records", did, data.Handle, len(records))

			// Log first record details
			rec := records[0]
			t.Logf("  First record URI: %s", rec.URI)
			t.Logf("  First record CID: %s", rec.CID)
			t.Logf("  First record value (truncated): %.200s...", string(rec.Value))
		}
	}

	t.Logf("\nSummary:")
	t.Logf("  Repos checked: %d", maxRepos)
	t.Logf("  Repos with records: %d", successfulRepos)
	t.Logf("  Total records found: %d", totalRecords)

	if totalRecords == 0 {
		t.Error("Expected to find at least some records")
	}
}

// TestBackfillClient_EndToEnd tests the full backfill flow without database.
func TestBackfillClient_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	client := NewClient("", "")

	t.Log("Step 1: Discovering repos...")
	repos, err := client.ListReposByCollection(ctx, HypercertsActivityCollection)
	if err != nil {
		t.Fatalf("Discovery failed: %v", err)
	}
	t.Logf("  Found %d repos", len(repos))

	if len(repos) == 0 {
		t.Skip("No repos to test")
	}

	// Limit to a few repos for the test
	testRepos := repos
	if len(testRepos) > 5 {
		testRepos = testRepos[:5]
	}

	t.Log("\nStep 2: Resolving DIDs and fetching records...")
	var totalRecords int
	for _, did := range testRepos {
		data, err := client.ResolveDID(ctx, did)
		if err != nil {
			t.Logf("  %s: resolve failed: %v", did, err)
			continue
		}

		records, err := client.ListRecords(ctx, data.PDS, did, HypercertsActivityCollection)
		if err != nil {
			t.Logf("  %s: list failed: %v", did, err)
			continue
		}

		totalRecords += len(records)
		t.Logf("  %s (%s): %d records from %s", did, data.Handle, len(records), data.PDS)
	}

	t.Logf("\nTotal records fetched: %d", totalRecords)

	if totalRecords == 0 {
		t.Error("Expected to fetch at least some records in end-to-end test")
	}
}

// TestGetRepo_CARRecordsConvertToATProtoJSON exercises the CAR-based backfill
// path and verifies raw record CBOR is converted to canonical AT Protocol JSON.
func TestGetRepo_CARRecordsConvertToATProtoJSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	client := NewClient("", "")
	collection := backfillIntegrationCollection()
	_, records := findRepoWithCARRecords(ctx, t, client, collection)

	jsonStr, err := CBORToJSON(records[0].Value)
	if err != nil {
		t.Fatalf("CBORToJSON() failed for %s: %v", records[0].URI, err)
	}
	if !json.Valid([]byte(jsonStr)) {
		t.Fatalf("CBORToJSON() returned invalid JSON: %s", jsonStr)
	}
	if _, err := atdata.UnmarshalJSON([]byte(jsonStr)); err != nil {
		t.Fatalf("CBORToJSON() returned JSON outside the AT Protocol data model: %v\n%s", err, jsonStr)
	}
}

// TestBackfillActor_CARPathInsertsRecords exercises the database-backed actor
// backfill flow, including CAR fetch, CBOR conversion, CID dedupe, and inserts.
func TestBackfillActor_CARPathInsertsRecords(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	client := NewClient("", "")
	collection := backfillIntegrationCollection()
	data, _ := findRepoWithCARRecords(ctx, t, client, collection)

	db := testutil.SetupTestDB(t)
	cfg := DefaultConfig()
	cfg.Collections = []string{collection}
	cfg.MaxHTTPConcurrent = 2
	cfg.MaxPDSWorkers = 1
	cfg.MaxConcurrentPerPDS = 1
	cfg.MaxConcurrentRepos = 1

	backfiller := NewBackfiller(cfg, db.Records, db.Actors, db.Activity)
	defer backfiller.Close()

	inserted, err := backfiller.BackfillActor(ctx, data.DID)
	if err != nil {
		t.Fatalf("BackfillActor(%q) failed: %v", data.DID, err)
	}
	if inserted == 0 {
		t.Fatalf("BackfillActor(%q) inserted 0 records, want > 0", data.DID)
	}

	records, err := db.Records.GetByDID(ctx, data.DID)
	if err != nil {
		t.Fatalf("GetByDID(%q) failed: %v", data.DID, err)
	}
	if len(records) == 0 {
		t.Fatalf("GetByDID(%q) returned 0 records after backfill", data.DID)
	}

	for _, record := range records {
		if record.Collection != collection {
			continue
		}
		if !json.Valid([]byte(record.JSON)) {
			t.Fatalf("inserted record %s has invalid JSON: %s", record.URI, record.JSON)
		}
		if _, err := atdata.UnmarshalJSON([]byte(record.JSON)); err != nil {
			t.Fatalf("inserted record %s has non-ATProto JSON: %v\n%s", record.URI, err, record.JSON)
		}
		return
	}

	t.Fatalf("BackfillActor(%q) inserted %d records but none for collection %s", data.DID, inserted, collection)
}

func backfillIntegrationCollection() string {
	if collection := os.Getenv("HYPERINDEX_BACKFILL_INTEGRATION_COLLECTION"); collection != "" {
		return collection
	}
	return CertifiedActorProfileCollection
}

func findRepoWithCARRecords(ctx context.Context, t *testing.T, client *Client, collection string) (*AtprotoData, []CARRecord) {
	t.Helper()

	if did := os.Getenv("HYPERINDEX_BACKFILL_INTEGRATION_DID"); did != "" {
		return fetchCARRecordsForDID(ctx, t, client, did, collection, true)
	}

	const maxReposToProbe = 20
	const maxPagesToProbe = 3

	cursor := ""
	checked := 0
	for page := 0; page < maxPagesToProbe && checked < maxReposToProbe; page++ {
		repos, nextCursor, err := client.listReposByCollectionPage(ctx, collection, cursor)
		if err != nil {
			t.Fatalf("list repos for collection %s: %v", collection, err)
		}
		if len(repos) == 0 {
			break
		}

		for _, did := range repos {
			if checked >= maxReposToProbe {
				break
			}
			checked++

			data, records := fetchCARRecordsForDID(ctx, t, client, did, collection, false)
			if len(records) > 0 {
				t.Logf("using %s (%s) with %d CAR records for %s", data.DID, data.Handle, len(records), collection)
				return data, records
			}
		}

		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	t.Fatalf("found no repo with CAR records for %s after checking %d repos", collection, checked)
	return nil, nil
}

func fetchCARRecordsForDID(ctx context.Context, t *testing.T, client *Client, did, collection string, failFast bool) (*AtprotoData, []CARRecord) {
	t.Helper()

	data, err := client.ResolveDID(ctx, did)
	if err != nil {
		if failFast {
			t.Fatalf("resolve DID %s: %v", did, err)
		}
		t.Logf("resolve DID %s failed: %v", did, err)
		return nil, nil
	}

	repoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	records, err := client.GetRepo(repoCtx, data.PDS, data.DID, []string{collection})
	if err != nil {
		if failFast {
			t.Fatalf("GetRepo(%s, %s) failed: %v", data.PDS, did, err)
		}
		t.Logf("GetRepo(%s, %s) failed: %v", data.PDS, did, err)
		return data, nil
	}
	if len(records) == 0 {
		if failFast {
			t.Fatalf("GetRepo(%s, %s) returned no records for %s", data.PDS, did, collection)
		}
		t.Logf("GetRepo(%s, %s) returned no records for %s", data.PDS, did, collection)
	}
	return data, records
}
