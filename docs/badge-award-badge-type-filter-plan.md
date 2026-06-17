# Badge award `badgeType` filter plan

## Goal

Add a collection-specific GraphQL where filter to `app.certified.badge.award` so clients can query badge awards by the referenced badge definition's `badgeType` without doing a client-side join.

Target public API shape:

```graphql
appCertifiedBadgeAward(
  where: { badgeType: { eq: "endorsement" } }
) {
  edges {
    node { uri }
  }
}
```

`badgeType` should use `StringFilterInput`, matching `AppCertifiedBadgeDefinitionWhereInput.badgeType`.

## Semantics

For each badge award record:

1. Read `award.json.badge.uri`.
2. Find a record where:
   - `record.uri = award.json.badge.uri`
   - `record.collection = "app.certified.badge.definition"`
3. Apply the requested string filter to `badge_definition.json.badgeType`.

This is a query-time derived filter. It does not hydrate the referenced badge definition into the award response, and it does not denormalize `badgeType` into award records.

## Boundary

Keep the generic lexicon filter system separate:

- Generated lexicon filters stay same-record only.
- Generated nested filters must not dereference arbitrary strong refs.
- Cross-record filters are explicit collection filter extensions, keyed by exact collection NSID and field name.
- Uploaded or unrelated lexicons must not inherit these fields accidentally.

## Implementation plan

### 1. Add collection filter extension registry

Create a small schema-layer helper, likely:

```text
internal/graphql/schema/collection_filter_extensions.go
```

The registry should expose descriptors keyed by exact collection NSID and filter field name, for example:

```go
org.hypercerts.claim.activity -> contributorDid
app.certified.badge.award -> badgeType
```

Each descriptor should include:

- collection NSID
- field name
- GraphQL input object
- description
- repository filter target
- extraction/validation rules if needed

Prefer the name `collection filter extensions` over generic `derived filters` because these are explicit product extensions, not automatic lexicon behavior.

### 2. Move existing `contributorDid`

Move the existing inline `contributorDid` schema registration/extraction out of `builder.go` into the collection filter extension registry.

Current behavior should remain unchanged:

```graphql
orgHypercertsClaimActivity(
  where: { contributorDid: { eq: "did:plc:..." } }
)
```

It should still route to:

```go
repositories.FieldFilterTargetContributorDID
```

### 3. Add badge award `badgeType`

Add this extension only for:

```text
app.certified.badge.award
```

GraphQL field:

```graphql
badgeType: StringFilterInput
```

Repository target:

```go
FieldFilterTargetBadgeAwardBadgeType
```

Support the same operators exposed by `StringFilterInput`, including the normal string operators such as `eq`, `neq`, `in`, `contains`, `startsWith`, and `isNull` if present in the current input definition.

### 4. Collision protection

If a lexicon-defined property already has the same name as a collection filter extension, do not silently override it.

Preferred behavior: fail schema build with a clear error explaining:

- collection NSID
- extension field name
- that the lexicon already defines that property
- what to do next

This protects future lexicon changes such as `app.certified.badge.award` eventually adding a real `badgeType` field.

### 5. Repository SQL

Add an explicit repository condition builder for badge award badge type.

Logical SQL shape:

```sql
EXISTS (
  SELECT 1
  FROM record badge_definition
  WHERE badge_definition.uri = award.json->'badge'->>'uri'
    AND badge_definition.collection = 'app.certified.badge.definition'
    AND <StringFilterInput predicate against badge_definition.json->>'badgeType'>
)
```

Keep this explicit. Do not add generic strongRef dereferencing.

Missing referenced badge definition records should simply not match `badgeType` filters, except for any explicit `isNull` semantics we choose and document.

## Files likely to change

- `internal/graphql/schema/builder.go`
- `internal/graphql/schema/collection_filter_extensions.go`
- `internal/database/repositories/records.go`
- `internal/graphql/schema/builder_test.go`
- `internal/database/repositories/records_filter_test.go`
- `tests/api-smoke/*` if public smoke coverage is added
- `docs/hyperindex.md`
- `.agents/skills/hyperindex/SKILL.md`
- `.agents/skills/hyperindex/references/schema-reference.md`
- `.changes/unreleased/*.yaml`

## Required tests

### Schema tests

- `AppCertifiedBadgeAwardWhereInput` exposes `badgeType: StringFilterInput`.
- `AppCertifiedBadgeDefinitionWhereInput.badgeType` remains unchanged.
- `contributorDid` still appears on `OrgHypercertsClaimActivityWhereInput` after moving it into the extension registry.
- Collision behavior is tested with a synthetic lexicon that defines a property with the same name as an extension.

### Repository tests

- Award with referenced badge definition `badgeType = endorsement` matches `badgeType.eq`.
- Award whose referenced definition has another type does not match.
- Award with missing referenced definition does not match positive `badgeType` filters.
- Supported string operators work consistently with `StringFilterInput`.
- Existing `contributorDid` tests still pass.

### API smoke tests

If staging/test data has stable badge awards and definitions:

- discover a badge definition with `badgeType`
- query awards by `badge.uri`
- query awards by `badgeType`
- assert the expected award appears

If data is not stable, prefer schema smoke plus a guarded/skippable positive runtime smoke.

## Documentation notes

Document this distinction clearly:

```text
Generated nested filters are same-record filters. They do not dereference strong refs.
Collection filter extensions are explicit allowlisted product filters and may perform cross-record lookups.
```

Document `badgeType` under badge award filters as a derived/cross-record filter from `badge.uri` to `app.certified.badge.definition.badgeType`.

## Non-goals

- No generic ref-following filters.
- No arbitrary cross-record joins from GraphQL input.
- No ingestion-time denormalization unless query-time performance later proves insufficient.
- No changes to uploaded lexicon behavior beyond exact allowlisted collection filter extensions.
