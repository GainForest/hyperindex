# GraphQL audit API implementation context — GON-52 / TAA-008

Read-only code context for adding `auditRecordEvents` after the audit repository query API lands. Code was not edited; this file is the handoff artifact.

## Source requirements

- `docs/tap-append-only-audit.md:311-315`: add a separate audit query beside current-state queries; avoid the type name `RecordEvent`; wire `AuditRepository` through resolver repository context/service container.
- `docs/tap-append-only-audit.md:319-347`: target schema names: `AuditRecordEvent`, `AuditRecordAction`, `Query.auditRecordEvents(first, after, where, orderBy): AuditRecordEventConnection!`.
- `docs/tap-append-only-audit.md:350-363`: first filters are `id`, `uri`, `did`, `collection`, `rkey`, `action`, `live`, `rev`, `cid`, `receivedAt`; cursors should be opaque and encode `record_events.id`.
- `docs/tap-append-only-audit.md:370-479`: example queries require URI trail, collection deletes with `action: { eq: DELETE }`, DID trail, and cursor continuation.
- `docs/tap-append-only-audit.md:495-496`: later test requirements: filters by `uri`, `did`, `collection`, `action`; cursor pagination uses stable `record_events.id` cursors.
- `plans/tap-audit-linear-issues.json:67-73`: TAA-008 scope confirms GraphQL types/query, service-container wiring, action mapping to `CREATE|UPDATE|DELETE`, JSON record payloads, and actionable resolver errors. TAA-008 is blocked by TAA-007.

## Current repository state relevant to GON-52

- Current working tree includes TAA-001/002/003 changes. `git status` shows config/docs, audit migrations, and Tap dispatch changes, but no `internal/database/repositories/audit.go` yet.
- `grep` found no current `AuditRepository`, `FindRecordEvents`, `RecordEventFindOptions`, or `RecordEventPage` in `internal/database`. GON-52 must be applied after the writer lands TAA-004/005/007. Do not assume exact field names beyond the planned `AuditRepository.FindRecordEvents(ctx, opts RecordEventFindOptions) (*RecordEventPage, error)` shape.
- Audit migrations are present:
  - SQLite `record_events` columns and indexes: `internal/database/migrations/sqlite/007_add_audit_events.up.sql:21-44`.
  - Postgres `record_events` columns and indexes: `internal/database/migrations/postgres/007_add_audit_events.up.sql:21-44`.
  - Important query columns: `id`, `received_at`, `live`, `rev`, `did`, `collection`, `rkey`, `uri`, `action`, `cid`, `record`.

## Existing GraphQL patterns to follow

### Schema builder

- `internal/graphql/schema/builder.go:53-85`: `Builder.Build()` builds all static/dynamic types, then `buildQueryType()`, then `buildSubscriptionType()`, and creates a single `graphql.Schema`.
- `internal/graphql/schema/builder.go:401-531`: `buildQueryType()` constructs root query fields in a `graphql.Fields` map. Add `auditRecordEvents` here, independent of lexicon collection loops.
- `internal/graphql/schema/builder.go:391-398`: generic connection shape uses `edges`, `pageInfo`, and `totalCount` with `query.PageInfoType`.
- `internal/graphql/query/connection.go:26-48`: shared `PageInfo` has `hasNextPage`, `hasPreviousPage`, `startCursor`, `endCursor`.
- `internal/graphql/query/connection.go:145-166`: `query.BuildConnectionType(nodeType)` can generate `AuditRecordEventEdge` and `AuditRecordEventConnection` if `nodeType.Name()` is `AuditRecordEvent`.
- `internal/graphql/schema/builder.go:1133-1144`: `emptyConnection()` returns non-null-compatible empty connection data. Use the same behavior if `repos` or `repos.Audit` is nil.
- `internal/graphql/schema/builder.go:1147-1170`: existing cursor helpers encode JSON arrays as base64 and currently expect multi-component cursors for record queries. Audit cursors are id-only; prefer the landed audit repository cursor helpers/page info if TAA-007 provides them, or add audit-specific cursor helpers rather than changing current record cursor semantics.

### Resolver context and handler

- `internal/graphql/resolver/context.go:18-31`: public resolver dependencies live in `resolver.Repositories`; add `Audit *repositories.AuditRepository` and initialize it in `NewRepositories` after the repository exists.
- `internal/graphql/handler.go:53-63`: HTTP handler injects `resolver.Repositories` into request context; the audit resolver should use `resolver.GetRepositories(p.Context)` just like current resolvers.
- `cmd/hyperindex/main.go:56-68` and `cmd/hyperindex/main.go:180-192`: runtime service container creates repository singletons. Add `audit *repositories.AuditRepository` there after the repository exists.
- `cmd/hyperindex/main.go:626-633`: public GraphQL handler currently receives `Records`, `Actors`, `Lexicons`; add `Audit: svc.audit`.

### Static types and scalars

- `internal/graphql/types/mapper.go:13-30`: `types.JSONScalar` exists and should be reused for `AuditRecordEvent.record`.
- `internal/graphql/types/filters.go:10-207`: existing reusable filter inputs: `StringFilterInput`, `IntFilterInput`, `BooleanFilterInput`, `DateTimeFilterInput`, and `DIDFilterInput`.
- `internal/graphql/query/connection.go:90-98`: existing `SortDirection` enum values are `ASC`/`DESC`; reuse it for `AuditRecordEventOrder.direction` unless the final repository API needs a separate enum.

## Naming collision risks

- There is already a GraphQL object named `RecordEvent` for subscriptions at `internal/graphql/schema/builder.go:204-234`.
- There is already a Go subscription model `subscription.RecordEvent` at `internal/graphql/subscription/pubsub.go:28-36`, and Tap also has `tap.RecordEvent`.
- Root subscription field `recordEvents` already exists at `internal/graphql/schema/builder.go:296-316`.
- Avoid adding any GraphQL type named `RecordEvent`. Use `AuditRecordEvent`, `AuditRecordAction`, `AuditRecordEventWhere`, `AuditRecordEventOrder`, `AuditRecordEventOrderField`, `AuditRecordEventConnection`, `AuditRecordEventEdge`.
- If the audit repository exports a Go type named `RecordEvent`, qualify it as `repositories.RecordEvent` or alias imports locally to avoid ambiguity with `subscription.RecordEvent`.

## Smallest implementation path for GON-52

1. Wait for TAA-007 to land `AuditRepository.FindRecordEvents`. Read the landed option/page/event structs before coding.
2. Wire repository access:
   - `internal/graphql/resolver/context.go`: add `Audit` to `Repositories`, `NewRepositories`, and optionally `GetAuditRepo`.
   - `cmd/hyperindex/main.go`: add `svc.audit`, instantiate it in `initServices`, pass it into public GraphQL repos in `setupGraphQL`.
   - Update `internal/testutil/db.go` only if tests use the shared helper after `AuditRepository` exists.
3. Add static audit schema types, preferably near existing static GraphQL types in `internal/graphql/schema/builder.go` or a small adjacent file in package `schema`:
   - `auditRecordActionEnum`: GraphQL names `CREATE|UPDATE|DELETE`; set internal values to lowercase DB strings (`create|update|delete`) so resolver output can return DB action strings and `where.action.eq` parses to DB strings.
   - `auditRecordEventType`: fields from the plan; `record` uses `types.JSONScalar`; `id` should be `ID!`; `receivedAt` should be an RFC3339 string unless the final decision is to expose the existing `DateTime` scalar.
   - `auditRecordEventWhereInput`: align fields/operators with the landed `RecordEventFindOptions`. A reasonable first mapping is `id: IntFilterInput`, `uri/collection/rkey/rev/cid: StringFilterInput`, `did: DIDFilterInput`, `action: AuditRecordActionFilterInput`, `live: BooleanFilterInput`, `receivedAt: DateTimeFilterInput`.
   - `auditRecordEventOrderInput`: `field` enum initially at least `ID` -> `id`; `direction` reuse `query.SortDirectionEnum`.
   - connection: use `query.BuildConnectionType(auditRecordEventType)` or define equivalent static edge/connection types.
4. Add `fields["auditRecordEvents"]` in `buildQueryType()` with `first`, `after`, `where`, `orderBy`; resolver delegates to `repos.Audit.FindRecordEvents(ctx, opts)`.
5. Resolver should only translate GraphQL args to the repository options and map repository results to GraphQL maps:
   - `first`: plan default is 50. Existing `query.ClampPageSize` defaults to 20 and caps at 100. Decide intentionally. To support the plan's `first: 1000` example, use audit-specific defaults/max (e.g. default 50, max 1000) or update docs if using the existing 100 cap.
   - `after`: pass through if the repository owns opaque cursor decoding; otherwise decode id-only cursors with audit-specific helpers and return errors containing `invalid cursor`.
   - `where`: parse only fields/operators supported by the repository; do not expose unsupported operators in GraphQL.
   - `orderBy`: default should match repository default; plan examples need ID ASC and ID DESC.
   - `record`: if repository returns raw JSON as string/bytes, unmarshal to `map[string]any` (or nil for deletes) before returning. Avoid returning raw `[]byte`, which would JSON-encode as base64.
6. If `repos == nil` or `repos.Audit == nil`, return `emptyConnection()` instead of panicking. This preserves schema availability in tests and non-audit deployments.

## Tests to add later (TAA-011)

Best location: `internal/graphql/schema/builder_test.go`, with possible small helpers near existing DB-backed helpers at `builder_test.go:896-920`. Seed audit rows directly with SQL instead of using ingestion, so GraphQL tests do not depend on TAA-004/005 internals.

Suggested fixture:
- Use in-memory SQLite with migrations.
- Insert matching `raw_tap_events` rows first, then `record_events` rows with explicit ids 1..4, fixed `received_at`, varied `uri`, `did`, `collection`, `action`, `live`, `rev`, `cid`, and `record` JSON/null.
- Context should include `resolver.Repositories{Audit: repositories.NewAuditRepository(exec)}`.

Coverage:
- Schema/introspection: `auditRecordEvents` exists; `AuditRecordEvent` exists; no new type named plain `RecordEvent` beyond the subscription type.
- Query examples:
  - `where: { uri: { eq: ... } }` returns only one record trail.
  - `where: { did: { eq: ... } }` returns only that DID.
  - `where: { collection: { eq: ... }, action: { eq: DELETE } }` returns deletes and serializes `action` as `DELETE`.
  - `record` returns JSON object for create/update and null for delete.
- Cursor/order:
  - ASC by ID first page returns ids `[1,2]`, has `hasNextPage=true`, and non-empty `endCursor`; querying `after: endCursor` returns `[3,4]` with no duplicates/skips.
  - DESC by ID returns `[4,3]`, then `[2,1]` after the cursor if DESC is supported.
  - Invalid cursor returns a GraphQL error containing `invalid cursor` or the repository's actionable message.
- Avoid wall-clock assertions; order by explicit `record_events.id`.

## Validation commands

Targeted while implementing GON-52:

```sh
go test -v ./internal/graphql/schema ./internal/graphql/resolver ./internal/graphql/query
go test -v ./cmd/hyperindex
```

If the audit repository or service wiring is touched, also run:

```sh
go build -v ./...
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

If database/query behavior changes and Postgres is available:

```sh
DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable go test -v -race ./...
```

## Implementation-ready meta-prompt for GON-52 / TAA-008

**Goal:** Add the public GraphQL `auditRecordEvents(first, after, where, orderBy)` API for append-only `record_events`, wired through the existing resolver repository context and runtime service container, using the landed `AuditRepository.FindRecordEvents` API.

**Context/evidence:** Follow `docs/tap-append-only-audit.md:311-363` for schema/filter/cursor requirements and `docs/tap-append-only-audit.md:370-479` for example query shapes. Existing GraphQL schema wiring is in `internal/graphql/schema/builder.go:401-531`; resolver repos are injected via `internal/graphql/resolver/context.go:18-31` and `internal/graphql/handler.go:53-63`; runtime service wiring is in `cmd/hyperindex/main.go:56-68`, `180-192`, and `626-633`. Do not name the audit GraphQL type `RecordEvent`; that name is already used by subscription schema at `internal/graphql/schema/builder.go:204-234` and `recordEvents` subscription at `296-316`.

**Success criteria:**
- `Query.auditRecordEvents` exists and returns a non-null connection of `AuditRecordEvent` nodes.
- Fields include `id`, `receivedAt`, `live`, `rev`, `did`, `collection`, `rkey`, `uri`, `action`, `cid`, `record`.
- `action` serializes as `CREATE`, `UPDATE`, `DELETE`; filters accept enum inputs such as `action: { eq: DELETE }`.
- Filters at least cover `uri`, `did`, `collection`, and `action`; support all other planned fields if the landed repository option type supports them.
- Cursor pagination uses stable `record_events.id` cursors through the repository or audit-specific helpers.
- Existing current-state GraphQL queries and subscriptions remain unchanged.

**Hard constraints:**
- Do not rename or replace the existing subscription `RecordEvent` type or `recordEvents` subscription field.
- Do not change current-state `record` query semantics to implement audit querying.
- Do not depend on ingestion methods for GraphQL tests; seed audit tables directly or use repository query fixtures.
- Match the landed TAA-007 repository API; if it differs from the planned shape in a way that changes public GraphQL behavior, stop and ask.

**Suggested approach:** Add audit repository plumbing first, then static GraphQL audit types, then the `auditRecordEvents` resolver that translates GraphQL args into `RecordEventFindOptions`, delegates to `FindRecordEvents`, and maps page results into the existing connection map shape. Reuse `types.JSONScalar`, `query.PageInfoType`, and `query.SortDirectionEnum`; use audit-specific cursor/page-size handling if current record-query helpers do not fit id-only cursors.

**Validation:** Run targeted GraphQL tests and `go test -v ./cmd/hyperindex`; then `go build -v ./...` and `DATABASE_URL=sqlite::memory: go test -v -race ./...`. Run the Postgres command above if local Postgres is available.

**Stop/escalation rules:** Stop if the final `AuditRepository.FindRecordEvents` API lacks fields needed for GraphQL page info/cursors, if page-size limits conflict with the plan's `first: 1000` example and no decision exists, or if adding filters would require exposing operators unsupported by the repository.

**Resolved assumptions:** GraphQL query can be available even when audit mode is disabled and return an empty connection when no repository/data is available; audit history remains Tap-only at ingestion, but the read API is not config-gated in the current schema builder pattern.
