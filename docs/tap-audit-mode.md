# Tap audit mode

Tap audit mode stores append-only history for Tap record and identity deliveries while keeping the existing current-state GraphQL queries fast.

Enable it only with Tap ingestion:

```env
TAP_ENABLED=true
AUDIT_ENABLED=true
```

If `AUDIT_ENABLED=true` is set without `TAP_ENABLED=true`, Hyperindex fails startup because audit storage is Tap-only.

## What audit mode stores

When audit mode is enabled, every valid Tap record or identity delivery is processed in one database transaction:

```txt
Tap delivery
  -> raw_tap_events      append raw delivery bytes
  -> record_events       append deduped record create/update/delete event
  -> identity_events     append identity event
  -> record / actor      update current-state projection
  -> ack Tap after commit when TAP_DISABLE_ACKS=false
```

The existing `record` and `actor` tables remain the current-state projection. Current-state GraphQL queries keep using those tables. Audit history is stored beside them.

### `raw_tap_events`

`raw_tap_events` stores every successfully parsed Tap record or identity delivery, including duplicates. This is useful because Tap is at-least-once and may redeliver after reconnects, crashes, or missed acknowledgements.

### `record_events`

`record_events` stores immutable record changes. Duplicate semantic record events are deduped by a key based on DID, revision, collection, record key, action, and CID.

When Tap omits the revision, Hyperindex uses a weaker fallback key based on the Tap delivery id and a normalized payload hash so ingestion can still commit and acknowledge the delivery.

A duplicate semantic record event still appends a new `raw_tap_events` row, but it does not append a second `record_events` row and does not update the current `record` projection again.

### `identity_events`

`identity_events` stores Tap identity changes. Identity purges remove current `record` and `actor` rows for the DID, but they do not create synthetic per-record delete audit events. The identity audit row is the explanation for the purge.

## Current-state behavior

- Create/update with a record body appends audit history and upserts the current `record` row.
- Create/update without a record body appends audit history but skips the current `record` update.
- Delete appends audit history and removes the current `record` row.
- Identity events append audit history and update or purge the current `actor`/`record` projection.

## Querying record audit history

Use `auditRecordEvents` for append-only record history. The `RecordEvent` type name is reserved for subscriptions, so audit rows use `AuditRecordEvent`.

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

### Deletes in a collection

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

### Changes by one DID

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

### Continue after a cursor

```graphql
query AuditSince($after: String!) {
  auditRecordEvents(
    first: 1000
    after: $after
    orderBy: { field: ID, direction: ASC }
  ) {
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

## Limitations

- Audit history starts when Tap audit mode starts. It does not reconstruct older edits that this indexer did not observe.
- `live = false` means Tap emitted the event from backfill or resync, not that the record is inactive.
- `live = true` means Tap emitted the event from the relay firehose after following the repo.
- Deletes are permanent audit events even though the current `record` row is removed.
- Duplicate Tap deliveries are expected and are preserved in `raw_tap_events`.
- Identity purges are represented by `identity_events`; they do not create synthetic per-record delete audit events.
- Malformed or unsupported websocket frames are not guaranteed to appear in `raw_tap_events` in the first implementation.
