// Package main is the entry point for the Hypergoat server.
//
// Hypergoat is a Go implementation of Quickslice - an AT Protocol AppView server
// that indexes Lexicon-defined records and exposes them via a dynamically-generated
// GraphQL API.
//
// For more information, see the README.md file.
package main

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/GainForest/hypergoat/internal/atproto"
	"github.com/GainForest/hypergoat/internal/backfill"
	"github.com/GainForest/hypergoat/internal/config"
	"github.com/GainForest/hypergoat/internal/database"
	"github.com/GainForest/hypergoat/internal/database/migrations"
	"github.com/GainForest/hypergoat/internal/database/repositories"
	hgraphql "github.com/GainForest/hypergoat/internal/graphql"
	"github.com/GainForest/hypergoat/internal/graphql/admin"
	"github.com/GainForest/hypergoat/internal/graphql/resolver"
	"github.com/GainForest/hypergoat/internal/graphql/subscription"
	"github.com/GainForest/hypergoat/internal/jetstream"
	"github.com/GainForest/hypergoat/internal/lexicon"
	"github.com/GainForest/hypergoat/internal/oauth"
	"github.com/GainForest/hypergoat/internal/server"
	"github.com/GainForest/hypergoat/internal/workers"
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
	db               database.Executor
	records          *repositories.RecordsRepository
	actors           *repositories.ActorsRepository
	lexicons         *repositories.LexiconsRepository
	config           *repositories.ConfigRepository
	activity         *repositories.JetstreamActivityRepository
	oauthClients     *repositories.OAuthClientsRepository
	labels           *repositories.LabelsRepository
	labelDefinitions *repositories.LabelDefinitionsRepository
	labelPreferences *repositories.LabelPreferencesRepository
	reports          *repositories.ReportsRepository
}

// backgroundServices tracks cancellable background goroutines for clean shutdown.
type backgroundServices struct {
	oauthCleanupCancel context.CancelFunc
	workersCancel      context.CancelFunc
	jsConsumer         *jetstream.Consumer
	jsCancel           context.CancelFunc
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
}

func run() error {
	// Set up structured logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("Starting Hypergoat - AT Protocol AppView Server")

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

	// Set up HTTP router with middleware and basic endpoints
	r := setupRouter(cfg, svc)

	// Track background services for clean shutdown
	bg := &backgroundServices{}
	defer bg.Stop()

	// Set up OAuth endpoints
	setupOAuth(r, cfg, svc, bg)

	// Set up admin GraphQL endpoint with backfill callbacks
	adminHandler := setupAdmin(r, cfg, svc)

	// Load lexicons and set up public GraphQL + subscriptions
	pubsub := subscription.NewPubSub()
	collections := setupGraphQL(r, cfg, svc, pubsub)

	// Start background workers (activity cleanup)
	startWorkers(svc, bg)

	// Start Jetstream consumer for real-time events
	startJetstream(cfg, svc, pubsub, collections, adminHandler, bg)

	// Start backfill if configured
	startBackfill(cfg, svc)

	// Run HTTP server with graceful shutdown
	return serve(r, cfg, bg)
}

// initServices connects to the database, runs migrations, and creates all
// repository instances. JetstreamActivityRepository is created once here
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
		activity:         repositories.NewJetstreamActivityRepository(db),
		oauthClients:     repositories.NewOAuthClientsRepository(db),
		labels:           repositories.NewLabelsRepository(db),
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

	if adminDIDs := cfg.AdminDIDs; adminDIDs != "" {
		existingAdmins := svc.config.GetAdminDIDs(ctx)
		if len(existingAdmins) == 0 {
			if err := svc.config.Set(ctx, "admin_dids", adminDIDs); err != nil {
				slog.Warn("Failed to set admin_dids from environment", "error", err)
			} else {
				slog.Info("Initialized admin DIDs from environment", "dids", adminDIDs)
			}
		}
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
// (health, stats, root info, XRPC placeholder).
func setupRouter(cfg *config.Config, svc *services) *chi.Mux {
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
	r.Use(server.CORSMiddleware(server.CORSConfig{
		AllowedOrigins: allowedOrigins,
		AdminAPIKeySet: cfg.AdminAPIKey != "",
	}))

	// Health check
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
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

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"records":  recordCount,
			"actors":   actorCount,
			"lexicons": lexiconCount,
			"time":     time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Root endpoint - server info
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name":        "Hypergoat",
			"description": "AT Protocol AppView Server",
			"version":     "0.1.0-dev",
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
		ClientName:      "Hypergoat",
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

	adminHandler, err := admin.NewHandler(adminRepos, authMiddleware, svc.config, domainDID, cfg.AdminAPIKey)
	if err != nil {
		slog.Error("Failed to create admin GraphQL handler", "error", err)
		return nil
	}

	// Wire up backfill callbacks for the admin UI
	configureBackfillCallbacks(adminHandler, cfg, svc)

	// Admin endpoint with optional auth (allows introspection without auth)
	r.Handle("/admin/graphql", adminHandler.OptionalAuth())
	r.Handle("/admin/graphql/", adminHandler.OptionalAuth())
	slog.Info("Admin GraphQL endpoint enabled", "path", "/admin/graphql")

	// GraphiQL playgrounds
	r.Get("/graphiql", server.HandleGraphiQL(server.GraphiQLConfig{
		EndpointPath:     "/graphql",
		SubscriptionPath: "/graphql/ws",
		Title:            "Hypergoat GraphQL",
		DefaultQuery: `# Hypergoat GraphQL API
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
		EndpointPath: "/admin/graphql",
		Title:        "Hypergoat Admin",
		AdminAuth:    true,
		DefaultQuery: `# Hypergoat Admin API
#
# Administrative operations for managing the AppView.
# Enter your API Key and DID above to authenticate.
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

	actorBackfiller := backfill.NewBackfiller(bfConfig, svc.records, svc.actors, svc.activity)

	// Single actor backfill
	adminHandler.Resolver().SetBackfillCallback(func(ctx context.Context, did string) error {
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
		bf := backfill.NewBackfiller(fullConfig, svc.records, svc.actors, svc.activity)
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
func setupGraphQL(r *chi.Mux, cfg *config.Config, svc *services, pubsub *subscription.PubSub) []string {
	// Load lexicons from filesystem
	registry := lexicon.NewRegistry()
	lexiconDir := cfg.LexiconDir
	if lexiconDir == "" {
		lexiconDir = "testdata/lexicons"
	}

	if _, err := os.Stat(lexiconDir); err == nil {
		if err := loadLexiconsFromDir(lexiconDir, registry); err != nil {
			slog.Warn("Failed to load lexicons from directory", "dir", lexiconDir, "error", err)
		} else {
			slog.Info("Loaded lexicons from directory", "count", registry.Count(), "dir", lexiconDir)
		}
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
				slog.Warn("Failed to parse database lexicon", "id", dbLex.ID, "error", err)
			} else {
				dbLoaded++
			}
		}
		slog.Info("Loaded lexicons from database", "count", dbLoaded, "total", len(dbLexicons))
	}

	slog.Info("Total lexicons registered", "count", registry.Count())

	// Create GraphQL handler
	repos := &resolver.Repositories{
		Records:  svc.records,
		Actors:   svc.actors,
		Lexicons: svc.lexicons,
	}

	graphqlHandler, err := hgraphql.NewHandler(registry, repos)
	if err != nil {
		slog.Error("Failed to create GraphQL handler", "error", err)
	} else {
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
	}

	// Resolve collections for Jetstream
	var collections []string
	if cfg.JetstreamCollections != "" {
		collections = atproto.ParseCollections(cfg.JetstreamCollections)
	} else {
		for _, lex := range dbLexicons {
			collections = append(collections, lex.ID)
		}
	}

	return collections
}

// startWorkers launches background worker goroutines (activity cleanup).
func startWorkers(svc *services, bg *backgroundServices) {
	activityCleanupWorker := workers.NewActivityCleanupWorker(svc.activity)
	workersCtx, workersCancel := context.WithCancel(context.Background())
	bg.workersCancel = workersCancel
	activityCleanupWorker.Start(workersCtx)
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

	backfiller := backfill.NewBackfiller(bfConfig, svc.records, svc.actors, svc.activity)

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
func loadLexiconsFromDir(dir string, registry *lexicon.Registry) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
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
			// Skip non-lexicon JSON files
			return nil //nolint:nilerr // intentionally skip parse errors
		}

		registry.Register(lex)
		return nil
	})
}

// populateActivityFromRecords creates activity entries from existing records.
func populateActivityFromRecords(
	ctx context.Context,
	recordsRepo *repositories.RecordsRepository,
	activityRepo *repositories.JetstreamActivityRepository,
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
