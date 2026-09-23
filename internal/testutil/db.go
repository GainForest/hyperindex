// Package testutil provides shared test helpers for the hyperindex test suite.
package testutil

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

// TestDB holds a test database with all migrations applied and
// pre-constructed repository instances.
type TestDB struct {
	Executor         database.Executor
	Records          *repositories.RecordsRepository
	Actors           *repositories.ActorsRepository
	Config           *repositories.ConfigRepository
	Lexicons         *repositories.LexiconsRepository
	Activity         *repositories.IndexingActivityRepository
	OAuthClients     *repositories.OAuthClientsRepository
	Labels           *repositories.LabelsRepository
	ExternalLabels   *repositories.ExternalLabelsRepository
	LabelDefinitions *repositories.LabelDefinitionsRepository
	LabelPreferences *repositories.LabelPreferencesRepository
	Reports          *repositories.ReportsRepository
	RecordVersions   *repositories.RecordVersionsRepository
}

// SetupTestDB creates an in-memory SQLite database with all migrations applied.
// The database is automatically closed when the test completes.
func SetupTestDB(t *testing.T) *TestDB {
	t.Helper()
	return SetupTestDBWithURL(t, "sqlite::memory:")
}

// SetupTestDBWithURL creates a test database for the supplied SQLite or
// PostgreSQL URL with all migrations applied. The database is automatically
// closed when the test completes.
func SetupTestDBWithURL(t *testing.T, databaseURL string) *TestDB {
	t.Helper()

	var exec database.Executor
	var err error
	switch database.ParseDialect(databaseURL) {
	case database.PostgreSQL:
		exec, err = postgres.NewExecutor(databaseURL)
	default:
		exec, err = sqlite.NewExecutor(databaseURL)
	}
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
		Activity:         repositories.NewIndexingActivityRepository(exec),
		OAuthClients:     repositories.NewOAuthClientsRepository(exec),
		Labels:           repositories.NewLabelsRepository(exec),
		ExternalLabels:   repositories.NewExternalLabelsRepository(exec),
		LabelDefinitions: repositories.NewLabelDefinitionsRepository(exec),
		LabelPreferences: repositories.NewLabelPreferencesRepository(exec),
		Reports:          repositories.NewReportsRepository(exec),
		RecordVersions:   repositories.NewRecordVersionsRepository(exec),
	}

	t.Cleanup(func() {
		exec.Close()
	})

	return db
}
