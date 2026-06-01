package admin

import (
	"context"
	"reflect"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestResolverRemoveLabelerSubscribeURLPersistsOverride(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	resolver := NewResolver(&Repositories{Config: db.Config}, "did:plc:test-labeler", nil)
	resolver.SetLabelerSubscribeConfig(true, " wss://one.example/xrpc/com.atproto.label.subscribeLabels, wss://two.example/labels ")

	settings, err := resolver.RemoveLabelerSubscribeURL(ctx, " wss://one.example/xrpc/com.atproto.label.subscribeLabels ")
	if err != nil {
		t.Fatalf("RemoveLabelerSubscribeURL() error = %v", err)
	}

	gotURLs, ok := settings["labelerSubscribeUrls"].([]string)
	if !ok {
		t.Fatalf("labelerSubscribeUrls = %#v, want []string", settings["labelerSubscribeUrls"])
	}
	wantURLs := []string{"wss://two.example/labels"}
	if !reflect.DeepEqual(gotURLs, wantURLs) {
		t.Fatalf("labelerSubscribeUrls = %#v, want %#v", gotURLs, wantURLs)
	}

	stored, err := db.Config.Get(ctx, repositories.ConfigKeyLabelerSubscribeURLs)
	if err != nil {
		t.Fatalf("Get(labeler_subscribe_urls) error = %v", err)
	}
	if stored != "wss://two.example/labels" {
		t.Fatalf("stored labeler URLs = %q, want %q", stored, "wss://two.example/labels")
	}
}

func TestResolverRemoveLabelerSubscribeURLRequiresExistingURL(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()

	resolver := NewResolver(&Repositories{Config: db.Config}, "did:plc:test-labeler", nil)
	resolver.SetLabelerSubscribeConfig(true, "wss://one.example/xrpc/com.atproto.label.subscribeLabels")

	if _, err := resolver.RemoveLabelerSubscribeURL(ctx, "wss://missing.example/labels"); err == nil {
		t.Fatal("RemoveLabelerSubscribeURL() error = nil, want error")
	}

	if _, err := db.Config.Get(ctx, repositories.ConfigKeyLabelerSubscribeURLs); err == nil {
		t.Fatal("expected no persisted override when removal fails")
	}
}
