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

const vercelBranchUrl = getEnv("NEXT_PUBLIC_VERCEL_BRANCH_URL", "");
const publicClientUrl = getEnv("NEXT_PUBLIC_CLIENT_URL", "");
const normalizedVercelBranchUrl =
  vercelBranchUrl && !vercelBranchUrl.startsWith("http://") && !vercelBranchUrl.startsWith("https://")
    ? `https://${vercelBranchUrl}`
    : vercelBranchUrl;

export const env = {
  // Secret for encrypting session cookies (must be at least 32 chars)
  COOKIE_SECRET: getEnv("COOKIE_SECRET", "development-secret-at-least-32-chars!!"),

  // Public URL for OAuth callbacks (empty = use localhost)
  PUBLIC_CLIENT_URL: publicClientUrl || normalizedVercelBranchUrl,

  // Port for the Next.js server
  PORT: getPort(),

  // Private JWK for confidential OAuth client (optional, for production)
  ATPROTO_JWK_PRIVATE: getEnv("ATPROTO_JWK_PRIVATE", ""),

  // Hyperindex backend URL
  HYPERINDEX_URL: getEnv("HYPERINDEX_URL", getEnv("HYPERGOAT_URL", "http://127.0.0.1:8080")),
};

console.log("CONFIGURED ENV", env)
console.log({
  NEXT_PUBLIC_VERCEL_BRANCH_URL: process.env.NEXT_PUBLIC_VERCEL_BRANCH_URL,
  VERCEL_URL: process.env.VERCEL_URL,
  VERCEL_ENV: process.env.VERCEL_ENV,
});