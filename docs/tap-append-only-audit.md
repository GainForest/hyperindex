# Tap Append-Only Audit Indexer Plan

## Goal

Add append-only audit history to Hyperindex's Tap ingestion path without replacing the existing current-state indexer.

Hyperindex should continue to expose fast current-state GraphQL queries from the existing `record` and `actor` tables. When Tap audit indexing is enabled, each Tap delivery is also persisted as immutable audit history before the current-state projection is updated.

```txt
Tap websocket delivery
  -> one database transaction
    -> raw_tap_events      append every delivery attempt
    -> record_events       append deduped create/update/delete record events
    -> identity_events     append identity events
    -> record / actor      update existing current-state projection
  -> ack Tap only after commit
```

This plan is Tap-only. Jetstream and legacy backfill do not need append-only audit support in the first implementation.

## Why this is an add-on

Append-only audit history can be implemented as an optional feature layered beside the current indexer:

- existing current-state queries keep reading from `record` and `actor`
- deletes can still remove rows from `record`
- `record_events` permanently preserves create/update/delete history
- operators can choose whether to store audit history

The first implementation should avoid introducing separate `records_current` / `actors_current` projection tables. The existing tables are enough for the active view.

## Configuration

Audit storage should be gated behind an explicit flag:

```env
TAP_ENABLED=true
AUDIT_ENABLED=true
```

First implementation rules:

- `AUDIT_ENABLED=false`: Tap ingestion keeps current-state behavior and does not write audit rows.
- `AUDIT_ENABLED=true` with `TAP_ENABLED=true`: Tap ingestion writes audit rows and current-state changes in one transaction.
- `AUDIT_ENABLED=true` with `TAP_ENABLED=false`: ignore audit mode or fail startup with a clear error. The simpler first implementation can fail startup because audit support is Tap-only.

The Tap consumer can still use the full-event `HandleEvent` interface in both modes. The config flag decides which handler implementation is wired at startup.

## Source model

Tap sends nested JSON events with a top-level delivery id.

Record event example:

```json
{
  "id": 12345,
  "type": "record",
  "record": {
    "live": true,
    "rev": "3kb3fge5lm32x",
    "did": "did:plc:abc123",
    "collection": "app.example.record",
    "rkey": "3kb3fge5lm32x",
    "action": "create",
    "cid": "bafyreig...",
    "record": {
      "$type": "app.example.record"
    }
  }
}
```

Identity event example:

```json
{
  "id": 12346,
  "type": "identity",
  "identity": {
    "did": "did:plc:abc123",
    "handle": "alice.example.com",
    "is_active": true,
    "status": "active"
  }
}
```

Treat Tap's top-level `id` as a delivery/ack id, not a permanent semantic event id. It belongs in `tap_delivery_id` and is used for acknowledgements only.

## Dedupe model

Tap delivery is at-least-once. Duplicate deliveries are expected after disconnects, crashes, or failed acknowledgements.

Raw deliveries should not be deduped. Every successfully parsed record/identity Tap delivery goes into `raw_tap_events`. Malformed or unsupported websocket frames can be logged and left unacked in the first implementation; the audit tables are for valid Tap deliveries, not a generic dead-letter queue.

Decoded record events should be deduped with a deterministic semantic key when possible:

```txt
record:{did}:{rev}:{collection}:{rkey}:{action}:{cid-or-empty}
```

Why these fields:

- `did`: repo identity
- `rev`: repo commit/revision
- `collection`: record namespace
- `rkey`: record key within the collection
- `action`: create/update/delete
- `cid`: content identifier for create/update, empty for delete

Do not reject otherwise valid Tap create/update events only because `cid` is missing. Store the audit row with an empty/null CID and use the empty CID slot in the key.

If `rev` is missing, do not poison Tap with an endless nack/retry loop. Store the row with an empty `rev` and use a fallback event key based on delivery id plus a hash of the normalized event payload. This is weaker than the normal semantic key, but it keeps ingestion append-only and operationally safe.

A duplicate semantic record event should:

1. insert another `raw_tap_events` row
2. conflict on `record_events.event_key`
3. skip current-state projection changes
4. still commit and ack Tap

Identity events do not currently include a protocol-level stable sequence in the Tap payload. The first implementation can use a best-effort key:

```txt
identity:{tap_delivery_id}:{did}:{handle}:{is_active}:{status}
```

## Schema plan

Add migrations for both supported database dialects:

```txt
internal/database/migrations/sqlite/007_add_audit_events.up.sql
internal/database/migrations/sqlite/007_add_audit_events.down.sql
internal/database/migrations/postgres/007_add_audit_events.up.sql
internal/database/migrations/postgres/007_add_audit_events.down.sql
```

### `raw_tap_events`

Archive of successfully parsed record/identity Tap deliveries, including duplicates.

Fields:

- `id`
- `source` defaulting to `tap`
- `tap_delivery_id`
- `type` constrained to `record` or `identity`
- `received_at`
- `payload`

Useful indexes:

- `(source, tap_delivery_id)`
- `(type, received_at DESC)`

### `record_events`

Append-only decoded record ledger.

Fields:

- `id`
- `event_key` unique
- `source` defaulting to `tap`
- `tap_delivery_id`
- `raw_event_id` referencing `raw_tap_events(id)`
- `received_at`
- `live`
- `rev`
- `did`
- `collection`
- `rkey`
- `uri`
- `action` constrained to `create`, `update`, or `delete`
- `cid`
- `record`

Useful indexes:

- `(uri, id)`
- `(did, id)`
- `(collection, id)`
- `(action, id)`
- `(received_at DESC)`

### `identity_events`

Append-only decoded identity ledger.

Fields:

- `id`
- `event_key` unique
- `source` defaulting to `tap`
- `tap_delivery_id`
- `raw_event_id` referencing `raw_tap_events(id)`
- `received_at`
- `did`
- `handle`
- `is_active`
- `status`

Useful indexes:

- `(did, id)`

## Repository plan

Add an audit repository:

```txt
internal/database/repositories/audit.go
internal/database/repositories/audit_test.go
```

Suggested public API:

```go
type AuditRepository struct {}

type AuditIngestResult struct {
    Type          string
    TapDeliveryID int64
    RawEventID    int64
    Inserted      bool
    EventID       *int64
    EventKey      *string
}

func (r *AuditRepository) IngestTapEvent(ctx context.Context, rawPayload []byte, event *tap.Event) (*AuditIngestResult, error)
func (r *AuditRepository) FindRecordEvents(ctx context.Context, opts RecordEventFindOptions) (*RecordEventPage, error)
```

Implementation decision: in audit mode, `AuditRepository` owns all database writes inside the ingest transaction. It should use transaction-local SQL for audit inserts and current `record` / `actor` projection changes instead of calling non-transaction-aware repository methods. Do not add tx-aware wrappers in the first implementation unless direct SQL becomes harder to maintain.

`IngestTapEvent` should run one transaction:

```txt
BEGIN
  insert raw_tap_events

  if event.type = record:
    compute semantic event_key
    insert record_events on conflict do nothing
    if inserted:
      if action = create/update and record body is present:
        upsert current record table
      if action = delete:
        delete from current record table

  if event.type = identity:
    compute best-effort event_key
    insert identity_events on conflict do nothing
    if inserted:
      apply current actor/identity behavior
COMMIT
```

The caller should acknowledge Tap only after this method returns successfully.

## Tap consumer changes

The current Tap handler receives only `RecordEvent` or `IdentityEvent`, so it loses the raw payload and top-level delivery id.

Change the Tap dispatch path to let an audit-aware handler receive the full event:

```go
type EventHandler interface {
    HandleEvent(ctx context.Context, rawPayload []byte, event *Event) error
}
```

The consumer should:

1. read raw websocket message bytes
2. parse into `tap.Event`
3. call `HandleEvent(ctx, rawPayload, event)`
4. send `{"type":"ack","id":event.ID}` only if the handler succeeds

The audit-aware `IndexHandler` should:

- call `AuditRepository.IngestTapEvent`
- avoid separate `RecordsRepository` / `ActorsRepository` writes in audit mode, because the audit repository owns transactional DB writes
- publish GraphQL subscriptions only after the transaction succeeds
- treat duplicate decoded events as successful no-ops for current-state updates

Legacy activity logging is not the audit source of truth. If kept, it should be best-effort after commit and must not block Tap acknowledgement.

## Current-state behavior

For create/update:

- append a `record_events` row
- if the semantic event is new and the record body is present, upsert `record`
- if the record body is missing, keep the audit row but skip the current projection update

For delete:

- append a `record_events` row with `action = "delete"`
- set `cid` and `record` to null/empty if Tap does not provide them
- remove the row from the current `record` table

For identity events:

- append an `identity_events` row
- keep the existing Tap identity behavior for current `actor` and record purging
- do not create synthetic per-record delete audit events for identity purges in the first implementation; the identity audit row explains why current records disappeared

## GraphQL audit API

Add a separate audit query alongside existing current-state queries. Avoid the type name `RecordEvent` because it is already used by subscriptions.

GraphQL wiring needs `AuditRepository` on the resolver repository context and service container, similar to the existing `Records`, `Actors`, and `Lexicons` repositories.

Suggested schema shape:

```graphql
type AuditRecordEvent {
  id: ID!
  receivedAt: String!
  live: Boolean!
  rev: String!
  did: String!
  collection: String!
  rkey: String!
  uri: String!
  action: AuditRecordAction!
  cid: String
  record: JSON
}

enum AuditRecordAction {
  CREATE
  UPDATE
  DELETE
}

type Query {
  auditRecordEvents(
    first: Int = 50
    after: String
    where: AuditRecordEventWhere
    orderBy: AuditRecordEventOrder
  ): AuditRecordEventConnection!
}
```

Supported filters for the first version:

- `id`
- `uri`
- `did`
- `collection`
- `rkey`
- `action`
- `live`
- `rev`
- `cid`
- `receivedAt`

Cursor pagination should use an opaque cursor encoding `record_events.id`.

## Example audit queries

### Full audit trail for one record

```graphql
query RecordAudit($uri: String!) {
  auditRecordEvents(
    first: 100
    where: { uri: { eq: $uri } }
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        did
        collection
        rkey
        uri
        cid
        rev
        live
        record
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

Variables:

```json
{
  "uri": "at://did:plc:alice/org.hypercerts.claim/abc123"
}
```

### All deletes in a collection

```graphql
query DeletedClaims {
  auditRecordEvents(
    first: 50
    where: {
      collection: { eq: "org.hypercerts.claim" }
      action: { eq: DELETE }
    }
    orderBy: { field: ID, direction: DESC }
  ) {
    edges {
      node {
        id
        receivedAt
        uri
        did
        rkey
        rev
      }
    }
  }
}
```

### All changes by one DID

```graphql
query ActorAudit($did: String!) {
  auditRecordEvents(
    first: 50
    where: { did: { eq: $did } }
    orderBy: { field: ID, direction: DESC }
  ) {
    edges {
      node {
        id
        receivedAt
        action
        collection
        uri
        cid
        record
      }
    }
  }
}
```

### Changes after a cursor

```graphql
query AuditSince($after: String!) {
  auditRecordEvents(first: 1000, after: $after) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        uri
        cid
        record
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

## Tests

Add tests for:

1. create event inserts raw, audit, and current-state rows
2. duplicate delivery inserts multiple raw rows but one `record_events` row
3. update event appends a second audit row and updates current state
4. delete event appends audit history and removes current state
5. create/update with missing record body is audited but does not poison Tap ack flow
6. create/update with missing CID is audited with an empty/null CID slot in the event key
7. record event with missing rev uses the fallback key and is acknowledged after commit
8. identity event appends identity audit history and updates current actor state
9. identity purge removes current records without creating synthetic record delete events
10. GraphQL `auditRecordEvents` filters by `uri`, `did`, `collection`, and `action`
11. GraphQL cursor pagination uses stable `record_events.id` cursors

## Documentation notes for users

Document these limitations clearly:

- Audit history starts when Tap audit ingestion starts. It does not reconstruct edits from before this indexer observed the repo.
- `live = false` means Tap emitted the event from backfill/resync, not that the record is inactive.
- `live = true` means Tap emitted the event from the relay firehose after following the repo.
- Deletes are permanent audit events even though the current `record` row is removed.
- Duplicate Tap deliveries are expected and are preserved in `raw_tap_events`.
- Identity purges are represented by `identity_events`; they do not create synthetic per-record delete audit events in the first implementation.
- Malformed or unsupported websocket frames are not guaranteed to appear in `raw_tap_events` in the first implementation.

## Implementation order

1. Add `AUDIT_ENABLED` config validation and startup wiring.
2. Add migrations for audit tables.
3. Add `AuditRepository` ingest and query methods with transaction-local SQL for current projection changes.
4. Adapt Tap consumer dispatch to pass raw payload and full events through `HandleEvent`.
5. Wire the audit repository into the Tap index handler when `AUDIT_ENABLED=true`.
6. Add GraphQL `auditRecordEvents` query and resolver repository plumbing.
7. Add docs and examples in the README or Tap deployment docs.
8. Verify SQLite first, then PostgreSQL if available.
