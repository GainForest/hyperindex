package migrations

import (
	"context"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

func newOutsideTransactionExecutor(t *testing.T) *sqlite.Executor {
	t.Helper()
	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("create sqlite executor: %v", err)
	}
	t.Cleanup(func() { _ = exec.Close() })
	if err := createMigrationsTable(context.Background(), exec); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	return exec
}

func appliedVersions(t *testing.T, exec *sqlite.Executor) map[string]bool {
	t.Helper()
	applied, err := getAppliedMigrations(context.Background(), exec)
	if err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	return applied
}

func tableExists(t *testing.T, exec *sqlite.Executor, name string) bool {
	t.Helper()
	var count int
	if err := exec.DB().QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&count); err != nil {
		t.Fatalf("check table %s: %v", name, err)
	}
	return count > 0
}

// A marked migration keeps the work done before a failing statement and does
// not record its version, so the next run repeats it. Idempotent statements
// make that repeat harmless; this pins the documented trade-off against the
// atomic behaviour of unmarked migrations.
func TestApplyMigration_OutsideTransactionKeepsEarlierStatementsOnFailure(t *testing.T) {
	exec := newOutsideTransactionExecutor(t)
	ctx := context.Background()
	up := noTransactionMarker + "\n" +
		"CREATE TABLE IF NOT EXISTS first_table (id INTEGER);\n" +
		"CREATE TABLE second_table (id INTEGER, id INTEGER);\n" +
		"CREATE TABLE IF NOT EXISTS third_table (id INTEGER);\n"
	broken := Migration{Version: "901", Name: "901_outside", UpSQL: up}

	err := applyMigration(ctx, exec, broken)
	if err == nil || !strings.Contains(err.Error(), "CREATE TABLE second_table") {
		t.Fatalf("applyMigration() error = %v, want the failing statement named", err)
	}
	if !tableExists(t, exec, "first_table") {
		t.Fatal("first statement was rolled back; a marked migration must not run in a transaction")
	}
	if tableExists(t, exec, "third_table") {
		t.Fatal("statements after the failure ran")
	}
	if appliedVersions(t, exec)["901"] {
		t.Fatal("version recorded despite a failed statement")
	}

	fixed := broken
	fixed.UpSQL = noTransactionMarker + "\n" +
		"CREATE TABLE IF NOT EXISTS first_table (id INTEGER);\n" +
		"CREATE TABLE IF NOT EXISTS third_table (id INTEGER);\n"
	if err := applyMigration(ctx, exec, fixed); err != nil {
		t.Fatalf("retry after fixing the migration: %v", err)
	}
	if !tableExists(t, exec, "third_table") || !appliedVersions(t, exec)["901"] {
		t.Fatal("retry did not finish the migration and record its version")
	}
}

func TestRollbackMigration_OutsideTransactionRemovesVersion(t *testing.T) {
	exec := newOutsideTransactionExecutor(t)
	ctx := context.Background()
	m := Migration{
		Version: "902",
		Name:    "902_outside",
		UpSQL:   noTransactionMarker + "\nCREATE TABLE IF NOT EXISTS outside_table (id INTEGER);",
		DownSQL: noTransactionMarker + "\nDROP TABLE IF EXISTS outside_table;",
	}
	if err := applyMigration(ctx, exec, m); err != nil {
		t.Fatalf("applyMigration() error = %v", err)
	}
	if err := rollbackMigration(ctx, exec, m); err != nil {
		t.Fatalf("rollbackMigration() error = %v", err)
	}
	if tableExists(t, exec, "outside_table") || appliedVersions(t, exec)["902"] {
		t.Fatal("rollback left the table or the version behind")
	}
}
