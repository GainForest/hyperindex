#!/usr/bin/env node

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);
const defaultEnvPath = path.resolve(__dirname, '..', '.env.loadtest.local');

loadEnv(process.env.LOADTEST_ENV_FILE || defaultEnvPath);

const pdsURL = requiredEnv('ATPROTO_PDS_URL').replace(/\/$/, '');
const identifier = requiredEnv('ATPROTO_IDENTIFIER');
const password = requiredEnv('ATPROTO_PASSWORD');
const hyperindexBaseURL = env('HYPERINDEX_BASE_URL', 'https://dev.api.indexer.hypercerts.dev').replace(/\/$/, '');
const graphqlURL = env('HYPERINDEX_GRAPHQL_URL', `${hyperindexBaseURL}/graphql`);
const collection = env('WRITE_COLLECTION', 'org.hypercerts.claim.activity');
const count = intEnv('WRITE_COUNT', 10);
const concurrency = intEnv('WRITE_CONCURRENCY', 1);
const pollTimeoutMs = intEnv('WRITE_POLL_TIMEOUT_MS', 120_000);
const pollIntervalMs = intEnv('WRITE_POLL_INTERVAL_MS', 1_000);
const requestTimeoutMs = intEnv('WRITE_REQUEST_TIMEOUT_MS', 15_000);
const validate = boolEnv('WRITE_VALIDATE', false);
const dryRun = boolEnv('WRITE_DRY_RUN', false);
const allowLarge = boolEnv('WRITE_ALLOW_LARGE', false);
const runID = env('WRITE_RUN_ID', makeRunID());
const resultsPath = env('WRITE_RESULTS_PATH', path.join('tmp', `load-write-smoke-${runID}.json`));

if (collection !== 'org.hypercerts.claim.activity') {
  throw new Error(`WRITE_COLLECTION=${collection} is not supported yet; this smoke script currently polls orgHypercertsClaimActivityByUri`);
}
if (count <= 0) {
  throw new Error('WRITE_COUNT must be greater than 0');
}
if (concurrency <= 0) {
  throw new Error('WRITE_CONCURRENCY must be greater than 0');
}
if (count > 100 && !allowLarge) {
  throw new Error('WRITE_COUNT > 100 requires WRITE_ALLOW_LARGE=true');
}

console.log('Hyperindex PDS write smoke');
console.log(`  PDS:        ${pdsURL}`);
console.log(`  GraphQL:    ${graphqlURL}`);
console.log(`  collection: ${collection}`);
console.log(`  run:        ${runID}`);
console.log(`  count:      ${count}`);
console.log(`  concurrency:${concurrency}`);
console.log(`  validate:   ${validate}`);
console.log(`  dry run:    ${dryRun}`);
console.log('');

const startedAt = new Date().toISOString();
const beforeStats = await getStats('before');

let session = null;
let repo = env('ATPROTO_REPO', '');
if (!dryRun) {
  session = await createSession();
  repo = repo || session.did;
  console.log(`Authenticated as ${session.handle || identifier} (${session.did})`);
} else {
  repo = repo || 'did:example:dryrun';
  console.log('Dry run enabled; not authenticating or publishing.');
}

const results = [];
const failures = [];
let nextIndex = 0;

await Promise.all(
  Array.from({ length: concurrency }, async (_unused, workerIndex) => {
    while (true) {
      const index = nextIndex + 1;
      nextIndex += 1;
      if (index > count) {
        return;
      }
      try {
        const result = await publishAndPoll(index, workerIndex + 1, session?.accessJwt, repo);
        results.push(result);
        const latency = result.indexLatencyMs == null ? 'missing' : formatMs(result.indexLatencyMs);
        console.log(`✓ #${index} published ${shortURI(result.uri)} indexed=${latency} attempts=${result.pollAttempts}`);
      } catch (err) {
        const failure = { index, error: err.message || String(err) };
        failures.push(failure);
        console.error(`✗ #${index} failed: ${failure.error}`);
      }
    }
  }),
);

const afterStats = await getStats('after');
const finishedAt = new Date().toISOString();
const indexedResults = results.filter((result) => result.indexLatencyMs != null);
const missingResults = results.filter((result) => result.indexLatencyMs == null);
const latencies = indexedResults.map((result) => result.indexLatencyMs).sort((a, b) => a - b);

const summary = {
  runID,
  startedAt,
  finishedAt,
  pdsURL,
  hyperindexBaseURL,
  graphqlURL,
  collection,
  repo,
  count,
  concurrency,
  validate,
  dryRun,
  published: results.length,
  indexed: indexedResults.length,
  missing: missingResults.length,
  failures: failures.length,
  latencyMs: {
    min: percentile(latencies, 0),
    p50: percentile(latencies, 50),
    p90: percentile(latencies, 90),
    p95: percentile(latencies, 95),
    max: percentile(latencies, 100),
  },
  stats: {
    before: beforeStats,
    after: afterStats,
  },
  results: results.sort((a, b) => a.index - b.index),
  failureDetails: failures,
};

fs.mkdirSync(path.dirname(resultsPath), { recursive: true });
fs.writeFileSync(resultsPath, `${JSON.stringify(summary, null, 2)}\n`);

console.log('');
console.log('Summary');
console.log(`  published: ${summary.published}/${count}`);
console.log(`  indexed:   ${summary.indexed}/${summary.published}`);
console.log(`  failures:  ${summary.failures}`);
console.log(`  latency:   p50=${formatMs(summary.latencyMs.p50)} p90=${formatMs(summary.latencyMs.p90)} p95=${formatMs(summary.latencyMs.p95)} max=${formatMs(summary.latencyMs.max)}`);
if (beforeStats?.tap || afterStats?.tap) {
  console.log(`  tap errors before/after: ${beforeStats?.tap?.errors ?? 'n/a'} -> ${afterStats?.tap?.errors ?? 'n/a'}`);
}
console.log(`  results:   ${resultsPath}`);

if (failures.length > 0 || missingResults.length > 0) {
  process.exitCode = 1;
}

async function publishAndPoll(index, worker, accessJwt, repo) {
  const record = buildActivityRecord(index, worker);
  const publishStartedAt = Date.now();

  let uri;
  let cid;
  if (dryRun) {
    uri = `at://${repo}/${collection}/dry-${runID}-${index}`;
    cid = `dry-cid-${index}`;
  } else {
    const created = await xrpc('com.atproto.repo.createRecord', {
      repo,
      collection,
      validate,
      record,
    }, accessJwt);
    uri = created.uri;
    cid = created.cid;
  }

  const publishedAt = Date.now();
  const indexed = dryRun
    ? { found: true, attempts: 0, firstSeenAt: publishedAt }
    : await pollIndexed(uri, record.title, publishedAt);

  return {
    index,
    worker,
    uri,
    cid,
    title: record.title,
    createdAt: record.createdAt,
    publishLatencyMs: publishedAt - publishStartedAt,
    publishedAt: new Date(publishedAt).toISOString(),
    indexedAt: indexed.firstSeenAt ? new Date(indexed.firstSeenAt).toISOString() : null,
    indexLatencyMs: indexed.firstSeenAt ? indexed.firstSeenAt - publishedAt : null,
    pollAttempts: indexed.attempts,
    found: indexed.found,
  };
}

function buildActivityRecord(index, worker) {
  const now = new Date().toISOString();
  const padded = String(index).padStart(4, '0');
  return {
    $type: 'org.hypercerts.claim.activity',
    title: `Hyperindex write smoke ${runID} #${padded}`,
    shortDescription: `Dummy Hyperindex write/indexing smoke record. run=${runID} index=${index} worker=${worker}. Safe to ignore.`,
    createdAt: now,
    contributors: [
      {
        contributorIdentity: {
          $type: 'org.hypercerts.claim.activity#contributorIdentity',
          identity: 'Hyperindex load-test writer',
        },
        contributionWeight: '1',
        contributionDetails: {
          $type: 'org.hypercerts.claim.activity#contributorRole',
          role: 'Synthetic write/indexing smoke test',
        },
      },
    ],
    workScope: {
      $type: 'org.hypercerts.claim.activity#workScopeString',
      scope: 'Hyperindex load testing',
    },
  };
}

async function createSession() {
  return xrpc('com.atproto.server.createSession', {
    identifier,
    password,
  });
}

async function xrpc(method, body, accessJwt) {
  const headers = { 'Content-Type': 'application/json' };
  if (accessJwt) {
    headers.Authorization = `Bearer ${accessJwt}`;
  }
  const res = await fetchWithTimeout(`${pdsURL}/xrpc/${method}`, {
    method: 'POST',
    headers,
    body: JSON.stringify(body),
  });
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`${method} returned HTTP ${res.status}: ${text.slice(0, 500)}`);
  }
  return text ? JSON.parse(text) : {};
}

async function pollIndexed(uri, expectedTitle, publishedAt) {
  const deadline = publishedAt + pollTimeoutMs;
  let attempts = 0;
  let lastError = '';

  while (Date.now() < deadline) {
    attempts += 1;
    try {
      const body = await graphql(
        `query ActivityByUri($uri: String!) {
          orgHypercertsClaimActivityByUri(uri: $uri) {
            uri
            title
            createdAt
          }
        }`,
        { uri },
      );
      const node = body.data?.orgHypercertsClaimActivityByUri;
      if (node?.uri === uri) {
        if (expectedTitle && node.title !== expectedTitle) {
          lastError = `found URI but title differed: ${node.title}`;
        } else {
          return { found: true, attempts, firstSeenAt: Date.now() };
        }
      }
    } catch (err) {
      lastError = err.message || String(err);
    }
    await sleep(pollIntervalMs);
  }

  return { found: false, attempts, firstSeenAt: null, lastError };
}

async function graphql(query, variables) {
  const res = await fetchWithTimeout(graphqlURL, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ query, variables }),
  });
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`GraphQL HTTP ${res.status}: ${text.slice(0, 500)}`);
  }
  const body = JSON.parse(text);
  if (body.errors?.length) {
    throw new Error(`GraphQL errors: ${JSON.stringify(body.errors).slice(0, 500)}`);
  }
  return body;
}

async function getStats(label) {
  try {
    const res = await fetchWithTimeout(`${hyperindexBaseURL}/stats`, { method: 'GET' });
    if (!res.ok) {
      console.warn(`${label} stats returned HTTP ${res.status}`);
      return null;
    }
    const stats = await res.json();
    console.log(`${label} stats: records=${stats.records ?? 'n/a'} tap_errors=${stats.tap?.errors ?? 'n/a'} tap_sidecar=${stats.tap?.sidecar ?? 'n/a'}`);
    return stats;
  } catch (err) {
    console.warn(`${label} stats failed: ${err.message || err}`);
    return null;
  }
}

async function fetchWithTimeout(url, options = {}) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), requestTimeoutMs);
  try {
    return await fetch(url, { ...options, signal: controller.signal });
  } finally {
    clearTimeout(timeout);
  }
}

function loadEnv(filePath) {
  if (!filePath || !fs.existsSync(filePath)) {
    return;
  }
  const raw = fs.readFileSync(filePath, 'utf8');
  for (const line of raw.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }
    const eq = trimmed.indexOf('=');
    if (eq === -1) {
      continue;
    }
    const key = trimmed.slice(0, eq).trim();
    let value = trimmed.slice(eq + 1).trim();
    if ((value.startsWith('"') && value.endsWith('"')) || (value.startsWith("'") && value.endsWith("'"))) {
      value = value.slice(1, -1);
    }
    if (!process.env[key]) {
      process.env[key] = value;
    }
  }
}

function env(key, fallback) {
  const value = process.env[key];
  return value == null || value === '' ? fallback : value;
}

function requiredEnv(key) {
  const value = env(key, '');
  if (!value) {
    throw new Error(`Missing required env ${key}. Add it to ${defaultEnvPath} or export it.`);
  }
  return value;
}

function intEnv(key, fallback) {
  const raw = env(key, String(fallback));
  const value = Number.parseInt(raw, 10);
  if (!Number.isFinite(value)) {
    throw new Error(`${key} must be an integer, got ${raw}`);
  }
  return value;
}

function boolEnv(key, fallback) {
  const raw = env(key, fallback ? 'true' : 'false').toLowerCase();
  return ['1', 'true', 'yes', 'y', 'on'].includes(raw);
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function makeRunID() {
  return new Date().toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
}

function percentile(sortedValues, p) {
  if (!sortedValues.length) {
    return null;
  }
  if (p <= 0) {
    return sortedValues[0];
  }
  if (p >= 100) {
    return sortedValues[sortedValues.length - 1];
  }
  const idx = ((sortedValues.length - 1) * p) / 100;
  const lo = Math.floor(idx);
  const hi = Math.min(lo + 1, sortedValues.length - 1);
  const frac = idx - lo;
  return sortedValues[lo] * (1 - frac) + sortedValues[hi] * frac;
}

function formatMs(value) {
  if (value == null) {
    return 'n/a';
  }
  if (value >= 1000) {
    return `${(value / 1000).toFixed(2)}s`;
  }
  return `${Math.round(value)}ms`;
}

function shortURI(uri) {
  if (!uri) {
    return 'n/a';
  }
  const parts = uri.split('/');
  return parts.length > 2 ? `${parts.at(-2)}/${parts.at(-1)}` : uri;
}
