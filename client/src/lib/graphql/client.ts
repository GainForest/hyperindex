import { GraphQLClient } from "graphql-request";
import { env } from "@/lib/env";

// Get the base URL for API requests
// - In browser: use current origin
// - On server (SSR): use localhost
function getBaseUrl(): string {
  if (typeof window !== "undefined") {
    return window.location.origin;
  }
  return "http://127.0.0.1:3000";
}

// Get Hyperindex URL for direct backend access (public API)
function getHyperindexUrl(): string {
  return env.HYPERINDEX_URL;
}

// Lazy-initialized clients to ensure proper URL detection after hydration
let _graphqlClient: GraphQLClient | null = null;
let _publicGraphqlClient: GraphQLClient | null = null;

/**
 * Admin GraphQL client - routes through Next.js API for authentication
 */
export const graphqlClient = {
  request: <T>(document: string, variables?: Record<string, unknown>): Promise<T> => {
    if (!_graphqlClient) {
      // Use the Next.js API proxy for admin requests (handles auth)
      _graphqlClient = new GraphQLClient(`${getBaseUrl()}/api/admin/graphql`, {
        credentials: "include",
      });
    }
    return _graphqlClient.request<T>(document, variables);
  },
};

/**
 * Public GraphQL client - direct to Hyperindex for unauthenticated queries
 */
export const publicGraphqlClient = {
  request: <T>(document: string, variables?: Record<string, unknown>): Promise<T> => {
    if (!_publicGraphqlClient) {
      // For SSR, go directly to Hyperindex; for client, use proxy
      const url = typeof window !== "undefined" 
        ? `${getBaseUrl()}/api/graphql`
        : `${getHyperindexUrl()}/graphql`;
      _publicGraphqlClient = new GraphQLClient(url, {
        credentials: "include",
      });
    }
    return _publicGraphqlClient.request<T>(document, variables);
  },
};
