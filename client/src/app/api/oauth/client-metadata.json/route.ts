import { NextResponse } from "next/server";
import { env } from "@/lib/env";
import { serverEnv } from "@/lib/server-env";

export const dynamic = "force-dynamic";

export async function GET() {
  const publicUrl = env.PUBLIC_CLIENT_URL;
  const url = publicUrl || `http://127.0.0.1:${env.PORT}`;
  const isConfidential = !!publicUrl && !!serverEnv.ATPROTO_JWK_PRIVATE;

  const metadata: Record<string, unknown> = {
    client_name: "Hypergoat Admin",
    client_uri: url,
    dpop_bound_access_tokens: true,
    grant_types: ["authorization_code", "refresh_token"],
    response_types: ["code"],
    scope: "atproto transition:generic",
    application_type: "web",
    redirect_uris: [`${url}/api/oauth/callback`],
  };

  if (isConfidential) {
    metadata.client_id = `${publicUrl}/api/oauth/client-metadata.json`;
    metadata.token_endpoint_auth_method = "private_key_jwt";
    metadata.token_endpoint_auth_signing_alg = "ES256";
    metadata.jwks_uri = `${publicUrl}/api/oauth/jwks.json`;
  } else {
    metadata.client_id = `http://localhost?redirect_uri=${encodeURIComponent(`${url}/api/oauth/callback`)}&scope=${encodeURIComponent("atproto transition:generic")}`;
    metadata.token_endpoint_auth_method = "none";
  }

  return NextResponse.json(metadata);
}
