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

## Confirmed decisions and invariants

- Malformed filesystem lexicons must fail schema reload.
- Malformed uploaded or registered lexicons must be rejected before they are stored in the database.
- Malformed lexicons already present in the database must fail schema reload and produce an actionable error.
- Reload must build and validate a completely new registry/schema off to the side before touching the live public schema.
- A failed reload must keep serving the previous working public schema when one exists.
- If no working schema has ever loaded, public GraphQL requests should return a clear 503-style response until the underlying lexicon issue is fixed and a reload succeeds.
- On reload failure, `lexiconCount` reports the currently active schema count, not the attempted source count.

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

- Rebuild the public schema from current filesystem + database lexicons in a fresh registry/schema without mutating live handler state.
- Treat malformed filesystem lexicons and malformed persisted database lexicons as reload failures.
- If rebuild succeeds, atomically replace the live schema snapshot.
- If rebuild fails, keep serving the previous schema and return `success: false` with an actionable error.
- On rebuild failure, return `lexiconCount` for the currently active schema. If no schema is active, return `0`.
- Rebuild failures should be represented in the payload as `success: false`; they should not also become GraphQL resolver errors. Reserve resolver errors for authorization, missing callback wiring, or unexpected infrastructure failures.
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

func NewPublicSchemaManager(cfg Config, repos *resolver.Repositories) *PublicSchemaManager
func (m *PublicSchemaManager) Reload(ctx context.Context) (*ReloadSchemaResult, error)
func (m *PublicSchemaManager) Snapshot() PublicSchemaSnapshot
func (m *PublicSchemaManager) Schema() *graphql.Schema
func (m *PublicSchemaManager) LexiconCount() int
```

Implementation notes:

- Prefer `atomic.Value` or an `RWMutex` for an immutable `PublicSchemaSnapshot` containing the schema pointer, active lexicon count, last successful reload time, and any source counts worth logging.
- Use a single reload mutex so two concurrent reloads cannot rebuild and swap at the same time.
- Build the fresh registry and `graphql.Schema` completely before swapping the snapshot.
- Keep the previous snapshot if loading, parsing, validation, or schema building fails.
- Construct the manager even when the initial schema cannot be built; register handlers anyway so the admin API can recover after the bad lexicon is fixed.
- Fail reload on malformed filesystem lexicons instead of silently skipping them.
- Fail reload on malformed database lexicons. Upload/register validation should prevent new bad rows, but reload should still protect against existing bad data.
- Log source counts separately if useful: filesystem count, database count, total registered count.
- Move `loadLexiconsFromDir` out of `cmd/hyperindex/main.go` or wrap it so reload and startup use exactly the same strict path.

### 2. Validate admin lexicon writes before persistence

Before register/upload stores lexicon JSON in the database, validate the full lexicon document using the same parser path that schema reload uses.

Validation behavior:

- Reject malformed JSON or lexicon documents before writing them to the `lexicon` table.
- Return an actionable admin error that says which lexicon failed and why.
- Keep the existing DB unchanged when validation fails.
- If the request includes a lexicon ID separately from the JSON body, reject mismatches instead of storing ambiguous data.

This prevents new invalid uploaded lexicons from becoming reload-time failures. Reload still needs to treat invalid existing database rows as failures so older bad data cannot poison the live schema silently.

### 3. Make the public HTTP handler schema-aware

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

### 4. Update subscriptions to use the current schema

The subscription WebSocket handler currently receives a startup schema pointer. Update it to accept a schema provider too.

Recommended behavior:

- Fetch or snapshot the current schema when a client starts a subscription operation, not only when the WebSocket connection is created.
- New subscription operations on an existing WebSocket connection should use the current schema after reload.
- Existing active subscription operations can keep using the schema they were validated with. Clients may need to resubscribe if their query references newly added fields.
- Keep subscription event delivery behavior unchanged.

### 5. Wire the manager in `cmd/hyperindex/main.go`

Startup should become:

1. Construct the schema manager.
2. Attempt an initial `Reload(ctx)` through the manager and log a clear startup error if it fails.
3. Register `/graphql` and `/graphql/ws` using the manager as schema provider, even if the initial schema is unavailable.
4. Wire the admin resolver with a reload callback so operators can recover after fixing bad lexicons without restarting the process.

The existing `LexiconChangeCallback` should stay focused on ingestion collection updates for legacy Jetstream mode. Manual schema reload should use a separate callback, for example:

```go
type SchemaReloadCallback func(ctx context.Context) (*ReloadSchemaResult, error)
```

### 6. Add admin resolver support

In `internal/graphql/admin/resolvers.go`:

- Add `SchemaReloadCallback` field.
- Add `SetSchemaReloadCallback(...)`.
- Add `ReloadSchema(ctx)` resolver method.
- Return a helpful error if the callback is not configured.
- Return reload build/validation failures as a `ReloadSchemaResult` with `success: false` instead of converting them into GraphQL execution errors.

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
- On failure, show the backend error and make clear that the previous public schema is still active. If `lexiconCount` is non-zero, make clear that it is the active schema count, not the failed attempt count.

## Documentation updates

Update user-facing docs that currently say a backend restart is required:

- `README.md`
- `client/README.md` if it still mentions redeploying or restarting after lexicon changes.
- Any Lexicons page copy that says restart is required.

New wording should say:

> After registering, uploading, deleting, or re-adding lexicons, run the `reloadSchema` admin mutation or click **Reload schema** on the Lexicons page. This rebuilds the public GraphQL schema in-place. If reload fails, Hyperindex keeps serving the previous working public schema. Ingestion filters are configured separately.

## Tests

### Backend tests

Add tests covering:

1. Initial schema load from DB lexicons.
2. Registering/upserting a new lexicon in the DB does not affect the public schema until reload.
3. `Reload(ctx)` makes the new typed field available without reconstructing the whole server.
4. Malformed filesystem lexicon JSON causes reload failure and leaves the previous schema active.
5. Malformed database lexicon JSON causes reload failure and leaves the previous schema active.
6. Failed reload returns `success: false`, an actionable error, and the currently active `lexiconCount`.
7. Public HTTP handler returns a clear 503-style GraphQL response when no schema is loaded.
8. Admin lexicon upload/register rejects invalid lexicon JSON before storing it.
9. Admin `reloadSchema` resolver calls the callback and returns success/error payloads correctly.
10. Unauthorized admin reload is rejected by the same admin authorization path as other mutations.
11. Missing reload callback returns a helpful admin error.
12. Subscription handler snapshots the current schema when a subscription operation starts, including a new operation on an already-open WebSocket after reload.

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
