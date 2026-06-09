package repositories_test

import (
	"context"
	"testing"
	"time"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupActivityTest(t *testing.T) *repositories.IndexingActivityRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Activity
}

func TestIndexingActivity_LogActivity(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	id, err := repo.LogActivity(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "abc123", `{"type":"create"}`)
	if err != nil {
		t.Fatalf("LogActivity() error = %v", err)
	}
	if id <= 0 {
		t.Errorf("LogActivity() returned id = %d, want > 0", id)
	}

	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 1 {
		t.Errorf("GetCount() = %d, want 1", count)
	}
}

func TestIndexingActivity_LogActivity_NormalizesEmptyEventJSON(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	id, err := repo.LogActivity(ctx, time.Now(), "delete", "app.bsky.feed.post", "did:plc:test1", "abc123", "")
	if err != nil {
		t.Fatalf("LogActivity() error = %v", err)
	}
	if id <= 0 {
		t.Errorf("LogActivity() returned id = %d, want > 0", id)
	}

	entries, err := repo.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetRecentActivity() returned %d entries, want 1", len(entries))
	}
	if entries[0].EventJSON != "{}" {
		t.Errorf("EventJSON = %q, want %q", entries[0].EventJSON, "{}")
	}
}

func TestIndexingActivity_LogActivity_NormalizesNullEventJSON(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	id, err := repo.LogActivity(ctx, time.Now(), "delete", "app.bsky.feed.post", "did:plc:test1", "abc123", "null")
	if err != nil {
		t.Fatalf("LogActivity() error = %v", err)
	}
	if id <= 0 {
		t.Errorf("LogActivity() returned id = %d, want > 0", id)
	}

	entries, err := repo.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetRecentActivity() returned %d entries, want 1", len(entries))
	}
	if entries[0].EventJSON != "{}" {
		t.Errorf("EventJSON = %q, want %q", entries[0].EventJSON, "{}")
	}
}

func TestIndexingActivity_LogActivity_RejectsInvalidEventJSON(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	_, err := repo.LogActivity(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "abc123", "not-json")
	if err == nil {
		t.Fatal("LogActivity() error = nil, want invalid JSON error")
	}

	count, countErr := repo.GetCount(ctx)
	if countErr != nil {
		t.Fatalf("GetCount() error = %v", countErr)
	}
	if count != 0 {
		t.Errorf("GetCount() = %d, want 0 after rejected invalid event JSON", count)
	}
}

func TestIndexingActivity_LogActivityWithStatus(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	id, err := repo.LogActivityWithStatus(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "abc123", `{"type":"create"}`, "success")
	if err != nil {
		t.Fatalf("LogActivityWithStatus() error = %v", err)
	}
	if id <= 0 {
		t.Errorf("LogActivityWithStatus() returned id = %d, want > 0", id)
	}

	// Verify the custom status is stored by checking recent activity
	entries, err := repo.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetRecentActivity() returned %d entries, want 1", len(entries))
	}
	if entries[0].Status != "success" {
		t.Errorf("Status = %q, want %q", entries[0].Status, "success")
	}
}

func TestIndexingActivity_UpdateStatus(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	id, err := repo.LogActivity(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "abc123", `{"type":"create"}`)
	if err != nil {
		t.Fatalf("LogActivity() error = %v", err)
	}

	errMsg := "something went wrong"
	err = repo.UpdateStatus(ctx, id, "error", &errMsg)
	if err != nil {
		t.Fatalf("UpdateStatus() error = %v", err)
	}

	entries, err := repo.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("GetRecentActivity() returned %d entries, want 1", len(entries))
	}
	if entries[0].Status != "error" {
		t.Errorf("Status = %q, want %q", entries[0].Status, "error")
	}
	if entries[0].ErrorMessage == nil {
		t.Fatal("ErrorMessage is nil, want non-nil")
	}
	if *entries[0].ErrorMessage != errMsg {
		t.Errorf("ErrorMessage = %q, want %q", *entries[0].ErrorMessage, errMsg)
	}
}

func TestIndexingActivity_GetRecentActivity(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	now := time.Now()

	// Log several entries with recent timestamps
	for i := 0; i < 3; i++ {
		_, err := repo.LogActivity(ctx, now, "create", "app.bsky.feed.post", "did:plc:test1", "rkey", `{}`)
		if err != nil {
			t.Fatalf("LogActivity() error = %v", err)
		}
	}

	entries, err := repo.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("GetRecentActivity() error = %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("GetRecentActivity(1 hour) returned %d entries, want 3", len(entries))
	}
}

func TestIndexingActivity_GetActivityBuckets(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	now := time.Now()

	// Log activities with different operations
	operations := []string{"create", "update", "delete"}
	for _, op := range operations {
		_, err := repo.LogActivity(ctx, now, op, "app.bsky.feed.post", "did:plc:test1", "rkey", `{}`)
		if err != nil {
			t.Fatalf("LogActivity() error = %v", err)
		}
	}

	buckets, err := repo.GetActivityBuckets(ctx, "ONE_HOUR")
	if err != nil {
		t.Fatalf("GetActivityBuckets() error = %v", err)
	}
	if len(buckets) == 0 {
		t.Fatal("GetActivityBuckets() returned 0 buckets, want at least 1")
	}

	// Verify aggregation: total across all buckets should be 3
	var totalSum int64
	for _, b := range buckets {
		totalSum += b.Total
	}
	if totalSum != 3 {
		t.Errorf("total across buckets = %d, want 3", totalSum)
	}
}

func TestIndexingActivity_CleanupOldActivity(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	// Log a recent entry
	_, err := repo.LogActivity(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "rkey", `{}`)
	if err != nil {
		t.Fatalf("LogActivity() error = %v", err)
	}

	// Cleanup entries older than 24 hours - should not delete the recent one
	err = repo.CleanupOldActivity(ctx, 24)
	if err != nil {
		t.Fatalf("CleanupOldActivity() error = %v", err)
	}

	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 1 {
		t.Errorf("GetCount() after cleanup = %d, want 1 (recent entry should remain)", count)
	}
}

func TestIndexingActivity_GetCount(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	// Empty table
	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("GetCount() on empty table = %d, want 0", count)
	}

	// After logging entries
	for i := 0; i < 3; i++ {
		_, err := repo.LogActivity(ctx, time.Now(), "create", "app.bsky.feed.post", "did:plc:test1", "rkey", `{}`)
		if err != nil {
			t.Fatalf("LogActivity() error = %v", err)
		}
	}

	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 3 {
		t.Errorf("GetCount() after 3 inserts = %d, want 3", count)
	}
}

func TestIndexingActivity_DeleteAll(t *testing.T) {
	repo := setupActivityTest(t)
	ctx := context.Background()

	// Insert entries
	for i := 0; i < 3; i++ {
		_, err := repo.LogActivity(ctx, time.Now(), "delete", "app.bsky.feed.post", "did:plc:test1", "rkey", `{}`)
		if err != nil {
			t.Fatalf("LogActivity() error = %v", err)
		}
	}

	// Verify they exist
	count, err := repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("GetCount() = %d, want 3 before delete", count)
	}

	// Delete all
	err = repo.DeleteAll(ctx)
	if err != nil {
		t.Fatalf("DeleteAll() error = %v", err)
	}

	// Verify empty
	count, err = repo.GetCount(ctx)
	if err != nil {
		t.Fatalf("GetCount() error = %v", err)
	}
	if count != 0 {
		t.Errorf("GetCount() after DeleteAll = %d, want 0", count)
	}
}
