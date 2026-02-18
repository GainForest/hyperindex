package tap_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GainForest/hypergoat/internal/graphql/subscription"
	"github.com/GainForest/hypergoat/internal/tap"
	"github.com/GainForest/hypergoat/internal/testutil"
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

func TestIndexHandler_HandleRecord_DeleteError(t *testing.T) {
	// Deleting a non-existent record should NOT return an error because
	// SQL DELETE of 0 rows is not an error. This test verifies the happy path
	// of the delete code path (no DB error → no returned error).
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

func TestIndexHandler_ImplementsEventHandler(t *testing.T) {
	// Compile-time check that IndexHandler implements EventHandler
	db := testutil.SetupTestDB(t)
	pubsub := subscription.NewPubSub()
	var _ tap.EventHandler = tap.NewIndexHandler(db.Records, db.Actors, db.Activity, pubsub)
}
