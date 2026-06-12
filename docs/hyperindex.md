# Hyperindex

Hyperindex is the hosted GraphQL read API for Hypercerts and Certified AT Protocol records. Use it when you want to build applications, profile pages, discovery views, dashboards, analytics, or label-driven curation over indexed Hypercerts data.

Hyperindex does not replace AT Protocol repositories. Records are created, updated, and deleted in users' repositories. Hyperindex indexes those records and exposes a queryable read model.

## Endpoints

| Environment | GraphQL endpoint | Playground |
| --- | --- | --- |
| Production | `https://api.indexer.hypercerts.dev/graphql` | `https://api.indexer.hypercerts.dev/graphiql` |
| Staging | `https://dev.api.indexer.hypercerts.dev/graphql` | `https://dev.api.indexer.hypercerts.dev/graphiql` |

Use production for normal application traffic. Use staging to test schema or indexing changes before they reach production.

Public GraphQL queries do not require an API key.

## What Hyperindex gives you

Hyperindex turns AT Protocol records into a GraphQL API with:

- typed queries for Hypercerts and Certified collections
- field-level selection, filtering, sorting, and pagination
- record metadata such as `uri`, `cid`, `did`, and `rkey`
- generic JSON access for lower-level workflows
- text search over indexed record JSON
- external label lookups and label-aware filtering
- convenience profile hydration through `certifiedProfileData`

## Architecture

At a high level, Hyperindex is a read-side indexer and GraphQL API.

```text
AT Protocol repositories
        │
        │ record events for indexed collections
        ▼
Tap / AT Protocol ingestion
        │
        │ normalized record writes
        ▼
Hyperindex record store
        │
        │ Lexicon-generated schema
        ▼
Public GraphQL API

External ATProto labelers
        │
        │ com.atproto.label.subscribeLabels streams
        ▼
Hyperindex external label store
        │
        ▼
GraphQL label fields and filters
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

Hyperindex builds its public GraphQL schema from AT Protocol Lexicons. A collection NSID becomes a typed GraphQL query:

| Collection NSID | List query | Single-record query |
| --- | --- | --- |
| `org.hypercerts.claim.activity` | `orgHypercertsClaimActivity` | `orgHypercertsClaimActivityByUri` |
| `org.hypercerts.collection` | `orgHypercertsCollection` | `orgHypercertsCollectionByUri` |
| `org.hypercerts.context.attachment` | `orgHypercertsContextAttachment` | `orgHypercertsContextAttachmentByUri` |
| `app.certified.actor.profile` | `appCertifiedActorProfile` | `appCertifiedActorProfileByUri` |
| `app.certified.link.evm` | `appCertifiedLinkEvm` | `appCertifiedLinkEvmByUri` |

Use typed queries first. They provide typed fields, filters, sorting, pagination, external labels, and profile hydration. Use the generic `records(collection: ...)` query when you need raw JSON or when a typed query is not available.

### Relationships between records

Hypercert records often reference other records with AT Protocol strong refs, usually `{ uri, cid }`. Hyperindex exposes those refs, but it does not automatically join every referenced record.

For example:

- a hypercert can reference a rights record
- an attachment can reference one or more subject records
- a collection can reference activity claims or other collections
- a badge award can reference a badge definition and a subject

For arbitrary references, read the referenced `uri` and fetch it with the matching `ByUri` query, or use `search` when you need to find records that mention a nested AT-URI.

The main built-in convenience join is `certifiedProfileData`, which resolves the author's `app.certified.actor.profile/self` record when it is indexed.

### External labels

External ATProto labels are stored separately from records. A label can target either:

- an account DID, such as `did:plc:...`
- a record AT-URI, such as `at://did:plc:.../org.hypercerts.claim.activity/...`

GraphQL exposes labels in three ways:

- `externalLabels(subjects: ...)` for direct subject lookups
- `node.externalLabels(...)` on indexed records
- `where.externalLabels.has` and `where.externalLabels.none` for label-aware typed queries

Labels are metadata over a subject. They do not become fields inside the original AT Protocol record.

### Consistency model

Hyperindex is eventually consistent with the AT Protocol network. Newly written records may take time to appear. Updates can change a record's `cid` while keeping the same `uri`. Consumers should design for this by:

- using pagination instead of assuming fixed result sets
- storing `uri` for stable record references
- storing `cid` when exact record versions matter
- retrying recent writes before treating missing records as permanent
- treating staging as unstable unless you have introspected its current schema

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
where: { did: { eq: "did:plc:example" } }
where: { title: { contains: "reforestation" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { image: { isNull: false } }
```

Scalar fields support value filters such as `eq`, `neq`, `in`, `contains`, `startsWith`, `gt`, `lt`, `gte`, `lte`, and `isNull`, depending on the scalar type.

Complex top-level fields such as arrays, refs, unions, blobs, and objects support presence checks only:

```graphql
where: {
  image: { isNull: false }
  contributors: { isNull: false }
}
```

Typed filters do not match nested values inside arrays, refs, unions, blobs, or objects. If you need nested matching, use `search(query: ..., collection: ...)`, fetch a narrower result set and filter client-side, or follow referenced `uri` values with `ByUri` queries.

## Quickstart

Send a POST request with a GraphQL query and optional variables.

```bash
curl -s https://api.indexer.hypercerts.dev/graphql \
  -H 'content-type: application/json' \
  --data '{"query":"query { collectionStats { collection count } }"}'
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
        certifiedProfileData {
          displayName
          website
        }
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

## Example: filter by an external label

```graphql
query LabeledHypercerts($labeler: String!, $value: String!, $after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    where: {
      externalLabels: {
        has: {
          src: { eq: $labeler }
          val: { eq: $value }
          activeOnly: true
        }
      }
    }
  ) {
    edges {
      cursor
      node {
        uri
        title
        externalLabels(sources: [$labeler], values: [$value], activeOnly: true) {
          src
          val
          cts
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{
  "labeler": "did:plc:antf7bsm6f4ohkqfdckefyt7",
  "value": "high-quality",
  "after": null
}
```

`activeOnly` defaults to `true`. Active labels exclude expired labels and labels whose latest state is a negation.

## Example: search record JSON

Use `search` for simple discovery or to find nested AT-URI references that typed filters cannot inspect.

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

## Best practices

- Use production unless you are deliberately testing staging changes.
- Prefer typed queries over generic JSON queries.
- Always paginate list queries.
- Keep selection sets small.
- Use `uri` for stable record identity.
- Use `uri` plus `cid` for version-sensitive data.
- Use `search` or client-side filtering for nested fields that typed filters cannot inspect.
- Repeat label constraints on `node.externalLabels(...)` when you only want to display the labels used by `where.externalLabels`.
- Request `totalCount` only when your UI needs it.

## Troubleshooting

### `Cannot query field ...`

The selected endpoint's schema does not expose that field. Check that you are using the right environment and inspect the schema in GraphiQL.

### A nested filter does not work

Typed filters support scalar comparisons and top-level presence checks. They do not inspect nested arrays, refs, unions, blobs, or objects. Use `search`, follow a referenced `uri`, or filter client-side.

### `certifiedProfileData` is `null`

The record author does not have an indexed Certified profile at `app.certified.actor.profile/self`, or the profile has not been indexed yet.

### Labels are missing

Check whether the label targets a DID or an AT-URI. Account labels use DID subjects; record labels use AT-URI subjects. Also check `activeOnly`: expired or negated labels are hidden unless you pass `activeOnly: false`.

### A recently written record is missing

Hyperindex is eventually consistent with AT Protocol repositories. Retry after a short delay and confirm the record belongs to an indexed collection.
