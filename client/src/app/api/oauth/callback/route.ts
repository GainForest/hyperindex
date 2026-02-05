import { NextRequest } from "next/server";
import { Agent } from "@atproto/api";
import { getGlobalOAuthClient } from "@/lib/auth/client";
import { getRawSession } from "@/lib/session";

export const dynamic = "force-dynamic";

export async function GET(request: NextRequest) {
  try {
    const client = await getGlobalOAuthClient();
    const url = new URL(request.url);
    const params = new URLSearchParams(url.search);

    // Retry OAuth callback up to 3 times for network errors
    let oauthSession;
    let lastError;

    for (let attempt = 1; attempt <= 3; attempt++) {
      try {
        const result = await client.callback(params);
        oauthSession = result.session;
        break;
      } catch (error) {
        lastError = error;
        const errorMessage =
          error instanceof Error ? error.message : String(error);
        console.error(
          `OAuth callback attempt ${attempt} failed:`,
          errorMessage
        );

        const isNetworkError =
          errorMessage.includes("UND_ERR_SOCKET") ||
          errorMessage.includes("fetch failed") ||
          errorMessage.includes("Failed to resolve OAuth server metadata");

        if (isNetworkError && attempt < 3) {
          await new Promise((resolve) => setTimeout(resolve, attempt * 1000));
          continue;
        }

        throw error;
      }
    }

    if (!oauthSession) {
      throw lastError || new Error("Failed to create session after retries");
    }

    // Fetch profile information
    let handle: string = oauthSession.did;
    let displayName: string | undefined;
    let avatar: string | undefined;

    try {
      const agent = new Agent(oauthSession);
      const profile = await agent.getProfile({ actor: oauthSession.did });

      if (profile.success) {
        handle = profile.data.handle;
        displayName = profile.data.displayName;
        avatar = profile.data.avatar;
      }
    } catch (err) {
      console.warn("Failed to fetch profile during login:", err);
    }

    // Save user info to session cookie and read returnTo
    const session = await getRawSession();
    const returnTo = session.returnTo || "/";
    session.did = oauthSession.did;
    session.handle = handle;
    session.displayName = displayName;
    session.avatar = avatar;
    session.returnTo = undefined; // Clear after use
    await session.save();

    // Redirect to the page the user was on before login
    const requestUrl = new URL(request.url);
    const origin = requestUrl.origin;
    const redirectPath = returnTo.startsWith("/") ? returnTo : "/";

    return Response.redirect(`${origin}${redirectPath}`, 303);
  } catch (error) {
    console.error("OAuth callback failed:", error);
    const requestUrl = new URL(request.url);
    return Response.redirect(
      `${requestUrl.origin}/?error=${encodeURIComponent("Authentication failed - please try again")}`,
      303
    );
  }
}
