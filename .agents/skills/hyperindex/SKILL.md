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

Use production by default for consumer examples. `api.indexer.hypercerts.dev` is the production endpoint and currently exposes presence filters and typed queries for the collections above. Staging and production do not always run the same schema, and this skill usually reflects the default branch before every endpoint has caught up. Because ATProto data is network-wide, do not describe staging as a separate dataset unless you have verified an environment-specific indexing difference.

## Before answering

1. If the user asks for an exact field, filter, enum, or union and you are not sure, introspect the endpoint first.
2. Prefer schema-specific queries such as `orgHypercertsClaimActivity` over generic `records` when the collection has a typed query.
3. Use `recordTimeline` when the caller needs one newest-first feed across multiple collections. It requires `where.collection.in`, supports optional `where.did.in` author filtering, and does not expose `totalCount`.
4. Always include pagination (`first`, `after`, `pageInfo { hasNextPage endCursor }`) in list examples.
5. Keep selection sets small. Add fields only when needed for the workflow.
6. Use inline fragments for union fields such as descriptions, images, attachment content, proof fields, and strong references.
7. Do not assume the target endpoint exposes every feature described in this skill. `main` may be ahead of staging, and staging may be ahead of production. When a query depends on newer schema features such as `recordTimeline`, nested filters, author labels, or collection-specific fields, introspect the target endpoint first. If the feature is missing, tell the user which endpoint lacks it and offer the closest fallback, such as `search(query: ..., collection: ...)` plus client-side filtering.

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

Common metadata, scalar, and DID filter operators:

```graphql
where: { uri: { eq: "at://did:plc:.../org.hypercerts.claim.activity/rkey" } }
where: { uri: { in: ["at://did:plc:.../collection/rkey1", "at://did:plc:.../collection/rkey2"] } }
where: { did: { eq: "did:plc:..." } }
where: { did: { in: ["did:plc:a", "did:plc:b"] } }
where: { title: { contains: "reforestation" } }
where: { title: { startsWith: "Q1" } }
where: { createdAt: { gte: "2026-01-01T00:00:00Z" } }
where: { endDate: { isNull: false } }
```

`uri` filters are record metadata filters, not JSON-field filters. They support exact lookup with `eq` and batched lookup with `in`.

Any single `in` operator accepts up to 100 values. For larger DID, URI, label source/value, or scalar batches, split the values into multiple GraphQL requests and merge the paginated results client-side. This limit is separate from connection page size.

Complex fields expose `isNull` for presence checks. Some complex fields use the shared `PresenceFilterInput`; arrays, refs, and unions can instead expose generated nested filter inputs up to three lexicon path segments deep. Do not rely on the input type name for presence checks; introspect the field and use `isNull`. Nested scalar leaves support exact operators only: `eq`, `in`, and `isNull`. Multiple predicates inside the same array `any` must match the same array item. Nested array fields inside an existing `any` scope expose presence checks only; Hyperindex does not advertise nested `any` within another `any`.

```graphql
where: { image: { isNull: false } }
where: { subjects: { isNull: false } }
where: { contributors: { any: { contributorIdentity: { identity: { eq: "did:plc:..." } } } } }
where: { items: { any: { itemIdentifier: { uri: { eq: "at://did:plc:.../org.hypercerts.claim.activity/rkey" } } } } }
```

Nested filters do not support substring operators (`contains`, `startsWith`), comparison operators (`gt`, `lt`, `gte`, `lte`), nested-array `any` filters inside another `any`, nested sorting, arbitrary JSON paths, or automatic strong-ref dereferencing. A small set of explicit collection filter extensions may perform product-specific cross-record lookups; uploaded lexicons do not get these fields automatically.

For Hypercerts activities, use `contributorDid` when the caller needs to match inline contributor DIDs, legacy bare DID array entries, or `org.hypercerts.claim.contributorInformation` strong refs by referenced `identifier`:

```graphql
where: { contributorDid: { eq: "did:plc:..." } }
```

For Certified badge awards, use `badgeType` to filter by the referenced `app.certified.badge.definition.badgeType` without joining badge definitions client-side:

```graphql
where: { badgeType: { eq: "endorsement" } }
```

`badgeType` uses `StringFilterInput`, so it supports the same string operators exposed for badge definitions. Awards whose referenced badge definition is missing or has no `badgeType` do not match positive value filters.

For DID-rooted Certified endorsement networks, use `endorsementClosure(where: ..., first: ..., after: ...)`:

```graphql
query EndorsementClosure($did: String!) {
  endorsementClosure(
    where: { did: { eq: $did } }
    first: 100
  ) {
    truncated
    totalCount
    pageInfo { hasNextPage endCursor }
    edges {
      cursor
      node {
        did
        degree
        certifiedProfileData { did displayName avatar }
        viaAccounts {
          did
          certifiedProfileData { did displayName avatar }
        }
      }
    }
  }
}
```

`where.did.eq` is required and selects the root DID. The endorsement closure DID filter exposes only `eq`, not `in`, because each request is rooted at one DID. Optional `where.degree.eq` returns only one hop distance; the value must be `1`, `2`, or `3`. Omit `where.degree` to return all supported degrees. Results are sorted by degree then DID. `certifiedProfileData` resolves the reached account's Certified profile when one exists. `viaAccounts` lists up to 64 previous-ring accounts that led to the account, including each predecessor DID and optional Certified profile data; it is empty for direct degree-1 accounts. `truncated: true` means the server-side account cap was reached. The resolver computes active endorsement edges from current Certified badge award, definition, and response records at request time; it only counts badge awards whose subject is the `app.certified.defs#did` account DID union member, ignores record strongRef subjects, respects badge-definition `allowedIssuers` allowlists, and does not use a persisted edge table.

If a workflow needs unsupported nested matching, use one of these patterns:

- Use typed nested/presence filters to narrow the set, then filter client-side.
- Use `search(query: ..., collection: ...)` to find records whose JSON contains a referenced AT-URI or string.
- Use `records(collection: ...)` as a fallback for collections without typed schema coverage.

## Generic record timeline

Use `recordTimeline` for cross-collection feeds ordered by the record JSON's top-level `createdAt` timestamp, not by indexer arrival time. Callers must pass `where.collection.in`; omit `where.did` for all authors, pass `where.did.in` to filter author DIDs, and pass `where.did.in: []` only when an empty result is intended. `first` defaults to 50 and is capped at 1000. The connection intentionally has no `totalCount`.

```graphql
query RecentCertifiedRecords($where: RecordTimelineWhereInput!, $after: String) {
  recordTimeline(
    where: $where
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
        rkey
        createdAt
        indexedAt
        value
        certifiedProfileData { did displayName createdAt }
      }
    }
    pageInfo { hasNextPage hasPreviousPage startCursor endCursor }
  }
}
```

Variables:

```json
{
  "where": {
    "collection": {
      "in": [
        "app.certified.actor.profile",
        "org.hypercerts.claim.activity",
        "org.hypercerts.collection"
      ]
    }
  },
  "after": null
}
```

Use typed collection queries when the caller needs collection-specific filters, sorting, exact totals, or typed fields. Use `recordTimeline` when the primary requirement is one stable newest-first page across selected collections.

## External labeler filtering

Use external label filtering only after confirming that the target endpoint exposes external label support.

Supported schemas expose:

- Root query: `externalLabels(subjects: ..., sources: ..., values: ..., activeOnly: ...)`
- Record field: `externalLabels(sources: ..., values: ..., activeOnly: ...)`
- Record-label predicates: `where.externalLabels.has` and `where.externalLabels.none`
- Author-account label predicates: `where.authorLabels.has` and `where.authorLabels.none`

Use this pattern to get `high-quality` activity claims from source DID `did:plc:antf7bsm6f4ohkqfdckefyt7`:

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

`externalLabels` matches labels attached to the record AT-URI. To filter by labels attached to the record author's DID, use `authorLabels` instead. This is the right shape for orglabeler account labels such as `likely-test`, `standard`, and `high-quality` from `did:plc:pswneepkd5lesumj7ejmkbal`:

```graphql
where: {
  authorLabels: {
    none: {
      src: { eq: "did:plc:pswneepkd5lesumj7ejmkbal" }
      val: { eq: "likely-test" }
      activeOnly: true
    }
  }
}
```

To require standard or high-quality authors:

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

`authorLabels` only matches DID-subject labels with no CID. It does not infer labels from `app.certified.actor.profile/self` or organization records. There is no node-level `authorLabels` field; use the root `externalLabels(subjects: [...])` query when you need to display labels for known author DIDs.

To combine record label filtering with author account label filtering, include both predicates:

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

`org.hypercerts.context.attachment` records connect to subjects through a `subjects` array of strong refs. When the endpoint exposes nested filters, find attachments for a specific hypercert AT-URI with `subjects.any.uri.eq`:

```graphql
query AttachmentsForHypercert($hypercertUri: String!, $after: String) {
  orgHypercertsContextAttachment(
    first: 20
    after: $after
    sortBy: createdAt
    sortDirection: DESC
    where: { subjects: { any: { uri: { eq: $hypercertUri } } } }
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
{ "hypercertUri": "at://did:plc:.../org.hypercerts.claim.activity/...", "after": null }
```

If the endpoint does not expose `subjects.any.uri`, fall back to `search(query: $hypercertUri, collection: "org.hypercerts.context.attachment")` and filter the returned JSON client-side. If the caller only needs attachments that have any subject reference, use the typed presence check:

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

Use `isNull` on the field's generated filter input for complex top-level fields such as `image`. The input type may be a generated nested filter input rather than `PresenceFilterInput`.

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
- When giving user-facing parameterized examples, include both the query and variables.
- When the schema has changed, prefer live introspection over this file and mention the endpoint used.
