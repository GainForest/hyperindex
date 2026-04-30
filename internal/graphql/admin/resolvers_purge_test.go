package admin

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestResolver_PurgeActor_TrimsDID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	repos := &Repositories{
		Records: db.Records,
		Actors:  db.Actors,
	}
	r := NewResolver(repos, "did:plc:test-labeler", nil)

	if err := db.Actors.Upsert(ctx, "did:plc:target", "target.bsky.social"); err != nil {
		t.Fatalf("failed to seed actor: %v", err)
	}
	if _, err := db.Records.Insert(ctx,
		"at://did:plc:target/app.certified.actor.profile/1",
		"cid1",
		"did:plc:target",
		"app.certified.actor.profile",
		`{"displayName":"Target"}`,
	); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}

	ok, err := r.PurgeActor(ctx, "  did:plc:target  ", "PURGE")
	if err != nil {
		t.Fatalf("PurgeActor() error = %v", err)
	}
	if !ok {
		t.Fatal("PurgeActor() = false, want true")
	}

	records, err := db.Records.GetByDID(ctx, "did:plc:target")
	if err != nil {
		t.Fatalf("GetByDID error: %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("expected records purged, got %d", len(records))
	}
}
