package repositories_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupLabelsTest(t *testing.T) *repositories.LabelsRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Labels
}

func TestLabelsRepository_Insert(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	label, err := repo.Insert(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/abc", nil, "spam", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if label.ID <= 0 {
		t.Errorf("Insert() returned id = %d, want > 0", label.ID)
	}
	if label.Src != "did:plc:labeler" {
		t.Errorf("Src = %q, want %q", label.Src, "did:plc:labeler")
	}
	if label.URI != "at://did:plc:user/app.bsky.feed.post/abc" {
		t.Errorf("URI = %q, want %q", label.URI, "at://did:plc:user/app.bsky.feed.post/abc")
	}
	if label.Val != "spam" {
		t.Errorf("Val = %q, want %q", label.Val, "spam")
	}
	if label.Neg {
		t.Error("Neg = true, want false")
	}
	if label.CID != nil {
		t.Errorf("CID = %v, want nil", label.CID)
	}
	if label.Exp != nil {
		t.Errorf("Exp = %v, want nil", label.Exp)
	}
}

func TestLabelsRepository_InsertNegation(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	// Insert original label first
	_, err := repo.Insert(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/abc", nil, "spam", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Insert negation
	neg, err := repo.InsertNegation(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/abc", "spam")
	if err != nil {
		t.Fatalf("InsertNegation() error = %v", err)
	}
	if !neg.Neg {
		t.Error("Neg = false, want true")
	}
	if neg.Val != "spam" {
		t.Errorf("Val = %q, want %q", neg.Val, "spam")
	}
	if neg.Src != "did:plc:labeler" {
		t.Errorf("Src = %q, want %q", neg.Src, "did:plc:labeler")
	}
}

func TestLabelsRepository_GetByID(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	// Insert a label
	inserted, err := repo.Insert(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/abc", nil, "spam", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	t.Run("found", func(t *testing.T) {
		label, err := repo.GetByID(ctx, inserted.ID)
		if err != nil {
			t.Fatalf("GetByID() error = %v", err)
		}
		if label.ID != inserted.ID {
			t.Errorf("ID = %d, want %d", label.ID, inserted.ID)
		}
		if label.Val != "spam" {
			t.Errorf("Val = %q, want %q", label.Val, "spam")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 99999)
		if err == nil {
			t.Fatal("GetByID() expected error for non-existing ID, got nil")
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("GetByID() error = %v, want sql.ErrNoRows", err)
		}
	})
}

func TestLabelsRepository_GetByURIs(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	t.Run("empty returns nil", func(t *testing.T) {
		labels, err := repo.GetByURIs(ctx, []string{})
		if err != nil {
			t.Fatalf("GetByURIs() error = %v", err)
		}
		if labels != nil {
			t.Errorf("GetByURIs([]) = %v, want nil", labels)
		}
	})

	t.Run("returns active labels", func(t *testing.T) {
		uri := "at://did:plc:user/app.bsky.feed.post/abc"

		_, err := repo.Insert(ctx, "did:plc:labeler", uri, nil, "spam", nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}

		labels, err := repo.GetByURIs(ctx, []string{uri})
		if err != nil {
			t.Fatalf("GetByURIs() error = %v", err)
		}
		if len(labels) != 1 {
			t.Fatalf("GetByURIs() returned %d labels, want 1", len(labels))
		}
		if labels[0].Val != "spam" {
			t.Errorf("Val = %q, want %q", labels[0].Val, "spam")
		}
	})

	t.Run("negated labels excluded", func(t *testing.T) {
		uri := "at://did:plc:user/app.bsky.feed.post/negtest"

		_, err := repo.Insert(ctx, "did:plc:labeler", uri, nil, "porn", nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}

		// Negate the label (no sleep needed: queries now use id-based ordering)
		_, err = repo.InsertNegation(ctx, "did:plc:labeler", uri, "porn")
		if err != nil {
			t.Fatalf("InsertNegation() error = %v", err)
		}

		labels, err := repo.GetByURIs(ctx, []string{uri})
		if err != nil {
			t.Fatalf("GetByURIs() error = %v", err)
		}
		if len(labels) != 0 {
			t.Errorf("GetByURIs() returned %d labels, want 0 (negated label should be excluded)", len(labels))
		}
	})
}

func TestLabelsRepository_GetPaginated(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	uri1 := "at://did:plc:user/app.bsky.feed.post/abc"
	uri2 := "at://did:plc:user/app.bsky.feed.post/def"

	// Insert labels
	_, err := repo.Insert(ctx, "did:plc:labeler", uri1, nil, "spam", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:labeler", uri2, nil, "porn", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:labeler", uri1, nil, "nudity", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	t.Run("no filter", func(t *testing.T) {
		result, err := repo.GetPaginated(ctx, nil, nil, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Labels) != 3 {
			t.Errorf("GetPaginated() returned %d labels, want 3", len(result.Labels))
		}
		if result.TotalCount != 3 {
			t.Errorf("TotalCount = %d, want 3", result.TotalCount)
		}
	})

	t.Run("URI filter", func(t *testing.T) {
		result, err := repo.GetPaginated(ctx, &uri1, nil, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Labels) != 2 {
			t.Errorf("GetPaginated(uri=%q) returned %d labels, want 2", uri1, len(result.Labels))
		}
	})

	t.Run("val filter", func(t *testing.T) {
		val := "spam"
		result, err := repo.GetPaginated(ctx, nil, &val, 10, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Labels) != 1 {
			t.Errorf("GetPaginated(val=%q) returned %d labels, want 1", val, len(result.Labels))
		}
	})

	t.Run("cursor pagination", func(t *testing.T) {
		// Get first page
		result, err := repo.GetPaginated(ctx, nil, nil, 2, nil)
		if err != nil {
			t.Fatalf("GetPaginated() error = %v", err)
		}
		if len(result.Labels) != 2 {
			t.Fatalf("first page returned %d labels, want 2", len(result.Labels))
		}
		if !result.HasNextPage {
			t.Error("HasNextPage = false, want true")
		}

		// Get second page using last ID as cursor
		cursor := result.Labels[len(result.Labels)-1].ID
		result2, err := repo.GetPaginated(ctx, nil, nil, 2, &cursor)
		if err != nil {
			t.Fatalf("GetPaginated() page 2 error = %v", err)
		}
		if len(result2.Labels) != 1 {
			t.Errorf("second page returned %d labels, want 1", len(result2.Labels))
		}
		if result2.HasNextPage {
			t.Error("HasNextPage on last page = true, want false")
		}
	})
}

func TestLabelsRepository_HasTakedown(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	uri := "at://did:plc:user/app.bsky.feed.post/abc"

	t.Run("true with active takedown", func(t *testing.T) {
		_, err := repo.Insert(ctx, "did:plc:labeler", uri, nil, "!takedown", nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}

		has, err := repo.HasTakedown(ctx, uri)
		if err != nil {
			t.Fatalf("HasTakedown() error = %v", err)
		}
		if !has {
			t.Error("HasTakedown() = false, want true")
		}
	})

	t.Run("false after negation", func(t *testing.T) {
		_, err := repo.InsertNegation(ctx, "did:plc:labeler", uri, "!takedown")
		if err != nil {
			t.Fatalf("InsertNegation() error = %v", err)
		}

		has, err := repo.HasTakedown(ctx, uri)
		if err != nil {
			t.Fatalf("HasTakedown() error = %v", err)
		}
		if has {
			t.Error("HasTakedown() = true after negation, want false")
		}
	})
}

func TestLabelsRepository_GetTakedownURIs(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	t.Run("empty returns nil", func(t *testing.T) {
		uris, err := repo.GetTakedownURIs(ctx, []string{})
		if err != nil {
			t.Fatalf("GetTakedownURIs() error = %v", err)
		}
		if uris != nil {
			t.Errorf("GetTakedownURIs([]) = %v, want nil", uris)
		}
	})

	t.Run("returns URIs with active takedowns", func(t *testing.T) {
		uri1 := "at://did:plc:user/app.bsky.feed.post/td1"
		uri2 := "at://did:plc:user/app.bsky.feed.post/td2"
		uri3 := "at://did:plc:user/app.bsky.feed.post/td3"

		// Takedown on uri1 and uri2
		_, err := repo.Insert(ctx, "did:plc:labeler", uri1, nil, "!takedown", nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
		_, err = repo.Insert(ctx, "did:plc:labeler", uri2, nil, "!takedown", nil)
		if err != nil {
			t.Fatalf("Insert() error = %v", err)
		}

		uris, err := repo.GetTakedownURIs(ctx, []string{uri1, uri2, uri3})
		if err != nil {
			t.Fatalf("GetTakedownURIs() error = %v", err)
		}
		if len(uris) != 2 {
			t.Errorf("GetTakedownURIs() returned %d URIs, want 2", len(uris))
		}
	})
}

func TestLabelsRepository_DeleteAll(t *testing.T) {
	repo := setupLabelsTest(t)
	ctx := context.Background()

	// Insert labels
	_, err := repo.Insert(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/abc", nil, "spam", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	_, err = repo.Insert(ctx, "did:plc:labeler", "at://did:plc:user/app.bsky.feed.post/def", nil, "porn", nil)
	if err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Delete all
	err = repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}

	// Verify empty via GetPaginated
	result, err := repo.GetPaginated(ctx, nil, nil, 10, nil)
	if err != nil {
		t.Fatalf("GetPaginated() error = %v", err)
	}
	if len(result.Labels) != 0 {
		t.Errorf("GetPaginated() after DeleteAll returned %d labels, want 0", len(result.Labels))
	}
	if result.TotalCount != 0 {
		t.Errorf("TotalCount after DeleteAll = %d, want 0", result.TotalCount)
	}
}

func TestIsValidSubjectURI(t *testing.T) {
	tests := []struct {
		name string
		uri  string
		want bool
	}{
		{name: "at:// URI", uri: "at://did:plc:test/col/key", want: true},
		{name: "did: URI", uri: "did:plc:test", want: true},
		{name: "http URL", uri: "http://example.com", want: false},
		{name: "empty string", uri: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := repositories.IsValidSubjectURI(tt.uri)
			if got != tt.want {
				t.Errorf("IsValidSubjectURI(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}
