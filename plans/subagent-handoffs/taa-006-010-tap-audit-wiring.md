# TAA-006/GON-50 and focused TAA-010/GON-54 handoff — Tap audit wiring

## Changed files

- `internal/tap/handler.go`
- `internal/tap/handler_test.go`
- `internal/tap/consumer_test.go`
- `cmd/hyperindex/main.go`

Notes:

- `context.md` and `plan.md` were not present in the checkout, so I used `docs/tap-append-only-audit.md` and the delegated task text.
- I did not edit GraphQL schema/resolver files, migrations, `AuditRepository`, Linear, or unrelated Tap/current-state behavior for this task.
- `cmd/hyperindex/main.go` already had parent-added base `services.audit`/GraphQL repository wiring in the working tree; this task only changed Tap startup selection/logging.

## Audit-mode wiring behavior

- Added `tap.NewAuditIndexHandler(...)`.
- `IndexHandler.HandleEvent` now has two paths:
  - non-audit mode: preserves existing `HandleRecord` / `HandleIdentity` current-state behavior;
  - audit mode: maps `*tap.Event` into `*repositories.AuditTapEvent`, calls `AuditRepository.IngestTapEvent(ctx, rawPayload, auditEvent)`, and returns only after that repository transaction succeeds.
- The audit handler returns a clear error if audit mode is enabled without an `AuditRepository`, so the consumer will not ack a delivery that was not durably audited.
- `cmd/hyperindex startTap` now switches to `tap.NewAuditIndexHandler` when `cfg.AuditEnabled` is true and logs `audit_enabled` with Tap startup.
- The audit path does not call `RecordsRepository` or `ActorsRepository` directly from `HandleEvent`; current-state writes remain owned by `AuditRepository.IngestTapEvent`.

## Subscription behavior

- Record subscriptions publish only after `AuditRepository.IngestTapEvent` returns successfully.
- Duplicate decoded record events (`result.Inserted == false`) do not publish duplicate subscriptions.
- Newly inserted create/update events publish only when the record body is present.
- Newly inserted delete events publish a delete subscription.
- Missing-body create/update events are audited and acknowledged by the handler path but do not update current state or publish subscriptions.
- Identity audit events update/purge current actor state through `AuditRepository` and do not publish record subscriptions.

## Tests added/adjusted

- Added audit-mode handler tests for:
  - create event inserts raw/audit/current state and publishes after ingest;
  - duplicate decoded record event inserts another raw row but does not publish again;
  - missing record body audits without current record or subscription;
  - identity event writes identity audit/current actor without record subscription.
- Added consumer-level audit handler test verifying a real audit handler acks after successful ingest and current-state commit.

## Validation

- `gofmt -w internal/tap/handler.go internal/tap/handler_test.go internal/tap/consumer_test.go cmd/hyperindex/main.go` — exit 0
- `go test ./internal/tap` — exit 0
- `go test ./cmd/hyperindex` — exit 0
- `go test ./internal/tap ./cmd/hyperindex ./internal/database/repositories` — exit 0
- `go test -race ./internal/tap` — exit 0
- `git diff --check -- internal/tap/handler.go internal/tap/handler_test.go internal/tap/consumer_test.go cmd/hyperindex/main.go plans/subagent-handoffs/taa-006-010-tap-audit-wiring.md` — exit 0

## Gaps / follow-ups

- Full TAA-010 matrix can still add more consumer failure cases around audit repository failures, but the existing handler-error no-ack test plus the new audit success path cover the focused ack-after-handler-success contract.
- Legacy activity logging is not performed in audit mode; audit storage is the source of truth, and this keeps the handler ack path limited to audit ingest plus subscription publish.
- Postgres runtime validation was not run in this task.

## Decisions needed

- None. The parent-approved `repositories.AuditTapEvent` adapter boundary was used.
