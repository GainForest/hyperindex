# Hypergoat

**A Go implementation of [Quickslice](https://github.com/quickslice/quickslice) - an AT Protocol AppView server**

Hypergoat indexes Lexicon-defined AT Protocol records into a database and exposes them via a dynamically-generated GraphQL API. It's a feature-complete port of Quickslice from Gleam/Erlang to Go.

> **Status:** In Development

## Features

- **Dynamic GraphQL API** - Automatically generates GraphQL schema from AT Protocol Lexicon definitions
- **Real-time Sync** - Connects to Jetstream for live AT Protocol event streaming
- **Multi-Database Support** - SQLite for development, PostgreSQL for production
- **OAuth 2.0 Server** - Full OAuth implementation with DPoP, PKCE, and AT Protocol bridge
- **GraphQL Subscriptions** - Real-time updates via WebSocket
- **Batch Backfill** - Import historical data from AT Protocol relays
- **Relay Connections** - Cursor-based pagination following the Relay specification
- **Automatic Joins** - Forward joins, reverse joins, and DID-based joins between record types

## Quick Start

### Docker (Recommended)

```bash
docker compose up --build
```

Open http://localhost:8080 and login with your Bluesky handle.

### Native Development

**Prerequisites:**
- Go 1.22+
- SQLite or PostgreSQL

**Setup:**

```bash
# Clone the repository
git clone https://github.com/GainForest/hypergoat.git
cd hypergoat

# Copy environment configuration
cp .env.example .env

# Run database migrations
make db-migrate

# Start the server
make run
```

## Architecture

Hypergoat is a port of Quickslice, maintaining the same architecture and API compatibility:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Hypergoat Server                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐ │
│  │   chi       │  │  GraphQL    │  │  OAuth 2.0  │  │    Admin API        │ │
│  │   Router    │  │  (runtime)  │  │   Server    │  │    (GraphQL)        │ │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────────┬──────────┘ │
│         │                │                │                     │           │
│  ┌──────┴────────────────┴────────────────┴─────────────────────┴─────────┐ │
│  │                         Handler Layer                                   │ │
│  └─────────────────────────────────┬───────────────────────────────────────┘ │
│                                    │                                         │
│  ┌─────────────────────────────────┴───────────────────────────────────────┐ │
│  │                         Service Layer                                   │ │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐  ┌─────────────┐  │ │
│  │  │   Lexicon    │  │   Jetstream  │  │   Backfill   │  │   PubSub    │  │ │
│  │  │   Parser     │  │   Consumer   │  │   Worker     │  │   System    │  │ │
│  │  └──────────────┘  └──────────────┘  └──────────────┘  └─────────────┘  │ │
│  └─────────────────────────────────┬───────────────────────────────────────┘ │
│                                    │                                         │
│  ┌─────────────────────────────────┴───────────────────────────────────────┐ │
│  │                       Database Executor                                 │ │
│  │              (Unified interface for SQLite/PostgreSQL)                  │ │
│  └─────────────────────────────────┬───────────────────────────────────────┘ │
│                                    │                                         │
│         ┌──────────────────────────┼──────────────────────────┐             │
│         │                          │                          │             │
│    ┌────┴────┐                ┌────┴────┐                ┌────┴────┐        │
│    │ SQLite  │                │PostgreSQL│               │ Repositories│    │
│    └─────────┘                └─────────┘                └─────────┘        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Technology Stack

| Component | Library | Purpose |
|-----------|---------|---------|
| HTTP Router | [chi](https://github.com/go-chi/chi) | Lightweight, idiomatic routing |
| Database | [pgx](https://github.com/jackc/pgx) + [modernc/sqlite](https://pkg.go.dev/modernc.org/sqlite) | Multi-database support |
| GraphQL | [graphql-go](https://github.com/graphql-go/graphql) | Runtime schema generation |
| WebSocket | [nhooyr/websocket](https://github.com/nhooyr/websocket) | GraphQL subscriptions |
| JWT | [golang-jwt](https://github.com/golang-jwt/jwt) | Token handling |
| JOSE | [go-jose](https://github.com/go-jose/go-jose) | DPoP, JWK, JWKS |

## Project Structure

```
hypergoat/
├── cmd/
│   └── hypergoat/
│       └── main.go                 # Application entry point
├── internal/
│   ├── config/                     # Configuration management
│   ├── database/
│   │   ├── executor.go             # Unified DB interface
│   │   ├── sqlite/                 # SQLite implementation
│   │   ├── postgres/               # PostgreSQL implementation
│   │   └── repositories/           # Data access layer
│   ├── graphql/
│   │   ├── schema/                 # Dynamic schema building
│   │   ├── resolvers/              # Query/mutation resolvers
│   │   ├── dataloader/             # N+1 prevention
│   │   └── where/                  # WHERE clause parsing
│   ├── lexicon/                    # Lexicon parsing & registry
│   ├── jetstream/                  # Real-time event consumer
│   ├── backfill/                   # Batch import worker
│   ├── oauth/                      # OAuth 2.0 server
│   ├── pubsub/                     # Event pub/sub system
│   └── handlers/                   # HTTP handlers
├── pkg/
│   └── atproto/                    # AT Protocol utilities
├── db/
│   └── migrations/                 # Database migrations
├── Makefile
├── Dockerfile
└── docker-compose.yml
```

## Configuration

Environment variables (see `.env.example`):

### Server Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `DATABASE_URL` | Database connection string | `sqlite:data/hypergoat.db` |
| `HOST` | Server bind address | `127.0.0.1` |
| `PORT` | Server port | `8080` |
| `SECRET_KEY_BASE` | Session encryption key (64+ chars) | Required |
| `EXTERNAL_BASE_URL` | Public URL for OAuth redirects | `http://{HOST}:{PORT}` |
| `OAUTH_SIGNING_KEY` | JWT signing key (multibase) | Optional |
| `OAUTH_LOOPBACK_MODE` | Enable loopback OAuth for local dev | `false` |

### Backfill Configuration

| Variable | Description | Default |
|----------|-------------|---------|
| `BACKFILL_RELAY_URL` | AT Protocol relay for repo discovery | `https://relay1.us-west.bsky.network` |
| `BACKFILL_PLC_URL` | PLC directory for DID resolution | `https://plc.directory` |
| `BACKFILL_MAX_HTTP` | Global max concurrent HTTP requests | `50` |
| `BACKFILL_MAX_PDS_WORKERS` | Max concurrent PDS workers | `10` |
| `BACKFILL_MAX_PER_PDS` | Max concurrent requests per PDS | `6` |
| `BACKFILL_MAX_REPOS` | Max concurrent DID resolutions | `50` |

## Database Schema

Hypergoat uses the same database schema as Quickslice for full compatibility:

### Core Tables
- `record` - AT Protocol records (posts, likes, follows, etc.)
- `actor` - User/actor information
- `lexicon` - Lexicon schema definitions
- `config` - Application configuration

### OAuth Tables (12 tables)
Full OAuth 2.0 implementation with DPoP support:
- `oauth_client`, `oauth_access_token`, `oauth_refresh_token`
- `oauth_authorization_code`, `oauth_par_request`
- `oauth_dpop_nonce`, `oauth_dpop_jti`
- `oauth_auth_request`, `oauth_atp_session`, `oauth_atp_request`

### Moderation Tables
- `label`, `label_definition`, `report`, `actor_label_preference`

## GraphQL API

The GraphQL schema is dynamically generated from Lexicon definitions:

```graphql
# Example: Query posts with filtering and pagination
query {
  appBskyFeedPost(
    first: 50
    sortBy: [{ field: indexedAt, direction: DESC }]
    where: {
      did: { eq: "did:plc:..." }
    }
  ) {
    edges {
      node {
        uri
        did
        text
        createdAt
        # Automatic joins
        appBskyActorProfileByDid {
          displayName
          avatar { url }
        }
        # Reverse joins
        appBskyFeedLikeViaSubject(first: 10) {
          totalCount
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

## API Compatibility

Hypergoat is designed as a drop-in replacement for Quickslice:

- **Same GraphQL schema** - Generated from the same Lexicons
- **Same REST endpoints** - OAuth, health, static files
- **Same database schema** - Direct migration possible
- **Same environment variables** - Minimal config changes
- **Compatible with quickslice-client-js** - Existing clients work unchanged

## Development

```bash
# Run tests
make test

# Run with hot reload
make dev

# Build binary
make build

# Run linter
make lint

# Generate SQL (if using sqlc)
make sqlc
```

## Roadmap

See [GitHub Issues](https://github.com/GainForest/hypergoat/issues) for detailed implementation tracking.

### Phase 1: Foundation
- [ ] Project structure and configuration
- [ ] Database executor abstraction
- [ ] Core repositories (records, actors, lexicons)

### Phase 2: Lexicon & GraphQL
- [ ] Lexicon JSON parser
- [ ] Runtime GraphQL schema builder
- [ ] Query resolvers with pagination

### Phase 3: Full GraphQL
- [ ] Mutation resolvers
- [ ] Join field generation
- [ ] DataLoader for N+1 prevention
- [ ] Aggregation support

### Phase 4: Real-time
- [ ] PubSub system
- [ ] Jetstream consumer
- [ ] GraphQL subscriptions

### Phase 5: OAuth
- [ ] OAuth 2.0 server
- [ ] DPoP implementation
- [ ] AT Protocol bridge

### Phase 6: Backfill & Admin
- [ ] Backfill worker
- [ ] CAR file parser
- [ ] Admin GraphQL API

## Contributing

Contributions are welcome! Please read our contributing guidelines before submitting PRs.

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.

## Acknowledgments

- [Quickslice](https://github.com/quickslice/quickslice) - The original Gleam implementation
- [AT Protocol](https://atproto.com/) - The underlying protocol
- [Bluesky](https://bsky.social/) - For building the ATmosphere

---

**Hypergoat** - *Fast, scalable AT Protocol indexing in Go*
