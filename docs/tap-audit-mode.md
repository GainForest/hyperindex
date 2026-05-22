# Append-only indexing with Tap

Hyperindex can run as a Tap-backed append-only indexer. In this mode, Tap verifies and orders AT Protocol repository events, and Hyperindex persists every valid Tap delivery to audit tables before updating the fast current-state `record` and `actor` projections used by normal GraphQL queries.

Use this mode when operators or downstream consumers need to answer both questions:

1. **What is the latest state?** Query `records`, typed collection fields, search, and `collectionStats`.
2. **What did the indexer observe over time?** Query `auditRecordEvents` or inspect the raw audit tables.

## At a glance

Enable append-only indexing only with Tap ingestion:

```env
TAP_ENABLED=true
AUDIT_ENABLED=true
TAP_URL=ws://localhost:2480
TAP_ADMIN_PASSWORD=replace-with-your-tap-admin-password
```

Configure the Tap sidecar to decide which repos and records it should deliver:

```env
TAP_SIGNAL_COLLECTION=app.certified.actor.profile
TAP_COLLECTION_FILTERS=app.certified.*,org.hypercerts.*
```

`AUDIT_ENABLED=true` without `TAP_ENABLED=true` is invalid. Hyperindex fails startup because audit storage currently depends on Tap delivery semantics.

## Why Tap is the recommended ingestion path

Tap is Bluesky's verified synchronization sidecar for AT Protocol repositories. Compared with the legacy Jetstream + backfill path, Tap gives Hyperindex stronger operational behavior:

- repo structure, MST integrity, and identity signatures are verified upstream by Tap
- events are processed with strict per-repo ordering
- ack-based delivery gives at-least-once behavior after crashes or reconnects
- identity updates are delivered alongside record events
- backfill and live follow are handled by one sidecar instead of separate workers

Append-only audit mode builds on those properties. Hyperindex commits audit history and current state in one transaction, then acknowledges Tap only after the commit succeeds when `TAP_DISABLE_ACKS=false`.

## Data flow

For each supported Tap `record` or `identity` delivery, Hyperindex performs one transaction:

```txt
Tap delivery
  -> raw_tap_events      append raw delivery bytes
  -> record_events       append deduped record create/update/delete event, when delivery type is record
  -> identity_events     append identity event, when delivery type is identity
  -> record / actor      update current-state projection
  -> ack Tap             after commit, unless TAP_DISABLE_ACKS=true
```

The current-state tables remain intentionally small and query-friendly:

- `record` stores the latest known version of each indexed record.
- `actor` stores the latest known identity state for each DID.

The append-only audit tables preserve what Hyperindex observed:

| Table | Purpose | Public GraphQL access |
|-------|---------|-----------------------|
| `raw_tap_events` | Every successfully parsed Tap `record` or `identity` delivery, including duplicate deliveries. | Operator/database access only. |
| `record_events` | Immutable record create/update/delete events, deduped by semantic event key. | `auditRecordEvents` query. |
| `identity_events` | Identity changes such as handle/status updates and purges. | Operator/database access only. |

## Local Tap test setup

The fastest local append-only test is usually: run Tap in Docker, run Hyperindex on the host.

Start Tap:

```bash
docker volume create hyperindex-tap-data

docker run --rm --name hyperindex-tap \
  -p 127.0.0.1:2480:2480 \
  -v hyperindex-tap-data:/data \
  -e TAP_DATABASE_URL=sqlite:///data/tap.db \
  -e TAP_ADMIN_PASSWORD=local-tap-password \
  -e TAP_SIGNAL_COLLECTION=app.certified.actor.profile \
  -e TAP_COLLECTION_FILTERS=app.certified.*,org.hypercerts.* \
  -e TAP_DISABLE_ACKS=false \
  ghcr.io/bluesky-social/indigo/tap:latest
```

In another shell, run Hyperindex against that Tap sidecar:

```bash
export TAP_ENABLED=true
export AUDIT_ENABLED=true
export TAP_URL=ws://localhost:2480
export TAP_ADMIN_PASSWORD=local-tap-password
export LEXICON_DIR=testdata/lexicons
export DATABASE_URL=sqlite:data/hyperindex-local-tap.db
export ADMIN_API_KEY="$(openssl rand -base64 32)"
export SECRET_KEY_BASE="$(openssl rand -hex 32)"
export EXTERNAL_BASE_URL=http://localhost:8080

make run
```

Check health:

```bash
curl -fsS http://localhost:8080/health
curl -fsS -u admin:local-tap-password http://localhost:2480/health
```

Query recent append-only record events:

```bash
curl -fsS http://localhost:8080/graphql \
  -H 'Content-Type: application/json' \
  --data '{"query":"{ auditRecordEvents(first: 5) { edges { node { id receivedAt action did collection uri live } } } }"}'
```

Stop local services:

```bash
docker stop hyperindex-tap
```

Remove `hyperindex-tap-data` if you want Tap to start from a clean SQLite database next time.

## Tap network selection

Tap separates **which repos to follow** from **which collections to emit**:

- `TAP_SIGNAL_COLLECTION` tells Tap to discover and follow every repo with at least one record in that collection.
- `TAP_COLLECTION_FILTERS` tells Tap which record collections to deliver for followed repos.

If you care about the signal collection records themselves, include the signal collection in the filters too. For example:

```env
TAP_SIGNAL_COLLECTION=app.certified.actor.profile
TAP_COLLECTION_FILTERS=app.certified.*,org.hypercerts.*
```

This tracks repos that publish `app.certified.actor.profile` and emits matching `app.certified.*` and `org.hypercerts.*` record events for those repos. Identity events are delivered for tracked repos regardless of record collection filters.

## GraphQL API

Current-state queries do not change when audit mode is enabled. Use existing queries for fast latest-state reads:

```graphql
query CurrentProfiles {
  appCertifiedActorProfile(first: 20) {
    edges {
      node { uri did rkey }
    }
  }
}
```

Use the built-in `auditRecordEvents` query for append-only record history. It is a first-class public GraphQL query, but it is not generated from lexicons. There is intentionally no `recordEvents` query; `record_events` is the internal database table name.

### Latest audit events

```graphql
query LatestAuditEvents {
  auditRecordEvents(first: 20) {
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

## Filtering and ordering

`auditRecordEvents` supports exact filters for:

- `id`
- `uri`
- `did`
- `collection`
- `rkey`
- `action`
- `live`
- `rev`
- `cid`

It also supports `receivedAt` equality and open-ended ranges with `eq`, `gt`, and `lt`.

Ordering is stable by append-only row id:

```graphql
orderBy: { field: ID, direction: ASC }
```

The default order is newest first.

## Smoke testing audit mode

The normal API smoke suite does not require audit history, because not every deployment runs with `AUDIT_ENABLED=true`.

Set `HYPERINDEX_SMOKE_AUDIT=1` to opt into audit checks:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_AUDIT=1 \
  make smoke-api
```

The audit smoke requires at least 5 audit record events by default. Override the minimum when an environment should prove more history exists:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_AUDIT=1 \
  HYPERINDEX_SMOKE_AUDIT_MIN_EVENTS=25 \
  make smoke-api
```

A passing audit smoke proves that:

- the public schema exposes `auditRecordEvents`
- at least the configured number of audit record events exists
- returned audit rows have usable cursors and expected fields such as `id`, `receivedAt`, `did`, `collection`, `rkey`, `uri`, and `action`

## Operator checklist

Before treating a deployment as an append-only indexer, verify:

- `TAP_ENABLED=true`
- `AUDIT_ENABLED=true`
- `TAP_DISABLE_ACKS=false` unless you are intentionally debugging fire-and-forget delivery
- `TAP_SIGNAL_COLLECTION` discovers the repos you expect
- `TAP_COLLECTION_FILTERS` includes every collection namespace you want indexed
- lexicons are loaded before backend startup through `LEXICON_DIR`, database registration, or uploaded lexicons
- the deployment uses durable storage for both Tap and Hyperindex databases
- `HYPERINDEX_SMOKE_AUDIT=1 make smoke-api` passes after deployment

## Duplicate deliveries and current-state updates

Tap delivery is at-least-once. Duplicate deliveries can happen after reconnects, crashes, or missed acknowledgements.

Hyperindex handles that by preserving raw deliveries and deduping semantic record events:

- every successfully parsed duplicate delivery still creates a new `raw_tap_events` row
- duplicate semantic record events do not create duplicate `record_events` rows
- duplicate semantic record events do not update the current-state `record` projection again

When Tap omits a repository revision, Hyperindex uses a weaker fallback event key based on Tap delivery id and a normalized payload hash so the delivery can still be committed and acknowledged.

## Identity events and purges

Identity events are stored in `identity_events`. They update the current `actor` projection, and purge-style identity statuses remove current `record` and `actor` rows for that DID.

Identity purges do not create synthetic per-record delete audit events. The identity audit row is the reason current-state rows disappeared.

## Limitations

- Audit history starts when Tap audit mode starts. It does not reconstruct older edits that this indexer did not observe.
- `live = false` means Tap emitted the event from backfill or resync, not that the record is inactive.
- `live = true` means Tap emitted the event from the relay firehose after following the repo.
- Deletes are permanent audit events even though the current `record` row is removed.
- Duplicate Tap deliveries are expected and are preserved in `raw_tap_events`.
- Identity purges are represented by `identity_events`; they do not create synthetic per-record delete audit events.
- Malformed or unsupported websocket frames are not guaranteed to appear in `raw_tap_events`.
