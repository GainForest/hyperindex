package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupActorsTest(t *testing.T) *repositories.ActorsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Actors
}

func TestActorsRepository_Upsert(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Insert new actor
	err := repo.Upsert(ctx, "did:plc:testactor1", "alice.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	actor, err := repo.GetByDID(ctx, "did:plc:testactor1")
	if err != nil {
		t.Fatalf("failed to get actor after insert: %v", err)
	}
	if actor.DID != "did:plc:testactor1" {
		t.Errorf("DID = %q, want %q", actor.DID, "did:plc:testactor1")
	}
	if actor.Handle != "alice.bsky.social" {
		t.Errorf("Handle = %q, want %q", actor.Handle, "alice.bsky.social")
	}

	// Update handle via upsert
	err = repo.Upsert(ctx, "did:plc:testactor1", "alice-new.bsky.social")
	if err != nil {
		t.Fatalf("failed to upsert actor: %v", err)
	}

	actor, err = repo.GetByDID(ctx, "did:plc:testactor1")
	if err != nil {
		t.Fatalf("failed to get actor after upsert: %v", err)
	}
	if actor.Handle != "alice-new.bsky.social" {
		t.Errorf("Handle after upsert = %q, want %q", actor.Handle, "alice-new.bsky.social")
	}
}

func TestActorsRepository_BatchUpsert(t *testing.T) {
	tests := []struct {
		name   string
		actors []repositories.ActorData
		want   int64
	}{
		{
			name:   "empty slice",
			actors: []repositories.ActorData{},
			want:   0,
		},
		{
			name: "single actor",
			actors: []repositories.ActorData{
				{DID: "did:plc:testactor1", Handle: "alice.bsky.social"},
			},
			want: 1,
		},
		{
			name: "multiple actors",
			actors: []repositories.ActorData{
				{DID: "did:plc:testactor1", Handle: "alice.bsky.social"},
				{DID: "did:plc:testactor2", Handle: "bob.bsky.social"},
				{DID: "did:plc:testactor3", Handle: "carol.bsky.social"},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupActorsTest(t)
			ctx := context.Background()

			err := repo.BatchUpsert(ctx, tt.actors)
			if err != nil {
				t.Fatalf("BatchUpsert() error = %v", err)
			}

			count, err := repo.GetCount(ctx)
			if err != nil {
				t.Fatalf("GetCount() error = %v", err)
			}
			if count != tt.want {
				t.Errorf("GetCount() = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestActorsRepository_GetByDID(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Setup: insert an actor
	err := repo.Upsert(ctx, "did:plc:testactor1", "alice.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		actor, err := repo.GetByDID(ctx, "did:plc:testactor1")
		if err != nil {
			t.Fatalf("GetByDID() error = %v", err)
		}
		if actor.DID != "did:plc:testactor1" {
			t.Errorf("DID = %q, want %q", actor.DID, "did:plc:testactor1")
		}
		if actor.Handle != "alice.bsky.social" {
			t.Errorf("Handle = %q, want %q", actor.Handle, "alice.bsky.social")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByDID(ctx, "did:plc:nonexistent")
		if err == nil {
			t.Fatal("GetByDID() expected error for non-existing DID, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("GetByDID() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestActorsRepository_GetByHandle(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Setup: insert an actor
	err := repo.Upsert(ctx, "did:plc:testactor1", "alice.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		actor, err := repo.GetByHandle(ctx, "alice.bsky.social")
		if err != nil {
			t.Fatalf("GetByHandle() error = %v", err)
		}
		if actor.DID != "did:plc:testactor1" {
			t.Errorf("DID = %q, want %q", actor.DID, "did:plc:testactor1")
		}
		if actor.Handle != "alice.bsky.social" {
			t.Errorf("Handle = %q, want %q", actor.Handle, "alice.bsky.social")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByHandle(ctx, "nobody.bsky.social")
		if err == nil {
			t.Fatal("GetByHandle() expected error for non-existing handle, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("GetByHandle() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestActorsRepository_GetCount(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Empty database
	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("GetCount() on empty db = %d, want 0", count)
	}

	// After inserts
	err = repo.Upsert(ctx, "did:plc:testactor1", "alice.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}
	err = repo.Upsert(ctx, "did:plc:testactor2", "bob.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("GetCount() after 2 inserts = %d, want 2", count)
	}
}

func TestActorsRepository_DeleteAll(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Insert some actors
	err := repo.BatchUpsert(ctx, []repositories.ActorData{
		{DID: "did:plc:testactor1", Handle: "alice.bsky.social"},
		{DID: "did:plc:testactor2", Handle: "bob.bsky.social"},
	})
	if err != nil {
		t.Fatalf("BatchUpsert() error = %v", err)
	}

	// Verify they exist
	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("GetCount() = %d, want 2 before delete", count)
	}

	// Delete all
	err = repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}

	// Verify empty
	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("GetCount() after DeleteAll = %d, want 0", count)
	}
}

func TestActorsRepository_DeleteByDID(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, repo *repositories.ActorsRepository, ctx context.Context)
		deleteDID string
		wantErr   bool
		verify    func(t *testing.T, repo *repositories.ActorsRepository, ctx context.Context)
	}{
		{
			name: "deletes only target actor",
			setup: func(t *testing.T, repo *repositories.ActorsRepository, ctx context.Context) {
				if err := repo.Upsert(ctx, "did:plc:alice", "alice.bsky.social"); err != nil {
					t.Fatalf("failed to upsert alice: %v", err)
				}
				if err := repo.Upsert(ctx, "did:plc:bob", "bob.bsky.social"); err != nil {
					t.Fatalf("failed to upsert bob: %v", err)
				}
			},
			deleteDID: "did:plc:alice",
			wantErr:   false,
			verify: func(t *testing.T, repo *repositories.ActorsRepository, ctx context.Context) {
				_, err := repo.GetByDID(ctx, "did:plc:alice")
				if !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("expected sql.ErrNoRows for deleted actor, got %v", err)
				}
				bob, err := repo.GetByDID(ctx, "did:plc:bob")
				if err != nil {
					t.Fatalf("expected bob actor to remain, got error: %v", err)
				}
				if bob.DID != "did:plc:bob" {
					t.Fatalf("bob.DID = %q, want %q", bob.DID, "did:plc:bob")
				}
			},
		},
		{
			name:      "non-existing did is no-op",
			setup:     nil,
			deleteDID: "did:plc:does-not-exist",
			wantErr:   false,
			verify:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupActorsTest(t)
			ctx := context.Background()

			if tt.setup != nil {
				tt.setup(t, repo, ctx)
			}

			err := repo.DeleteByDID(ctx, tt.deleteDID)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DeleteByDID() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.verify != nil {
				tt.verify(t, repo, ctx)
			}
		})
	}
}

func TestActorsRepository_Exists(t *testing.T) {
	repo := setupActorsTest(t)
	ctx := context.Background()

	// Insert an actor
	err := repo.Upsert(ctx, "did:plc:testactor1", "alice.bsky.social")
	if err != nil {
		t.Fatalf("failed to insert actor: %v", err)
	}

	t.Run("existing actor", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "did:plc:testactor1")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true for existing actor")
		}
	})

	t.Run("non-existing actor", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "did:plc:nonexistent")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false for non-existing actor")
		}
	})
}
