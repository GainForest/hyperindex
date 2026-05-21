package migrations_test

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

// newTestExecutor creates an in-memory SQLite executor for testing.
func newTestExecutor(t *testing.T) *sqlite.Executor {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("failed to create SQLite executor: %v", err)
	}
	t.Cleanup(func() { exec.Close() })

	return exec
}

func TestMigrations_Run(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Verify key tables exist by querying sqlite_master.
	expectedTables := []string{
		"record",
		"actor",
		"config",
		"lexicon",
		"jetstream_activity",
		"label",
		"report",
		"label_definition",
		"actor_label_preference",
		"label_subscription_state",
		"external_label",
	}

	for _, table := range expectedTables {
		var name string
		err := exec.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected table %q to exist, but got error: %v", table, err)
		}
	}

	expectedIndexes := []string{
		"idx_external_label_active_lookup",
	}

	for _, index := range expectedIndexes {
		var name string
		err := exec.DB().QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", index,
		).Scan(&name)
		if err != nil {
			t.Errorf("expected index %q to exist, but got error: %v", index, err)
		}
	}
}

func TestMigrations_RunIdempotent(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("first Run() returned error: %v", err)
	}

	// Running a second time should be a no-op (all migrations already applied).
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("second Run() returned error: %v", err)
	}

	// Verify tables still present after second run.
	var count int
	err := exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='record'",
	).Scan(&count)
	if err != nil {
		t.Fatalf("failed to query sqlite_master: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 record table, got %d", count)
	}
}

func TestMigrations_Rollback(t *testing.T) {
	exec := newTestExecutor(t)
	ctx := context.Background()

	// Apply all migrations first.
	if err := migrations.Run(ctx, exec); err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}

	// Count applied migrations before rollback.
	var countBefore int
	err := exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&countBefore)
	if err != nil {
		t.Fatalf("failed to count migrations: %v", err)
	}

	if countBefore == 0 {
		t.Fatal("expected at least one applied migration before rollback")
	}

	// Rollback the last migration.
	if err := migrations.Rollback(ctx, exec); err != nil {
		t.Fatalf("Rollback() returned error: %v", err)
	}

	// Verify one fewer migration is recorded.
	var countAfter int
	err = exec.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM schema_migrations",
	).Scan(&countAfter)
	if err != nil {
		t.Fatalf("failed to count migrations after rollback: %v", err)
	}

	if countAfter != countBefore-1 {
		t.Errorf("expected %d migrations after rollback, got %d", countBefore-1, countAfter)
	}
}
