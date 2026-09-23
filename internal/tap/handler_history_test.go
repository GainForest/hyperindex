package tap_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/tap"
)

func TestIndexHandler_RecordHistory(t *testing.T) {
	handler, db, _ := setupHandler(t)
	handler.WithRecordHistory(db.RecordVersions, repositories.NewCollectionMatcher("app.gainforest.dwc.occurrence"))
	ctx := context.Background()

	occurrence := func(action tap.ActionType, cid, body string, live bool) *tap.RecordEvent {
		return &tap.RecordEvent{
			Live: live, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", RKey: "occ1",
			Action: action, CID: cid, Record: json.RawMessage(body),
		}
	}
	events := []*tap.RecordEvent{
		occurrence(tap.ActionCreate, "cid1", `{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`, true),
		occurrence(tap.ActionCreate, "cid1", `{"scientificName":"Hirundo rustica","identifiedBy":"ai:gemini-3.8-flash"}`, false), // resync of the same version
		occurrence(tap.ActionUpdate, "cid2", `{"scientificName":"Hirundo tahitica","identifiedBy":"Maria"}`, true),
		{Live: true, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", RKey: "occ1", Action: tap.ActionDelete},
		{Live: true, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", RKey: "occ1", Action: tap.ActionDelete}, // redelivered delete
		// Collections without history are indexed as before and leave no versions.
		{Live: true, DID: "did:plc:alice", Collection: "app.bsky.feed.post", RKey: "p1", Action: tap.ActionCreate, CID: "cidp", Record: json.RawMessage(`{"text":"hi"}`)},
	}
	for i, event := range events {
		if err := handler.HandleRecord(ctx, event); err != nil {
			t.Fatalf("event %d: HandleRecord: %v", i, err)
		}
	}

	versions, err := db.RecordVersions.ListByURI(ctx, "at://did:plc:alice/app.gainforest.dwc.occurrence/occ1", 0, 500)
	if err != nil {
		t.Fatalf("ListByURI: %v", err)
	}
	var actions []string
	for _, v := range versions {
		actions = append(actions, v.Action+":"+v.CID)
	}
	want := []string{"create:cid1", "update:cid2", "delete:cid2"}
	if len(actions) != len(want) {
		t.Fatalf("versions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("versions = %v, want %v", actions, want)
		}
	}

	posts, err := db.RecordVersions.ListByURI(ctx, "at://did:plc:alice/app.bsky.feed.post/p1", 0, 500)
	if err != nil || len(posts) != 0 {
		t.Fatalf("untracked collection versions = %v (err %v), want none", posts, err)
	}
}

func TestIndexHandler_NoHistoryByDefault(t *testing.T) {
	handler, db, _ := setupHandler(t)
	ctx := context.Background()
	if err := handler.HandleRecord(ctx, &tap.RecordEvent{
		Live: true, DID: "did:plc:alice", Collection: "app.gainforest.dwc.occurrence", RKey: "occ2",
		Action: tap.ActionCreate, CID: "cid1", Record: json.RawMessage(`{"scientificName":"Quercus robur"}`),
	}); err != nil {
		t.Fatalf("HandleRecord: %v", err)
	}
	versions, err := db.RecordVersions.ListByURI(ctx, "at://did:plc:alice/app.gainforest.dwc.occurrence/occ2", 0, 500)
	if err != nil || len(versions) != 0 {
		t.Fatalf("versions without WithRecordHistory = %v (err %v), want none", versions, err)
	}
}
