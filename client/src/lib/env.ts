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

const vercelBranchUrl = process.env.NEXT_PUBLIC_VERCEL_BRANCH_URL || "";
const publicClientUrl = process.env.NEXT_PUBLIC_CLIENT_URL || "";

export const env = {
  // Secret for encrypting session cookies (must be at least 32 chars)
  COOKIE_SECRET: getEnv("COOKIE_SECRET", "development-secret-at-least-32-chars!!"),

  // Public URL for OAuth callbacks (empty = use localhost)
  PUBLIC_CLIENT_URL: resolvePublicClientURL(publicClientUrl, vercelBranchUrl),

  // Port for the Next.js server
  PORT: getPort(),

  // Private JWK for confidential OAuth client (optional, for production)
  ATPROTO_JWK_PRIVATE: getEnv("ATPROTO_JWK_PRIVATE", ""),

  // Hyperindex backend URL
  HYPERINDEX_URL: getEnv("HYPERINDEX_URL", getEnv("HYPERGOAT_URL", "http://127.0.0.1:8080")),
};
