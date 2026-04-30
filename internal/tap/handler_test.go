package tap_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"

	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/tap"
	"github.com/GainForest/hyperindex/internal/testutil"
)

// setupHandler creates an IndexHandler backed by a real in-memory SQLite database.
func setupHandler(t *testing.T) (*tap.IndexHandler, *testutil.TestDB, *subscription.PubSub) {
	t.Helper()
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	handler := tap.NewIndexHandler(db.Records, db.Actors, db.Activity, pubsub)
	return handler, db, pubsub
}

func TestIndexHandler_HandleRecord_Create(t *testing.T) {
	handler, db, pubsub := setupHandler(t)
	ctx := context.Background()

	// Subscribe to capture published events
	sub := pubsub.Subscribe("app.bsky.feed.post")
	defer pubsub.Unsubscribe(sub)

	event := &tap.RecordEvent{
		Live:       true,
		Rev:        "rev1",
		DID:        "did:plc:alice",
		Collection: "app.bsky.feed.post",
		RKey:       "post1",
		Action:     tap.ActionCreate,
		CID:        "bafyrei123",
		Record:     json.RawMessage(`{"text":"hello"}`),
	}

	if err := handler.HandleRecord(ctx, event); err != nil {
		t.Fatalf("HandleRecord returned error: %v", err)
	}

	// Verify record was inserted
	uri := "at://did:plc:alice/app.bsky.feed.post/post1"
	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("record not found after create: %v", err)
	}
	if rec.CID != "bafyrei123" {
		t.Errorf("expected CID %q, got %q", "bafyrei123", rec.CID)
	}

	// Verify actor was upserted
	actor, err := db.Actors.GetByDID(ctx, "did:plc:alice")
	if err != nil {
		t.Fatalf("actor not found after create: %v", err)
	}
	if actor.DID != "did:plc:alice" {
		t.Errorf("expected DID %q, got %q", "did:plc:alice", actor.DID)
	}

	// Verify pubsub event was published
	select {
	case pubEvent := <-sub.Events:
		if pubEvent.Type != subscription.EventCreate {
			t.Errorf("expected EventCreate, got %q", pubEvent.Type)
		}
		if pubEvent.URI != uri {
			t.Errorf("expected URI %q, got %q", uri, pubEvent.URI)
		}
		if pubEvent.CID != "bafyrei123" {
			t.Errorf("expected CID %q, got %q", "bafyrei123", pubEvent.CID)
		}
	default:
		t.Error("expected pubsub event to be published for create")
	}
}

func TestIndexHandler_HandleRecord_Update(t *testing.T) {
	handler, db, pubsub := setupHandler(t)
	ctx := context.Background()

	// Subscribe to capture published events
	sub := pubsub.Subscribe("app.bsky.feed.post")
	defer pubsub.Unsubscribe(sub)

	// First create the record
	createEvent := &tap.RecordEvent{
		DID:        "did:plc:alice",
		Collection: "app.bsky.feed.post",
		RKey:       "post1",
		Action:     tap.ActionCreate,
		CID:        "bafyrei_original",
		Record:     json.RawMessage(`{"text":"original"}`),
	}
	if err := handler.HandleRecord(ctx, createEvent); err != nil {
		t.Fatalf("HandleRecord (create) returned error: %v", err)
	}
	// Drain the create event
	<-sub.Events

	// Now update the record
	updateEvent := &tap.RecordEvent{
		DID:        "did:plc:alice",
		Collection: "app.bsky.feed.post",
		RKey:       "post1",
		Action:     tap.ActionUpdate,
		CID:        "bafyrei_updated",
		Record:     json.RawMessage(`{"text":"updated"}`),
	}
	if err := handler.HandleRecord(ctx, updateEvent); err != nil {
		t.Fatalf("HandleRecord (update) returned error: %v", err)
	}

	// Verify record was updated
	uri := "at://did:plc:alice/app.bsky.feed.post/post1"
	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("record not found after update: %v", err)
	}
	if rec.CID != "bafyrei_updated" {
		t.Errorf("expected updated CID %q, got %q", "bafyrei_updated", rec.CID)
	}

	// Verify pubsub event was published with EventUpdate
	select {
	case pubEvent := <-sub.Events:
		if pubEvent.Type != subscription.EventUpdate {
			t.Errorf("expected EventUpdate, got %q", pubEvent.Type)
		}
		if pubEvent.URI != uri {
			t.Errorf("expected URI %q, got %q", uri, pubEvent.URI)
		}
	default:
		t.Error("expected pubsub event to be published for update")
	}
}

func TestIndexHandler_HandleRecord_Delete(t *testing.T) {
	handler, db, pubsub := setupHandler(t)
	ctx := context.Background()

	// Subscribe to capture published events
	sub := pubsub.Subscribe("app.bsky.feed.post")
	defer pubsub.Unsubscribe(sub)

	// First create the record
	createEvent := &tap.RecordEvent{
		DID:        "did:plc:alice",
		Collection: "app.bsky.feed.post",
		RKey:       "post1",
		Action:     tap.ActionCreate,
		CID:        "bafyrei123",
		Record:     json.RawMessage(`{"text":"hello"}`),
	}
	if err := handler.HandleRecord(ctx, createEvent); err != nil {
		t.Fatalf("HandleRecord (create) returned error: %v", err)
	}
	// Drain the create event
	<-sub.Events

	// Now delete the record
	deleteEvent := &tap.RecordEvent{
		DID:        "did:plc:alice",
		Collection: "app.bsky.feed.post",
		RKey:       "post1",
		Action:     tap.ActionDelete,
	}
	if err := handler.HandleRecord(ctx, deleteEvent); err != nil {
		t.Fatalf("HandleRecord (delete) returned error: %v", err)
	}

	// Verify record was deleted
	uri := "at://did:plc:alice/app.bsky.feed.post/post1"
	_, err := db.Records.GetByURI(ctx, uri)
	if err == nil {
		t.Error("expected record to be deleted, but it still exists")
	}

	// Verify pubsub event was published with EventDelete
	select {
	case pubEvent := <-sub.Events:
		if pubEvent.Type != subscription.EventDelete {
			t.Errorf("expected EventDelete, got %q", pubEvent.Type)
		}
		if pubEvent.URI != uri {
			t.Errorf("expected URI %q, got %q", uri, pubEvent.URI)
		}
	default:
		t.Error("expected pubsub event to be published for delete")
	}
}

func TestIndexHandler_HandleRecord_Create_UpsertActor(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()

	event := &tap.RecordEvent{
		DID:        "did:plc:bob",
		Collection: "app.bsky.feed.post",
		RKey:       "post2",
		Action:     tap.ActionCreate,
		CID:        "bafyrei456",
		Record:     json.RawMessage(`{"text":"hi"}`),
	}

	if err := handler.HandleRecord(ctx, event); err != nil {
		t.Fatalf("HandleRecord returned error: %v", err)
	}

	// Verify actor was upserted
	actor, err := db.Actors.GetByDID(ctx, "did:plc:bob")
	if err != nil {
		t.Fatalf("actor not found: %v", err)
	}
	if actor.DID != "did:plc:bob" {
		t.Errorf("expected DID %q, got %q", "did:plc:bob", actor.DID)
	}
}

func TestIndexHandler_HandleRecord_ActivityLogged(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()

	event := &tap.RecordEvent{
		DID:        "did:plc:carol",
		Collection: "app.bsky.feed.post",
		RKey:       "post3",
		Action:     tap.ActionCreate,
		CID:        "bafyrei789",
		Record:     json.RawMessage(`{"text":"activity test"}`),
	}

	if err := handler.HandleRecord(ctx, event); err != nil {
		t.Fatalf("HandleRecord returned error: %v", err)
	}

	// Verify activity was logged
	count, err := db.Activity.GetCount(ctx)
	if err != nil {
		t.Fatalf("failed to get activity count: %v", err)
	}
	if count == 0 {
		t.Error("expected activity to be logged, but count is 0")
	}

	// Verify activity status is "completed" (not "pending")
	entries, err := db.Activity.GetRecentActivity(ctx, 1)
	if err != nil {
		t.Fatalf("failed to get recent activity: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one activity entry")
	}
	if entries[0].Status != "completed" {
		t.Errorf("expected activity status %q, got %q", "completed", entries[0].Status)
	}
}

func TestIndexHandler_HandleRecord_NilActivity(t *testing.T) {
	// Handler with nil activity repo should not panic
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	handler := tap.NewIndexHandler(db.Records, db.Actors, nil, pubsub)
	ctx := context.Background()

	event := &tap.RecordEvent{
		DID:        "did:plc:dave",
		Collection: "app.bsky.feed.post",
		RKey:       "post4",
		Action:     tap.ActionCreate,
		CID:        "bafyreinil",
		Record:     json.RawMessage(`{"text":"no activity"}`),
	}

	if err := handler.HandleRecord(ctx, event); err != nil {
		t.Fatalf("HandleRecord with nil activity returned error: %v", err)
	}
}

func TestIndexHandler_HandleRecord_NilPubSub(t *testing.T) {
	// Handler with nil pubsub should not panic
	db := testutil.SetupTestDB(t)
	handler := tap.NewIndexHandler(db.Records, db.Actors, nil, nil)
	ctx := context.Background()

	// Test create event does not panic and stores the record
	createEvent := &tap.RecordEvent{
		DID:        "did:plc:grace",
		Collection: "app.bsky.feed.post",
		RKey:       "post5",
		Action:     tap.ActionCreate,
		CID:        "bafyreinilpubsub",
		Record:     json.RawMessage(`{"text":"no pubsub"}`),
	}

	if err := handler.HandleRecord(ctx, createEvent); err != nil {
		t.Fatalf("HandleRecord (create) with nil pubsub returned error: %v", err)
	}

	// Verify record was still stored correctly
	uri := "at://did:plc:grace/app.bsky.feed.post/post5"
	rec, err := db.Records.GetByURI(ctx, uri)
	if err != nil {
		t.Fatalf("record not found after create with nil pubsub: %v", err)
	}
	if rec.CID != "bafyreinilpubsub" {
		t.Errorf("expected CID %q, got %q", "bafyreinilpubsub", rec.CID)
	}

	// Test delete event does not panic
	deleteEvent := &tap.RecordEvent{
		DID:        "did:plc:grace",
		Collection: "app.bsky.feed.post",
		RKey:       "post5",
		Action:     tap.ActionDelete,
	}

	if err := handler.HandleRecord(ctx, deleteEvent); err != nil {
		t.Fatalf("HandleRecord (delete) with nil pubsub returned error: %v", err)
	}

	// Verify record was deleted
	_, err = db.Records.GetByURI(ctx, uri)
	if err == nil {
		t.Error("expected record to be deleted, but it still exists")
	}
}

// TestIndexHandler_HandleRecord_DeleteNonExistent verifies that deleting a record
// that does not exist is not an error (SQL DELETE of 0 rows is fine) and that
// pubsub events are still published.
func TestIndexHandler_HandleRecord_DeleteNonExistent(t *testing.T) {
	handler, _, pubsub := setupHandler(t)
	ctx := context.Background()

	// Subscribe to capture published events
	sub := pubsub.Subscribe("app.bsky.feed.post")
	defer pubsub.Unsubscribe(sub)

	// Delete a record that does not exist — should succeed (0 rows affected is fine)
	deleteEvent := &tap.RecordEvent{
		DID:        "did:plc:nonexistent",
		Collection: "app.bsky.feed.post",
		RKey:       "doesnotexist",
		Action:     tap.ActionDelete,
	}

	if err := handler.HandleRecord(ctx, deleteEvent); err != nil {
		t.Fatalf("HandleRecord (delete non-existent) returned unexpected error: %v", err)
	}

	// Verify pubsub event was still published (delete succeeded)
	select {
	case pubEvent := <-sub.Events:
		if pubEvent.Type != subscription.EventDelete {
			t.Errorf("expected EventDelete, got %q", pubEvent.Type)
		}
	default:
		t.Error("expected pubsub event to be published for delete of non-existent record")
	}
}

func TestIndexHandler_HandleRecord_DeleteActivityCompleted(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()

	// First create a record so there is something to delete.
	createEvent := &tap.RecordEvent{
		DID:        "did:plc:henry",
		Collection: "app.bsky.feed.post",
		RKey:       "post-del-activity",
		Action:     tap.ActionCreate,
		CID:        "bafyreihenry",
		Record:     json.RawMessage(`{"text":"to be deleted"}`),
	}
	if err := handler.HandleRecord(ctx, createEvent); err != nil {
		t.Fatalf("HandleRecord (create) returned error: %v", err)
	}

	// Now delete the record.
	deleteEvent := &tap.RecordEvent{
		DID:        "did:plc:henry",
		Collection: "app.bsky.feed.post",
		RKey:       "post-del-activity",
		Action:     tap.ActionDelete,
	}
	if err := handler.HandleRecord(ctx, deleteEvent); err != nil {
		t.Fatalf("HandleRecord (delete) returned error: %v", err)
	}

	// GetRecentActivity returns entries ordered by timestamp DESC, so the delete
	// entry should be first. Fetch enough entries to find the delete one.
	entries, err := db.Activity.GetRecentActivity(ctx, 10)
	if err != nil {
		t.Fatalf("failed to get recent activity: %v", err)
	}

	found := false
	for _, e := range entries {
		if e.Operation == "delete" {
			found = true
			if e.Status != "completed" {
				t.Errorf("expected delete activity status %q, got %q", "completed", e.Status)
			}
			break
		}
	}
	if !found {
		t.Error("expected to find an activity entry with operation=\"delete\"")
	}
}

func TestIndexHandler_HandleIdentity(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()

	event := &tap.IdentityEvent{
		DID:      "did:plc:eve",
		Handle:   "eve.bsky.social",
		IsActive: true,
		Status:   "active",
	}

	if err := handler.HandleIdentity(ctx, event); err != nil {
		t.Fatalf("HandleIdentity returned error: %v", err)
	}

	// Verify actor was upserted with handle
	actor, err := db.Actors.GetByDID(ctx, "did:plc:eve")
	if err != nil {
		t.Fatalf("actor not found after HandleIdentity: %v", err)
	}
	if actor.DID != "did:plc:eve" {
		t.Errorf("expected DID %q, got %q", "did:plc:eve", actor.DID)
	}
	if actor.Handle != "eve.bsky.social" {
		t.Errorf("expected handle %q, got %q", "eve.bsky.social", actor.Handle)
	}
}

func TestIndexHandler_HandleIdentity_UpdatesHandle(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()

	// First upsert with old handle
	if err := db.Actors.Upsert(ctx, "did:plc:frank", "frank-old.bsky.social"); err != nil {
		t.Fatalf("failed to upsert actor: %v", err)
	}

	// Now handle identity event with new handle
	event := &tap.IdentityEvent{
		DID:      "did:plc:frank",
		Handle:   "frank-new.bsky.social",
		IsActive: true,
		Status:   "active",
	}

	if err := handler.HandleIdentity(ctx, event); err != nil {
		t.Fatalf("HandleIdentity returned error: %v", err)
	}

	// Verify handle was updated
	actor, err := db.Actors.GetByDID(ctx, "did:plc:frank")
	if err != nil {
		t.Fatalf("actor not found: %v", err)
	}
	if actor.Handle != "frank-new.bsky.social" {
		t.Errorf("expected updated handle %q, got %q", "frank-new.bsky.social", actor.Handle)
	}
}

func TestIndexHandler_HandleIdentity_TapShapedActivePayloadKeepsRecords(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()
	did := "did:plc:tapactive"
	uri := "at://did:plc:tapactive/app.bsky.feed.post/post1"

	if err := db.Actors.Upsert(ctx, did, "old-handle.bsky.social"); err != nil {
		t.Fatalf("failed to seed actor: %v", err)
	}
	if _, err := db.Records.Insert(ctx, uri, "bafyreitapactive", did, "app.bsky.feed.post", `{"text":"keep me"}`); err != nil {
		t.Fatalf("failed to seed record: %v", err)
	}

	event, err := tap.ParseEvent([]byte(`{"id":1,"type":"identity","identity":{"did":"did:plc:tapactive","handle":"new-handle.bsky.social","is_active":true,"status":"active"}}`))
	if err != nil {
		t.Fatalf("ParseEvent returned error: %v", err)
	}

	if err := handler.HandleIdentity(ctx, event.Identity); err != nil {
		t.Fatalf("HandleIdentity returned error: %v", err)
	}

	actor, err := db.Actors.GetByDID(ctx, did)
	if err != nil {
		t.Fatalf("expected actor to remain: %v", err)
	}
	if actor.Handle != "new-handle.bsky.social" {
		t.Fatalf("expected updated handle %q, got %q", "new-handle.bsky.social", actor.Handle)
	}
	if _, err := db.Records.GetByURI(ctx, uri); err != nil {
		t.Fatalf("expected record to remain: %v", err)
	}
}

func TestIndexHandler_HandleIdentity_PurgePolicy(t *testing.T) {
	tests := []struct {
		name       string
		isActive   bool
		status     string
		wantPurged bool
	}{
		{name: "active status keeps data", isActive: true, status: "active", wantPurged: false},
		{name: "active status keeps data when is_active false", isActive: false, status: "active", wantPurged: false},
		{name: "empty status keeps data when is_active false", isActive: false, status: "", wantPurged: false},
		{name: "deleted status purges", isActive: false, status: "deleted", wantPurged: true},
		{name: "deactivated status purges", isActive: false, status: "deactivated", wantPurged: true},
		{name: "suspended status purges", isActive: true, status: "suspended", wantPurged: true},
		{name: "takendown status purges", isActive: true, status: "takendown", wantPurged: true},
		{name: "mixed-case status purges", isActive: true, status: "Suspended", wantPurged: true},
		{name: "unknown status keeps data when is_active false", isActive: false, status: "mystery", wantPurged: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler, db, _ := setupHandler(t)
			ctx := context.Background()

			did := "did:plc:testpurge"
			if err := db.Actors.Upsert(ctx, did, "old-handle.bsky.social"); err != nil {
				t.Fatalf("failed to seed actor: %v", err)
			}

			_, err := db.Records.Insert(ctx,
				"at://did:plc:testpurge/app.certified.actor.profile/p1",
				"bafyreitest1",
				did,
				"app.certified.actor.profile",
				`{"displayName":"Seed"}`,
			)
			if err != nil {
				t.Fatalf("failed to seed profile record: %v", err)
			}

			_, err = db.Records.Insert(ctx,
				"at://did:plc:testpurge/app.certified.actor.organization/o1",
				"bafyreitest2",
				did,
				"app.certified.actor.organization",
				`{"organizationType":"ngo"}`,
			)
			if err != nil {
				t.Fatalf("failed to seed organization record: %v", err)
			}

			event := &tap.IdentityEvent{
				DID:      did,
				Handle:   "new-handle.bsky.social",
				IsActive: tt.isActive,
				Status:   tt.status,
			}

			if err := handler.HandleIdentity(ctx, event); err != nil {
				t.Fatalf("HandleIdentity returned error: %v", err)
			}

			records, err := db.Records.GetByDID(ctx, did)
			if err != nil {
				t.Fatalf("GetByDID(records) error: %v", err)
			}

			actor, actorErr := db.Actors.GetByDID(ctx, did)
			if tt.wantPurged {
				if len(records) != 0 {
					t.Fatalf("expected records purged, got %d", len(records))
				}
				if !errors.Is(actorErr, sql.ErrNoRows) {
					t.Fatalf("expected actor deleted (sql.ErrNoRows), got actor=%v err=%v", actor, actorErr)
				}
				return
			}

			if len(records) == 0 {
				t.Fatal("expected records to remain for active identity")
			}
			if actorErr != nil {
				t.Fatalf("expected actor to remain, got error: %v", actorErr)
			}
			if actor.Handle != "new-handle.bsky.social" {
				t.Fatalf("expected updated handle %q, got %q", "new-handle.bsky.social", actor.Handle)
			}
		})
	}
}

func TestIndexHandler_ImplementsEventHandler(t *testing.T) {
	// Compile-time check that IndexHandler implements EventHandler
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	var _ tap.EventHandler = tap.NewIndexHandler(db.Records, db.Actors, db.Activity, pubsub)
}
