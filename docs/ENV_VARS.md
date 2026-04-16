# Environment Variables Reference

This document summarizes the environment variables currently used by Hyperindex (backend/indexer) and the Next.js client.

It also calls out legacy aliases and config drift discovered during a repo audit.

## Quick Summary

- `INDEXER_URL` is **not used** anywhere in this repository.
- Backend canonical public URL is `EXTERNAL_BASE_URL`.
- Client uses both:
  - `NEXT_PUBLIC_HYPERINDEX_URL` (build-time/public-facing)
  - `HYPERINDEX_URL` (runtime/server-side, falls back to `NEXT_PUBLIC_HYPERINDEX_URL`)

---

## 1) Backend (Hyperindex / indexer)

Source of truth: `internal/config/config.go`

### Core

| Variable | Required | Why it exists |
|---|---:|---|
| `HOST` | No | Bind address for the Go server. |
| `PORT` | No | HTTP listen port. |
| `DATABASE_URL` | Yes (prod) | Database connection string (SQLite/Postgres). |
| `SECRET_KEY_BASE` | Yes (prod) | Server secret material (must be stable across restarts). |

### Security / access

| Variable | Required | Why it exists |
|---|---:|---|
| `TRUST_PROXY_HEADERS` | No | Trust `X-User-DID` from a trusted reverse proxy for admin auth. |
| `ALLOWED_ORIGINS` | No | Allowed origins for CORS/WebSocket checks. |
| `ADMIN_DIDS` | No | Comma-separated DIDs with admin access. |

### OAuth / public URL identity

| Variable | Required | Why it exists |
|---|---:|---|
| `EXTERNAL_BASE_URL` | Yes (prod) | Canonical public URL used to generate OAuth metadata, callback URLs, issuer, GraphiQL links/endpoints. |
| `OAUTH_SIGNING_KEY` | No | Persistent OAuth signing key. If absent, ephemeral key is generated. |
| `OAUTH_LOOPBACK_MODE` | No | Enables localhost loopback-style OAuth flow for local development. |
| `DOMAIN_DID` | No | Optional explicit DID for domain identity. |

> `EXTERNAL_BASE_URL` should include a scheme (e.g. `https://...`). Missing scheme can break GraphiQL/OAuth URL generation.

### Lexicons / ingestion

| Variable | Required | Why it exists |
|---|---:|---|
| `LEXICON_DIR` | No | Directory containing lexicon JSON files at startup. |
| `TAP_ENABLED` | No | Switches ingestion path to Tap (recommended) instead of legacy Jetstream+Backfill. |
| `TAP_URL` | No | Tap WebSocket endpoint. |
| `TAP_ADMIN_PASSWORD` | Depends | Needed when using Tap admin operations. |
| `TAP_DISABLE_ACKS` | No | Debug/throughput tuning for Tap delivery semantics. |
| `JETSTREAM_URL` | No | Legacy Jetstream endpoint. |
| `JETSTREAM_COLLECTIONS` | No | Legacy collection filter list. |
| `JETSTREAM_DISABLE_CURSOR` | No | Disable cursor resume behavior in legacy mode. |
| `BACKFILL_ON_START` | No | Run legacy backfill automatically at startup. |
| `BACKFILL_COLLECTIONS` | No | Legacy backfill collection list. |
| `BACKFILL_RELAY_URL` | No | Relay URL for legacy backfill discovery. |
| `BACKFILL_PLC_URL` | No | PLC URL for legacy backfill DID resolution. |
| `BACKFILL_PDS_CONCURRENCY` | No | Per-PDS backfill concurrency. |
| `BACKFILL_MAX_PDS_WORKERS` | No | Max concurrent PDS workers in backfill. |
| `BACKFILL_MAX_HTTP` | No | Global HTTP concurrency for backfill. |
| `BACKFILL_MAX_PER_PDS` | No | Max concurrent requests per PDS. |
| `BACKFILL_MAX_REPOS` | No | Max repo/DID resolution concurrency. |
| `BACKFILL_REPO_TIMEOUT` | No | Per-repo timeout (ms) for backfill work. |
| `PLC_DIRECTORY_URL` | No | Global PLC directory override. |

---

## 2) Client (Next.js)

Primary env parser: `client/src/lib/env.ts`

### API routing

| Variable | Required | Why it exists |
|---|---:|---|
| `NEXT_PUBLIC_HYPERINDEX_URL` | Yes for non-local deploys | Build-time/public API URL baked into the JS bundle. Used by Next rewrites and docs/examples shown in UI. |
| `HYPERINDEX_URL` | No | Server-side only — prefer this for private/internal network endpoints (e.g. Railway private networking). Falls back to `NEXT_PUBLIC_HYPERINDEX_URL`. |

### OAuth and session

| Variable | Required | Why it exists |
|---|---:|---|
| `NEXT_PUBLIC_CLIENT_URL` | Recommended in prod | Canonical public client URL used for OAuth callback and client metadata URLs. |
| `NEXT_PUBLIC_VERCEL_BRANCH_URL` | Optional fallback | Fallback used if `NEXT_PUBLIC_CLIENT_URL` is empty. |
| `COOKIE_SECRET` | Yes (prod) | Encrypts/signs session cookies (`iron-session`). |
| `ATPROTO_JWK_PRIVATE` | Optional | Enables confidential OAuth client mode (`private_key_jwt`). If empty, public client mode is used. |
| `PORT` | No | Used for localhost callback URL in dev mode. |

---

## 3) Legacy aliases / drift

| Name | Status | Notes |
|---|---|---|
| `INDEXER_URL` | Unused | No references found in code. |
| `NEXT_PUBLIC_API_URL` | Removed | Renamed to `NEXT_PUBLIC_HYPERINDEX_URL`. |
| `HYPERGOAT_URL` | Removed | Dropped entirely. Use `HYPERINDEX_URL` or `NEXT_PUBLIC_HYPERINDEX_URL`. |
| `PUBLIC_URL` | Docs/skill drift | Referenced in deployment skill docs, but app code expects `NEXT_PUBLIC_CLIENT_URL`. |

---

## 4) Skill/doc follow-up

The Railway deployment skill should stay aligned with runtime code expectations:

- Prefer `NEXT_PUBLIC_CLIENT_URL` over `PUBLIC_URL` in frontend deployment instructions.
- Keep both `NEXT_PUBLIC_HYPERINDEX_URL` (build-time/public) and `HYPERINDEX_URL` (runtime/private) documented.
- Keep backend `EXTERNAL_BASE_URL` explicitly scheme-qualified (`https://...`).
