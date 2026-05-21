package graphql

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	graphqlgo "github.com/graphql-go/graphql"

	"github.com/GainForest/hyperindex/internal/database/repositories"
	"github.com/GainForest/hyperindex/internal/graphql/resolver"
	"github.com/GainForest/hyperindex/internal/graphql/schema"
	"github.com/GainForest/hyperindex/internal/lexicon"
)

// DefaultLexiconDir is the filesystem source used when no explicit lexicon
// directory is configured.
const DefaultLexiconDir = "testdata/lexicons"

// SchemaProvider exposes the active public GraphQL schema to request handlers.
// Implementations return nil when no schema has loaded successfully yet.
type SchemaProvider interface {
	// Schema returns the currently active schema, or nil when none is available.
	Schema() *graphqlgo.Schema
}

// PublicSchemaManagerConfig configures filesystem sources for public schema reloads.
type PublicSchemaManagerConfig struct {
	// LexiconDir is the directory tree containing filesystem lexicon JSON files.
	// When empty, DefaultLexiconDir is used to preserve existing startup behavior.
	LexiconDir string
}

// PublicSchemaSourceCounts records how many lexicons contributed to a schema snapshot.
type PublicSchemaSourceCounts struct {
	// Filesystem is the number of lexicon JSON files loaded from the filesystem.
	Filesystem int

	// Database is the number of lexicon rows loaded from the database.
	Database int

	// Registered is the number of unique lexicon IDs registered in the active schema.
	Registered int
}

// PublicSchemaSnapshot is an immutable view of the currently active public schema.
// A zero-value snapshot means no public schema has successfully loaded yet.
type PublicSchemaSnapshot struct {
	// Schema is the active public GraphQL schema. It is nil until a reload succeeds.
	Schema *graphqlgo.Schema

	// LexiconCount is the number of unique lexicon IDs in Schema.
	LexiconCount int

	// ReloadedAt is the time the active schema snapshot was successfully built.
	ReloadedAt time.Time

	// SourceCounts describes filesystem, database, and registered lexicon counts.
	SourceCounts PublicSchemaSourceCounts
}

// ReloadSchemaResult describes the outcome of a public schema reload attempt.
type ReloadSchemaResult struct {
	// Success is true when the live public schema snapshot was replaced.
	Success bool

	// LexiconCount is the active schema's lexicon count after the attempt. On
	// failure this is the previous working schema count, or zero if none exists.
	LexiconCount int

	// ReloadedAt is set to the active schema timestamp after a successful reload.
	ReloadedAt *time.Time

	// Error explains why reload failed and what an operator should fix next.
	Error string
}

type lexiconSource interface {
	GetAll(ctx context.Context) ([]*repositories.Lexicon, error)
}

// PublicSchemaManager rebuilds and atomically swaps the live public GraphQL schema.
// It preserves the previous working snapshot whenever a reload attempt fails.
type PublicSchemaManager struct {
	config   PublicSchemaManagerConfig
	lexicons lexiconSource

	reloadMu sync.Mutex
	snapshot atomic.Value // stores PublicSchemaSnapshot
}

// NewPublicSchemaManager creates a manager for reloading the public GraphQL schema.
// The manager can exist before any schema has loaded; call Reload to build the
// first snapshot. The provided repositories must include Lexicons before Reload
// can load database lexicons.
func NewPublicSchemaManager(cfg PublicSchemaManagerConfig, repos *resolver.Repositories) *PublicSchemaManager {
	var lexicons lexiconSource
	if repos != nil && repos.Lexicons != nil {
		lexicons = repos.Lexicons
	}

	return &PublicSchemaManager{
		config:   cfg,
		lexicons: lexicons,
	}
}

// Reload rebuilds the public schema from filesystem and database lexicons.
// Expected load, parse, validation, and schema-build failures return a result
// with Success=false and keep the previous snapshot active. Programmer wiring
// errors, such as a missing lexicon repository, are returned as Go errors.
func (m *PublicSchemaManager) Reload(ctx context.Context) (*ReloadSchemaResult, error) {
	if m == nil {
		return nil, fmt.Errorf("reload public GraphQL schema: schema manager is nil; construct a PublicSchemaManager before wiring reload")
	}
	if m.lexicons == nil {
		return nil, fmt.Errorf("reload public GraphQL schema: lexicon repository is not configured; pass resolver.Repositories with Lexicons set")
	}

	m.reloadMu.Lock()
	defer m.reloadMu.Unlock()

	snapshot, err := m.buildSnapshot(ctx)
	if err != nil {
		return m.failedReloadResult(err), nil
	}

	m.snapshot.Store(*snapshot)
	reloadedAt := snapshot.ReloadedAt
	return &ReloadSchemaResult{
		Success:      true,
		LexiconCount: snapshot.LexiconCount,
		ReloadedAt:   &reloadedAt,
	}, nil
}

// Snapshot returns the current public schema snapshot. The zero value means no
// schema has loaded successfully yet.
func (m *PublicSchemaManager) Snapshot() PublicSchemaSnapshot {
	if m == nil {
		return PublicSchemaSnapshot{}
	}

	value := m.snapshot.Load()
	if value == nil {
		return PublicSchemaSnapshot{}
	}

	return value.(PublicSchemaSnapshot)
}

// Schema returns the currently active public GraphQL schema, or nil if no schema
// has loaded successfully yet.
func (m *PublicSchemaManager) Schema() *graphqlgo.Schema {
	return m.Snapshot().Schema
}

// LexiconCount returns the active schema's lexicon count, or zero if no schema
// has loaded successfully yet.
func (m *PublicSchemaManager) LexiconCount() int {
	return m.Snapshot().LexiconCount
}

// ParseAndValidateLexiconDocument parses a raw lexicon JSON document and verifies
// its document ID when expectedID is provided. The sourceDescription should name
// the operator-visible source, such as `filesystem lexicon "path"` or
// `database lexicon "app.example.post"`, so returned errors explain exactly what
// to fix before storing or reloading the lexicon.
func ParseAndValidateLexiconDocument(sourceDescription, jsonData, expectedID string) (*lexicon.Lexicon, error) {
	sourceDescription = strings.TrimSpace(sourceDescription)
	if sourceDescription == "" {
		sourceDescription = "lexicon document"
	}

	parsed, err := lexicon.Parse(jsonData)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", sourceDescription, err)
	}

	if expectedID != "" && parsed.ID != expectedID {
		return nil, fmt.Errorf("validate %s: source id %q does not match document id %q; use matching lexicon IDs before storing or reloading", sourceDescription, expectedID, parsed.ID)
	}

	return parsed, nil
}

// LoadLexiconsFromDirStrict loads lexicon JSON files from a directory tree.
// Malformed lexicon documents fail the load instead of being silently skipped.
// A missing directory returns zero lexicons so database-only deployments can run
// without a filesystem lexicon source.
func LoadLexiconsFromDirStrict(dir string) ([]*lexicon.Lexicon, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, nil
	}

	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect lexicon directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("lexicon path %q is not a directory; set LEXICON_DIR to a directory or remove the invalid path", dir)
	}

	lexicons := make([]*lexicon.Lexicon, 0)
	if err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("walk lexicon path %q: %w", path, walkErr)
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read filesystem lexicon %q: %w", path, err)
		}

		parsed, err := ParseAndValidateLexiconDocument(fmt.Sprintf("filesystem lexicon %q", path), string(data), "")
		if err != nil {
			return err
		}

		lexicons = append(lexicons, parsed)
		return nil
	}); err != nil {
		return nil, err
	}

	return lexicons, nil
}

func (m *PublicSchemaManager) buildSnapshot(ctx context.Context) (*PublicSchemaSnapshot, error) {
	registry := lexicon.NewRegistry()
	counts := PublicSchemaSourceCounts{}

	filesystemLexicons, err := LoadLexiconsFromDirStrict(m.lexiconDir())
	if err != nil {
		return nil, fmt.Errorf("load filesystem lexicons: %w", err)
	}
	counts.Filesystem = len(filesystemLexicons)
	for _, parsed := range filesystemLexicons {
		replaceLexicon(registry, parsed)
	}

	databaseLexicons, err := m.lexicons.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("load database lexicons: %w", err)
	}
	counts.Database = len(databaseLexicons)
	for _, stored := range databaseLexicons {
		parsed, err := ParseAndValidateLexiconDocument(fmt.Sprintf("database lexicon %q", stored.ID), stored.JSON, stored.ID)
		if err != nil {
			return nil, err
		}
		replaceLexicon(registry, parsed)
	}

	builder := schema.NewBuilder(registry)
	builtSchema, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("build public GraphQL schema from %d registered lexicons: %w", registry.Count(), err)
	}

	counts.Registered = registry.Count()
	return &PublicSchemaSnapshot{
		Schema:       builtSchema,
		LexiconCount: registry.Count(),
		ReloadedAt:   time.Now().UTC(),
		SourceCounts: counts,
	}, nil
}

func (m *PublicSchemaManager) lexiconDir() string {
	if strings.TrimSpace(m.config.LexiconDir) == "" {
		return DefaultLexiconDir
	}
	return m.config.LexiconDir
}

func (m *PublicSchemaManager) failedReloadResult(err error) *ReloadSchemaResult {
	return &ReloadSchemaResult{
		Success:      false,
		LexiconCount: m.LexiconCount(),
		Error:        fmt.Sprintf("%v. Fix the lexicon source and reload the public GraphQL schema again.", err),
	}
}

func replaceLexicon(registry *lexicon.Registry, parsed *lexicon.Lexicon) {
	if _, ok := registry.GetLexicon(parsed.ID); ok {
		registry.Unregister(parsed.ID)
	}
	registry.Register(parsed)
}
