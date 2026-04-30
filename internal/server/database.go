// Package server contains the main server initialization and orchestration.
package server

import (
	"fmt"
	"log/slog"

	"github.com/GainForest/hyperindex/internal/config"
	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/postgres"
	"github.com/GainForest/hyperindex/internal/database/sqlite"
)

// ConnectDatabase creates a database executor based on the database URL.
// Supported formats:
//   - sqlite:path/to/file.db
//   - sqlite::memory:
//   - postgres://user:pass@host:port/dbname
//   - postgresql://user:pass@host:port/dbname
func ConnectDatabase(databaseURL string) (database.Executor, error) {
	dialect := database.ParseDialect(databaseURL)

	slog.Info("Connecting to database",
		"dialect", dialect.String(),
		"url", config.RedactPassword(databaseURL),
	)

	switch dialect {
	case database.PostgreSQL:
		return postgres.NewExecutor(databaseURL)
	case database.SQLite:
		return sqlite.NewExecutor(databaseURL)
	default:
		return nil, fmt.Errorf("unsupported database URL: %s", databaseURL)
	}
}
