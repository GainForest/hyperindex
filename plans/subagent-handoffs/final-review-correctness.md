## Review

- Correct:
  - Ack-after-commit is preserved: `Consumer.dispatch` only sends ack after `HandleEvent` returns nil (`internal/tap/consumer.go:287-308`), and audit ingest commits before returning (`internal/database/repositories/audit.go:206-226`).
  - Duplicate record deliveries preserve raw rows and skip decoded/projection repeats via `ON CONFLICT(event_key) DO NOTHING` plus `if inserted` projection gating (`internal/database/repositories/audit.go:334-365`, `448-499`).
  - GraphQL action enum maps DB values to `CREATE|UPDATE|DELETE` correctly (`internal/graphql/schema/audit.go:17-24`, `339-349`).

- Blocker:
  - SQLite `receivedAt` filters are incorrect for runtime-created rows. SQLite migrations default `received_at` to `datetime('now')`, producing `"YYYY-MM-DD HH:MM:SS"` (`internal/database/migrations/sqlite/007_add_audit_events.up.sql:12`, `27`, `54`), but GraphQL DateTime inputs/examples are ISO-style strings. The repository compares strings directly for SQLite (`internal/database/repositories/audit.go:656-663`, `700-707`), so same-day comparisons like `received_at > "2026-01-01T00:00:01Z"` can return wrong results.
    - Smallest safe fix: in SQLite timestamp conditions, compare normalized timestamps, e.g. `julianday(received_at) <op> julianday(?)` or `datetime(received_at) <op> datetime(?)`; keep Postgres cast behavior as-is. Add a test that filters an audit row inserted through `IngestTapEvent`, not only manually seeded ISO timestamps.

- Note:
  - I did not write `/home/kzoeps/Projects/gainforest/append-only-indexer/plans/subagent-handoffs/final-review-correctness.md` because the task also said review-only/do not edit files.