import { getIronSession, type SessionOptions } from "iron-session";
import { cookies } from "next/headers";
import { getCookieSecret } from "./server-env";

/**
 * Session data stored in the encrypted cookie.
 */
export interface Session {
  did?: string;
  handle?: string;
  displayName?: string;
  avatar?: string;
  returnTo?: string;
  // OAuth session data (serialized) - persisted across serverless invocations
  oauthSession?: string;
}

const isProduction = process.env.NODE_ENV === "production";

function getSessionOptions(): SessionOptions {
  return {
    cookieName: "hypergoat_sid",
    password: getCookieSecret(),
    cookieOptions: {
      secure: isProduction,
      maxAge: 60 * 60 * 24 * 30, // 30 days
    },
  };
}

/**
 * Get the current user's session from their encrypted cookie.
 */
export async function getSession(): Promise<Session> {
  const cookieStore = await cookies();
  const session = await getIronSession<Session>(cookieStore, getSessionOptions());
  return session;
}

/**
 * Get the raw iron-session object for direct manipulation (save/destroy).
 */
export async function getRawSession() {
  const cookieStore = await cookies();
  return await getIronSession<Session>(cookieStore, getSessionOptions());
}

/**
 * Clear the current user's session.
 */
export async function clearSession(): Promise<void> {
  const session = await getRawSession();
  session.destroy();
}
