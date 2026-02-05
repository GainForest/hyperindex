import { NextRequest, NextResponse } from "next/server";
import { getGlobalOAuthClient } from "@/lib/auth/client";
import { isValidHandle } from "@atproto/syntax";
import { OAuthResolverError } from "@atproto/oauth-client-node";
import { getRawSession } from "@/lib/session";

export const dynamic = "force-dynamic";

export async function POST(request: NextRequest) {
  try {
    const client = await getGlobalOAuthClient();
    const body = await request.json();
    const handle = body?.handle;
    const returnTo = body?.returnTo;

    if (typeof handle !== "string" || !isValidHandle(handle)) {
      return NextResponse.json({ error: "Invalid handle" }, { status: 400 });
    }

    // Store returnTo in session before redirecting
    if (returnTo && typeof returnTo === "string") {
      const session = await getRawSession();
      session.returnTo = returnTo;
      await session.save();
    }

    const url = await client.authorize(handle, {
      scope: "atproto transition:generic",
    });

    return NextResponse.json({ redirectUrl: url.toString() });
  } catch (error) {
    console.error("OAuth authorize failed:", error);
    let errorMessage = "Couldn't initiate login";

    if (error instanceof OAuthResolverError) {
      errorMessage = error.message;
    }

    return NextResponse.json({ error: errorMessage }, { status: 500 });
  }
}
