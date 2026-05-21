# TAA-008/TAA-011 handoff — GraphQL audit API

## Changed files

- `internal/graphql/schema/audit.go`
- `internal/graphql/schema/builder.go`
- `internal/graphql/schema/builder_test.go`

No edits were made to `cmd/hyperindex/main.go`, `internal/graphql/resolver/context.go`, Tap handler/consumer files, or Linear.

## Schema/API behavior

Added `Query.auditRecordEvents(first, after, where, orderBy): AuditRecordEventConnection!`.

New GraphQL types/input types:

- `AuditRecordEvent`
- `AuditRecordAction` enum with `CREATE`, `UPDATE`, `DELETE`
- `AuditRecordEventWhere`
- `AuditRecordEventOrder`
- `AuditRecordEventOrderField`
- focused audit filter inputs for exact string/int/bool/action filtering and `receivedAt` `eq`/`gt`/`lt` range filtering

Resolver behavior:

- Delegates to `repos.Audit.FindRecordEvents(ctx, repositories.RecordEventFindOptions)`.
- Returns `emptyConnection()` when resolver repositories or `repos.Audit` are nil.
- Keeps cursors repository-owned and passes `after` through unchanged.
- Supports ID ASC/DESC ordering through `orderBy: { field: ID, direction: ASC|DESC }`.
- Supports planned filters: `id`, `uri`, `did`, `collection`, `rkey`, `action`, `live`, `rev`, `cid`, and `receivedAt`.
- Maps DB action values `create/update/delete` to GraphQL enum output `CREATE/UPDATE/DELETE` by using lowercase enum internal values.
- Parses `record` JSON strings into JSON objects before returning; delete/missing bodies return `null`.
- Uses audit-specific page size behavior: default `50`, max `1000`, matching the audit repository constants and the plan examples.

## Tests added

Focused GraphQL coverage in `internal/graphql/schema/builder_test.go`:

- `TestAuditRecordEventsQueryFilters`
  - filters by `uri`, `did`, `collection`, and `action`
  - verifies enum output is `CREATE`/`UPDATE`/`DELETE`
  - verifies `record` returns a JSON object for create/update and `null` for delete
- `TestAuditRecordEventsQueryCursorPagination`
  - verifies ID ASC first/after pagination with stable repository cursors
  - verifies no duplicate/skipped IDs across pages
  - verifies ID DESC ordering
- `TestAuditRecordEventsQueryWithoutRepositoryReturnsEmptyConnection`
  - verifies nil/missing audit repository does not panic and returns an empty connection

Test fixtures use in-memory SQLite with migrations and seed `raw_tap_events`/`record_events` directly.

## Validation

- `gofmt -w internal/graphql/schema/audit.go internal/graphql/schema/builder.go internal/graphql/schema/builder_test.go` — exit 0
- `go test ./internal/graphql/schema` — exit 0
- `go test ./internal/graphql/schema ./internal/graphql/resolver ./internal/graphql/query` — exit 0
- `go test ./internal/graphql/...` — exit 0
- `go test ./cmd/hyperindex` — exit 0
- `go test ./...` — exit 0

## Gaps / notes

- `/home/kzoeps/Projects/gainforest/append-only-indexer/context.md` and `plan.md` were not present; implementation used `docs/tap-append-only-audit.md` and `plans/subagent-handoffs/graphql-audit-context.md`.
- `totalCount` is nullable for non-empty audit results and is not computed because `AuditRepository.FindRecordEvents` does not expose a total-count query. Empty repository fallback still returns `totalCount: 0` through the existing `emptyConnection()` helper.
- Postgres runtime behavior was not separately exercised in this task; GraphQL tests use SQLite per the focused TAA-011 scope.

## Decisions needed

- None for TAA-008/TAA-011 focused scope.
