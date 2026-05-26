---
name: hyperindex-consumer
description: Query the Hypercerts Hyperindex GraphQL indexer for ATProto hypercert records. Use when consumers need GraphQL queries, filters, pagination, sorting, or workflows for org.hypercerts.* and app.certified.* records on api.indexer.hypercerts.dev or dev.api.indexer.hypercerts.dev.
compatibility: Requires network access to the Hyperindex GraphQL endpoint. Examples target the current ATProto Hypercerts schema exposed by api.indexer.hypercerts.dev/graphql.
allowed-tools: bash read fetch_content
metadata:
  product: Hyperindex
  production_endpoint: https://api.indexer.hypercerts.dev/graphql
  staging_endpoint: https://dev.api.indexer.hypercerts.dev/graphql
---

# Hyperindex Consumer Queries

Use this skill when helping consumers read Hypercerts records from the hosted Hyperindex GraphQL API.

The current indexer is ATProto-first. Do **not** explain it using old Hyperindex concepts such as generic attestations or older claim models unless the live schema exposes them. Base generated queries on the live schema and the current lexicons:

- `org.hypercerts.claim.activity`
- `org.hypercerts.context.attachment`
- `org.hypercerts.claim.contribution`
- `org.hypercerts.claim.contributorInformation`
- `org.hypercerts.claim.rights`
- `org.hypercerts.collection`
- `org.hypercerts.context.evaluation`
- `org.hypercerts.context.measurement`
- `org.hypercerts.context.acknowledgement`
- `org.hypercerts.funding.receipt`
- `org.hypercerts.workscope.tag`
- `app.certified.actor.profile`
- `app.certified.actor.organization`

## Endpoints

- Production: `https://api.indexer.hypercerts.dev/graphql`
- Staging: `https://dev.api.indexer.hypercerts.dev/graphql`
- Label test endpoint: `https://hyperindex-test.up.railway.app/graphql`

Use production by default for consumer examples. Production and staging should have the same indexed data; staging may expose schema changes or new features earlier than production. Use the label test endpoint for external label filtering examples until prod/staging expose `externalLabels`.

## Before answering

1. If the user asks for an exact field/filter and you are not sure, introspect the endpoint first.
2. Prefer schema-specific queries such as `orgHypercertsClaimActivity` over generic `records` when the collection has a typed query.
3. Always include pagination (`first`, `after`, `pageInfo { hasNextPage endCursor }`) in list examples.
4. Keep selection sets small. Add fields only when needed for the workflow.
5. Use inline fragments for union fields such as descriptions, images, attachment content, and strong references.

Detailed schema reference: [references/schema-reference.md](references/schema-reference.md)


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

Common filter operators:

```graphql
where: { did: { eq: "did:plc:..." } }
where: { did: { in: ["did:plc:a", "did:plc:b"] } }
where: { title: { contains: "reforestation" } }
where: { title: { startsWith: "Q1" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { endDate: { isNull: false } }
```

Typed filters currently do not expose every nested relation. If a workflow needs nested matching, use one of these patterns:

- Query a small typed set, then filter nested fields client-side.
- Use `search(query: ..., collection: ...)` to find records whose JSON contains a referenced AT-URI.
- Use `records(collection: ...)` as a fallback for collections without typed schema coverage.

## External labeler filtering

Hyperindex can expose locally ingested external ATProto labels and use them to filter records before pagination. This is the most important consumer workflow for curated activity claims, but do not assume every hosted endpoint has the label schema enabled.

Endpoint status tested on 2026-05-25:

- Label test endpoint supports external label queries: `https://hyperindex-test.up.railway.app/graphql`
- Production did **not** expose `externalLabels` yet: `https://api.indexer.hypercerts.dev/graphql`
- Staging did **not** expose `externalLabels` yet: `https://dev.api.indexer.hypercerts.dev/graphql`

Before giving a label-based query for an endpoint, introspect and confirm these fields exist:

- root query: `externalLabels(subjects: ..., sources: ..., values: ..., activeOnly: ...)`
- record field: `externalLabels(sources: ..., values: ..., activeOnly: ...)`
- typed `where.externalLabels.has` / `where.externalLabels.none` predicates

Use this tested pattern to get `high-quality` activity claims labeled by `did:plc:edod7rboajioq3jbyxsgeicc` on the label test endpoint:

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
{ "labeler": "did:plc:edod7rboajioq3jbyxsgeicc", "after": null }
```

Test result on `https://hyperindex-test.up.railway.app/graphql`: the query returned high-quality `org.hypercerts.claim.activity` records, including `Mangrove Restoration & Environmental Education`, `Jagomir Bee Corridor - dMRV Report`, and `Restoring soil and food security at Kanyanjwa community in Homa Bay, Kenya`.

To combine label filtering with author filtering, add a normal DID filter beside `externalLabels`:

```graphql
where: {
  did: { eq: "did:plc:activity-author..." }
  externalLabels: {
    has: {
      src: { eq: "did:plc:edod7rboajioq3jbyxsgeicc" }
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

If the endpoint does not expose `externalLabels`, tell the consumer the hosted schema cannot yet filter by labels. Do not silently replace label filtering with text search; that can return false positives.

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

`org.hypercerts.context.attachment` records connect to subjects through a `subjects` array of strong refs. The typed attachment filter does not currently expose `subjects`, so use `search` with the hypercert AT-URI, then read matching attachment records.

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

If the caller only needs attachments by a known author, use the typed query and filter client-side by `subjects.uri`:

```graphql
query AttachmentsByDid($did: String!, $after: String) {
  orgHypercertsContextAttachment(
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
        contentType
        createdAt
        subjects { uri cid }
        content {
          __typename
          ... on OrgHypercertsDefsUri { uri }
          ... on OrgHypercertsDefsSmallBlob { blob { ref mimeType size } }
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

### Sort hypercerts by activity dates or creation time

Use `sortBy` on `orgHypercertsClaimActivity`. Available sort fields include `indexed_at`, `title`, `startDate`, `endDate`, `createdAt`, and `shortDescription`.

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

### Get record counts by collection

Use this for dashboards, health checks, and deciding which typed query to use.

```graphql
query HypercertCollectionStats {
  collectionStats(collections: [
    "org.hypercerts.claim.activity",
    "org.hypercerts.context.attachment",
    "org.hypercerts.collection",
    "app.certified.actor.profile"
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
- Do not claim attachments are directly joinable through GraphQL filters unless the live schema adds a subject filter.
- When giving user-facing parameterized examples, include both the query and variables.
- When the schema has changed, prefer live introspection over this file and mention the endpoint used.
