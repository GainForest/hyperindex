package tap_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/tap"
)

const historyURI = "at://did:plc:alice/app.gainforest.dwc.occurrence/occ1"

func occurrenceEvent(action tap.ActionType, cid, body string, live bool) *tap.RecordEvent {
	event := &tap.RecordEvent{
		Live: live, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", RKey: "occ1",
		Action: action, CID: cid,
	}
	if body != "" {
		event.Record = json.RawMessage(body)
	}
	return event
}

func versionActions(t *testing.T, repo *repositories.RecordVersionsRepository, uri string) []string {
	t.Helper()
	versions, err := repo.ListByURI(context.Background(), uri, 0, 500)
	if err != nil {
		t.Fatalf("ListByURI: %v", err)
	}
	actions := make([]string, 0, len(versions))
	for _, v := range versions {
		actions = append(actions, v.Action+":"+v.CID)
	}
	return actions
}

func TestIndexHandler_RecordHistory(t *testing.T) {
	handler, db, _ := setupHandler(t)
	handler.WithRecordHistory(db.RecordVersions, repositories.NewCollectionMatcher("app.gainforest.dwc.occurrence"))
	ctx := context.Background()

	for i, event := range []*tap.RecordEvent{
		occurrenceEvent(tap.ActionCreate, "cid1", `{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`, true),
		occurrenceEvent(tap.ActionCreate, "cid1", `{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`, false), // resync of the same version
		occurrenceEvent(tap.ActionUpdate, "cid2", `{"scientificName":"Hirundo tahitica","identifiedBy":"Maria"}`, true),
		// Collections without history are indexed as before and leave no versions.
		{Live: true, DID: "did:plc:alice", Collection: "app.bsky.feed.post", RKey: "p1", Action: tap.ActionCreate, CID: "cidp", Record: json.RawMessage(`{"text":"hi"}`)},
	} {
		if err := handler.HandleRecord(ctx, event); err != nil {
			t.Fatalf("event %d: HandleRecord: %v", i, err)
		}
	}

	got := versionActions(t, db.RecordVersions, historyURI)
	if len(got) != 2 || got[0] != "create:cid1" || got[1] != "update:cid2" {
		t.Fatalf("versions = %v, want [create:cid1 update:cid2]", got)
	}
	if posts := versionActions(t, db.RecordVersions, "at://did:plc:alice/app.bsky.feed.post/p1"); len(posts) != 0 {
		t.Fatalf("untracked collection versions = %v, want none", posts)
	}

	// Deleting the record removes its history; a redelivered delete is fine.
	for i := 0; i < 2; i++ {
		if err := handler.HandleRecord(ctx, occurrenceEvent(tap.ActionDelete, "", "", true)); err != nil {
			t.Fatalf("delete %d: HandleRecord: %v", i, err)
		}
	}
	if got := versionActions(t, db.RecordVersions, historyURI); len(got) != 0 {
		t.Fatalf("versions after delete = %v, want none", got)
	}
}

func TestIndexHandler_DeletePurgesHistoryAfterTrackingStops(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()
	// History was recorded while the collection was tracked...
	body := `{"scientificName":"Quercus robur"}`
	if err := db.RecordVersions.Append(ctx, repositories.RecordVersionWrite{
		URI: historyURI, CID: "cid1", DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence",
		Action: repositories.RecordVersionCreate, JSON: &body,
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	// ...and tracking has since been switched off (empty matcher).
	handler.WithRecordHistory(db.RecordVersions, repositories.CollectionMatcher{})
	if err := handler.HandleRecord(ctx, occurrenceEvent(tap.ActionCreate, "cid2", `{"scientificName":"Quercus petraea"}`, true)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := versionActions(t, db.RecordVersions, historyURI); len(got) != 1 {
		t.Fatalf("untracked write recorded a version: %v", got)
	}
	if err := handler.HandleRecord(ctx, occurrenceEvent(tap.ActionDelete, "", "", true)); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := versionActions(t, db.RecordVersions, historyURI); len(got) != 0 {
		t.Fatalf("versions after delete = %v, want none", got)
	}
}

func TestIndexHandler_NoHistoryByDefault(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()
	if err := handler.HandleRecord(ctx, occurrenceEvent(tap.ActionCreate, "cid1", `{"scientificName":"Quercus robur"}`, true)); err != nil {
		t.Fatalf("HandleRecord: %v", err)
	}
	if got := versionActions(t, db.RecordVersions, historyURI); len(got) != 0 {
		t.Fatalf("versions without WithRecordHistory = %v, want none", got)
	}
}
