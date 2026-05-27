# GON-62 / MSR-004 handoff: make public HTTP and subscription handlers schema-provider aware

## Relevant files/functions

### Plan and issue evidence

- `plans/manual-schema-reload-linear-created.json:51-55` maps `MSR-004` to Linear `GON-62` (`Make HTTP and subscription handlers schema-provider aware`).
- `plans/manual-schema-reload-linear-issues.json:40-46` says GON-62 depends on MSR-001/GON-59 and requires:
  - public HTTP handler asks a schema provider per request,
  - no-schema state returns a clear 503-style GraphQL-shaped JSON response,
  - subscription handler accepts a schema provider,
  - subscription operation snapshots schema at operation start,
  - new operations on existing WebSocket connections use latest schema after reload,
  - active subscriptions are not force-dropped.
- `docs/manual-schema-reload-plan.md:5-18` describes the current startup-only schema problem: `setupGraphQL` loads lexicons, builds the public schema, and stores the resulting pointer in the handler until process restart.
- `docs/manual-schema-reload-plan.md:40-46` confirms invariants: failed reload keeps previous working schema; if no working schema exists, public GraphQL returns a clear 503-style response; reload failure reports active `lexiconCount`.
- `docs/manual-schema-reload-plan.md:96-120` describes GON-59/MSR-001 manager expectations: fresh registry/schema build, immutable snapshot, atomic swap, `Schema()`/`Snapshot()`/`LexiconCount()`, initial no-schema state allowed.
- `docs/manual-schema-reload-plan.md:135-169` is the direct design for this issue: HTTP should use `h.schemaProvider.Schema()` per request; subscriptions should snapshot current schema at operation start while active operations keep their validated schema.
- `docs/manual-schema-reload-plan.md:171-184` says main wiring should register `/graphql` and `/graphql/ws` with the manager even if initial schema load fails, and admin schema reload should be a separate callback from lexicon-change ingestion updates.
- `docs/manual-schema-reload-plan.md:245-262` lists backend tests; #7 and #12 are the GON-62 tests.

### Public HTTP handler

`internal/graphql/handler.go` currently owns schema building and captures a fixed schema pointer:

- `internal/graphql/handler.go:15-19`: `Handler` has `schema *graphql.Schema` and `repos *resolver.Repositories`.
- `internal/graphql/handler.go:21-29`: `NewHandler(registry, repos)` builds a schema from the startup registry and stores it in the handler.
- `internal/graphql/handler.go:42-51`: request parsing supports GET query params and POST JSON; invalid POST JSON returns `http.StatusBadRequest` via `http.Error`.
- `internal/graphql/handler.go:53-63`: repositories are injected into context, then `graphql.Do` is called with `Schema: *h.schema`.
- `internal/graphql/handler.go:65-70`: response is JSON; GraphQL errors currently make HTTP status `400`.
- `internal/graphql/handler.go:73-76`: `Schema()` returns the captured schema pointer; main currently uses this to seed subscription handler.

Existing tests in `internal/graphql/handler_test.go` directly construct `&Handler{schema: schema, repos: ...}` and will need a static provider/helper after the field changes:

- `handler_test.go:15-36`: `createMinimalSchema()` helper.
- `handler_test.go:38-61`: handler should not set CORS headers directly.
- `handler_test.go:63-113`: valid POST succeeds; invalid JSON remains `400`.
- `handler_test.go:115-147`: GET query param succeeds.
- `handler_test.go:149-160`: tests `Schema()` returning the pointer.
- `handler_test.go:162-184`: content type is `application/json`.
- `handler_test.go:186-219`: GraphQL validation errors return `400` and include `errors`.
- `handler_test.go:221-271`: repositories are still injected into resolver context.

### Subscription WebSocket handler

`internal/graphql/subscription/handler.go` currently captures schema at handler creation and again at WebSocket connection creation:

- `subscription/handler.go:45-50`: `Handler` has `schema *graphql.Schema`, `pubsub`, and `upgrader`.
- `subscription/handler.go:52-64`: `NewHandler(schema, pubsub, allowedOrigins)` stores the schema pointer.
- `subscription/handler.go:100-116`: `ServeHTTP` upgrades the connection and copies `h.schema` into a new `wsClient`.
- `subscription/handler.go:118-126`: `wsClient` stores `schema *graphql.Schema` for the whole WebSocket connection.
- `subscription/handler.go:176-193`: `handleSubscribe` only parses payload, creates a cancellable context, registers the cancel func, and starts `runSubscription`; it does not snapshot current provider state.
- `subscription/handler.go:195-240`: `runSubscription` subscribes to `PubSub`, waits for events, and executes `graphql.Do` with `Schema: *c.schema` on every event.
- `subscription/handler.go:287-296`: `sendError` can currently send only `[{"message": ...}]`; add/adjust a helper if no-schema WS errors should include GraphQL `extensions`.
- `subscription/handler.go:299-315`: `send` serializes WebSocket writes with `c.mu`; avoid introducing concurrent writes.

`internal/graphql/subscription/pubsub.go` should remain behaviorally unchanged:

- `pubsub.go:59-77`: `Subscribe(collection)` registers a subscriber.
- `pubsub.go:90-110`: `Publish` broadcasts matching events non-blockingly.
- `pubsub.go:113-133`: `PublishRecord` decodes record JSON and publishes a `RecordEvent`.

Existing subscription tests are only origin-check tests:

- `subscription/handler_test.go:8-79`: `TestMakeOriginChecker`; no current WebSocket operation tests.

### Schema builder and main wiring

- `internal/graphql/schema/builder.go:53-85`: `Builder.Build()` creates one `graphql.Schema` containing both Query and Subscription root types.
- `schema/builder.go:296-347`: subscription root includes generic `recordEvents` plus per-collection `<fieldName>Events` fields derived from the registry. There is no separate subscription-only schema to manage.
- `cmd/hyperindex/main.go:588-624`: `setupGraphQL` builds a one-off registry from filesystem and DB lexicons.
- `cmd/hyperindex/main.go:633-651`: main creates the public GraphQL handler, registers `/graphql`, then passes `graphqlHandler.Schema()` into `subscription.NewHandler`. This is the main startup capture point that must be replaced with the provider/manager.
- `cmd/hyperindex/main.go:897-921`: current `loadLexiconsFromDir` skips parse errors; GON-59 is expected to move/wrap this as strict reload behavior. GON-62 should not solve strict loading itself unless GON-59 already exposes it.

## Current schema capture points

1. `cmd/hyperindex/main.go:633` calls `hgraphql.NewHandler(registry, repos)`, which builds the public schema from the startup registry.
2. `internal/graphql/handler.go:17` stores that schema as `Handler.schema`; `ServeHTTP` dereferences it at `handler.go:58` for every request.
3. `internal/graphql/handler.go:74-75` exposes that same startup pointer through `Handler.Schema()`.
4. `cmd/hyperindex/main.go:649` passes `graphqlHandler.Schema()` into `subscription.NewHandler`, so the subscription handler receives a point-in-time schema snapshot rather than the live provider.
5. `internal/graphql/subscription/handler.go:58` stores the schema on the subscription `Handler`.
6. `subscription/handler.go:108-112` copies the handler schema into each `wsClient` when the WebSocket connection opens, so an already-open WebSocket cannot see a reload for new operations.
7. `subscription/handler.go:121` stores that schema on the `wsClient`; `runSubscription` uses it for every event at `subscription/handler.go:233-239`.

## Proposed implementation steps

### 1. Add provider-aware public HTTP handler

- Replace `Handler.schema *graphql.Schema` with a provider field. Minimum handler-facing interface:

  ```go
  type SchemaProvider interface {
      // Schema returns the currently active public GraphQL schema.
      // A nil return means no schema has successfully loaded yet.
      Schema() *graphql.Schema
  }
  ```

- Change `NewHandler` so it accepts a `SchemaProvider` and repositories instead of a `lexicon.Registry`:
  - The handler should no longer build schemas itself; GON-59/MSR-001 owns building, fallback snapshots, and initial no-schema state.
  - Constructor should reject a nil provider with a clear error, or otherwise make nil-provider behavior explicit. No-schema is represented by `provider.Schema() == nil`, not by a nil provider.
  - Remove now-unused `internal/graphql/schema` and `internal/lexicon` imports from `handler.go` once schema building moves out.
- In `ServeHTTP`, keep existing request parsing semantics, then call `schema := h.schemaProvider.Schema()` immediately before `graphql.Do`.
- If `schema == nil`, return the no-schema response below and do not call `graphql.Do`.
- Otherwise execute with `Schema: *schema` and the existing repository context injection.
- If `Handler.Schema()` is kept, make it return `h.schemaProvider.Schema()` and update the doc comment to say it returns the current provider schema. Do not use it in main to seed subscriptions; pass the provider/manager directly.

### 2. Exact no-schema HTTP semantics

For a syntactically valid HTTP GraphQL request when no public schema is active:

- HTTP status: `503 Service Unavailable`.
- Header: `Content-Type: application/json`.
- Body: GraphQL-shaped JSON with an `errors` array and no successful `data` payload. Recommended exact shape:

  ```json
  {
    "errors": [
      {
        "message": "Public GraphQL schema is unavailable. Fix lexicon load errors, run the admin reloadSchema mutation, or check backend logs.",
        "extensions": {
          "code": "SCHEMA_UNAVAILABLE",
          "httpStatus": 503
        }
      }
    ]
  }
  ```

- This should be treated as a service/schema-availability failure, not as a GraphQL validation error from `graphql.Do`.
- Preserve existing malformed-body behavior unless deliberately changed: invalid POST JSON currently returns `400` text via `http.Error` before GraphQL execution (`handler.go:47-49`).
- Preserve existing GraphQL validation/execution error behavior for loaded schemas: status `400` with JSON `errors` (`handler.go:65-70`, `handler_test.go:186-219`).
- Keep CORS at router middleware only; the handler should still not set `Access-Control-Allow-Origin` directly.

Implementation note: `github.com/graphql-go/graphql/gqlerrors.FormattedError` supports `Extensions`, so the body can be encoded manually or via a `graphql.Result` with formatted errors.

### 3. Make subscription handler provider-aware

- Add a small provider interface in `internal/graphql/subscription/handler.go` rather than importing a concrete GON-59 manager type:

  ```go
  type SchemaProvider interface {
      Schema() *graphql.Schema
  }
  ```

- Change subscription `Handler` and `wsClient` to hold the provider, not a schema pointer.
- `NewHandler` should accept the provider and keep the existing `pubsub` and `allowedOrigins` behavior.
- `ServeHTTP` should pass `h.schemaProvider` into `wsClient`; do not snapshot schema at WebSocket upgrade time.
- In `handleSubscribe`, after payload JSON is valid and before adding an entry to `c.subscriptions`, snapshot the operation schema:

  ```go
  schema := c.schemaProvider.Schema()
  if schema == nil {
      c.sendSchemaUnavailableError(msg.ID)
      return
  }
  ```

- If schema is non-nil, then create/register the cancel func and start `go c.runSubscription(ctx, msg.ID, payload, schema)`.
- Change `runSubscription` to accept `schema *graphql.Schema` and use `Schema: *schema` inside `graphql.Do`.
- Existing active operations keep the pointer captured at their `subscribe` message. Reloads must not cancel them, disconnect their sockets, or mutate the schema pointer they use.
- New `subscribe` messages on an already-open WebSocket call `provider.Schema()` again, so they see the latest successful reload.
- If no schema is available on `subscribe`, send an operation-level `error` message and keep the WebSocket connection open so the same connection can retry after a successful reload. Do not register a PubSub subscription or cancel func in this case.
- Keep event delivery behavior unchanged: `PubSub.Subscribe`, `Publish`, event filtering, `rootObject` construction, and per-event `graphql.Do` remain as-is except for the captured schema parameter.

Recommended no-schema WebSocket operation error payload:

```json
{
  "id": "<operation id>",
  "type": "error",
  "payload": [
    {
      "message": "Public GraphQL schema is unavailable. Fix lexicon load errors, run the admin reloadSchema mutation, or check backend logs.",
      "extensions": {
        "code": "SCHEMA_UNAVAILABLE",
        "httpStatus": 503
      }
    }
  ]
}
```

There is no HTTP 503 after upgrade; `httpStatus` in extensions is just a machine-readable hint matching HTTP semantics.

### 4. Update main wiring

After GON-59 provides a public schema manager/provider:

- `setupGraphQL` should construct/use that provider instead of constructing a one-off schema inside `hgraphql.NewHandler`.
- Register `/graphql`, `/graphql/`, and `/graphql/ws` even if the initial manager reload failed and `provider.Schema() == nil`.
- Replace `subscription.NewHandler(graphqlHandler.Schema(), pubsub, allowedOrigins)` at `cmd/hyperindex/main.go:649` with `subscription.NewHandler(schemaProvider, pubsub, allowedOrigins)`.
- Do not route subscriptions through `graphqlHandler.Schema()`, because that captures a pointer and defeats operation-level snapshots.
- Keep Jetstream collection resolution separate from public schema reload; the plan explicitly says schema reload does not alter ingestion filters.

## Interface needs from GON-59 / MSR-001

GON-62 should require only a minimal, stable interface from the public schema manager:

1. `Schema() *graphql.Schema`
   - Must return the active schema from an immutable snapshot.
   - Must return `nil` when no schema has ever loaded successfully.
   - Must never return a partially built or failed-attempt schema.
   - Must be safe to call concurrently with reloads, ideally an atomic/RLock read.
   - The returned `*graphql.Schema` must be safe for callers to retain after later reloads. Active subscriptions rely on old schema pointers staying valid until their goroutines exit.
2. Optional but useful for other issues: `Snapshot() PublicSchemaSnapshot`, `LexiconCount() int`, `Reload(ctx)` as described in `docs/manual-schema-reload-plan.md:96-120`.
3. Avoid forcing subscription package imports of the concrete manager or snapshot type. `internal/graphql/schema` currently imports `internal/graphql/subscription` (`schema/builder.go:21`), so if GON-59 places the manager in `internal/graphql/schema`, importing that package from `subscription` would create a package cycle. Use structural local interfaces in handlers.
4. GON-59 should handle strict lexicon loading and reload fallback. GON-62 should not duplicate lexicon loading logic.

## Tests/validation to propose

### Public HTTP handler tests (`internal/graphql/handler_test.go`)

Add a test provider helper, e.g. `staticSchemaProvider` and/or `swappableSchemaProvider`, and update existing direct `&Handler{schema: ...}` construction.

1. **Uses current schema per request**
   - Create one handler with a swappable provider.
   - Serve a request with schema A and assert schema A field/data works.
   - Swap provider to schema B without reconstructing the handler.
   - Serve another request and assert schema B field/data works.
   - Optional: assert a field removed from B now returns the normal GraphQL `400`, proving the handler did not keep A.
2. **No-schema 503 response**
   - Provider returns nil.
   - Send a valid POST body.
   - Assert status `503`, `Content-Type: application/json`, `errors[0].message` is actionable, and `errors[0].extensions.code == "SCHEMA_UNAVAILABLE"` / `httpStatus == 503`.
   - Assert no CORS header is added directly.
3. **Existing behavior still works**
   - POST/GET success, invalid JSON `400`, GraphQL validation errors `400`, JSON content type, and repository context injection should still pass after replacing direct schema fields with providers.

### Subscription tests (`internal/graphql/subscription/handler_test.go`)

Current subscription tests only cover origins, so add WebSocket operation tests using `httptest.NewServer`, `gorilla/websocket`, and subprotocol `graphql-transport-ws`.

Suggested test helpers:

- `swappableSchemaProvider` with mutex/atomic schema pointer.
- `buildSubscriptionSchema(fieldName string)` returning a schema with:
  - dummy Query root (GraphQL schema creation generally expects Query),
  - Subscription root with `recordEvents` returning an object type that exposes `fieldName` with a resolver returning a known value.
- WebSocket helpers: dial server, send `connection_init`, expect `connection_ack`, send `subscribe`, publish `PubSub` event, read messages by operation ID.

Tests:

1. **Snapshots schema at subscribe start**
   - Provider starts with schema A that supports `oldField`.
   - Open WebSocket, init, send subscribe for `oldField`.
   - Before publishing any event, swap provider to schema B that does not support `oldField`.
   - Publish event.
   - Expect operation receives `next` data for `oldField`, proving the operation captured A at subscribe time rather than reading provider at event time.
2. **New operation on existing WebSocket uses latest schema**
   - Keep the same WebSocket open.
   - After provider swap to schema B, send a second subscribe for `newField`.
   - Publish event.
   - Expect the second operation receives `newField` data.
   - If first operation is still active, it may also receive old-schema data; that is acceptable and verifies no forced drop.
3. **No schema at subscribe keeps connection open**
   - Provider returns nil.
   - Open/init WebSocket and send subscribe.
   - Expect operation-level `error` payload with `SCHEMA_UNAVAILABLE`.
   - Assert `pubsub.SubscriberCount() == 0` so no subscription was registered.
   - Set provider to a real schema and send a new subscribe on the same connection.
   - Publish event and expect `next`, proving existing connections can recover after reload.
4. **Active subscriptions are not forcefully dropped on reload**
   - Start a subscription with schema A.
   - Swap provider to schema B.
   - Publish another event.
   - The existing operation should still receive data or normal query-specific errors from schema A; it should not receive a forced `complete`/disconnect solely because provider changed.

### Main wiring tests

There are no existing tests for `setupGraphQL`. If adding integration-level coverage is too much for GON-62, rely on handler/subscription tests plus compile-time coverage in `cmd/hyperindex/main.go`. If feasible, add a small test around route registration only after GON-59 makes setup testable.

### Targeted validation commands

For this issue, at minimum:

```bash
go test -v ./internal/graphql/...
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

If GON-62 also touches database/schema-manager code from GON-59, include the PostgreSQL test command from `AGENTS.md`. This issue is backend-only unless frontend/docs are also changed.

## Risks/open questions

- **GON-59 dependency:** This issue should start after GON-59 exposes a no-schema-capable provider. If that interface is not available, stop and coordinate rather than inventing a parallel manager.
- **Package cycles:** Do not import a concrete manager package into `internal/graphql/subscription` if that manager lives in `internal/graphql/schema`; `schema` already imports `subscription`. Use local structural interfaces.
- **Schema pointer lifetime:** Active subscriptions retaining old schemas is correct, but it means old schema objects remain live until those operations end. This is expected and better than forced disconnects.
- **No-schema response exactness:** The plan requires a clear 503-style GraphQL JSON response but does not specify exact `extensions`. The proposed `SCHEMA_UNAVAILABLE`/`httpStatus: 503` should be treated as the chosen contract if tests assert it.
- **WebSocket no-schema semantics:** There is no HTTP 503 after upgrade. Operation-level `error` with the same message/code is the closest protocol-compatible behavior; keep the socket open.
- **Do not add early validation unless intentional:** Current subscription behavior runs `graphql.Do` per event and reports errors when events arrive. Adding subscribe-time validation may be useful but could change delivery/error timing. GON-62 can satisfy snapshot timing by capturing the schema pointer at `handleSubscribe` and keeping per-event execution unchanged.
- **Existing stale comment near subscription origins:** `subscription/handler.go:53-56` says nil/empty origins enforce same-origin, but `makeOriginChecker` at `handler.go:69-80` allows all origins. If the `NewHandler` doc comment is edited for the provider signature, fix this adjacent doc to match current behavior, but do not change origin behavior in GON-62.
- **Changie:** The parent plan says this feature is operator-facing (`docs/manual-schema-reload-plan.md:288-290`). If GON-62 lands separately with observable handler behavior, coordinate a single operator-facing Changie fragment for the parent feature or add one here according to repo rules.

## Compact worker meta-prompt for GON-62

Implement GON-62 / MSR-004: make the public `/graphql` HTTP handler and `/graphql/ws` subscription handler use the current public schema provider instead of a startup-only `*graphql.Schema`.

Context/evidence:
- Current HTTP handler stores `schema *graphql.Schema` (`internal/graphql/handler.go:15-19`), builds it in `NewHandler` (`:21-29`), and executes with `Schema: *h.schema` (`:56-63`).
- Main builds that handler once and passes `graphqlHandler.Schema()` into subscriptions (`cmd/hyperindex/main.go:633-650`).
- Subscription handler stores schema in `Handler` (`subscription/handler.go:45-64`), copies it into each `wsClient` on upgrade (`:100-126`), and uses it for every event (`:195-240`).
- GON-59/MSR-001 should provide a manager/provider where `Schema() *graphql.Schema` returns the active immutable snapshot schema or nil for no active schema (`docs/manual-schema-reload-plan.md:96-120`).

Success criteria:
- Public HTTP requests call provider `Schema()` per request and use the latest successful schema without reconstructing the handler.
- Public HTTP requests with no active schema return HTTP `503`, `Content-Type: application/json`, and GraphQL-shaped `errors` with actionable message and `extensions.code = "SCHEMA_UNAVAILABLE"`.
- Subscription handler accepts a provider, not a schema pointer.
- WebSocket connections do not snapshot schema at upgrade time.
- Each `subscribe` operation snapshots the current schema once at operation start and uses that pointer for all later events for that operation.
- New operations on an already-open WebSocket see the latest schema after reload.
- Existing active subscriptions are not forcefully disconnected/cancelled solely due to reload.
- Event delivery behavior via PubSub remains unchanged.

Hard constraints:
- Do not implement automatic schema reload or ingestion filter changes.
- Do not force-disconnect active subscriptions on reload.
- Avoid package cycles; use small local `SchemaProvider` interfaces instead of importing concrete manager types into `subscription`.
- Do not duplicate GON-59 schema loading/fallback logic.

Suggested approach:
1. Add/update a documented `SchemaProvider` interface with `Schema() *graphql.Schema` in the HTTP handler package and a local equivalent in the subscription package.
2. Change HTTP `Handler` to hold the provider and repos; make `NewHandler` accept the provider. Keep parsing/repo context/error behavior, but before `graphql.Do`, fetch `schema := provider.Schema()` and return the 503 JSON response if nil.
3. Change subscription `Handler`/`wsClient` to hold provider. In `handleSubscribe`, parse payload, fetch schema, send operation error if nil, otherwise register cancel func and call `runSubscription(..., schema)`.
4. Update main to pass the same GON-59 provider/manager to both public HTTP and subscription handlers. Do not pass `graphqlHandler.Schema()` to subscriptions.
5. Update tests with static/swappable provider helpers and add HTTP/no-schema plus WebSocket snapshot-timing tests.

Validation:
- Run `go test -v ./internal/graphql/...` first.
- Then run `go build -v ./...`, `make lint`, and `DATABASE_URL=sqlite::memory: go test -v -race ./...`.

Stop/escalation rules:
- If GON-59 has not provided a `Schema() *graphql.Schema` no-schema-capable provider, stop and ask for the interface decision.
- If a desired shared provider type would introduce an import cycle with `internal/graphql/schema` and `internal/graphql/subscription`, stop and use local structural interfaces instead.
- If asked to change subscription validation timing or disconnect behavior, escalate; GON-62 explicitly keeps event delivery behavior and active subscriptions intact.
