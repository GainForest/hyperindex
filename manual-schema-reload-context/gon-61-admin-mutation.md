# GON-61 / MSR-003 read-only implementation handoff: admin `reloadSchema` mutation

## Scope and status

- Task: add an admin-gated `reloadSchema` mutation and resolver callback after GON-59 (`PublicSchemaManager` / fallback snapshots) lands.
- Current branch is pre-GON-59: no `PublicSchemaManager`, `ReloadSchemaResult`, or schema-provider symbols were found under `internal/**/*.go`.
- This handoff is read-only analysis. Do not conflate schema reload with the existing lexicon ingestion callback.

## Relevant files/functions

### Requirement sources

- `docs/manual-schema-reload-plan.md:48-82`
  - Defines admin API:
    - `reloadSchema { success lexiconCount reloadedAt error }`
    - `ReloadSchemaResult { success: Boolean!, lexiconCount: Int!, reloadedAt: String, error: String }`
  - Reload build/validation failures must be returned as `success: false` payloads, not GraphQL resolver errors.
  - Resolver errors are for authorization, missing callback wiring, or unexpected infrastructure failures.
- `docs/manual-schema-reload-plan.md:174-199`
  - Startup wiring should create/use schema manager and wire admin resolver with a separate reload callback.
  - Existing `LexiconChangeCallback` must remain focused on ingestion collection updates.
- `docs/manual-schema-reload-plan.md:245-262`
  - Backend tests include admin resolver callback, unauthorized access, missing callback, and failure payload behavior.
- `plans/manual-schema-reload-linear-issues.json:30-38`
  - MSR-003 depends on MSR-001 / GON-59.
- `plans/manual-schema-reload-linear-created.json:39-49`
  - MSR-003 is Linear `GON-61`.

### Admin GraphQL schema/resolver files

- `internal/graphql/admin/resolvers.go:41-58`
  - Existing callback types:
    - `BackfillCallback func(ctx context.Context, did string) error`
    - `FullBackfillCallback func(ctx context.Context) error`
    - `LexiconChangeCallback func(collections []string) error`
  - `Resolver` currently stores `backfillCallback`, `fullBackfillCallback`, `lexiconChangeCallback`.
  - Add a separate `SchemaReloadCallback` field here. Do not reuse `LexiconChangeCallback`.
- `internal/graphql/admin/resolvers.go:70-83`
  - Existing setter pattern: `SetBackfillCallback`, `SetFullBackfillCallback`, `SetLexiconChangeCallback` simply assign callback fields.
  - Add `SetSchemaReloadCallback(...)` beside these.
- `internal/graphql/admin/resolvers.go:85-105`
  - `notifyLexiconChange` pulls DB lexicons and calls `LexiconChangeCallback` for Jetstream collection updates, logging errors without failing lexicon mutation.
  - `reloadSchema` must not call this; reload is a public GraphQL schema operation, not ingestion filter mutation.
- `internal/graphql/admin/resolvers.go:516-590`
  - `RegisterLexicon` and `DeleteLexicon` call `notifyLexiconChange` after DB write/delete.
  - Keep that behavior unchanged; manual schema reload remains an explicit separate operator action.
- `internal/graphql/admin/schema.go:265-670`
  - `buildMutationType` defines admin mutations in one `graphql.Fields` map.
  - Each admin-only mutation starts with `if err := requireAdmin(p.Context); err != nil { return nil, err }`.
  - Add `reloadSchema` here with same auth pattern.
- `internal/graphql/admin/schema.go:672-693`
  - Auth context keys and `requireAdmin`; unauthorized admin mutations return `admin privileges required` as a GraphQL resolver error.
- `internal/graphql/admin/types.go:328-346`
  - `LexiconType` pattern for GraphQL object type definitions.
  - Add `ReloadSchemaResultType` near other object types, with real descriptions for all fields.
- `internal/graphql/admin/handler.go:24-41`
  - `NewHandler` creates the resolver, builds schema immediately, and returns handler.
  - It is safe to set callbacks after `NewHandler`; schema field closures capture `b.resolver`.
- `internal/graphql/admin/handler.go:83-121`
  - Auth is extracted from OAuth middleware context or from `X-User-DID` only when `X-Admin-API-Key` matches configured admin API key.
  - Context is populated with `ContextWithAuth` before GraphQL execution.
- `internal/graphql/admin/handler.go:123-131`
  - Any GraphQL errors produce HTTP 400. Successful payloads, including `success:false`, should have no GraphQL errors and should return HTTP 200.

### Main wiring files

- `cmd/hyperindex/main.go:139-144`
  - Current startup creates admin handler before public GraphQL setup:
    - `adminHandler := setupAdmin(...)`
    - `collections := setupGraphQL(...)`
  - After GON-59, schema manager must be available for wiring. Either reorder setup or wire callback immediately after `setupGraphQL` returns/creates the manager.
- `cmd/hyperindex/main.go:440-528`
  - `setupAdmin` builds admin repos, OAuth middleware, `admin.NewHandler`, backfill callbacks, routes `/admin/graphql`, and GraphiQL.
  - `setupAdmin` currently does not know about public schema manager.
- `cmd/hyperindex/main.go:588-665`
  - Current `setupGraphQL` loads filesystem + DB lexicons, builds a startup-only public handler, and returns Jetstream collections.
  - GON-59 should replace or adapt this to construct/use a schema manager. GON-61 should only consume that manager via callback.
- `cmd/hyperindex/main.go:722-752`
  - `SetLexiconChangeCallback` is wired only in Jetstream mode to dynamically update ingestion collections.
  - Do not put schema reload wiring here unless you keep it separate and available in Tap mode too.
- `cmd/hyperindex/main.go:831-840`
  - Tap mode rewires backfill callbacks only; schema reload callback must also work in Tap mode because it is unrelated to ingestion.
- `cmd/hyperindex/main.go:897-921`
  - Current `loadLexiconsFromDir` skips parse errors. GON-59 is expected to make reload strict; GON-61 should not reimplement this.

### Existing admin tests to extend

- `internal/graphql/admin/handler_test.go:24-33`
  - `newTestAdminHandler` creates a handler with empty repos and no middleware. This is sufficient for callback-only reload tests.
- `internal/graphql/admin/handler_test.go:90-121`
  - Pattern for authenticated request via valid `X-Admin-API-Key` + `X-User-DID`.
- `internal/graphql/admin/handler_test.go:124-150`
  - Pattern for invalid/missing admin API key; `X-User-DID` is ignored.
- `internal/graphql/admin/handler_test.go:299-324`
  - Schema/mutation introspection test pattern.
- `internal/graphql/admin/handler_test.go:386-404`
  - Unit coverage for `requireAdmin`.

## Current admin mutation/auth pattern

1. Admin routes use `adminHandler.OptionalAuth()` in `cmd/hyperindex/main.go:477-479`; introspection/public admin queries can be unauthenticated.
2. Mutations themselves enforce admin auth in `schema.go`; each admin-only mutation calls `requireAdmin` as the first resolver step.
3. Direct `ServeHTTP` tests can authenticate with headers:
   - `X-Admin-API-Key: <configured key>`
   - `X-User-DID: <DID present in cfg.AdminDIDs>`
4. If auth fails, resolver returns an error; the handler writes HTTP 400 because `len(result.Errors) > 0`.
5. For `reloadSchema`, expected reload build/validation failures must not return resolver errors. The callback/resolver must return a normal payload with `success: false`, `error: <actionable message>`, and active `lexiconCount`.

## Proposed implementation steps

1. Add an admin result/callback contract in `internal/graphql/admin/resolvers.go`.
   - Suggested shape:
     - `type ReloadSchemaResult struct { Success bool; LexiconCount int; ReloadedAt *time.Time or string; Error string }`
     - `type SchemaReloadCallback func(ctx context.Context) (*ReloadSchemaResult, error)`
   - If GON-59 defines its own result type, either:
     - use that type directly if it does not create an import cycle, or
     - keep an admin-local result type and convert in `cmd/hyperindex/main.go`.
   - Callback contract must be explicit: expected rebuild/validation failures are returned as `result.Success == false` with `err == nil`; only unexpected infrastructure/programming failures use `err`.
2. Add `schemaReloadCallback SchemaReloadCallback` to `Resolver` and `SetSchemaReloadCallback(cb SchemaReloadCallback)` next to existing callback setters.
3. Add `func (r *Resolver) ReloadSchema(ctx context.Context) (map[string]interface{} or *ReloadSchemaResult, error)`.
   - If callback is nil: return helpful resolver error, e.g. `schema reload callback not configured`.
   - If callback returns error: return GraphQL resolver error with context, e.g. `failed to reload public GraphQL schema: ...`.
   - If callback returns nil result with nil error: treat as unexpected resolver error.
   - If callback returns `{Success:false,...}` with nil error: return it as data, with no GraphQL error.
   - Format `reloadedAt` as RFC3339 string if the callback uses `time.Time`; return `nil`/`null` when absent.
4. Add `ReloadSchemaResultType` in `internal/graphql/admin/types.go`.
   - Fields:
     - `success: Boolean!`
     - `lexiconCount: Int!`
     - `reloadedAt: String`
     - `error: String`
   - Include descriptions explaining active-count semantics on failure.
5. Add `reloadSchema` mutation in `internal/graphql/admin/schema.go`.
   - Type should be `graphql.NewNonNull(ReloadSchemaResultType)` if resolver guarantees non-nil payload on non-error paths.
   - Resolver must call `requireAdmin` first, then `b.resolver.ReloadSchema(p.Context)`.
6. Wire in `cmd/hyperindex/main.go` after GON-59 manager exists.
   - Preferred helper: `configureSchemaReloadCallback(adminHandler, publicSchemaManager)` called once when both are non-nil.
   - Keep it independent of `configureBackfillCallbacks` and `SetLexiconChangeCallback`.
   - Ensure it runs in both Tap and Jetstream modes.
   - Adapter example at concept level: `adminHandler.Resolver().SetSchemaReloadCallback(func(ctx context.Context) (*admin.ReloadSchemaResult, error) { return convert(publicSchemaManager.Reload(ctx)) })`.
7. Add an operator-facing Changie fragment unless the parent implementation deliberately centralizes the changelog in another issue. This mutation is an operator-visible API change.

## Interface needs from GON-59

GON-61 needs GON-59 to expose, at minimum:

- A long-lived public schema manager instance available from startup wiring in `cmd/hyperindex/main.go`.
- An exported reload method callable by admin resolver callback:
  - likely `Reload(ctx context.Context) (*ReloadSchemaResult, error)` or equivalent.
- A result model containing:
  - `Success bool`
  - `LexiconCount int` for the currently active schema after the attempt; on failure this is the previous active schema count or `0` when no active schema exists.
  - `ReloadedAt` optional timestamp/string for successful reloads.
  - `Error` actionable message for expected reload failures.
- A clear error contract:
  - malformed filesystem/DB lexicons and schema-build validation failures => result with `Success:false`, `err:nil`.
  - unexpected infrastructure/programming failures => non-nil `err`.
- Import-cycle-safe placement for the result type.
  - Admin package can import a lower-level schema-manager package or the root `internal/graphql` package only if that package does not import `internal/graphql/admin`.
  - If uncertain, define an admin-local payload type and convert in `main`.

## Tests/validation to propose

### Unit/schema tests in `internal/graphql/admin`

1. `TestReloadSchemaResultTypeFields`
   - Assert `ReloadSchemaResultType` exists and has `success`, `lexiconCount`, `reloadedAt`, `error`.
2. `TestSchemaIncludesReloadSchemaMutation`
   - Build handler via `newTestAdminHandler`.
   - Assert `handler.Schema().MutationType().Fields()["reloadSchema"]` exists and returns `ReloadSchemaResult`.
3. `TestResolver_ReloadSchema_Success`
   - Create resolver, set callback returning success payload.
   - Assert callback called once and payload fields are preserved/formatted.
4. `TestResolver_ReloadSchema_FailurePayloadDoesNotError`
   - Callback returns `Success:false`, `LexiconCount:7`, `Error:"failed to parse database lexicon app.example.bad: ..."`, nil error.
   - Assert resolver returns nil error and payload includes failure details.
5. `TestResolver_ReloadSchema_MissingCallback`
   - No callback set.
   - Assert error contains `schema reload callback not configured`.

### HTTP/admin auth tests in `internal/graphql/admin/handler_test.go`

Use mutation:

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

1. Success path
   - Handler with admin DID/API key.
   - Set schema reload callback.
   - Send headers `X-Admin-API-Key` and `X-User-DID` for an admin.
   - Expect HTTP 200, no GraphQL errors, `data.reloadSchema.success == true`, callback called once.
2. Reload failure payload
   - Authorized request.
   - Callback returns `success:false` payload with active `lexiconCount` and error string.
   - Expect HTTP 200, no GraphQL errors, data contains failure payload.
3. Unauthorized access
   - No headers, or wrong `X-Admin-API-Key` with `X-User-DID`.
   - Callback should not be called.
   - Expect HTTP 400 and GraphQL error message `admin privileges required`.
4. Missing callback
   - Authorized request but no `SetSchemaReloadCallback` call.
   - Expect HTTP 400 and helpful GraphQL error; this is an intended resolver error.

### Targeted validation commands

- `go test -v ./internal/graphql/admin -run 'ReloadSchema|SchemaIncludesReloadSchema|ReloadSchemaResult'`
- `go test -v -race ./internal/graphql/admin`
- Because `cmd/hyperindex/main.go` wiring changes too: `go test -v ./cmd/hyperindex ./internal/graphql/admin`
- Before merge with backend changes: `go build -v ./...`, `make lint`, `DATABASE_URL=sqlite::memory: go test -v -race ./...`

## Risks/open questions

- GON-59 result type placement is not known yet. Avoid import cycles; use an adapter in `main` if needed.
- Make sure callback wiring works in Tap mode and Jetstream mode. Do not hide it inside Jetstream-only `startJetstream` logic.
- Admin handler returns HTTP 400 for any GraphQL resolver error. Failure payload tests must assert no `errors` array for expected reload failures.
- If the mutation field is NonNull and resolver returns `nil, nil`, GraphQL will surface a non-null violation. Treat nil result as an explicit resolver error instead.
- `reloadedAt` should be null on failure unless GON-59 defines a useful last-success timestamp. The plan’s requested API only needs `reloadedAt` as nullable string.
- `lexiconCount` on failure must be active schema count, not attempted source count; rely on GON-59 manager contract.
- Existing `LexiconChangeCallback` updates ingestion collection filters and is currently wired only in Jetstream mode. Schema reload must be separate and available regardless of ingestion mode.

## Compact worker meta-prompt for GON-61

Implement Linear GON-61 / MSR-003 after GON-59 lands: add an admin-gated `reloadSchema` mutation that calls a schema reload callback and returns `{ success, lexiconCount, reloadedAt, error }`.

Evidence: `docs/manual-schema-reload-plan.md:48-82` defines the API and says reload build/validation failures are payloads, not GraphQL errors. `docs/manual-schema-reload-plan.md:174-199` requires a separate `SchemaReloadCallback`, not reuse of `LexiconChangeCallback`. Existing admin mutation auth is in `internal/graphql/admin/schema.go:265-670`: every admin mutation calls `requireAdmin` first. Handler auth/status behavior is in `internal/graphql/admin/handler.go:83-131`: valid `X-Admin-API-Key` + admin `X-User-DID` sets admin context; GraphQL errors return HTTP 400. Existing callback patterns are in `internal/graphql/admin/resolvers.go:41-83`; lexicon ingestion callback behavior is in `resolvers.go:85-105` and `cmd/hyperindex/main.go:722-752` and must stay separate.

Change only directly related files: likely `internal/graphql/admin/resolvers.go`, `internal/graphql/admin/types.go`, `internal/graphql/admin/schema.go`, `internal/graphql/admin/handler_test.go` and/or a new admin resolver test file, `cmd/hyperindex/main.go`, plus a Changie fragment if not covered elsewhere. Add real doc comments for exported Go types/functions.

Success criteria: authorized admins can call `reloadSchema`; unauthorized callers get the existing `admin privileges required` GraphQL error; missing callback returns a helpful resolver error; callback-reported reload failures return HTTP 200 GraphQL data with `success:false` and no GraphQL errors; successful reload returns `success:true`, active `lexiconCount`, and `reloadedAt`; callback is separate from lexicon ingestion callbacks and works in both Tap and Jetstream modes.

Validation: run targeted admin tests for success/failure/unauthorized/missing callback, then `go test -v -race ./internal/graphql/admin`; include `go test -v ./cmd/hyperindex ./internal/graphql/admin` for wiring and full backend verification before merge.

Stop/escalate if GON-59 does not expose a reload method/result with active-count semantics, if result type placement would create an import cycle, or if deciding whether reload failures should be callback `err` vs payload is still ambiguous. Do not change ingestion callback semantics or public HTTP/subscription schema-provider behavior in this issue unless required by already-landed GON-59 interfaces.
