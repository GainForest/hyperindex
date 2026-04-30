# API smoke tests

This directory contains a public, read-only, post-deploy API and GraphQL smoke suite for Hyperindex operators. Use it after deployment to verify that the public API endpoint is reachable and serving the expected read paths.

The suite does not test the Next.js client, admin authentication, lexicon upload or register flows, mutations, OAuth, or subscriptions.

## Run manually

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com \
  go test -tags=api_smoke ./tests/api-smoke -count=1
```

Or use the Make target, with the URL supplied by your environment:

```bash
HYPERINDEX_SMOKE_URL=https://api.example.com make smoke-api
```

Do not bake an environment-specific URL into the command or Makefile target.

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
