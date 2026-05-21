# TAA-002 / GON-46 handoff — audit event migrations

## Changed files

- `internal/database/migrations/sqlite/007_add_audit_events.up.sql`
- `internal/database/migrations/sqlite/007_add_audit_events.down.sql`
- `internal/database/migrations/postgres/007_add_audit_events.up.sql`
- `internal/database/migrations/postgres/007_add_audit_events.down.sql`

## Table and index details

### `raw_tap_events`

Archive table for every successfully parsed Tap record/identity delivery, including duplicate deliveries.

Columns:

- `id` primary key (`INTEGER PRIMARY KEY AUTOINCREMENT` in SQLite, `BIGSERIAL` in Postgres)
- `source TEXT NOT NULL DEFAULT 'tap'`
- `tap_delivery_id` (`INTEGER` in SQLite, `BIGINT` in Postgres)
- `type TEXT NOT NULL CHECK (type IN ('record', 'identity'))`
- `received_at` defaulting to current database time
- `payload` (`TEXT` in SQLite, `JSONB` in Postgres)

Indexes:

- `idx_raw_tap_events_source_delivery` on `(source, tap_delivery_id)`
- `idx_raw_tap_events_type_received_at` on `(type, received_at DESC)`

### `record_events`

Append-only decoded record ledger with semantic dedupe via `event_key`.

Columns:

- `id` primary key (`INTEGER PRIMARY KEY AUTOINCREMENT` in SQLite, `BIGSERIAL` in Postgres)
- `event_key TEXT NOT NULL UNIQUE`
- `source TEXT NOT NULL DEFAULT 'tap'`
- `tap_delivery_id` (`INTEGER` in SQLite, `BIGINT` in Postgres)
- `raw_event_id` referencing `raw_tap_events(id)`
- `received_at` defaulting to current database time
- `live` (`INTEGER CHECK (live IN (0, 1))` in SQLite, `BOOLEAN` in Postgres)
- `rev TEXT NOT NULL DEFAULT ''`
- `did TEXT NOT NULL`
- `collection TEXT NOT NULL`
- `rkey TEXT NOT NULL`
- `uri TEXT NOT NULL`
- `action TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete'))`
- `cid TEXT`
- `record` (`TEXT` in SQLite, `JSONB` in Postgres)

Indexes:

- `idx_record_events_uri_id` on `(uri, id)`
- `idx_record_events_did_id` on `(did, id)`
- `idx_record_events_collection_id` on `(collection, id)`
- `idx_record_events_action_id` on `(action, id)`
- `idx_record_events_received_at` on `(received_at DESC)`

### `identity_events`

Append-only decoded identity ledger with a best-effort unique `event_key`.

Columns:

- `id` primary key (`INTEGER PRIMARY KEY AUTOINCREMENT` in SQLite, `BIGSERIAL` in Postgres)
- `event_key TEXT NOT NULL UNIQUE`
- `source TEXT NOT NULL DEFAULT 'tap'`
- `tap_delivery_id` (`INTEGER` in SQLite, `BIGINT` in Postgres)
- `raw_event_id` referencing `raw_tap_events(id)`
- `received_at` defaulting to current database time
- `did TEXT NOT NULL`
- `handle TEXT NOT NULL DEFAULT ''`
- `is_active` nullable boolean-compatible field (`INTEGER CHECK (is_active IN (0, 1))` in SQLite, `BOOLEAN` in Postgres) so missing Tap booleans can remain distinguishable at the decoded-row level
- `status TEXT NOT NULL DEFAULT ''`

Indexes:

- `idx_identity_events_did_id` on `(did, id)`

Down migrations drop `identity_events`, `record_events`, then `raw_tap_events` so foreign-key dependencies are removed before the raw parent table.

## Validation

- `go test -v ./internal/database/migrations` — exit code 0
  - Applied migrations 001 through 007 on SQLite.
  - Re-ran migrations idempotently.
  - Rolled back migration 007 successfully.

## Notes / blockers

- `context.md` and `plan.md` requested by the task were not present in the checkout, so I used `docs/tap-append-only-audit.md` plus existing migrations as source context.
- No repository or application code was touched.
- No older migrations were edited.
- Postgres SQL was matched to existing repo conventions (`TIMESTAMP WITH TIME ZONE`, `JSONB`, `BOOLEAN`) but was not executed because no Postgres validation target was provided in this task.
- Concurrent-worker risk is low: this change only adds new migration files under version `007`; no shared config/Tap dispatch files were touched.
