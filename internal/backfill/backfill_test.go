package backfill

import (
	"context"
	"testing"

	"github.com/GainForest/hyperindex/internal/testutil"
)

func TestPLCDocument_ToAtprotoData(t *testing.T) {
	tests := []struct {
		name string
		doc  PLCDocument
		did  string
		want AtprotoData
	}{
		{
			name: "full document",
			doc: PLCDocument{
				Service: []PLCService{
					{
						ID:              "#atproto_pds",
						Type:            "AtprotoPersonalDataServer",
						ServiceEndpoint: "https://pds.example.com",
					},
				},
				AlsoKnownAs: []string{"at://alice.example.com"},
			},
			did: "did:plc:abc123",
			want: AtprotoData{
				DID:    "did:plc:abc123",
				Handle: "alice.example.com",
				PDS:    "https://pds.example.com",
			},
		},
		{
			name: "no handle",
			doc: PLCDocument{
				Service: []PLCService{
					{
						Type:            "AtprotoPersonalDataServer",
						ServiceEndpoint: "https://pds.example.com",
					},
				},
				AlsoKnownAs: nil,
			},
			did: "did:plc:abc123",
			want: AtprotoData{
				DID:    "did:plc:abc123",
				Handle: "",
				PDS:    "https://pds.example.com",
			},
		},
		{
			name: "no pds",
			doc: PLCDocument{
				Service:     nil,
				AlsoKnownAs: []string{"at://bob.example.com"},
			},
			did: "did:plc:xyz789",
			want: AtprotoData{
				DID:    "did:plc:xyz789",
				Handle: "bob.example.com",
				PDS:    "https://bsky.social", // Default
			},
		},
		{
			name: "multiple services",
			doc: PLCDocument{
				Service: []PLCService{
					{
						Type:            "OtherService",
						ServiceEndpoint: "https://other.example.com",
					},
					{
						Type:            "AtprotoPersonalDataServer",
						ServiceEndpoint: "https://correct-pds.example.com",
					},
				},
				AlsoKnownAs: []string{"at://user.example.com"},
			},
			did: "did:plc:multi",
			want: AtprotoData{
				DID:    "did:plc:multi",
				Handle: "user.example.com",
				PDS:    "https://correct-pds.example.com",
			},
		},
		{
			name: "non-at handle ignored",
			doc: PLCDocument{
				Service: []PLCService{
					{
						Type:            "AtprotoPersonalDataServer",
						ServiceEndpoint: "https://pds.example.com",
					},
				},
				AlsoKnownAs: []string{"https://example.com/profile", "at://real.handle.com"},
			},
			did: "did:plc:handle",
			want: AtprotoData{
				DID:    "did:plc:handle",
				Handle: "real.handle.com",
				PDS:    "https://pds.example.com",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.doc.ToAtprotoData(tt.did)
			if got.DID != tt.want.DID {
				t.Errorf("DID = %q, want %q", got.DID, tt.want.DID)
			}
			if got.Handle != tt.want.Handle {
				t.Errorf("Handle = %q, want %q", got.Handle, tt.want.Handle)
			}
			if got.PDS != tt.want.PDS {
				t.Errorf("PDS = %q, want %q", got.PDS, tt.want.PDS)
			}
		})
	}
}

func TestBackfillerStoreResolvedActorPreservesHandleWhenResolutionHasNone(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	did := "did:plc:alice"
	if err := db.Actors.UpsertIdentity(ctx, did, "alice.example"); err != nil {
		t.Fatalf("UpsertIdentity() error = %v", err)
	}

	backfiller := &Backfiller{actorsRepo: db.Actors}
	if err := backfiller.storeResolvedActor(ctx, &AtprotoData{DID: did}); err != nil {
		t.Fatalf("storeResolvedActor() error = %v", err)
	}

	actor, err := db.Actors.GetByDID(ctx, did)
	if err != nil {
		t.Fatalf("GetByDID() error = %v", err)
	}
	if actor.Handle != "alice.example" {
		t.Fatalf("actor.Handle = %q, want alice.example", actor.Handle)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.RelayURL != DefaultRelayURL {
		t.Errorf("RelayURL = %q, want %q", cfg.RelayURL, DefaultRelayURL)
	}

	if cfg.PLCURL != DefaultPLCURL {
		t.Errorf("PLCURL = %q, want %q", cfg.PLCURL, DefaultPLCURL)
	}

	if cfg.MaxConcurrentRepos <= 0 {
		t.Errorf("MaxConcurrentRepos = %d, want > 0", cfg.MaxConcurrentRepos)
	}

	if cfg.MaxConcurrentPerPDS <= 0 {
		t.Errorf("MaxConcurrentPerPDS = %d, want > 0", cfg.MaxConcurrentPerPDS)
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name      string
		relayURL  string
		plcURL    string
		wantRelay string
		wantPLC   string
	}{
		{
			name:      "defaults",
			relayURL:  "",
			plcURL:    "",
			wantRelay: DefaultRelayURL,
			wantPLC:   DefaultPLCURL,
		},
		{
			name:      "custom relay",
			relayURL:  "https://custom-relay.example.com",
			plcURL:    "",
			wantRelay: "https://custom-relay.example.com",
			wantPLC:   DefaultPLCURL,
		},
		{
			name:      "custom plc",
			relayURL:  "",
			plcURL:    "https://custom-plc.example.com",
			wantRelay: DefaultRelayURL,
			wantPLC:   "https://custom-plc.example.com",
		},
		{
			name:      "both custom",
			relayURL:  "https://relay.example.com",
			plcURL:    "https://plc.example.com",
			wantRelay: "https://relay.example.com",
			wantPLC:   "https://plc.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.relayURL, tt.plcURL)
			if client.relayURL != tt.wantRelay {
				t.Errorf("relayURL = %q, want %q", client.relayURL, tt.wantRelay)
			}
			if client.plcURL != tt.wantPLC {
				t.Errorf("plcURL = %q, want %q", client.plcURL, tt.wantPLC)
			}
		})
	}
}

func TestStats_Duration(t *testing.T) {
	stats := Stats{}

	// Duration before end time set should return time since start
	// stats.StartTime is already zero value

	// Set an end time
	stats.EndTime = stats.StartTime.Add(5 * 1e9) // 5 seconds

	if stats.Duration() != 5*1e9 {
		t.Errorf("Duration() = %v, want 5s", stats.Duration())
	}
}
