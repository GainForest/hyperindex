/**
 * Environment variables for the client.
 * Uses process.env directly with defaults for development.
 */

function getEnv(key: string, defaultValue: string = ""): string {
  return process.env[key] || defaultValue;
}

function getPort(): number {
  const port = process.env.PORT;
  return port ? parseInt(port, 10) : 3000;
}

function getHostname(value: string): string {
  const normalized = normalizePublicURL(value);
  if (!normalized) {
    return "";
  }

  try {
    return new URL(normalized).hostname;
  } catch {
    return "";
  }
}

export function normalizePublicURL(value: string): string {
  const trimmed = value.trim().replace(/\/+$/, "");
  if (!trimmed) {
    return "";
  }

  if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) {
    return trimmed;
  }

  return `https://${trimmed}`;
}

export function resolvePublicClientURL(publicClientUrl: string, vercelBranchUrl: string): string {
  const normalizedPublicClientUrl = normalizePublicURL(publicClientUrl);
  const normalizedVercelBranchUrl = normalizePublicURL(vercelBranchUrl);
  return normalizedPublicClientUrl || normalizedVercelBranchUrl;
}

export function validateHyperindexURLConfiguration(
  publicClientUrl: string,
  vercelBranchUrl: string,
  hyperindexUrl: string,
): void {
  const clientHostname = getHostname(resolvePublicClientURL(publicClientUrl, vercelBranchUrl));
  const hyperindexHostname = getHostname(hyperindexUrl);

  if (clientHostname && hyperindexHostname && clientHostname === hyperindexHostname) {
    throw new Error(
      `Invalid config: HYPERINDEX_URL / NEXT_PUBLIC_HYPERINDEX_URL points to the client hostname (${clientHostname}). ` +
        `It must point to the backend/Hyperindex URL, not the frontend.`,
    );
  }
}

const vercelBranchUrl = process.env.NEXT_PUBLIC_VERCEL_BRANCH_URL || "";
const publicClientUrl = process.env.NEXT_PUBLIC_CLIENT_URL || "";
const nextPublicHyperindexUrl = process.env.NEXT_PUBLIC_HYPERINDEX_URL || "";
const normalizedNextPublicHyperindexUrl = normalizePublicURL(nextPublicHyperindexUrl);
const normalizedHyperindexUrl = normalizePublicURL(process.env.HYPERINDEX_URL || "");
const resolvedHyperindexUrl = normalizedHyperindexUrl || normalizedNextPublicHyperindexUrl || "http://127.0.0.1:8080";

validateHyperindexURLConfiguration(publicClientUrl, vercelBranchUrl, resolvedHyperindexUrl);

export const env = {
  // Secret for encrypting session cookies (must be at least 32 chars)
  COOKIE_SECRET: getEnv("COOKIE_SECRET", "development-secret-at-least-32-chars!!"),

  // Public URL for OAuth callbacks (empty = use localhost)
  PUBLIC_CLIENT_URL: resolvePublicClientURL(publicClientUrl, vercelBranchUrl),

  // Port for the Next.js server
  PORT: getPort(),

  // Private JWK for confidential OAuth client (optional, for production)
  ATPROTO_JWK_PRIVATE: getEnv("ATPROTO_JWK_PRIVATE", ""),

  // Client-facing URL baked into the JS bundle at build time
  NEXT_PUBLIC_HYPERINDEX_URL: normalizedNextPublicHyperindexUrl,

  // Server-side only — use this for private/internal network endpoints (e.g. Railway private networking).
  // Falls back to NEXT_PUBLIC_HYPERINDEX_URL so you only need one var if both URLs are the same.
  HYPERINDEX_URL: resolvedHyperindexUrl,
};
