# TAA-004/TAA-005/TAA-007 handoff — AuditRepository

## Changed files

- `internal/database/repositories/audit.go`
- `internal/database/repositories/audit_test.go`

## Public API added

- `NewAuditRepository(db database.Executor) *AuditRepository`
- `(*AuditRepository).IngestTapEvent(ctx, rawPayload, event)`
- `(*AuditRepository).FindRecordEvents(ctx, opts)`
- Ingest types:
  - `AuditTapEvent`
  - `AuditTapRecordEvent`
  - `AuditTapIdentityEvent`
  - `AuditIngestResult`
- Query/pagination types:
  - `AuditRecordEvent`
  - `AuditRecordEventFilters`
  - `AuditRecordEventOrder`
  - `RecordEventFindOptions`
  - `AuditRecordEventPage`
  - `AuditRecordEventEdge`
- Page-size constants:
  - `DefaultAuditRecordEventPageSize`
  - `MaxAuditRecordEventPageSize`

Important API note: `IngestTapEvent` currently accepts the repository-local `*repositories.AuditTapEvent`, not `*tap.Event`, because `internal/tap` already imports `internal/database/repositories`. Importing `tap` from `repositories` would create a Go import cycle. GON-50 should either map `tap.Event` into `repositories.AuditTapEvent` at the Tap handler boundary or approve a shared model/refactor.

## Ingest behavior implemented

### Shared transaction behavior

- Starts one DB transaction per valid record/identity delivery.
- Inserts a `raw_tap_events` row before decoded audit insert.
- Inserts decoded audit rows with `ON CONFLICT(event_key) DO NOTHING`.
- Applies current-state projection changes only when the decoded audit row was newly inserted.
- Commits once at the end; caller should ack Tap only after this method returns nil.

### Record events

- Computes the normal semantic key:
  - `record:{did}:{rev}:{collection}:{rkey}:{action}:{cid-or-empty}`
- If `rev` is empty, computes fallback key:
  - `record:fallback:{tap_delivery_id}:{sha256(normalized_payload)}`
- Stores missing/empty CID as SQL NULL while still using the empty CID key slot.
- Stores missing record body as SQL NULL and skips current `record` projection updates for create/update.
- For new create/update events with a body:
  - upserts the current `actor` row with empty handle;
  - upserts current `record` inside the same transaction;
  - preserves existing CID-same skip behavior for current projection updates.
- For new delete events:
  - appends a `record_events` row;
  - deletes the current `record` row.
- Duplicate semantic record events:
  - insert another `raw_tap_events` row;
  - return `Inserted=false`;
  - point `EventID` to the existing decoded row;
  - skip current-state mutation;
  - commit successfully.

### Identity events

- Computes best-effort key:
  - `identity:{tap_delivery_id}:{did}:{handle}:{is_active-or-empty}:{status}`
- Stores missing `is_active` as SQL NULL and uses an empty key slot for the missing value.
- For new active/non-purge identity events, upserts current `actor`.
- For purge statuses `deleted`, `deactivated`, `suspended`, and `takendown`, deletes current records and actor for the DID.
- Does not create synthetic per-record delete audit events for identity purges.
- Duplicate identity events conflict on `identity_events.event_key`, return `Inserted=false`, and skip current-state mutation.

## Query behavior implemented

`FindRecordEvents` supports:

- filters: `id`, `uri`, `did`, `collection`, `rkey`, `action`, `live`, `rev`, `cid`, `receivedAt`, `receivedAtAfter`, `receivedAtBefore`;
- parameterized SQL for all caller-provided values;
- action validation for `create`, `update`, and `delete`;
- stable opaque cursors around `record_events.id`;
- ASC/DESC id ordering, defaulting to DESC;
- dialect-compatible `record` JSON scanning (`record::text` on Postgres, plain text on SQLite);
- `first + 1` pagination to populate `HasNextPage`.

## Tests added

Focused repository tests cover:

- record create inserts raw/audit/current rows;
- duplicate record delivery inserts another raw row but no second `record_events` row;
- record update appends audit and updates current state;
- record delete appends audit and removes current state;
- missing record body/CID/rev is audited with fallback key and no current projection update;
- identity active update upserts actor;
- identity purge removes current actor/records without synthetic record deletes;
- `FindRecordEvents` filters and id-cursor pagination.

## Commands run

- `gofmt -w internal/database/repositories/audit.go internal/database/repositories/audit_test.go` — exit 0
- `go test ./internal/database/repositories` — exit 0
- `go test ./internal/config ./internal/tap ./internal/database/migrations ./internal/database/repositories` — exit 0
- `go test -race ./internal/database/repositories` — exit 0
- `go test ./...` — exit 0

## Known gaps versus TAA-004/TAA-005/TAA-007

- The ingest method is not wired into Tap mode yet; GON-50 owns wiring/ack-after-commit integration.
- The method currently consumes `repositories.AuditTapEvent` instead of `tap.Event` to avoid the existing `tap -> repositories` import cycle.
- Query ordering is intentionally limited to stable `record_events.id` ASC/DESC; no receivedAt ordering was added because the approved cursor model is id-based.
- PostgreSQL SQL paths were compiled but not exercised against a live Postgres database in this worker.
- This is focused repository coverage, not the full TAA-009 test matrix.

## Decisions needing parent approval

- Approve the `repositories.AuditTapEvent` adapter boundary for GON-50, or choose a shared Tap event model refactor before wiring audit mode into `internal/tap`.
