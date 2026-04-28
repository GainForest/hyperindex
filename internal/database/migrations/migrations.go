// Package migrations handles database schema migrations.
package migrations

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"

	"github.com/GainForest/hyperindex/internal/database"
)

//go:embed sqlite/*.sql
var sqliteMigrations embed.FS

//go:embed postgres/*.sql
var postgresMigrations embed.FS

// Migration represents a single migration.
type Migration struct {
	Version string
	Name    string
	UpSQL   string
	DownSQL string
}

// Run applies all pending migrations.
func Run(ctx context.Context, exec database.Executor) error {
	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(ctx, exec); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(ctx, exec)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Load migrations for the current dialect
	migrations, err := loadMigrations(exec.Dialect())
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Apply pending migrations
	for _, m := range migrations {
		if applied[m.Version] {
			slog.Debug("Migration already applied", "version", m.Version, "name", m.Name)
			continue
		}

		slog.Info("Applying migration", "version", m.Version, "name", m.Name)

		// Execute migration SQL
		if _, err := exec.DB().ExecContext(ctx, m.UpSQL); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.Version, err)
		}

		// Record migration
		if err := recordMigration(ctx, exec, m.Version); err != nil {
			return fmt.Errorf("failed to record migration %s: %w", m.Version, err)
		}

		slog.Info("Migration applied successfully", "version", m.Version)
	}

	return nil
}

// Rollback reverses the last applied migration.
func Rollback(ctx context.Context, exec database.Executor) error {
	// Get the last applied migration
	var version string
	err := exec.QueryRow(ctx,
		"SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1",
		nil, &version)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("No migrations to rollback")
			return nil
		}
		return fmt.Errorf("failed to get last migration: %w", err)
	}

	// Load migrations
	migrations, err := loadMigrations(exec.Dialect())
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Find the migration to rollback
	var migration *Migration
	for i := range migrations {
		if migrations[i].Version == version {
			migration = &migrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %s not found", version)
	}

	slog.Info("Rolling back migration", "version", version, "name", migration.Name)

	// Execute rollback SQL
	if _, err := exec.DB().ExecContext(ctx, migration.DownSQL); err != nil {
		return fmt.Errorf("failed to rollback migration %s: %w", version, err)
	}

	// Remove migration record
	_, err = exec.Exec(ctx,
		fmt.Sprintf("DELETE FROM schema_migrations WHERE version = %s", exec.Placeholder(1)),
		[]database.Value{database.Text(version)})
	if err != nil {
		return fmt.Errorf("failed to remove migration record: %w", err)
	}

	slog.Info("Migration rolled back successfully", "version", version)
	return nil
}

func createMigrationsTable(ctx context.Context, exec database.Executor) error {
	var sqlStr string
	switch exec.Dialect() {
	case database.PostgreSQL:
		sqlStr = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`
	default:
		sqlStr = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`
	}

	_, err := exec.DB().ExecContext(ctx, sqlStr)
	return err
}

func getAppliedMigrations(ctx context.Context, exec database.Executor) (map[string]bool, error) {
	applied := make(map[string]bool)

	rows, err := exec.DB().QueryContext(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		applied[version] = true
	}

	return applied, rows.Err()
}

func recordMigration(ctx context.Context, exec database.Executor, version string) error {
	_, err := exec.Exec(ctx,
		fmt.Sprintf("INSERT INTO schema_migrations (version) VALUES (%s)", exec.Placeholder(1)),
		[]database.Value{database.Text(version)})
	return err
}

func loadMigrations(dialect database.Dialect) ([]Migration, error) {
	var fs embed.FS
	var dir string

	switch dialect {
	case database.PostgreSQL:
		fs = postgresMigrations
		dir = "postgres"
	default:
		fs = sqliteMigrations
		dir = "sqlite"
	}

	entries, err := fs.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Group up/down files by version
	migrationFiles := make(map[string]map[string]string)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}

		// Parse filename: 001_name.up.sql or 001_name.down.sql
		parts := strings.Split(name, ".")
		if len(parts) < 3 {
			continue
		}

		direction := parts[len(parts)-2] // "up" or "down"
		if direction != "up" && direction != "down" {
			continue
		}

		baseName := strings.Join(parts[:len(parts)-2], ".")
		version := strings.Split(baseName, "_")[0]

		if migrationFiles[version] == nil {
			migrationFiles[version] = make(map[string]string)
			migrationFiles[version]["name"] = baseName
		}

		content, err := fs.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration %s: %w", name, err)
		}

		migrationFiles[version][direction] = string(content)
	}

	// Convert to slice and sort
	var migrations []Migration
	for version, files := range migrationFiles {
		migrations = append(migrations, Migration{
			Version: version,
			Name:    files["name"],
			UpSQL:   files["up"],
			DownSQL: files["down"],
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
