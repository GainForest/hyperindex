# API smoke tests

This directory contains a public, read-only, post-deploy API and GraphQL smoke suite for Hyperindex operators. Use it after deployment to verify that the public API endpoint is reachable and serving the expected read paths.

The suite does not test the Next.js client, admin authentication, lexicon upload or register flows, mutations, OAuth, or subscriptions.

## Run manually

`HYPERINDEX_SMOKE_URL` is required for both direct `go test` runs and the Make target. It must point to the public Hyperindex API endpoint you want to check.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -tags=api_smoke ./tests/api-smoke -count=1
```

Direct `go test` runs use standard Go test output. Successful test stdout is only shown when you pass `-v`.

Use the Make target for operator-friendly smoke output, with the URL supplied by your environment. The target runs verbose tests internally so friendly progress lines are streamed incrementally, then filters Go harness noise such as `=== RUN`, `--- PASS`, package `PASS`, and final `ok` lines.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com make smoke-api
```

`make smoke-api` fails before running tests when `HYPERINDEX_SMOKE_URL` is unset. If a smoke check fails, the target preserves the failure output, exits non-zero, and does not print the final `✓ API smoke checks passed` line. That success message is printed only after all checks pass.

Do not bake an environment-specific URL into the command or Makefile target.

Set `HYPERINDEX_SMOKE_DEBUG=1` when you need compact lower-level request and response logs without enabling Go verbose subtest output. Debug logs stream incrementally alongside the friendly progress lines and include GraphQL operation names, variables, HTTP status, error counts, data byte lengths, and REST method, path, HTTP status, and response byte lengths.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_DEBUG=1 \
  make smoke-api
```

Set `HYPERINDEX_SMOKE_AUDIT=1` to opt into the Tap append-only audit smoke checks. The audit smoke queries `auditRecordEvents`, requires at least 5 events by default, verifies total counts, cursor pagination, supported filters, decoded record bodies, and optional lifecycle behavior when update/delete rows are present. Override the required count with `HYPERINDEX_SMOKE_AUDIT_MIN_EVENTS` when an environment should prove more audit history is present.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  HYPERINDEX_SMOKE_AUDIT=1 \
  HYPERINDEX_SMOKE_AUDIT_MIN_EVENTS=10 \
  make smoke-api
```

Developers who want Go test and subtest names can manually add `-v` to direct `go test` runs:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -v -tags=api_smoke ./tests/api-smoke -count=1
```

## Optional expectations file

By default, the suite loads `tests/api-smoke/expectations.json`. Set `HYPERINDEX_SMOKE_EXPECTATIONS=/path/to/expectations.json` to provide environment-specific expectations for the smoke run.

The expectations file is read, decoded, and validated before requests are sent. Expectation load failures include the file path so operators can see which file failed; for example, a missing override reports `read expectations file "/path/to/expectations.json": no such file or directory`.

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
- Optional append-only `auditRecordEvents` shape, `totalCount`, cursor pagination, filters, decoded records, and sampled lifecycle behavior when `HYPERINDEX_SMOKE_AUDIT=1`

## Public-only limitation

Because this suite uses only the public GraphQL API, it cannot strictly prove that helper and non-record lexicons are loaded. Strict lexicon identity would require a future admin-authenticated smoke mode.

Public typed GraphQL collection and `ByUri` fields are generated from the lexicons available when the backend starts. After changing which lexicons the backend loads, or after updating smoke expectations for newly loaded lexicons, restart or redeploy the API before expecting schema checks for those typed fields to pass.

These helper and non-record lexicons are excluded from typed field assertions because the public GraphQL schema should not expose typed collection or `ByUri` query fields for them:

- `app.certified.defs`
- `org.hypercerts.defs`
- `org.hypercerts.workscope.cel`

The default typed expectations also intentionally omit `app.certified.link.evm`. The development schema does not expose `appCertifiedLinkEvm` or `appCertifiedLinkEvmByUri`, so the default `tests/api-smoke/expectations.json` does not require that NSID as a typed public GraphQL collection.

## Production data assumptions

The target deployment must have enough public data for read-path checks. These collections must each contain at least 20 records:

- `org.hypercerts.claim.activity`
- `app.certified.actor.profile`

When `HYPERINDEX_SMOKE_AUDIT=1`, the target deployment must also run with `TAP_ENABLED=true` and `AUDIT_ENABLED=true`, and it must have at least `HYPERINDEX_SMOKE_AUDIT_MIN_EVENTS` audit rows. If the minimum is unset, the smoke suite requires 5 audit events. The append-only E2E checks need at least 2 audit rows so cursor pagination can be exercised.
