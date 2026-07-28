package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupLexiconsTest(t *testing.T) *repositories.LexiconsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Lexicons
}

func TestLexiconsRepository_Upsert(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	// Insert new lexicon
	jsonData := `{"lexicon":1,"id":"app.bsky.feed.post"}`
	err := repo.Upsert(ctx, "app.bsky.feed.post", jsonData)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}

	lex, err := repo.GetByID(ctx, "app.bsky.feed.post")
	if err != nil {
		t.Fatalf("failed to get lexicon after insert: %v", err)
	}
	if lex.ID != "app.bsky.feed.post" {
		t.Errorf("ID = %q, want %q", lex.ID, "app.bsky.feed.post")
	}
	if lex.JSON != jsonData {
		t.Errorf("JSON = %q, want %q", lex.JSON, jsonData)
	}

	// Update existing lexicon with new JSON
	updatedJSON := `{"lexicon":1,"id":"app.bsky.feed.post","revision":2}`
	err = repo.Upsert(ctx, "app.bsky.feed.post", updatedJSON)
	if err != nil {
		t.Fatalf("failed to upsert lexicon: %v", err)
	}

	lex, err = repo.GetByID(ctx, "app.bsky.feed.post")
	if err != nil {
		t.Fatalf("failed to get lexicon after upsert: %v", err)
	}
	if lex.JSON != updatedJSON {
		t.Errorf("JSON after upsert = %q, want %q", lex.JSON, updatedJSON)
	}
}

func TestLexiconsRepository_UpsertPreservesExactJSON(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()
	const raw = "{\n  \"lexicon\": 1,\n  \"id\": \"app.example.formatted\",\n  \"defs\": {}\n}\n"

	if err := repo.Upsert(ctx, "app.example.formatted", raw); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	lex, err := repo.GetByID(ctx, "app.example.formatted")
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if lex.JSON != raw {
		t.Fatalf("GetByID().JSON = %q, want exact saved bytes %q", lex.JSON, raw)
	}
}

func TestLexiconsRepository_UpsertMany(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	writes := []repositories.LexiconWrite{
		{ID: "app.example.first", JSON: `{"lexicon":1,"id":"app.example.first"}`},
		{ID: "app.example.second", JSON: `{"lexicon":1,"id":"app.example.second"}`},
	}

	if err := db.Lexicons.UpsertMany(ctx, writes); err != nil {
		t.Fatalf("UpsertMany() error = %v", err)
	}
	for _, write := range writes {
		lex, err := db.Lexicons.GetByID(ctx, write.ID)
		if err != nil {
			t.Fatalf("GetByID(%s) error = %v", write.ID, err)
		}
		if lex.JSON != write.JSON {
			t.Fatalf("GetByID(%s).JSON = %q, want %q", write.ID, lex.JSON, write.JSON)
		}
	}
}

func TestLexiconsRepository_UpsertManyRollsBackOnFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	if db.Executor.Dialect() != database.SQLite {
		t.Skip("SQLite trigger provides deterministic mid-batch failure coverage")
	}
	ctx := context.Background()
	if _, err := db.Executor.DB().ExecContext(ctx, `
		CREATE TRIGGER fail_lexicon_batch
		BEFORE INSERT ON lexicon
		WHEN NEW.id = 'app.example.fail'
		BEGIN
			SELECT RAISE(ABORT, 'forced batch failure');
		END;
	`); err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	err := db.Lexicons.UpsertMany(ctx, []repositories.LexiconWrite{
		{ID: "app.example.first", JSON: `{"lexicon":1,"id":"app.example.first"}`},
		{ID: "app.example.fail", JSON: `{"lexicon":1,"id":"app.example.fail"}`},
	})
	if err == nil {
		t.Fatal("UpsertMany() error = nil, want forced failure")
	}
	if exists, existsErr := db.Lexicons.Exists(ctx, "app.example.first"); existsErr != nil || exists {
		t.Fatalf("first Lexicon exists = %v, error = %v; want false after rollback", exists, existsErr)
	}
}

func TestLexiconsRepository_GetByID(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	// Setup: insert a lexicon
	err := repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		lex, err := repo.GetByID(ctx, "app.bsky.feed.post")
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if lex.ID != "app.bsky.feed.post" {
			t.Errorf("ID = %q, want %q", lex.ID, "app.bsky.feed.post")
		}
		if lex.JSON != `{"lexicon":1,"id":"app.bsky.feed.post"}` {
			t.Errorf("JSON = %q, want %q", lex.JSON, `{"lexicon":1,"id":"app.bsky.feed.post"}`)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByID(ctx, "app.bsky.nonexistent")
		if err == nil {
			t.Fatal("GetByID() expected error for non-existing ID, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("GetByID() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestLexiconsRepository_GetAll(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		lexicons, err := repo.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if lexicons != nil {
			t.Errorf("GetAll() on empty db = %v, want nil", lexicons)
		}
	})

	t.Run("after inserts", func(t *testing.T) {
		// Insert in non-alphabetical order to verify ORDER BY id
		err := repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
		if err != nil {
			t.Fatalf("failed to insert lexicon: %v", err)
		}
		err = repo.Upsert(ctx, "app.bsky.actor.profile", `{"lexicon":1,"id":"app.bsky.actor.profile"}`)
		if err != nil {
			t.Fatalf("failed to insert lexicon: %v", err)
		}

		lexicons, err := repo.GetAll(ctx)
		if err != nil {
			t.Fatalf("GetAll() error = %v", err)
		}
		if len(lexicons) != 2 {
			t.Fatalf("GetAll() returned %d lexicons, want 2", len(lexicons))
		}

		// Verify order by ID (alphabetical)
		if lexicons[0].ID != "app.bsky.actor.profile" {
			t.Errorf("lexicons[0].ID = %q, want %q", lexicons[0].ID, "app.bsky.actor.profile")
		}
		if lexicons[1].ID != "app.bsky.feed.post" {
			t.Errorf("lexicons[1].ID = %q, want %q", lexicons[1].ID, "app.bsky.feed.post")
		}
	})
}

func TestLexiconsRepository_Delete(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	t.Run("existing ID", func(t *testing.T) {
		err := repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
		if err != nil {
			t.Fatalf("failed to insert lexicon: %v", err)
		}

		err = repo.Delete(ctx, "app.bsky.feed.post")
		if err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		exists, err := repo.Exists(ctx, "app.bsky.feed.post")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Error("lexicon still exists after Delete()")
		}
	})

	t.Run("non-existing ID", func(t *testing.T) {
		err := repo.Delete(ctx, "app.bsky.nonexistent")
		if err != nil {
			t.Errorf("Delete() on non-existing ID error = %v, want nil", err)
		}
	})
}

func TestLexiconsRepository_DeleteAll(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	// Insert some lexicons
	err := repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}
	err = repo.Upsert(ctx, "app.bsky.actor.profile", `{"lexicon":1,"id":"app.bsky.actor.profile"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
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

func TestLexiconsRepository_GetCount(t *testing.T) {
	repo := setupLexiconsTest(t)
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
	err = repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}
	err = repo.Upsert(ctx, "app.bsky.actor.profile", `{"lexicon":1,"id":"app.bsky.actor.profile"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}

	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 2 {
		t.Errorf("GetCount() after 2 inserts = %d, want 2", count)
	}
}

func TestLexiconsRepository_Exists(t *testing.T) {
	repo := setupLexiconsTest(t)
	ctx := context.Background()

	// Insert a lexicon
	err := repo.Upsert(ctx, "app.bsky.feed.post", `{"lexicon":1,"id":"app.bsky.feed.post"}`)
	if err != nil {
		t.Fatalf("failed to insert lexicon: %v", err)
	}

	t.Run("existing lexicon", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "app.bsky.feed.post")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true for existing lexicon")
		}
	})

	t.Run("non-existing lexicon", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "app.bsky.nonexistent")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false for non-existing lexicon")
		}
	})
}
