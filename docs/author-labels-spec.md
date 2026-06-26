# Author Account Labels for Record Connections

## Status

Proposed implementation spec for GitHub issue #92.

This spec covers Hyperindex GraphQL filtering for labels attached to a record author's account DID. It does not change how external labeler events are ingested; it uses the existing `external_label` table populated by configured external label subscriptions.

Related architecture visualization: [`author-labels-architecture.html`](author-labels-architecture.html).

## Problem

Hyperindex already supports record-level external label filtering:

```graphql
where: {
  externalLabels: {
    has: {
      src: { eq: "did:plc:labeler" }
      val: { eq: "high-quality" }
      activeOnly: true
    }
  }
}
```

That filter matches labels whose subject is the record itself:

```sql
external_label.uri = record.uri
```

This is correct for labels attached to an AT-URI such as:

```text
at://did:plc:author/org.hypercerts.claim.activity/rkey
```

It does not support account-quality labels emitted on the author DID, such as the live orglabeler labels from `orglabeler.hypercerts.dev`:

```json
{
  "src": "did:plc:pswneepkd5lesumj7ejmkbal",
  "uri": "did:plc:author",
  "val": "likely-test"
}
```

Certified-app needs to filter record connections by those author-account labels before pagination and before `totalCount`, especially to exclude records authored by likely-test accounts while still allowing unlabeled accounts during warmup.

## Goals

- Add `where.authorLabels` to generated record connection filters.
- Make `authorLabels` filter labels whose subject is the current record author's DID.
- Reuse the existing `ExternalLabelWhereInput` predicate shape: `has`, `none`, `src`, `val`, `activeOnly`.
- Apply `authorLabels` inside repository SQL before pagination and count calculation.
- Keep `externalLabels` behavior unchanged for record-subject labels.
- Avoid Certified-specific exceptions such as mapping `app.certified.actor.profile/self` or `app.certified.actor.organization/self` to the account.

## Non-goals

- Do not infer account labels from profile or organization record labels.
- Do not inspect labeler declaration records at query time.
- Do not add a generic public `labelSubject` enum until more subject scopes exist.
- Do not add GraphQL joins to fetch author profile or organization records for label matching.
- Do not change external label ingestion, signature verification, or labeler subscription configuration in this slice.
- Do not silently fall back from `authorLabels` to text search or client-side filtering.

## Definitions

### Record label

A label whose subject is an AT-URI. It may optionally include a CID to target one record version.

```json
{
  "uri": "at://did:plc:abc/org.hypercerts.claim.activity/123",
  "cid": "bafy...",
  "val": "high-quality"
}
```

### Account label

A label whose subject is a DID. Account labels should not carry a CID.

```json
{
  "uri": "did:plc:abc",
  "val": "likely-test"
}
```

### `externalLabels`

GraphQL filter and hydration field for labels attached to the record node itself.

### `authorLabels`

GraphQL filter for labels attached to the record author's account DID. For any record row, the author label subject is `record.did`.

## Public GraphQL API

Inject a virtual `authorLabels` field into every generated collection `WhereInput`, next to the existing `externalLabels` field.

```graphql
input SomeCollectionWhereInput {
  uri: URIFilterInput
  did: DIDFilterInput
  externalLabels: ExternalLabelWhereInput
  authorLabels: ExternalLabelWhereInput
  # generated lexicon field filters...
}
```

Descriptions should make the distinction explicit:

- `externalLabels`: "Filter records by locally ingested external labels attached to the record URI before pagination."
- `authorLabels`: "Filter records by locally ingested external labels attached to the record author's DID before pagination."

The input shape is reused:

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

### Example: exclude likely-test authors

```graphql
query PublicActivities($orglabeler: String!, $after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: {
      authorLabels: {
        none: {
          src: { eq: $orglabeler }
          val: { eq: "likely-test" }
          activeOnly: true
        }
      }
    }
  ) {
    edges {
      cursor
      node { uri did title createdAt }
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}
```

Variables:

```json
{
  "orglabeler": "did:plc:pswneepkd5lesumj7ejmkbal",
  "after": null
}
```

Unlabeled authors pass this `none` predicate. Only authors with an active matching `likely-test` label from the selected source are excluded.

### Example: require high-quality or standard authors

```graphql
where: {
  authorLabels: {
    has: {
      src: { eq: "did:plc:pswneepkd5lesumj7ejmkbal" }
      val: { in: ["standard", "high-quality"] }
      activeOnly: true
    }
  }
}
```

Unlabeled authors do not pass this `has` predicate.

### Example: combine record labels and author labels

```graphql
where: {
  externalLabels: {
    has: {
      src: { eq: "did:plc:activity-labeler" }
      val: { eq: "verified-impact" }
      activeOnly: true
    }
  }
  authorLabels: {
    none: {
      src: { eq: "did:plc:pswneepkd5lesumj7ejmkbal" }
      val: { eq: "likely-test" }
      activeOnly: true
    }
  }
}
```

Both predicates must pass.

## Semantics

### Subject binding

`externalLabels` and `authorLabels` use the same predicate shape but bind to different label subjects.

| GraphQL field | Label subject | SQL subject condition |
| --- | --- | --- |
| `externalLabels` | current record AT-URI | `el.uri = record.uri` and `(el.cid IS NULL OR el.cid = record.cid)` |
| `authorLabels` | current record author DID | `el.uri = record.did` and `el.cid IS NULL` |

`authorLabels` intentionally does not match labels on:

- `at://did/app.certified.actor.profile/self`
- `at://did/app.certified.actor.organization/self`
- any other account-carrier record

If a label is meant to describe account quality, the labeler should emit a DID-subject label.

### `has` and `none`

- `has` keeps records whose bound label subject has at least one matching label.
- `none` keeps records whose bound label subject has no matching label.
- If both are provided, both conditions must hold.
- Empty or omitted `src` means any labeler source can match.
- Empty or omitted `val` means any label value can match.

### Active labels

`activeOnly` defaults to `true`, matching existing external-label behavior.

An active label is:

- not negated,
- not expired,
- and not superseded by a newer label with the same `src`, `uri`, `cid`, and `val`.

For `authorLabels`, account labels should be stored with `cid = NULL`. CID-specific labels should not match `authorLabels`.

### Pagination and counts

`authorLabels` must be applied in the database query before:

- `LIMIT`,
- forward and backward cursor pagination,
- and `totalCount` calculation.

Filtering after fetching a page is incorrect because it creates short pages, unstable cursors, and wrong counters.

## Recommended internal design

Keep the public GraphQL API explicit (`externalLabels`, `authorLabels`) while using a small internal subject-binding abstraction to avoid duplicated SQL.

```go
type externalLabelSubject int

const (
    externalLabelSubjectRecord externalLabelSubject = iota
    externalLabelSubjectAuthor
)

// ExternalLabelFilterSet groups all external-label predicates that can apply to
// a record query. Record preserves the existing where.externalLabels behavior;
// Author powers where.authorLabels.
type ExternalLabelFilterSet struct {
    Record ExternalLabelRecordFilter
    Author ExternalLabelRecordFilter
}
```

`ExternalLabelRecordFilter` remains the subject-filter struct for compatibility, but the repository treats it as a predicate for whichever subject binding contains it.

### SQL builder

Generalize the existing label subquery builder so it receives the subject binding:

```go
func (r *RecordsRepository) buildExternalLabelExistsSubquery(
    alias string,
    subject externalLabelSubject,
    predicate ExternalLabelPredicate,
    startPlaceholder int,
) (string, []database.Value, int, error)
```

Subject conditions:

```go
switch subject {
case externalLabelSubjectRecord:
    conditions = append(conditions,
        fmt.Sprintf("%s.uri = record.uri", alias),
        fmt.Sprintf("(%s.cid IS NULL OR %s.cid = record.cid)", alias, alias),
    )
case externalLabelSubjectAuthor:
    conditions = append(conditions,
        fmt.Sprintf("%s.uri = record.did", alias),
        fmt.Sprintf("%s.cid IS NULL", alias),
    )
}
```

Then apply the existing source, value, and active-label predicates exactly as today.

Build the record and author subject clauses through one filter-set builder that carries a single monotonically increasing placeholder index across all `has` and `none` subqueries. This avoids PostgreSQL placeholder reuse when both `externalLabels` and `authorLabels` contain `src` or `val` predicates. Use distinct aliases such as `el_record_has`, `el_record_none`, `el_author_has`, and `el_author_none`.

### Repository query signatures

Replace the single external-label filter parameter with the filter set in all record-query paths that currently support external labels:

```go
GetByCollectionSortedWithKeysetCursorAndExternalLabels(
    ctx,
    collection,
    filters,
    didFilter,
    externalLabelFilters,
    sort,
    limit,
    afterCursorValues,
)
```

Do the same for:

- reversed keyset pagination,
- filtered counts,
- any helper wrappers that pass an empty external-label filter.

### GraphQL parsing

Update where-filter extraction to return a filter set:

```go
filters, didFilter, externalLabelFilters, err := extractFiltersWithExternalLabels(...)
```

Parsing rules:

- `where.externalLabels` populates `externalLabelFilters.Record`.
- `where.authorLabels` populates `externalLabelFilters.Author`.
- Existing unsupported operator errors should be preserved.
- Label filters should not count against JSON-field `MaxFilterConditions`, matching current `externalLabels` behavior.

### GraphQL schema builder

Add `authorLabels` wherever `externalLabels` is injected into generated `WhereInput` types.

Reserve or otherwise special-case `authorLabels` as a generated filter name so lexicon properties cannot overwrite `WhereInput.authorLabels`. Do not add a node field named `authorLabels` in the first slice. Consumers that need account labels for known DIDs can use the root `externalLabels(subjects: [...])` query. A node field can be added later if there is a proven hydration use case.

## Database and indexing

No migration is required for the first implementation.

The existing external label lookup index begins with `uri`:

```sql
CREATE INDEX IF NOT EXISTS idx_external_label_active_lookup
ON external_label(uri, val, src, cid, cts DESC, id DESC);
```

That supports both record AT-URI subjects and account DID subjects because both are stored in `external_label.uri`.

If production query plans show regressions, consider later index changes based on measured plans only. Do not add speculative indexes in the initial slice.

## Label producer contract

For `authorLabels` to work cleanly, account-quality labelers should emit DID-subject labels:

```json
{
  "src": "did:plc:pswneepkd5lesumj7ejmkbal",
  "uri": "did:plc:author",
  "val": "likely-test",
  "neg": false
}
```

They should declare themselves as account labelers:

```json
{
  "$type": "app.bsky.labeler.service",
  "subjectTypes": ["account"],
  "policies": {
    "labelValues": ["likely-test", "standard", "high-quality"]
  }
}
```

Hyperindex should not require the declaration to be correct at query time; it should filter by stored label subjects. The declaration is still important metadata for ATProto clients and labeler correctness.

## Test plan

### Repository tests

Add SQLite-backed tests and ensure the same paths pass under PostgreSQL CI.

1. **`has` matches DID-subject labels**
   - Insert records by authors A, B, C.
   - Label author A's DID as `high-quality`.
   - `authorLabels.has.val.eq = high-quality` returns only A's records.

2. **`none` excludes matching authors but keeps unlabeled authors**
   - Label author B's DID as `likely-test`.
   - `authorLabels.none.val.eq = likely-test` returns A and C, not B.

3. **Record labels do not satisfy author labels**
   - Label record A's AT-URI as `high-quality`.
   - `authorLabels.has.val.eq = high-quality` does not match unless A's DID is also labeled.

4. **Author labels do not satisfy record labels**
   - Label author A's DID as `high-quality`.
   - `externalLabels.has.val.eq = high-quality` does not match A's records unless their record URIs are also labeled.

5. **Both filters combine with AND semantics**
   - Require a record label and exclude a likely-test author.
   - Confirm only records satisfying both remain.

6. **Active-label semantics**
   - Positive label then newer negation removes the match for `activeOnly: true`.
   - Expired labels do not match for `activeOnly: true`.
   - Historical labels can match with `activeOnly: false`.

7. **CID-specific account labels do not match author labels**
   - Insert a label with `uri = did` and non-null `cid`.
   - `authorLabels.has` should not match it.

8. **Pagination and count**
   - Build enough records to require multiple pages.
   - Confirm page sizes, cursors, and `totalCount` reflect pre-pagination filtering.

### GraphQL schema tests

- Introspect representative generated where inputs and assert `authorLabels` exists with type `ExternalLabelWhereInput`.
- Assert existing `externalLabels` still exists and has the same type.

### GraphQL resolver tests

- Query a typed connection with `where.authorLabels.has`.
- Query a typed connection with `where.authorLabels.none` and `totalCount`.
- Query with both `externalLabels` and `authorLabels`.
- Verify malformed operators still return helpful errors.

### Smoke / integration tests

Add a dedicated API smoke path for author labels instead of reusing record-subject external label expectations. The smoke suite should cover:

- a baseline typed collection query with no `authorLabels` filter,
- `authorLabels.has.val.eq` for `likely-test`, `standard`, and `high-quality`,
- `authorLabels.none.val.eq` for `likely-test`,
- `authorLabels.has.val.in` for `standard` plus `high-quality`,
- two-page pagination and duplicate-cursor checks for the multi-value `has` query,
- root `externalLabels(subjects: [did])` lookups proving returned author DIDs have or do not have the expected non-CID DID-subject labels.

API smoke should validate the indexed data served by the target API; it should not seed author labels. Isolated local Tap smoke expectations should omit `authorLabelActivityClaims` unless the target has real author-label data. For staging or production validation, run the same API smoke test with the live orglabeler source DID. The orglabeler service is hosted at `orglabeler.hypercerts.dev`; confirm the source DID from the service metadata before relying on environment-specific smoke expectations.

## Rollout notes

1. Ensure Hyperindex is subscribed to the orglabeler label stream in the target environment.
2. Confirm ingested labels contain DID subjects using the root query:

   ```graphql
   query LabelsForKnownAuthors($subjects: [String!]!, $source: String!) {
     externalLabels(subjects: $subjects, sources: [$source], activeOnly: true) {
       src
       uri
       val
       neg
       cts
     }
   }
   ```

3. Deploy `authorLabels` to staging.
4. Validate issue #92 workflows on staging:
   - public counters excluding likely-test authors,
   - default feed exclusion,
   - account-quality include/exclude filters.
5. Update public docs and local agent skill references when the field is available:
   - `.agents/skills/hyperindex/SKILL.md`
   - `.agents/skills/hyperindex/references/schema-reference.md`

## Failure modes and expected behavior

| Situation | Expected behavior |
| --- | --- |
| Author has no labels | `none` predicates pass; `has` predicates fail |
| Labeler source omitted | Labels from any source can match |
| Label subject is profile/org AT-URI | Matches `externalLabels` only for that profile/org record; does not match `authorLabels` |
| Label subject is author DID | Matches `authorLabels` for records authored by that DID |
| Author DID label has non-null CID | Does not match `authorLabels` |
| Label was negated or expired | Does not match when `activeOnly: true` |
| Both `externalLabels` and `authorLabels` are present | Record must satisfy both filters |

## Rationale

The clean conceptual split is:

```text
externalLabels = labels on this record
 authorLabels = labels on this record's author account
```

That keeps Hyperindex generic and ATProto-aligned. It avoids encoding Certified-specific account-carrier records into the indexer while giving certified-app a scalable server-side filter for feeds, counters, and explore policy.
