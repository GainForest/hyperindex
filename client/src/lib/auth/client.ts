import { NodeOAuthClient } from "@atproto/oauth-client-node";
import { JoseKey } from "@atproto/jwk-jose";
import { env } from "../env";
import { getRawSession } from "../session";
import { serverEnv } from "../server-env";

const oauthClientKey = "globalOAuthClient";

// In development, clear cached client on hot reload to pick up config changes
if (process.env.NODE_ENV !== "production") {
  (global as Record<string, unknown>)[oauthClientKey] = null;
}
if (!(global as Record<string, unknown>)[oauthClientKey]) {
  (global as Record<string, unknown>)[oauthClientKey] = null;
}

// ============================================================================
// In-Memory Store with Cookie Sync
// ============================================================================

const globalStoreKey = "oauthSharedStore";
if (!(global as Record<string, unknown>)[globalStoreKey]) {
  (global as Record<string, unknown>)[globalStoreKey] = new Map();
}
const sharedStore: Map<string, unknown> = (
  global as Record<string, unknown>
)[globalStoreKey] as Map<string, unknown>;

// State store - in-memory only, used during short-lived OAuth flow
const stateStore = {
  async get(key: string) {
    return sharedStore.get(`state:${key}`);
  },
  async set(key: string, value: unknown) {
    sharedStore.set(`state:${key}`, value);
  },
  async del(key: string) {
    sharedStore.delete(`state:${key}`);
  },
};

// Session store - syncs with cookie for persistence
const sessionStore = {
  async get(key: string) {
    const memValue = sharedStore.get(`session:${key}`);
    if (memValue) {
      return memValue;
    }

    try {
      const session = await getRawSession();
      if (session.oauthSession && session.did === key) {
        const parsed = JSON.parse(session.oauthSession);
        sharedStore.set(`session:${key}`, parsed);
        return parsed;
      }
    } catch (err) {
      console.warn("Failed to restore OAuth session from cookie:", err);
    }

    return undefined;
  },
  async set(key: string, value: unknown) {
    sharedStore.set(`session:${key}`, value);

    try {
      const session = await getRawSession();
      session.oauthSession = JSON.stringify(value);
      await session.save();
    } catch (err) {
      console.warn("Failed to save OAuth session to cookie:", err);
    }
  },
  async del(key: string) {
    sharedStore.delete(`session:${key}`);

    try {
      const session = await getRawSession();
      session.oauthSession = undefined;
      await session.save();
    } catch (err) {
      console.warn("Failed to clear OAuth session from cookie:", err);
    }
  },
};

// ============================================================================
// JWK Keyset Management
// ============================================================================

let cachedKeyset: Awaited<ReturnType<typeof JoseKey.fromImportable>>[] | null =
  null;

async function getKeyset() {
  if (cachedKeyset) {
    return cachedKeyset;
  }

  const jwkPrivate = serverEnv.ATPROTO_JWK_PRIVATE;
  if (!jwkPrivate) {
    return null; // Public client mode
  }

  try {
    const jwk = JSON.parse(jwkPrivate);
    const key = await JoseKey.fromImportable(jwk, jwk.kid || "key-1");
    cachedKeyset = [key];
    return cachedKeyset;
  } catch (err) {
    console.error("Failed to parse ATPROTO_JWK_PRIVATE:", err);
    return null;
  }
}

// ============================================================================
// OAuth Client Factory
// ============================================================================

export const createClient = async () => {
  const publicUrl = env.PUBLIC_CLIENT_URL;
  // Must use 127.0.0.1 per RFC 8252 for ATProto OAuth localhost development
  const localhostUrl = `http://127.0.0.1:${env.PORT}`;
  const enc = encodeURIComponent;

  // Detect if we're running on localhost (dev mode)
  const isLocalDev = process.env.NODE_ENV !== "production";

  // Use localhost URL in development, production URL otherwise
  const url = isLocalDev ? localhostUrl : publicUrl || localhostUrl;

  let keyset = null;
  try {
    // Only load keyset for production (confidential client)
    if (!isLocalDev && publicUrl) {
      keyset = await getKeyset();
    }
  } catch (err) {
    console.error("Error getting keyset:", err);
  }

  const isConfidentialClient = keyset !== null && !!publicUrl && !isLocalDev;

  // Build client metadata based on client type
  const clientMetadata: Record<string, unknown> = {
    client_name: "Hyperindex Admin",
    client_uri: url,
    dpop_bound_access_tokens: true,
    grant_types: ["authorization_code", "refresh_token"],
    response_types: ["code"],
    scope: "atproto transition:generic",
    application_type: "web",
  };

  if (isConfidentialClient) {
    clientMetadata.client_id = `${publicUrl}/api/oauth/client-metadata.json`;
    clientMetadata.redirect_uris = [`${publicUrl}/api/oauth/callback`];
    clientMetadata.token_endpoint_auth_method = "private_key_jwt";
    clientMetadata.token_endpoint_auth_signing_alg = "ES256";
    clientMetadata.jwks_uri = `${publicUrl}/api/oauth/jwks.json`;
  } else {
    clientMetadata.client_id = `http://localhost?redirect_uri=${enc(`${url}/api/oauth/callback`)}&scope=${enc("atproto transition:generic")}`;
    clientMetadata.redirect_uris = [`${url}/api/oauth/callback`];
    clientMetadata.token_endpoint_auth_method = "none";
  }

  const clientConfig: Record<string, unknown> = {
    clientMetadata,
    stateStore,
    sessionStore,
  };

  if (keyset) {
    clientConfig.keyset = keyset;
  }

  return new NodeOAuthClient(
    clientConfig as ConstructorParameters<typeof NodeOAuthClient>[0]
  );
};

export const getGlobalOAuthClient = async () => {
  const currentClient = (global as Record<string, unknown>)[oauthClientKey];
  if (!currentClient) {
    try {
      const newClient = await createClient();
      (global as Record<string, unknown>)[oauthClientKey] = newClient;
      return newClient;
    } catch (err) {
      console.error("Failed to create OAuth client:", err);
      throw err;
    }
  }
  return currentClient as NodeOAuthClient;
};

/**
 * Get the JWKS (public keys) for the confidential client.
 */
export async function getJwks(): Promise<{ keys: unknown[] } | null> {
  const client = await getGlobalOAuthClient();

  if ("jwks" in client && client.jwks) {
    return client.jwks as { keys: unknown[] };
  }

  return null;
}
