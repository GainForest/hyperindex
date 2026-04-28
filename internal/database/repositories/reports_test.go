package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupReportsTest(t *testing.T) *repositories.ReportsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Reports
}

func TestReportsRepository_Insert(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()

	t.Run("nil reason", func(t *testing.T) {
		report, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc", repositories.ReasonSpam, nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if report.ID <= 0 {
			t.Errorf("Insert() returned id = %d, want > 0", report.ID)
		}
		if report.ReporterDID != "did:plc:reporter1" {
			t.Errorf("ReporterDID = %q, want %q", report.ReporterDID, "did:plc:reporter1")
		}
		if report.SubjectURI != "at://did:plc:user/app.bsky.feed.post/abc" {
			t.Errorf("SubjectURI = %q, want %q", report.SubjectURI, "at://did:plc:user/app.bsky.feed.post/abc")
		}
		if report.ReasonType != repositories.ReasonSpam {
			t.Errorf("ReasonType = %q, want %q", report.ReasonType, repositories.ReasonSpam)
		}
		if report.Reason != nil {
			t.Errorf("Reason = %v, want nil", report.Reason)
		}
		if report.Status != repositories.StatusPending {
			t.Errorf("Status = %q, want %q", report.Status, repositories.StatusPending)
		}
	})

	t.Run("non-nil reason", func(t *testing.T) {
		reason := "This post is clearly spam"
		report, err := repo.Insert(ctx, "did:plc:reporter2", "at://did:plc:user/app.bsky.feed.post/def", repositories.ReasonViolation, &reason)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		if report.Reason == nil {
			t.Fatal("Reason is nil, want non-nil")
		}
		if *report.Reason != reason {
			t.Errorf("Reason = %q, want %q", *report.Reason, reason)
		}
	})
}

func TestReportsRepository_Get(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()
	inserted, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc", repositories.ReasonSpam, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	t.Run("found", func(t *testing.T) {
		report, err := repo.Get(ctx, inserted.ID)
		if err != nil {
			t.Fatalf("Get() error = %v", err)
		}
		if report.ID != inserted.ID {
			t.Errorf("ID = %d, want %d", report.ID, inserted.ID)
		}
		if report.ReporterDID != "did:plc:reporter1" {
			t.Errorf("ReporterDID = %q, want %q", report.ReporterDID, "did:plc:reporter1")
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, err := repo.Get(ctx, 99999)
		if err == nil {
			t.Fatal("Get() expected error for non-existing ID, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("Get() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestReportsRepository_GetPaginated(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()
	_, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/a", repositories.ReasonSpam, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:reporter2", "at://did:plc:user/app.bsky.feed.post/b", repositories.ReasonViolation, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:reporter3", "at://did:plc:user/app.bsky.feed.post/c", repositories.ReasonRude, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	t.Run("no filter returns all", func(t *testing.T) {
		result, err := repo.GetPaginated(ctx, nil, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Reports) != 3 {
			t.Errorf("GetPaginated() returned %d reports, want 3", len(result.Reports))
		}
		if result.TotalCount != 3 {
			t.Errorf("TotalCount = %d, want 3", result.TotalCount)
		}
	})
	t.Run("status filter", func(t *testing.T) {
		status := repositories.StatusPending
		result, err := repo.GetPaginated(ctx, &status, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Reports) != 3 {
			t.Errorf("GetPaginated(pending) returned %d reports, want 3", len(result.Reports))
		}
		resolved := repositories.StatusResolved
		result, err = repo.GetPaginated(ctx, &resolved, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Reports) != 0 {
			t.Errorf("GetPaginated(resolved) returned %d reports, want 0", len(result.Reports))
		}
	})
	t.Run("cursor and hasNextPage", func(t *testing.T) {
		result, err := repo.GetPaginated(ctx, nil, 2, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Reports) != 2 {
			t.Fatalf("first page returned %d reports, want 2", len(result.Reports))
		}
		if !result.HasNextPage {
			t.Error("HasNextPage = false, want true")
		}
		cursor := result.Reports[len(result.Reports)-1].ID
		result2, err := repo.GetPaginated(ctx, nil, 2, &cursor)
		if err != nil {
			t.Fatalf("GetPaginated() page 2 error = %v", err)
		}
		if len(result2.Reports) != 1 {
			t.Errorf("second page returned %d reports, want 1", len(result2.Reports))
		}
		if result2.HasNextPage {
			t.Error("HasNextPage on last page = true, want false")
		}
	})
}

func TestReportsRepository_Resolve(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()
	inserted, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc", repositories.ReasonSpam, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	resolved, err := repo.Resolve(ctx, inserted.ID, repositories.StatusResolved, "did:plc:moderator")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Status != repositories.StatusResolved {
		t.Errorf("Status = %q, want %q", resolved.Status, repositories.StatusResolved)
	}
	if resolved.ResolvedBy == nil {
		t.Fatal("ResolvedBy is nil, want non-nil")
	}
	if *resolved.ResolvedBy != "did:plc:moderator" {
		t.Errorf("ResolvedBy = %q, want %q", *resolved.ResolvedBy, "did:plc:moderator")
	}
	if resolved.ResolvedAt == nil {
		t.Error("ResolvedAt is nil, want non-nil")
	}
}

func TestReportsRepository_GetByReporterAndSubject(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()
	_, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc", repositories.ReasonSpam, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	t.Run("found", func(t *testing.T) {
		report, err := repo.GetByReporterAndSubject(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc")
		if err != nil {
			t.Fatalf("GetByReporterAndSubject() error = %v", err)
		}
		if report.ReporterDID != "did:plc:reporter1" {
			t.Errorf("ReporterDID = %q, want %q", report.ReporterDID, "did:plc:reporter1")
		}
	})
	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByReporterAndSubject(ctx, "did:plc:nonexistent", "at://did:plc:user/app.bsky.feed.post/abc")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestReportsRepository_DeleteAll(t *testing.T) {
	repo := setupReportsTest(t)
	ctx := context.Background()
	_, err := repo.Insert(ctx, "did:plc:reporter1", "at://did:plc:user/app.bsky.feed.post/abc", repositories.ReasonSpam, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:reporter2", "at://did:plc:user/app.bsky.feed.post/def", repositories.ReasonRude, nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	err = repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}
	result, err := repo.GetPaginated(ctx, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetPaginated() error = %v", err)
	}
	if len(result.Reports) != 0 {
		t.Errorf("after DeleteAll returned %d reports, want 0", len(result.Reports))
	}
}

func TestValidateReasonType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    repositories.ReportReasonType
		wantErr bool
	}{
		{name: "spam", input: "spam", want: repositories.ReasonSpam},
		{name: "violation", input: "violation", want: repositories.ReasonViolation},
		{name: "misleading", input: "misleading", want: repositories.ReasonMisleading},
		{name: "sexual", input: "sexual", want: repositories.ReasonSexual},
		{name: "rude", input: "rude", want: repositories.ReasonRude},
		{name: "other", input: "other", want: repositories.ReasonOther},
		{name: "invalid", input: "invalid", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ValidateReasonType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReasonType(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ValidateReasonType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateReportStatus(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    repositories.ReportStatus
		wantErr bool
	}{
		{name: "pending", input: "pending", want: repositories.StatusPending},
		{name: "resolved", input: "resolved", want: repositories.StatusResolved},
		{name: "dismissed", input: "dismissed", want: repositories.StatusDismissed},
		{name: "invalid", input: "invalid", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repositories.ValidateReportStatus(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateReportStatus(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ValidateReportStatus(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
