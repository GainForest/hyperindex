# API smoke tests

This directory contains a public, read-only, post-deploy API and GraphQL smoke suite for Hyperindex operators. Use it after deployment to verify that the public API endpoint is reachable and serving the expected read paths.

The suite does not test the Next.js client, admin authentication, lexicon upload or register flows, mutations, OAuth, or subscriptions.

## Run manually

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -tags=api_smoke ./tests/api-smoke -count=1
```

Direct `go test` runs use standard Go test output. Successful test stdout is only shown when you pass `-v`.

Use the Make target for operator-friendly smoke output, with the URL supplied by your environment. The target runs verbose tests internally so friendly progress lines are streamed incrementally, then filters Go harness noise such as `=== RUN`, `--- PASS`, package `PASS`, and final `ok` lines.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com make smoke-api
```

Do not bake an environment-specific URL into the command or Makefile target.

Set `HYPERINDEX_SMOKE_DEBUG=1` when you need compact lower-level request and response logs without enabling Go verbose subtest output. Debug logs stream incrementally alongside the friendly progress lines and include GraphQL operation names, variables, HTTP status, error counts, data byte lengths, and REST method, path, HTTP status, and response byte lengths.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_DEBUG=1 \
  make smoke-api
```

Developers who want Go test and subtest names can manually add `-v` to direct `go test` runs:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -v -tags=api_smoke ./tests/api-smoke -count=1
```

## Optional expectations file

Set `HYPERINDEX_SMOKE_EXPECTATIONS=/path/to/expectations.json` to provide environment-specific expectations for the smoke run.

## What the suite checks

- `/health`
- `/stats`
- GraphQL `__typename`
- Introspection query fields
- Generic records
- Data shape
- `collectionStats`
- Search
- Strict pagination
- Typed `ByUri` roundtrip

## Public-only limitation

Because this suite uses only the public GraphQL API, it cannot strictly prove that helper lexicons such as `app.certified.defs` are loaded. Strict lexicon identity would require a future admin-authenticated smoke mode.

## Production data assumptions

The target deployment must have enough public data for read-path checks. These collections must each contain at least 20 records:

- `org.hypercerts.claim.activity`
- `app.certified.actor.profile`
