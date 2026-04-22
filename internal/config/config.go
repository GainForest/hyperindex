// Package config handles application configuration loading from environment variables.
package config

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all application configuration.
type Config struct {
	// Server configuration
	Host string
	Port int

	// Database
	DatabaseURL string

	// Security
	SecretKeyBase  string
	AllowedOrigins string // Comma-separated allowed WebSocket/CORS origins (empty = same-origin only, "*" = allow all)

	// OAuth
	ExternalBaseURL   string
	OAuthSigningKey   string
	OAuthLoopbackMode bool

	// Admin
	AdminDIDs   string // Comma-separated list of admin DIDs
	DomainDID   string // Domain DID for identity
	AdminAPIKey string // API key for admin API access

	// Lexicons
	LexiconDir string // Directory to load lexicon JSON files from

	// Jetstream
	JetstreamURL           string // Jetstream WebSocket URL
	JetstreamCollections   string // Comma-separated collections to subscribe to
	JetstreamDisableCursor bool   // Disable cursor-based resume

	// Backfill
	BackfillOnStart           bool   // Run backfill on server start
	BackfillCollections       string // Comma-separated collections to backfill (defaults to JetstreamCollections)
	BackfillRelayURL          string // AT Protocol relay URL
	BackfillPLCURL            string // PLC directory URL
	BackfillPDSConcurrency    int
	BackfillMaxPDSWorkers     int
	BackfillMaxHTTPConcurrent int
	BackfillMaxPerPDS         int
	BackfillMaxRepos          int
	BackfillRepoTimeoutMS     int

	// Tap (replaces Jetstream + Backfill)
	TapURL           string // Tap WebSocket URL (default: ws://localhost:2480)
	TapAdminPassword string // Tap admin API password for Basic auth
	TapDisableAcks   bool   // Fire-and-forget mode (default: false)
	TapEnabled       bool   // Use Tap instead of Jetstream+Backfill (default: false)

	// PLC Directory
	PLCDirectoryURL string // PLC directory URL for DID resolution
}

// Load reads configuration from environment variables.
// It loads .env file if present and applies defaults.
func Load() (*Config, error) {
	// Load .env file if it exists (ignore error if not found)
	_ = godotenv.Load()

	cfg := &Config{
		// Server
		Host: getEnv("HOST", "127.0.0.1"),
		Port: getEnvInt("PORT", 8080),

		// Database
		DatabaseURL: getEnv("DATABASE_URL", "sqlite:data/hypergoat.db"),

		// Security
		SecretKeyBase:  getEnv("SECRET_KEY_BASE", ""),
		AllowedOrigins: getEnv("ALLOWED_ORIGINS", ""),

		// OAuth
		ExternalBaseURL:   getEnv("EXTERNAL_BASE_URL", ""),
		OAuthSigningKey:   getEnv("OAUTH_SIGNING_KEY", ""),
		OAuthLoopbackMode: getEnvBool("OAUTH_LOOPBACK_MODE", false),

		// Admin
		AdminDIDs:   getEnv("ADMIN_DIDS", ""),
		DomainDID:   getEnv("DOMAIN_DID", ""),
		AdminAPIKey: getEnv("ADMIN_API_KEY", ""),

		// Lexicons
		LexiconDir: getEnv("LEXICON_DIR", ""),

		// Jetstream
		JetstreamURL:           getEnv("JETSTREAM_URL", ""),
		JetstreamCollections:   getEnv("JETSTREAM_COLLECTIONS", ""),
		JetstreamDisableCursor: getEnvBool("JETSTREAM_DISABLE_CURSOR", false),

		// Backfill
		BackfillOnStart:           getEnvBool("BACKFILL_ON_START", false),
		BackfillCollections:       getEnv("BACKFILL_COLLECTIONS", ""),
		BackfillRelayURL:          getEnv("BACKFILL_RELAY_URL", ""),
		BackfillPLCURL:            getEnv("BACKFILL_PLC_URL", ""),
		BackfillPDSConcurrency:    getEnvInt("BACKFILL_PDS_CONCURRENCY", 4),
		BackfillMaxPDSWorkers:     getEnvInt("BACKFILL_MAX_PDS_WORKERS", 10),
		BackfillMaxHTTPConcurrent: getEnvInt("BACKFILL_MAX_HTTP", 50),
		BackfillMaxPerPDS:         getEnvInt("BACKFILL_MAX_PER_PDS", 6),
		BackfillMaxRepos:          getEnvInt("BACKFILL_MAX_REPOS", 50),
		BackfillRepoTimeoutMS:     getEnvInt("BACKFILL_REPO_TIMEOUT", 60000),

		// Tap
		TapURL:           getEnv("TAP_URL", "ws://localhost:2480"),
		TapAdminPassword: getEnv("TAP_ADMIN_PASSWORD", ""),
		TapDisableAcks:   getEnvBool("TAP_DISABLE_ACKS", false),
		TapEnabled:       getEnvBool("TAP_ENABLED", false),

		// PLC Directory
		PLCDirectoryURL: getEnv("PLC_DIRECTORY_URL", ""),
	}

	// Generate SecretKeyBase if not provided
	if cfg.SecretKeyBase == "" {
		slog.Warn("SECRET_KEY_BASE not set, generating random key",
			"warning", "Sessions will be invalidated on server restart")
		key, err := generateRandomKey(64)
		if err != nil {
			return nil, fmt.Errorf("failed to generate secret key: %w", err)
		}
		cfg.SecretKeyBase = key
	}

	if cfg.AdminAPIKey != "" {
		slog.Info("ADMIN_API_KEY enabled for admin API access", "admin_api_key_set", true)
	}

	// Set default external base URL if not provided
	cfg.ExternalBaseURL = strings.TrimSpace(cfg.ExternalBaseURL)
	if cfg.ExternalBaseURL == "" {
		cfg.ExternalBaseURL = fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port)
	}

	// Normalize external base URL: add https:// if no scheme is present
	lower := strings.ToLower(cfg.ExternalBaseURL)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		cfg.ExternalBaseURL = "https://" + cfg.ExternalBaseURL
	}

	return cfg, nil
}

// Validate checks that all required configuration is present and valid.
func (c *Config) Validate() error {
	if c.AdminAPIKey == "" {
		return fmt.Errorf("ADMIN_API_KEY must be set")
	}

	if len(c.AdminAPIKey) < 16 {
		return fmt.Errorf("ADMIN_API_KEY must be at least 16 characters long")
	}

	if len(c.SecretKeyBase) < 64 {
		return fmt.Errorf("SECRET_KEY_BASE must be at least 64 characters")
	}

	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("PORT must be between 1 and 65535")
	}

	return nil
}

// LogConfig logs the configuration (with sensitive values redacted).
func (c *Config) LogConfig() {
	slog.Info("Configuration loaded",
		"host", c.Host,
		"port", c.Port,
		"database_url", RedactPassword(c.DatabaseURL),
		"external_base_url", c.ExternalBaseURL,
		"oauth_loopback_mode", c.OAuthLoopbackMode,
		"oauth_signing_key_set", c.OAuthSigningKey != "",
		"admin_dids_set", c.AdminDIDs != "",
		"admin_api_key_set", c.AdminAPIKey != "",
		"lexicon_dir", c.LexiconDir,
		"jetstream_url", c.JetstreamURL,
		"jetstream_collections", c.JetstreamCollections,
		"jetstream_disable_cursor", c.JetstreamDisableCursor,
		"backfill_on_start", c.BackfillOnStart,
		"allowed_origins", c.AllowedOrigins,
		"tap_enabled", c.TapEnabled,
		"tap_url", c.TapURL,
		"tap_admin_password_set", c.TapAdminPassword != "",
		"tap_disable_acks", c.TapDisableAcks,
	)
}

// Address returns the server address in host:port format.
func (c *Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Helper functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		lower := strings.ToLower(value)
		return lower == "true" || lower == "1" || lower == "yes"
	}
	return defaultValue
}

func generateRandomKey(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}

// RedactPassword hides the password in a database URL for logging.
// Example: postgres://user:pass@host becomes postgres://user:***@host
func RedactPassword(url string) string {
	if !strings.Contains(url, "@") {
		return url
	}

	parts := strings.SplitN(url, "@", 2)
	if len(parts) != 2 {
		return url
	}

	prefix := parts[0]
	suffix := parts[1]

	if idx := strings.LastIndex(prefix, ":"); idx > 0 {
		if protoIdx := strings.Index(prefix, "://"); protoIdx > 0 && idx > protoIdx {
			return prefix[:idx+1] + "***@" + suffix
		}
	}

	return url
}
