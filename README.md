<p align="center">
  <img src="hyperindex.png" alt="Hyperindex" width="600">
</p>

# Hyperindex (hi)

**A Go AT Protocol AppView server that indexes records and exposes them via GraphQL**

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for local setup, verification, and pull request guidance.

Hyperindex (hi) connects to the AT Protocol network, indexes records matching your configured Lexicons, and provides a GraphQL API for querying them. It's a Go port of [Quickslice](https://github.com/quickslice/quickslice).

> **Rename note:** this project was renamed from Hypergoat to Hyperindex.

## Agent skill

This repository includes a `hyperindex-consumer` Agent Skill for querying the hosted Hypercerts indexer, including common GraphQL workflows, filters, pagination, attachments, certified profiles, and external label filtering notes.

Install it with the [`skills`](https://www.skills.sh/) CLI:

```bash
npx skills add https://github.com/GainForest/hyperindex --skill hyperindex-consumer
```

## Quick Start

```bash
# Clone and run
git clone git@github.com:GainForest/hyperindex.git
cd hyperindex
cp .env.example .env
# Replace the placeholder secrets in .env (especially SECRET_KEY_BASE and ADMIN_API_KEY)
# before using the server in production or against real data.
go run ./cmd/hyperindex
```

Open http://localhost:8080/graphiql/admin to access the admin interface.

## Usage

### 1. Register Lexicons

Lexicons define the AT Protocol record types you want to index. Hyperindex supports two registration modes via the Admin GraphQL API at `/graphiql/admin`:

1. **Register by NSID** — use this when the lexicon can be resolved by its NSID.

   ```graphql
   mutation {
     registerLexicon(nsid: "org.hypercerts.claim.activity")
   }
   ```

2. **Upload a ZIP file** — use this for custom lexicons or lexicons that are not publicly resolvable. The ZIP should contain lexicon JSON files, which are stored in the database.

   ```graphql
   mutation {
     uploadLexicons(zipBase64: "...")
   }
   ```

Or place lexicon JSON files in a directory and set the `LEXICON_DIR` environment variable.

After registering by NSID or uploading a ZIP file, restart/redeploy the backend indexer for the new lexicons to appear in the public GraphQL schema and query list. The admin lexicon list updates immediately, but typed GraphQL queries are generated at backend startup.

**Example lexicons:**
- `org.hypercerts.claim.activity` - Hypercert claim activity
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
# Generate the minimal environment needed for Tap Docker
./scripts/generate-env.sh

# Start Tap + Hyperindex together
docker compose -f docker-compose.tap.yml up --build
```

The generator prompts for the Tap Docker values needed by `docker-compose.tap.yml`. Blank secret prompts auto-generate secure values with `openssl`, and rerunning it against an existing `.env` requires confirmation before overwriting. If you prefer to configure all environment variables manually, you can still copy `.env.example` to `.env` and edit it directly.

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
| `TAP_COLLECTION_FILTERS` | Comma-separated collection NSIDs for Tap sidecar record filtering; set independently from legacy `JETSTREAM_COLLECTIONS` | *(empty)* |

Tap Docker deployments also require `ADMIN_API_KEY` in `.env` because Hyperindex requires admin authentication at startup. `TAP_COLLECTION_FILTERS` is read by the Tap sidecar only; legacy `JETSTREAM_COLLECTIONS` remains part of Jetstream mode and is not used as a Tap filtering fallback.

**Local isolated Tap smoke stack (requires Docker):**

Use this when you want to test current local changes against Tap with the Hypercerts/Certified lexicon set mounted into the backend container:

```bash
make smoke-tap-local
```

The target starts a fresh Docker Compose project with Tap and Hyperindex, mounts `testdata/lexicons` into the backend so the listed lexicons are present in the generated GraphQL schema, uses `TAP_SIGNAL_COLLECTION=app.certified.actor.profile`, uses `TAP_COLLECTION_FILTERS=app.certified.*,org.hypercerts.*`, waits 20 seconds for Tap discovery/backfill to warm up after Hyperindex is ready, runs the API smoke suite against `http://127.0.0.1:8080` with `tests/api-smoke/expectations/local-tap.json`, retries every 15 seconds while Tap catches up, and then stops the stack. The filters use Tap's `.*` wildcard syntax for NSID prefixes. Set `HYPERINDEX_LOCAL_TAP_KEEP=1` to leave the stack running for debugging, or `HYPERINDEX_LOCAL_TAP_HOST_PORT=18080` if port 8080 is already in use.

#### Optional: External Labeler Streams

Hyperindex can subscribe to external ATProto labeler streams using `com.atproto.label.subscribeLabels` and persist raw label events locally. This is independent of Tap or Jetstream record ingestion: Tap/Jetstream store records, while labeler subscriptions store external labels.

External labels are written to dedicated tables:

- `label_subscription_state` — one cursor row per subscription URL
- `external_label` — one row per label event item

Stored external labels are exposed through public GraphQL subject lookups and record `externalLabels` fields. They are not merged into the existing local/admin `label` table.

```bash
LABELER_SUBSCRIBE_ENABLED=true
LABELER_SUBSCRIBE_URLS=wss://hyperlabel-proxy-test.up.railway.app/xrpc/com.atproto.label.subscribeLabels
# Optional reconnect backoff bounds:
# LABELER_SUBSCRIBE_RECONNECT_MIN=1s
# LABELER_SUBSCRIBE_RECONNECT_MAX=60s
```

| Variable | Description | Default |
|----------|-------------|---------|
| `LABELER_SUBSCRIBE_ENABLED` | Enable external labeler websocket ingestion | `false` |
| `LABELER_SUBSCRIBE_URLS` | Comma-separated `com.atproto.label.subscribeLabels` websocket URLs | *(empty)* |
| `LABELER_SUBSCRIBE_RECONNECT_MIN` | Minimum reconnect backoff | `1s` |
| `LABELER_SUBSCRIBE_RECONNECT_MAX` | Maximum reconnect backoff | `60s` |

Admins can remove a configured labeler URL from the Settings page or with the `removeLabelerSubscribeUrl` admin GraphQL mutation. Removal writes a persisted override for the subscription URL list; restart Hyperindex to stop any subscription goroutine that was already running for that URL.

If a labeler returns `FutureCursor` or `OutdatedCursor`, Hyperindex records a `FATAL_CURSOR ...` marker in `label_subscription_state.last_error`, stops retrying that labeler, and returns `503` from `/ready`. `/health` remains a liveness-only endpoint for process checks. Use `/stats` to see the affected labeler URL, `status: "fatal"`, `lastErrorCode`, and reset guidance. Repair requires resetting the saved cursor and replaying or purging labels as needed, then clearing `last_error`; because the subscription goroutine stops, restart Hyperindex after repair.

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

Typed GraphQL query field names are generated from lexicon NSIDs. For example, `org.hypercerts.claim.activity` becomes `orgHypercertsClaimActivity`. Newly registered or uploaded lexicons appear in these typed queries after the backend indexer restarts.

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

#### External labels

When external labeler ingestion is enabled, Hyperindex stores received ATProto labels locally and exposes them as generic subject metadata. Labels attach to DIDs or AT-URIs, not to app-specific fields inside a record.

```graphql
query {
  externalLabels(
    subjects: ["at://did:plc:abc/app.bsky.feed.post/3kabc"]
    values: ["high-quality"]
  ) {
    src
    uri
    cid
    val
    cts
  }
}
```

Generated record types and `GenericRecord` also include a virtual `externalLabels` field:

```graphql
query {
  appBskyFeedPost(first: 20) {
    edges {
      node {
        uri
        cid
        externalLabels(values: ["high-quality"]) {
          src
          val
          cts
        }
      }
    }
  }
}
```

By default, `externalLabels` returns only active labels: the latest label for each `(src, uri, val)` tuple, excluding latest negations and expired labels. Use `activeOnly: false` to inspect historical rows. Hyperindex only serves labels already ingested locally; it does not subscribe to arbitrary request-provided labelers.

Typed collection queries can filter records by external labels before pagination with `where.externalLabels`:

```graphql
query {
  appBskyFeedPost(
    first: 20
    where: {
      externalLabels: {
        has: { val: { eq: "high-quality" } }
        none: { val: { eq: "spam" } }
      }
    }
  ) {
    edges {
      node {
        uri
        externalLabels(values: ["high-quality"]) { src val }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

`where.externalLabels` decides which records qualify. The `node.externalLabels(...)` field decides which labels are displayed on each returned record, so repeat the same source/value constraints on the field if you only want to display labels used for filtering.

Generated record types and `GenericRecord` also include a virtual `certifiedProfileData` field when the `app.certified.actor.profile` lexicon is registered. This field resolves the author's `at://<did>/app.certified.actor.profile/self` record and can include external labels attached to that profile record:

```graphql
query {
  orgHypercertsClaimActivity(first: 20) {
    edges {
      node {
        uri
        did
        certifiedProfileData {
          displayName
          description
          externalLabels(values: ["test-account"]) { src val cts }
        }
      }
    }
  }
}
```

`certifiedProfileData` is nullable when the author has no Certified profile record. Its nested `externalLabels` field uses labels on the profile record URI only.

#### Filtering (`where`)

Typed collection queries accept a `where` argument with per-field filters:

| Operator | Types | Example |
|----------|-------|---------|
| `eq` | String, Int, Float, Boolean, DateTime | `{ title: { eq: "Hello" } }` |
| `neq` | String, Int, Float, DateTime | `{ status: { neq: "draft" } }` |
| `gt`, `lt`, `gte`, `lte` | Int, Float, DateTime | `{ score: { gt: 5, lte: 100 } }` |
| `in` | String, Int | `{ type: { in: ["post", "reply"] } }` |
| `contains` | String | `{ text: { contains: "forest" } }` |
| `startsWith` | String | `{ name: { startsWith: "Gain" } }` |
| `isNull` | Scalar fields and complex top-level fields | `{ optionalField: { isNull: true } }` |

Complex top-level fields such as arrays, refs, unions, objects, blobs, bytes, unknown values, and CID links support presence filtering only:

```graphql
where: {
  image: { isNull: false }
  contributors: { isNull: false }
}
```

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
| `/health` | Liveness check for the running process |
| `/ready` | Readiness check for database and configured labeler diagnostics |
| `/stats` | Server statistics and diagnostics |
| `/.well-known/oauth-authorization-server` | OAuth 2.0 server metadata |
| `/oauth/authorize` | OAuth authorization endpoint |
| `/oauth/token` | OAuth token endpoint |
| `/oauth/jwks` | JSON Web Key Set |

## Configuration

Create a `.env` file or set environment variables:

The `.env.example` file includes placeholder values for required secrets. After copying it to `.env`, replace those placeholders with real random secrets before running in production or against real data.

```bash
# Database (SQLite or PostgreSQL)
DATABASE_URL=sqlite:data/hyperindex.db
# DATABASE_URL=postgres://user:pass@localhost/hyperindex

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

# Tap record ingestion (recommended)
# TAP_ENABLED=true
# TAP_URL=ws://localhost:2480
# TAP_ADMIN_PASSWORD=replace-with-a-random-secret

# Optional external labeler ingestion. Stores raw labels locally and exposes them through public GraphQL.
# LABELER_SUBSCRIBE_ENABLED=true
# LABELER_SUBSCRIBE_URLS=wss://hyperlabel-proxy-test.up.railway.app/xrpc/com.atproto.label.subscribeLabels

# Jetstream (legacy real-time indexing)
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
- `activityBuckets` / `recentActivity` - Record indexing activity data
- `settings` - Server configuration

**Mutations:**
- `uploadLexicons` - Register new lexicons
- `deleteLexicon` - Remove a lexicon
- `backfillActor` - Backfill a specific user
- `triggerBackfill` - Full network backfill
- `populateActivity` - Populate activity from existing records
- `updateSettings` - Update server settings
- `removeLabelerSubscribeUrl` - Remove a configured external labeler subscription URL
- `resetAll` - Clear all data (requires confirmation)

## Architecture

```txt
Tap Sidecar ──→ Tap Consumer ──→ Records DB ──→ GraphQL API
                         │
                         └──→ Activity Log ──→ Admin Dashboard

Labeler WebSocket ──→ Labeler Subscriber ──→ External Labels DB

Legacy mode: Jetstream + Backfill ─────────→ Records DB
```

**Key Components:**
- **Tap Consumer** - Recommended record ingestion path backed by the Tap sidecar
- **Labeler Subscriber** - Optionally stores raw external `com.atproto.label.subscribeLabels` events and saved cursors
- **Jetstream Consumer** - Legacy real-time AT Protocol event ingestion
- **Backfill Worker** - Legacy historical import from relays
- **GraphQL Schema Builder** - Generates schema from Lexicons
- **Activity Tracker** - Logs record indexing activity for monitoring in the `indexing_activity` table

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

Hyperindex was incubated and created by [GainForest](https://gainforest.earth) and [Claude Opus 4.5](https://www.anthropic.com/claude) (Anthropic). It has since been moved to [hypercerts-org](https://github.com/hypercerts-org) for community maintenance.

## License

Apache License 2.0

## Acknowledgments

- [GainForest](https://gainforest.earth) & [Claude Opus 4.5](https://www.anthropic.com/claude) - Original creators
- [Quickslice](https://github.com/quickslice/quickslice) - Original Gleam implementation
- [AT Protocol](https://atproto.com/) - The underlying protocol
