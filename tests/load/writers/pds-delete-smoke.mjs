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
const inputGlobs = env('DELETE_INPUT_GLOBS', 'tmp/load-write-smoke-*.json,tmp/mixed-write-*.json')
  .split(',')
  .map((value) => value.trim())
  .filter(Boolean);
const titlePrefix = env('DELETE_TITLE_PREFIX', 'Hyperindex write smoke');
const includeAnyTitle = boolEnv('DELETE_INCLUDE_ANY_TITLE', false);
const limit = intEnv('DELETE_LIMIT', 0);
const concurrency = intEnv('DELETE_CONCURRENCY', 25);
const pollTimeoutMs = intEnv('DELETE_POLL_TIMEOUT_MS', 120_000);
const pollIntervalMs = intEnv('DELETE_POLL_INTERVAL_MS', 1_000);
const requestTimeoutMs = intEnv('DELETE_REQUEST_TIMEOUT_MS', 15_000);
const dryRun = boolEnv('DELETE_DRY_RUN', false);
const allowLarge = boolEnv('DELETE_ALLOW_LARGE', false);
const runID = env('DELETE_RUN_ID', makeRunID());
const resultsPath = env('DELETE_RESULTS_PATH', path.join('tmp', `load-delete-smoke-${runID}.json`));

if (concurrency <= 0) {
  throw new Error('DELETE_CONCURRENCY must be greater than 0');
}

const records = discoverRecords();
if (limit > 0) {
  records.splice(limit);
}
if (records.length > 100 && !allowLarge && !dryRun) {
  throw new Error(`Deleting ${records.length} records requires DELETE_ALLOW_LARGE=true`);
}

console.log('Hyperindex PDS delete smoke');
console.log(`  PDS:          ${pdsURL}`);
console.log(`  GraphQL:      ${graphqlURL}`);
console.log(`  input globs:  ${inputGlobs.join(', ')}`);
console.log(`  title prefix: ${includeAnyTitle ? '(any title allowed)' : titlePrefix}`);
console.log(`  run:          ${runID}`);
console.log(`  records:      ${records.length}`);
console.log(`  concurrency:  ${concurrency}`);
console.log(`  dry run:      ${dryRun}`);
console.log('');

if (records.length === 0) {
  console.log('No matching records to delete.');
  process.exit(0);
}

const startedAt = new Date().toISOString();
const beforeStats = await getStats('before');

let session = null;
if (!dryRun) {
  session = await createSession();
  console.log(`Authenticated as ${session.handle || identifier} (${session.did})`);
} else {
  console.log('Dry run enabled; not authenticating or deleting.');
}

const results = [];
const failures = [];
let nextIndex = 0;

await Promise.all(
  Array.from({ length: Math.min(concurrency, records.length) }, async (_unused, workerIndex) => {
    while (true) {
      const index = nextIndex;
      nextIndex += 1;
      if (index >= records.length) {
        return;
      }
      const record = records[index];
      try {
        const result = await deleteAndPoll(record, workerIndex + 1, session?.accessJwt, index + 1);
        results.push(result);
        const latency = result.deleteIndexLatencyMs == null ? 'missing' : formatMs(result.deleteIndexLatencyMs);
        console.log(`✓ #${index + 1} deleted ${shortURI(result.uri)} absent=${latency} attempts=${result.pollAttempts}`);
      } catch (err) {
        const failure = { index: index + 1, uri: record.uri, error: err.message || String(err) };
        failures.push(failure);
        console.error(`✗ #${index + 1} failed ${shortURI(record.uri)}: ${failure.error}`);
      }
    }
  }),
);

const afterStats = await getStats('after');
const finishedAt = new Date().toISOString();
const absentResults = results.filter((result) => result.deleteIndexLatencyMs != null);
const stillPresentResults = results.filter((result) => result.deleteIndexLatencyMs == null);
const latencies = absentResults.map((result) => result.deleteIndexLatencyMs).sort((a, b) => a - b);

const summary = {
  runID,
  startedAt,
  finishedAt,
  pdsURL,
  hyperindexBaseURL,
  graphqlURL,
  inputGlobs,
  titlePrefix: includeAnyTitle ? null : titlePrefix,
  requested: records.length,
  concurrency,
  dryRun,
  deleted: results.length,
  absent: absentResults.length,
  stillPresent: stillPresentResults.length,
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
console.log(`  requested:     ${summary.requested}`);
console.log(`  deleted:       ${summary.deleted}/${summary.requested}`);
console.log(`  absent:        ${summary.absent}/${summary.deleted}`);
console.log(`  still present: ${summary.stillPresent}`);
console.log(`  failures:      ${summary.failures}`);
console.log(`  latency:       p50=${formatMs(summary.latencyMs.p50)} p90=${formatMs(summary.latencyMs.p90)} p95=${formatMs(summary.latencyMs.p95)} max=${formatMs(summary.latencyMs.max)}`);
if (beforeStats?.tap || afterStats?.tap) {
  console.log(`  tap errors before/after: ${beforeStats?.tap?.errors ?? 'n/a'} -> ${afterStats?.tap?.errors ?? 'n/a'}`);
}
console.log(`  results:       ${resultsPath}`);

if (failures.length > 0 || stillPresentResults.length > 0) {
  process.exitCode = 1;
}

async function deleteAndPoll(record, worker, accessJwt, index) {
  const parsed = parseATURI(record.uri);
  const deleteStartedAt = Date.now();

  if (!dryRun) {
    await xrpc('com.atproto.repo.deleteRecord', {
      repo: parsed.repo,
      collection: parsed.collection,
      rkey: parsed.rkey,
    }, accessJwt);
  }

  const deletedAt = Date.now();
  const absent = dryRun
    ? { absent: true, attempts: 0, firstAbsentAt: deletedAt }
    : await pollAbsent(record.uri, deletedAt);

  return {
    index,
    worker,
    uri: record.uri,
    title: record.title,
    sourceFile: record.sourceFile,
    repo: parsed.repo,
    collection: parsed.collection,
    rkey: parsed.rkey,
    deleteLatencyMs: deletedAt - deleteStartedAt,
    deletedAt: new Date(deletedAt).toISOString(),
    absentAt: absent.firstAbsentAt ? new Date(absent.firstAbsentAt).toISOString() : null,
    deleteIndexLatencyMs: absent.firstAbsentAt ? absent.firstAbsentAt - deletedAt : null,
    pollAttempts: absent.attempts,
    absent: absent.absent,
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

async function pollAbsent(uri, deletedAt) {
  const deadline = deletedAt + pollTimeoutMs;
  let attempts = 0;
  let lastError = '';

  while (Date.now() < deadline) {
    attempts += 1;
    try {
      const body = await graphql(
        `query ActivityByUri($uri: String!) {
          orgHypercertsClaimActivityByUri(uri: $uri) {
            uri
          }
        }`,
        { uri },
      );
      const node = body.data?.orgHypercertsClaimActivityByUri;
      if (!node) {
        return { absent: true, attempts, firstAbsentAt: Date.now() };
      }
    } catch (err) {
      lastError = err.message || String(err);
    }
    await sleep(pollIntervalMs);
  }

  return { absent: false, attempts, firstAbsentAt: null, lastError };
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

function discoverRecords() {
  const seen = new Set();
  const records = [];
  for (const glob of inputGlobs) {
    for (const file of expandSimpleGlob(glob)) {
      let parsed;
      try {
        parsed = JSON.parse(fs.readFileSync(file, 'utf8'));
      } catch (err) {
        console.warn(`Skipping ${file}: ${err.message || err}`);
        continue;
      }
      for (const result of parsed.results || []) {
        if (!result.uri || seen.has(result.uri)) {
          continue;
        }
        if (!includeAnyTitle && !String(result.title || '').startsWith(titlePrefix)) {
          continue;
        }
        seen.add(result.uri);
        records.push({
          uri: result.uri,
          title: result.title || '',
          sourceFile: file,
        });
      }
    }
  }
  records.sort((a, b) => a.uri.localeCompare(b.uri));
  return records;
}

function expandSimpleGlob(pattern) {
  if (!pattern.includes('*')) {
    return fs.existsSync(pattern) ? [pattern] : [];
  }
  const dir = path.dirname(pattern);
  const base = path.basename(pattern);
  const [prefix, suffix] = base.split('*');
  if (!fs.existsSync(dir)) {
    return [];
  }
  return fs.readdirSync(dir)
    .filter((name) => name.startsWith(prefix) && name.endsWith(suffix))
    .map((name) => path.join(dir, name))
    .sort();
}

function parseATURI(uri) {
  const match = /^at:\/\/([^/]+)\/([^/]+)\/([^/]+)$/.exec(uri);
  if (!match) {
    throw new Error(`Invalid AT URI: ${uri}`);
  }
  return {
    repo: match[1],
    collection: match[2],
    rkey: match[3],
  };
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
