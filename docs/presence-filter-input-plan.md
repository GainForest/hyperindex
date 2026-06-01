# Presence Filter Input Implementation Plan

## Goal

Add basic presence filtering for non-scalar top-level lexicon fields in generated collection queries.

Target GraphQL shape:

```graphql
query {
  orgHypercertsClaimActivity(
    where: {
      image: { isNull: false }
      contributors: { isNull: false }
    }
  ) {
    edges {
      node {
        uri
        title
        shortDescription
      }
    }
  }
}
```

This lets consumers ask whether complex fields such as `image`, `description`, `contributors`, `locations`, or `rights` are present without pretending Hyperindex supports deep object, union, or array filtering.

The example intentionally selects scalar fields only. Existing GraphQL object/union selection rules still apply when clients select the complex fields themselves.

## Scope

This slice will:

- Add a reusable GraphQL input type for presence checks:

  ```graphql
  input PresenceFilterInput {
    isNull: Boolean!
  }
  ```

- Generate `PresenceFilterInput` fields for non-scalar top-level lexicon properties, including `cid-link`.
- Keep existing scalar filters unchanged.
- Keep `did` as a column-level filter.
- Keep `externalLabels` as its existing record-level filter.
- Reuse the existing `FieldFilter{Operator: "isNull"}` SQL path where possible.
- Preserve restart/startup-time GraphQL schema generation.

This slice will not:

- Add nested `and` / `or` filtering.
- Add deep JSON path filters such as `image.type.eq` or `contributors.identity.eq`.
- Treat arrays, refs, objects, or unions as string-searchable values.
- Rebuild the public GraphQL schema per request.
- Add `actorHandle` filters or actor joins.
- Change generic `records(collection: ...)` query behavior.

## Desired filterability rules

Given a top-level record property in a collection lexicon:

| Lexicon type | GraphQL filter input | Operators |
| --- | --- | --- |
| `string` | `StringFilterInput` or `DateTimeFilterInput` when `format: "datetime"` | existing scalar operators |
| `integer` | `IntFilterInput` | existing scalar operators |
| `number` | `FloatFilterInput` | existing scalar operators |
| `boolean` | `BooleanFilterInput` | existing scalar operators |
| `array` | `PresenceFilterInput` | `isNull` only |
| `ref` | `PresenceFilterInput` | `isNull` only |
| `union` | `PresenceFilterInput` | `isNull` only |
| `object` | `PresenceFilterInput` | `isNull` only |
| `blob` | `PresenceFilterInput` | `isNull` only |
| `bytes` | `PresenceFilterInput` | `isNull` only |
| `unknown` | `PresenceFilterInput` | `isNull` only |
| `cid-link` | `PresenceFilterInput` | `isNull` only |
| reserved metadata field names | skipped, except existing explicit metadata filters | n/a |

For `org.hypercerts.claim.activity`, expected `where` fields after this change:

```graphql
input OrgHypercertsClaimActivityWhereInput {
  did: DIDFilterInput
  externalLabels: ExternalLabelWhereInput

  title: StringFilterInput
  shortDescription: StringFilterInput
  startDate: DateTimeFilterInput
  endDate: DateTimeFilterInput
  createdAt: DateTimeFilterInput

  shortDescriptionFacets: PresenceFilterInput
  description: PresenceFilterInput
  image: PresenceFilterInput
  contributors: PresenceFilterInput
  workScope: PresenceFilterInput
  locations: PresenceFilterInput
  rights: PresenceFilterInput
}
```

Nested definitions such as `contributor.identity` or `workScopeString.scope` remain unavailable as filters in this slice.

## Null semantics

Define presence consistently across SQLite and PostgreSQL:

- `isNull: true` means the top-level JSON field is missing or explicitly JSON `null`.
- `isNull: false` means the top-level JSON field exists and is not JSON `null`.
- Empty array `[]` counts as present / not null.
- Empty object `{}` counts as present / not null.
- Empty string `""` counts as present / not null.

Existing SQLite behavior with `json_extract(json, '$.field') IS NULL` already treats missing and JSON null as null.

For PostgreSQL, keep behavior aligned with current text extraction semantics where possible:

```sql
json->>'field' IS NULL
json->>'field' IS NOT NULL
```

Do not switch only presence filters to `json->'field' IS NOT NULL` unless tests confirm the intended JSON-null semantics, because JSON `null` and missing keys behave differently depending on `->` vs `->>`.

## Architecture direction

The long-term architecture should mirror Quickslice's separation pattern:

```text
GraphQL where input builder
  -> parsed intermediate filter representation
  -> SQL where builder
  -> RecordsRepository assembles collection/cursor/sort/count query
```

However, this slice should stay incremental. Presence-only filtering can be added with the current Hyperindex structure first:

```text
buildWhereInputTypes()
  -> PresenceFilterInput for complex fields
  -> extractFiltersWithExternalLabels()
  -> FieldFilter{Operator: "isNull"}
  -> existing buildFilterClause()
```

After this lands and tests cover the behavior, a follow-up refactor can extract `buildFilterClause`, DID filter SQL, and external-label filter SQL into a dedicated query-filter builder package.

## Implementation steps

### 1. Add `PresenceFilterInput`

File:

- `internal/graphql/types/filters.go`

Add:

```go
// PresenceFilterInput is a GraphQL InputObject for checking whether a field is missing/null or present.
var PresenceFilterInput = graphql.NewInputObject(graphql.InputObjectConfig{
    Name:        "PresenceFilterInput",
    Description: "Filter conditions for checking whether a top-level JSON field is missing/null or present.",
    Fields: graphql.InputObjectConfigFieldMap{
        "isNull": &graphql.InputObjectFieldConfig{
            Type:        graphql.NewNonNull(graphql.Boolean),
            Description: "True matches missing or null fields; false matches present and non-null fields.",
        },
    },
})
```

### 2. Add a filter-input selector for all top-level fields

File:

- `internal/graphql/types/filters.go`

Keep the existing scalar-only helper unchanged:

```go
FilterInputForLexiconType(lexiconType, format string) *graphql.InputObject
```

Add a new property-level helper and use it only from where-input generation:

```go
FilterInputForLexiconProperty(lexiconType, format string) *graphql.InputObject
```

Suggested behavior:

```go
switch lexiconType {
case "string":
    if format == "datetime" { return DateTimeFilterInput }
    return StringFilterInput
case "integer":
    return IntFilterInput
case "number":
    return FloatFilterInput
case "boolean":
    return BooleanFilterInput
case "array", "ref", "union", "object", "blob", "bytes", "unknown", "cid-link":
    return PresenceFilterInput
case "record":
    return nil
}
```

This keeps the old helper contract and tests meaningful while making the new where-input behavior explicit.

### 3. Generate presence fields in `WhereInput`

File:

- `internal/graphql/schema/builder.go`

Update `buildWhereInputTypes()` to use `FilterInputForLexiconProperty()`.

Keep existing skips:

- skip property named `did`, because `did` is a metadata column filter.
- skip `types.ReservedRecordFields` collisions.

Expected result: complex top-level fields appear in per-collection `WhereInput`, but with only `isNull`. Generated field descriptions should make the limitation clear, for example: `Filter by whether image is missing/null or present; nested values are not filterable.`

### 4. Keep operator validation scoped

Files:

- `internal/graphql/schema/builder.go`
- `internal/database/repositories/records.go`

`extractFiltersWithExternalLabels()` currently loops over whatever operators are present in the GraphQL argument map.

Do not add a full scalar-versus-presence operator matrix in the extractor for this slice. Normal requests are constrained by the generated GraphQL input types, and schema tests should prove `PresenceFilterInput` only exposes `isNull`.

If it stays small, add repository-level defensive handling instead:

- return an error for unsupported `FieldFilter.Operator` values in `buildFilterClause()` instead of silently ignoring them.
- return an error when `isNull` receives a non-boolean internal value.

Skip this hardening if it causes broad plumbing changes.

### 5. Reuse existing SQL path

File:

- `internal/database/repositories/records.go`

Existing `buildFilterClause()` already supports:

```go
case "isNull":
    if isNull { extract IS NULL } else { extract IS NOT NULL }
```

No SQL expression change should be needed if PostgreSQL `JSONExtract` remains `json->>'field'` and SQLite remains `json_extract(json, '$.field')`.

Do not switch presence filters to PostgreSQL `json->'field'` or key-existence operators in this slice; that would change the intended JSON-null semantics.

Add focused semantic tests to confirm this for complex field names and both count/query repository paths.

### 6. Update docs

Update the public GraphQL guide:

- `client/src/app/docs/agents/route.ts`

Document that:

- scalar fields support typed scalar operators.
- complex top-level fields expose only `PresenceFilterInput { isNull }`.
- `isNull: true` matches missing and explicit JSON `null`.
- empty arrays, empty objects, and empty strings count as present.
- presence filtering is top-level only.
- generic `records(collection: ...)` is unchanged and does not get `where` filters.

Example:

```graphql
where: {
  image: { isNull: false }
  contributors: { isNull: false }
}
```

Add an introspection snippet that uses `inputFields`, for example:

```graphql
{
  __type(name: "OrgHypercertsClaimActivityWhereInput") {
    inputFields {
      name
      type { name kind ofType { name kind } }
    }
  }
}
```

## Tests

### Unit tests: filter input shape and mapping

File:

- `internal/graphql/types/filters_test.go`

Add a direct `PresenceFilterInput` shape/name test:

- name is `PresenceFilterInput`.
- exactly one field exists: `isNull`.
- `isNull` is `Boolean!`.

Keep `FilterInputForLexiconType()` scalar-only and add tests for the new `FilterInputForLexiconProperty()`:

- `array` -> `PresenceFilterInput`
- `ref` -> `PresenceFilterInput`
- `union` -> `PresenceFilterInput`
- `object` -> `PresenceFilterInput`
- `blob` -> `PresenceFilterInput`
- `bytes` -> `PresenceFilterInput`
- `unknown` -> `PresenceFilterInput`
- `cid-link` -> `PresenceFilterInput`
- `record` -> nil, if preserving record as non-filterable.

### Schema builder tests

File:

- `internal/graphql/schema/builder_test.go`

Add or extend a lexicon with mixed field types and assert the generated where input includes:

- scalar field with scalar input.
- datetime field with `DateTimeFilterInput`.
- representative complex fields with `PresenceFilterInput`.
- `cid-link` with `PresenceFilterInput`.
- `record` absent if preserving record as non-filterable.
- no reserved metadata collisions.

Mapping tests should cover every complex type; schema builder tests only need enough representative fields to prove the builder uses the new property-level selector.

### Integration tests

File:

- `internal/integration/graphql_filter_test.go`

Add records where complex fields are:

- missing
- explicitly `null`
- present as object, including `{}`.
- present as array, including `[]`.

Queries:

```graphql
where: { image: { isNull: false } }
where: { image: { isNull: true } }
where: { contributors: { isNull: false } }
where: { contributors: { isNull: true } }
```

Expected behavior:

- `isNull: false` returns object/array values, including `{}` and `[]`.
- `isNull: true` returns missing and explicit null values.

### Repository semantic tests

File:

- `internal/database/repositories/records_filter_test.go` or a small new repository test file if cleaner.

Add a data-level test for `FieldFilter{Field: "image", Operator: "isNull", Value: true/false, FieldType: "object"}` and an array field. The test should insert records where the target field is:

- missing.
- explicit JSON `null`.
- object / `{}`.
- array / `[]`.
- empty string, for the documented present semantics.

Assert both:

- `GetByCollectionSortedWithKeysetCursor...` or the equivalent query path used by GraphQL.
- `GetCollectionCountFiltered...`.

Run this semantic test against SQLite and PostgreSQL when a PostgreSQL `DATABASE_URL` is available. Keep the helper local and small; do not build a broad test harness just for this slice.

Also add low-cost executor safety tests for invalid JSON field names in both SQLite and PostgreSQL executors if those tests are not already sufficient.

## Verification

For Go-only changes:

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

Because this feature promises cross-dialect JSON-null behavior, also run PostgreSQL-backed tests when available:

```bash
DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable go test -v -race ./...
```

If no local PostgreSQL test database is available, document that only SQLite verification was run and that PostgreSQL remains to be validated in CI or a prepared local database.

If docs or client code changes:

```bash
npm --prefix client run lint
npm --prefix client run test
npm --prefix client run build
```

## Release note

This is externally meaningful for GraphQL API consumers. Add a Changie fragment unless the final change is plan/docs-only.

Suggested fragment:

```yaml
kind: added
body: Add presence-only GraphQL filters for complex top-level lexicon fields.
custom:
  Affects: user
```

## Resolved decisions and deferrals

For the first implementation:

- Name it `PresenceFilterInput`.
- Make `isNull` `Boolean!` so `{ image: {} }` and `{ image: { isNull: null } }` do not silently become no-ops.
- Apply it to all non-scalar, non-record, non-reserved top-level lexicon properties, including `blob`, `bytes`, `unknown`, and `cid-link`.
- Keep `record` non-filterable.
- Keep SQL behavior using the existing `isNull` implementation and current `JSONExtract` helpers.
- Do not add nested `and/or` yet.
- Do not add deep JSON path filtering yet.
- Do not extract the SQL/query-filter builder in this slice.
- Do not add broad defensive operator validation in `extractFiltersWithExternalLabels()`; rely on GraphQL validation plus `PresenceFilterInput` schema tests.
- If simple, harden `buildFilterClause()` so unsupported internal operators and non-boolean `isNull` values return clear errors instead of being ignored.
