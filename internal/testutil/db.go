// Package testutil provides shared test helpers for the hyperindex test suite.
package testutil

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

// TestDB holds an in-memory SQLite database with all migrations applied
// and pre-constructed repository instances.
type TestDB struct {
	Executor         database.Executor
	Records          *repositories.RecordsRepository
	Actors           *repositories.ActorsRepository
	Config           *repositories.ConfigRepository
	Lexicons         *repositories.LexiconsRepository
	Activity         *repositories.JetstreamActivityRepository
	OAuthClients     *repositories.OAuthClientsRepository
	Labels           *repositories.LabelsRepository
	LabelDefinitions *repositories.LabelDefinitionsRepository
	LabelPreferences *repositories.LabelPreferencesRepository
	Reports          *repositories.ReportsRepository
}

// SetupTestDB creates an in-memory SQLite database with all migrations applied.
// The database is automatically closed when the test completes.
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()

	exec, err := sqlite.NewExecutor("sqlite::memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	ctx := context.Background()
	if err := migrations.Run(ctx, exec); err != nil {
		exec.Close()
		t.Fatalf("Failed to run migrations: %v", err)
	}

	db := &TestDB{
		Executor:         exec,
		Records:          repositories.NewRecordsRepository(exec),
		Actors:           repositories.NewActorsRepository(exec),
		Config:           repositories.NewConfigRepository(exec),
		Lexicons:         repositories.NewLexiconsRepository(exec),
		Activity:         repositories.NewJetstreamActivityRepository(exec),
		OAuthClients:     repositories.NewOAuthClientsRepository(exec),
		Labels:           repositories.NewLabelsRepository(exec),
		LabelDefinitions: repositories.NewLabelDefinitionsRepository(exec),
		LabelPreferences: repositories.NewLabelPreferencesRepository(exec),
		Reports:          repositories.NewReportsRepository(exec),
	}

	t.Cleanup(func() {
		exec.Close()
	})

	return db
}
