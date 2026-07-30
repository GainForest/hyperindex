//go:build integration

package repositories_test

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestActorsRepository_MissingHandleReconciliation_PostgreSQL(t *testing.T) {
	databaseURL, ok := safePostgresTestDatabaseURL(t)
	if !ok {
		t.Skip("PostgreSQL actor reconciliation test requires DATABASE_URL pointing at a postgres database named test or ending with _test/-test")
	}

	db := testutil.SetupTestDBWithURL(t, databaseURL)
	repo := db.Actors
	ctx := context.Background()
	suffix := strconv.FormatInt(time.Now().UnixNano(), 36)
	blankDID := "did:plc:zzzzpr128blank" + suffix
	placeholderDID := "did:plc:zzzzpr128placeholder" + suffix
	validDID := "did:plc:zzzzpr128valid" + suffix
	actors := []repositories.ActorData{
		{DID: blankDID, Handle: "   "},
		{DID: placeholderDID, Handle: placeholderDID},
		{DID: validDID, Handle: "valid.example"},
	}
	t.Cleanup(func() {
		for _, actor := range actors {
			if err := repo.DeleteByDID(context.Background(), actor.DID); err != nil {
				t.Errorf("cleanup actor %s: %v", actor.DID, err)
			}
		}
	})
	for _, actor := range actors {
		if err := repo.UpsertIdentity(ctx, actor.DID, actor.Handle); err != nil {
			t.Fatalf("UpsertIdentity(%s) error = %v", actor.DID, err)
		}
	}

	dids, err := repo.ListDIDsMissingHandle(ctx, "did:plc:zzzzpr128", 100)
	if err != nil {
		t.Fatalf("ListDIDsMissingHandle() error = %v", err)
	}
	missing := make(map[string]bool, len(dids))
	for _, did := range dids {
		missing[did] = true
	}
	if !missing[blankDID] || !missing[placeholderDID] {
		t.Fatalf("missing handles = %v, want blank and placeholder DIDs", dids)
	}
	if missing[validDID] {
		t.Fatalf("missing handles = %v, did not want valid DID", dids)
	}

	for _, test := range []struct {
		did        string
		handle     string
		wantUpdate bool
	}{
		{did: blankDID, handle: "blank.example", wantUpdate: true},
		{did: placeholderDID, handle: "placeholder.example", wantUpdate: true},
		{did: validDID, handle: "stale.example", wantUpdate: false},
	} {
		updated, err := repo.SetHandleIfMissing(ctx, test.did, test.handle)
		if err != nil {
			t.Fatalf("SetHandleIfMissing(%s) error = %v", test.did, err)
		}
		if updated != test.wantUpdate {
			t.Fatalf("SetHandleIfMissing(%s) = %v, want %v", test.did, updated, test.wantUpdate)
		}
	}

	valid, err := repo.GetByDID(ctx, validDID)
	if err != nil {
		t.Fatalf("GetByDID(valid) error = %v", err)
	}
	if valid.Handle != "valid.example" {
		t.Fatalf("valid handle = %q, want valid.example", valid.Handle)
	}
}
