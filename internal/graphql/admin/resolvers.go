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
	"strconv"
	"strings"
	"time"

	"github.com/GainForest/hypergoat/internal/database/repositories"
	"github.com/GainForest/hypergoat/internal/lexicon"
	"github.com/GainForest/hypergoat/internal/oauth"
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

// BackfillCallback is called when backfill is triggered.
type BackfillCallback func(ctx context.Context, did string) error

// Resolver provides methods for resolving admin GraphQL queries and mutations.
type Resolver struct {
	repos            *Repositories
	backfillActive   bool
	domainDID        string // The DID of this labeler instance
	backfillCallback BackfillCallback
}

// NewResolver creates a new admin resolver.
func NewResolver(repos *Repositories, domainDID string) *Resolver {
	return &Resolver{
		repos:     repos,
		domainDID: domainDID,
	}
}

// SetBackfillCallback sets the callback for backfill operations.
func (r *Resolver) SetBackfillCallback(cb BackfillCallback) {
	r.backfillCallback = cb
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
	isAdmin := false
	for _, adminDID := range adminDIDs {
		if adminDID == userDID {
			isAdmin = true
			break
		}
	}

	return map[string]interface{}{
		"did":     userDID,
		"handle":  handle,
		"isAdmin": isAdmin,
	}
}

// Settings returns system settings.
func (r *Resolver) Settings(ctx context.Context) (map[string]interface{}, error) {
	domainAuthority, _ := r.repos.Config.Get(ctx, "domain_authority")
	adminDidsStr, _ := r.repos.Config.Get(ctx, "admin_dids")
	relayURL, _ := r.repos.Config.Get(ctx, "relay_url")
	plcDirectoryURL, _ := r.repos.Config.Get(ctx, "plc_directory_url")
	jetstreamURL, _ := r.repos.Config.Get(ctx, "jetstream_url")
	oauthScopes, _ := r.repos.Config.Get(ctx, "oauth_supported_scopes")

	// Parse admin DIDs from comma-separated string
	var adminDids []string
	if adminDidsStr != "" {
		adminDids = strings.Split(adminDidsStr, ",")
		for i := range adminDids {
			adminDids[i] = strings.TrimSpace(adminDids[i])
		}
	}

	return map[string]interface{}{
		"id":                   "settings",
		"domainAuthority":      domainAuthority,
		"adminDids":            adminDids,
		"relayUrl":             relayURL,
		"plcDirectoryUrl":      plcDirectoryURL,
		"jetstreamUrl":         jetstreamURL,
		"oauthSupportedScopes": oauthScopes,
	}, nil
}

// IsBackfilling returns whether a backfill is currently active.
func (r *Resolver) IsBackfilling() bool {
	return r.backfillActive
}

// SetBackfillActive sets the backfill status.
func (r *Resolver) SetBackfillActive(active bool) {
	r.backfillActive = active
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

// UploadLexicons extracts lexicons from a base64-encoded ZIP file.
func (r *Resolver) UploadLexicons(ctx context.Context, zipBase64 string) (int, error) {
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

	// Process each file
	count := 0
	for _, file := range zipReader.File {
		// Skip directories and non-JSON files
		if file.FileInfo().IsDir() || !strings.HasSuffix(file.Name, ".json") {
			continue
		}

		// Open and read file
		rc, err := file.Open()
		if err != nil {
			continue
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		// Parse JSON to extract lexicon ID
		var lexicon struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &lexicon); err != nil {
			continue
		}

		if lexicon.ID == "" {
			continue
		}

		// Upsert lexicon
		if err := r.repos.Lexicons.Upsert(ctx, lexicon.ID, string(data)); err != nil {
			return count, fmt.Errorf("failed to save lexicon %s: %w", lexicon.ID, err)
		}
		count++
	}

	return count, nil
}

// TriggerBackfill starts a full backfill process.
func (r *Resolver) TriggerBackfill(ctx context.Context) (bool, error) {
	if r.backfillActive {
		return false, fmt.Errorf("backfill already in progress")
	}
	r.backfillActive = true
	// The actual backfill is handled by the background worker that checks this flag
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

// AddAdmin adds a DID to the admin list.
func (r *Resolver) AddAdmin(ctx context.Context, did string) (bool, error) {
	// Validate DID format
	if !strings.HasPrefix(did, "did:") {
		return false, fmt.Errorf("invalid DID format")
	}

	// Get current admin DIDs
	adminDidsStr, _ := r.repos.Config.Get(ctx, "admin_dids")
	var adminDids []string
	if adminDidsStr != "" {
		adminDids = strings.Split(adminDidsStr, ",")
		for i := range adminDids {
			adminDids[i] = strings.TrimSpace(adminDids[i])
		}
	}

	// Check if already admin
	for _, existingDID := range adminDids {
		if existingDID == did {
			return true, nil // Already an admin
		}
	}

	// Add new admin
	adminDids = append(adminDids, did)
	newAdminDidsStr := strings.Join(adminDids, ",")

	if err := r.repos.Config.Set(ctx, "admin_dids", newAdminDidsStr); err != nil {
		return false, fmt.Errorf("failed to update admin_dids: %w", err)
	}

	return true, nil
}

// RemoveAdmin removes a DID from the admin list.
func (r *Resolver) RemoveAdmin(ctx context.Context, did string) (bool, error) {
	// Get current admin DIDs
	adminDidsStr, _ := r.repos.Config.Get(ctx, "admin_dids")
	if adminDidsStr == "" {
		return false, fmt.Errorf("no admins configured")
	}

	adminDids := strings.Split(adminDidsStr, ",")
	for i := range adminDids {
		adminDids[i] = strings.TrimSpace(adminDids[i])
	}

	// Prevent removing the last admin
	if len(adminDids) <= 1 {
		return false, fmt.Errorf("cannot remove the last admin")
	}

	// Find and remove the DID
	found := false
	newAdminDids := make([]string, 0, len(adminDids)-1)
	for _, existingDID := range adminDids {
		if existingDID == did {
			found = true
		} else {
			newAdminDids = append(newAdminDids, existingDID)
		}
	}

	if !found {
		return false, fmt.Errorf("DID is not an admin")
	}

	newAdminDidsStr := strings.Join(newAdminDids, ",")
	if err := r.repos.Config.Set(ctx, "admin_dids", newAdminDidsStr); err != nil {
		return false, fmt.Errorf("failed to update admin_dids: %w", err)
	}

	return true, nil
}

// RegisterLexicon resolves an NSID via DNS and registers the lexicon schema.
func (r *Resolver) RegisterLexicon(ctx context.Context, nsid string) (map[string]interface{}, error) {
	// Validate NSID format (at least 3 dot-separated segments)
	parts := strings.Split(nsid, ".")
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid NSID format: must have at least 3 segments (e.g., app.bsky.feed.post)")
	}

	// Check if lexicon already exists
	exists, err := r.repos.Lexicons.Exists(ctx, nsid)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing lexicon: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("lexicon %s is already registered", nsid)
	}

	// Resolve lexicon via DNS and PDS
	resolver := lexicon.NewResolver()
	resolved, err := resolver.ResolveLexicon(ctx, nsid)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve lexicon: %w", err)
	}

	// Store the lexicon schema
	schemaJSON := string(resolved.Schema)
	if err := r.repos.Lexicons.Upsert(ctx, nsid, schemaJSON); err != nil {
		return nil, fmt.Errorf("failed to save lexicon: %w", err)
	}

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
		"id":          nsid,
		"json":        schemaJSON,
		"createdAt":   time.Now().Format(time.RFC3339),
		"did":         resolved.DID,
		"description": description,
	}, nil
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
func (r *Resolver) UpdateSettings(ctx context.Context, domainAuthority, adminDids, relayURL, plcDirectoryURL, jetstreamURL, oauthScopes *string) (map[string]interface{}, error) {
	if domainAuthority != nil {
		if err := r.repos.Config.Set(ctx, "domain_authority", *domainAuthority); err != nil {
			return nil, fmt.Errorf("failed to update domain_authority: %w", err)
		}
	}

	if adminDids != nil {
		// adminDids is passed as comma-separated string
		if err := r.repos.Config.Set(ctx, "admin_dids", *adminDids); err != nil {
			return nil, fmt.Errorf("failed to update admin_dids: %w", err)
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
