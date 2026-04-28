# Contributing to Hyperindex

Thanks for your interest in contributing to Hyperindex.

This repository contains two codebases:

- a **Go backend** at the repository root
- a **Next.js frontend** in `client/`

You may still see the older **`hypergoat`** name in Go module paths, binaries, and entrypoint directories. The product name is **Hyperindex**.

For product usage and deployment basics, start with [`README.md`](./README.md). This document focuses on contributor workflow.

## Repository map

- Backend entrypoint: `cmd/hypergoat/`
- Backend schema and migrations: `internal/database/migrations/`
- Frontend app: `client/`
- Changelog workflow: `docs/changelog-workflow.md`

## Prerequisites

Before contributing, make sure you have:

- **Go 1.26**
- **Node.js and npm** for frontend work
- **Bash 4+** if you want to use the tracked git hooks
- **Docker** if you want to use local Compose-based workflows
- **PostgreSQL** if you need to verify Postgres-specific database behavior

Install repository tooling with:

```bash
make tools
```

## Local setup

### Backend

```bash
cp .env.example .env
make run
```

For development with hot reload:

```bash
make dev
```

### Frontend

```bash
npm --prefix client install
npm --prefix client run dev
```

### Full stack

Run the backend from the repo root and the frontend from `client/` in separate terminals.

If you are working on ingestion behavior, prefer **Tap**-based workflows when applicable. Jetstream + backfill is the legacy path.

## Environment notes

A few settings commonly trip people up:

- `ADMIN_API_KEY` is required at startup
- `SECRET_KEY_BASE` must be at least 64 characters
- `TAP_ENABLED=true` switches ingestion to Tap mode
- migrations run automatically on startup
- when unset, `ALLOWED_ORIGINS` currently allows all origins

Use `.env.example` and any frontend env examples as your starting point.

## Common commands

### Backend

| Command | Purpose |
|---|---|
| `make build` | build backend binary |
| `make run` | build and run backend |
| `make dev` | run backend with hot reload |
| `make test` | run Go tests with race detection |
| `make lint` | run `golangci-lint` |
| `make fmt` | run `go fmt` and `gofumpt` |

Useful targeted test commands:

```bash
go test -v -run TestName ./...
go test -v ./path/to/package/...
go test -v -race -tags=integration ./internal/integration/...
```

### Frontend

| Command | Purpose |
|---|---|
| `npm --prefix client run dev` | run frontend locally |
| `npm --prefix client run lint` | lint frontend |
| `npm --prefix client run test` | run frontend tests |
| `npm --prefix client run build` | build frontend |

### Hooks

Install the tracked git hooks once:

```bash
make hooks-install
```

The pre-commit hook:

- runs on **staged Go files only**
- checks that staged Go files are properly formatted
- runs `golangci-lint` on changed Go packages

## Choosing verification

Run verification that matches the kind of change you made.

### If you changed Go code

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

### If you changed database code, migrations, repositories, or dialect-specific behavior

Also run:

```bash
DATABASE_URL=postgres://hypergoat:hypergoat@localhost:5432/hypergoat_test?sslmode=disable go test -v -race ./...
```

### If you changed integration behavior

```bash
go test -v -race -tags=integration ./internal/integration/...
```

### If you changed frontend code in `client/`

```bash
npm --prefix client run lint
npm --prefix client run test
npm --prefix client run build
```

CI runs Go tests against both SQLite and PostgreSQL, so database-related changes should be verified against both where relevant.

## Database and migration notes

Hyperindex uses **embedded migrations** at runtime and in tests.

The source-of-truth migration directories are:

- `internal/database/migrations/sqlite/`
- `internal/database/migrations/postgres/`

Do not assume other migration directories are the canonical runtime path.

If you change schema or migration behavior, verify both SQLite and PostgreSQL paths.

Most DB-backed tests use `sqlite::memory:`. Prefer shared helpers such as `internal/testutil/db.go` when adding DB-backed tests.

## Code conventions

- keep changes focused and minimal
- follow existing package and file structure
- match naming and style conventions in nearby code
- reuse shared helpers and existing test patterns where possible

If you are doing Go-heavy work, `docs/agents-go-reference.md` may also be useful as a repo-specific reference.

## Changelog fragments

This repository uses **Changie** for release notes.

Release notes come from:

- `.changes/unreleased/*.yaml`

Add a changelog fragment for changes that affect:

- end users
- operators or deployers
- contributors or downstream developers

You can usually skip a fragment for:

- docs-only changes
- tests-only changes
- internal refactors with no meaningful external impact

Create a fragment with:

```bash
make changie-new
```

For more detail, see [`docs/changelog-workflow.md`](./docs/changelog-workflow.md).

## Pull requests

When opening a pull request:

- describe the change and why it was made
- list the verification you ran
- note any config, schema, migration, or API impacts
- include screenshots or recordings for frontend/UI changes
- mention any follow-up work or known limitations

Small, focused PRs are much easier to review than large mixed changes.

## Common pitfalls

A few frequent sources of confusion:

- Hyperindex is the product name, but `hypergoat` still appears in code paths and binaries
- Docker and PostgreSQL are useful, but not required for every contribution
- the tracked git hooks require **Bash 4+**
- DB-related changes should usually be tested against both SQLite and PostgreSQL
- Tap is the preferred ingestion path for new ingestion work

## Reporting issues and asking questions

If you found a bug or have a feature idea, open an issue with as much context as possible.

For questions during development, prefer linking the relevant code path, failing test, or workflow so others can reproduce the issue quickly.

## Security

If you discover a security issue, avoid posting sensitive details publicly in a GitHub issue. Report it privately through the maintainers’ preferred security contact path if one is available.

## Thanks

We appreciate contributions of all sizes, including bug fixes, docs improvements, tests, and feature work.
