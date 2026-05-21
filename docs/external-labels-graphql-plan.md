# External Labels GraphQL Exposure Implementation Plan

## Scope

Expose locally stored external ATProto labeler events through public GraphQL.

This slice will:

- Add a public GraphQL `ExternalLabel` type.
- Add a generic `externalLabels` query for subject-based lookup.
- Add a virtual `externalLabels` field to every generated record type.
- Add the same virtual `externalLabels` field to `GenericRecord`.
- Add `where.externalLabels` filtering to generated collection queries.
- Keep pagination correct by applying label filters inside the main record SQL query before `LIMIT`.
- Batch-hydrate labels for returned records to avoid N+1 queries.
- Add compound indexes that support active-label lookup and record filtering at scale.

This slice will not:

- Dynamically subscribe to arbitrary labelers from request headers.
- Implement `com.atproto.label.queryLabels` fallback.
- Verify label signatures.
- Apply moderation/redaction behavior for `!takedown`, `!suspend`, or `redact`.
- Merge external labeler data into the existing local/admin `label` table.
- Interpret label values using labeler declaration records or label definitions.

## Background

ATProto labels are metadata over a subject. The subject is represented by the label `uri` field:

- A record label targets an AT-URI such as `at://did:plc:abc/collection/rkey`.
- An account label targets a DID such as `did:plc:abc`.
- A CID-specific label targets a specific version of a record using both `uri` and `cid`.

Hyperindex already stores raw external labels in the `external_label` table. Those labels should remain separate from the existing local/admin `label` table, which is used by Hyperindex's admin moderation features.

The public GraphQL model should therefore treat labels as generic subject metadata, not as fields that belong to any specific application concept like activity claims, orgs, or reviews.

## GraphQL API

### Public type

Add a public GraphQL type:

```graphql
type ExternalLabel {
  src: String!
  uri: String!
  cid: String
  val: String!
  neg: Boolean!
  cts: String!
  exp: String
  ver: Int
}
```

Do not expose `subscriptionURL`, `seq`, `labelIndex`, `sig`, or `rawJson` in the first public API. Those are ingestion/debugging details rather than consumer-facing label metadata.

### Generic subject lookup

Add a root query:

```graphql
externalLabels(
  subjects: [String!]!
  sources: [String!]
  values: [String!]
  activeOnly: Boolean = true
): [ExternalLabel!]!
```

Example:

```graphql
{
  externalLabels(
    subjects: ["at://did:plc:abc/org.hypercerts.claim.activity/123"]
    values: ["high-quality"]
  ) {
    src
    uri
    cid
    val
    cts
  }
}
```

### Generated record hydration

Inject a virtual field into every generated record type:

```graphql
externalLabels(
  sources: [String!]
  values: [String!]
  activeOnly: Boolean = true
): [ExternalLabel!]!
```

This field is Hyperindex metadata, not a lexicon field. It follows the same pattern as existing injected metadata fields such as `uri`, `cid`, `did`, and `rkey`.

Example:

```graphql
{
  orgHypercertsClaimActivity(first: 20) {
    edges {
      node {
        uri
        cid
        externalLabels {
          src
          val
          cts
        }
      }
    }
  }
}
```

Also add the same field to `GenericRecord`:

```graphql
{
  records(collection: "org.hypercerts.claim.activity", first: 20) {
    edges {
      node {
        uri
        cid
        externalLabels(values: ["high-quality"]) {
          src
          val
        }
      }
    }
  }
}
```

## Filtering API

Inject a virtual `externalLabels` field into every generated collection `WhereInput`.

Suggested input shape:

```graphql
input ExternalLabelWhereInput {
  has: ExternalLabelPredicateInput
  none: ExternalLabelPredicateInput
}

input ExternalLabelPredicateInput {
  src: StringFilterInput
  val: StringFilterInput
  activeOnly: Boolean = true
}
```

Example:

```graphql
{
  orgHypercertsClaimActivity(
    first: 20
    where: {
      externalLabels: {
        has: {
          val: { eq: "high-quality" }
        }
      }
    }
  ) {
    edges {
      cursor
      node {
        uri
        cid
        externalLabels {
          src
          val
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

`where.externalLabels` controls which records qualify. The `node.externalLabels(...)` field controls which labels are returned for each qualifying record. These can be independent. For example, a query can filter for records that have `high-quality` while returning all active labels on those records.

## Active-label semantics

By default, public label queries should return active labels only.

A label is active when:

- It is the latest label for the tuple `(src, uri, val)`.
- The latest label is not a negation (`neg = false`).
- `exp` is null or later than the current time.
- If the label has a `cid`, it only matches the record with that exact CID.

Keep `cid` out of the latest-label identity tuple. ATProto labels define currentness by source, subject URI, and value. `cid` narrows which record version the label applies to, but it should not turn `(src, uri, val)` into `(src, uri, cid, val)` for active-label semantics.

Use `cts` as the primary latest-label ordering because that is the protocol-defined created-at timestamp. Since timestamps can tie or be untrustworthy, use the local `id` as a deterministic tie-breaker:

```sql
newer.cts > el.cts
OR (newer.cts = el.cts AND newer.id > el.id)
```

Negation labels should still be stored in `external_label`, but should not be returned when `activeOnly = true`.

Expired labels should still be stored, but should not be returned when `activeOnly = true`.

For generic subject lookup with `activeOnly = false`, return historical/raw matching rows ordered by newest first, using `cts DESC, id DESC` for deterministic ordering.

## Repository changes

Extend `internal/database/repositories/external_labels.go`.

Suggested types:

```go
// LabelSubject identifies a subject whose external labels should be queried.
// URI is an AT-URI for record subjects or a DID for account subjects. CID is
// optional and is used to match CID-specific record labels.
type LabelSubject struct {
    URI string
    CID string
}

// ExternalLabelFilter restricts external labels by source, value, and active
// status. Empty Sources or Values means no restriction for that dimension.
type ExternalLabelFilter struct {
    Sources    []string
    Values     []string
    ActiveOnly bool
}
```

Suggested methods:

```go
func (r *ExternalLabelsRepository) GetBySubjects(
    ctx context.Context,
    subjects []LabelSubject,
    filter ExternalLabelFilter,
) (map[string][]ExternalLabel, error)
```

The returned map should be keyed by the requested subject, not just raw `uri`, so the same URI can be queried with different CIDs without accidental grouping. A helper such as `LabelSubject.Key()` can use `uri` for account/URI-only subjects and `uri + "\x00" + cid` for CID-specific subjects.

CID matching should happen in SQL. For a record subject with CID, match labels where `el.uri = subject.uri` and `(el.cid IS NULL OR el.cid = subject.cid)`. For a subject without CID, match labels where `el.uri = subject.uri`; if the subject is a generic URI-only lookup, CID-specific labels may be returned because there is no record CID to compare against.

Also add helper functions for building label `EXISTS` predicates used by `RecordsRepository`, or define a small query-shaping type that `RecordsRepository` can consume without knowing GraphQL details.

## Database indexes

Add a new migration after `007_add_external_label_ingestion` for both SQLite and PostgreSQL.

The active-label lookup and record filtering paths need a compound index. The existing `uri`, `src`, and `val` single-column indexes are not enough for the correlated latest-label predicate.

Suggested index:

```sql
CREATE INDEX IF NOT EXISTS idx_external_label_active_lookup
ON external_label(uri, val, src, cid, cts DESC, id DESC);
```

This supports:

- subject lookup by `uri`
- value/source filters
- CID applicability checks
- latest-label checks by `cts DESC, id DESC`

Keep the existing subscription sequence index for ingestion cursor/event debugging paths.

## Record query filtering

Extend record repository query methods to accept external label filters in addition to existing JSON field filters and DID filters.

Current query inputs are roughly:

```txt
collection + JSON field filters + DID filter + sort + cursor + limit
```

New query inputs should be:

```txt
collection + JSON field filters + DID filter + external label filters + sort + cursor + limit
```

The external label filter must be applied in SQL before pagination `LIMIT`.

SQL shape:

```sql
SELECT r.*
FROM record r
WHERE r.collection = ?
  AND EXISTS (
    SELECT 1
    FROM external_label el
    WHERE el.uri = r.uri
      AND (el.cid IS NULL OR el.cid = r.cid)
      AND el.val = ?
      -- optional source predicates
      -- active-label predicates
  )
  -- keyset cursor predicates
ORDER BY r.indexed_at DESC, r.uri DESC
LIMIT ?;
```

For active labels, the `EXISTS` predicate must account for the latest row per `(src, uri, val)`. The exact SQL can be implemented with a correlated `NOT EXISTS` against newer rows, or a derived latest-label subquery, as long as both SQLite and PostgreSQL are supported.

Conceptual active predicate:

```sql
el.neg = false
AND (el.exp IS NULL OR el.exp > current_timestamp)
AND NOT EXISTS (
  SELECT 1
  FROM external_label newer
  WHERE newer.src = el.src
    AND newer.uri = el.uri
    AND newer.val = el.val
    AND (
      newer.cts > el.cts
      OR (newer.cts = el.cts AND newer.id > el.id)
    )
)
```

Use dialect-specific boolean and timestamp handling where needed. `cts` and `exp` are stored as text in the current schema, so comparisons should use normalized RFC3339 values or dialect-safe timestamp casts/functions instead of assuming arbitrary text timestamps sort correctly.

## Pagination behavior

Pagination works when label filtering happens inside the record query.

Correct flow:

1. Apply collection filter.
2. Apply normal `where` filters.
3. Apply `where.externalLabels` as SQL `EXISTS` / `NOT EXISTS`.
4. Apply keyset cursor condition.
5. Order deterministically.
6. Fetch `first + 1` records.
7. Use the extra record to compute `hasNextPage`.
8. Batch-hydrate labels only for returned records.

Do not fetch a page first and then remove non-matching records in Go. That would break pagination by returning partial pages even when more matching records exist later.

## Avoiding N+1 queries

Do not implement `externalLabels` as a per-record database resolver.

Bad flow:

```txt
1 query for 20 records
20 separate label queries
```

Instead, batch-hydrate from the parent record resolver:

1. Fetch the page of records.
2. Inspect the GraphQL selection set to determine whether `externalLabels` was requested.
3. Collect all `{uri, cid}` pairs from the returned page.
4. Fetch labels for all subjects in one `GetBySubjects` call.
5. Group labels by subject URI.
6. Attach `externalLabels` to each node map.
7. Let the field resolver read the pre-attached value from `p.Source`.

If `externalLabels` is not selected, skip the label query entirely.

## GraphQL integration points

### `internal/graphql/types/object.go`

- Add `externalLabels` to `ReservedRecordFields` so lexicon fields cannot silently collide with the virtual field.
- Inject `externalLabels` into generated record types in `buildRecordFields`.
- The field resolver should return a pre-attached value from the source map. It should not query the database per record.

### `internal/graphql/schema/builder.go`

- Define `ExternalLabel` GraphQL type.
- Add `externalLabels` to `GenericRecord`.
- Add root `Query.externalLabels`.
- Add `ExternalLabelWhereInput` and `ExternalLabelPredicateInput`.
- Inject `externalLabels` into every generated `WhereInput`.
- Extend `extractFilters` to parse virtual label filters separately from JSON field filters.
- Extend record resolution to batch-hydrate labels only when selected.

### `internal/graphql/resolver/context.go`

Add `ExternalLabels` to public resolver repositories:

```go
type Repositories struct {
    Records        *repositories.RecordsRepository
    Actors         *repositories.ActorsRepository
    Lexicons       *repositories.LexiconsRepository
    ExternalLabels *repositories.ExternalLabelsRepository
}
```

Update `NewRepositories` accordingly.

### `cmd/hyperindex/main.go`

Pass `svc.externalLabels` into the public GraphQL repository bundle in `setupGraphQL`.

### `internal/database/repositories/records.go`

- Add repository-level representation for external label filters.
- Thread those filters through sorted and reversed keyset query methods.
- Implement SQL generation for `has` and `none` predicates.
- Keep SQLite and PostgreSQL behavior aligned.

## Header support

ATProto defines `atproto-accept-labelers` and `atproto-content-labelers` for choosing label sources on a request.

For this slice, prefer explicit GraphQL arguments first:

```graphql
externalLabels(sources: ["did:web:labeler.example"])
```

Do not dynamically subscribe to new labelers based on request headers yet.

Optional follow-up:

- Parse `atproto-accept-labelers` in `internal/graphql/handler.go`.
- Store accepted labeler DIDs in request context.
- Use them as default `sources` for hydrated `externalLabels` fields when field args omit `sources`.
- Set `atproto-content-labelers` in the response with labelers that are configured/available locally.
- Reject or explicitly ignore unsupported `redact` parameters until redaction behavior is implemented.

## Tests

### Migration/index tests

Add or extend migration tests to confirm the external label active-lookup index exists for both SQLite and PostgreSQL migration sets.

### Repository tests

Add tests for `ExternalLabelsRepository.GetBySubjects`:

- returns labels for multiple subjects in one call
- filters by `sources`
- filters by `values`
- active labels exclude latest negations
- active labels exclude expired labels
- CID-specific labels only match the correct record CID
- active-label tie-breaking is deterministic when two rows have the same `cts`
- `activeOnly = false` returns historical rows including negations ordered by `cts DESC, id DESC`

### Record query tests

Add tests for `RecordsRepository` label filtering:

- `has.val.eq` returns only records with that active label
- `has.src.in` limits matching to selected labelers
- `none` excludes records with matching active labels
- negated latest labels do not qualify as active
- expired labels do not qualify as active
- CID-specific labels only match the correct record CID
- latest-label tie-breaking uses `cts` first and `id` second
- keyset pagination returns full pages when enough matching records exist

### GraphQL schema tests

Add tests that introspection or query execution confirms:

- generated record types include `externalLabels`
- `GenericRecord` includes `externalLabels`
- generated `WhereInput` includes `externalLabels`
- lexicon fields named `externalLabels` are handled as reserved collisions

### GraphQL resolver tests

Add tests for:

- root `externalLabels(subjects: ...)`
- typed collection query returning hydrated `externalLabels`
- generic `records` query returning hydrated `externalLabels`
- typed collection query with `where.externalLabels.has.val.eq`
- typed collection pagination with label filtering

If feasible, add a test that ensures label hydration is batched rather than performed once per record. If that is awkward with current repository types, rely on code structure and integration tests for now.

## Documentation updates

Update the README or GraphQL docs to explain:

- External labels are labels received from configured ATProto labeler subscriptions.
- Labels attach to generic ATProto subjects, not to app-specific entities or fields inside a record.
- Generated record types include `externalLabels` as a Hyperindex virtual field.
- `where.externalLabels` filters records before pagination, so pagination remains valid.
- `where.externalLabels` and `node.externalLabels(...)` are independent: the `where` clause decides which records qualify, while the field arguments decide which labels are displayed for each returned record. To display only the labels used for filtering, repeat the same source/value constraints on the field.
- `sources`/`values` field arguments are convenience list filters; `where.externalLabels.has.src` and `where.externalLabels.has.val` use generic filter-input naming that mirrors the underlying label fields.
- Signatures are stored but not verified yet.
- Hyperindex only serves labels already ingested locally; it does not subscribe to arbitrary request-provided labelers yet.

## Changie

This will be externally meaningful for consumers of the public GraphQL API, so the implementation PR should include a Changie fragment.

Affects: `user`

Suggested summary:

```txt
Expose locally ingested external ATProto labels in public GraphQL record queries and subject lookups.
```
