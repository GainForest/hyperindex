---
name: deploy-railway
description: Deploy the Hyperindex frontend and backend to Railway. Use this skill when the user asks to deploy, redeploy, or update the production services on Railway.
---

# Deploy Hyperindex to Railway

## Project Layout

Hyperindex is a monorepo with two Railway services:

| Service | Source | Dockerfile | Railway Name |
|---------|--------|------------|-------------|
| **Backend** (Go) | repo root `/` | `Dockerfile` | `backend` |
| **Frontend** (Next.js) | `client/` | `client/Dockerfile` | `frontend` |

## Custom Domains

| Service | Domain |
|---------|--------|
| Backend | `https://api.hi.gainforest.app` |
| Frontend | `https://hi.gainforest.app` |

Legacy domains (still active): `backend-production-95a22.up.railway.app`, `frontend-production-dcce.up.railway.app`

## Prerequisites

- Railway CLI v4+ installed and logged in (`railway whoami`)
- Linked to project: `railway status` should show project `hyperindex`
- On the correct git branch (typically `tap-feature`)

## Deploy Backend

The backend deploys from the repo root using the root `Dockerfile`:

```bash
railway up -s backend -d
```

This uploads the entire repo, builds the Go binary in Docker, and deploys it. Takes ~3-5 minutes.

### Verify backend:
```bash
curl -s https://api.hi.gainforest.app/
# Should return: {"name":"Hyperindex","version":"0.1.0-dev",...}
```

## Deploy Frontend

**CRITICAL:** The frontend MUST use `--path-as-root` to avoid Railway picking up the root Go Dockerfile:

```bash
railway up --path-as-root client/ -s frontend -d
```

This makes `client/` the archive root so Railway only sees `client/Dockerfile` (the Next.js build). Takes ~3-5 minutes.

### Why `--path-as-root`?

Without it, `railway up` uploads the entire monorepo and Railway finds the root `Dockerfile` (Go backend) instead of `client/Dockerfile` (Next.js frontend). This causes the frontend service to run the Go binary instead of the Next.js app.

### Verify frontend:
```bash
curl -s -o /dev/null -w "%{http_code}" https://hi.gainforest.app/
# Should return: 200

# Verify it's actually Next.js (not the Go server):
curl -s https://hi.gainforest.app/ | grep -o '<title>[^<]*</title>'
# Should return: <title>Hyperindex</title>
```

## Deploy Both Services

```bash
# Backend (from repo root)
railway up -s backend -d

# Frontend (with path-as-root)
railway up --path-as-root client/ -s frontend -d
```

## Environment Variables

### Backend (`backend` service)
| Variable | Value |
|----------|-------|
| `HOST` | `0.0.0.0` |
| `PORT` | `8080` |
| `DATABASE_URL` | `sqlite:/app/data/hypergoat.db` |
| `EXTERNAL_BASE_URL` | `https://api.hi.gainforest.app` |
| `TRUST_PROXY_HEADERS` | `true` |
| `ADMIN_DIDS` | `did:plc:qc42fmqqlsmdq7jiypiiigww` (daviddao.org) |
| `OAUTH_LOOPBACK_MODE` | `true` |
| `SECRET_KEY_BASE` | *(set on Railway, do not change)* |

### Frontend (`frontend` service)
| Variable | Value |
|----------|-------|
| `PORT` | `3000` |
| `NEXT_PUBLIC_CLIENT_URL` | `https://hi.gainforest.app` |
| `NEXT_PUBLIC_HYPERINDEX_URL` | `https://api.hi.gainforest.app` |
| `NEXT_PUBLIC_ADMIN_DIDS` | `did:plc:qc42fmqqlsmdq7jiypiiigww` (UI gating only; backend `ADMIN_DIDS` remains authoritative) |
| `HYPERINDEX_URL` | `https://api.hi.gainforest.app` |
| `COOKIE_SECRET` | *(set on Railway, do not change)* |
| `ATPROTO_JWK_PRIVATE` | *(ES256 JWK, set on Railway, do not change)* |

**Note:** `NEXT_PUBLIC_HYPERINDEX_URL` is a build-time variable (inlined by Next.js during `npm run build`). The `client/Dockerfile` declares `ARG NEXT_PUBLIC_HYPERINDEX_URL` so Railway passes it during Docker build. If you set the old variable name instead, the frontend can fall back to `http://127.0.0.1:8080`, which breaks production.

## Troubleshooting

### Frontend shows Go JSON response instead of HTML
You forgot `--path-as-root client/`. Redeploy with:
```bash
railway up --path-as-root client/ -s frontend -d
```

### "Application not found" on custom domain
SSL certificate is still provisioning. Wait 5-15 minutes after adding DNS records.

### GraphiQL returns 500 through frontend
GraphiQL is served directly by the backend. The frontend has a `/graphiql` server-side redirect route that redirects to `https://api.hi.gainforest.app/graphiql`.

### OAuth login fails
Check that `ATPROTO_JWK_PRIVATE` and `NEXT_PUBLIC_CLIENT_URL` are set on the frontend service. Generate a new JWK with:
```bash
node scripts/generate-jwk.js  # (in hyperscan repo, or client/scripts/ if copied)
```

### "admin privileges required" after login
Ensure `TRUST_PROXY_HEADERS=true` is set on the backend. Without it, the backend ignores the `X-User-DID` header from the Next.js proxy.

## Setting Environment Variables

```bash
# Set a variable on a service
railway variables set 'KEY=value' -s backend
railway variables set 'KEY=value' -s frontend

# View all variables for a service
railway variables -s backend
railway variables -s frontend
```

After changing env vars, redeploy the affected service.
