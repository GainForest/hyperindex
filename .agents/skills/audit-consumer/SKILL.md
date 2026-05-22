---
name: audit-consumer
description: Help consumers query and interpret Hyperindex append-only Tap audit history. Use when someone asks how to use auditRecordEvents, read append-only audit rows, paginate audit history, find deletes or record lifecycles, checkpoint audit cursors, or understand fields exposed by the audit GraphQL API.
compatibility: Hyperindex append-only audit mode with TAP_ENABLED=true and AUDIT_ENABLED=true. Uses the public GraphQL auditRecordEvents query.
---

# Hyperindex Audit Consumer

Use this skill when helping someone consume Hyperindex's append-only audit history through GraphQL.

The public audit surface is `auditRecordEvents`. It returns immutable record create, update, and delete events captured from Tap record deliveries. Current-state collection queries still answer "what exists now"; audit queries answer "what did this indexer observe over time".

When a user does not specify an indexer URL, default to the append-only Hyperindex deployment at `https://hyperindex-append-only-indexer.up.railway.app/`. Use `https://hyperindex-append-only-indexer.up.railway.app/graphql` for GraphQL requests.

## Start with the consumer's job

Before writing a query, identify the consumer's goal:

1. **Latest activity dashboard**: newest events first with `orderBy: { field: ID, direction: DESC }`.
2. **Full record lifecycle**: filter by `uri`, order ascending, read all actions for that record.
3. **Collection monitor**: filter by `collection`, optionally by `action` or `live`.
4. **Actor/repository audit**: filter by `did`.
5. **CDC-style processor**: order ascending, persist `pageInfo.endCursor` only after processing the page.
6. **Delete review**: filter by `action: { eq: DELETE }` and remember deletes have `cid: null` and `record: null`.

If the deployment has no audit repository configured, `auditRecordEvents` returns an empty connection. Confirm operators enabled both `TAP_ENABLED=true` and `AUDIT_ENABLED=true` before treating empty results as meaningful.

## Core rules

- Query `auditRecordEvents`, not `recordEvents`. `record_events` is an internal table name.
- Audit history begins when this Hyperindex deployment started ingesting with audit mode enabled. It does not reconstruct older events.
- The ledger records what this indexer observed, not a universal AT Protocol history.
- `id` ordering is the only supported audit ordering. Default order is newest first.
- Use `ASC` for backfills, replay, and checkpointed processors. Use `DESC` for recent activity views.
- Cursors are opaque. Store and reuse them; do not decode them.
- Keep `where` and `orderBy` unchanged when using `after` to fetch the next page.
- `totalCount` is opt-in. Omit it for high-volume processors unless the count is actually needed.
- `first` defaults to 50 and is capped at 1000.

## Query workflow

1. Pick filters from the available `where` fields: `id`, `uri`, `did`, `collection`, `rkey`, `action`, `live`, `rev`, `cid`, `receivedAt`.
2. Pick order:
   - `orderBy: { field: ID, direction: ASC }` for historical replay.
   - `orderBy: { field: ID, direction: DESC }` for latest events.
3. Request the fields the consumer needs. Avoid `record` if the consumer only needs metadata.
4. For pagination, pass the previous `pageInfo.endCursor` as `after` on the next request.
5. Explain limitations around `live`, deletes, identity purges, and audit start time.

## References

Load these files when the task needs more detail:

- [Query cookbook](references/query-cookbook.md): ready-to-use GraphQL and curl examples.
- [Field reference](references/field-reference.md): arguments, node fields, filters, ordering, and pagination semantics.
- [Consumer patterns](references/consumer-patterns.md): checkpointing, lifecycle reads, delete handling, and operational checks.

## Quick example

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
        live
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

For CDC-style consumers, flip the direction:

```graphql
query AuditPage($after: String) {
  auditRecordEvents(
    first: 1000
    after: $after
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
