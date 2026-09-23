package repositories_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

// recordVersionTestDBs returns SQLite always, plus PostgreSQL when DATABASE_URL
// points at a safe test database.
func recordVersionTestDBs(t *testing.T) map[string]*testutil.TestDB {
	t.Helper()
	dbs := map[string]*testutil.TestDB{"sqlite": testutil.SetupTestDB(t)}
	if url, ok := safePostgresTestDatabaseURL(t); ok {
		dbs["postgres"] = testutil.SetupTestDBWithURL(t, url)
	}
	return dbs
}

func boolPtr(value bool) *bool { return &value }

func TestCollectionMatcher(t *testing.T) {
	m := repositories.NewCollectionMatcher(" app.gainforest.dwc.occurrence , org.hypercerts.* ,,")
	for collection, want := range map[string]bool{
		"app.gainforest.dwc.occurrence": true,
		"app.gainforest.dwc.dataset":    false,
		"org.hypercerts.collection":     true,
		"org.hypercertsx.collection":    false,
		"app.bsky.feed.post":            false,
	} {
		if got := m.Matches(collection); got != want {
			t.Errorf("Matches(%q) = %v, want %v", collection, got, want)
		}
	}
	if !repositories.NewCollectionMatcher("  ").Empty() {
		t.Error("blank list should disable history")
	}
}

func TestRecordVersions_AppendDedupesAndListsOldestFirst(t *testing.T) {
	for name, db := range recordVersionTestDBs(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := db.RecordVersions
			suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
			uri := "at://did:plc:alice/app.gainforest.dwc.occurrence/" + suffix
			write := func(action, cid string, body *string) {
				t.Helper()
				if err := repo.Append(ctx, repositories.RecordVersionWrite{
					URI: uri, CID: cid, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence",
					Action: action, JSON: body, Live: boolPtr(true),
				}); err != nil {
					t.Fatalf("Append(%s, %s): %v", action, cid, err)
				}
			}
			write(repositories.RecordVersionCreate, "cid1", strPtr(`{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`))
			write(repositories.RecordVersionCreate, "cid1", strPtr(`{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`)) // redelivery
			write(repositories.RecordVersionUpdate, "cid2", strPtr(`{"scientificName":"Hirundo tahitica","identifiedBy":"Maria"}`))
			write(repositories.RecordVersionDelete, "cid2", nil)

			versions, err := repo.ListByURI(ctx, uri, 0)
			if err != nil {
				t.Fatalf("ListByURI: %v", err)
			}
			if len(versions) != 3 {
				t.Fatalf("got %d versions, want 3 (redelivery deduped): %+v", len(versions), versions)
			}
			wantActions := []string{"create", "update", "delete"}
			for i, v := range versions {
				if v.Action != wantActions[i] {
					t.Errorf("version %d action = %q, want %q", i, v.Action, wantActions[i])
				}
				if v.ObservedAt.IsZero() {
					t.Errorf("version %d has no observedAt", i)
				}
			}
			if versions[1].JSON == nil || versions[1].CID != "cid2" {
				t.Errorf("update version = %+v", versions[1])
			}
			if versions[2].JSON != nil {
				t.Errorf("delete tombstone should have no body, got %q", *versions[2].JSON)
			}
			if versions[0].Live == nil || !*versions[0].Live {
				t.Errorf("live flag not kept: %+v", versions[0].Live)
			}
			if err := repo.Append(ctx, repositories.RecordVersionWrite{URI: uri, DID: "did:plc:alice", Collection: "x", Action: "baseline"}); err == nil {
				t.Error("Append should refuse the baseline action; baselines come from SeedBaseline")
			}
		})
	}
}

func TestRecordVersions_SeedBaselineIsIdempotentAndScoped(t *testing.T) {
	for name, db := range recordVersionTestDBs(t) {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
			tracked := "at://did:plc:bob/app.gainforest.dwc.occurrence/" + suffix
			untracked := "at://did:plc:bob/app.bsky.feed.post/" + suffix
			withHistory := "at://did:plc:bob/app.gainforest.dwc.occurrence/h" + suffix
			if err := db.Records.BatchInsert(ctx, []*repositories.Record{
				{URI: tracked, CID: "c-tracked", DID: "did:plc:bob", Collection: "app.gainforest.dwc.occurrence", JSON: `{"scientificName":"Quercus robur"}`},
				{URI: untracked, CID: "c-post", DID: "did:plc:bob", Collection: "app.bsky.feed.post", JSON: `{"text":"hi"}`},
				{URI: withHistory, CID: "c-new", DID: "did:plc:bob", Collection: "app.gainforest.dwc.occurrence", JSON: `{"scientificName":"Quercus petraea"}`},
			}); err != nil {
				t.Fatalf("BatchInsert: %v", err)
			}
			if err := db.RecordVersions.Append(ctx, repositories.RecordVersionWrite{
				URI: withHistory, CID: "c-new", DID: "did:plc:bob", Collection: "app.gainforest.dwc.occurrence",
				Action: repositories.RecordVersionCreate, JSON: strPtr(`{"scientificName":"Quercus petraea"}`), Live: boolPtr(true),
			}); err != nil {
				t.Fatalf("Append: %v", err)
			}

			matcher := repositories.NewCollectionMatcher("app.gainforest.*")
			if _, err := db.RecordVersions.SeedBaseline(ctx, matcher); err != nil {
				t.Fatalf("SeedBaseline: %v", err)
			}
			if _, err := db.RecordVersions.SeedBaseline(ctx, matcher); err != nil {
				t.Fatalf("SeedBaseline (second run): %v", err)
			}

			assertVersions := func(uri string, wantActions ...string) {
				t.Helper()
				versions, err := db.RecordVersions.ListByURI(ctx, uri, 0)
				if err != nil {
					t.Fatalf("ListByURI(%s): %v", uri, err)
				}
				if len(versions) != len(wantActions) {
					t.Fatalf("%s: got %d versions, want %v", uri, len(versions), wantActions)
				}
				for i, want := range wantActions {
					if versions[i].Action != want {
						t.Errorf("%s version %d = %q, want %q", uri, i, versions[i].Action, want)
					}
				}
			}
			assertVersions(tracked, "baseline")
			assertVersions(untracked)
			assertVersions(withHistory, "create")

			versions, _ := db.RecordVersions.ListByURI(ctx, tracked, 0)
			if versions[0].CID != "c-tracked" || versions[0].JSON == nil || versions[0].Live != nil {
				t.Errorf("baseline = %+v", versions[0])
			}
		})
	}
}
