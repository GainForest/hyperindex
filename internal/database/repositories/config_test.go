package repositories_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/testutil"
)

func setupConfigTest(t *testing.T) *repositories.ConfigRepository {
	t.Helper()
	db := testutil.SetupTestDB(t)
	return db.Config
}

func TestConfigRepository_GetSet(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, repo *repositories.ConfigRepository)
	}{
		{
			name: "set then get returns value",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.Set(ctx, "foo", "bar"); err != nil {
					t.Fatalf("Set() error = %v", err)
				}
				got, err := repo.Get(ctx, "foo")
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}
				if got != "bar" {
					t.Errorf("Get() = %q, want %q", got, "bar")
				}
			},
		},
		{
			name: "get non-existent key returns config key not found error",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				_, err := repo.Get(ctx, "does_not_exist")
				if err == nil {
					t.Fatal("Get() expected error, got nil")
				}
				if !strings.Contains(err.Error(), "config key not found") {
					t.Errorf("Get() error = %q, want it to contain %q", err.Error(), "config key not found")
				}
			},
		},
		{
			name: "set overwrites existing value",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.Set(ctx, "key", "first"); err != nil {
					t.Fatalf("Set() error = %v", err)
				}
				if err := repo.Set(ctx, "key", "second"); err != nil {
					t.Fatalf("Set() overwrite error = %v", err)
				}
				got, err := repo.Get(ctx, "key")
				if err != nil {
					t.Fatalf("Get() error = %v", err)
				}
				if got != "second" {
					t.Errorf("Get() = %q, want %q", got, "second")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			tt.run(t, repo)
		})
	}
}

func TestConfigRepository_Delete(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, repo *repositories.ConfigRepository)
	}{
		{
			name: "delete existing key then get returns error",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.Set(ctx, "to_delete", "value"); err != nil {
					t.Fatalf("Set() error = %v", err)
				}
				if err := repo.Delete(ctx, "to_delete"); err != nil {
					t.Fatalf("Delete() error = %v", err)
				}
				_, err := repo.Get(ctx, "to_delete")
				if err == nil {
					t.Fatal("Get() after Delete() expected error, got nil")
				}
				if !strings.Contains(err.Error(), "config key not found") {
					t.Errorf("Get() error = %q, want it to contain %q", err.Error(), "config key not found")
				}
			},
		},
		{
			name: "delete non-existent key returns no error",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.Delete(ctx, "never_existed"); err != nil {
					t.Errorf("Delete() non-existent key error = %v, want nil", err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			tt.run(t, repo)
		})
	}
}

func TestConfigRepository_DeleteDomainAuthority(t *testing.T) {
	repo := setupConfigTest(t)
	ctx := context.Background()

	if err := repo.Set(ctx, "domain_authority", "example.com"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	if err := repo.DeleteDomainAuthority(ctx); err != nil {
		t.Fatalf("DeleteDomainAuthority() error = %v", err)
	}

	_, err := repo.Get(ctx, "domain_authority")
	if err == nil {
		t.Fatal("Get() after DeleteDomainAuthority() expected error, got nil")
	}
	if !strings.Contains(err.Error(), "config key not found") {
		t.Errorf("Get() error = %q, want it to contain %q", err.Error(), "config key not found")
	}
}

func TestConfigRepository_AdminDIDs(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, repo *repositories.ConfigRepository)
	}{
		{
			name: "GetAdminDIDs with no admins returns empty slice",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				dids := repo.GetAdminDIDs(ctx)
				if len(dids) != 0 {
					t.Errorf("GetAdminDIDs() = %v, want empty slice", dids)
				}
			},
		},
		{
			name: "AddAdminDID then appears in GetAdminDIDs",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:admin1"); err != nil {
					t.Fatalf("AddAdminDID() error = %v", err)
				}
				dids := repo.GetAdminDIDs(ctx)
				if len(dids) != 1 {
					t.Fatalf("GetAdminDIDs() len = %d, want 1", len(dids))
				}
				if dids[0] != "did:plc:admin1" {
					t.Errorf("GetAdminDIDs()[0] = %q, want %q", dids[0], "did:plc:admin1")
				}
			},
		},
		{
			name: "AddAdminDID duplicate is idempotent",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:dup"); err != nil {
					t.Fatalf("AddAdminDID() first call error = %v", err)
				}
				if err := repo.AddAdminDID(ctx, "did:plc:dup"); err != nil {
					t.Fatalf("AddAdminDID() duplicate call error = %v", err)
				}
				dids := repo.GetAdminDIDs(ctx)
				if len(dids) != 1 {
					t.Errorf("GetAdminDIDs() len = %d, want 1 (no duplicate)", len(dids))
				}
			},
		},
		{
			name: "RemoveAdminDID removes from list",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:a"); err != nil {
					t.Fatalf("AddAdminDID(a) error = %v", err)
				}
				if err := repo.AddAdminDID(ctx, "did:plc:b"); err != nil {
					t.Fatalf("AddAdminDID(b) error = %v", err)
				}
				remaining, err := repo.RemoveAdminDID(ctx, "did:plc:a")
				if err != nil {
					t.Fatalf("RemoveAdminDID() error = %v", err)
				}
				if len(remaining) != 1 || remaining[0] != "did:plc:b" {
					t.Errorf("RemoveAdminDID() returned %v, want [did:plc:b]", remaining)
				}
				dids := repo.GetAdminDIDs(ctx)
				if len(dids) != 1 || dids[0] != "did:plc:b" {
					t.Errorf("GetAdminDIDs() = %v, want [did:plc:b]", dids)
				}
			},
		},
		{
			name: "RemoveAdminDID not found returns RemoveAdminNotFound error",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:exists"); err != nil {
					t.Fatalf("AddAdminDID() error = %v", err)
				}
				_, err := repo.RemoveAdminDID(ctx, "did:plc:ghost")
				if err == nil {
					t.Fatal("RemoveAdminDID() expected error, got nil")
				}
				if !errors.Is(err, repositories.RemoveAdminNotFound) {
					t.Errorf("RemoveAdminDID() error = %v, want RemoveAdminNotFound", err)
				}
			},
		},
		{
			name: "RemoveAdminDID last admin returns RemoveAdminLastAdmin error",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:only"); err != nil {
					t.Fatalf("AddAdminDID() error = %v", err)
				}
				_, err := repo.RemoveAdminDID(ctx, "did:plc:only")
				if err == nil {
					t.Fatal("RemoveAdminDID() expected error, got nil")
				}
				if !errors.Is(err, repositories.RemoveAdminLastAdmin) {
					t.Errorf("RemoveAdminDID() error = %v, want RemoveAdminLastAdmin", err)
				}
			},
		},
		{
			name: "SetAdminDIDs replaces entire list",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.AddAdminDID(ctx, "did:plc:old"); err != nil {
					t.Fatalf("AddAdminDID() error = %v", err)
				}
				newList := []string{"did:plc:x", "did:plc:y", "did:plc:z"}
				if err := repo.SetAdminDIDs(ctx, newList); err != nil {
					t.Fatalf("SetAdminDIDs() error = %v", err)
				}
				dids := repo.GetAdminDIDs(ctx)
				if len(dids) != 3 {
					t.Fatalf("GetAdminDIDs() len = %d, want 3", len(dids))
				}
				for i, want := range newList {
					if dids[i] != want {
						t.Errorf("GetAdminDIDs()[%d] = %q, want %q", i, dids[i], want)
					}
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			tt.run(t, repo)
		})
	}
}

func TestConfigRepository_IsAdmin(t *testing.T) {
	tests := []struct {
		name  string
		setup func(ctx context.Context, repo *repositories.ConfigRepository)
		did   string
		want  bool
	}{
		{
			name: "returns true for admin DID",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.AddAdminDID(ctx, "did:plc:admin")
			},
			did:  "did:plc:admin",
			want: true,
		},
		{
			name: "returns false for non-admin DID",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.AddAdminDID(ctx, "did:plc:admin")
			},
			did:  "did:plc:regular",
			want: false,
		},
		{
			name:  "returns false when no admins configured",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {},
			did:   "did:plc:anyone",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			ctx := context.Background()
			tt.setup(ctx, repo)
			got := repo.IsAdmin(ctx, tt.did)
			if got != tt.want {
				t.Errorf("IsAdmin(%q) = %v, want %v", tt.did, got, tt.want)
			}
		})
	}
}

func TestConfigRepository_HasAdmins(t *testing.T) {
	tests := []struct {
		name  string
		setup func(ctx context.Context, repo *repositories.ConfigRepository)
		want  bool
	}{
		{
			name:  "false when empty",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {},
			want:  false,
		},
		{
			name: "true after adding admin",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.AddAdminDID(ctx, "did:plc:someone")
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			ctx := context.Background()
			tt.setup(ctx, repo)
			got := repo.HasAdmins(ctx)
			if got != tt.want {
				t.Errorf("HasAdmins() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConfigRepository_URLDefaults(t *testing.T) {
	tests := []struct {
		name  string
		setup func(ctx context.Context, repo *repositories.ConfigRepository)
		call  func(ctx context.Context, repo *repositories.ConfigRepository) string
		want  string
	}{
		{
			name:  "GetRelayURL returns default when not set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetRelayURL(ctx)
			},
			want: repositories.DefaultRelayURL,
		},
		{
			name: "GetRelayURL returns custom value when set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.SetRelayURL(ctx, "https://custom-relay.example.com")
			},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetRelayURL(ctx)
			},
			want: "https://custom-relay.example.com",
		},
		{
			name: "GetPLCDirectoryURL returns override when set via SetPLCDirectoryOverride",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.SetPLCDirectoryURL(ctx, "https://db-value.example.com")
				repo.SetPLCDirectoryOverride("https://override.example.com")
			},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetPLCDirectoryURL(ctx)
			},
			want: "https://override.example.com",
		},
		{
			name: "GetPLCDirectoryURL returns DB value when override not set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.SetPLCDirectoryURL(ctx, "https://db-plc.example.com")
			},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetPLCDirectoryURL(ctx)
			},
			want: "https://db-plc.example.com",
		},
		{
			name:  "GetPLCDirectoryURL returns default when neither override nor DB set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetPLCDirectoryURL(ctx)
			},
			want: repositories.DefaultPLCDirectoryURL,
		},
		{
			name:  "GetJetstreamURL returns default when not set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetJetstreamURL(ctx)
			},
			want: repositories.DefaultJetstreamURL,
		},
		{
			name: "GetJetstreamURL returns custom value when set",
			setup: func(ctx context.Context, repo *repositories.ConfigRepository) {
				_ = repo.SetJetstreamURL(ctx, "wss://custom-jetstream.example.com/subscribe")
			},
			call: func(ctx context.Context, repo *repositories.ConfigRepository) string {
				return repo.GetJetstreamURL(ctx)
			},
			want: "wss://custom-jetstream.example.com/subscribe",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			ctx := context.Background()
			tt.setup(ctx, repo)
			got := tt.call(ctx, repo)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestConfigRepository_OAuthScopes(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, repo *repositories.ConfigRepository)
	}{
		{
			name: "GetOAuthSupportedScopes returns default when not set",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				got := repo.GetOAuthSupportedScopes(ctx)
				if got != repositories.DefaultOAuthSupportedScopes {
					t.Errorf("GetOAuthSupportedScopes() = %q, want %q", got, repositories.DefaultOAuthSupportedScopes)
				}
			},
		},
		{
			name: "GetOAuthSupportedScopesList parses space-separated list",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.SetOAuthSupportedScopes(ctx, "scope1 scope2 scope3"); err != nil {
					t.Fatalf("SetOAuthSupportedScopes() error = %v", err)
				}
				got := repo.GetOAuthSupportedScopesList(ctx)
				want := []string{"scope1", "scope2", "scope3"}
				if len(got) != len(want) {
					t.Fatalf("GetOAuthSupportedScopesList() len = %d, want %d", len(got), len(want))
				}
				for i, w := range want {
					if got[i] != w {
						t.Errorf("GetOAuthSupportedScopesList()[%d] = %q, want %q", i, got[i], w)
					}
				}
			},
		},
		{
			name: "SetOAuthSupportedScopes persists value",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.SetOAuthSupportedScopes(ctx, "custom:scope read write"); err != nil {
					t.Fatalf("SetOAuthSupportedScopes() error = %v", err)
				}
				got := repo.GetOAuthSupportedScopes(ctx)
				if got != "custom:scope read write" {
					t.Errorf("GetOAuthSupportedScopes() = %q, want %q", got, "custom:scope read write")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			tt.run(t, repo)
		})
	}
}

func TestConfigRepository_InitializeDefaults(t *testing.T) {
	tests := []struct {
		name string
		run  func(t *testing.T, repo *repositories.ConfigRepository)
	}{
		{
			name: "sets all defaults on first call",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()
				if err := repo.InitializeDefaults(ctx); err != nil {
					t.Fatalf("InitializeDefaults() error = %v", err)
				}

				checks := map[string]string{
					"relay_url":              repositories.DefaultRelayURL,
					"plc_directory_url":      repositories.DefaultPLCDirectoryURL,
					"jetstream_url":          repositories.DefaultJetstreamURL,
					"oauth_supported_scopes": repositories.DefaultOAuthSupportedScopes,
				}
				for key, want := range checks {
					got, err := repo.Get(ctx, key)
					if err != nil {
						t.Errorf("Get(%q) error = %v after InitializeDefaults", key, err)
						continue
					}
					if got != want {
						t.Errorf("Get(%q) = %q, want %q", key, got, want)
					}
				}
			},
		},
		{
			name: "does not overwrite existing values on second call",
			run: func(t *testing.T, repo *repositories.ConfigRepository) {
				ctx := context.Background()

				// Set a custom value before initializing defaults
				if err := repo.Set(ctx, "relay_url", "https://custom-relay.example.com"); err != nil {
					t.Fatalf("Set() error = %v", err)
				}

				if err := repo.InitializeDefaults(ctx); err != nil {
					t.Fatalf("InitializeDefaults() error = %v", err)
				}

				// The custom relay_url should be preserved
				got, err := repo.Get(ctx, "relay_url")
				if err != nil {
					t.Fatalf("Get(relay_url) error = %v", err)
				}
				if got != "https://custom-relay.example.com" {
					t.Errorf("Get(relay_url) = %q, want %q (should not overwrite)", got, "https://custom-relay.example.com")
				}

				// Other defaults should still be set
				got, err = repo.Get(ctx, "jetstream_url")
				if err != nil {
					t.Fatalf("Get(jetstream_url) error = %v", err)
				}
				if got != repositories.DefaultJetstreamURL {
					t.Errorf("Get(jetstream_url) = %q, want %q", got, repositories.DefaultJetstreamURL)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := setupConfigTest(t)
			tt.run(t, repo)
		})
	}
}
