# AGENTS.md

## Repository orientation

- Product name is **Hyperindex**. The project was renamed from Hypergoat; current technical identifiers should use **`hyperindex`**.
- This repo contains two codebases:
  - the **Go backend** at the repo root
  - the **Next.js frontend** in `client/`
- Use `bd` for task tracking. Run `bd onboard` if you need the repo-local workflow.

## Key boundaries and entrypoints

- Backend entrypoint: `cmd/hyperindex/`
- Backend database schema source of truth: `internal/database/migrations/`
- Frontend app: `client/`
- Tap ingestion support is available and should be preferred for new ingestion work when applicable; Jetstream + backfill is the legacy path.

## Commands agents should prefer

### Backend

- `make build` — build backend binary to `bin/hyperindex`
- `make run` — build and run backend
- `make dev` — run backend with hot reload (`air` required)
- `make test` — run Go tests with `-race`
- `go test -v -run TestName ./...` — run a single Go test by name
- `go test -v ./path/to/package/...` — run one Go package
- `go test -v -race -tags=integration ./internal/integration/...` — run integration tests
- `make lint` — run `golangci-lint`
- `make fmt` — run `go fmt` + `gofumpt`

### Frontend

- `npm --prefix client run dev`
- `npm --prefix client run build`
- `npm --prefix client run lint`
- `npm --prefix client run test`

### Tooling

- `make tools` — install repo development tools
- `make hooks-install` — enable tracked git hooks
- `make changie-new` — create a Changie fragment

## Verification

Run verification based on what changed.

- If you changed **Go code**:
  - `go build -v ./...`
  - `make lint`
  - `DATABASE_URL=sqlite::memory: go test -v -race ./...`

- If you changed **database code, migrations, repositories, or dialect-specific behavior**:
  - also run:
    - `DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable go test -v -race ./...`

- If you changed **integration behavior**:
  - `go test -v -race -tags=integration ./internal/integration/...`

- If you changed **frontend code in `client/`**:
  - `npm --prefix client run lint`
  - `npm --prefix client run test`
  - `npm --prefix client run build`

## Testing notes

- CI runs Go tests against **both SQLite and PostgreSQL**.
- Integration tests are run with `-tags=integration`.
- Most DB-backed tests use `sqlite::memory:`.
- Prefer shared test helpers such as `internal/testutil/db.go` when adding DB-backed tests.

## Schema and migrations

- Runtime and tests use **embedded migrations** from:
  - `internal/database/migrations/sqlite/`
  - `internal/database/migrations/postgres/`
- These are embedded with `go:embed` and applied by the app at startup.
- Do **not** assume `db/migrations/` or `db/migrations_postgres/` is the canonical runtime migration path.
- If you change schema or migration behavior, verify both SQLite and PostgreSQL paths.

## Config and startup gotchas

- `ADMIN_API_KEY` is required at startup.
- `SECRET_KEY_BASE` must be at least 64 characters.
- `TAP_ENABLED=true` switches ingestion to Tap mode.
- Migrations run automatically on startup.
- Be careful with `ALLOWED_ORIGINS`: current code allows all origins when unset, even if older prose suggests stricter defaults.

## Changie fragments

- Release notes come from `.changes/unreleased/*.yaml`, not commit messages.
- If your change affects:
  - end users,
  - operators/deployers,
  - contributors,
  - or people forking/reusing this codebase,
  add a Changie fragment unless the change is docs-only or purely internal.
- This applies whether the change is in Go or frontend code.
- Good candidates include:
  - user-visible behavior changes
  - GraphQL/API changes
  - config or deployment changes
  - migration/runtime behavior changes
  - contributor workflow changes that matter to downstream users or forks
- Usually skip fragments for:
  - docs-only changes
  - tests-only changes
  - internal refactors with no externally meaningful behavior change
- Prefer `make changie-new`.
- When writing the fragment, use the local **`writing-changie`** skill.
- `Affects` must be one of:
  - `user`
  - `operator`
  - `developer`
- Maintainers should follow `docs/changelog-workflow.md` for the release execution runbook.

## Git hooks

- Run `make hooks-install` once to enable the tracked hooks in `.githooks/`.
- The pre-commit hook checks **staged Go files only**.
- It fails if staged Go files are not `gofmt`-formatted.
- It then runs `golangci-lint` on changed Go packages.
- The hook requires **Bash 4+**.
- `SKIP_GOLANGCI=1` is an emergency local bypass, not normal workflow.
