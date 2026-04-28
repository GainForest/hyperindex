# Hypergoat Implementation Plan

> Comprehensive porting plan from Quickslice (Gleam) to Hypergoat (Go)

## Overview

This document tracks the implementation of Hypergoat, a Go port of [Quickslice](https://github.com/quickslice/quickslice). Quickslice is an AT Protocol AppView server written in Gleam that indexes Lexicon-defined records and exposes them via a dynamically-generated GraphQL API.

**Target:** Feature parity with Quickslice while leveraging Go's performance and ecosystem.

**Estimated Timeline:** 8-10 weeks

---

## Architecture Decisions

### Technology Choices

| Component | Quickslice (Gleam) | Hypergoat (Go) | Rationale |
|-----------|-------------------|----------------|-----------|
| HTTP | wisp/mist | chi | Lightweight, idiomatic, good middleware |
| Database | sqlight/pog | pgx + modernc/sqlite | Native drivers, no CGO for SQLite |
| GraphQL | swell | graphql-go/graphql | Runtime schema generation (like Quickslice) |
| WebSocket | mist | nhooyr/websocket | Modern, context-aware |
| JSON | gleam_json | encoding/json + gjson | Standard library + fast queries |
| JWT/JOSE | jose | golang-jwt + go-jose | Industry standard libraries |
| Logging | logging | slog | Standard library (Go 1.21+) |
| Config | envoy/dotenv | caarlos0/env | Struct tags, validation |

### Concurrency Model Translation

| Gleam/Erlang Pattern | Go Equivalent |
|---------------------|---------------|
| Actor with `process.Subject` | Goroutine with `chan T` |
| `group_registry` (PubSub) | `sync.Map` + channels / custom broker |
| ETS tables | `sync.Map` or `groupcache` |
| Supervisor trees | `errgroup` or custom manager |
| `process.sleep_forever()` | `select {}` or signal handling |

---

## Phase 1: Foundation (Week 1-2) ✅ COMPLETE

### 1.1 Project Setup

- [x] Initialize Go module
- [x] Create project directory structure
- [x] Configure `.golangci.yml` for linting
- [x] Set up Makefile with common tasks
- [x] Create Dockerfile and docker-compose.yml
- [x] Set up GitHub Actions for CI

**Files to create:**
```
cmd/hypergoat/main.go
internal/config/config.go
Makefile
Dockerfile
docker-compose.yml
.github/workflows/ci.yml
```

### 1.2 Configuration Management

Port environment variable handling from Quickslice.

**Source:** `quickslice/server/src/server.gleam` (lines 62-221)

```go
// internal/config/config.go
type Config struct {
    // Server
    Host string `env:"HOST" envDefault:"127.0.0.1"`
    Port int    `env:"PORT" envDefault:"8080"`
    
    // Database
    DatabaseURL string `env:"DATABASE_URL" envDefault:"sqlite:data/hyperindex.db"`
    
    // Security
    SecretKeyBase string `env:"SECRET_KEY_BASE,required"`
    
    // OAuth
    ExternalBaseURL   string `env:"EXTERNAL_BASE_URL"`
    OAuthSigningKey   string `env:"OAUTH_SIGNING_KEY"`
    OAuthLoopbackMode bool   `env:"OAUTH_LOOPBACK_MODE" envDefault:"false"`
    
    // Jetstream
    JetstreamDisableCursor bool `env:"JETSTREAM_DISABLE_CURSOR" envDefault:"false"`
    
    // Backfill
    BackfillPDSConcurrency    int `env:"BACKFILL_PDS_CONCURRENCY" envDefault:"4"`
    BackfillMaxPDSWorkers     int `env:"BACKFILL_MAX_PDS_WORKERS" envDefault:"10"`
    BackfillMaxHTTPConcurrent int `env:"BACKFILL_MAX_HTTP_CONCURRENT" envDefault:"50"`
    BackfillRepoTimeout       int `env:"BACKFILL_REPO_TIMEOUT" envDefault:"60000"`
}
```

### 1.3 Database Executor Abstraction

Port the unified database interface from Quickslice.

**Source:** `quickslice/server/src/database/executor.gleam`

```go
// internal/database/executor.go
type Dialect int

const (
    SQLite Dialect = iota
    PostgreSQL
)

type Value interface {
    isValue()
}

type TextValue string
type IntValue int64
type FloatValue float64
type BoolValue bool
type NullValue struct{}
type BlobValue []byte
type TimestamptzValue string

type DbError struct {
    Type    string // "connection", "query", "decode", "constraint"
    Message string
    Cause   error
}

type Executor interface {
    // Core operations
    Query(ctx context.Context, sql string, params []Value, dest any) error
    Exec(ctx context.Context, sql string, params []Value) error
    
    // Dialect helpers
    Dialect() Dialect
    Placeholder(index int) string        // "?" vs "$1"
    JSONExtract(column, field string) string
    JSONExtractPath(column string, path []string) string
    Now() string                         // "datetime('now')" vs "NOW()"
    
    // Utilities
    Placeholders(count, startIndex int) string
}
```

**SQLite Implementation:** `internal/database/sqlite/executor.go`
- Use `modernc.org/sqlite` (pure Go, no CGO)
- Placeholder: `?`
- JSON: `json_extract(col, '$.field')`
- Now: `datetime('now')`

**PostgreSQL Implementation:** `internal/database/postgres/executor.go`
- Use `github.com/jackc/pgx/v5`
- Placeholder: `$1`, `$2`, ...
- JSON: `col->>'field'`
- Now: `NOW()`

### 1.4 Database Connection Factory

**Source:** `quickslice/server/src/database/connection.gleam`

```go
// internal/database/connection.go
func Connect(databaseURL string) (Executor, error) {
    if strings.HasPrefix(databaseURL, "postgres://") || 
       strings.HasPrefix(databaseURL, "postgresql://") {
        return postgres.NewExecutor(databaseURL)
    }
    return sqlite.NewExecutor(databaseURL)
}
```

### 1.5 Core Repositories

Port the main data access repositories.

**Source:** `quickslice/server/src/database/repositories/`

#### Records Repository
**Source:** `records.gleam`

```go
// internal/database/repositories/records.go
type Record struct {
    URI        string
    CID        string
    DID        string
    Collection string
    JSON       string
    IndexedAt  time.Time
    RKey       string
}

type RecordsRepository interface {
    Insert(ctx context.Context, record *Record) error
    InsertBatch(ctx context.Context, records []*Record) error
    GetByURI(ctx context.Context, uri string) (*Record, error)
    GetByURIs(ctx context.Context, uris []string) ([]*Record, error)
    Delete(ctx context.Context, uri string) error
    Query(ctx context.Context, params QueryParams) (*QueryResult, error)
}
```

#### Actors Repository
**Source:** `actors.gleam`

```go
// internal/database/repositories/actors.go
type Actor struct {
    DID       string
    Handle    string
    IndexedAt time.Time
}

type ActorsRepository interface {
    Upsert(ctx context.Context, actor *Actor) error
    GetByDID(ctx context.Context, did string) (*Actor, error)
    GetByHandle(ctx context.Context, handle string) (*Actor, error)
}
```

#### Lexicons Repository
**Source:** `lexicons.gleam`

```go
// internal/database/repositories/lexicons.go
type Lexicon struct {
    ID        string
    JSON      string
    CreatedAt time.Time
}

type LexiconsRepository interface {
    Upsert(ctx context.Context, lexicon *Lexicon) error
    GetAll(ctx context.Context) ([]*Lexicon, error)
    GetByID(ctx context.Context, id string) (*Lexicon, error)
    Delete(ctx context.Context, id string) error
}
```

#### Config Repository
**Source:** `config.gleam`

```go
// internal/database/repositories/config.go
type ConfigRepository interface {
    Get(ctx context.Context, key string) (string, error)
    Set(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
    GetAdminDIDs(ctx context.Context) ([]string, error)
    SetAdminDIDs(ctx context.Context, dids []string) error
}
```

### 1.6 Database Migrations

Port all migrations from Quickslice.

**Source:** `quickslice/server/db/migrations/` and `db/migrations_postgres/`

Create migration files:
1. `000001_initial_schema.up.sql` / `000001_initial_schema.down.sql`
2. `000002_add_rkey_column.up.sql` / `000002_add_rkey_column.down.sql`
3. `000003_add_labels_and_reports.up.sql` / `000003_add_labels_and_reports.down.sql`
4. `000004_add_label_preferences.up.sql` / `000004_add_label_preferences.down.sql`

---

## Phase 2: Lexicon Parsing & GraphQL Core (Week 2-3) ✅ COMPLETE

> **Implementation Notes:**
> - Lexicon parser: `internal/lexicon/parser.go`
> - Registry: `internal/lexicon/registry.go`
> - NSID utilities: `internal/lexicon/nsid.go`
> - GraphQL type mapper: `internal/graphql/types/mapper.go`
> - Object builder: `internal/graphql/types/object.go`

### 2.1 Lexicon Type Definitions

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/types.gleam`

```go
// internal/lexicon/types.go
type Lexicon struct {
    ID   string `json:"id"`
    Defs Defs   `json:"defs"`
}

type Defs struct {
    Main   *RecordDef         `json:"main,omitempty"`
    Others map[string]*ObjDef `json:"-"` // Parsed from remaining fields
}

type RecordDef struct {
    Type       string     `json:"type"`
    Key        *string    `json:"key,omitempty"`
    Properties []Property `json:"-"` // Parsed from record.properties
}

type Property struct {
    Name     string
    Type     string    `json:"type"`
    Required bool      // Computed from parent's "required" array
    Format   *string   `json:"format,omitempty"`
    Ref      *string   `json:"ref,omitempty"`
    Refs     []string  `json:"refs,omitempty"`
    Items    *Property `json:"items,omitempty"`
}
```

### 2.2 Lexicon Parser

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/internal/lexicon/parser.gleam`

```go
// internal/lexicon/parser.go
func ParseLexicon(jsonStr string) (*Lexicon, error)
func parseDefs(data map[string]any) (*Defs, error)
func parseRecordDef(data map[string]any) (*RecordDef, error)
func parseProperty(name string, data map[string]any, required bool) (*Property, error)
func parseArrayItems(data map[string]any) (*Property, error)
```

**Key parsing logic:**
- Handle both `type: "record"` (with nested `record` wrapper) and `type: "object"` (flat)
- Parse `required` array from parent to set property.Required
- Handle array items with nested types (ref, union, primitives)

### 2.3 Lexicon Registry

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/internal/lexicon/registry.gleam`

```go
// internal/lexicon/registry.go
type Registry struct {
    lexicons map[string]*Lexicon
}

func NewRegistry(lexicons []*Lexicon) *Registry
func (r *Registry) Resolve(ref string) (*ObjDef, error)
func (r *Registry) GetLexicon(id string) (*Lexicon, bool)
```

### 2.4 NSID Utilities

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/internal/lexicon/nsid.gleam`

```go
// internal/lexicon/nsid.go
func ToTypeName(nsid string) string      // "app.bsky.feed.post" -> "AppBskyFeedPost"
func ToFieldName(nsid string) string     // "app.bsky.feed.post" -> "appBskyFeedPost"
func ParseCollection(uri string) string  // Extract collection from at:// URI
```

### 2.5 GraphQL Type Mapping

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/internal/graphql/type_mapper.gleam`

```go
// internal/graphql/types/mapper.go
func MapType(lexiconType string) graphql.Type
func MapInputType(lexiconType string) graphql.Input
func MapPropertyType(prop *lexicon.Property, registry *lexicon.Registry) graphql.Type
```

**Mapping table:**
| Lexicon | GraphQL |
|---------|---------|
| string | String |
| integer | Int |
| boolean | Boolean |
| number | Float |
| blob | Blob (custom object) |
| bytes | String |
| cid-link | String |
| ref | Resolved type or union |
| union | GraphQL Union |
| array | List type |

### 2.6 GraphQL Object Builder

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/internal/graphql/object_builder.gleam`

```go
// internal/graphql/schema/object_builder.go
func BuildObjectType(lexicon *lexicon.Lexicon, registry *lexicon.Registry) *graphql.Object
func BuildUnionType(refs []string, registry *lexicon.Registry) *graphql.Union
```

### 2.7 WHERE Input Types

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/input/where.gleam`

```go
// internal/graphql/where/types.go
type WhereClause struct {
    Conditions map[string]*WhereCondition
    And        []*WhereClause
    Or         []*WhereClause
}

type WhereCondition struct {
    Eq       *Value
    In       []Value
    Contains *string
    Gt       *Value
    Gte      *Value
    Lt       *Value
    Lte      *Value
    IsNull   *bool
}
```

```go
// internal/graphql/where/builder.go
func BuildWhereInputType(typeName string, props []lexicon.Property) *graphql.InputObject
func ParseWhereInput(input map[string]any) (*WhereClause, error)
```

### 2.8 Connection Types (Relay Spec)

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/input/connection.gleam`

```go
// internal/graphql/types/connection.go
func BuildConnectionType(nodeType *graphql.Object) *graphql.Object
func BuildEdgeType(nodeType *graphql.Object) *graphql.Object
func BuildPageInfoType() *graphql.Object
func BuildSortEnumType(typeName string, props []lexicon.Property) *graphql.Enum
func BuildSortInputType(enumType *graphql.Enum) *graphql.InputObject
```

### 2.9 Schema Builder (Multi-Pass)

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/schema/database.gleam`

The schema builder uses a multi-pass approach:

```go
// internal/graphql/schema/builder.go

// Pass 0: Build object types from defs (e.g., #aspectRatio)
// Pass 1: Extract metadata, build basic types, create Record union
// Pass 0b: Rebuild with forward joins (now that Record union exists)
// Pass 2: Build RecordTypes with ALL join fields
// Pass 3: Rebuild join fields with complete types

func BuildSchema(
    lexicons []*lexicon.Lexicon,
    fetcher RecordFetcher,
    mutationFactory MutationFactory,
) (*graphql.Schema, error)
```

---

## Phase 3: GraphQL API (Week 3-4) ✅ COMPLETE

> **Implementation Notes:**
> - Schema builder: `internal/graphql/schema/builder.go`
> - Query resolvers: `internal/graphql/schema/builder.go` (createCollectionResolver, createSingleRecordResolver)
> - Connection types: `internal/graphql/query/connection.go`
> - Handler: `internal/graphql/handler.go`

### 3.1 Record Fetcher Interface

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/schema/database.gleam`

```go
// internal/graphql/resolvers/fetcher.go
type PaginationParams struct {
    First   *int
    After   *string
    Last    *int
    Before  *string
    SortBy  []SortField
    Where   *where.WhereClause
}

type QueryResult struct {
    Edges          []Edge
    HasNextPage    bool
    HasPreviousPage bool
    TotalCount     *int
}

type RecordFetcher interface {
    FetchRecords(ctx context.Context, collection string, params PaginationParams) (*QueryResult, error)
    FetchByURIs(ctx context.Context, uris []string) (map[string]any, error)
}
```

### 3.2 Query Resolvers

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/query/dataloader.gleam`

```go
// internal/graphql/resolvers/query.go
func MakeRecordQueryResolver(collection string, fetcher RecordFetcher) graphql.FieldResolveFn
func MakeAggregateResolver(collection string, fetcher AggregateFetcher) graphql.FieldResolveFn
```

### 3.3 DataLoader Implementation

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/query/dataloader.gleam`

```go
// internal/graphql/dataloader/loader.go
type Loader struct {
    batchFn   func(keys []string) (map[string]any, error)
    cache     sync.Map
    pending   []string
    pendingMu sync.Mutex
}

func NewLoader(batchFn func([]string) (map[string]any, error)) *Loader
func (l *Loader) Load(ctx context.Context, key string) (any, error)
func (l *Loader) LoadMany(ctx context.Context, keys []string) ([]any, error)
```

### 3.4 Join Field Resolvers

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/schema/database.gleam` (join sections)

```go
// internal/graphql/resolvers/joins.go

// Forward join: resolve strongRef/at-uri to the target record
func MakeForwardJoinResolver(fieldName string, loader *Loader) graphql.FieldResolveFn

// Reverse join: find records that reference this record
func MakeReverseJoinResolver(
    sourceCollection string,
    joinField string,
    fetcher RecordFetcher,
) graphql.FieldResolveFn

// DID join: find records by the same author
func MakeDIDJoinResolver(
    targetCollection string,
    fetcher RecordFetcher,
) graphql.FieldResolveFn

// Viewer state: find viewer's record relating to this record
func MakeViewerStateResolver(
    viewerCollection string,
    joinField string,
    fetcher RecordFetcher,
) graphql.FieldResolveFn
```

### 3.5 Mutation Resolvers

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/mutation/builder.gleam`

```go
// internal/graphql/resolvers/mutation.go
type MutationFactory interface {
    CreateResolver(collection string) graphql.FieldResolveFn
    UpdateResolver(collection string) graphql.FieldResolveFn
    DeleteResolver(collection string) graphql.FieldResolveFn
    UploadBlobResolver() graphql.FieldResolveFn
}

func BuildMutationType(
    lexicons []*lexicon.Lexicon,
    factory MutationFactory,
) *graphql.Object
```

### 3.6 Aggregation Support

**Source:** `quickslice/lexicon_graphql/src/lexicon_graphql/input/aggregate.gleam`

```go
// internal/graphql/resolvers/aggregate.go
type AggregateParams struct {
    GroupBy []GroupByField
    Where   *where.WhereClause
    OrderBy *string // "asc" or "desc"
    Limit   *int
}

type GroupByField struct {
    Field    string
    Interval *string // "HOUR", "DAY", "WEEK", "MONTH"
}

type AggregateFetcher interface {
    Aggregate(ctx context.Context, collection string, params AggregateParams) ([]map[string]any, error)
}
```

### 3.7 GraphQL HTTP Handler

**Source:** `quickslice/server/src/handlers/graphql.gleam`

```go
// internal/handlers/graphql.go
func NewGraphQLHandler(schema *graphql.Schema, authMiddleware AuthMiddleware) http.Handler
```

---

## Phase 4: Real-time Features (Week 4-5) ✅ COMPLETE

> **Implementation Notes:**
> - **Jetstream Consumer:** `internal/jetstream/consumer.go`, `internal/jetstream/client.go`
> - **GraphQL Subscriptions:** `internal/graphql/subscription/pubsub.go`, `internal/graphql/subscription/handler.go`
> - **Historical Backfill:** `internal/backfill/backfill.go`, `internal/backfill/client.go`
> - WebSocket endpoint at `/graphql/ws` implementing graphql-transport-ws protocol
> - Integration tests for hypercerts.claim.activity in `internal/backfill/backfill_integration_test.go`

### 4.1 PubSub System

**Source:** `quickslice/server/src/pubsub.gleam`

```go
// internal/pubsub/pubsub.go
type RecordOperation int

const (
    Create RecordOperation = iota
    Update
    Delete
)

type RecordEvent struct {
    URI        string
    CID        string
    DID        string
    Collection string
    Value      string
    IndexedAt  string
    Operation  RecordOperation
}

type PubSub struct {
    subscribers sync.Map // map[string][]chan RecordEvent
}

func NewPubSub() *PubSub
func (ps *PubSub) Subscribe(collection string) (<-chan RecordEvent, func())
func (ps *PubSub) Publish(event RecordEvent)
```

### 4.2 Stats PubSub

**Source:** `quickslice/server/src/stats_pubsub.gleam`

```go
// internal/pubsub/stats.go
type StatsEvent struct {
    Type       string // "record_created", "record_deleted", "actor_created", "activity_logged"
    Collection string
    Count      int
}
```

### 4.3 Jetstream Consumer

**Source:** `quickslice/server/src/jetstream_consumer.gleam`

```go
// internal/jetstream/consumer.go
type Consumer struct {
    url         string
    collections []string
    cursor      int64
    db          database.Executor
    pubsub      *pubsub.PubSub
    handler     EventHandler
}

func NewConsumer(config ConsumerConfig) *Consumer
func (c *Consumer) Start(ctx context.Context) error
func (c *Consumer) Stop() error
```

### 4.4 Jetstream Manager (Supervisor)

**Source:** `quickslice/server/src/jetstream_consumer.gleam` (manager section)

```go
// internal/jetstream/manager.go
type Manager struct {
    consumer       *Consumer
    lastMessage    time.Time
    heartbeatCheck time.Duration
    restartCh      chan struct{}
}

func NewManager(consumer *Consumer) *Manager
func (m *Manager) Start(ctx context.Context) error
func (m *Manager) Restart() error
func (m *Manager) Stop() error
```

### 4.5 Cursor Tracker

**Source:** `quickslice/server/src/jetstream_consumer.gleam` (cursor tracker)

```go
// internal/jetstream/cursor.go
type CursorTracker struct {
    db       database.Executor
    cursor   int64
    dirty    bool
    flushInt time.Duration
}

func NewCursorTracker(db database.Executor) *CursorTracker
func (ct *CursorTracker) Update(cursor int64)
func (ct *CursorTracker) Start(ctx context.Context) // Periodic flush
```

### 4.6 Event Handler

**Source:** `quickslice/server/src/event_handler.gleam`

```go
// internal/jetstream/handler.go
type EventHandler interface {
    HandleCreate(ctx context.Context, event JetstreamEvent) error
    HandleUpdate(ctx context.Context, event JetstreamEvent) error
    HandleDelete(ctx context.Context, event JetstreamEvent) error
}

type DefaultHandler struct {
    db        database.Executor
    pubsub    *pubsub.PubSub
    lexicons  *lexicon.Registry
    validator RecordValidator
}
```

### 4.7 GraphQL Subscriptions

**Source:** `quickslice/server/src/handlers/graphql_ws.gleam`

```go
// internal/handlers/graphql_ws.go
// Implements graphql-ws protocol (graphql-transport-ws)

type SubscriptionManager struct {
    schema        *graphql.Schema
    pubsub        *pubsub.PubSub
    subscriptions sync.Map // map[string]*Subscription
    maxPerConn    int
    maxGlobal     int
}

func NewSubscriptionManager(schema *graphql.Schema, pubsub *pubsub.PubSub) *SubscriptionManager
func (sm *SubscriptionManager) HandleWebSocket(w http.ResponseWriter, r *http.Request)
```

---

## Phase 5: OAuth & Authentication (Week 5-6) ✅ COMPLETE

> **Implementation Notes:**
> - OAuth repositories: `internal/database/repositories/oauth_*.go` (10 tables)
> - DID resolver: `internal/oauth/did.go`, `internal/oauth/did_cache.go`
> - PKCE: `internal/oauth/pkce.go` (RFC 7636)
> - DPoP: `internal/oauth/dpop.go` (RFC 9449)
> - OAuth server: `internal/oauth/server.go`, `internal/oauth/token_generator.go`
> - AT Protocol bridge: `internal/oauth/bridge.go`
> - Auth middleware: `internal/oauth/middleware.go`
> - HTTP handlers: `internal/server/oauth_handlers.go`
> - 92+ tests passing

### 5.1 OAuth Repositories

**Source:** `quickslice/server/src/database/repositories/oauth_*.gleam`

Port all 10 OAuth repositories:
1. `oauth_clients.go`
2. `oauth_access_tokens.go`
3. `oauth_refresh_tokens.go`
4. `oauth_authorization_code.go`
5. `oauth_par_requests.go`
6. `oauth_dpop_nonces.go`
7. `oauth_dpop_jti.go`
8. `oauth_auth_requests.go`
9. `oauth_atp_sessions.go`
10. `oauth_atp_requests.go`

### 5.2 OAuth Server

**Source:** `quickslice/server/src/handlers/oauth/`

```go
// internal/oauth/server.go
type Server struct {
    config      *Config
    db          database.Executor
    signingKey  *jose.JSONWebKey
    atpBridge   *atproto.Bridge
}

// Endpoints
func (s *Server) MetadataHandler() http.Handler      // /.well-known/oauth-authorization-server
func (s *Server) JWKSHandler() http.Handler          // /.well-known/jwks.json
func (s *Server) RegisterHandler() http.Handler      // /oauth/register
func (s *Server) PARHandler() http.Handler           // /oauth/par
func (s *Server) AuthorizeHandler() http.Handler     // /oauth/authorize
func (s *Server) TokenHandler() http.Handler         // /oauth/token
```

### 5.3 PKCE Implementation

**Source:** `quickslice/server/src/lib/oauth/pkce.gleam`

```go
// internal/oauth/pkce/pkce.go
func GenerateVerifier() (string, error)
func GenerateChallenge(verifier string) string
func VerifyChallenge(verifier, challenge, method string) bool
```

### 5.4 DPoP Implementation

**Source:** `quickslice/server/src/lib/oauth/dpop/`

```go
// internal/oauth/dpop/dpop.go
type Proof struct {
    Header    jose.Header
    Claims    DPoPClaims
    Signature []byte
}

func GenerateKey() (*jose.JSONWebKey, error)
func CreateProof(key *jose.JSONWebKey, method, uri, nonce string) (string, error)
func ValidateProof(proofJWT, method, uri string, nonces NonceStore) (*Proof, error)
```

### 5.5 DID Resolver

**Source:** `quickslice/server/src/lib/oauth/atproto/did_resolver.gleam`

```go
// internal/oauth/atproto/did_resolver.go
type DIDDocument struct {
    ID                 string
    AlsoKnownAs        []string
    VerificationMethod []VerificationMethod
    Service            []Service
}

type Resolver struct {
    plcDirectoryURL string
    httpClient      *http.Client
    cache           *DIDCache
}

func (r *Resolver) Resolve(ctx context.Context, did string) (*DIDDocument, error)
func (r *Resolver) GetPDSEndpoint(doc *DIDDocument) (string, error)
func (r *Resolver) GetHandle(doc *DIDDocument) (string, error)
```

### 5.6 DID Cache

**Source:** `quickslice/server/src/lib/oauth/did_cache.gleam`

```go
// internal/oauth/atproto/did_cache.go
type DIDCache struct {
    entries sync.Map // map[string]*CacheEntry
    ttl     time.Duration
}

type CacheEntry struct {
    Document  *DIDDocument
    ExpiresAt time.Time
}
```

### 5.7 AT Protocol Bridge

**Source:** `quickslice/server/src/lib/oauth/atproto/bridge.gleam`

```go
// internal/oauth/atproto/bridge.go
type Bridge struct {
    resolver    *Resolver
    dpopKeygen  func() (*jose.JSONWebKey, error)
    httpClient  *http.Client
}

func (b *Bridge) InitiateAuth(ctx context.Context, did string, scopes []string) (*AuthRequest, error)
func (b *Bridge) HandleCallback(ctx context.Context, code, state string) (*TokenResponse, error)
func (b *Bridge) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error)
```

### 5.8 Auth Middleware

**Source:** `quickslice/server/src/atproto_auth.gleam`

```go
// internal/middleware/auth.go
type AuthContext struct {
    DID          string
    Handle       string
    AccessToken  string
    IsAdmin      bool
}

func AuthMiddleware(db database.Executor, oauthServer *oauth.Server) func(http.Handler) http.Handler
func ExtractAuthContext(ctx context.Context) (*AuthContext, bool)
```

---

## Phase 6: Backfill & Admin (Week 6-7) ✅ COMPLETE

> **Implementation Notes:**
> - Backfill: `internal/backfill/backfill.go`, `internal/backfill/client.go` (from Phase 4)
> - Admin repositories: `internal/database/repositories/jetstream_activity.go`, `labels.go`, `label_definitions.go`, `label_preferences.go`, `reports.go`
> - Admin GraphQL types: `internal/graphql/admin/types.go` (594 lines)
> - Admin resolvers: `internal/graphql/admin/resolvers.go` (563 lines)
> - Admin schema: `internal/graphql/admin/schema.go` (487 lines)
> - Admin handler: `internal/graphql/admin/handler.go` (141 lines)
> - Background workers: `internal/workers/activity_cleanup.go`, `backfill_state.go`
> - Tests: `internal/graphql/admin/handler_test.go` (9 tests passing)

### 6.1 Backfill Configuration

**Source:** `quickslice/server/src/backfill.gleam`

```go
// internal/backfill/config.go
type Config struct {
    PLCDirectoryURL       string
    IndexActors           bool
    MaxConcurrentPerPDS   int
    MaxPDSWorkers         int
    MaxHTTPConcurrent     int
    RepoFetchTimeout      time.Duration
}
```

### 6.2 Backfill Worker

**Source:** `quickslice/server/src/backfill.gleam`

```go
// internal/backfill/worker.go
type Worker struct {
    config     *Config
    db         database.Executor
    resolver   *atproto.Resolver
    httpClient *http.Client
    semaphore  chan struct{}
}

func (w *Worker) BackfillCollections(ctx context.Context, collections []string) error
func (w *Worker) BackfillActor(ctx context.Context, did string) error
func (w *Worker) discoverRepos(ctx context.Context, collection string) ([]string, error)
func (w *Worker) groupByPDS(ctx context.Context, dids []string) (map[string][]string, error)
func (w *Worker) fetchRepo(ctx context.Context, pds, did string) ([]byte, error)
```

### 6.3 CAR Parser

**Source:** `quickslice/atproto_car/`

```go
// internal/backfill/car/parser.go
type Block struct {
    CID  string
    Data []byte
}

type Record struct {
    Collection string
    RKey       string
    CID        string
    Value      []byte
}

func ParseCAR(data []byte) (*CAR, error)
func (c *CAR) WalkRecords(fn func(Record) error) error
```

### 6.4 Backfill State

**Source:** `quickslice/server/src/backfill_state.gleam`

```go
// internal/backfill/state.go
type State struct {
    mu         sync.RWMutex
    isRunning  bool
    progress   int
    total      int
    collection string
    startedAt  time.Time
}

func NewState() *State
func (s *State) Start(collection string, total int)
func (s *State) Update(progress int)
func (s *State) Finish()
func (s *State) IsRunning() bool
func (s *State) Status() StatusResponse
```

### 6.5 Admin GraphQL Schema

**Source:** `quickslice/server/src/graphql/admin/schema.gleam`

```go
// internal/graphql/admin/schema.go
func BuildAdminSchema(
    db database.Executor,
    backfillState *backfill.State,
    jetstreamManager *jetstream.Manager,
) (*graphql.Schema, error)
```

**Admin Queries:**
- `currentSession` - Get current admin session
- `settings` - Get server settings
- `statistics` - Get record/actor counts
- `activityBuckets` - Get activity over time
- `recentActivity` - Get recent Jetstream activity
- `oauthClients` - List OAuth clients
- `lexicons` - List loaded lexicons

**Admin Mutations:**
- `updateSettings` - Update server configuration
- `triggerBackfill` - Start backfill operation
- `backfillActor` - Backfill specific actor
- `uploadLexicons` - Upload lexicon files
- `resetAll` - Reset all data (danger zone)
- `createOAuthClient` / `updateOAuthClient` / `deleteOAuthClient`
- `addAdmin` / `removeAdmin`

### 6.6 Admin Handler

**Source:** `quickslice/server/src/handlers/admin_graphql.gleam`

```go
// internal/handlers/admin.go
func NewAdminHandler(
    schema *graphql.Schema,
    db database.Executor,
    authMiddleware func(http.Handler) http.Handler,
) http.Handler
```

### 6.7 Activity Cleanup Worker

**Source:** `quickslice/server/src/activity_cleanup.gleam`

```go
// internal/workers/activity_cleanup.go
type ActivityCleanup struct {
    db       database.Executor
    interval time.Duration
    maxAge   time.Duration
}

func NewActivityCleanup(db database.Executor) *ActivityCleanup
func (ac *ActivityCleanup) Start(ctx context.Context)
```

---

## Phase 7: Server Integration & Complete OAuth (Week 7-8) ✅ COMPLETE

> **Implementation Notes:**
> - Main server: `cmd/hypergoat/main.go` (730 lines)
> - OAuth handlers: `internal/server/oauth_handlers.go`, `oauth_register.go`, `oauth_par.go`
> - GraphiQL: `internal/server/graphiql.go`
> - All endpoints wired up and functional
> - Background workers running (activity cleanup, OAuth token cleanup)
> - Graceful shutdown implemented

### 7.1 HTTP Router Setup

**Source:** `quickslice/server/src/server.gleam` (handle_request)

```go
// cmd/hypergoat/main.go
func setupRouter(deps *Dependencies) *chi.Mux {
    r := chi.NewRouter()
    
    // Middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(corsMiddleware)
    
    // Static files (serve Quickslice client)
    r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(...)))
    
    // Health
    r.Get("/health", handlers.HealthHandler(deps.db))
    
    // GraphQL
    r.Handle("/graphql", deps.graphqlHandler)
    r.Handle("/admin/graphql", deps.adminHandler)
    
    // GraphiQL
    r.Get("/graphiql", handlers.GraphiQLHandler())
    r.Get("/graphiql/admin", handlers.AdminGraphiQLHandler())
    
    // OAuth
    r.Get("/.well-known/oauth-authorization-server", deps.oauth.MetadataHandler())
    r.Get("/.well-known/jwks.json", deps.oauth.JWKSHandler())
    r.Post("/oauth/register", deps.oauth.RegisterHandler())
    r.Post("/oauth/par", deps.oauth.PARHandler())
    r.Get("/oauth/authorize", deps.oauth.AuthorizeHandler())
    r.Post("/oauth/authorize", deps.oauth.AuthorizeHandler())
    r.Post("/oauth/token", deps.oauth.TokenHandler())
    r.Get("/oauth/atp/callback", deps.oauth.ATPCallbackHandler())
    
    // Admin OAuth
    r.Get("/admin/oauth/authorize", deps.oauth.AdminAuthorizeHandler())
    r.Get("/admin/oauth/callback", deps.oauth.AdminCallbackHandler())
    
    // SPA fallback
    r.NotFound(handlers.SPAHandler())
    
    return r
}
```

### 7.2 Graceful Shutdown

```go
// cmd/hypergoat/main.go
func main() {
    // ... setup ...
    
    server := &http.Server{
        Addr:    fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
        Handler: router,
    }
    
    go func() {
        slog.Info("Starting server", "addr", server.Addr)
        if err := server.ListenAndServe(); err != http.ErrServerClosed {
            slog.Error("Server error", "error", err)
        }
    }()
    
    // Wait for interrupt
    quit := make(chan os.Signal, 1)
    signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
    <-quit
    
    slog.Info("Shutting down...")
    
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Stop workers
    jetstreamManager.Stop()
    activityCleanup.Stop()
    
    // Shutdown server
    server.Shutdown(ctx)
}
```

### 7.3 Observability

```go
// internal/middleware/logging.go
func LoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        start := time.Now()
        ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
        
        defer func() {
            slog.Info("Request",
                "method", r.Method,
                "path", r.URL.Path,
                "status", ww.Status(),
                "duration", time.Since(start),
                "bytes", ww.BytesWritten(),
            )
        }()
        
        next.ServeHTTP(ww, r)
    })
}
```

### 7.4 Health Check

```go
// internal/handlers/health.go
func HealthHandler(db database.Executor) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // Check database
        if err := db.Exec(r.Context(), "SELECT 1", nil); err != nil {
            http.Error(w, "Database unhealthy", http.StatusServiceUnavailable)
            return
        }
        
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    }
}
```

### 7.5 API Compatibility Testing

- [x] Verify GraphQL schema matches Quickslice output
- [ ] Test with quickslice-client-js SDK
- [ ] Test OAuth flows with Bluesky PDS
- [x] Test Jetstream event handling
- [x] Test backfill with real data (integration tests in `internal/backfill/backfill_integration_test.go`)

---

## Testing Strategy

### Unit Tests
```go
// Example: internal/lexicon/parser_test.go
func TestParseLexicon(t *testing.T) {
    tests := []struct {
        name    string
        json    string
        wantID  string
        wantErr bool
    }{
        {
            name: "simple record",
            json: `{"lexicon":1,"id":"xyz.test.record",...}`,
            wantID: "xyz.test.record",
        },
        // ... more cases
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseLexicon(tt.json)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if got.ID != tt.wantID {
                t.Errorf("ID = %v, want %v", got.ID, tt.wantID)
            }
        })
    }
}
```

### Integration Tests
```go
// Example: internal/database/repositories/records_test.go
func TestRecordsRepository(t *testing.T) {
    // Test with both SQLite and PostgreSQL
    for _, dialect := range []string{"sqlite", "postgres"} {
        t.Run(dialect, func(t *testing.T) {
            db := setupTestDB(t, dialect)
            repo := NewRecordsRepository(db)
            
            // Test Insert
            // Test Query
            // Test Delete
        })
    }
}
```

### Test Fixtures
Copy test fixtures from Quickslice:
- `quickslice/server/test/fixtures/` -> `hypergoat/testdata/fixtures/`

---

## Migration Guide (From Quickslice)

For users migrating from Quickslice:

1. **Database**: Same schema, direct migration
   ```bash
   # Stop Quickslice
   # Point Hypergoat to same DATABASE_URL
   # Start Hypergoat
   ```

2. **Configuration**: Same environment variables

3. **Client**: quickslice-client-js works unchanged

4. **Static files**: Copy `quickslice/server/priv/static/` to `hypergoat/static/`

---

## Dependencies Summary

```go
// go.mod (estimated)
require (
    github.com/go-chi/chi/v5 v5.0.12
    github.com/jackc/pgx/v5 v5.5.3
    modernc.org/sqlite v1.29.1
    github.com/graphql-go/graphql v0.8.1
    nhooyr.io/websocket v1.8.10
    github.com/golang-jwt/jwt/v5 v5.2.0
    github.com/go-jose/go-jose/v4 v4.0.1
    github.com/caarlos0/env/v10 v10.0.0
    github.com/golang-migrate/migrate/v4 v4.17.0
    github.com/tidwall/gjson v1.17.0
)
```

---

## Risk Mitigation

| Risk | Mitigation |
|------|------------|
| Runtime GraphQL performance | Profile early, cache compiled resolvers |
| CAR parsing complexity | Use existing Go CBOR/CAR libraries if available |
| OAuth spec compliance | Comprehensive test suite against Bluesky PDS |
| Dialect differences | Integration tests for both databases |
| Jetstream reconnection | Exponential backoff, dead letter handling |

---

## Success Criteria

- [x] All Quickslice GraphQL queries work identically
- [ ] OAuth flows work with Bluesky PDS (partial - endpoints implemented, needs real-world testing)
- [x] Jetstream sync maintains cursor across restarts
- [x] Backfill completes successfully
- [x] Admin UI functions correctly
- [ ] Performance >= Quickslice for typical workloads (not benchmarked)
- [x] Docker deployment works
- [x] All tests pass
