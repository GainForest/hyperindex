package repositories

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/GainForest/hyperindex/internal/database"
)

// Default values for external services
const (
	DefaultRelayURL             = "https://relay1.us-west.bsky.network"
	DefaultPLCDirectoryURL      = "https://plc.directory"
	DefaultJetstreamURL         = "wss://jetstream2.us-west.bsky.network/subscribe"
	DefaultOAuthSupportedScopes = "atproto transition:generic"
)

// ConfigKeyLabelerSubscribeURLs stores the comma-separated external labeler
// websocket URLs managed through the admin UI.
const ConfigKeyLabelerSubscribeURLs = "labeler_subscribe_urls"

// ConfigRepository handles key-value configuration persistence.
type ConfigRepository struct {
	db                   database.Executor
	plcDirectoryOverride string // Set from app config to avoid os.Getenv in repo layer
}

// NewConfigRepository creates a new config repository.
func NewConfigRepository(db database.Executor) *ConfigRepository {
	return &ConfigRepository{db: db}
}

// SetPLCDirectoryOverride sets an override for the PLC directory URL,
// allowing the caller to centralize env var reading in the config package.
func (r *ConfigRepository) SetPLCDirectoryOverride(url string) {
	r.plcDirectoryOverride = url
}

// Get retrieves a config value by key.
func (r *ConfigRepository) Get(ctx context.Context, key string) (string, error) {
	sqlStr := fmt.Sprintf("SELECT value FROM config WHERE key = %s", r.db.Placeholder(1))

	var value string
	err := r.db.QueryRow(ctx, sqlStr, []database.Value{database.Text(key)}, &value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("config key not found: %s", key)
		}
		return "", err
	}
	return value, nil
}

// Set inserts or updates a config value.
func (r *ConfigRepository) Set(ctx context.Context, key, value string) error {
	p1 := r.db.Placeholder(1)
	p2 := r.db.Placeholder(2)
	now := r.db.Now()

	var sqlStr string
	switch r.db.Dialect() {
	case database.PostgreSQL:
		sqlStr = fmt.Sprintf(`INSERT INTO config (key, value, updated_at)
			VALUES (%s, %s, %s)
			ON CONFLICT(key) DO UPDATE SET
				value = EXCLUDED.value,
				updated_at = %s`, p1, p2, now, now)
	default:
		sqlStr = fmt.Sprintf(`INSERT INTO config (key, value, updated_at)
			VALUES (%s, %s, %s)
			ON CONFLICT(key) DO UPDATE SET
				value = excluded.value,
				updated_at = %s`, p1, p2, now, now)
	}

	_, err := r.db.Exec(ctx, sqlStr, []database.Value{
		database.Text(key),
		database.Text(value),
	})
	return err
}

// Delete removes a config value by key.
func (r *ConfigRepository) Delete(ctx context.Context, key string) error {
	sqlStr := fmt.Sprintf("DELETE FROM config WHERE key = %s", r.db.Placeholder(1))
	_, err := r.db.Exec(ctx, sqlStr, []database.Value{database.Text(key)})
	return err
}

// DeleteDomainAuthority removes the domain_authority config entry.
func (r *ConfigRepository) DeleteDomainAuthority(ctx context.Context) error {
	return r.Delete(ctx, "domain_authority")
}

// RemoveAdminError represents errors when removing admin DIDs.
type RemoveAdminError int

const (
	RemoveAdminLastAdmin RemoveAdminError = iota
	RemoveAdminNotFound
	RemoveAdminDatabaseError
)

func (e RemoveAdminError) Error() string {
	switch e {
	case RemoveAdminLastAdmin:
		return "cannot remove the last admin"
	case RemoveAdminNotFound:
		return "admin DID not found"
	case RemoveAdminDatabaseError:
		return "database error"
	default:
		return "unknown error"
	}
}

// GetAdminDIDs retrieves the list of admin DIDs from config.
func (r *ConfigRepository) GetAdminDIDs(ctx context.Context) []string {
	value, err := r.Get(ctx, "admin_dids")
	if err != nil {
		return []string{}
	}

	var result []string
	for _, did := range strings.Split(value, ",") {
		trimmed := strings.TrimSpace(did)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// AddAdminDID adds an admin DID to the list.
func (r *ConfigRepository) AddAdminDID(ctx context.Context, did string) error {
	current := r.GetAdminDIDs(ctx)
	for _, existing := range current {
		if existing == did {
			return nil // Already exists
		}
	}

	current = append(current, did)
	value := strings.Join(current, ",")
	return r.Set(ctx, "admin_dids", value)
}

// RemoveAdminDID removes an admin DID from the list.
// Returns the new list of admins on success.
// Returns error if trying to remove the last admin or if DID not found.
func (r *ConfigRepository) RemoveAdminDID(ctx context.Context, did string) ([]string, error) {
	current := r.GetAdminDIDs(ctx)

	// Check if DID exists
	found := false
	var newList []string
	for _, d := range current {
		if d == did {
			found = true
		} else {
			newList = append(newList, d)
		}
	}

	if !found {
		return nil, RemoveAdminNotFound
	}

	if len(newList) == 0 {
		return nil, RemoveAdminLastAdmin
	}

	value := strings.Join(newList, ",")
	if err := r.Set(ctx, "admin_dids", value); err != nil {
		return nil, RemoveAdminDatabaseError
	}

	return newList, nil
}

// SetAdminDIDs sets the full admin DIDs list (replaces existing).
func (r *ConfigRepository) SetAdminDIDs(ctx context.Context, dids []string) error {
	value := strings.Join(dids, ",")
	return r.Set(ctx, "admin_dids", value)
}

// IsAdmin checks if a DID is an admin.
func (r *ConfigRepository) IsAdmin(ctx context.Context, did string) bool {
	admins := r.GetAdminDIDs(ctx)
	for _, admin := range admins {
		if admin == did {
			return true
		}
	}
	return false
}

// HasAdmins checks if any admins are configured.
func (r *ConfigRepository) HasAdmins(ctx context.Context) bool {
	return len(r.GetAdminDIDs(ctx)) > 0
}

// GetRelayURL retrieves the relay URL from config, with default fallback.
func (r *ConfigRepository) GetRelayURL(ctx context.Context) string {
	if url, err := r.Get(ctx, "relay_url"); err == nil {
		return url
	}
	return DefaultRelayURL
}

// GetPLCDirectoryURL retrieves the PLC directory URL with precedence:
// config override -> database -> default
func (r *ConfigRepository) GetPLCDirectoryURL(ctx context.Context) string {
	if r.plcDirectoryOverride != "" {
		return r.plcDirectoryOverride
	}
	if url, err := r.Get(ctx, "plc_directory_url"); err == nil {
		return url
	}
	return DefaultPLCDirectoryURL
}

// GetJetstreamURL retrieves the Jetstream URL from config, with default fallback.
func (r *ConfigRepository) GetJetstreamURL(ctx context.Context) string {
	if url, err := r.Get(ctx, "jetstream_url"); err == nil {
		return url
	}
	return DefaultJetstreamURL
}

// GetOAuthSupportedScopes retrieves OAuth supported scopes from config, with default fallback.
func (r *ConfigRepository) GetOAuthSupportedScopes(ctx context.Context) string {
	if scopes, err := r.Get(ctx, "oauth_supported_scopes"); err == nil {
		return scopes
	}
	return DefaultOAuthSupportedScopes
}

// GetOAuthSupportedScopesList parses OAuth supported scopes into a slice.
func (r *ConfigRepository) GetOAuthSupportedScopesList(ctx context.Context) []string {
	scopesStr := r.GetOAuthSupportedScopes(ctx)
	var result []string
	for _, scope := range strings.Split(scopesStr, " ") {
		trimmed := strings.TrimSpace(scope)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// SetRelayURL sets the relay URL.
func (r *ConfigRepository) SetRelayURL(ctx context.Context, url string) error {
	return r.Set(ctx, "relay_url", url)
}

// SetPLCDirectoryURL sets the PLC directory URL.
func (r *ConfigRepository) SetPLCDirectoryURL(ctx context.Context, url string) error {
	return r.Set(ctx, "plc_directory_url", url)
}

// SetJetstreamURL sets the Jetstream URL.
func (r *ConfigRepository) SetJetstreamURL(ctx context.Context, url string) error {
	return r.Set(ctx, "jetstream_url", url)
}

// SetOAuthSupportedScopes sets the OAuth supported scopes (space-separated string).
func (r *ConfigRepository) SetOAuthSupportedScopes(ctx context.Context, scopes string) error {
	return r.Set(ctx, "oauth_supported_scopes", scopes)
}

// InitializeDefaults initializes config with defaults if not already set.
func (r *ConfigRepository) InitializeDefaults(ctx context.Context) error {
	defaults := map[string]string{
		"relay_url":              DefaultRelayURL,
		"plc_directory_url":      DefaultPLCDirectoryURL,
		"jetstream_url":          DefaultJetstreamURL,
		"oauth_supported_scopes": DefaultOAuthSupportedScopes,
	}

	for key, defaultValue := range defaults {
		if _, err := r.Get(ctx, key); err != nil {
			if err := r.Set(ctx, key, defaultValue); err != nil {
				return err
			}
		}
	}

	return nil
}
