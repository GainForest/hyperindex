# Record Timeline GraphQL API

## Status

Proposed implementation spec for replacing the certified-app-specific `followerEvents` request from GitHub issue #76 with a generic Hyperindex query primitive.

This spec is documentation/planning only. It does not describe a currently deployed public API until the matching GraphQL, repository, migration, and documentation changes land.

## Summary

Add a generic `recordTimeline` root query that returns one newest-first page of current records across selected collections, optionally filtered to selected author DIDs, with a single stable cursor.

Certified-app can build a home feed by resolving the viewer's followed DIDs separately through `app.certified.graph.follow`, then passing those DIDs and its chosen Hypercerts/Certified collections through `recordTimeline(where: { did: { in: ... }, collection: { in: ... } })`. Hyperindex remains a generic indexer; the application owns feed policy.

The resolver returns raw generic record nodes plus optional selection-based `certifiedProfileData` hydration for each record author. It is not an append-only operation log and does not emit update/delete events.

## Problem

Consumers can already query each typed collection independently:

```graphql
orgHypercertsCollection(where: { did: { in: $authors } }) { ... }
appCertifiedBadgeAward(where: { did: { in: $authors } }) { ... }
orgHypercertsClaimActivity(where: { did: { in: $authors } }) { ... }
```

A feed page can be reconstructed by querying several connections, merging results client-side, and sorting by `createdAt`. The hard part is stable pagination across N independent streams: each stream has its own cursor, and merging them into one infinite-scroll cursor is fragile.

A `followerEvents` resolver would solve the certified-app case, but it would bake application vocabulary and follow-graph policy into Hyperindex. The underlying useful primitive is broader:

> Return current records authored by a set of DIDs, across a selected set of collections, ordered by record creation time, with one keyset cursor.

## Goals

- Add a generic `recordTimeline` root query.
- Require callers to choose explicit collections.
- Optionally filter by author DIDs.
- Return current records only, ordered by materialized record creation time.
- Use keyset pagination with a compound `{ createdAt, uri }` cursor.
- Avoid per-collection GraphQL fanout inside the resolver.
- Avoid runtime JSON extraction/sorting for every request.
- Avoid exact `totalCount`; timeline consumers need `hasNextPage`, not a count.
- Keep SQLite and PostgreSQL as first-class supported databases with equivalent behavior.
- Return raw generic record data so the resolver is reusable.
- Provide optional selection-based `certifiedProfileData` author hydration to improve Hypercerts-stack DX without making the timeline typed or app-specific.

## Non-goals

- Do not resolve a viewer's follow graph inside `recordTimeline`.
- Do not add a `followerEvents(viewer: ...)` product resolver.
- Do not return typed union nodes for every selected collection.
- Do not make profile data affect timeline filtering, ordering, or pagination.
- Do not emit update/delete events.
- Do not move old records to the top when their indexed row is updated.
- Do not compute an exact `totalCount`.
- Do not add arbitrary joins to collection-specific tables or lexicon-specific concepts.

A future append-only `recordEvents` API can cover create/update/delete operation streams if that product need appears.

## Public GraphQL API

```graphql
type Query {
  recordTimeline(
    where: RecordTimelineWhereInput!
    first: Int = 50
    after: String
  ): RecordTimelineConnection!
}

input RecordTimelineWhereInput {
  collection: RecordTimelineCollectionFilterInput!
  did: DIDFilterInput
}

input RecordTimelineCollectionFilterInput {
  in: [String!]!
}
```

```graphql
type RecordTimelineConnection {
  edges: [RecordTimelineEdge!]!
  pageInfo: PageInfo!
}

type RecordTimelineEdge {
  cursor: String!
  node: RecordTimelineNode!
}

type RecordTimelineNode {
  uri: String!
  cid: String!
  did: DID!
  collection: String!
  rkey: String
  createdAt: DateTime!
  indexedAt: DateTime!
  value: JSON!

  certifiedProfileData: AppCertifiedActorProfile
}
```

`RecordTimelineConnection` intentionally omits `totalCount`.

### Arguments

| Argument | Required | Semantics |
| --- | --- | --- |
| `where` | Yes | Filter object. `where.collection.in` is required; `where.did.in` is optional. |
| `first` | No | Forward page size. Default `50`; maximum `1000`. |
| `after` | No | Opaque keyset cursor returned by a previous page. |

`where.collection.in` contains ATProto collection NSIDs to include. Empty lists are invalid. `where.did.in` contains author DIDs to include; omitted or `null` means no author filter, and an empty list returns an empty connection.

Suggested validation caps:

- `first <= 1000`
- `len(where.did.in) <= 1000`
- `len(where.collection.in) <= 25`
- every collection value must be a syntactically valid NSID-like collection string

### Example: followed-author home timeline

The application first resolves followed DIDs through the follow collection:

```graphql
query Following($viewer: DID!, $after: String) {
  appCertifiedGraphFollow(
    first: 100
    after: $after
    where: { did: { eq: $viewer } }
  ) {
    edges { node { subject } }
    pageInfo { hasNextPage endCursor }
  }
}
```

Then it calls the generic timeline query:

```graphql
query HomeTimeline($authors: [String!]!, $after: String) {
  recordTimeline(
    where: {
      did: { in: $authors }
      collection: {
        in: [
          "org.hypercerts.claim.activity"
          "org.hypercerts.collection"
          "app.certified.badge.award"
          "app.certified.actor.profile"
          "app.certified.actor.organization"
        ]
      }
    }
    first: 50
    after: $after
  ) {
    edges {
      cursor
      node {
        uri
        cid
        did
        collection
        rkey
        createdAt
        indexedAt
        value
        certifiedProfileData {
          did
          displayName
          avatar {
            __typename
            ... on OrgHypercertsDefsSmallImage { image { ref } }
          }
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

The client can render directly from `value` or group returned URIs by `collection` and hydrate typed records through existing `uri: { in: [...] }` filters.

## Semantics

### Current records, not events

`recordTimeline` reads the current `record` table.

- A create appears if the record currently exists and has a valid materialized creation timestamp.
- A delete removes the record from future pages.
- An update changes the raw `json`/`cid` but does not create a new timeline entry.
- An update should not move a record to the top unless the stored creation timestamp was previously null and becomes parseable.

### Ordering

Rows are ordered by:

```text
record_created_at DESC, uri DESC
```

`uri` is the stable tie-breaker for records with the same creation timestamp.

The edge cursor is an opaque base64url-encoded payload containing at least:

```json
{
  "v": 1,
  "createdAt": "2026-06-29T08:00:00.000Z",
  "uri": "at://did:plc:abc/org.hypercerts.collection/rkey"
}
```

A request with `after` returns rows strictly older than the cursor in that total order:

```sql
record_created_at < cursor.createdAt
OR (record_created_at = cursor.createdAt AND uri < cursor.uri)
```

### Creation timestamp

The timeline uses a materialized `record_created_at` column parsed from the record JSON's top-level `createdAt` field.

Records with missing, malformed, or unparseable `createdAt` are excluded from `recordTimeline` because the resolver cannot place them correctly in a creation-time feed.

### Author identity and profile hydration

`did` is always the record author's DID.

`certifiedProfileData` is an optional virtual field on the timeline node. It follows the same concept as the existing generated-node `certifiedProfileData` field: the resolver batch-fetches `at://<did>/app.certified.actor.profile/self` for distinct page authors and attaches the profile record before GraphQL resolves the field.

If no Certified profile record exists for an author, `certifiedProfileData` is `null`.

`author` is non-null virtual identity metadata on every timeline node. It exposes the record author's non-null DID and nullable current handle from the `actor` table; the legacy direct `did` output remains available but is deprecated. The resolver batch-fetches actors for the page's distinct DIDs only when `author` is selected.

## Database design

SQLite and PostgreSQL are both first-class targets. Migrations, repository behavior, and tests must support both dialects.

### Schema

Add a materialized creation-time column to `record`.

PostgreSQL:

```sql
ALTER TABLE record
  ADD COLUMN record_created_at TIMESTAMP WITH TIME ZONE;
```

SQLite:

```sql
ALTER TABLE record
  ADD COLUMN record_created_at TEXT;
```

SQLite values should be normalized RFC3339 UTC strings so lexicographic ordering matches chronological ordering.

### Ingest/upsert behavior

When inserting a record:

1. Parse top-level `json.createdAt` as a timestamp.
2. Store the normalized value in `record_created_at`.
3. If parsing fails, store `NULL`.

When updating an existing URI:

- Preserve an existing non-null `record_created_at`.
- Fill `record_created_at` if it is currently null and the incoming record has a parseable `createdAt`.
- Do not overwrite a non-null value with a different incoming `createdAt`; otherwise edits could move old records in the timeline.

PostgreSQL upsert sketch:

```sql
ON CONFLICT(uri) DO UPDATE SET
  cid = EXCLUDED.cid,
  json = EXCLUDED.json,
  indexed_at = NOW(),
  record_created_at = COALESCE(record.record_created_at, EXCLUDED.record_created_at)
```

SQLite upsert sketch:

```sql
ON CONFLICT(uri) DO UPDATE SET
  cid = excluded.cid,
  json = excluded.json,
  indexed_at = datetime('now'),
  record_created_at = COALESCE(record.record_created_at, excluded.record_created_at)
```

### Backfill

The migration should backfill `record_created_at` for existing rows where top-level `createdAt` is present and parseable.

If SQL-only parsing is too brittle across dialects, use a repository/admin backfill step that reads existing JSON and updates normalized values in batches. The important requirement is equivalent SQLite/PostgreSQL behavior.

### Indexes

Add an author-filtered timeline index:

PostgreSQL and SQLite:

```sql
CREATE INDEX idx_record_timeline_author_collection_created
ON record (did, collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL;
```

Typed collection connections sorted by `createdAt` keep rows whose `record_created_at` is NULL and order them as the oldest, so the partial indexes above and below cannot serve them. Migration 018 adds full indexes next to the partial ones; collection-wide timelines (`where.did` omitted) use the first as well:

```sql
-- PostgreSQL
CREATE INDEX CONCURRENTLY idx_record_collection_created_at
ON record (collection, record_created_at DESC NULLS LAST, uri DESC);

CREATE INDEX CONCURRENTLY idx_record_collection_created_at_valid
ON record (collection, record_created_at DESC NULLS LAST, uri DESC)
WHERE validation_status = 'valid';

-- Typed connections filtered by where.did
CREATE INDEX CONCURRENTLY idx_record_author_collection_created_at
ON record (did, collection, record_created_at DESC NULLS LAST, uri DESC);
```

SQLite creates the same indexes without `NULLS LAST`, because SQLite already sorts NULL as the smallest value.

Add a collection-first partial index for timelines that read only dated rows (migration 010), and a valid-only variant of it for collection-wide readers that select only `validation_status = 'valid'` rows, such as the GainForest feed service (migration 017). Without the valid-only variant, those reads walk the collection index and fetch every invalid row from the table just to discard it, which on a cold PostgreSQL cache costs one random read per skipped row:

```sql
CREATE INDEX idx_record_timeline_collection_created
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL;

CREATE INDEX idx_record_timeline_valid
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
```

These two partial indexes are retained because readers outside Hyperindex order `NULLS FIRST`, which the full indexes cannot serve. Drop them in a follow-up once those readers write `ORDER BY record_created_at DESC NULLS LAST, uri DESC`.

A PostgreSQL query can only walk the full indexes in order when its `ORDER BY` uses the same NULL placement. `ORDER BY record_created_at DESC` defaults to `NULLS FIRST` and falls back to a sort; write `ORDER BY record_created_at DESC NULLS LAST, uri DESC`, even when the query also filters `record_created_at IS NOT NULL`. The `recordTimeline` collection-wide query does this. Author-filtered `recordTimeline` reads keep the default placement and the partial author index above.

Keyset cursors must be index bounds. Write the cursor as a row-value comparison, `(record_created_at, uri) < ($c, $u)` for descending pages and `>` for ascending ones. The equivalent `record_created_at < $c OR (record_created_at = $c AND uri < $u)` is only a filter: PostgreSQL walks the index from its head and discards every row before the cursor, so each page costs more than the last. Typed connections add the NULL rows after the dated ones when descending (before them when ascending). A condition such as `dated range OR created IS NULL` cannot be an index bound either, so the page is read as separate statements, one per range: the first runs with `LIMIT n`, and the next runs only if the first returned fewer than n rows, with the shortfall as its limit. A page the dated rows fill therefore never touches the undated rows, which matters when a filter matches few of them. The statements are not one snapshot; a write between them that gives an undated record its first parseable `createdAt` can make that page repeat or miss that one record, the same effect a write between two pages has.

PostgreSQL-specific covering-index additions such as `INCLUDE (cid, rkey, indexed_at)` may be considered after benchmarking, but the base behavior and test expectations must remain dialect-independent.

## Repository query plan

### PostgreSQL author-filtered query

```sql
SELECT uri, cid, did, collection, rkey, json::text, record_created_at::text, indexed_at::text
FROM record
WHERE record_created_at IS NOT NULL
  AND did = ANY($1)
  AND collection = ANY($2)
  AND (record_created_at, uri) < ($3::timestamptz, $4) -- only when `after` is present
ORDER BY record.record_created_at DESC, uri DESC
LIMIT $5;
```

Use `first + 1` as `$5` to compute `hasNextPage` without `totalCount`.

Qualify `record.record_created_at` in `ORDER BY`. The select list outputs `record_created_at::text` under the same name, and PostgreSQL resolves a bare `ORDER BY record_created_at` to that text output column, which no index can serve.

### PostgreSQL global selected-collection query

When `where.did` is omitted, remove the `did = ANY($1)` predicate, order by `record.record_created_at DESC NULLS LAST, uri DESC`, and use `idx_record_collection_created_at`.

### SQLite query

SQLite should use the same logical predicates with generated `IN (?, ?, ...)` placeholders and normalized `TEXT` timestamps:

```sql
SELECT uri, cid, did, collection, rkey, json, record_created_at, indexed_at
FROM record
WHERE record_created_at IS NOT NULL
  AND did IN (?, ?, ...)
  AND collection IN (?, ?, ...)
  AND (record_created_at, uri) < (?, ?) -- only when `after` is present
ORDER BY record_created_at DESC, uri DESC
LIMIT ?;
```

## Resolver flow

1. Validate `where.collection.in`, optional `where.did.in`, `first`, and `after`.
2. Decode `after` into `{ createdAt, uri }` when present.
3. Query timeline records with `LIMIT first + 1`.
4. Slice to `first`; use the extra row to set `hasNextPage` and `endCursor`.
5. Convert each row into a generic timeline node.
6. If `certifiedProfileData` is selected:
   - collect distinct DIDs from the page,
   - build profile URIs `at://<did>/app.certified.actor.profile/self`,
   - fetch them with one batch `GetByURIs` call,
   - attach profile source maps by DID.
7. If `author` is selected:
   - batch-fetch actors for the same distinct DIDs,
   - attach `{ did, handle }` identity source maps before field resolution.

This is not N+1. The expected query count is:

- one query for timeline rows,
- plus one optional batch query for Certified profile records,
- plus one optional batch query for actor identities when `author` is selected.

## Performance expectations

With materialized timestamps and bounded inputs, the query should be safe for feed-style usage.

Expected typical bounds:

- followed authors: 50-500 DIDs,
- selected collections: 5-10,
- page size: 50-100.

The resolver must not sort by `json->>'createdAt'` or `json_extract(json, '$.createdAt')` at request time. That path would require evaluating and sorting many candidate rows as the table grows.

Profile hydration happens after pagination, so its cost is bounded by the number of distinct authors on one page, not by the full follow set.

## Error behavior

- Invalid `first` returns a GraphQL validation/resolver error that states the allowed range.
- Empty `where.collection.in` returns a clear error because a timeline over all indexed collections is too broad.
- Too many `where.did.in` or `where.collection.in` values returns a clear error with the configured maximum.
- Malformed `after` returns a clear cursor error.
- Missing or invalid `createdAt` on individual records excludes those records from the timeline; it should not fail the whole query.
- Profile hydration failures should fail the query only if the selected field cannot be resolved reliably; do not silently return partial profile data unless existing GraphQL conventions already allow that behavior.

## Testing plan

Repository tests, run against both SQLite and PostgreSQL:

- returns records across multiple collections in `record_created_at DESC, uri DESC` order,
- filters by author DID list,
- supports omitted `where.did` for global selected-collection timelines,
- returns empty connection for `where.did.in: []`,
- rejects empty `where.collection.in`,
- applies cursor pagination strictly after `{ createdAt, uri }`,
- excludes records with null `record_created_at`,
- preserves `record_created_at` on update,
- fills `record_created_at` when an existing null row receives a parseable timestamp,
- uses `first + 1` for `hasNextPage` without exact `totalCount`.

GraphQL schema/resolver tests:

- exposes `recordTimeline` and the connection/node fields,
- omits `totalCount`,
- returns the decoded `value` payload, metadata fields, and cursors,
- hydrates `certifiedProfileData` only when selected,
- does not hydrate profiles when the field is not selected,
- avoids N+1 profile lookup behavior with multiple records from the same author,
- preserves behavior for existing typed collection queries.

Migration tests:

- apply embedded SQLite and PostgreSQL migrations,
- backfill parseable top-level `createdAt` values,
- leave malformed/missing values null,
- create the expected indexes in both dialects.

## Documentation follow-up

When implemented and deployed, update:

- `.agents/skills/hyperindex/SKILL.md`,
- `.agents/skills/hyperindex/references/schema-reference.md`,
- consumer-facing GraphQL examples that currently recommend client-side multi-connection merging.

Do not document `recordTimeline` in the Hyperindex consumer skill as live behavior before it is actually available on the hosted endpoint.
