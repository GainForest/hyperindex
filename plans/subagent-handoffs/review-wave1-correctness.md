# Review — Tap append-only audit wave 1 correctness

## Blockers

- None found in the reviewed wave-1 changes.

## Fixes worth doing now

- **Decide/fix Postgres raw payload storage before implementing `AuditRepository`.** The plan routes the exact websocket bytes into `HandleEvent` (`docs/tap-append-only-audit.md:263-280`) and describes `raw_tap_events` as the archive for every parsed Tap delivery (`docs/tap-append-only-audit.md:140-151`, `docs/tap-append-only-audit.md:500-508`). SQLite stores `raw_tap_events.payload` as `TEXT` (`internal/database/migrations/sqlite/007_add_audit_events.up.sql:7-14`), but Postgres stores it as `JSONB` (`internal/database/migrations/postgres/007_add_audit_events.up.sql:7-14`). `JSONB` will normalize the JSON and cannot preserve byte-for-byte payload details such as object key order/whitespace/duplicate keys. If the intended raw archive is the exact payload passed by the consumer, the smallest safe fix is to change the Postgres `raw_tap_events.payload` column to `TEXT NOT NULL` now, before repository code depends on the schema. If semantic JSON storage is intentional, document that `raw_tap_events.payload` is normalized on Postgres rather than byte-exact.

## Optional / defer

- **User-facing docs are forward-looking until TAA-006 wiring lands.** `README.md:104-108` says `AUDIT_ENABLED` stores append-only audit history, and `internal/config/config.go:63-64` describes the config field the same way. In the current working tree, startup accepts `AUDIT_ENABLED=true` with Tap enabled (`internal/config/config.go:182-184`), but `startTap` still always constructs the non-audit `tap.NewIndexHandler` (`cmd/hyperindex/main.go:803-811`). This is expected for wave 1 if the branch will continue directly into the repository/wiring tickets, but do not ship this wave standalone with that wording unless it is rephrased as reserved/coming-soon or the audit handler is wired.
- **Postgres migration execution still needs real-DB verification later.** SQLite migration tests passed and the SQL follows existing Postgres conventions, but I did not have evidence of `007_add_audit_events` being applied against a live Postgres instance in this review.

## Correct

- **Config gate matches the plan.** `AUDIT_ENABLED` is parsed with default `false` (`internal/config/config.go:119-123`), logged as non-secret state (`internal/config/config.go:202-208`), and validation fails with an actionable error when audit is enabled without Tap (`internal/config/config.go:182-184`). Tests cover both failure and accepted Tap-enabled cases (`internal/config/config_test.go:334-356`) plus env loading (`internal/config/config_test.go:390-405`).
- **Tap dispatch now preserves the data needed by audit ingest and keeps ack-after-handler semantics.** The handler interface receives raw payload bytes and the full top-level event (`internal/tap/consumer.go:52-59`). The consumer parses the websocket bytes, calls `dispatch` with the same `data`, invokes `HandleEvent`, and sends `{"type":"ack","id":event.ID}` only after the handler returns nil (`internal/tap/consumer.go:247-308`). Unsupported typed events remain logged and unacked (`internal/tap/consumer.go:273-284`). Tests verify exact raw payload/full event delivery (`internal/tap/consumer_test.go:224-273`), handler-error no-ack behavior (`internal/tap/consumer_test.go:955-990`), and top-level ack id format (`internal/tap/consumer_test.go:1038-1065`).
- **Legacy Tap behavior remains adapted.** `IndexHandler.HandleEvent` delegates record/identity events to the existing current-state methods (`internal/tap/handler.go:37-47`), so audit-disabled Tap mode continues through the current repositories until audit wiring is implemented.
- **Audit migrations mostly match the schema plan.** The new migrations create `raw_tap_events`, `record_events`, and `identity_events` with unique event keys, raw-event foreign keys, Tap delivery ids, action/type checks, and the planned indexes (`internal/database/migrations/sqlite/007_add_audit_events.up.sql:7-62`, `internal/database/migrations/postgres/007_add_audit_events.up.sql:7-62`). Down migrations drop child tables before `raw_tap_events`.

## Verification performed

- `gofmt -l internal/config/config.go internal/config/config_test.go internal/tap/consumer.go internal/tap/consumer_test.go internal/tap/handler.go` — no output.
- `git diff --check` on reviewed source/docs/migration files — no output.
- `go test ./cmd/hyperindex ./internal/config ./internal/tap ./internal/database/migrations` — passed.
- `go test ./...` — passed.

## Review notes

- `/home/kzoeps/Projects/gainforest/append-only-indexer/plan.md` and `progress.md` were not present, so I used `docs/tap-append-only-audit.md`, the scoped files, and the wave handoff notes under `plans/subagent-handoffs/`.

## Confidence

- **High** for Go integration, config validation, Tap ack semantics, and SQLite migration behavior.
- **Medium** for Postgres migration runtime behavior until applied against a live Postgres database; the only substantive schema concern is the `raw_tap_events.payload` `JSONB` vs exact raw payload decision above.
