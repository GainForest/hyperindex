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
- Tap ingestion support is available and should be preferred for new record ingestion work when applicable; Jetstream + backfill is the legacy record path.
- External ATProto labeler ingestion lives in `internal/labeler/` and stores raw label events in dedicated external label tables; labels are exposed through public GraphQL query and record fields.

## Commands agents should prefer

### Backend

- `make build` — build backend binary to `bin/hyperindex`
- `make run` — build and run backend
- `make dev` — run backend with hot reload (`air` required)
- `make test` — run Go tests with `-race`
- `make smoke-tap-local` — run a full isolated local Tap Docker stack and API smoke tests using `app.certified.actor.profile` as the Tap signal collection and `app.certified.*,org.hypercerts.*` as Tap collection filters
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
- `TAP_ENABLED=true` switches record ingestion to Tap mode.
- `LABELER_SUBSCRIBE_ENABLED=true` with `LABELER_SUBSCRIBE_URLS` starts optional external `com.atproto.label.subscribeLabels` ingestion.
- Migrations run automatically on startup.
- CORS origin config is split by route group: `PUBLIC_ALLOWED_ORIGINS` controls public GraphQL/OAuth browser access and defaults to `*`; `ADMIN_ALLOWED_ORIGINS` must list trusted admin frontend origins explicitly and rejects wildcard `*`. Deprecated `ALLOWED_ORIGINS` is only a compatibility fallback for explicit admin origins.

## Documentation and agent skills

- When public GraphQL behavior, hosted endpoint guidance, production schema, consumer examples, or API docs change, update the local Hyperindex skill too:
  - `.agents/skills/hyperindex/SKILL.md`
  - `.agents/skills/hyperindex/references/schema-reference.md`

## Changie fragments

`docs/changelog-workflow.md` is the source of truth for when to add or skip Changie fragments, how to write them, and how maintainers run releases. Read it before adding or intentionally skipping a fragment.

- Release notes are produced from `.changes/unreleased/*.yaml`, not commit history.
- Prefer `make changie-new` when creating a fragment.
- Follow `docs/changelog-workflow.md` directly; there is no separate Changie skill.

## Keeping this file current

- Update `AGENTS.md` whenever changes affect repository structure, entrypoints, required commands, verification steps, config requirements, migration behavior, release-note workflow, or agent/developer operating instructions.
- Do not leave stale guidance in this file; update or remove obsolete instructions in the same change that makes them obsolete.

## Git hooks

- Run `make hooks-install` once to enable the tracked hooks in `.githooks/`.
- The pre-commit hook checks **staged Go files only**.
- It fails if staged Go files are not `gofmt`-formatted.
- It then runs `golangci-lint` on changed Go packages.
- The hook requires **Bash 4+**.
- `SKIP_GOLANGCI=1` is an emergency local bypass, not normal workflow.
