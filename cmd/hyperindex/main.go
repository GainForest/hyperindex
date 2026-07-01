// Package main is the entry point for the Hyperindex server.
//
// Hyperindex is a Go implementation of Quickslice - an AT Protocol AppView server
// that indexes Lexicon-defined records and exposes them via a dynamically-generated
// GraphQL API.
//
// For more information, see the README.md file.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/GainForest/hyperindex/internal/atproto"
	"github.com/GainForest/hyperindex/internal/backfill"
	"github.com/GainForest/hyperindex/internal/buildinfo"
	"github.com/GainForest/hyperindex/internal/config"
	"github.com/GainForest/hyperindex/internal/database"
	"github.com/GainForest/hyperindex/internal/database/migrations"
	"github.com/GainForest/hyperindex/internal/database/repositories"
	hgraphql "github.com/GainForest/hyperindex/internal/graphql"
	"github.com/GainForest/hyperindex/internal/graphql/admin"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/subscription"
	"github.com/GainForest/hyperindex/internal/jetstream"
	"github.com/GainForest/hyperindex/internal/labeler"
	"github.com/GainForest/hyperindex/internal/lexicon"
	"github.com/GainForest/hyperindex/internal/logsafe"
	"github.com/GainForest/hyperindex/internal/oauth"
	"github.com/GainForest/hyperindex/internal/server"
	"github.com/GainForest/hyperindex/internal/tap"
	"github.com/GainForest/hyperindex/internal/validation"
	"github.com/GainForest/hyperindex/internal/validationrefresh"
	"github.com/GainForest/hyperindex/internal/workers"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

// services holds all shared repositories and infrastructure dependencies.
// Created once in initServices and threaded through all setup functions,
// eliminating duplicate repository instantiation.
type services struct {
	db                database.Executor
	records           *repositories.RecordsRepository
	actors            *repositories.ActorsRepository
	lexicons          *repositories.LexiconsRepository
	config            *repositories.ConfigRepository
	activity          *repositories.IndexingActivityRepository
	oauthClients      *repositories.OAuthClientsRepository
	labels            *repositories.LabelsRepository
	externalLabels    *repositories.ExternalLabelsRepository
	labelDefinitions  *repositories.LabelDefinitionsRepository
	labelPreferences  *repositories.LabelPreferencesRepository
	reports           *repositories.ReportsRepository
	validator         *validation.Validator
	validationRefresh *validationrefresh.Scheduler
}

// backgroundServices tracks cancellable background goroutines for clean shutdown.
type backgroundServices struct {
	oauthCleanupCancel context.CancelFunc
	workersCancel      context.CancelFunc
	jsConsumer         *jetstream.Consumer
	jsCancel           context.CancelFunc
	tapConsumer        *tap.Consumer
	tapCancel          context.CancelFunc
	tapAdminClient     *tap.AdminClient // reused for stats checks; nil when TAP_ENABLED=false
	labelerCancel      context.CancelFunc
}

// Stop cleanly shuts down all background services.
func (bg *backgroundServices) Stop() {
	if bg.oauthCleanupCancel != nil {
		bg.oauthCleanupCancel()
	}
	if bg.workersCancel != nil {
		bg.workersCancel()
	}
	if bg.jsConsumer != nil {
		bg.jsConsumer.Stop()
	}
	if bg.jsCancel != nil {
		bg.jsCancel()
	}
	if bg.tapConsumer != nil {
		bg.tapConsumer.Stop()
	}
	if bg.tapCancel != nil {
		bg.tapCancel()
	}
	if bg.labelerCancel != nil {
		bg.labelerCancel()
	}
}

func run() error {
	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Hyperindex - AT Protocol AppView Server")

	// Load and validate configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	cfg.LogConfig()

	// Initialize all services (DB, migrations, repositories)
	svc, err := initServices(cfg)
	if err != nil {
		return err
	}
	defer svc.db.Close()

	// Track background services for clean shutdown
	bg := &backgroundServices{}
	defer bg.Stop()

	// Set up HTTP router with middleware and basic endpoints
	r := setupRouter(cfg, svc, bg)

	// Set up OAuth endpoints
	setupOAuth(r, cfg, svc, bg)

	// Set up admin GraphQL endpoint with backfill callbacks
	adminHandler := setupAdmin(r, cfg, svc)

	// Load lexicons and set up public GraphQL + subscriptions
	pubsub := subscription.NewPubSub()
	collections, err := setupGraphQL(r, cfg, svc, pubsub, adminHandler)
	if err != nil {
		return err
	}

	// Start background workers (activity cleanup)
	startWorkers(svc, bg)

	// Start external labeler subscriptions if configured.
	startLabelerSubscribers(cfg, svc, bg)

	if cfg.TapEnabled {
		// Start Tap consumer instead of Jetstream+Backfill
		startTap(cfg, svc, pubsub, adminHandler, bg)
	} else {
		// Start Jetstream consumer for real-time events
		startJetstream(cfg, svc, pubsub, collections, adminHandler, bg)

		// Start backfill if configured
		startBackfill(cfg, svc)
	}

	// Run HTTP server with graceful shutdown
	return serve(r, cfg, bg)
}

// initServices connects to the database, runs migrations, and creates all
// repository instances. IndexingActivityRepository is created once here
// instead of being duplicated across multiple call sites.
func initServices(cfg *config.Config) (*services, error) {
	db, err := server.ConnectDatabase(cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	slog.Info("Database connected successfully", "dialect", db.Dialect().String())

	slog.Info("Running database migrations...")
	if err := migrations.Run(context.Background(), db); err != nil {
		return nil, err
	}
	slog.Info("Database migrations complete")

	svc := &services{
		db:               db,
		records:          repositories.NewRecordsRepository(db),
		actors:           repositories.NewActorsRepository(db),
		lexicons:         repositories.NewLexiconsRepository(db),
		config:           repositories.NewConfigRepository(db),
		activity:         repositories.NewIndexingActivityRepository(db),
		oauthClients:     repositories.NewOAuthClientsRepository(db),
		labels:           repositories.NewLabelsRepository(db),
		externalLabels:   repositories.NewExternalLabelsRepository(db),
		labelDefinitions: repositories.NewLabelDefinitionsRepository(db),
		labelPreferences: repositories.NewLabelPreferencesRepository(db),
		reports:          repositories.NewReportsRepository(db),
	}

	if cfg.PLCDirectoryURL != "" {
		svc.config.SetPLCDirectoryOverride(cfg.PLCDirectoryURL)
	}

	// Initialize config defaults and admin DIDs
	ctx := context.Background()
	if err := svc.config.InitializeDefaults(ctx); err != nil {
		slog.Warn("Failed to initialize config defaults", "error", err)
	}

	// Auto-populate activity from existing records if activity table is empty
	go populateActivityIfEmpty(ctx, svc)

	return svc, nil
}

// populateActivityIfEmpty creates activity entries from existing records when
// the activity table is empty but records exist (e.g., after a migration).
func populateActivityIfEmpty(ctx context.Context, svc *services) {
	recordCount, err := svc.records.GetCount(ctx)
	if err != nil {
		slog.Warn("Failed to get record count for activity population", "error", err)
		return
	}
	if recordCount == 0 {
		return
	}

	activityCount, err := svc.activity.GetCount(ctx)
	if err != nil {
		slog.Warn("Failed to get activity count", "error", err)
		return
	}
	if activityCount > 0 {
		return
	}

	slog.Info("Populating activity from existing records...", "record_count", recordCount)
	populated, err := populateActivityFromRecords(ctx, svc.records, svc.activity)
	if err != nil {
		slog.Error("Failed to populate activity", "error", err)
	} else {
		slog.Info("Activity populated from existing records", "count", populated)
	}
}

// setupRouter creates the chi router with middleware and basic HTTP endpoints
// (health, readiness, stats, root info, XRPC placeholder).
// bg is passed so that stats handlers can access the Tap admin client once it
// is wired up by startTap (after setupRouter returns).
func setupRouter(cfg *config.Config, svc *services, bg *backgroundServices) *chi.Mux {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(60 * time.Second))

	// CORS — uses AllowedOrigins from config; defaults to "*" if not set
	var allowedOrigins []string
	if cfg.AllowedOrigins != "" {
		for _, o := range strings.Split(cfg.AllowedOrigins, ",") {
			allowedOrigins = append(allowedOrigins, strings.TrimSpace(o))
		}
	}
	allowAdminAPIKeyAuth := cfg.AdminAPIKey != ""
	r.Use(server.CORSMiddleware(server.CORSConfig{
		AllowedOrigins:       allowedOrigins,
		AllowAdminAPIKeyAuth: allowAdminAPIKeyAuth,
	}))

	// Health check reports process liveness only. Dependency readiness and
	// deterministic labeler cursor failures are exposed via GET /ready.
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		payload := map[string]any{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	})

	// Readiness check reports whether dependencies that should gate traffic are
	// currently usable. Detailed non-blocking observability lives in GET /stats.
	r.Get("/ready", func(w http.ResponseWriter, req *http.Request) {
		payload := map[string]any{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		}
		statusCode := http.StatusOK

		if err := databaseReady(req.Context(), svc); err != nil {
			statusCode = http.StatusServiceUnavailable
			payload["status"] = "not_ready"
			payload["databaseError"] = err.Error()
		} else {
			labelerDiagnostics, labelerFatal, labelerErr := configuredLabelerDiagnostics(req.Context(), cfg, svc.config, svc.externalLabels)
			if labelerErr != nil {
				statusCode = http.StatusServiceUnavailable
				payload["status"] = "not_ready"
				payload["labelersError"] = labelerErr.Error()
			} else if labelerFatal {
				statusCode = http.StatusServiceUnavailable
				payload["status"] = "not_ready"
				payload["labelers"] = labelerDiagnostics
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		_ = json.NewEncoder(w).Encode(payload)
	})

	// Stats endpoint
	r.Get("/stats", func(w http.ResponseWriter, req *http.Request) {
		reqCtx := req.Context()

		recordCount, err := svc.records.GetCount(reqCtx)
		if err != nil {
			slog.Error("Failed to get record count", "error", err)
			recordCount = -1
		}

		actorCount, err := svc.actors.GetCount(reqCtx)
		if err != nil {
			slog.Error("Failed to get actor count", "error", err)
			actorCount = -1
		}

		lexiconCount, err := svc.lexicons.GetCount(reqCtx)
		if err != nil {
			slog.Error("Failed to get lexicon count", "error", err)
			lexiconCount = -1
		}

		stats := map[string]any{
			"records":  recordCount,
			"actors":   actorCount,
			"lexicons": lexiconCount,
			"time":     time.Now().UTC().Format(time.RFC3339),
		}

		if cfg.TapEnabled && bg.tapConsumer != nil {
			tapStats := bg.tapConsumer.Stats()
			tapInfo := map[string]any{
				"events_received": tapStats.EventsReceived,
				"records_created": tapStats.RecordsCreated,
				"records_updated": tapStats.RecordsUpdated,
				"records_deleted": tapStats.RecordsDeleted,
				"identity_events": tapStats.IdentityEvents,
				"errors":          tapStats.Errors,
			}
			// Include Tap sidecar health in stats for observability (non-blocking).
			if bg.tapAdminClient != nil {
				applyTapSidecarHealth(reqCtx, tapInfo, 1500*time.Millisecond, bg.tapAdminClient.Health)
			}
			stats["tap"] = tapInfo
		}

		if cfg.LabelerSubscribeEnabled {
			labelerDiagnostics, _, err := configuredLabelerDiagnostics(reqCtx, cfg, svc.config, svc.externalLabels)
			if err != nil {
				stats["labelers"] = []map[string]any{}
				stats["labelersError"] = err.Error()
			} else {
				stats["labelers"] = labelerDiagnostics
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(stats)
	})

	// Root endpoint - server info
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name":        "Hyperindex",
			"description": "AT Protocol AppView Server",
			"version":     buildinfo.Version,
			"docs":        cfg.ExternalBaseURL + "/docs",
		})
	})

	// Placeholder for XRPC endpoints (AT Protocol)
	r.Route("/xrpc", func(r chi.Router) {
		r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotImplemented)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "NotImplemented",
				"message": "XRPC endpoints are not yet implemented",
			})
		})
	})

	return r
}

func databaseReady(ctx context.Context, svc *services) error {
	if svc == nil || svc.db == nil {
		return errors.New("database executor is unavailable")
	}

	db := svc.db.DB()
	if db == nil {
		return errors.New("database connection is unavailable")
	}
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}

func applyTapSidecarHealth(
	reqCtx context.Context,
	tapInfo map[string]any,
	timeout time.Duration,
	healthFn func(context.Context) error,
) {
	sidecarCtx, cancel := context.WithTimeout(reqCtx, timeout)
	defer cancel()

	if err := healthFn(sidecarCtx); err != nil {
		tapInfo["sidecar"] = "unreachable"
		slog.Warn("Tap sidecar health check failed", "error", err)
		return
	}

	tapInfo["sidecar"] = "ok"
}

func configuredLabelerSubscribeURLs(ctx context.Context, cfg *config.Config, repo *repositories.ConfigRepository) []string {
	if cfg == nil {
		return nil
	}
	if repo != nil {
		if value, err := repo.Get(ctx, repositories.ConfigKeyLabelerSubscribeURLs); err == nil {
			return config.ParseLabelerSubscribeURLs(value)
		}
	}

	return cfg.LabelerSubscribeURLList()
}

func configuredLabelerDiagnostics(ctx context.Context, cfg *config.Config, configRepo *repositories.ConfigRepository, repo *repositories.ExternalLabelsRepository) ([]map[string]any, bool, error) {
	if cfg == nil || !cfg.LabelerSubscribeEnabled {
		return nil, false, nil
	}

	urls := configuredLabelerSubscribeURLs(ctx, cfg, configRepo)
	if len(urls) == 0 {
		return nil, false, nil
	}
	if repo == nil {
		return nil, false, fmt.Errorf("external label subscriptions are enabled but the external label repository is unavailable")
	}

	states, err := repo.ListStates(ctx, urls)
	if err != nil {
		return nil, false, fmt.Errorf("list external label subscription states: %w", err)
	}

	diagnostics := make([]map[string]any, 0, len(states))
	fatal := false
	for _, state := range states {
		if state.IsFatal() {
			fatal = true
		}
		diagnostics = append(diagnostics, labelerDiagnostic(state))
	}

	return diagnostics, fatal, nil
}

func labelerDiagnostic(state repositories.LabelSubscriptionState) map[string]any {
	diagnostic := map[string]any{
		"url":     state.URL,
		"status":  state.Status(),
		"lastSeq": state.LastSeq,
	}

	if code := repositories.FatalCursorCode(state.LastError); code != "" {
		diagnostic["lastErrorCode"] = code
	}
	if state.LastError != nil {
		diagnostic["lastError"] = *state.LastError
	}
	if state.LastConnectedAt != nil {
		diagnostic["lastConnectedAt"] = state.LastConnectedAt.UTC().Format(time.RFC3339)
	}
	if state.LastEventAt != nil {
		diagnostic["lastEventAt"] = state.LastEventAt.UTC().Format(time.RFC3339)
	}
	if !state.CreatedAt.IsZero() {
		diagnostic["createdAt"] = state.CreatedAt.UTC().Format(time.RFC3339)
	}
	if !state.UpdatedAt.IsZero() {
		diagnostic["updatedAt"] = state.UpdatedAt.UTC().Format(time.RFC3339)
	}

	return diagnostic
}

// setupOAuth registers all OAuth 2.0 endpoints (discovery, authorization flow,
// token management, DPoP, client registration, PAR) and starts the token
// cleanup worker.
func setupOAuth(r *chi.Mux, cfg *config.Config, svc *services, bg *backgroundServices) {
	oauthSigningKey, _ := oauth.GenerateDPoPKeyPair()
	if cfg.OAuthSigningKey != "" {
		if key, err := oauth.ParseDPoPKeyPair(cfg.OAuthSigningKey); err == nil {
			oauthSigningKey = key
		} else {
			slog.Warn("Failed to parse OAuth signing key, using ephemeral key", "error", err)
		}
	}

	oauthHandlers := server.NewOAuthHandlers(server.OAuthHandlerConfig{
		ExternalBaseURL:             cfg.ExternalBaseURL,
		ClientID:                    cfg.ExternalBaseURL + "/oauth-client-metadata.json",
		CallbackURL:                 cfg.ExternalBaseURL + "/oauth/callback",
		SigningKey:                  oauthSigningKey,
		Issuer:                      cfg.ExternalBaseURL,
		ScopesSupported:             []string{"atproto", "transition:generic", "transition:chat.bsky"},
		AccessTokenExpiration:       3600,    // 1 hour
		RefreshTokenExpiration:      1209600, // 14 days
		AuthorizationCodeExpiration: 600,     // 10 minutes
	}, svc.db)

	// Discovery endpoints
	r.Get("/.well-known/oauth-authorization-server", oauthHandlers.HandleAuthorizationServerMetadata)
	r.Get("/.well-known/oauth-protected-resource", oauthHandlers.HandleProtectedResourceMetadata)

	// Client metadata (this server as an OAuth client)
	r.Get("/oauth-client-metadata.json", server.HandleClientMetadata(server.ClientMetadataConfig{
		ExternalBaseURL: cfg.ExternalBaseURL,
		ClientName:      "Hyperindex",
		Scope:           "atproto transition:generic",
	}))

	// OAuth flow endpoints
	r.Get("/oauth/authorize", oauthHandlers.HandleAuthorize)
	r.Post("/oauth/authorize", oauthHandlers.HandleAuthorize)
	r.Get("/oauth/callback", oauthHandlers.HandleCallback)
	r.Post("/oauth/token", oauthHandlers.HandleToken)
	r.Get("/oauth/jwks", oauthHandlers.HandleJWKS)
	r.Post("/oauth/revoke", oauthHandlers.HandleRevoke)

	// Additional OAuth endpoints
	registerHandler := server.NewOAuthRegisterHandler(svc.db)
	r.Post("/oauth/register", registerHandler.HandleRegister)

	parHandler := server.NewOAuthPARHandler(svc.db)
	r.Post("/oauth/par", parHandler.HandlePAR)

	r.Get("/oauth/dpop/nonce", server.HandleDPoPNonce)
	r.Post("/oauth/dpop/nonce", server.HandleDPoPNonce)

	// Start cleanup worker
	oauthCleanupCtx, oauthCleanupCancel := context.WithCancel(context.Background())
	bg.oauthCleanupCancel = oauthCleanupCancel
	oauthHandlers.StartCleanupWorker(oauthCleanupCtx, 1*time.Hour)

	slog.Info("OAuth endpoints enabled",
		"authorization_server", cfg.ExternalBaseURL+"/.well-known/oauth-authorization-server",
		"client_metadata", cfg.ExternalBaseURL+"/oauth-client-metadata.json",
	)
}

// setupAdmin creates the admin GraphQL handler with backfill callbacks and
// registers admin routes + GraphiQL playgrounds. Returns the handler (or nil
// if setup fails) so callers can wire up the lexicon change callback.
func setupAdmin(r *chi.Mux, cfg *config.Config, svc *services) *admin.Handler {
	adminRepos := &admin.Repositories{
		Records:          svc.records,
		Actors:           svc.actors,
		Lexicons:         svc.lexicons,
		Config:           svc.config,
		OAuthClients:     svc.oauthClients,
		Activity:         svc.activity,
		Labels:           svc.labels,
		LabelDefinitions: svc.labelDefinitions,
		LabelPreferences: svc.labelPreferences,
		Reports:          svc.reports,
	}

	authMiddleware := oauth.NewAuthMiddleware(
		repositories.NewOAuthAccessTokensRepository(svc.db),
		repositories.NewOAuthDPoPJTIRepository(svc.db),
		cfg.ExternalBaseURL,
	)

	domainDID := cfg.DomainDID
	if domainDID == "" {
		domainDID = "did:web:" + cfg.Host
	}

	adminHandler, err := admin.NewHandler(adminRepos, authMiddleware, domainDID, cfg.AdminAPIKey, admin.ParseAdminDIDs(cfg.AdminDIDs))
	if err != nil {
		slog.Error("Failed to create admin GraphQL handler", "error", err)
		return nil
	}

	adminHandler.Resolver().SetLabelerSubscribeConfig(cfg.LabelerSubscribeEnabled, cfg.LabelerSubscribeURLs)

	// Wire up backfill callbacks for the admin UI
	configureBackfillCallbacks(adminHandler, cfg, svc)

	// Admin endpoint with optional auth (allows introspection without auth)
	r.Handle("/admin/graphql", adminHandler.OptionalAuth())
	r.Handle("/admin/graphql/", adminHandler.OptionalAuth())
	slog.Info("Admin GraphQL endpoint enabled", "path", "/admin/graphql")

	// GraphiQL playgrounds
	r.Get("/graphiql", server.HandleGraphiQL(server.GraphiQLConfig{
		Endpoint:             cfg.ExternalBaseURL + "/graphql",
		SubscriptionEndpoint: strings.Replace(cfg.ExternalBaseURL, "http", "ws", 1) + "/graphql/ws",
		Title:                "Hyperindex GraphQL",
		DefaultQuery: `# Hyperindex GraphQL API
# 
# Explore the AT Protocol data indexed by this AppView.
# Try querying for records from your configured lexicons.
#
# Example:
{
  __schema {
    types {
      name
    }
  }
}
`,
	}))

	r.Get("/graphiql/admin", server.HandleGraphiQL(server.GraphiQLConfig{
		Endpoint: cfg.ExternalBaseURL + "/admin/graphql",
		Title:    "Hyperindex Admin",
		DefaultQuery: `# Hyperindex Admin API
#
# Administrative operations for managing the AppView.
# Note: Some operations require authentication.
#
# Example:
{
  statistics {
    recordCount
    actorCount
    lexiconCount
  }
}
`,
	}))

	slog.Info("GraphiQL playgrounds enabled",
		"public", cfg.ExternalBaseURL+"/graphiql",
		"admin", cfg.ExternalBaseURL+"/graphiql/admin",
	)

	return adminHandler
}

// configureBackfillCallbacks sets up single-actor and full-network backfill
// callbacks on the admin handler's resolver, used by the admin UI.
func configureBackfillCallbacks(adminHandler *admin.Handler, cfg *config.Config, svc *services) {
	bfConfig := backfill.NewConfigFromApp(cfg)
	if bfConfig.Collections == nil {
		bfConfig.Collections = atproto.ParseCollections(cfg.JetstreamCollections)
	}

	// Single actor backfill
	adminHandler.Resolver().SetBackfillCallback(func(ctx context.Context, did string) error {
		actorBackfiller := backfill.NewBackfiller(bfConfig, svc.records, svc.actors, svc.activity, svc.validator)
		defer actorBackfiller.Close()
		_, err := actorBackfiller.BackfillActor(ctx, did)
		return err
	})

	// Full network backfill (runs in background)
	adminHandler.Resolver().SetFullBackfillCallback(func(ctx context.Context) error {
		collections := bfConfig.Collections
		if len(collections) == 0 {
			lexicons, err := svc.lexicons.GetAll(ctx)
			if err != nil {
				slog.Error("[backfill] Failed to get lexicons", "error", err)
				return err
			}
			for _, lex := range lexicons {
				collections = append(collections, lex.ID)
			}
		}

		if len(collections) == 0 {
			slog.Warn("[backfill] No collections configured - register lexicons first or set BACKFILL_COLLECTIONS")
			return nil
		}

		fullConfig := bfConfig
		fullConfig.Collections = collections
		bf := backfill.NewBackfiller(fullConfig, svc.records, svc.actors, svc.activity, svc.validator)
		defer bf.Close()

		slog.Info("[backfill] Starting full network backfill", "collections", collections)
		stats, err := bf.Run(ctx)
		if err != nil {
			slog.Error("[backfill] Full backfill failed", "error", err)
			return err
		}
		slog.Info("[backfill] Full backfill completed",
			"repos_discovered", stats.ReposDiscovered,
			"repos_processed", stats.ReposProcessed,
			"records_inserted", stats.RecordsInserted,
			"duration", stats.Duration(),
		)
		return nil
	})

	slog.Info("Backfill callbacks configured for admin UI")
}

// setupGraphQL loads lexicons from disk and database, creates the public GraphQL
// handler with WebSocket subscriptions, and returns the resolved collection list
// for Jetstream configuration.
func setupGraphQL(r *chi.Mux, cfg *config.Config, svc *services, pubsub *subscription.PubSub, adminHandler *admin.Handler) ([]string, error) {
	// Load lexicons from filesystem
	registry := lexicon.NewRegistry()
	lexiconDir := cfg.LexiconDir
	if lexiconDir == "" {
		lexiconDir = "testdata/lexicons"
	}

	lexiconHashes := make(map[string]string)
	if _, err := os.Stat(lexiconDir); err == nil {
		loadedHashes, err := loadLexiconsFromDir(lexiconDir, registry)
		if err != nil {
			return nil, fmt.Errorf("failed to load lexicons from directory %s: %w", lexiconDir, err)
		}
		for collection, hash := range loadedHashes {
			lexiconHashes[collection] = hash
		}
		slog.Info("Loaded lexicons from directory", "count", len(loadedHashes), "dir", lexiconDir)
	}

	// Load lexicons from database (uploaded via admin UI)
	ctx := context.Background()
	dbLexicons, err := svc.lexicons.GetAll(ctx)
	if err != nil {
		slog.Warn("Failed to load lexicons from database", "error", err)
	} else if len(dbLexicons) > 0 {
		dbLoaded := 0
		for _, dbLex := range dbLexicons {
			if _, err := registry.ParseAndRegister(dbLex.JSON); err != nil {
				return nil, fmt.Errorf("failed to parse database lexicon %s: %w", dbLex.ID, err)
			}
			lexiconHashes[dbLex.ID] = validation.HashLexiconJSON([]byte(dbLex.JSON))
			dbLoaded++
		}
		slog.Info("Loaded lexicons from database", "count", dbLoaded, "total", len(dbLexicons))
	}

	slog.Info("Total lexicons registered", "count", registry.Count())

	svc.validator = validation.NewValidator(registry, lexiconHashes)
	svc.validationRefresh = validationrefresh.NewScheduler(svc.records, svc.validator)
	if adminHandler != nil {
		adminHandler.Resolver().SetValidationRefresh(registry, svc.validator, svc.validationRefresh)
	}
	if err := svc.validationRefresh.RefreshCollections(ctx, registry.GetAllRecordRefs(), "startup"); err != nil {
		return nil, fmt.Errorf("startup validation refresh failed: %w", err)
	}

	// Create GraphQL handler
	repos := &resolver.Repositories{
		Records:        svc.records,
		Actors:         svc.actors,
		Lexicons:       svc.lexicons,
		ExternalLabels: svc.externalLabels,
	}

	graphqlHandler, err := hgraphql.NewHandler(registry, repos)
	if err != nil {
		return nil, fmt.Errorf("failed to create GraphQL handler: %w", err)
	}
	r.Handle("/graphql", graphqlHandler)
	r.Handle("/graphql/", graphqlHandler)
	slog.Info("GraphQL endpoint enabled", "path", "/graphql")

	// WebSocket subscription endpoint
	var allowedOrigins []string
	if cfg.AllowedOrigins != "" {
		allowedOrigins = strings.Split(cfg.AllowedOrigins, ",")
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
	}
	subscriptionHandler := subscription.NewHandler(graphqlHandler.Schema(), pubsub, allowedOrigins)
	r.Handle("/graphql/ws", subscriptionHandler)
	slog.Info("GraphQL subscriptions enabled", "path", "/graphql/ws")

	// Resolve collections for Jetstream
	var collections []string
	if cfg.JetstreamCollections != "" {
		collections = atproto.ParseCollections(cfg.JetstreamCollections)
	} else {
		for _, lex := range dbLexicons {
			collections = append(collections, lex.ID)
		}
	}

	return collections, nil
}

// startWorkers launches background worker goroutines (activity cleanup).
func startWorkers(svc *services, bg *backgroundServices) {
	activityCleanupWorker := workers.NewActivityCleanupWorker(svc.activity)
	workersCtx, workersCancel := context.WithCancel(context.Background())
	bg.workersCancel = workersCancel
	activityCleanupWorker.Start(workersCtx)
}

func startLabelerSubscribers(cfg *config.Config, svc *services, bg *backgroundServices) {
	urls := configuredLabelerSubscribeURLs(context.Background(), cfg, svc.config)
	if !cfg.LabelerSubscribeEnabled || len(urls) == 0 {
		if cfg.LabelerSubscribeEnabled && len(urls) == 0 {
			slog.Info("Labeler subscriptions disabled (LABELER_SUBSCRIBE_URLS empty)")
		}
		return
	}

	labelerCtx, labelerCancel := context.WithCancel(context.Background())
	bg.labelerCancel = labelerCancel

	subscriber := labeler.NewSubscriber(svc.externalLabels, labeler.Config{
		URLs:         urls,
		ReconnectMin: cfg.LabelerSubscribeReconnectMin,
		ReconnectMax: cfg.LabelerSubscribeReconnectMax,
	})
	subscriber.Start(labelerCtx)

	slog.Info("Labeler subscriptions started", "urls", logsafe.URLs(urls))
}

// startJetstream creates and starts the Jetstream consumer for real-time AT Protocol
// events. It also wires up the lexicon change callback on the admin handler so that
// adding/removing lexicons dynamically updates the consumer's collection filter.
func startJetstream(
	cfg *config.Config,
	svc *services,
	pubsub *subscription.PubSub,
	collections []string,
	adminHandler *admin.Handler,
	bg *backgroundServices,
) {
	jsURL := cfg.JetstreamURL
	if jsURL == "" {
		jsURL = jetstream.DefaultJetstreamURL
	}

	if len(collections) > 0 {
		bg.jsConsumer = jetstream.NewConsumer(
			jetstream.ConsumerConfig{
				JetstreamURL:  jsURL,
				Collections:   collections,
				DisableCursor: cfg.JetstreamDisableCursor,
			},
			svc.records,
			svc.actors,
			svc.config,
			svc.activity,
			pubsub,
			svc.validator,
		)

		jsCtx, jsCancel := context.WithCancel(context.Background())
		bg.jsCancel = jsCancel

		go func() {
			slog.Info("Starting Jetstream consumer",
				"url", jsURL,
				"collections", collections,
				"disable_cursor", cfg.JetstreamDisableCursor,
			)
			if err := bg.jsConsumer.Start(jsCtx); err != nil {
				slog.Error("Jetstream consumer error", "error", err)
			}
		}()
	} else {
		slog.Info("Jetstream consumer disabled (no collections - register lexicons or set JETSTREAM_COLLECTIONS)")
	}

	// Wire up lexicon change callback for dynamic Jetstream updates
	if adminHandler != nil {
		adminHandler.Resolver().SetLexiconChangeCallback(func(updatedCollections []string) error {
			if bg.jsConsumer == nil {
				bg.jsConsumer = jetstream.NewConsumer(
					jetstream.ConsumerConfig{
						JetstreamURL:  jsURL,
						Collections:   updatedCollections,
						DisableCursor: cfg.JetstreamDisableCursor,
					},
					svc.records,
					svc.actors,
					svc.config,
					svc.activity,
					pubsub,
					svc.validator,
				)

				go func() {
					slog.Info("Starting Jetstream consumer (dynamic)",
						"collections", updatedCollections,
					)
					if err := bg.jsConsumer.Start(context.Background()); err != nil {
						slog.Error("Jetstream consumer error", "error", err)
					}
				}()
				return nil
			}
			return bg.jsConsumer.UpdateCollections(updatedCollections)
		})
		slog.Info("Lexicon change callback configured for dynamic Jetstream updates")
	}
}

// startBackfill runs the initial backfill in the background if BACKFILL_ON_START is set.
func startBackfill(cfg *config.Config, svc *services) {
	if !cfg.BackfillOnStart {
		return
	}

	bfConfig := backfill.NewConfigFromApp(cfg)
	if bfConfig.Collections == nil {
		bfConfig.Collections = atproto.ParseCollections(cfg.JetstreamCollections)
	}

	if len(bfConfig.Collections) == 0 {
		slog.Warn("BACKFILL_ON_START=true but no collections specified")
		return
	}

	backfiller := backfill.NewBackfiller(bfConfig, svc.records, svc.actors, svc.activity, svc.validator)

	go func() {
		slog.Info("Starting backfill operation",
			"collections", bfConfig.Collections,
			"relay", bfConfig.RelayURL,
		)
		stats, err := backfiller.Run(context.Background())
		if err != nil {
			slog.Error("Backfill failed", "error", err)
		} else {
			slog.Info("Backfill completed",
				"repos_discovered", stats.ReposDiscovered,
				"repos_processed", stats.ReposProcessed,
				"records_inserted", stats.RecordsInserted,
				"duration", stats.Duration(),
			)
		}
	}()
}

// startTap creates and starts the Tap consumer as a replacement for Jetstream+Backfill.
// It also wires admin backfill callbacks to use Tap's /repos/add HTTP API.
func startTap(
	cfg *config.Config,
	svc *services,
	pubsub *subscription.PubSub,
	adminHandler *admin.Handler,
	bg *backgroundServices,
) {
	tapURL := cfg.TapURL

	// Create handler that stores records and publishes to subscriptions.
	handler := tap.NewIndexHandler(svc.records, svc.actors, svc.activity, pubsub, svc.validator)

	// Create consumer.
	consumer := tap.NewConsumer(tap.ConsumerConfig{
		TapURL:      tapURL,
		Password:    cfg.TapAdminPassword,
		DisableAcks: cfg.TapDisableAcks,
	}, handler)

	// Store consumer reference for clean shutdown.
	bg.tapConsumer = consumer

	tapCtx, tapCancel := context.WithCancel(context.Background())
	bg.tapCancel = tapCancel

	go func() {
		slog.Info("Starting Tap consumer", "url", tapURL, "disable_acks", cfg.TapDisableAcks)
		if err := consumer.Start(tapCtx); err != nil {
			slog.Error("Tap consumer error", "error", err)
		}
	}()

	// Create a shared admin client for stats checks and backfill callbacks.
	tapHTTPURL := strings.Replace(strings.Replace(tapURL, "ws://", "http://", 1), "wss://", "https://", 1)
	adminClient := tap.NewAdminClient(tapHTTPURL, cfg.TapAdminPassword)
	bg.tapAdminClient = adminClient

	// Wire admin backfill callbacks to use Tap's /repos/add API.
	if adminHandler != nil {
		adminHandler.Resolver().SetBackfillCallback(func(ctx context.Context, did string) error {
			return adminClient.AddRepos(ctx, []string{did})
		})

		adminHandler.Resolver().SetFullBackfillCallback(func(_ context.Context) error {
			return fmt.Errorf("full network backfill not supported via Tap admin API — configure TAP_SIGNAL_COLLECTION or TAP_FULL_NETWORK on the Tap sidecar instead")
		})
	}

	slog.Info("Tap consumer started (replaces Jetstream + Backfill)")
}

// serve starts the HTTP server and blocks until a shutdown signal is received,
// then performs a graceful shutdown with a 30-second timeout.
func serve(r *chi.Mux, cfg *config.Config, bg *backgroundServices) error {
	srv := &http.Server{
		Addr:        cfg.Address(),
		Handler:     r,
		ReadTimeout: 15 * time.Second,
		// WriteTimeout disabled (set to 0) to support long-lived WebSocket connections.
		// Individual handlers enforce their own write deadlines:
		// - WebSocket: per-message deadline in subscription/handler.go
		// - HTTP: standard response lifecycle
		WriteTimeout: 0,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Server listening", "address", cfg.Address(), "url", cfg.ExternalBaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	// Wait for interrupt signal or server error
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return err
	case <-quit:
	}

	slog.Info("Shutting down server...")

	// Stop background services
	bg.Stop()

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}

	slog.Info("Server stopped gracefully")
	return nil
}

// loadLexiconsFromDir loads all lexicon JSON files from a directory tree.
func loadLexiconsFromDir(dir string, registry *lexicon.Registry) (map[string]string, error) {
	hashes := make(map[string]string)
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		lex, parseErr := lexicon.ParseBytes(data)
		if parseErr != nil {
			return fmt.Errorf("failed to parse lexicon file %s: %w", path, parseErr)
		}

		registry.Register(lex)
		hashes[lex.ID] = validation.HashLexiconJSON(data)
		return nil
	})
	return hashes, err
}

// populateActivityFromRecords creates activity entries from existing records.
func populateActivityFromRecords(
	ctx context.Context,
	recordsRepo *repositories.RecordsRepository,
	activityRepo *repositories.IndexingActivityRepository,
) (int64, error) {
	var count int64
	_, err := recordsRepo.IterateAll(ctx, 1000, func(rec *repositories.Record) error {
		// Extract createdAt from the record JSON, fall back to IndexedAt
		timestamp := atproto.ExtractCreatedAt(rec.JSON, rec.IndexedAt)

		// Log as a successful create operation
		if _, logErr := activityRepo.LogActivityWithStatus(ctx, timestamp, "create", rec.Collection, rec.DID, rec.RKey, rec.JSON, "success"); logErr == nil {
			count++
		}
		return nil
	})
	return count, err
}
