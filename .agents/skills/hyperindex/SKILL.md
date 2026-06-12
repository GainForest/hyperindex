---
name: hyperindex
description: Query the Hypercerts Hyperindex GraphQL indexer for ATProto hypercert records. Use when consumers need GraphQL queries, filters, pagination, sorting, or workflows for org.hypercerts.* and app.certified.* records on api.indexer.hypercerts.dev or dev.api.indexer.hypercerts.dev.
compatibility: Requires network access to the Hyperindex GraphQL endpoint. Examples target the current ATProto Hypercerts schema exposed by api.indexer.hypercerts.dev/graphql.
allowed-tools: bash read fetch_content
metadata:
  product: Hyperindex
  production_endpoint: https://api.indexer.hypercerts.dev/graphql
  staging_endpoint: https://dev.api.indexer.hypercerts.dev/graphql
---

# Hyperindex Queries

Use this skill when helping consumers read Hypercerts records from the hosted Hyperindex GraphQL API.

The current production indexer is ATProto-first. Do **not** explain it using old Hyperindex concepts such as generic attestations or older claim models unless the live schema exposes them. Base generated queries on the live schema and the current typed production collections:

- `app.certified.actor.organization`
- `app.certified.actor.profile`
- `app.certified.badge.award`
- `app.certified.badge.definition`
- `app.certified.badge.response`
- `app.certified.graph.follow`
- `app.certified.link.evm`
- `app.certified.location`
- `org.hypercerts.claim.activity`
- `org.hypercerts.claim.contribution`
- `org.hypercerts.claim.contributorInformation`
- `org.hypercerts.claim.rights`
- `org.hypercerts.collection`
- `org.hypercerts.context.acknowledgement`
- `org.hypercerts.context.attachment`
- `org.hypercerts.context.evaluation`
- `org.hypercerts.context.measurement`
- `org.hypercerts.funding.receipt`
- `org.hypercerts.workscope.tag`

## Endpoints

- Production: `https://api.indexer.hypercerts.dev/graphql`
- Staging: `https://dev.api.indexer.hypercerts.dev/graphql`

Use production by default for consumer examples. `api.indexer.hypercerts.dev` is the production endpoint and currently exposes external label queries, label-aware filters, presence filters, and typed queries for the collections above. Staging may expose schema changes earlier or have different indexed data; introspect it before giving staging-specific answers.

## Before answering

1. If the user asks for an exact field, filter, enum, or union and you are not sure, introspect the endpoint first.
2. Prefer schema-specific queries such as `orgHypercertsClaimActivity` over generic `records` when the collection has a typed query.
3. Always include pagination (`first`, `after`, `pageInfo { hasNextPage endCursor }`) in list examples.
4. Keep selection sets small. Add fields only when needed for the workflow.
5. Use inline fragments for union fields such as descriptions, images, attachment content, proof fields, and strong references.
6. Use `search(query: ..., collection: ...)` when the caller needs to match a nested URI/string inside JSON. Typed `where` inputs only support scalar comparisons and top-level presence checks for complex fields.

Detailed schema reference: [references/schema-reference.md](references/schema-reference.md)
Consumer-facing documentation page: [`docs/hyperindex.md`](../../../docs/hyperindex.md)

## GraphQL request shape

```bash
curl -s https://api.indexer.hypercerts.dev/graphql \
  -H 'content-type: application/json' \
  --data '{"query":"query { collectionStats { collection count } }"}'
```

For examples with variables:

```json
{
  "query": "query HypercertsForDid($did: String!) { orgHypercertsClaimActivity(first: 20, where: { did: { eq: $did } }) { edges { node { uri title } } } }",
  "variables": { "did": "did:plc:..." }
}
```

## Core filter model

Most typed list queries accept:

- `where`: collection-specific filter object
- `first` / `after`: forward pagination
- `last` / `before`: backward pagination
- `sortBy`: collection-specific enum, default is usually `indexed_at`
- `sortDirection`: `ASC` or `DESC`, default is `DESC`

Common scalar and DID filter operators:

```graphql
where: { did: { eq: "did:plc:..." } }
where: { did: { in: ["did:plc:a", "did:plc:b"] } }
where: { title: { contains: "reforestation" } }
where: { title: { startsWith: "Q1" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { endDate: { isNull: false } }
```

Complex top-level fields such as arrays, refs, objects, blobs, and unions use `PresenceFilterInput`. That means you can ask whether the field is present, but not match nested values through typed `where` inputs:

```graphql
where: { image: { isNull: false } }
where: { subjects: { isNull: false } }
where: { rights: { isNull: true } }
```

If a workflow needs nested matching, use one of these patterns:

- Use typed presence filters to narrow the set, then filter nested fields client-side.
- Use `search(query: ..., collection: ...)` to find records whose JSON contains a referenced AT-URI or string.
- Use `records(collection: ...)` as a fallback for collections without typed schema coverage.

## External labeler filtering

Production exposes locally ingested external ATProto labels and can filter records by those labels before pagination.

Endpoint status tested on 2026-06-10:

- Production supports external label queries and filters: `https://api.indexer.hypercerts.dev/graphql`
- Root query: `externalLabels(subjects: ..., sources: ..., values: ..., activeOnly: ...)`
- Record field: `externalLabels(sources: ..., values: ..., activeOnly: ...)`
- Typed predicates: `where.externalLabels.has` and `where.externalLabels.none`

Current production activity labels are available from source DID `did:plc:antf7bsm6f4ohkqfdckefyt7`. Use this tested pattern to get `high-quality` activity claims:

```graphql
query HighQualityActivityClaimsByLabeler($labeler: String!, $after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: {
      externalLabels: {
        has: {
          src: { eq: $labeler }
          val: { eq: "high-quality" }
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
        createdAt
        externalLabels(
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
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}
```

Variables:

```json
{ "labeler": "did:plc:antf7bsm6f4ohkqfdckefyt7", "after": null }
```

Test result on production: the query returned high-quality `org.hypercerts.claim.activity` records, including `Building food systems and native forests in Mangaroa` and `Hypercerts for Land Stewards`.

To combine label filtering with author filtering, add a normal DID filter beside `externalLabels`:

```graphql
where: {
  did: { eq: "did:plc:activity-author..." }
  externalLabels: {
    has: {
      src: { eq: "did:plc:antf7bsm6f4ohkqfdckefyt7" }
      val: { eq: "high-quality" }
      activeOnly: true
    }
  }
}
```

Use `none` to exclude labels:

```graphql
where: {
  externalLabels: {
    none: { val: { eq: "low-quality" }, activeOnly: true }
  }
}
```

Use the root `externalLabels` query when you already have subject DIDs or AT-URIs and only need their labels:

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

If an endpoint does not expose `externalLabels`, tell the consumer the selected hosted schema cannot filter by labels. Do not silently replace label filtering with text search; that can return false positives.

## Consumer workflows

### Get hypercerts for a specific DID

Use `orgHypercertsClaimActivity` and filter by author DID.

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
        rkey
        title
        shortDescription
        createdAt
        startDate
        endDate
        rights { uri cid }
        image {
          __typename
          ... on OrgHypercertsDefsUri { uri }
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
{ "did": "did:plc:...", "after": null }
```

### Get a single hypercert by AT-URI

Use the `ByUri` query for a known AT-URI.

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
      ... on OrgHypercertsDefsDescriptionString { value }
      ... on ComAtprotoRepoStrongRef { uri cid }
    }
    contributors {
      contributionWeight
      contributorIdentity {
        __typename
        ... on OrgHypercertsClaimActivityContributorIdentity { identity }
        ... on ComAtprotoRepoStrongRef { uri cid }
      }
      contributionDetails {
        __typename
        ... on OrgHypercertsClaimActivityContributorRole { role }
        ... on ComAtprotoRepoStrongRef { uri cid }
      }
    }
    workScope { __typename }
    rights { uri cid }
  }
}
```

Variables:

```json
{ "uri": "at://did:plc:.../org.hypercerts.claim.activity/..." }
```

### Get attachments relevant to a hypercert

`org.hypercerts.context.attachment` records connect to subjects through a `subjects` array of strong refs. Production exposes `subjects` as a presence filter, not as a nested URI filter. To find attachments for a specific hypercert AT-URI, use `search` with the hypercert AT-URI, then read matching attachment records.

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
{ "hypercertUri": "at://did:plc:.../org.hypercerts.claim.activity/...", "after": null }
```

If the caller only needs attachments that have any subject reference, use the typed presence filter:

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

Variables:

```json
{ "after": null }
```

### Filter hypercerts with images

Use `PresenceFilterInput` for complex top-level fields such as `image`.

```graphql
query ActivityClaimsWithImages($after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    where: { image: { isNull: false } }
  ) {
    edges {
      cursor
      node {
        uri
        title
        image { __typename }
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

### Sort hypercerts by activity dates or creation time

Use `sortBy` on `orgHypercertsClaimActivity`. Production sort fields are `indexed_at`, `title`, `startDate`, `endDate`, `createdAt`, and `shortDescription`.

```graphql
query RecentActivity($after: String) {
  orgHypercertsClaimActivity(
    first: 20
    after: $after
    sortBy: startDate
    sortDirection: DESC
  ) {
    edges {
      cursor
      node { uri title did startDate endDate createdAt }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

Variables:

```json
{ "after": null }
```

Use `createdAt` for client-declared record creation time. Use `indexed_at` when the consumer wants indexer arrival order.

### Get a certified profile for a DID

Use `appCertifiedActorProfile` with the DID filter.

```graphql
query CertifiedProfile($did: String!) {
  appCertifiedActorProfile(first: 1, where: { did: { eq: $did } }) {
    edges {
      cursor
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
        }
      }
    }
    pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
  }
}
```

Variables:

```json
{ "did": "did:plc:..." }
```

### Get collections for a DID

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
{ "did": "did:plc:...", "after": null }
```

### Get EVM account links

Use `appCertifiedLinkEvm` for `app.certified.link.evm` records. Filter by author DID or EVM `address` when the consumer needs wallet-account links.

```graphql
query EvmLinksForDid($did: String!, $after: String) {
  appCertifiedLinkEvm(
    first: 20
    after: $after
    where: { did: { eq: $did } }
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
{ "did": "did:plc:...", "after": null }
```

### Get record counts by collection

Use this for dashboards, health checks, and deciding which typed query to use.

```graphql
query HypercertCollectionStats {
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

## Answering rules for agents

- Say “hypercert” when referring to `org.hypercerts.claim.activity` records unless the user is asking about another collection explicitly.
- Say “attachment” for `org.hypercerts.context.attachment` records.
- Say “certified profile” for `app.certified.actor.profile` records.
- Say “EVM link” or “wallet link” for `app.certified.link.evm` records.
- Do not claim typed filters can match nested values inside refs, arrays, blobs, objects, or unions. Production exposes those fields as presence filters only.
- When giving user-facing parameterized examples, include both the query and variables.
- When the schema has changed, prefer live introspection over this file and mention the endpoint used.
