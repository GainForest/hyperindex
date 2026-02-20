package admin

import (
	"context"
	"fmt"

	"github.com/graphql-go/graphql"
)

// SchemaBuilder builds the admin GraphQL schema.
type SchemaBuilder struct {
	resolver *Resolver
}

// NewSchemaBuilder creates a new admin schema builder.
func NewSchemaBuilder(resolver *Resolver) *SchemaBuilder {
	return &SchemaBuilder{resolver: resolver}
}

// Build creates the complete admin GraphQL schema.
func (b *SchemaBuilder) Build() (*graphql.Schema, error) {
	queryType := b.buildQueryType()
	mutationType := b.buildMutationType()

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    queryType,
		Mutation: mutationType,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create admin schema: %w", err)
	}

	return &schema, nil
}

// buildQueryType builds the root Query type for the admin API.
func (b *SchemaBuilder) buildQueryType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name:        "Query",
		Description: "Admin API queries",
		Fields: graphql.Fields{
			"currentSession": &graphql.Field{
				Type:        CurrentSessionType,
				Description: "Get current authenticated user session",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					// Session info is extracted from context in the HTTP handler
					ctx := p.Context
					userDID, _ := ctx.Value(contextKeyUserDID).(string)
					handle, _ := ctx.Value(contextKeyHandle).(string)
					adminDIDs, _ := ctx.Value(contextKeyAdminDIDs).([]string)

					if userDID == "" {
						return nil, nil // Not authenticated
					}

					return b.resolver.CurrentSession(ctx, userDID, handle, adminDIDs), nil
				},
			},
			"statistics": &graphql.Field{
				Type:        graphql.NewNonNull(StatisticsType),
				Description: "Get system statistics (public)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					return b.resolver.Statistics(p.Context)
				},
			},
			"settings": &graphql.Field{
				Type:        graphql.NewNonNull(SettingsType),
				Description: "Get system settings (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.Settings(p.Context)
				},
			},
			"isBackfilling": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Check if a backfill is currently running (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.IsBackfilling(), nil
				},
			},
			"lexicons": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(LexiconType))),
				Description: "Get all lexicon definitions (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.Lexicons(p.Context)
				},
			},
			"oauthClients": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(OAuthClientType))),
				Description: "Get all OAuth client registrations (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.OAuthClients(p.Context)
				},
			},
			"activityBuckets": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(ActivityBucketType))),
				Description: "Get aggregated activity data for a time range (public)",
				Args: graphql.FieldConfigArgument{
					"range": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(TimeRangeEnum),
						Description: "Time range for bucketing",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					timeRange, _ := p.Args["range"].(string)
					return b.resolver.ActivityBuckets(p.Context, timeRange)
				},
			},
			"recentActivity": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(ActivityEntryType))),
				Description: "Get recent activity entries (public)",
				Args: graphql.FieldConfigArgument{
					"hours": &graphql.ArgumentConfig{
						Type:         graphql.NewNonNull(graphql.Int),
						Description:  "Number of hours to look back",
						DefaultValue: 1,
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					hours, _ := p.Args["hours"].(int)
					if hours < 1 {
						hours = 1
					}
					if hours > 168 {
						hours = 168 // Max 7 days
					}
					return b.resolver.RecentActivity(p.Context, hours)
				},
			},
			"labelDefinitions": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(LabelDefinitionType))),
				Description: "Get all label definitions (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.LabelDefinitions(p.Context)
				},
			},
			"viewerLabelPreferences": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(LabelPreferenceType))),
				Description: "Get current user's label preferences (authenticated)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					userDID, _ := p.Context.Value(contextKeyUserDID).(string)
					if userDID == "" {
						return nil, fmt.Errorf("authentication required")
					}
					return b.resolver.ViewerLabelPreferences(p.Context, userDID)
				},
			},
			"labels": &graphql.Field{
				Type:        graphql.NewNonNull(LabelConnectionType),
				Description: "Get labels with optional filters (admin only)",
				Args: graphql.FieldConfigArgument{
					"uri": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Filter by subject URI",
					},
					"val": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Filter by label value",
					},
					"first": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						Description:  "Number of items to return",
						DefaultValue: 20,
					},
					"after": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Cursor for pagination",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					uri, _ := p.Args["uri"].(string)
					val, _ := p.Args["val"].(string)
					first, _ := p.Args["first"].(int)
					after, _ := p.Args["after"].(string)

					var uriPtr, valPtr, afterPtr *string
					if uri != "" {
						uriPtr = &uri
					}
					if val != "" {
						valPtr = &val
					}
					if after != "" {
						afterPtr = &after
					}

					return b.resolver.Labels(p.Context, uriPtr, valPtr, first, afterPtr)
				},
			},
			"reports": &graphql.Field{
				Type:        graphql.NewNonNull(ReportConnectionType),
				Description: "Get moderation reports (admin only)",
				Args: graphql.FieldConfigArgument{
					"status": &graphql.ArgumentConfig{
						Type:        ReportStatusEnum,
						Description: "Filter by status",
					},
					"first": &graphql.ArgumentConfig{
						Type:         graphql.Int,
						Description:  "Number of items to return",
						DefaultValue: 20,
					},
					"after": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Cursor for pagination",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					status, _ := p.Args["status"].(string)
					first, _ := p.Args["first"].(int)
					after, _ := p.Args["after"].(string)

					var statusPtr, afterPtr *string
					if status != "" {
						statusPtr = &status
					}
					if after != "" {
						afterPtr = &after
					}

					return b.resolver.Reports(p.Context, statusPtr, first, afterPtr)
				},
			},
		},
	})
}

// buildMutationType builds the root Mutation type for the admin API.
func (b *SchemaBuilder) buildMutationType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name:        "Mutation",
		Description: "Admin API mutations",
		Fields: graphql.Fields{
			"updateSettings": &graphql.Field{
				Type:        graphql.NewNonNull(SettingsType),
				Description: "Update system settings (admin only)",
				Args: graphql.FieldConfigArgument{
					"domainAuthority": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Domain authority (e.g., example.com)",
					},
					"adminDids": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Comma-separated list of admin DIDs",
					},
					"relayUrl": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "AT Protocol relay URL",
					},
					"plcDirectoryUrl": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "PLC directory URL",
					},
					"jetstreamUrl": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Jetstream WebSocket URL",
					},
					"oauthSupportedScopes": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Space-separated OAuth scopes",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}

					domainAuthority, _ := p.Args["domainAuthority"].(string)
					adminDids, _ := p.Args["adminDids"].(string)
					relayURL, _ := p.Args["relayUrl"].(string)
					plcDirectoryURL, _ := p.Args["plcDirectoryUrl"].(string)
					jetstreamURL, _ := p.Args["jetstreamUrl"].(string)
					oauthScopes, _ := p.Args["oauthSupportedScopes"].(string)

					var domainPtr, adminPtr, relayPtr, plcPtr, jetPtr, scopesPtr *string
					if domainAuthority != "" {
						domainPtr = &domainAuthority
					}
					if adminDids != "" {
						adminPtr = &adminDids
					}
					if relayURL != "" {
						relayPtr = &relayURL
					}
					if plcDirectoryURL != "" {
						plcPtr = &plcDirectoryURL
					}
					if jetstreamURL != "" {
						jetPtr = &jetstreamURL
					}
					if oauthScopes != "" {
						scopesPtr = &oauthScopes
					}

					return b.resolver.UpdateSettings(p.Context, domainPtr, adminPtr, relayPtr, plcPtr, jetPtr, scopesPtr)
				},
			},
			"resetAll": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Delete all data (admin only, requires confirmation)",
				Args: graphql.FieldConfigArgument{
					"confirm": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Must be 'RESET' to confirm",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					confirm, _ := p.Args["confirm"].(string)
					return b.resolver.ResetAll(p.Context, confirm)
				},
			},
			"populateActivity": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Int),
				Description: "Populate activity entries from existing records (admin only). Returns count of entries created.",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.PopulateActivity(p.Context)
				},
			},
			"createLabel": &graphql.Field{
				Type:        graphql.NewNonNull(LabelType),
				Description: "Create a label on a record or account (admin only)",
				Args: graphql.FieldConfigArgument{
					"uri": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Subject URI (at:// or did:)",
					},
					"val": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Label value",
					},
					"cid": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Optional CID for version-specific label",
					},
					"exp": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Optional expiration timestamp (ISO 8601)",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					uri, _ := p.Args["uri"].(string)
					val, _ := p.Args["val"].(string)
					cid, _ := p.Args["cid"].(string)
					exp, _ := p.Args["exp"].(string)

					var cidPtr, expPtr *string
					if cid != "" {
						cidPtr = &cid
					}
					if exp != "" {
						expPtr = &exp
					}

					return b.resolver.CreateLabel(p.Context, uri, val, cidPtr, expPtr)
				},
			},
			"negateLabel": &graphql.Field{
				Type:        graphql.NewNonNull(LabelType),
				Description: "Negate (retract) a label (admin only)",
				Args: graphql.FieldConfigArgument{
					"uri": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Subject URI (at:// or did:)",
					},
					"val": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Label value to negate",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					uri, _ := p.Args["uri"].(string)
					val, _ := p.Args["val"].(string)
					return b.resolver.NegateLabel(p.Context, uri, val)
				},
			},
			"createLabelDefinition": &graphql.Field{
				Type:        graphql.NewNonNull(LabelDefinitionType),
				Description: "Create a new label definition (admin only)",
				Args: graphql.FieldConfigArgument{
					"val": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Label value",
					},
					"description": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Human-readable description",
					},
					"severity": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(LabelSeverityEnum),
						Description: "Severity level",
					},
					"defaultVisibility": &graphql.ArgumentConfig{
						Type:        LabelVisibilityEnum,
						Description: "Default visibility (defaults to 'warn')",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					val, _ := p.Args["val"].(string)
					description, _ := p.Args["description"].(string)
					severity, _ := p.Args["severity"].(string)
					defaultVisibility, _ := p.Args["defaultVisibility"].(string)

					var visPtr *string
					if defaultVisibility != "" {
						visPtr = &defaultVisibility
					}

					return b.resolver.CreateLabelDefinition(p.Context, val, description, severity, visPtr)
				},
			},
			"resolveReport": &graphql.Field{
				Type:        graphql.NewNonNull(ReportType),
				Description: "Resolve a moderation report (admin only)",
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.Int),
						Description: "Report ID",
					},
					"action": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(ReportActionEnum),
						Description: "Action to take",
					},
					"labelVal": &graphql.ArgumentConfig{
						Type:        graphql.String,
						Description: "Label value to apply (required for apply_label action)",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					id, _ := p.Args["id"].(int)
					action, _ := p.Args["action"].(string)
					labelVal, _ := p.Args["labelVal"].(string)

					var labelValPtr *string
					if labelVal != "" {
						labelValPtr = &labelVal
					}

					resolverDID, _ := p.Context.Value(contextKeyUserDID).(string)
					return b.resolver.ResolveReport(p.Context, int64(id), action, labelValPtr, resolverDID)
				},
			},
			"uploadLexicons": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Int),
				Description: "Upload lexicons from a base64-encoded ZIP file (admin only)",
				Args: graphql.FieldConfigArgument{
					"zipBase64": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Base64-encoded ZIP file containing lexicon JSON files",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					zipBase64, _ := p.Args["zipBase64"].(string)
					return b.resolver.UploadLexicons(p.Context, zipBase64)
				},
			},
			"triggerBackfill": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Trigger a full backfill of all known actors (admin only)",
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					return b.resolver.TriggerBackfill(p.Context)
				},
			},
			"backfillActor": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Backfill a single actor by DID (admin only)",
				Args: graphql.FieldConfigArgument{
					"did": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "The DID of the actor to backfill",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					did, _ := p.Args["did"].(string)
					return b.resolver.BackfillActor(p.Context, did)
				},
			},
			"createOAuthClient": &graphql.Field{
				Type:        graphql.NewNonNull(OAuthClientType),
				Description: "Create a new OAuth client (admin only)",
				Args: graphql.FieldConfigArgument{
					"clientName": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Human-readable client name",
					},
					"clientType": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Client type: 'public' or 'confidential'",
					},
					"redirectUris": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String))),
						Description: "List of allowed redirect URIs",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					clientName, _ := p.Args["clientName"].(string)
					clientType, _ := p.Args["clientType"].(string)
					redirectURIsRaw, _ := p.Args["redirectUris"].([]interface{})
					redirectURIs := make([]string, len(redirectURIsRaw))
					for i, uri := range redirectURIsRaw {
						redirectURIs[i], _ = uri.(string)
					}
					return b.resolver.CreateOAuthClient(p.Context, clientName, clientType, redirectURIs)
				},
			},
			"updateOAuthClient": &graphql.Field{
				Type:        graphql.NewNonNull(OAuthClientType),
				Description: "Update an existing OAuth client (admin only)",
				Args: graphql.FieldConfigArgument{
					"clientId": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Client ID to update",
					},
					"clientName": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "New client name",
					},
					"redirectUris": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.NewList(graphql.NewNonNull(graphql.String))),
						Description: "New list of allowed redirect URIs",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					clientID, _ := p.Args["clientId"].(string)
					clientName, _ := p.Args["clientName"].(string)
					redirectURIsRaw, _ := p.Args["redirectUris"].([]interface{})
					redirectURIs := make([]string, len(redirectURIsRaw))
					for i, uri := range redirectURIsRaw {
						redirectURIs[i], _ = uri.(string)
					}
					return b.resolver.UpdateOAuthClient(p.Context, clientID, clientName, redirectURIs)
				},
			},
			"deleteOAuthClient": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Delete an OAuth client (admin only)",
				Args: graphql.FieldConfigArgument{
					"clientId": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "Client ID to delete",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					clientID, _ := p.Args["clientId"].(string)
					return b.resolver.DeleteOAuthClient(p.Context, clientID)
				},
			},
			"addAdmin": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Add a DID to the admin list (admin only)",
				Args: graphql.FieldConfigArgument{
					"did": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "DID to add as admin",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					did, _ := p.Args["did"].(string)
					return b.resolver.AddAdmin(p.Context, did)
				},
			},
			"removeAdmin": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Remove a DID from the admin list (admin only)",
				Args: graphql.FieldConfigArgument{
					"did": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "DID to remove from admin list",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					did, _ := p.Args["did"].(string)
					return b.resolver.RemoveAdmin(p.Context, did)
				},
			},
			"registerLexicon": &graphql.Field{
				Type:        graphql.NewNonNull(LexiconType),
				Description: "Register a lexicon by NSID (resolves via DNS and fetches from PDS)",
				Args: graphql.FieldConfigArgument{
					"nsid": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "The NSID of the lexicon to register (e.g., org.hypercerts.claim.activity)",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					nsid, _ := p.Args["nsid"].(string)
					return b.resolver.RegisterLexicon(p.Context, nsid)
				},
			},
			"deleteLexicon": &graphql.Field{
				Type:        graphql.NewNonNull(graphql.Boolean),
				Description: "Delete a registered lexicon by NSID (admin only)",
				Args: graphql.FieldConfigArgument{
					"nsid": &graphql.ArgumentConfig{
						Type:        graphql.NewNonNull(graphql.String),
						Description: "The NSID of the lexicon to delete",
					},
				},
				Resolve: func(p graphql.ResolveParams) (interface{}, error) {
					if err := requireAdmin(p.Context); err != nil {
						return nil, err
					}
					nsid, _ := p.Args["nsid"].(string)
					return b.resolver.DeleteLexicon(p.Context, nsid)
				},
			},
		},
	})
}

// Context keys for passing auth info to resolvers.
type contextKey string

const (
	contextKeyUserDID   contextKey = "userDID"
	contextKeyHandle    contextKey = "handle"
	contextKeyIsAdmin   contextKey = "isAdmin"
	contextKeyAdminDIDs contextKey = "adminDIDs"
)

// ContextWithAuth adds authentication info to the context.
func ContextWithAuth(ctx context.Context, userDID, handle string, isAdmin bool, adminDIDs []string) context.Context {
	ctx = context.WithValue(ctx, contextKeyUserDID, userDID)
	ctx = context.WithValue(ctx, contextKeyHandle, handle)
	ctx = context.WithValue(ctx, contextKeyIsAdmin, isAdmin)
	ctx = context.WithValue(ctx, contextKeyAdminDIDs, adminDIDs)
	return ctx
}

// requireAdmin checks if the current user is an admin.
func requireAdmin(ctx context.Context) error {
	isAdmin, _ := ctx.Value(contextKeyIsAdmin).(bool)
	if !isAdmin {
		return fmt.Errorf("admin privileges required")
	}
	return nil
}
