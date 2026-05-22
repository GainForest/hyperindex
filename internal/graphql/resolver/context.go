// Package resolver provides GraphQL resolver context and utilities.
package resolver

import (
	"context"

	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/repositories"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	repoContextKey contextKey = "repos"
)

// Repositories holds all database repositories needed by resolvers.
type Repositories struct {
	Records        *repositories.RecordsRepository
	Actors         *repositories.ActorsRepository
	Lexicons       *repositories.LexiconsRepository
	ExternalLabels *repositories.ExternalLabelsRepository
}

// NewRepositories creates a new Repositories from a database executor.
func NewRepositories(db database.Executor) *Repositories {
	return &Repositories{
		Records:        repositories.NewRecordsRepository(db),
		Actors:         repositories.NewActorsRepository(db),
		Lexicons:       repositories.NewLexiconsRepository(db),
		ExternalLabels: repositories.NewExternalLabelsRepository(db),
	}
}

// WithRepositories adds repositories to a context.
func WithRepositories(ctx context.Context, repos *Repositories) context.Context {
	return context.WithValue(ctx, repoContextKey, repos)
}

// GetRepositories retrieves repositories from context.
func GetRepositories(ctx context.Context) *Repositories {
	if repos, ok := ctx.Value(repoContextKey).(*Repositories); ok {
		return repos
	}
	return nil
}

// GetRecordsRepo is a convenience function to get the records repository.
func GetRecordsRepo(ctx context.Context) *repositories.RecordsRepository {
	if repos := GetRepositories(ctx); repos != nil {
		return repos.Records
	}
	return nil
}
