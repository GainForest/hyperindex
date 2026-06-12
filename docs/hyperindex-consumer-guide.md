# Hyperindex consumer guide

Hyperindex is the hosted GraphQL API for reading Hypercerts and Certified AT Protocol records. Use it to build applications, profile pages, search experiences, dashboards, analytics, and label-driven discovery over indexed Hypercerts data.

Hyperindex is a read API. Records are created and updated in AT Protocol repositories; Hyperindex indexes those records and exposes them through GraphQL.

## Endpoints

| Environment | GraphQL endpoint | Playground |
| --- | --- | --- |
| Production | `https://api.indexer.hypercerts.dev/graphql` | `https://api.indexer.hypercerts.dev/graphiql` |
| Staging | `https://dev.api.indexer.hypercerts.dev/graphql` | `https://dev.api.indexer.hypercerts.dev/graphiql` |

Use production for normal application traffic. Use staging only when you are testing schema or indexing changes before they reach production.

Public GraphQL queries do not require an API key.

## How Hyperindex data is organized

Hyperindex indexes AT Protocol records by collection NSID. Each collection gets:

- a typed list query, such as `orgHypercertsClaimActivity`
- a single-record query by AT-URI, such as `orgHypercertsClaimActivityByUri`
- standard metadata fields: `uri`, `cid`, `did`, and `rkey`
- Relay-style pagination: `edges`, `cursor`, and `pageInfo`

Important identifiers:

| Field | Meaning |
| --- | --- |
| `uri` | The AT-URI of the record, usually `at://<did>/<collection>/<rkey>` |
| `cid` | The CID of the indexed record version |
| `did` | The DID of the account that owns the record |
| `rkey` | The record key, which is the last segment of the AT-URI |
| `createdAt` | Timestamp declared inside the record |
| `indexed_at` | Hyperindex arrival order, available as a sort field |

## Quickstart

Send a POST request with a GraphQL query and optional variables.

```bash
curl -s https://api.indexer.hypercerts.dev/graphql \
  -H 'content-type: application/json' \
  --data '{"query":"query { collectionStats { collection count } }"}'
```

A minimal JavaScript helper:

```ts
const endpoint = "https://api.indexer.hypercerts.dev/graphql";

export async function hyperindex<T>(query: string, variables?: Record<string, unknown>) {
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

## Indexed collections

The hosted Hypercerts indexer currently exposes typed GraphQL queries for these collections.

| What you want | Collection | List query |
| --- | --- | --- |
| Hypercert activity claims | `org.hypercerts.claim.activity` | `orgHypercertsClaimActivity` |
| Contribution details | `org.hypercerts.claim.contribution` | `orgHypercertsClaimContribution` |
| Contributor profiles | `org.hypercerts.claim.contributorInformation` | `orgHypercertsClaimContributorInformation` |
| Rights and transfer terms | `org.hypercerts.claim.rights` | `orgHypercertsClaimRights` |
| Hypercert collections | `org.hypercerts.collection` | `orgHypercertsCollection` |
| Acknowledgements | `org.hypercerts.context.acknowledgement` | `orgHypercertsContextAcknowledgement` |
| Attachments and evidence | `org.hypercerts.context.attachment` | `orgHypercertsContextAttachment` |
| Evaluations | `org.hypercerts.context.evaluation` | `orgHypercertsContextEvaluation` |
| Measurements | `org.hypercerts.context.measurement` | `orgHypercertsContextMeasurement` |
| Funding receipts | `org.hypercerts.funding.receipt` | `orgHypercertsFundingReceipt` |
| Work-scope tags | `org.hypercerts.workscope.tag` | `orgHypercertsWorkscopeTag` |
| Certified account profiles | `app.certified.actor.profile` | `appCertifiedActorProfile` |
| Certified organization metadata | `app.certified.actor.organization` | `appCertifiedActorOrganization` |
| Certified follows | `app.certified.graph.follow` | `appCertifiedGraphFollow` |
| EVM wallet links | `app.certified.link.evm` | `appCertifiedLinkEvm` |
| Locations | `app.certified.location` | `appCertifiedLocation` |
| Badge definitions | `app.certified.badge.definition` | `appCertifiedBadgeDefinition` |
| Badge awards | `app.certified.badge.award` | `appCertifiedBadgeAward` |
| Badge responses | `app.certified.badge.response` | `appCertifiedBadgeResponse` |

For each list query, the single-record query appends `ByUri`. For example, `orgHypercertsClaimActivityByUri(uri: ...)` returns one `org.hypercerts.claim.activity` record by AT-URI.

## Query recent hypercerts

Use `orgHypercertsClaimActivity` for hypercert activity claims.

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
        rkey
        title
        shortDescription
        createdAt
        startDate
        endDate
        image {
          __typename
          ... on OrgHypercertsDefsUri {
            uri
          }
          ... on OrgHypercertsDefsSmallImage {
            image { ref mimeType size }
          }
        }
        certifiedProfileData {
          displayName
          description
          website
        }
      }
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}
```

Variables:

```json
{ "after": null }
```

`certifiedProfileData` is a convenience field that resolves the record author's `app.certified.actor.profile/self` record when one exists. It is nullable.

## Fetch one hypercert by AT-URI

Use a `ByUri` query when you already have the record AT-URI.

```graphql
query HypercertByUri($uri: String!) {
  orgHypercertsClaimActivityByUri(uri: $uri) {
    uri
    cid
    did
    rkey
    title
    shortDescription
    createdAt
    startDate
    endDate
    description {
      __typename
      ... on OrgHypercertsDefsDescriptionString {
        value
      }
      ... on ComAtprotoRepoStrongRef {
        uri
        cid
      }
    }
    contributors {
      contributionWeight
      contributorIdentity {
        __typename
        ... on OrgHypercertsClaimActivityContributorIdentity {
          identity
        }
        ... on ComAtprotoRepoStrongRef {
          uri
          cid
        }
      }
      contributionDetails {
        __typename
        ... on OrgHypercertsClaimActivityContributorRole {
          role
        }
        ... on ComAtprotoRepoStrongRef {
          uri
          cid
        }
      }
    }
    rights { uri cid }
  }
}
```

Variables:

```json
{
  "uri": "at://did:plc:example/org.hypercerts.claim.activity/example-rkey"
}
```

## Query records by DID

Every typed collection can filter by the author DID.

```graphql
query HypercertsForDid($did: String!, $after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: { did: { eq: $did } }
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
{ "did": "did:plc:example", "after": null }
```

The `did` filter matches the account that owns the record. It does not search contributor arrays or other nested fields inside the record.

## Query Certified profiles

Certified profiles are stored in `app.certified.actor.profile`, usually at record key `self`.

```graphql
query CertifiedProfile($did: String!) {
  appCertifiedActorProfile(first: 1, where: { did: { eq: $did } }) {
    edges {
      node {
        uri
        cid
        did
        displayName
        description
        website
        pronouns
        createdAt
        avatar {
          __typename
          ... on OrgHypercertsDefsUri { uri }
          ... on OrgHypercertsDefsSmallImage {
            image { ref mimeType size }
          }
        }
      }
    }
  }
}
```

Variables:

```json
{ "did": "did:plc:example" }
```

## Find attachments for a hypercert

Attachment records link to subjects through nested strong references. Typed filters can check whether `subjects` is present, but they cannot filter inside the `subjects` array. To find attachments for a specific hypercert AT-URI, use `search`.

```graphql
query AttachmentsForHypercert($hypercertUri: String!, $after: String) {
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
    totalCount
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

If you only need attachments that have any subject reference, use the typed presence filter:

```graphql
query AttachmentsWithSubjects($after: String) {
  orgHypercertsContextAttachment(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: { subjects: { isNull: false } }
  ) {
    edges {
      cursor
      node {
        uri
        title
        contentType
        createdAt
        subjects { uri cid }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

## Query collections

Use `orgHypercertsCollection` to read curated groups of hypercerts and nested collections.

```graphql
query CollectionsForDid($did: String!, $after: String) {
  orgHypercertsCollection(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: { did: { eq: $did } }
  ) {
    edges {
      cursor
      node {
        uri
        cid
        did
        title
        type
        shortDescription
        createdAt
        items {
          itemIdentifier { uri cid }
          itemWeight
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{ "did": "did:plc:example", "after": null }
```

## Query EVM wallet links

Use `appCertifiedLinkEvm` for verified links between ATProto accounts and EVM wallet addresses.

```graphql
query EvmLinksForDid($did: String!, $after: String) {
  appCertifiedLinkEvm(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: { did: { eq: $did } }
  ) {
    edges {
      cursor
      node {
        uri
        cid
        did
        rkey
        address
        createdAt
        proof { __typename }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{ "did": "did:plc:example", "after": null }
```

You can also filter by address:

```graphql
where: { address: { eq: "0x0000000000000000000000000000000000000000" } }
```

## Filter by external labels

Hyperindex can expose labels from configured external ATProto labelers. Labels can target DIDs or record AT-URIs.

Use `where.externalLabels.has` to keep records with a matching active label before pagination:

```graphql
query HypercertsByLabel($labeler: String!, $value: String!, $after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
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
        cid
        did
        title
        shortDescription
        externalLabels(sources: [$labeler], values: [$value], activeOnly: true) {
          src
          uri
          cid
          val
          neg
          cts
          exp
          ver
        }
      }
    }
    pageInfo { hasNextPage endCursor }
    totalCount
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

Use `none` to exclude records with a matching label:

```graphql
where: {
  externalLabels: {
    none: { val: { eq: "low-quality" }, activeOnly: true }
  }
}
```

Use the root `externalLabels` query when you already know the DID or AT-URI subjects and only need labels:

```graphql
query LabelsForSubjects($subjects: [String!]!, $labeler: String!) {
  externalLabels(
    subjects: $subjects
    sources: [$labeler]
    values: ["high-quality"]
    activeOnly: true
  ) {
    src
    uri
    cid
    val
    neg
    cts
    exp
    ver
  }
}
```

Variables:

```json
{
  "subjects": ["at://did:plc:example/org.hypercerts.claim.activity/example-rkey"],
  "labeler": "did:plc:antf7bsm6f4ohkqfdckefyt7"
}
```

`activeOnly` defaults to `true`. Active labels exclude expired labels and labels whose latest state is a negation. Set `activeOnly: false` only when you need historical label rows.

The list-level `where.externalLabels` filter decides which records are returned. The node-level `externalLabels(...)` field decides which labels are displayed on each returned record. If you want to display only the labels used for filtering, repeat the same `sources`, `values`, and `activeOnly` arguments on the node field.

## Filtering

Typed list queries accept a collection-specific `where` object. Scalar fields support value filters; complex fields support presence filters.

Common examples:

```graphql
where: { did: { eq: "did:plc:example" } }
where: { did: { in: ["did:plc:a", "did:plc:b"] } }
where: { title: { contains: "reforestation" } }
where: { title: { startsWith: "Mangaroa" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { endDate: { isNull: false } }
where: { image: { isNull: false } }
```

Supported scalar operators:

| Operator | Applies to | Notes |
| --- | --- | --- |
| `eq` | String, Int, Float, Boolean, DateTime, DID | Exact match |
| `neq` | String, Int, Float, DateTime | Not equal |
| `in` | String, Int, DID | Match any listed value |
| `contains` | String | Substring match |
| `startsWith` | String | Prefix match |
| `gt`, `lt`, `gte`, `lte` | Int, Float, DateTime | Range comparisons |
| `isNull` | Scalar fields and complex top-level fields | Missing/null checks |

Complex top-level fields such as arrays, refs, unions, blobs, objects, bytes, and unknown JSON values only support presence checks:

```graphql
where: {
  image: { isNull: false }
  contributors: { isNull: false }
  rights: { isNull: true }
}
```

Typed filters do not match nested values inside arrays, refs, unions, blobs, or objects. If you need nested matching, use one of these patterns:

1. Use a presence filter to narrow the result set, then filter nested fields in your application.
2. Use `search(query: ..., collection: ...)` to match an AT-URI or string inside record JSON.
3. Use `records(collection: ...)` as a generic fallback when a typed query is not available.

Practical limits:

- default page size is `20`
- maximum page size is `1000`
- `in` lists can contain up to `100` values
- a query can include up to `20` field filter conditions, excluding the DID filter

## Sorting

Typed queries support `sortBy` and `sortDirection`.

```graphql
query RecentActivity($after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: startDate
    sortDirection: DESC
  ) {
    edges {
      node { uri title did startDate endDate createdAt }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Default sorting is `indexed_at DESC`, which returns records in newest-indexed order. Use `createdAt` when you care about the timestamp declared by the record author. Available sort fields are generated from scalar fields in each collection's lexicon.

## Pagination

Hyperindex uses Relay-style connections.

```graphql
query PageThroughHypercerts($after: String) {
  orgHypercertsClaimActivity(first: 20, after: $after) {
    edges {
      cursor
      node { uri title }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

To fetch the next page, pass the previous response's `pageInfo.endCursor` as `after`.

Rules of thumb:

- Use `first` and `after` for forward pagination.
- Use `last` and `before` for backward pagination.
- Do not mix forward and backward pagination arguments in the same query.
- Request `totalCount` only when you need it.

## Generic records and search

Prefer typed queries when possible. Use generic queries when you need lower-level access.

### Query any collection as JSON

```graphql
query GenericRecords($collection: String!, $after: String) {
  records(collection: $collection, first: 20, after: $after) {
    edges {
      cursor
      node {
        uri
        cid
        did
        collection
        value
        externalLabels { src val cts }
        certifiedProfileData {
          displayName
          description
        }
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{ "collection": "org.hypercerts.claim.activity", "after": null }
```

### Search record JSON

`search` matches text against record JSON. Use it for simple discovery and for nested AT-URI matching when typed filters cannot inspect nested values.

```graphql
query SearchHypercerts($query: String!, $after: String) {
  search(
    query: $query
    collection: "org.hypercerts.claim.activity"
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
    totalCount
  }
}
```

Variables:

```json
{ "query": "forest", "after": null }
```

## Collection stats and time series

Use `collectionStats` for counts and `collectionTimeSeries` for charting indexed activity by date.

```graphql
query HypercertStats {
  collectionStats(collections: [
    "org.hypercerts.claim.activity",
    "org.hypercerts.context.attachment",
    "org.hypercerts.collection",
    "app.certified.actor.profile",
    "app.certified.link.evm"
  ]) {
    collection
    count
  }
}
```

```graphql
query ActivityTimeSeries {
  collectionTimeSeries(collection: "org.hypercerts.claim.activity") {
    collection
    totalRecords
    uniqueUsers
    data {
      date
      count
      cumulative
    }
  }
}
```

## Working with unions, refs, and blobs

AT Protocol lexicons often use unions and strong references.

For unions, always request `__typename` and select fields through inline fragments:

```graphql
image {
  __typename
  ... on OrgHypercertsDefsUri {
    uri
  }
  ... on OrgHypercertsDefsSmallImage {
    image { ref mimeType size }
  }
}
```

For strong references, request `uri` and `cid`:

```graphql
rights { uri cid }
```

For blobs, request the blob reference and metadata:

```graphql
image { ref mimeType size }
```

## Best practices

- Use typed queries first. They give you field-level selection, filters, sorting, labels, and pagination.
- Always paginate list queries. Do not request unbounded lists.
- Keep selection sets small. Ask only for fields your UI or job needs.
- Use `ByUri` for canonical detail pages and cache keys.
- Use `uri` plus `cid` when a specific record version matters.
- Use `createdAt` for user-declared record chronology and `indexed_at` for indexer arrival order.
- Use `search` or client-side filtering for nested values inside arrays, refs, unions, blobs, or objects.
- Treat staging as unstable. Introspect staging before depending on staging-only fields.

## Troubleshooting

### `Cannot query field ...`

The selected endpoint's schema does not expose that field. Check that you are using the right environment, then inspect the schema in GraphiQL. Typed fields are generated from the lexicons loaded by that deployment.

### A typed `where` filter cannot match a nested value

This is expected for arrays, refs, unions, blobs, and objects. Typed filters only support scalar comparisons and top-level presence checks for complex fields. Use `search`, fetch a narrower page and filter client-side, or model the lookup through a scalar field when available.

### `certifiedProfileData` is `null`

The record author does not have an indexed Certified profile at `app.certified.actor.profile/self`, or that profile has not been indexed yet.

### Labels are missing

Confirm that you are using the right subject form. Account labels use a DID subject; record labels use an AT-URI subject. By default, `externalLabels` returns only active labels, so expired or negated labels are hidden unless you pass `activeOnly: false`.

### Newly created AT Protocol records are not visible yet

Hyperindex is an indexer. There can be delay between writing a record to an AT Protocol repository and seeing it in GraphQL. Also confirm that the record uses one of the indexed collections and that you are querying the environment that indexes that data.
