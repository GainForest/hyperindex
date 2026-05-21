# TAA-001 / GON-45 handoff

## Changed files

- `internal/config/config.go`
- `internal/config/config_test.go`
- `.env.example`
- `README.md`
- `CONTRIBUTING.md`
- `AGENTS.md`
- `docker-compose.tap.yml`

## Implementation summary

- Added `Config.AuditEnabled` with `AUDIT_ENABLED` env parsing and default `false`.
- Added startup/config validation: `AUDIT_ENABLED=true` now fails validation unless `TAP_ENABLED=true`.
- Error is actionable: it tells operators to set `TAP_ENABLED=true` or unset `AUDIT_ENABLED` because audit storage is currently Tap-only.
- Added `audit_enabled` to redacted config logging.
- Updated focused config references/examples:
  - `.env.example` documents `AUDIT_ENABLED=false` under Tap config.
  - README Tap env table includes `AUDIT_ENABLED` and its Tap requirement.
  - `CONTRIBUTING.md` and `AGENTS.md` environment notes include the Tap-only audit constraint.
  - `docker-compose.tap.yml` passes `AUDIT_ENABLED` through to the Hyperindex service with default `false`.
- Added config tests for:
  - audit validation failure without Tap,
  - audit validation success with Tap,
  - `Load()` parsing `AUDIT_ENABLED=true`,
  - default/env helper coverage alongside existing Tap config tests.

## Commands run

- `gofmt -w internal/config/config.go internal/config/config_test.go && go test ./internal/config` — exit 0
- `git diff --check -- internal/config/config.go internal/config/config_test.go .env.example README.md AGENTS.md CONTRIBUTING.md docker-compose.tap.yml` — exit 0

## Remaining blockers

- None for TAA-001 scope.
- No audit repository, migrations, GraphQL, or Tap ack-flow wiring was implemented here by design.
- No Changie fragment was added in this ticket; the Linear plan assigns final release-note work to TAA-013.

## Concurrent-worker notes / risks

- The requested `context.md` and `plan.md` files were not present in the checkout, so implementation used `docs/tap-append-only-audit.md` and the delegated task text.
- `git status` shows unrelated concurrent changes in Tap consumer/handler files and audit migrations. I did not edit those files.
