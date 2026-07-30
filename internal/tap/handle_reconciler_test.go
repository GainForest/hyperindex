package tap

import (
	"context"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/testutil"
)

type repoInfoClientFunc func(ctx context.Context, did string) (*RepoInfoResponse, error)

func (f repoInfoClientFunc) RepoInfo(ctx context.Context, did string) (*RepoInfoResponse, error) {
	return f(ctx, did)
}

func TestReconcileActorHandles(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	for _, did := range []string{"did:plc:alice", "did:plc:bob", "did:plc:missing", "did:plc:failed"} {
		if err := db.Actors.Ensure(ctx, did); err != nil {
			t.Fatalf("Ensure(%s) error = %v", did, err)
		}
	}
	if err := db.Actors.UpsertIdentity(ctx, "did:plc:resolved", "resolved.example"); err != nil {
		t.Fatalf("UpsertIdentity(resolved) error = %v", err)
	}

	client := repoInfoClientFunc(func(_ context.Context, did string) (*RepoInfoResponse, error) {
		switch did {
		case "did:plc:alice":
			return &RepoInfoResponse{DID: did, Handle: "alice.example"}, nil
		case "did:plc:bob":
			return &RepoInfoResponse{DID: did, Handle: "handle.invalid"}, nil
		case "did:plc:missing":
			return &RepoInfoResponse{DID: did}, nil
		case "did:plc:failed":
			return nil, errors.New("Tap unavailable")
		default:
			return nil, errors.New("unexpected DID")
		}
	})

	stats, err := ReconcileActorHandles(ctx, db.Actors, client)
	if err != nil {
		t.Fatalf("ReconcileActorHandles() error = %v", err)
	}
	if stats.Scanned != 4 || stats.Updated != 2 || stats.Unresolved != 1 || stats.Failed != 1 || stats.AlreadyResolved != 0 {
		t.Fatalf("ReconcileActorHandles() stats = %+v", stats)
	}

	alice, err := db.Actors.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("GetByDID(alice) error = %v", err)
	}
	if alice.Handle != "alice.example" {
		t.Fatalf("alice.Handle = %q, want alice.example", alice.Handle)
	}
	bob, err := db.Actors.GetByDID(ctx, "did:plc:bob")
	if err != nil {
		t.Fatalf("GetByDID(bob) error = %v", err)
	}
	if bob.Handle != "handle.invalid" {
		t.Fatalf("bob.Handle = %q, want handle.invalid", bob.Handle)
	}
}

func TestReconcileActorHandlesDoesNotOverwriteConcurrentIdentityEvent(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	did := "did:plc:alice"
	if err := db.Actors.Ensure(ctx, did); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	client := repoInfoClientFunc(func(ctx context.Context, gotDID string) (*RepoInfoResponse, error) {
		if err := db.Actors.UpsertIdentity(ctx, gotDID, "new.example"); err != nil {
			return nil, err
		}
		return &RepoInfoResponse{DID: gotDID, Handle: "stale.example"}, nil
	})
	stats, err := ReconcileActorHandles(ctx, db.Actors, client)
	if err != nil {
		t.Fatalf("ReconcileActorHandles() error = %v", err)
	}
	if stats.AlreadyResolved != 1 || stats.Updated != 0 {
		t.Fatalf("ReconcileActorHandles() stats = %+v", stats)
	}

	actor, err := db.Actors.GetByDID(ctx, did)
	if err != nil {
		t.Fatalf("GetByDID() error = %v", err)
	}
	if actor.Handle != "new.example" {
		t.Fatalf("actor.Handle = %q, want new.example", actor.Handle)
	}
}
