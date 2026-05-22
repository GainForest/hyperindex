# Audit consumer patterns

## Choosing audit queries vs current-state queries

Use current-state collection queries when the consumer needs the latest indexed record state.

Use `auditRecordEvents` when the consumer needs immutable history:

- create/update/delete timelines
- deletion evidence
- downstream replay or export
- record lifecycle investigation
- collection activity monitoring
- repository-level audit trails

The two views are intentionally different. A deleted record can be present in `auditRecordEvents` and absent from the current-state collection query.

## Checkpointed replay

For append-only processors, use ascending ID order:

```graphql
query AuditReplayPage($after: String) {
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

Recommended processor behavior:

1. Keep a durable checkpoint containing the last successfully processed `endCursor`.
2. Request the next page with `after` set to that checkpoint.
3. Process edges in returned order.
4. Commit downstream side effects.
5. Persist the new `endCursor` only after step 4 succeeds.
6. If a page is empty, keep the old checkpoint and poll again later.

Do not update the checkpoint before downstream work succeeds. That can skip events after a crash.

## Record lifecycle reads

For one record, filter by `uri` and order ascending:

```graphql
query RecordLifecycle($uri: String!) {
  auditRecordEvents(
    first: 100
    where: { uri: { eq: $uri } }
    orderBy: { field: ID, direction: ASC }
  ) {
    edges { node { id receivedAt action cid record } }
    pageInfo { hasNextPage endCursor }
  }
}
```

Expected lifecycle signals:

- `CREATE` usually has `cid` and `record`.
- `UPDATE` usually has a new `cid` and `record`.
- `DELETE` has no current-state record and normally returns `cid: null` and `record: null`.
- A lifecycle can be incomplete if audit mode started after the original create or if Tap did not deliver older history to this deployment.

## Delete handling

When handling `DELETE` actions:

- Treat the audit row as permanent evidence that Hyperindex observed a delete.
- Do not expect `record` to contain the deleted body.
- Query current state separately if the consumer needs to confirm that the projection is currently absent.
- Remember identity purges remove current-state rows but do not create synthetic per-record `DELETE` audit rows.

## Live and backfill interpretation

`live` is Tap's delivery marker, not a record status.

- `live: false` means Tap emitted the event from backfill or resync.
- `live: true` means Tap emitted the event from live firehose following.

Do not interpret `live: false` as deleted, inactive, or untrusted.

## Counting and large ledgers

`totalCount` is useful for dashboards and sanity checks, but it causes Hyperindex to run a count query. For replay processors, omit `totalCount` and rely on `edges` plus `pageInfo`.

If `totalCount` becomes too large for GraphQL Int, add narrower filters or omit `totalCount`.

## Smoke test expectation

For deployments that should expose audit history, operators can run the API smoke suite with audit checks:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_AUDIT=1 \
  make smoke-api
```

Use this before telling a consumer that empty audit results are expected. A healthy append-only deployment should have:

- `TAP_ENABLED=true`
- `AUDIT_ENABLED=true`
- Tap collection filters covering the collections the consumer cares about
- durable storage for Tap and Hyperindex databases
- at least one returned `auditRecordEvents` edge after ingestion has started

## Consumer troubleshooting

| Symptom | Likely cause | What to do |
| --- | --- | --- |
| `auditRecordEvents` returns zero edges | Audit mode disabled, no Tap events delivered yet, or filters are too narrow. | Confirm `TAP_ENABLED=true`, `AUDIT_ENABLED=true`, Tap filters, and try no `where` filter. |
| Current-state query has a record but audit trail is empty | Audit mode may have started after that record was indexed, or the record came from legacy ingestion. | Treat audit history as starting at audit-mode enablement. |
| Audit has a delete but current-state typed query returns null | Expected for a deleted record. | Use the audit row as history and current-state query as latest state. |
| `record` is null | Delete row or Tap omitted a body. | Use metadata fields; do not assume every event includes JSON. |
| Pagination repeats or skips rows | Consumer changed `where` or `orderBy`, or checkpointed before processing succeeded. | Keep query shape stable and persist `endCursor` after successful processing only. |
| `live: false` appears in results | Tap backfill or resync event. | Do not treat it as inactive or deleted. |
