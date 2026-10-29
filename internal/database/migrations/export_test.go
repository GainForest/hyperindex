package migrations

import (
	"context"
	"fmt"

	"github.com/GainForest/hyperindex/internal/database"
)

// RunThrough applies pending migrations up to and including version, so tests
// can stage a database at an older schema and exercise one migration in
// isolation.
func RunThrough(ctx context.Context, exec database.Executor, version string) error {
	if err := createMigrationsTable(ctx, exec); err != nil {
		return err
	}
	applied, err := getAppliedMigrations(ctx, exec)
	if err != nil {
		return err
	}
	all, err := loadMigrations(exec.Dialect())
	if err != nil {
		return err
	}
	for _, m := range all {
		if m.Version > version {
			break
		}
		if applied[m.Version] {
			continue
		}
		if err := applyMigration(ctx, exec, m); err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.Version, err)
		}
	}
	return nil
}
