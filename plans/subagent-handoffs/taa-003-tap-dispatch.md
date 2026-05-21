# TAA-003 / GON-47 — Tap full-event handler dispatch

## Changed files

- `internal/tap/consumer.go`
- `internal/tap/handler.go`
- `internal/tap/consumer_test.go`

## Implementation summary

- Changed `tap.EventHandler` to expose `HandleEvent(ctx context.Context, rawPayload []byte, event *Event) error`.
- Updated the Tap consumer to pass the exact raw websocket text payload plus the parsed top-level `Event` into the handler.
- Kept ack behavior tied to the top-level delivery ID and only sent ack after `HandleEvent` succeeds.
- Preserved current unsupported-event behavior: unknown typed events are logged and skipped without acking.
- Added `IndexHandler.HandleEvent` as a compatibility adapter that delegates to the existing `HandleRecord` / `HandleIdentity` methods, preserving non-audit indexing behavior.
- Updated Tap consumer tests and added coverage that verifies the handler receives both the raw payload bytes and the full event envelope.

## Commands run

- `gofmt -w internal/tap/consumer.go internal/tap/handler.go internal/tap/consumer_test.go` — exit 0
- `go test ./internal/tap` — exit 0
- `go test ./cmd/hyperindex` — exit 0
- `go test -race ./internal/tap` — exit 0

## Remaining blockers

- None for TAA-003.
- Audit config, migrations, repository wiring, and GraphQL remain intentionally unimplemented for later tickets.

## Concurrent worker / checkout risks

- The requested `context.md` and `plan.md` files were not present in the checkout; implementation used `docs/tap-append-only-audit.md` context and local Tap files instead.
- The working tree contains unrelated concurrent changes in config/docs/migrations (`.env.example`, `AGENTS.md`, `CONTRIBUTING.md`, `README.md`, `docker-compose.tap.yml`, `internal/config/*`, audit migration files, docs/plans). I did not modify those.
