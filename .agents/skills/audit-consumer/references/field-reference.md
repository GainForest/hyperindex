# Audit GraphQL field reference

## Root query

```graphql
auditRecordEvents(
  first: Int
  after: String
  where: AuditRecordEventWhere
  orderBy: AuditRecordEventOrder
): AuditRecordEventConnection!
```

`auditRecordEvents` is a built-in public GraphQL query for append-only record history. It is not generated from lexicons.

## Arguments

| Argument | Meaning | Notes |
| --- | --- | --- |
| `first` | Number of events to return. | Defaults to 50. Maximum is 1000. |
| `after` | Cursor returned by a previous page. | Keep the same `where` and `orderBy` between pages. |
| `where` | Filter object. | Filters are exact-match except `receivedAt`, which supports ranges. |
| `orderBy` | Stable ID ordering. | Only `field: ID` is supported. Direction defaults to newest first when omitted. |

## Connection fields

| Field | Meaning |
| --- | --- |
| `edges` | Page rows. Each edge has `cursor` and `node`. |
| `pageInfo.hasNextPage` | Whether another page exists after this page using the same query shape. |
| `pageInfo.hasPreviousPage` | Whether rows exist before this page in the selected order. |
| `pageInfo.startCursor` | Cursor for the first returned edge, or null when empty. |
| `pageInfo.endCursor` | Cursor for the last returned edge, or null when empty. |
| `totalCount` | Count of all rows matching `where`. Only computed when selected. |

`totalCount` does not count only the current page. It also does not apply the `after` cursor.

## Node fields

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | `ID!` | Stable append-only `record_events` row id. Returned as a GraphQL ID string. |
| `receivedAt` | `String!` | Database timestamp when Hyperindex stored the Tap delivery. |
| `live` | `Boolean!` | Tap live/backfill marker. `false` means backfill or resync; `true` means live firehose after following the repo. |
| `rev` | `String!` | Repository revision for the change. Can be an empty string when Tap omitted it. |
| `did` | `String!` | Repository DID that owns the record. |
| `collection` | `String!` | AT Protocol collection NSID. |
| `rkey` | `String!` | Record key within the collection. |
| `uri` | `String!` | AT-URI assembled as `at://<did>/<collection>/<rkey>`. |
| `action` | `AuditRecordAction!` | `CREATE`, `UPDATE`, or `DELETE`. |
| `cid` | `String` | Content identifier for create/update events when Tap provided one. Usually null for deletes. |
| `record` | `JSON` | Decoded record body for create/update events. Null for deletes and missing bodies. |

## Filters

`AuditRecordEventWhere` supports these fields:

| Filter | Shape | Example |
| --- | --- | --- |
| `id` | `{ eq: Int }` | `{ id: { eq: 123 } }` |
| `uri` | `{ eq: String }` | `{ uri: { eq: "at://did:plc:alice/org.hypercerts.claim/abc123" } }` |
| `did` | `{ eq: String }` | `{ did: { eq: "did:plc:alice" } }` |
| `collection` | `{ eq: String }` | `{ collection: { eq: "org.hypercerts.claim" } }` |
| `rkey` | `{ eq: String }` | `{ rkey: { eq: "abc123" } }` |
| `action` | `{ eq: AuditRecordAction }` | `{ action: { eq: DELETE } }` |
| `live` | `{ eq: Boolean }` | `{ live: { eq: false } }` |
| `rev` | `{ eq: String }` | `{ rev: { eq: "3lxyz..." } }` |
| `cid` | `{ eq: String }` | `{ cid: { eq: "bafy..." } }` |
| `receivedAt` | `{ eq: DateTime, gt: DateTime, lt: DateTime }` | `{ receivedAt: { gt: $start, lt: $end } }` |

Notes:

- `id` is returned as GraphQL `ID`, but the `id` filter accepts GraphQL `Int`. For very large ledgers, an ID may exceed GraphQL Int range; prefer cursors, `uri`, `receivedAt`, or other filters in that case.
- Use `cid: { eq: "" }` only when looking for rows where Tap omitted a CID or no CID was stored. Deletes usually return `cid: null` in the node.
- Action filter literals are enum values: `CREATE`, `UPDATE`, `DELETE`. Do not quote them in inline GraphQL. When using JSON variables, pass strings such as `"DELETE"`.

## Ordering

Only stable row-id ordering is supported:

```graphql
orderBy: { field: ID, direction: ASC }
orderBy: { field: ID, direction: DESC }
```

Use `ASC` for replay and checkpointing. Use `DESC` for latest events. If `orderBy` is omitted, consumers should treat the result as newest first.

## What is not exposed publicly

- `raw_tap_events` is operator/database-only. It stores every successfully parsed Tap delivery, including duplicate deliveries.
- `identity_events` is operator/database-only. It records identity updates and purge-style events.
- Identity purges do not create synthetic per-record delete rows in `auditRecordEvents`.
