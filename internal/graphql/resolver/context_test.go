package resolver

import (
	"context"
	"testing"
)

func TestWithRepositories(t *testing.T) {
	ctx := context.Background()
	repos := &Repositories{}

	ctx = WithRepositories(ctx, repos)

	got := GetRepositories(ctx)
	if got != repos {
		t.Errorf("GetRepositories() = %v, want %v", got, repos)
	}
}

func TestGetRepositories_NilContext(t *testing.T) {
	ctx := context.Background()

	got := GetRepositories(ctx)
	if got != nil {
		t.Errorf("GetRepositories() = %v, want nil", got)
	}
}

func TestGetRepositories_WrongType(t *testing.T) {
	// Put wrong type in context
	ctx := context.WithValue(context.Background(), repoContextKey, "not a repositories")

	got := GetRepositories(ctx)
	if got != nil {
		t.Errorf("GetRepositories() = %v, want nil for wrong type", got)
	}
}

func TestGetRecordsRepo(t *testing.T) {
	t.Run("returns nil when no repositories in context", func(t *testing.T) {
		ctx := context.Background()
		got := GetRecordsRepo(ctx)
		if got != nil {
			t.Errorf("GetRecordsRepo() = %v, want nil", got)
		}
	})

	t.Run("returns nil when repos.Records is nil", func(t *testing.T) {
		ctx := context.Background()
		repos := &Repositories{Records: nil}
		ctx = WithRepositories(ctx, repos)

		got := GetRecordsRepo(ctx)
		if got != nil {
			t.Errorf("GetRecordsRepo() = %v, want nil", got)
		}
	})
}

func TestRepositoriesStruct(t *testing.T) {
	// Test that Repositories struct fields are accessible
	repos := &Repositories{}

	if repos.Records != nil {
		t.Errorf("Expected Records to be nil initially")
	}
	if repos.Actors != nil {
		t.Errorf("Expected Actors to be nil initially")
	}
	if repos.Lexicons != nil {
		t.Errorf("Expected Lexicons to be nil initially")
	}
	if repos.ExternalLabels != nil {
		t.Errorf("Expected ExternalLabels to be nil initially")
	}
}
