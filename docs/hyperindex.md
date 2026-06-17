# Hyperindex

Hyperindex is the hosted GraphQL read API for Hypercerts and Certified AT Protocol records. Use it when you want to build applications, profile pages, discovery views, dashboards, analytics, or curated experiences over indexed Hypercerts data.

Hyperindex does not replace AT Protocol repositories. Records are created, updated, and deleted in users' repositories. Hyperindex indexes those records and exposes a queryable read model.

## Endpoints

| Environment | GraphQL endpoint | Playground |
| --- | --- | --- |
| Production | `https://api.indexer.hypercerts.dev/graphql` | `https://api.indexer.hypercerts.dev/graphiql` |
| Staging | `https://dev.api.indexer.hypercerts.dev/graphql` | `https://dev.api.indexer.hypercerts.dev/graphiql` |

Use production for normal application traffic. Production and staging both index data from the same AT Protocol network. Staging is mainly for earlier Hyperindex API, schema, and indexing features; it may expose new functionality before production does.

Public GraphQL queries do not require an API key.

## What Hyperindex gives you

Hyperindex turns AT Protocol records into a GraphQL API with:

- typed queries for Hypercerts and Certified collections
- field-level selection, filtering, sorting, and pagination
- record metadata such as `uri`, `cid`, `did`, and `rkey`
- generic JSON access for lower-level workflows
- text search over indexed record JSON

## Architecture

At a high level, Hyperindex is a read-side indexer and GraphQL API.

```text
AT Protocol repositories
        │
        │ repo commits and record events
        ▼
AT Protocol Relay
        │
        │ network event stream
        ▼
Tap / AT Protocol ingestion
        │
        │ normalized records for indexed collections
        ▼
Hyperindex record store
        │
        │ dynamically generated GraphQL schema
        ▼
Public GraphQL API
```

### Source of truth

The source of truth is the AT Protocol repository that owns a record. Hyperindex stores an indexed copy so consumers can query across records efficiently.

Use these identifiers carefully:

| Field | Meaning | When to use it |
| --- | --- | --- |
| `uri` | The AT-URI of a record, usually `at://<did>/<collection>/<rkey>` | Stable record identity and links between records |
| `cid` | The CID of the indexed record version | Version-sensitive reads and cache validation |
| `did` | The DID of the account that owns the record | Author/account filtering |
| `rkey` | The record key, the last segment of the AT-URI | Low-level AT Protocol workflows |
| `createdAt` | Timestamp declared inside the record | User-facing chronology |
| `indexed_at` | Hyperindex arrival order | Indexer-facing chronology |

If you only need to link to a logical record, store the `uri`. If your application cares about the exact content version, store both `uri` and `cid`.

### Schema generation

Hyperindex dynamically builds its public GraphQL schema from AT Protocol Lexicons. A collection NSID becomes a typed GraphQL query:

| Collection NSID | List query | Single-record query |
| --- | --- | --- |
| `org.hypercerts.claim.activity` | `orgHypercertsClaimActivity` | `orgHypercertsClaimActivityByUri` |
| `org.hypercerts.collection` | `orgHypercertsCollection` | `orgHypercertsCollectionByUri` |
| `org.hypercerts.context.attachment` | `orgHypercertsContextAttachment` | `orgHypercertsContextAttachmentByUri` |
| `app.certified.actor.profile` | `appCertifiedActorProfile` | `appCertifiedActorProfileByUri` |
| `app.certified.link.evm` | `appCertifiedLinkEvm` | `appCertifiedLinkEvmByUri` |

Use typed queries first. They provide typed fields, filters, sorting, and pagination. Use the generic `records(collection: ...)` query when you need raw JSON or when a typed query is not available.

### Relationships between records

Hypercert records often reference other records with AT Protocol strong refs, usually `{ uri, cid }`. Hyperindex exposes those refs, but it does not automatically join every referenced record.

For example:

- a hypercert can reference a rights record
- an attachment can reference one or more subject records
- a collection can reference activity claims or other collections
- a badge award can reference a badge definition and a subject

For arbitrary references, read the referenced `uri` and fetch it with the matching `ByUri` query, or use `search` when you need to find records that mention a nested AT-URI.

### Consistency model

Hyperindex is eventually consistent with the AT Protocol network. Indexing is generally fast, but relay or Tap hiccups can occasionally make indexing slower, so consumers should design for this by:

- using pagination instead of assuming fixed result sets
- storing `uri` for stable record references
- storing `cid` when exact record versions matter
- retrying recent writes before treating missing records as permanent

## Core collections

The hosted Hypercerts indexer exposes typed queries for these main collections.

| Concept | Collection | Query |
| --- | --- | --- |
| Hypercert activity claims | `org.hypercerts.claim.activity` | `orgHypercertsClaimActivity` |
| Contribution details | `org.hypercerts.claim.contribution` | `orgHypercertsClaimContribution` |
| Contributor information | `org.hypercerts.claim.contributorInformation` | `orgHypercertsClaimContributorInformation` |
| Rights and transfer terms | `org.hypercerts.claim.rights` | `orgHypercertsClaimRights` |
| Collections of hypercerts | `org.hypercerts.collection` | `orgHypercertsCollection` |
| Acknowledgements | `org.hypercerts.context.acknowledgement` | `orgHypercertsContextAcknowledgement` |
| Attachments and evidence | `org.hypercerts.context.attachment` | `orgHypercertsContextAttachment` |
| Evaluations | `org.hypercerts.context.evaluation` | `orgHypercertsContextEvaluation` |
| Measurements | `org.hypercerts.context.measurement` | `orgHypercertsContextMeasurement` |
| Funding receipts | `org.hypercerts.funding.receipt` | `orgHypercertsFundingReceipt` |
| Work-scope tags | `org.hypercerts.workscope.tag` | `orgHypercertsWorkscopeTag` |
| Certified profiles | `app.certified.actor.profile` | `appCertifiedActorProfile` |
| Certified organizations | `app.certified.actor.organization` | `appCertifiedActorOrganization` |
| Certified follows | `app.certified.graph.follow` | `appCertifiedGraphFollow` |
| EVM wallet links | `app.certified.link.evm` | `appCertifiedLinkEvm` |
| Locations | `app.certified.location` | `appCertifiedLocation` |
| Badge definitions | `app.certified.badge.definition` | `appCertifiedBadgeDefinition` |
| Badge awards | `app.certified.badge.award` | `appCertifiedBadgeAward` |
| Badge responses | `app.certified.badge.response` | `appCertifiedBadgeResponse` |

## Query model

Most typed list queries accept:

- `first` and `after` for forward pagination
- `last` and `before` for backward pagination
- `where` for filtering
- `sortBy` and `sortDirection` for ordering

Pagination defaults to 20 records. The maximum page size is 1000 records.

Common filters:

```graphql
where: { uri: { eq: "at://did:plc:example/org.hypercerts.claim.activity/rkey" } }
where: { uri: { in: ["at://did:plc:example/org.hypercerts.claim.activity/rkey1", "at://did:plc:example/org.hypercerts.claim.activity/rkey2"] } }
where: { did: { eq: "did:plc:example" } }
where: { title: { contains: "reforestation" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { image: { isNull: false } }
```

The generated `uri` filter is a record metadata filter for exact AT-URI lookup and batched hydration. It supports `eq` and `in` and does not search the JSON payload.

Scalar fields support value filters such as `eq`, `neq`, `in`, `contains`, `startsWith`, `gt`, `lt`, `gte`, `lte`, and `isNull`, depending on the scalar type.

Complex fields support presence checks with `isNull`. Some complex fields use the shared `PresenceFilterInput`; arrays, refs, and unions may instead expose generated nested filter inputs that also include `isNull`. Do not rely on the input type name for presence checks; introspect the field and use `isNull`. Nested scalar leaves support exact operators only: `eq`, `in`, and `isNull`. Use array `any` when at least one array item should match; multiple predicates inside the same `any` must match the same array item.

```graphql
where: {
  image: { isNull: false }
  contributors: {
    any: {
      contributorIdentity: { identity: { eq: "did:plc:example" } }
    }
  }
}
```

Nested filters do not support substring operators (`contains`, `startsWith`), comparison operators (`gt`, `lt`, `gte`, `lte`), nested sorting, arbitrary JSON paths, or automatic strong-ref dereferencing. A small set of explicit collection filter extensions may perform product-specific cross-record lookups; uploaded lexicons do not get these fields automatically.

For Hypercerts activity contributors that may be inline, legacy bare DID strings, or `org.hypercerts.claim.contributorInformation` strong refs, use the compatibility filter:

```graphql
where: { contributorDid: { eq: "did:plc:example" } }
```

For Certified badge awards, use `badgeType` to filter by the referenced `app.certified.badge.definition.badgeType` without joining badge definitions client-side:

```graphql
where: { badgeType: { eq: "endorsement" } }
```

`badgeType` uses `StringFilterInput`, so it supports the same string operators exposed for badge definitions. Awards whose referenced badge definition is missing or has no `badgeType` do not match positive value filters.

## Quickstart

Send a POST request with a GraphQL query and optional variables.

```bash
curl -s https://api.indexer.hypercerts.dev/graphql \
  -H 'content-type: application/json' \
  --data '{"query":"query { orgHypercertsClaimActivity(first: 1) { edges { node { uri title } } } }"}'
```

A minimal TypeScript helper:

```ts
const endpoint = "https://api.indexer.hypercerts.dev/graphql";

export async function queryHyperindex<T>(
  query: string,
  variables?: Record<string, unknown>,
): Promise<T> {
  const response = await fetch(endpoint, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ query, variables }),
  });

  if (!response.ok) {
    throw new Error(`Hyperindex request failed: ${response.status} ${response.statusText}`);
  }

  const payload = await response.json();

  if (payload.errors?.length) {
    throw new Error(payload.errors.map((error: { message: string }) => error.message).join("\n"));
  }

  return payload.data as T;
}
```

## Example: query recent hypercerts

```graphql
query RecentHypercerts($after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
  ) {
    edges {
      cursor
      node {
        uri
        cid
        did
        title
        shortDescription
        createdAt
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{ "after": null }
```

## Example: fetch a hypercert by AT-URI

```graphql
query HypercertByUri($uri: String!) {
  orgHypercertsClaimActivityByUri(uri: $uri) {
    uri
    cid
    did
    title
    shortDescription
    createdAt
    startDate
    endDate
    rights { uri cid }
    image {
      __typename
      ... on OrgHypercertsDefsUri { uri }
      ... on OrgHypercertsDefsSmallImage {
        image { ref mimeType size }
      }
    }
  }
}
```

Variables:

```json
{
  "uri": "at://did:plc:example/org.hypercerts.claim.activity/example-rkey"
}
```

## Example: search record JSON

Use `search` for simple discovery, substring matching, or nested AT-URI references that are not covered by generated exact nested filters.

```graphql
query SearchAttachments($hypercertUri: String!, $after: String) {
  search(
    query: $hypercertUri
    collection: "org.hypercerts.context.attachment"
    first: 20
    after: $after
  ) {
    edges {
      cursor
      node {
        uri
        cid
        did
        collection
        value
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{
  "hypercertUri": "at://did:plc:example/org.hypercerts.claim.activity/example-rkey",
  "after": null
}
```

## Working with unions, refs, and blobs

For unions, request `__typename` and use inline fragments:

```graphql
image {
  __typename
  ... on OrgHypercertsDefsUri { uri }
  ... on OrgHypercertsDefsSmallImage {
    image { ref mimeType size }
  }
}
```

For strong references, request both `uri` and `cid`:

```graphql
rights { uri cid }
```

For blobs, request the blob reference and metadata:

```graphql
image { ref mimeType size }
```

## Service health endpoints

In addition to GraphQL, hosted Hyperindex exposes lightweight status endpoints:

| Endpoint | Meaning |
| --- | --- |
| `/health` | Liveness check. Use this to check whether the process is running. |
| `/ready` | Readiness check. Use this to check whether the API is ready to serve traffic. |
| `/stats` | Public operational stats and diagnostics for the indexer. |

## Best practices

- Prefer typed queries over generic JSON queries.
- Always paginate list queries.
- Keep selection sets small.
- Use `uri` for stable record identity.
- Use `uri` plus `cid` for version-sensitive data.
- Use generated nested filters for exact nested matches when available; use `search` or client-side filtering for substring matching or unsupported nested shapes.
- Request `totalCount` only when your UI needs it.

## Troubleshooting

### `Cannot query field ...`

The selected endpoint's schema does not expose that field. Check that you are using the right environment and inspect the schema in GraphiQL.

### A nested filter does not work

Generated nested filters only cover arrays, refs, and unions up to three lexicon path segments deep, and nested scalar leaves only support `eq`, `in`, and `isNull`. Multiple predicates inside the same array `any` are evaluated against the same array item. They do not support substring operators (`contains`, `startsWith`), comparison operators (`gt`, `lt`, `gte`, `lte`), arbitrary JSON paths, nested sorting, or automatic strong-ref dereferencing. Introspect the target endpoint's `WhereInput`; if the nested input is absent, use `search`, follow a referenced `uri`, or filter client-side.

### A recently written record is missing

Hyperindex is generally fast, but it is still eventually consistent with AT Protocol repositories. Retry after a short delay and confirm the record belongs to an indexed collection.
