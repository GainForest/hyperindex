<p align="center">
  <img src="hypergoat.png" alt="Hyperindex" width="600">
</p>

# Hyperindex (hi)

**A Go AT Protocol AppView server that indexes records and exposes them via GraphQL**

*Formerly known as Hypergoat.*

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for local setup, verification, and pull request guidance.

Hyperindex (hi) connects to the AT Protocol network, indexes records matching your configured Lexicons, and provides a GraphQL API for querying them. It's a Go port of [Quickslice](https://github.com/quickslice/quickslice).

## Quick Start

```bash
# Clone and run
git clone https://github.com/GainForest/hypergoat.git
cd hypergoat
cp .env.example .env
# Replace the placeholder secrets in .env (especially SECRET_KEY_BASE and ADMIN_API_KEY)
# before using the server in production or against real data.
go run ./cmd/hypergoat
```

Open http://localhost:8080/graphiql/admin to access the admin interface.

## Usage

### 1. Register Lexicons

Lexicons define the AT Protocol record types you want to index. Register them via the Admin GraphQL API at `/graphiql/admin`:

```graphql
mutation {
  uploadLexicons(files: [...])  # Upload lexicon JSON files
}
```

Or place lexicon JSON files in a directory and set `LEXICON_DIR` environment variable.

**Example lexicons:**
- `app.bsky.feed.post` - Bluesky posts
- `app.bsky.feed.like` - Likes
- `app.bsky.actor.profile` - User profiles

### 2. Start Indexing

#### Using Tap (Recommended)

[Tap](https://github.com/bluesky-social/indigo/tree/main/cmd/tap) is Bluesky's official sidecar utility for consuming AT Protocol events. It is the recommended way to run Hyperindex because it provides:

- **Cryptographic verification** — verifies repo structure, MST integrity, and identity signatures
- **Ordering guarantees** — strict per-repo event ordering, no backfill/live race conditions
- **At-least-once delivery** — ack-based protocol ensures no events are lost on crash
- **Identity tracking** — handle changes and account status updates are handled automatically
- **Simplified architecture** — Tap manages backfill automatically; no separate backfill worker needed

**Run with Tap sidecar:**

```bash
# Copy and configure environment
cp .env.example .env
# Set TAP_ADMIN_PASSWORD and other vars in .env

# Start Tap + Hyperindex together
docker compose -f docker-compose.tap.yml up --build
```

**Add repos to track via Tap admin API:**

```bash
# Add a specific repo (DID) for Tap to index
curl -X POST http://localhost:2480/repos/add \
  -u "admin:${TAP_ADMIN_PASSWORD}" \
  -H "Content-Type: application/json" \
  -d '{"dids": ["did:plc:your-did-here"]}'
```

**Auto-discovery with `TAP_SIGNAL_COLLECTION`:**

Set `TAP_SIGNAL_COLLECTION` to a collection NSID (e.g. `app.bsky.feed.post`) and Tap will automatically discover and index all repos that publish records in that collection. This replaces the need for a manual full-network backfill.

```bash
TAP_SIGNAL_COLLECTION=app.bsky.feed.post docker compose -f docker-compose.tap.yml up
```

**Tap environment variables:**

| Variable | Description | Default |
|----------|-------------|---------|
| `TAP_ENABLED` | Enable Tap consumer (disables Jetstream+Backfill) | `false` |
| `TAP_URL` | WebSocket URL of the Tap sidecar | `ws://localhost:2480` |
| `TAP_ADMIN_PASSWORD` | Password for Tap's admin HTTP API | *(required for docker-compose.tap.yml)* |
| `TAP_DISABLE_ACKS` | Disable ack-based delivery (useful for debugging) | `false` |
| `TAP_SIGNAL_COLLECTION` | Collection NSID for auto-discovery of repos | *(empty)* |

#### Legacy Mode: Jetstream + Backfill

> **Note:** Jetstream+Backfill mode is the legacy ingestion path. It lacks cryptographic verification and ordering guarantees. Use Tap (above) for new deployments.

Once lexicons are registered, Hyperindex automatically:
- **Connects to Jetstream** for real-time events
- **Indexes matching records** to your database

To backfill historical data, use the admin API:

```graphql
mutation {
  triggerBackfill  # Full network backfill for registered collections
}

# Or backfill a specific user
mutation {
  backfillActor(did: "did:plc:...")
}
```

### 3. Query via GraphQL

Access your indexed data at `/graphql`:

```graphql
# Generic query — all records by collection
query {
  records(collection: "app.bsky.feed.post", first: 20) {
    edges {
      node { uri did collection value }
      cursor
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}

# Typed queries — with filtering, sorting, and field-level access
query {
  appBskyFeedPost(
    where: { text: { contains: "hello" }, did: { eq: "did:plc:..." } }
    sortBy: "createdAt"
    sortDirection: DESC
    first: 10
  ) {
    edges {
      node {
        uri
        did
        rkey
        text
        createdAt
      }
    }
    totalCount
    pageInfo { hasNextPage hasPreviousPage endCursor }
  }
}

# Backward pagination
query {
  appBskyFeedPost(last: 10, before: "cursor_value") {
    edges { node { uri text } }
    pageInfo { hasPreviousPage startCursor }
  }
}

# Cross-collection text search
query {
  search(query: "climate", collection: "app.bsky.feed.post", first: 20) {
    edges {
      node { uri did collection value }
    }
  }
}
```

#### Filtering (`where`)

Typed collection queries accept a `where` argument with per-field filters:

| Operator | Types | Example |
|----------|-------|---------|
| `eq` | All | `{ title: { eq: "Hello" } }` |
| `neq` | All | `{ status: { neq: "draft" } }` |
| `gt`, `lt`, `gte`, `lte` | Int, Float, DateTime | `{ score: { gt: 5, lte: 100 } }` |
| `in` | String, Int, Float | `{ type: { in: ["post", "reply"] } }` |
| `contains` | String | `{ text: { contains: "forest" } }` |
| `startsWith` | String | `{ name: { startsWith: "Gain" } }` |
| `isNull` | All | `{ optionalField: { isNull: true } }` |

Every `where` input also includes a `did` field for filtering by author DID.

#### Sorting (`sortBy`, `sortDirection`)

Typed queries support sorting by any scalar field:

```graphql
query {
  appBskyFeedPost(sortBy: "createdAt", sortDirection: ASC, first: 10) {
    edges { node { uri createdAt } }
  }
}
```

Default sort is `indexed_at DESC` (newest first). Available sort fields are generated per-collection from the lexicon schema.

#### Pagination

- **Forward**: `first` + `after` (default: 20, max: 100)
- **Backward**: `last` + `before`
- **`totalCount`**: Returned when requested (opt-in, computed only when selected)
- Cannot use `first`/`after` and `last`/`before` simultaneously

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `/graphql` | Public GraphQL API |
| `/graphql/ws` | GraphQL subscriptions (WebSocket) |
| `/admin/graphql` | Admin GraphQL API |
| `/graphiql` | GraphQL playground (public API) |
| `/graphiql/admin` | GraphQL playground (admin API) |
| `/health` | Health check |
| `/stats` | Server statistics |
| `/.well-known/oauth-authorization-server` | OAuth 2.0 server metadata |
| `/oauth/authorize` | OAuth authorization endpoint |
| `/oauth/token` | OAuth token endpoint |
| `/oauth/jwks` | JSON Web Key Set |

## Configuration

Create a `.env` file or set environment variables:

The `.env.example` file includes placeholder values for required secrets. After copying it to `.env`, replace those placeholders with real random secrets before running in production or against real data.

```bash
# Database (SQLite or PostgreSQL)
DATABASE_URL=sqlite:data/hypergoat.db
# DATABASE_URL=postgres://user:pass@localhost/hypergoat

# Server
HOST=127.0.0.1
PORT=8080
EXTERNAL_BASE_URL=http://localhost:8080

# Admin access (comma-separated DIDs)
# Managed via deployment environment; shown read-only in the admin UI.
ADMIN_DIDS=did:plc:your-did-here

# Security — required for session encryption (min 64 chars)
SECRET_KEY_BASE=your-secret-key-at-least-64-characters-long-generate-with-openssl-rand

# Admin API key — required at startup; the server will not start without it.
# Also enables trusted X-User-DID proxy requests when the request includes:
# X-Admin-API-Key: <key>
# Example: openssl rand -base64 32
ADMIN_API_KEY=replace-with-a-random-secret

# WebSocket origins — comma-separated allowed origins for subscriptions.
# Unset or empty allows all origins. Set a comma-separated list to restrict origins; "*" also allows all origins.
# ALLOWED_ORIGINS=https://your-frontend.vercel.app

# Jetstream (real-time indexing)
# Collections are auto-discovered from registered lexicons
# Or specify manually:
# JETSTREAM_COLLECTIONS=app.bsky.feed.post,app.bsky.feed.like

# Backfill
BACKFILL_RELAY_URL=https://relay1.us-west.bsky.network
```

## Docker

```bash
docker compose up --build
```

Or build manually:

```bash
docker build -t hyperindex .
docker run -p 8080:8080 -v ./data:/data hyperindex
```

## Admin API

The admin API at `/admin/graphql` provides:

**Queries:**
- `statistics` - Record, actor, lexicon counts
- `lexicons` - List registered lexicons
- `activityBuckets` / `recentActivity` - Jetstream activity data
- `settings` - Server configuration

**Mutations:**
- `uploadLexicons` - Register new lexicons
- `deleteLexicon` - Remove a lexicon
- `backfillActor` - Backfill a specific user
- `triggerBackfill` - Full network backfill
- `populateActivity` - Populate activity from existing records
- `updateSettings` - Update server settings
- `resetAll` - Clear all data (requires confirmation)

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                   Hyperindex (hi) Server                  │
├─────────────────────────────────────────────────────────┤
│                                                         │
│  Jetstream ──→ Consumer ──→ Records DB ──→ GraphQL API │
│                    │                                    │
│              Activity Log ──→ Admin Dashboard           │
│                                                         │
│  Backfill Worker ──→ AT Protocol Relay ──→ Records DB  │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

**Key Components:**
- **Jetstream Consumer** - Subscribes to real-time AT Protocol events
- **Backfill Worker** - Imports historical data from relays
- **GraphQL Schema Builder** - Generates schema from Lexicons
- **Activity Tracker** - Logs all indexing activity for monitoring

## Development

```bash
# One-time: enable tracked git hooks
make hooks-install

# Run with hot reload
make dev

# Run tests
make test
go test -v -run TestName ./...  # Single test

# Lint
make lint

# Build binary
make build
```

## Changelog workflow

We use [Changie](https://github.com/miniscruff/changie) for release-note fragments.

```bash
go install github.com/miniscruff/changie@v1.24.0
make tools
make changie-new
```

- Add a changelog fragment for user-facing changes, operator-facing changes, bug fixes, and other work that should appear in the next release notes.
- You do not need a fragment for docs-only edits, tests-only changes, or internal refactors that do not affect behavior.
- Maintainers run **Prepare release notes PR** on `main` to batch pending fragments and open or update a release PR.
- After the release PR is merged, maintainers run **Publish release tag and GitHub Release** on `main` to create the `vX.Y.Z` tag and publish the matching GitHub Release from the generated `.changes` version file.
- See `docs/changelog-workflow.md` for the full maintainer runbook, token requirements, and validation workflow details.

Recommended fragment kinds:

- `added` — new functionality
- `breaking` — behavior or interface changes that require users, operators, or developers to adapt
- `changed` — changed behavior, enhancements, or workflow changes
- `deprecated` — functionality that still works now but should be migrated away from
- `removed` — functionality removed
- `fixed` — bug fixes
- `security` — security-relevant fixes or hardening worth calling out

### Affects and body guidance

`Affects` describes who or what the change impacts most. Use the smallest audience that still fits the change.

Recommended values:

- `user` — changes that affect product behavior, APIs, queries, or UX
- `operator` — changes that affect deployment, configuration, monitoring, or runtime behavior
- `developer` — changes that affect contributor workflows, tooling, tests, or documentation

Write the release-note body as a short description of the impact, not the implementation. Good bodies explain what changed, why it matters, and what readers should expect. Bad bodies focus on internal code paths, file names, or implementation details instead of the visible effect.

### Release PR automation

- Merge feature PRs with their Changie fragments into `main`.
- Run **Prepare release notes PR** from GitHub Actions on `main` and choose `auto`, `patch`, `minor`, or `major` batching.
- If unreleased fragments exist, the workflow runs `go build ./...`, `go test ./...`, `changie batch <release_type>`, and `changie merge`, then creates or updates a PR from `release/changelog` back into `main` for review.
- Merge the generated release PR after reviewing the versioned `.changes` file and `CHANGELOG.md` diff.
- Run **Publish release tag and GitHub Release** on `main` after the PR is merged.
- Publish uses the latest generated `.changes/vX.Y.Z.md` or `.changes/X.Y.Z.md` release file as the GitHub Release notes body; newer unreleased fragments for the next cycle do not block publishing that prepared version.

### Local pre-commit linting

This repo includes a tracked pre-commit hook at `.githooks/pre-commit`.

- It runs on **staged Go files only**
- Checks staged `.go` files are already `gofmt`-formatted (fails if not)
- Runs `golangci-lint` on changed packages before commit
- Requires **Bash 4+** (`mapfile` and associative arrays); macOS users may need `brew install bash`

If you need to bypass it for an emergency local commit:

```bash
SKIP_GOLANGCI=1 git commit -m "..."
```

## Database Support

- **SQLite** - Default, great for development and small deployments
- **PostgreSQL** - Recommended for production

Migrations run automatically on startup.

## History

Hyperindex was incubated and created by [GainForest](https://gainforest.earth) and [Claude Opus 4.5](https://www.anthropic.com/claude) (Anthropic), originally under the name *Hypergoat*. It has since been moved to [hypercerts-org](https://github.com/hypercerts-org) for community maintenance.

## License

Apache License 2.0

## Acknowledgments

- [GainForest](https://gainforest.earth) & [Claude Opus 4.5](https://www.anthropic.com/claude) - Original creators
- [Quickslice](https://github.com/quickslice/quickslice) - Original Gleam implementation
- [AT Protocol](https://atproto.com/) - The underlying protocol
