# Load tests

This directory contains manual load-test tooling for Hyperindex. The tests are intentionally not part of CI and should only be run against environments where load testing is allowed.

## Read-only GraphQL load test

`k6/read-only.js` exercises public GraphQL read paths only. It does not publish PDS records, call admin mutations, or change indexed data.

Install k6 into the ignored local `bin/` directory if it is not already available:

```bash
GOBIN="$PWD/bin" go install go.k6.io/k6@latest
```

Default target:

```bash
BASE_URL=https://dev.api.indexer.hypercerts.dev \
  ./bin/k6 run tests/load/k6/read-only.js
```

Docker, without installing k6 locally:

```bash
BASE_URL=https://dev.api.indexer.hypercerts.dev

docker run --rm -i \
  -e BASE_URL="$BASE_URL" \
  grafana/k6 run - < tests/load/k6/read-only.js
```

Useful environment variables:

| Variable | Default | Description |
| --- | --- | --- |
| `BASE_URL` | `https://dev.api.indexer.hypercerts.dev` | Hyperindex API base URL. |
| `GRAPHQL_URL` | `$BASE_URL/graphql` | Full GraphQL endpoint URL. |
| `PROFILE` | `smoke` | Load profile: `smoke`, `baseline`, `spike`, or `soak`. |
| `THINK_TIME_SECONDS` | `1` | Sleep between each virtual-user iteration. |
| `OPERATION` | unset | Restrict the run to one operation, such as `activitySearch` or `genericRecordsMaxMetadata`. |
| `INCLUDE_EXPENSIVE` | unset | Set to `true` to include deliberately heavy operations such as `genericRecordsRawValueMax`. |
| `LOG_ERRORS` | unset | Set to `true` to print failed operation names and truncated responses. |

Profiles:

| Profile | Shape | Purpose |
| --- | --- | --- |
| `smoke` | 5 VUs for about 1 minute | Confirm the target and script are healthy. |
| `baseline` | 5 → 20 → 50 VUs over about 8 minutes | First meaningful read-capacity baseline. |
| `spike` | 100 VUs quickly for about 2 minutes | Short connection/concurrency spike. |
| `soak` | 25 VUs for about 30 minutes | Watch for slow degradation. |

The script fails if HTTP error rate reaches 1%, GraphQL error rate reaches 1%, or p95 request latency is at least 2 seconds.

## Write/indexing smoke test

`writers/pds-write-smoke.mjs` publishes dummy `org.hypercerts.claim.activity` records to a PDS, then polls Hyperindex until each published URI appears in the public typed GraphQL API.

Store credentials in the ignored local file:

```bash
cat > tests/load/.env.loadtest.local <<'EOF'
ATPROTO_PDS_URL=https://certified.one
ATPROTO_IDENTIFIER=your-handle-or-did
ATPROTO_PASSWORD=your-password-or-app-password
HYPERINDEX_BASE_URL=https://dev.api.indexer.hypercerts.dev
WRITE_COUNT=10
WRITE_CONCURRENCY=1
# certified.one currently needs validation disabled for this custom lexicon.
WRITE_VALIDATE=false
EOF
chmod 600 tests/load/.env.loadtest.local
```

Run a small write smoke:

```bash
WRITE_COUNT=3 WRITE_CONCURRENCY=1 \
  node tests/load/writers/pds-write-smoke.mjs
```

Useful write-test variables:

| Variable | Default | Description |
| --- | --- | --- |
| `LOADTEST_ENV_FILE` | `tests/load/.env.loadtest.local` | Env file to load before reading settings. |
| `ATPROTO_PDS_URL` | required | PDS URL that receives writes, e.g. `https://certified.one`. |
| `ATPROTO_IDENTIFIER` | required | Handle or DID for the writer account. |
| `ATPROTO_PASSWORD` | required | Password or app password for the writer account. |
| `ATPROTO_REPO` | session DID | Repo to write to. Usually leave unset. |
| `HYPERINDEX_BASE_URL` | `https://dev.api.indexer.hypercerts.dev` | Hyperindex API base URL used for `/stats` and GraphQL polling. |
| `HYPERINDEX_GRAPHQL_URL` | `$HYPERINDEX_BASE_URL/graphql` | Full GraphQL endpoint URL. |
| `WRITE_COUNT` | `10` | Number of dummy records to publish. |
| `WRITE_CONCURRENCY` | `1` | Concurrent publish-and-poll workers. |
| `WRITE_POLL_TIMEOUT_MS` | `120000` | Maximum time to wait for each record to appear. |
| `WRITE_POLL_INTERVAL_MS` | `1000` | Poll interval per record. |
| `WRITE_VALIDATE` | `false` | Pass ATProto record validation through to `createRecord`. Keep `false` when the PDS does not have the custom Hypercerts lexicon loaded. |
| `WRITE_DRY_RUN` | `false` | Build records and output a summary without publishing. |
| `WRITE_RESULTS_PATH` | `tmp/load-write-smoke-$RUN_ID.json` | JSON result file. |

## Update/indexing load test

`writers/pds-update-smoke.mjs` reads a write result file, fetches each current record from the PDS, updates the title and short description with a new run marker, then polls Hyperindex until the updated title appears in the public typed GraphQL API.

By default it uses the newest `tmp/load-write-smoke-*.json` or `tmp/mixed-write-*.json` result file.

Update the newest write batch:

```bash
UPDATE_CONCURRENCY=25 UPDATE_ALLOW_LARGE=true \
  node tests/load/writers/pds-update-smoke.mjs
```

Update a specific batch:

```bash
UPDATE_INPUT_FILE=tmp/load-write-smoke-20260518114955.json \
UPDATE_CONCURRENCY=100 \
UPDATE_ALLOW_LARGE=true \
  node tests/load/writers/pds-update-smoke.mjs
```

Useful update-test variables:

| Variable | Default | Description |
| --- | --- | --- |
| `UPDATE_INPUT_FILE` | newest write result | Result file containing URIs to update. |
| `UPDATE_TITLE_PREFIX` | `Hyperindex write smoke` | Safety filter for records loaded from the result file. |
| `UPDATE_INCLUDE_ANY_TITLE` | `false` | Set to `true` to disable the title-prefix safety filter. |
| `UPDATE_LIMIT` | `0` | Optional max number of discovered records to update. `0` means no limit. |
| `UPDATE_CONCURRENCY` | `25` | Concurrent get/put/poll workers. |
| `UPDATE_ALLOW_LARGE` | `false` | Required when updating more than 100 records. |
| `UPDATE_VALIDATE` | `false` | Pass ATProto record validation through to `putRecord`. Keep `false` when the PDS does not have the custom Hypercerts lexicon loaded. |
| `UPDATE_DRY_RUN` | `false` | Show/update-poll plan without authenticating or updating. |
| `UPDATE_RESULTS_PATH` | `tmp/load-update-smoke-$RUN_ID.json` | JSON result file. |

## Delete/indexing cleanup test

`writers/pds-delete-smoke.mjs` reads the JSON result files produced by the write smoke test, deletes those exact AT-URIs from the PDS, then polls Hyperindex until each URI disappears from the public typed GraphQL API.

By default it only deletes records whose saved title starts with `Hyperindex write smoke`, so it does not accidentally delete unrelated records from an arbitrary result file.

Dry-run the cleanup target set:

```bash
DELETE_DRY_RUN=true DELETE_CONCURRENCY=50 \
  node tests/load/writers/pds-delete-smoke.mjs
```

Delete all write-smoke records discovered under `tmp/`:

```bash
DELETE_CONCURRENCY=50 DELETE_ALLOW_LARGE=true \
  node tests/load/writers/pds-delete-smoke.mjs
```

Useful delete-test variables:

| Variable | Default | Description |
| --- | --- | --- |
| `DELETE_INPUT_GLOBS` | `tmp/load-write-smoke-*.json,tmp/mixed-write-*.json` | Comma-separated result-file globs to load URIs from. |
| `DELETE_TITLE_PREFIX` | `Hyperindex write smoke` | Safety filter for records loaded from result files. |
| `DELETE_INCLUDE_ANY_TITLE` | `false` | Set to `true` to disable the title-prefix safety filter. |
| `DELETE_LIMIT` | `0` | Optional max number of discovered records to delete. `0` means no limit. |
| `DELETE_CONCURRENCY` | `25` | Concurrent delete-and-poll workers. |
| `DELETE_ALLOW_LARGE` | `false` | Required when deleting more than 100 records. |
| `DELETE_DRY_RUN` | `false` | Show/delete-poll plan without authenticating or deleting. |
| `DELETE_RESULTS_PATH` | `tmp/load-delete-smoke-$RUN_ID.json` | JSON result file. |

## Safety notes

- Start with `PROFILE=smoke` before larger read profiles.
- Start with `WRITE_COUNT=1` or `WRITE_COUNT=3` before larger write tests.
- Dry-run delete cleanup before deleting more than a handful of records.
- Watch `/health`, `/stats`, application logs, database metrics, and Tap errors during the run.
- Stop the test if latency, error rate, or Tap errors rise unexpectedly.
- Do not commit `tests/load/.env.loadtest.local` or any PDS credentials.
