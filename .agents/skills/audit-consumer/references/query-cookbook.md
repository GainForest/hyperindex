# Audit query cookbook

Use these examples for consumers of Hyperindex append-only audit history. All examples use the public GraphQL field `auditRecordEvents`.

## HTTP request shape

```bash
curl -fsS "$HYPERINDEX_GRAPHQL_URL" \
  -H 'Content-Type: application/json' \
  --data @- <<'JSON'
{
  "query": "query LatestAuditEvents { auditRecordEvents(first: 5) { edges { node { id receivedAt action uri } } pageInfo { endCursor } } }"
}
JSON
```

Use the deployment's GraphQL endpoint, usually `https://<api-host>/graphql`. If the user does not provide an indexer URL, use the default append-only deployment:

```bash
export HYPERINDEX_GRAPHQL_URL=https://hyperindex-append-only-indexer.up.railway.app/graphql
```

## Latest audit events

Use this for activity dashboards or a quick smoke check.

```graphql
query LatestAuditEvents {
  auditRecordEvents(first: 20, orderBy: { field: ID, direction: DESC }) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        did
        collection
        rkey
        uri
        cid
        rev
        live
        record
      }
    }
    pageInfo {
      hasNextPage
      hasPreviousPage
      startCursor
      endCursor
    }
  }
}
```

## Full audit trail for one record

Use ascending ID order to read the lifecycle in the order Hyperindex stored it.

```graphql
query RecordAudit($uri: String!) {
  auditRecordEvents(
    first: 100
    where: { uri: { eq: $uri } }
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        did
        collection
        rkey
        uri
        cid
        rev
        live
        record
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
  "uri": "at://did:plc:alice/org.hypercerts.claim/abc123"
}
```

## Changes by DID

Use this when a consumer wants everything observed for one repository.

```graphql
query ActorAudit($did: String!, $after: String) {
  auditRecordEvents(
    first: 100
    after: $after
    where: { did: { eq: $did } }
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        collection
        rkey
        uri
        cid
        record
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

## Deletes in a collection

Deletes are durable audit rows even after the current-state record disappears.

```graphql
query DeletedClaims($after: String) {
  auditRecordEvents(
    first: 100
    after: $after
    where: {
      collection: { eq: "org.hypercerts.claim" }
      action: { eq: DELETE }
    }
    orderBy: { field: ID, direction: DESC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        uri
        did
        rkey
        rev
      }
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}
```

For delete rows, `cid` and `record` are expected to be `null`.

## Backfill or CDC-style replay

Use ascending order, process each page fully, then persist `endCursor` as the checkpoint.

```graphql
query AuditReplayPage($after: String) {
  auditRecordEvents(
    first: 1000
    after: $after
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        uri
        did
        collection
        rkey
        cid
        record
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

Consumer loop:

1. Start with `after: null` for a full replay, or a stored cursor to resume.
2. Process all `edges` in order.
3. Commit downstream side effects.
4. Persist `pageInfo.endCursor` only after downstream work succeeds.
5. Repeat while `pageInfo.hasNextPage` is true.
6. Poll again later with the last stored cursor for newly appended rows.

## Count matching events

Only request `totalCount` when a consumer needs a count. Hyperindex runs a separate count query when this field is selected.

```graphql
query CountCollectionEvents {
  auditRecordEvents(
    first: 1
    where: { collection: { eq: "org.hypercerts.claim" } }
  ) {
    totalCount
    edges { node { id uri action } }
  }
}
```

`totalCount` counts all rows matching `where`; it is not limited to the returned page and does not apply the `after` cursor.

## Received-at range

Use timestamp ranges for time-windowed exports or audits.

```graphql
query AuditWindow($start: DateTime!, $end: DateTime!) {
  auditRecordEvents(
    first: 1000
    where: { receivedAt: { gt: $start, lt: $end } }
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node { id receivedAt action uri cid record }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```

## Metadata-only query

Use this when consumers do not need full record JSON.

```graphql
query AuditMetadataOnly($after: String) {
  auditRecordEvents(
    first: 1000
    after: $after
    orderBy: { field: ID, direction: ASC }
  ) {
    edges {
      cursor
      node {
        id
        receivedAt
        action
        did
        collection
        rkey
        uri
        cid
        rev
        live
      }
    }
    pageInfo { hasNextPage endCursor }
  }
}
```
