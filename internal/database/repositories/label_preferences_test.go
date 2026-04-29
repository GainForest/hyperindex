package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupLabelPreferencesTest(t *testing.T) *repositories.LabelPreferencesRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.LabelPreferences
}

func TestLabelPreferencesRepository_SetAndGet(t *testing.T) {
	repo := setupLabelPreferencesTest(t)
	ctx := context.Background()

	inserted, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	if inserted.DID != "did:plc:user" {
		t.Errorf("DID = %q, want %q", inserted.DID, "did:plc:user")
	}
	if inserted.LabelVal != "spam" {
		t.Errorf("LabelVal = %q, want %q", inserted.LabelVal, "spam")
	}
	if inserted.Visibility != repositories.VisibilityWarn {
		t.Errorf("Visibility = %q, want %q", inserted.Visibility, repositories.VisibilityWarn)
	}
	if inserted.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want non-zero timestamp")
	}

	got, err := repo.Get(ctx, "did:plc:user", "spam")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.DID != inserted.DID {
		t.Errorf("Get().DID = %q, want %q", got.DID, inserted.DID)
	}
	if got.LabelVal != inserted.LabelVal {
		t.Errorf("Get().LabelVal = %q, want %q", got.LabelVal, inserted.LabelVal)
	}
	if got.Visibility != inserted.Visibility {
		t.Errorf("Get().Visibility = %q, want %q", got.Visibility, inserted.Visibility)
	}
	if got.CreatedAt.IsZero() {
		t.Error("Get().CreatedAt is zero, want non-zero timestamp")
	}
}

func TestLabelPreferencesRepository_GetByDID(t *testing.T) {
	repo := setupLabelPreferencesTest(t)
	ctx := context.Background()

	_, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Set(spam) error = %v", err)
	}
	_, err = repo.Set(ctx, "did:plc:user", "nudity", repositories.VisibilityHide)
	if err != nil {
		t.Fatalf("Set(nudity) error = %v", err)
	}
	_, err = repo.Set(ctx, "did:plc:other", "adult", repositories.VisibilityIgnore)
	if err != nil {
		t.Fatalf("Set(other) error = %v", err)
	}

	preferences, err := repo.GetByDID(ctx, "did:plc:user")
	if err != nil {
		t.Fatalf("GetByDID() error = %v", err)
	}
	if len(preferences) != 2 {
		t.Fatalf("GetByDID() returned %d preferences, want 2", len(preferences))
	}
	if preferences[0].DID != "did:plc:user" || preferences[1].DID != "did:plc:user" {
		t.Errorf("GetByDID() returned DIDs %q and %q, want only did:plc:user", preferences[0].DID, preferences[1].DID)
	}
	if preferences[0].LabelVal != "nudity" || preferences[1].LabelVal != "spam" {
		t.Errorf("GetByDID() label order = [%q, %q], want [nudity, spam]", preferences[0].LabelVal, preferences[1].LabelVal)
	}
}

func TestLabelPreferencesRepository_SetUpdatesExistingPreference(t *testing.T) {
	repo := setupLabelPreferencesTest(t)
	ctx := context.Background()

	_, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Set() insert error = %v", err)
	}
	updated, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityHide)
	if err != nil {
		t.Fatalf("Set() update error = %v", err)
	}
	if updated.Visibility != repositories.VisibilityHide {
		t.Errorf("Visibility after update = %q, want %q", updated.Visibility, repositories.VisibilityHide)
	}

	preferences, err := repo.GetByDID(ctx, "did:plc:user")
	if err != nil {
		t.Fatalf("GetByDID() error = %v", err)
	}
	if len(preferences) != 1 {
		t.Fatalf("GetByDID() returned %d preferences, want 1 after upsert", len(preferences))
	}
	if preferences[0].Visibility != repositories.VisibilityHide {
		t.Errorf("stored Visibility = %q, want %q", preferences[0].Visibility, repositories.VisibilityHide)
	}
}

func TestLabelPreferencesRepository_Delete(t *testing.T) {
	repo := setupLabelPreferencesTest(t)
	ctx := context.Background()

	_, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := repo.Delete(ctx, "did:plc:user", "spam"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	_, err = repo.Get(ctx, "did:plc:user", "spam")
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("Get() after Delete() error = %v, want sql.ErrNoRows", err)
	}
}

func TestLabelPreferencesRepository_DeleteByDID(t *testing.T) {
	repo := setupLabelPreferencesTest(t)
	ctx := context.Background()

	_, err := repo.Set(ctx, "did:plc:user", "spam", repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Set(user spam) error = %v", err)
	}
	_, err = repo.Set(ctx, "did:plc:user", "nudity", repositories.VisibilityHide)
	if err != nil {
		t.Fatalf("Set(user nudity) error = %v", err)
	}
	_, err = repo.Set(ctx, "did:plc:other", "spam", repositories.VisibilityShow)
	if err != nil {
		t.Fatalf("Set(other spam) error = %v", err)
	}

	if err := repo.DeleteByDID(ctx, "did:plc:user"); err != nil {
		t.Fatalf("DeleteByDID() error = %v", err)
	}

	deletedPreferences, err := repo.GetByDID(ctx, "did:plc:user")
	if err != nil {
		t.Fatalf("GetByDID(deleted DID) error = %v", err)
	}
	if len(deletedPreferences) != 0 {
		t.Errorf("GetByDID(deleted DID) returned %d preferences, want 0", len(deletedPreferences))
	}

	remainingPreferences, err := repo.GetByDID(ctx, "did:plc:other")
	if err != nil {
		t.Fatalf("GetByDID(other DID) error = %v", err)
	}
	if len(remainingPreferences) != 1 {
		t.Fatalf("GetByDID(other DID) returned %d preferences, want 1", len(remainingPreferences))
	}
	if remainingPreferences[0].LabelVal != "spam" {
		t.Errorf("remaining LabelVal = %q, want %q", remainingPreferences[0].LabelVal, "spam")
	}
}
