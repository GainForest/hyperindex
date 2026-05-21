package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	publicgraphql "github.com/GainForest/hyperindex/internal/graphql"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/oauth"
)

// Repositories holds the database repositories needed by the admin API.
type Repositories struct {
	Records          *repositories.RecordsRepository
	Actors           *repositories.ActorsRepository
	Lexicons         *repositories.LexiconsRepository
	Config           *repositories.ConfigRepository
	OAuthClients     *repositories.OAuthClientsRepository
	Activity         *repositories.JetstreamActivityRepository
	Labels           *repositories.LabelsRepository
	LabelDefinitions *repositories.LabelDefinitionsRepository
	LabelPreferences *repositories.LabelPreferencesRepository
	Reports          *repositories.ReportsRepository
}

// BackfillCallback is called when single-actor backfill is triggered.
type BackfillCallback func(ctx context.Context, did string) error

// FullBackfillCallback is called when full network backfill is triggered.
type FullBackfillCallback func(ctx context.Context) error

// LexiconChangeCallback is called when lexicons are added or removed.
type LexiconChangeCallback func(collections []string) error

type lexiconResolveFunc func(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error)

// Resolver provides methods for resolving admin GraphQL queries and mutations.
type Resolver struct {
	repos                 *Repositories
	adminDIDs             []string
	backfillActive        atomic.Bool
	domainDID             string // The DID of this labeler instance
	backfillCallback      BackfillCallback
	fullBackfillCallback  FullBackfillCallback
	lexiconChangeCallback LexiconChangeCallback
	resolveLexicon        lexiconResolveFunc
}

// NewResolver creates a new admin resolver.
func NewResolver(repos *Repositories, domainDID string, adminDIDs []string) *Resolver {
	return &Resolver{
		repos:          repos,
		adminDIDs:      adminDIDs,
		domainDID:      domainDID,
		resolveLexicon: lexicon.NewResolver().ResolveLexicon,
	}
}

// SetBackfillCallback sets the callback for single-actor backfill operations.
func (r *Resolver) SetBackfillCallback(cb BackfillCallback) {
	r.backfillCallback = cb
}

// SetFullBackfillCallback sets the callback for full network backfill operations.
func (r *Resolver) SetFullBackfillCallback(cb FullBackfillCallback) {
	r.fullBackfillCallback = cb
}

// SetLexiconChangeCallback sets the callback for lexicon changes.
func (r *Resolver) SetLexiconChangeCallback(cb LexiconChangeCallback) {
	r.lexiconChangeCallback = cb
}

// notifyLexiconChange calls the lexicon change callback with current collections.
func (r *Resolver) notifyLexiconChange(ctx context.Context) {
	if r.lexiconChangeCallback == nil {
		return
	}

	lexicons, err := r.repos.Lexicons.GetAll(ctx)
	if err != nil {
		return
	}

	collections := make([]string, len(lexicons))
	for i, lex := range lexicons {
		collections[i] = lex.ID
	}

	if err := r.lexiconChangeCallback(collections); err != nil {
		// Log but don't fail the operation
		slog.Warn("Failed to notify lexicon change", "error", err)
	}
}

// =============================================================================
// Query Resolvers
// =============================================================================

// Statistics returns system statistics.
func (r *Resolver) Statistics(ctx context.Context) (map[string]interface{}, error) {
	recordCount, err := r.repos.Records.GetCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get record count: %w", err)
	}

	actorCount, err := r.repos.Actors.GetCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get actor count: %w", err)
	}

	lexiconCount, err := r.repos.Lexicons.GetCount(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get lexicon count: %w", err)
	}

	return map[string]interface{}{
		"recordCount":  recordCount,
		"actorCount":   actorCount,
		"lexiconCount": lexiconCount,
	}, nil
}

// CurrentSession returns the current user's session info.
func (r *Resolver) CurrentSession(ctx context.Context, userDID, handle string, adminDIDs []string) map[string]interface{} {
	isAdmin := isAdminDID(userDID, adminDIDs)

	return map[string]interface{}{
		"did":     userDID,
		"handle":  handle,
		"isAdmin": isAdmin,
	}
}

// Settings returns system settings.
func (r *Resolver) Settings(ctx context.Context) (map[string]interface{}, error) {
	domainAuthority, _ := r.repos.Config.Get(ctx, "domain_authority")
	relayURL, _ := r.repos.Config.Get(ctx, "relay_url")
	plcDirectoryURL, _ := r.repos.Config.Get(ctx, "plc_directory_url")
	jetstreamURL, _ := r.repos.Config.Get(ctx, "jetstream_url")
	oauthScopes, _ := r.repos.Config.Get(ctx, "oauth_supported_scopes")

	return map[string]interface{}{
		"id":                   "settings",
		"domainAuthority":      domainAuthority,
		"adminDids":            r.adminDIDs,
		"relayUrl":             relayURL,
		"plcDirectoryUrl":      plcDirectoryURL,
		"jetstreamUrl":         jetstreamURL,
		"oauthSupportedScopes": oauthScopes,
	}, nil
}

// IsBackfilling returns whether a backfill is currently active.
func (r *Resolver) IsBackfilling() bool {
	return r.backfillActive.Load()
}

// SetBackfillActive sets the backfill status.
func (r *Resolver) SetBackfillActive(active bool) {
	r.backfillActive.Store(active)
}

// Lexicons returns all lexicon definitions.
func (r *Resolver) Lexicons(ctx context.Context) ([]map[string]interface{}, error) {
	lexicons, err := r.repos.Lexicons.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get lexicons: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(lexicons))
	for _, lex := range lexicons {
		result = append(result, map[string]interface{}{
			"id":        lex.ID,
			"json":      lex.JSON,
			"createdAt": lex.CreatedAt.Format(time.RFC3339),
		})
	}

	return result, nil
}

// OAuthClients returns all OAuth client registrations.
func (r *Resolver) OAuthClients(ctx context.Context) ([]map[string]interface{}, error) {
	clients, err := r.repos.OAuthClients.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth clients: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(clients))
	for _, client := range clients {
		result = append(result, map[string]interface{}{
			"clientId":     client.ClientID,
			"clientSecret": client.ClientSecret,
			"clientName":   client.ClientName,
			"clientType":   string(client.ClientType),
			"redirectUris": client.RedirectURIs,
			"scope":        client.Scope,
			"createdAt":    client.CreatedAt,
		})
	}

	return result, nil
}

// Upload size limits for lexicon ZIP files.
const (
	maxLexiconUploadBytes = 10 * 1024 * 1024 // 10MB max ZIP size
	maxLexiconFileCount   = 500              // Max files in ZIP
	maxLexiconFileSize    = 1 * 1024 * 1024  // 1MB max per file
)

type validatedLexiconDocument struct {
	id   string
	json string
}

// UploadLexicons extracts lexicons from a base64-encoded ZIP file.
func (r *Resolver) UploadLexicons(ctx context.Context, zipBase64 string) (int, error) {
	// Validate base64 input size before decoding (base64 encodes 3 bytes as 4 chars)
	maxBase64Len := maxLexiconUploadBytes * 4 / 3
	if len(zipBase64) > maxBase64Len {
		return 0, fmt.Errorf("upload too large: estimated %d bytes exceeds %d byte limit",
			len(zipBase64)*3/4, maxLexiconUploadBytes)
	}

	// Decode base64
	zipData, err := base64.StdEncoding.DecodeString(zipBase64)
	if err != nil {
		return 0, fmt.Errorf("invalid base64 data: %w", err)
	}

	// Open ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return 0, fmt.Errorf("invalid ZIP file: %w", err)
	}

	// Check file count to prevent zip bombs
	if len(zipReader.File) > maxLexiconFileCount {
		return 0, fmt.Errorf("too many files in ZIP: %d exceeds limit of %d",
			len(zipReader.File), maxLexiconFileCount)
	}

	validated := make([]validatedLexiconDocument, 0)
	for _, file := range zipReader.File {
		// Skip directories and non-JSON files
		if file.FileInfo().IsDir() || !strings.HasSuffix(file.Name, ".json") {
			continue
		}

		// Check individual uncompressed file size
		if file.UncompressedSize64 > maxLexiconFileSize {
			return 0, fmt.Errorf("file %s too large: %d bytes exceeds %d byte limit; Hyperindex did not store any lexicons from this ZIP",
				file.Name, file.UncompressedSize64, maxLexiconFileSize)
		}

		// Open and read file with size limit
		rc, err := file.Open()
		if err != nil {
			return 0, fmt.Errorf("failed to open lexicon file %s from ZIP for validation: %w; Hyperindex did not store any lexicons from this ZIP", file.Name, err)
		}

		data, err := io.ReadAll(io.LimitReader(rc, maxLexiconFileSize+1))
		_ = rc.Close()
		if err != nil {
			return 0, fmt.Errorf("failed to read lexicon file %s from ZIP for validation: %w; Hyperindex did not store any lexicons from this ZIP", file.Name, err)
		}
		if len(data) > maxLexiconFileSize {
			return 0, fmt.Errorf("file %s exceeds %d byte limit after decompression; Hyperindex did not store any lexicons from this ZIP",
				file.Name, maxLexiconFileSize)
		}

		parsed, err := publicgraphql.ParseAndValidateLexiconDocument(fmt.Sprintf("uploaded lexicon %q", file.Name), string(data), "")
		if err != nil {
			return 0, fmt.Errorf("%w. Hyperindex did not store any lexicons from this ZIP; fix or remove %s and upload again", err, file.Name)
		}

		validated = append(validated, validatedLexiconDocument{id: parsed.ID, json: string(data)})
	}

	count := 0
	for _, doc := range validated {
		if err := r.repos.Lexicons.Upsert(ctx, doc.id, doc.json); err != nil {
			return count, fmt.Errorf("failed to save validated lexicon %s: %w", doc.id, err)
		}
		count++
	}

	// Notify Jetstream consumer of collection changes
	if count > 0 {
		r.notifyLexiconChange(ctx)
	}

	return count, nil
}

// TriggerBackfill starts a full backfill process.
// Uses atomic CompareAndSwap to prevent concurrent backfill launches (race-safe).
func (r *Resolver) TriggerBackfill(ctx context.Context) (bool, error) {
	if r.fullBackfillCallback == nil {
		return false, fmt.Errorf("full backfill not configured")
	}

	// Atomically check-and-set to prevent concurrent backfill launches
	if !r.backfillActive.CompareAndSwap(false, true) {
		return false, fmt.Errorf("backfill already in progress")
	}

	// Run backfill in background goroutine
	go func() {
		defer r.backfillActive.Store(false)

		// Use background context since HTTP request context will be cancelled
		if err := r.fullBackfillCallback(context.Background()); err != nil {
			slog.Error("[backfill] Full backfill failed in background", "error", err)
			return
		}
	}()

	return true, nil
}

// BackfillActor queues a single actor for backfill.
func (r *Resolver) BackfillActor(ctx context.Context, did string) (bool, error) {
	// Validate DID format
	if !strings.HasPrefix(did, "did:") {
		return false, fmt.Errorf("invalid DID format")
	}

	// Ensure actor exists (creates if not)
	if err := r.repos.Actors.Upsert(ctx, did, ""); err != nil {
		return false, fmt.Errorf("failed to register actor: %w", err)
	}

	// Trigger backfill callback if registered
	if r.backfillCallback != nil {
		if err := r.backfillCallback(ctx, did); err != nil {
			return false, fmt.Errorf("failed to trigger backfill: %w", err)
		}
	}

	return true, nil
}

// CreateOAuthClient creates a new OAuth client.
func (r *Resolver) CreateOAuthClient(ctx context.Context, clientName, clientType string, redirectURIs []string) (map[string]interface{}, error) {
	// Generate client ID
	clientIDBytes := make([]byte, 16)
	if _, err := rand.Read(clientIDBytes); err != nil {
		return nil, fmt.Errorf("failed to generate client ID: %w", err)
	}
	clientID := hex.EncodeToString(clientIDBytes)

	// Generate client secret for confidential clients
	var clientSecret *string
	ct := oauth.ClientType(clientType)
	if ct == oauth.ClientConfidential {
		secretBytes := make([]byte, 32)
		if _, err := rand.Read(secretBytes); err != nil {
			return nil, fmt.Errorf("failed to generate client secret: %w", err)
		}
		secret := hex.EncodeToString(secretBytes)
		clientSecret = &secret
	}

	now := time.Now().Unix()
	client := &oauth.Client{
		ClientID:                clientID,
		ClientSecret:            clientSecret,
		ClientName:              clientName,
		RedirectURIs:            redirectURIs,
		GrantTypes:              []oauth.GrantType{oauth.GrantAuthorizationCode, oauth.GrantRefreshToken},
		ResponseTypes:           []oauth.ResponseType{oauth.ResponseCode},
		TokenEndpointAuthMethod: oauth.AuthClientSecret,
		ClientType:              ct,
		CreatedAt:               now,
		UpdatedAt:               now,
		Metadata:                "{}",
		AccessTokenExpiration:   3600,       // 1 hour
		RefreshTokenExpiration:  86400 * 30, // 30 days
		RequireRedirectExact:    true,
	}

	if ct == oauth.ClientPublic {
		client.TokenEndpointAuthMethod = oauth.AuthNone
	}

	if err := r.repos.OAuthClients.Insert(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to create OAuth client: %w", err)
	}

	result := map[string]interface{}{
		"clientId":     client.ClientID,
		"clientName":   client.ClientName,
		"clientType":   string(client.ClientType),
		"redirectUris": client.RedirectURIs,
		"createdAt":    time.Unix(client.CreatedAt, 0).Format(time.RFC3339),
	}
	if client.ClientSecret != nil {
		result["clientSecret"] = *client.ClientSecret
	}

	return result, nil
}

// UpdateOAuthClient updates an existing OAuth client.
func (r *Resolver) UpdateOAuthClient(ctx context.Context, clientID, clientName string, redirectURIs []string) (map[string]interface{}, error) {
	// Get existing client
	client, err := r.repos.OAuthClients.Get(ctx, clientID)
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth client: %w", err)
	}
	if client == nil {
		return nil, fmt.Errorf("OAuth client not found")
	}

	// Update fields
	client.ClientName = clientName
	client.RedirectURIs = redirectURIs
	client.UpdatedAt = time.Now().Unix()

	if err := r.repos.OAuthClients.Update(ctx, client); err != nil {
		return nil, fmt.Errorf("failed to update OAuth client: %w", err)
	}

	result := map[string]interface{}{
		"clientId":     client.ClientID,
		"clientName":   client.ClientName,
		"clientType":   string(client.ClientType),
		"redirectUris": client.RedirectURIs,
		"createdAt":    time.Unix(client.CreatedAt, 0).Format(time.RFC3339),
	}
	if client.ClientSecret != nil {
		result["clientSecret"] = *client.ClientSecret
	}

	return result, nil
}

// DeleteOAuthClient deletes an OAuth client.
func (r *Resolver) DeleteOAuthClient(ctx context.Context, clientID string) (bool, error) {
	// Don't allow deleting the admin client
	if clientID == "admin" {
		return false, fmt.Errorf("cannot delete the admin client")
	}

	if err := r.repos.OAuthClients.Delete(ctx, clientID); err != nil {
		return false, fmt.Errorf("failed to delete OAuth client: %w", err)
	}

	return true, nil
}

// PurgeActor removes all indexed data for a DID.
func (r *Resolver) PurgeActor(ctx context.Context, did, confirm string) (bool, error) {
	if confirm != "PURGE" {
		return false, fmt.Errorf("confirmation required: pass 'PURGE' to confirm")
	}

	normalizedDID := strings.TrimSpace(did)

	if !strings.HasPrefix(normalizedDID, "did:") {
		return false, fmt.Errorf("invalid DID format")
	}

	if err := r.repos.Records.PurgeActorData(ctx, normalizedDID); err != nil {
		return false, fmt.Errorf("failed to purge actor data for DID: %w", err)
	}

	return true, nil
}

// PurgeActorPreview returns the impact of purging a DID.
func (r *Resolver) PurgeActorPreview(ctx context.Context, did string) (map[string]interface{}, error) {
	normalizedDID := strings.TrimSpace(did)
	isValidDID := strings.HasPrefix(normalizedDID, "did:")

	if !isValidDID {
		return map[string]interface{}{
			"did":         normalizedDID,
			"isValidDid":  false,
			"actorExists": false,
			"recordCount": 0,
		}, nil
	}

	actorExists, err := r.repos.Actors.Exists(ctx, normalizedDID)
	if err != nil {
		return nil, fmt.Errorf("failed to check actor existence: %w", err)
	}

	recordCount, err := r.repos.Records.GetCountByDID(ctx, normalizedDID)
	if err != nil {
		return nil, fmt.Errorf("failed to count records by DID: %w", err)
	}

	return map[string]interface{}{
		"did":         normalizedDID,
		"isValidDid":  true,
		"actorExists": actorExists,
		"recordCount": recordCount,
	}, nil
}

// RegisterLexicon resolves an NSID via DNS and registers the lexicon schema.
func (r *Resolver) RegisterLexicon(ctx context.Context, nsid string) (map[string]interface{}, error) {
	requestedNSID, err := normalizeRequestedLexiconNSID(nsid)
	if err != nil {
		return nil, err
	}

	// Check if lexicon already exists
	exists, err := r.repos.Lexicons.Exists(ctx, requestedNSID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing lexicon: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("lexicon %s is already registered", requestedNSID)
	}

	// Resolve lexicon via DNS and PDS
	resolved, err := r.resolveLexiconDocument(ctx, requestedNSID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve lexicon: %w", err)
	}

	schemaJSON := string(resolved.Schema)
	parsed, err := publicgraphql.ParseAndValidateLexiconDocument(fmt.Sprintf("registered lexicon %q", requestedNSID), schemaJSON, requestedNSID)
	if err != nil {
		return nil, fmt.Errorf("%w. Hyperindex did not store lexicon %q; fix the published lexicon document and register it again", err, requestedNSID)
	}

	// Store the validated lexicon schema
	if err := r.repos.Lexicons.Upsert(ctx, parsed.ID, schemaJSON); err != nil {
		return nil, fmt.Errorf("failed to save validated lexicon %s: %w", parsed.ID, err)
	}

	// Notify Jetstream consumer of collection changes
	r.notifyLexiconChange(ctx)

	// Parse schema to extract description
	var schema struct {
		ID          string `json:"id"`
		Description string `json:"description"`
		Defs        map[string]struct {
			Description string `json:"description"`
		} `json:"defs"`
	}
	_ = json.Unmarshal(resolved.Schema, &schema)

	description := schema.Description
	if description == "" && schema.Defs != nil {
		if main, ok := schema.Defs["main"]; ok {
			description = main.Description
		}
	}

	return map[string]interface{}{
		"id":          parsed.ID,
		"json":        schemaJSON,
		"createdAt":   time.Now().Format(time.RFC3339),
		"did":         resolved.DID,
		"description": description,
	}, nil
}

func normalizeRequestedLexiconNSID(nsid string) (string, error) {
	normalized := strings.TrimSpace(nsid)
	parts := strings.Split(normalized, ".")
	if normalized == "" || len(parts) < 3 {
		return "", fmt.Errorf("invalid NSID format: must be a dotted identifier with at least 3 segments and no empty segments (e.g., app.bsky.feed.post)")
	}
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return "", fmt.Errorf("invalid NSID format: must be a dotted identifier with at least 3 segments and no empty segments (e.g., app.bsky.feed.post)")
		}
	}
	return normalized, nil
}

func (r *Resolver) resolveLexiconDocument(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error) {
	if r.resolveLexicon != nil {
		return r.resolveLexicon(ctx, nsid)
	}
	return lexicon.NewResolver().ResolveLexicon(ctx, nsid)
}

// DeleteLexicon removes a registered lexicon by NSID.
func (r *Resolver) DeleteLexicon(ctx context.Context, nsid string) (bool, error) {
	exists, err := r.repos.Lexicons.Exists(ctx, nsid)
	if err != nil {
		return false, fmt.Errorf("failed to check lexicon: %w", err)
	}
	if !exists {
		return false, fmt.Errorf("lexicon %s not found", nsid)
	}

	if err := r.repos.Lexicons.Delete(ctx, nsid); err != nil {
		return false, fmt.Errorf("failed to delete lexicon: %w", err)
	}

	// Notify Jetstream consumer of collection changes
	r.notifyLexiconChange(ctx)

	return true, nil
}

// ActivityBuckets returns aggregated activity data for the specified time range.
func (r *Resolver) ActivityBuckets(ctx context.Context, timeRange string) ([]map[string]interface{}, error) {
	buckets, err := r.repos.Activity.GetActivityBuckets(ctx, timeRange)
	if err != nil {
		return nil, fmt.Errorf("failed to get activity buckets: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(buckets))
	for _, bucket := range buckets {
		result = append(result, map[string]interface{}{
			"timestamp": bucket.Timestamp.Format(time.RFC3339),
			"total":     bucket.Total,
			"creates":   bucket.Creates,
			"updates":   bucket.Updates,
			"deletes":   bucket.Deletes,
		})
	}

	return result, nil
}

// RecentActivity returns recent activity entries.
func (r *Resolver) RecentActivity(ctx context.Context, hours int) ([]map[string]interface{}, error) {
	entries, err := r.repos.Activity.GetRecentActivity(ctx, hours)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent activity: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		item := map[string]interface{}{
			"id":         entry.ID,
			"timestamp":  entry.Timestamp.Format(time.RFC3339),
			"operation":  entry.Operation,
			"collection": entry.Collection,
			"did":        entry.DID,
			"status":     entry.Status,
			"eventJson":  entry.EventJSON,
		}
		if entry.RKey != nil {
			item["rkey"] = *entry.RKey
		}
		if entry.ErrorMessage != nil {
			item["errorMessage"] = *entry.ErrorMessage
		}
		result = append(result, item)
	}

	return result, nil
}

// LabelDefinitions returns all label definitions.
func (r *Resolver) LabelDefinitions(ctx context.Context) ([]map[string]interface{}, error) {
	defs, err := r.repos.LabelDefinitions.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get label definitions: %w", err)
	}

	result := make([]map[string]interface{}, 0, len(defs))
	for _, def := range defs {
		result = append(result, map[string]interface{}{
			"val":               def.Val,
			"description":       def.Description,
			"severity":          string(def.Severity),
			"defaultVisibility": string(def.DefaultVisibility),
			"createdAt":         def.CreatedAt.Format(time.RFC3339),
		})
	}

	return result, nil
}

// ViewerLabelPreferences returns the current user's label preferences.
func (r *Resolver) ViewerLabelPreferences(ctx context.Context, userDID string) ([]map[string]interface{}, error) {
	// Get all label definitions
	defs, err := r.repos.LabelDefinitions.GetNonSystem(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get label definitions: %w", err)
	}

	// Get user preferences
	prefs, err := r.repos.LabelPreferences.GetByDID(ctx, userDID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user preferences: %w", err)
	}

	// Build preference map for quick lookup
	prefMap := make(map[string]repositories.LabelVisibility)
	for _, pref := range prefs {
		prefMap[pref.LabelVal] = pref.Visibility
	}

	// Build result with effective visibility
	result := make([]map[string]interface{}, 0, len(defs))
	for _, def := range defs {
		visibility := def.DefaultVisibility
		if userVis, ok := prefMap[def.Val]; ok {
			visibility = userVis
		}

		result = append(result, map[string]interface{}{
			"val":               def.Val,
			"description":       def.Description,
			"severity":          string(def.Severity),
			"defaultVisibility": string(def.DefaultVisibility),
			"visibility":        string(visibility),
		})
	}

	return result, nil
}

// Labels returns labels with optional filters and pagination.
func (r *Resolver) Labels(ctx context.Context, uriFilter, valFilter *string, first int, after *string) (map[string]interface{}, error) {
	if first == 0 {
		first = 20
	}

	// Decode cursor to get afterID
	var afterID *int64
	if after != nil && *after != "" {
		decoded, err := base64.URLEncoding.DecodeString(*after)
		if err == nil {
			if id, err := strconv.ParseInt(string(decoded), 10, 64); err == nil {
				afterID = &id
			}
		}
	}

	paginated, err := r.repos.Labels.GetPaginated(ctx, uriFilter, valFilter, first, afterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get labels: %w", err)
	}

	edges := make([]map[string]interface{}, 0, len(paginated.Labels))
	var startCursor, endCursor string

	for _, label := range paginated.Labels {
		cursor := base64.URLEncoding.EncodeToString([]byte(strconv.FormatInt(label.ID, 10)))
		if startCursor == "" {
			startCursor = cursor
		}
		endCursor = cursor

		node := map[string]interface{}{
			"id":  label.ID,
			"src": label.Src,
			"uri": label.URI,
			"val": label.Val,
			"neg": label.Neg,
			"cts": label.Cts.Format(time.RFC3339),
		}
		if label.CID != nil {
			node["cid"] = *label.CID
		}
		if label.Exp != nil {
			node["exp"] = label.Exp.Format(time.RFC3339)
		}

		edges = append(edges, map[string]interface{}{
			"cursor": cursor,
			"node":   node,
		})
	}

	return map[string]interface{}{
		"edges": edges,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     paginated.HasNextPage,
			"hasPreviousPage": after != nil && *after != "",
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
		"totalCount": paginated.TotalCount,
	}, nil
}

// Reports returns reports with optional status filter and pagination.
func (r *Resolver) Reports(ctx context.Context, statusFilter *string, first int, after *string) (map[string]interface{}, error) {
	if first == 0 {
		first = 20
	}

	// Convert status filter
	var status *repositories.ReportStatus
	if statusFilter != nil {
		s := repositories.ReportStatus(*statusFilter)
		status = &s
	}

	// Decode cursor to get afterID
	var afterID *int64
	if after != nil && *after != "" {
		decoded, err := base64.URLEncoding.DecodeString(*after)
		if err == nil {
			if id, err := strconv.ParseInt(string(decoded), 10, 64); err == nil {
				afterID = &id
			}
		}
	}

	paginated, err := r.repos.Reports.GetPaginated(ctx, status, first, afterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get reports: %w", err)
	}

	edges := make([]map[string]interface{}, 0, len(paginated.Reports))
	var startCursor, endCursor string

	for _, report := range paginated.Reports {
		cursor := base64.URLEncoding.EncodeToString([]byte(strconv.FormatInt(report.ID, 10)))
		if startCursor == "" {
			startCursor = cursor
		}
		endCursor = cursor

		node := map[string]interface{}{
			"id":          report.ID,
			"reporterDid": report.ReporterDID,
			"subjectUri":  report.SubjectURI,
			"reasonType":  string(report.ReasonType),
			"status":      string(report.Status),
			"createdAt":   report.CreatedAt.Format(time.RFC3339),
		}
		if report.Reason != nil {
			node["reason"] = *report.Reason
		}
		if report.ResolvedBy != nil {
			node["resolvedBy"] = *report.ResolvedBy
		}
		if report.ResolvedAt != nil {
			node["resolvedAt"] = report.ResolvedAt.Format(time.RFC3339)
		}

		edges = append(edges, map[string]interface{}{
			"cursor": cursor,
			"node":   node,
		})
	}

	return map[string]interface{}{
		"edges": edges,
		"pageInfo": map[string]interface{}{
			"hasNextPage":     paginated.HasNextPage,
			"hasPreviousPage": after != nil && *after != "",
			"startCursor":     startCursor,
			"endCursor":       endCursor,
		},
		"totalCount": paginated.TotalCount,
	}, nil
}

// =============================================================================
// Mutation Resolvers
// =============================================================================

// UpdateSettings updates system settings.
func (r *Resolver) UpdateSettings(ctx context.Context, domainAuthority, relayURL, plcDirectoryURL, jetstreamURL, oauthScopes *string) (map[string]interface{}, error) {
	if domainAuthority != nil {
		if err := r.repos.Config.Set(ctx, "domain_authority", *domainAuthority); err != nil {
			return nil, fmt.Errorf("failed to update domain_authority: %w", err)
		}
	}

	if relayURL != nil {
		if err := r.repos.Config.Set(ctx, "relay_url", *relayURL); err != nil {
			return nil, fmt.Errorf("failed to update relay_url: %w", err)
		}
	}

	if plcDirectoryURL != nil {
		if err := r.repos.Config.Set(ctx, "plc_directory_url", *plcDirectoryURL); err != nil {
			return nil, fmt.Errorf("failed to update plc_directory_url: %w", err)
		}
	}

	if jetstreamURL != nil {
		if err := r.repos.Config.Set(ctx, "jetstream_url", *jetstreamURL); err != nil {
			return nil, fmt.Errorf("failed to update jetstream_url: %w", err)
		}
	}

	if oauthScopes != nil {
		if err := r.repos.Config.Set(ctx, "oauth_supported_scopes", *oauthScopes); err != nil {
			return nil, fmt.Errorf("failed to update oauth_supported_scopes: %w", err)
		}
	}

	return r.Settings(ctx)
}

// ResetAll deletes all data (requires confirmation).
func (r *Resolver) ResetAll(ctx context.Context, confirm string) (bool, error) {
	if confirm != "RESET" {
		return false, fmt.Errorf("confirmation required: pass 'RESET' to confirm")
	}

	// Delete in order (respecting foreign key constraints)
	if err := r.repos.Activity.DeleteAll(ctx); err != nil {
		return false, fmt.Errorf("failed to delete activity: %w", err)
	}
	if err := r.repos.Reports.DeleteAll(ctx); err != nil {
		return false, fmt.Errorf("failed to delete reports: %w", err)
	}
	if err := r.repos.Labels.DeleteAll(ctx); err != nil {
		return false, fmt.Errorf("failed to delete labels: %w", err)
	}
	if err := r.repos.Records.DeleteAll(ctx); err != nil {
		return false, fmt.Errorf("failed to delete records: %w", err)
	}
	if err := r.repos.Actors.DeleteAll(ctx); err != nil {
		return false, fmt.Errorf("failed to delete actors: %w", err)
	}

	return true, nil
}

// PopulateActivity creates activity entries from existing records in the database.
// This is useful after a backfill to populate the activity dashboard with historical data.
func (r *Resolver) PopulateActivity(ctx context.Context) (int64, error) {
	// First clear existing activity to avoid duplicates
	if err := r.repos.Activity.DeleteAll(ctx); err != nil {
		return 0, fmt.Errorf("failed to clear existing activity: %w", err)
	}

	var count int64
	_, err := r.repos.Records.IterateAll(ctx, 1000, func(rec *repositories.Record) error {
		// Extract createdAt from the record JSON
		timestamp := atproto.ExtractCreatedAt(rec.JSON, time.Now())

		// Log as a successful create operation
		if _, logErr := r.repos.Activity.LogActivityWithStatus(ctx, timestamp, "create", rec.Collection, rec.DID, rec.RKey, rec.JSON, "success"); logErr == nil {
			count++
		}
		return nil
	})

	if err != nil {
		return count, fmt.Errorf("error iterating records: %w", err)
	}

	return count, nil
}

// CreateLabel creates a new label on a record or account.
func (r *Resolver) CreateLabel(ctx context.Context, uri, val string, cid, exp *string) (map[string]interface{}, error) {
	// Validate URI format
	if !repositories.IsValidSubjectURI(uri) {
		return nil, fmt.Errorf("invalid subject URI: must start with 'at://' or 'did:'")
	}

	// Validate label value exists
	exists, err := r.repos.LabelDefinitions.Exists(ctx, val)
	if err != nil {
		return nil, fmt.Errorf("failed to check label definition: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("label value '%s' not defined", val)
	}

	// Parse expiration if provided
	var expTime *time.Time
	if exp != nil {
		t, err := time.Parse(time.RFC3339, *exp)
		if err != nil {
			return nil, fmt.Errorf("invalid expiration format: %w", err)
		}
		expTime = &t
	}

	label, err := r.repos.Labels.Insert(ctx, r.domainDID, uri, cid, val, expTime)
	if err != nil {
		return nil, fmt.Errorf("failed to create label: %w", err)
	}

	result := map[string]interface{}{
		"id":  label.ID,
		"src": label.Src,
		"uri": label.URI,
		"val": label.Val,
		"neg": label.Neg,
		"cts": label.Cts.Format(time.RFC3339),
	}
	if label.CID != nil {
		result["cid"] = *label.CID
	}
	if label.Exp != nil {
		result["exp"] = label.Exp.Format(time.RFC3339)
	}

	return result, nil
}

// NegateLabel retracts a label from a record or account.
func (r *Resolver) NegateLabel(ctx context.Context, uri, val string) (map[string]interface{}, error) {
	// Validate URI format
	if !repositories.IsValidSubjectURI(uri) {
		return nil, fmt.Errorf("invalid subject URI: must start with 'at://' or 'did:'")
	}

	label, err := r.repos.Labels.InsertNegation(ctx, r.domainDID, uri, val)
	if err != nil {
		return nil, fmt.Errorf("failed to negate label: %w", err)
	}

	return map[string]interface{}{
		"id":  label.ID,
		"src": label.Src,
		"uri": label.URI,
		"val": label.Val,
		"neg": label.Neg,
		"cts": label.Cts.Format(time.RFC3339),
	}, nil
}

// CreateLabelDefinition creates a new label definition.
func (r *Resolver) CreateLabelDefinition(ctx context.Context, val, description, severity string, defaultVisibility *string) (map[string]interface{}, error) {
	// Validate severity
	sev, err := repositories.ValidateSeverity(severity)
	if err != nil {
		return nil, err
	}

	// Default visibility
	vis := repositories.VisibilityWarn
	if defaultVisibility != nil {
		vis, err = repositories.ValidateVisibility(*defaultVisibility)
		if err != nil {
			return nil, err
		}
	}

	// Check if already exists
	exists, err := r.repos.LabelDefinitions.Exists(ctx, val)
	if err != nil {
		return nil, fmt.Errorf("failed to check label definition: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("label '%s' already exists", val)
	}

	if err := r.repos.LabelDefinitions.Insert(ctx, val, description, sev, vis); err != nil {
		return nil, fmt.Errorf("failed to create label definition: %w", err)
	}

	def, err := r.repos.LabelDefinitions.Get(ctx, val)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created definition: %w", err)
	}

	return map[string]interface{}{
		"val":               def.Val,
		"description":       def.Description,
		"severity":          string(def.Severity),
		"defaultVisibility": string(def.DefaultVisibility),
		"createdAt":         def.CreatedAt.Format(time.RFC3339),
	}, nil
}

// ResolveReport resolves a moderation report.
func (r *Resolver) ResolveReport(ctx context.Context, id int64, action string, labelVal *string, resolverDID string) (map[string]interface{}, error) {
	// Get the report
	report, err := r.repos.Reports.Get(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("report not found")
		}
		return nil, fmt.Errorf("failed to get report: %w", err)
	}

	var status repositories.ReportStatus
	switch action {
	case "apply_label":
		if labelVal == nil {
			return nil, fmt.Errorf("labelVal required for apply_label action")
		}
		// Apply the label
		_, err := r.repos.Labels.Insert(ctx, r.domainDID, report.SubjectURI, nil, *labelVal, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to apply label: %w", err)
		}
		status = repositories.StatusResolved
	case "dismiss":
		status = repositories.StatusDismissed
	default:
		return nil, fmt.Errorf("invalid action: %s", action)
	}

	// Update report status
	updatedReport, err := r.repos.Reports.Resolve(ctx, id, status, resolverDID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve report: %w", err)
	}

	result := map[string]interface{}{
		"id":          updatedReport.ID,
		"reporterDid": updatedReport.ReporterDID,
		"subjectUri":  updatedReport.SubjectURI,
		"reasonType":  string(updatedReport.ReasonType),
		"status":      string(updatedReport.Status),
		"createdAt":   updatedReport.CreatedAt.Format(time.RFC3339),
	}
	if updatedReport.Reason != nil {
		result["reason"] = *updatedReport.Reason
	}
	if updatedReport.ResolvedBy != nil {
		result["resolvedBy"] = *updatedReport.ResolvedBy
	}
	if updatedReport.ResolvedAt != nil {
		result["resolvedAt"] = updatedReport.ResolvedAt.Format(time.RFC3339)
	}

	return result, nil
}
