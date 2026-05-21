# Manual public GraphQL schema reload plan

## Problem

Hyperindex currently builds the public `/graphql` schema once during backend startup. Admin lexicon operations update the `lexicon` database table, but the running public GraphQL handler keeps using the schema that was built before those database changes.

That means operators must restart the backend process after registering, uploading, deleting, or re-adding lexicons before typed GraphQL fields appear or disappear.

## Current flow

1. `cmd/hyperindex/main.go` calls `setupGraphQL(...)` during startup.
2. `setupGraphQL(...)` creates a new `lexicon.Registry`.
3. It loads lexicons from:
   - `LEXICON_DIR`, or `testdata/lexicons` when `LEXICON_DIR` is unset.
   - the `lexicon` database table via `svc.lexicons.GetAll(ctx)`.
4. It parses each JSON document with `registry.ParseAndRegister(...)`.
5. It builds a public GraphQL schema with `hgraphql.NewHandler(registry, repos)`.
6. The resulting schema pointer is stored in the public GraphQL handler and reused for every request until process restart.

## Goal

Add an explicit admin action that rebuilds the public GraphQL schema in-process from the current lexicon sources, then swaps the live `/graphql` schema without restarting the indexer.

Operators should be able to:

1. Register, upload, delete, or re-add lexicons.
2. Click **Reload public GraphQL schema** in the Lexicons admin page, or run an admin GraphQL mutation.
3. Query the new typed schema immediately from `/graphql`.

## Non-goals

- Do not automatically reload the schema after every lexicon mutation in this first pass.
- Do not change Tap or Jetstream ingestion filters.
- Do not restart the backend process.
- Do not migrate existing records or reprocess indexed data.
- Do not require lexicons to be committed to a filesystem folder.

## Proposed admin API

Add an admin GraphQL mutation:

```graphql
mutation ReloadSchema {
  reloadSchema {
    success
    lexiconCount
    reloadedAt
    error
  }
}
```

Suggested result shape:

```graphql
type ReloadSchemaResult {
  success: Boolean!
  lexiconCount: Int!
  reloadedAt: String
  error: String
}
```

Behavior:

- Rebuild the public schema from current filesystem + database lexicons.
- If rebuild succeeds, atomically replace the live schema.
- If rebuild fails, keep serving the previous schema and return `success: false` with an actionable error.
- Require the same admin authorization as other mutating admin operations.

## Backend design

### 1. Extract schema loading into a reusable component

Create a small public schema manager, likely under `internal/graphql` or `internal/graphql/schema`, responsible for:

- Loading lexicons from configured filesystem path.
- Loading lexicons from `LexiconsRepository.GetAll(ctx)`.
- Building a fresh `lexicon.Registry`.
- Building a fresh `graphql.Schema`.
- Atomically swapping the current schema only after a successful build.

The manager should expose methods similar to:

```go
type PublicSchemaManager struct {
    // internal state guarded by a mutex or atomic.Value
}

func NewPublicSchemaManager(cfg Config, repos *resolver.Repositories) (*PublicSchemaManager, error)
func (m *PublicSchemaManager) Reload(ctx context.Context) (*ReloadSchemaResult, error)
func (m *PublicSchemaManager) Schema() *graphql.Schema
func (m *PublicSchemaManager) LexiconCount() int
```

Implementation notes:

- Prefer `atomic.Value` or an `RWMutex` for the current `*graphql.Schema`.
- Use a single reload mutex so two concurrent reloads cannot rebuild and swap at the same time.
- Keep the previous schema if loading or building the new schema fails.
- Log source counts separately if useful: filesystem count, database count, total registered count.
- Reuse the current `loadLexiconsFromDir` behavior, but move it out of `cmd/hyperindex/main.go` or wrap it so reload and startup use exactly the same path.

### 2. Make the public HTTP handler schema-aware

Change the public GraphQL handler so it asks the manager for the current schema per request instead of holding a startup-only schema pointer.

Current behavior:

```go
result := graphql.Do(graphql.Params{
    Schema: *h.schema,
    ...
})
```

Target behavior:

```go
schema := h.schemaProvider.Schema()
result := graphql.Do(graphql.Params{
    Schema: *schema,
    ...
})
```

If no schema is loaded, return a clear 503-style error explaining that the public GraphQL schema is unavailable and should be reloaded or the backend logs checked.

### 3. Update subscriptions to use the current schema

The subscription WebSocket handler currently receives a startup schema pointer. Update it to accept a schema provider too.

Recommended behavior:

- New subscription operations use the current schema.
- Existing WebSocket connections do not need to reconnect just because the schema changed, but clients may need to resubscribe if their query references newly-added fields.
- Keep subscription event delivery behavior unchanged.

### 4. Wire the manager in `cmd/hyperindex/main.go`

Startup should become:

1. Build the initial schema through the manager.
2. Register `/graphql` and `/graphql/ws` using the manager as schema provider.
3. Wire the admin resolver with a reload callback.

The existing `LexiconChangeCallback` should stay focused on ingestion collection updates for legacy Jetstream mode. Manual schema reload should use a separate callback, for example:

```go
type SchemaReloadCallback func(ctx context.Context) (*ReloadSchemaResult, error)
```

### 5. Add admin resolver support

In `internal/graphql/admin/resolvers.go`:

- Add `SchemaReloadCallback` field.
- Add `SetSchemaReloadCallback(...)`.
- Add `ReloadSchema(ctx)` resolver method.
- Return a helpful error if the callback is not configured.

In `internal/graphql/admin/schema.go`:

- Add `ReloadSchemaResult` GraphQL object type.
- Add `reloadSchema` mutation.

## Frontend design

Update the Lexicons page at `client/src/app/lexicons/page.tsx`.

Add a new admin-only section near the register/upload controls:

- Title: **Public GraphQL schema**
- Description: explains that registering/uploading lexicons stores them in the database, and this button reloads the live public `/graphql` schema without restarting the backend.
- Warning copy: schema reload does not change Tap/Jetstream ingestion filters.
- Button: **Reload schema**

Add a mutation in `client/src/lib/graphql/mutations.ts`:

```ts
export const RELOAD_SCHEMA = gql`
  mutation ReloadSchema {
    reloadSchema {
      success
      lexiconCount
      reloadedAt
      error
    }
  }
`;
```

UI behavior:

- Show loading state while reload is running.
- On success, show something like: `Reloaded public schema with 42 lexicons.`
- On failure, show the backend error and make clear that the previous public schema is still active.

## Documentation updates

Update user-facing docs that currently say a backend restart is required:

- `README.md`
- `client/README.md` if it still mentions redeploying or restarting after lexicon changes.
- Any Lexicons page copy that says restart is required.

New wording should say:

> After registering, uploading, deleting, or re-adding lexicons, run the `reloadSchema` admin mutation or click **Reload schema** on the Lexicons page. This rebuilds the public GraphQL schema in-place. Ingestion filters are configured separately.

## Tests

### Backend tests

Add tests covering:

1. Initial schema load from DB lexicons.
2. Registering/upserting a new lexicon in the DB does not affect the public schema until reload.
3. `Reload(ctx)` makes the new typed field available without reconstructing the whole server.
4. Invalid lexicon JSON causes reload failure and leaves the previous schema active.
5. Admin `reloadSchema` resolver calls the callback and returns success/error payloads correctly.
6. Subscription handler can retrieve the current schema through the provider.

### Frontend tests

If practical with existing frontend test setup:

1. Mutation document exports the expected `reloadSchema` fields.
2. Lexicons page shows the reload button for admins.
3. Success and error messages render after mutation results.

## Verification

Because this touches Go backend and Next.js frontend code, run:

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...

npm --prefix client run lint
npm --prefix client run test
npm --prefix client run build
```

If schema loading or repository behavior changes in a dialect-specific way, also run the PostgreSQL test command from `AGENTS.md`.

## Release note

This is operator-facing behavior, so add a Changie fragment with `Affects: operator` when implementing the feature.

## Open questions

1. Should `reloadSchema` also return filesystem/database source counts, or is total `lexiconCount` enough for the first version?
2. Should the UI offer a direct link to `/graphiql` after successful reload?
3. Should future work add automatic reload after lexicon mutations, or is explicit manual control preferred long-term?
