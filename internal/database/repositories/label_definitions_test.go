package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupLabelDefsTest(t *testing.T) *repositories.LabelDefinitionsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.LabelDefinitions
}

func TestLabelDefinitions_Insert(t *testing.T) {
	repo := setupLabelDefsTest(t)
	ctx := context.Background()

	err := repo.Insert(ctx, "test-custom-label", "Custom test label", repositories.SeverityInform, repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Verify it was inserted
	def, err := repo.Get(ctx, "test-custom-label")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if def.Val != "test-custom-label" {
		t.Errorf("Val = %q, want %q", def.Val, "test-custom-label")
	}
	if def.Description != "Custom test label" {
		t.Errorf("Description = %q, want %q", def.Description, "Custom test label")
	}
	if def.Severity != repositories.SeverityInform {
		t.Errorf("Severity = %q, want %q", def.Severity, repositories.SeverityInform)
	}
	if def.DefaultVisibility != repositories.VisibilityWarn {
		t.Errorf("DefaultVisibility = %q, want %q", def.DefaultVisibility, repositories.VisibilityWarn)
	}
}

func TestLabelDefinitions_Get(t *testing.T) {
	repo := setupLabelDefsTest(t)
	ctx := context.Background()

	err := repo.Insert(ctx, "test-get-label", "Label for get test", repositories.SeverityInform, repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	t.Run("found", func(t *testing.T) {
		def, err := repo.Get(ctx, "test-get-label")
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if def.Val != "test-get-label" {
			t.Errorf("Val = %q, want %q", def.Val, "test-get-label")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.Get(ctx, "nonexistent")
		if err == nil {
			t.Fatal("Get() expected error for non-existing val, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("Get() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestLabelDefinitions_GetAll(t *testing.T) {
	repo := setupLabelDefsTest(t)
	ctx := context.Background()

	// Insert a custom definition in addition to seeded ones
	err := repo.Insert(ctx, "zzz-test-custom", "Custom for GetAll", repositories.SeverityInform, repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	defs, err := repo.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll() error = %v", err)
	}

	// Migrations seed 11 definitions; we added 1 more
	if len(defs) < 12 {
		t.Errorf("GetAll() returned %d definitions, want at least 12", len(defs))
	}

	// Verify order by val: first should start with "!" (system labels come first alphabetically)
	if len(defs) > 0 && defs[0].Val[0] != '!' {
		t.Errorf("defs[0].Val = %q, expected system label starting with '!'", defs[0].Val)
	}

	// Our custom label should be last (zzz-test-custom sorts last)
	if defs[len(defs)-1].Val != "zzz-test-custom" {
		t.Errorf("last def Val = %q, want %q", defs[len(defs)-1].Val, "zzz-test-custom")
	}
}

func TestLabelDefinitions_GetNonSystem(t *testing.T) {
	repo := setupLabelDefsTest(t)
	ctx := context.Background()

	// Insert a custom non-system label and a system label
	err := repo.Insert(ctx, "test-nonsys", "Non-system label", repositories.SeverityInform, repositories.VisibilityWarn)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	err = repo.Insert(ctx, "!test-sys", "System test label", repositories.SeverityTakedown, repositories.VisibilityHide)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	defs, err := repo.GetNonSystem(ctx)
	if err != nil {
		t.Fatalf("GetNonSystem() error = %v", err)
	}

	// Verify no system labels are included
	for _, def := range defs {
		if len(def.Val) > 0 && def.Val[0] == '!' {
			t.Errorf("GetNonSystem() included system label %q", def.Val)
		}
	}

	// Verify our custom non-system label is present
	found := false
	for _, def := range defs {
		if def.Val == "test-nonsys" {
			found = true
			break
		}
	}
	if !found {
		t.Error("GetNonSystem() did not include test-nonsys label")
	}
}

func TestLabelDefinitions_Exists(t *testing.T) {
	repo := setupLabelDefsTest(t)
	ctx := context.Background()

	// Use a seeded label for the "existing" check
	t.Run("existing", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "spam")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if !exists {
			t.Error("Exists() = false, want true for existing label")
		}
	})

	t.Run("non-existing", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("Exists() error = %v", err)
		}
		if exists {
			t.Error("Exists() = true, want false for non-existing label")
		}
	})
}

func TestValidateVisibility(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    repositories.LabelVisibility
		wantErr bool
	}{
		{name: "ignore", input: "ignore", want: repositories.VisibilityIgnore},
		{name: "show", input: "show", want: repositories.VisibilityShow},
		{name: "warn", input: "warn", want: repositories.VisibilityWarn},
		{name: "hide", input: "hide", want: repositories.VisibilityHide},
		{name: "invalid", input: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ValidateVisibility(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVisibility(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ValidateVisibility(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateSeverity(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    repositories.LabelSeverity
		wantErr bool
	}{
		{name: "inform", input: "inform", want: repositories.SeverityInform},
		{name: "alert", input: "alert", want: repositories.SeverityAlert},
		{name: "takedown", input: "takedown", want: repositories.SeverityTakedown},
		{name: "invalid", input: "invalid", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ValidateSeverity(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateSeverity(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ValidateSeverity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
