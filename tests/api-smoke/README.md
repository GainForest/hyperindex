# API smoke tests

This directory contains a post-deploy API and GraphQL smoke suite for Hyperindex operators. By default it is public and read-only; an opt-in write-through check can also create, update, and delete disposable ATProto records to verify ingestion end to end.

The default suite does not test the Next.js client, admin authentication, lexicon upload or register flows, mutations, OAuth, subscriptions, or writes. The opt-in write-through check writes through an ATProto PDS, not Hyperindex mutations.

## Run manually

`HYPERINDEX_SMOKE_URL` is required unless it is provided by the smoke `.env` file. It must point to the public Hyperindex API endpoint you want to check.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -tags=api_smoke ./tests/api-smoke -count=1
```

Direct `go test` runs use standard Go test output. Successful test stdout is only shown when you pass `-v`.

Use the Make target for operator-friendly smoke output, with the URL supplied by your environment or `tests/api-smoke/.env`. The target runs verbose tests internally so friendly progress lines are streamed incrementally, then filters Go harness noise such as `=== RUN`, `--- PASS`, package `PASS`, and final `ok` lines.

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com make smoke-api
```

If `HYPERINDEX_SMOKE_URL` is unset in both the environment and the smoke `.env` file, `make smoke-api` fails with the test suite's config error. If a smoke check fails, the target preserves the failure output, exits non-zero, and does not print the final `✓ API smoke checks passed` line. That success message is printed only after all checks pass.

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

## Optional smoke `.env` file

By default, the suite loads `tests/api-smoke/.env` if the file exists. Copy `tests/api-smoke/.env.example` to `tests/api-smoke/.env` for local or staging smoke settings. Set `HYPERINDEX_SMOKE_ENV_FILE=/path/to/.env` to load a different file. Values already present in the process environment take precedence over values from the file.

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
- Activity claim external label querying, value filtering, and pagination
- Typed `ByUri` roundtrip
- `app.certified.graph.follow` typed pagination, filters, and sorting
- Optional external label filtering and pagination for `org.hypercerts.claim.activity`
- Optional ATProto write-through lifecycle for `app.certified.actor.profile` and `org.hypercerts.claim.activity`

## Optional external label smoke check

The default expectations file requires at least 20 `org.hypercerts.claim.activity` records labeled `high-quality` and at least 20 labeled `standard` by the configured external label source. The label smoke checks also expect at least four activity claim records labeled `likely-test` from that same configured source. The tests query the typed activity claim collection with `where.externalLabels`, verify pagination, check each returned node exposes the matching `externalLabels` entry, and cross-check one URI through the root `externalLabels` query.

Set the source DID to enable this check:

```bash
HYPERINDEX_SMOKE_EXTERNAL_LABEL_SOURCE_DID=did:plc:example \
  HYPERINDEX_SMOKE_URL=https://api.example.com \
  make smoke-api
```

If `HYPERINDEX_SMOKE_EXTERNAL_LABEL_SOURCE_DID` is unset, the external label smoke test is skipped. Environment-specific expectations can override `externalLabelActivityClaims` in the expectations JSON to change the source DID env var name, page size, label values, or minimum record counts.

## Optional write-through smoke check

Set `HYPERINDEX_SMOKE_WRITE_THROUGH=1` to enable an end-to-end ingestion check. The test logs in to an ATProto PDS, creates `app.certified.actor.profile/self`, waits for Hyperindex to expose it, updates it and verifies the new CID/fields, then creates, updates, and deletes an `org.hypercerts.claim.activity` record and finally deletes the profile. Each create/update/delete verification logs the observed ingestion time and poll count.

Required write-through settings:

- `HYPERINDEX_SMOKE_ATPROTO_PDS_URL` — ATProto PDS base URL, for example `https://bsky.social`
- `HYPERINDEX_SMOKE_ATPROTO_IDENTIFIER` — handle or DID for a disposable smoke account
- `HYPERINDEX_SMOKE_ATPROTO_PASSWORD` — app password or account password for that smoke account

Optional timing settings:

- `HYPERINDEX_SMOKE_WRITE_POLL_TIMEOUT` — per-step indexing timeout, default `60s`
- `HYPERINDEX_SMOKE_WRITE_POLL_INTERVAL` — polling interval, default `2s`

Use a dedicated disposable account. If `app.certified.actor.profile/self` already exists, the test temporarily deletes it so it can exercise create semantics and restores the original record during cleanup. The exception is stale smoke data: if the existing record already looks like a previous smoke run, the test removes it and does not restore it. A record is treated as stale smoke data when its `displayName`, `description`, `title`, or `shortDescription` contains `Hyperindex write-through smoke test`, when its `displayName` starts with `Hyperindex Smoke Profile`, or when its `title` starts with `Hyperindex smoke activity`.

## Public API limitation

Because the suite verifies Hyperindex through the public GraphQL API, it cannot strictly prove that helper and non-record lexicons are loaded. Strict lexicon identity would require a future admin-authenticated smoke mode.

Public typed GraphQL collection and `ByUri` fields are generated from the lexicons available when the backend starts. After changing which lexicons the backend loads, or after updating smoke expectations for newly loaded lexicons, restart or redeploy the API before expecting schema checks for those typed fields to pass.

These helper and non-record lexicons are excluded from typed field assertions because the public GraphQL schema should not expose typed collection or `ByUri` query fields for them:

- `app.certified.defs`
- `org.hypercerts.defs`
- `org.hypercerts.workscope.cel`

The default typed expectations also intentionally omit `app.certified.link.evm`. The development schema does not expose `appCertifiedLinkEvm` or `appCertifiedLinkEvmByUri`, so the default `tests/api-smoke/expectations.json` does not require that NSID as a typed public GraphQL collection.

## Production data assumptions

The target deployment must have enough public data for read-path checks. These collections must each contain at least 20 records:

The label smoke checks also assume `org.hypercerts.claim.activity` has active external labels from the source DID configured by `HYPERINDEX_SMOKE_EXTERNAL_LABEL_SOURCE_DID` or the expectation file's `externalLabelActivityClaims.sourceDIDEnv` setting.
- `org.hypercerts.claim.activity`
- `app.certified.actor.profile`
- `app.certified.graph.follow`
