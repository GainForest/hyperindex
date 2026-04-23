import "server-only";

function getEnv(key: string, defaultValue: string = ""): string {
  return process.env[key] || defaultValue;
}

function getCookieSecret(): string {
  const isProduction = process.env.NODE_ENV === "production";
  const fallback = "development-secret-at-least-32-chars!!";
  const secret = process.env.COOKIE_SECRET || (isProduction ? "" : fallback);

  if (isProduction) {
    if (!secret) {
      throw new Error("COOKIE_SECRET must be set in production and be at least 32 characters long");
    }

    if (secret.length < 32) {
      throw new Error("COOKIE_SECRET must be at least 32 characters long in production");
    }
  }

  return secret;
}

export const serverEnv = {
  // Secret for encrypting session cookies (must be at least 32 chars)
  COOKIE_SECRET: getCookieSecret(),

  // Private JWK for confidential OAuth client (optional, for production)
  ATPROTO_JWK_PRIVATE: getEnv("ATPROTO_JWK_PRIVATE", ""),

  // Server-side only admin API key used by the Next.js proxy when calling Hyperindex admin routes.
  HYPERINDEX_ADMIN_API_KEY: getEnv("HYPERINDEX_ADMIN_API_KEY", ""),
};
