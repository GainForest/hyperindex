import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/lib/session";
import { env } from "@/lib/env";

export const dynamic = "force-dynamic";

/**
 * Proxy for admin GraphQL requests.
 * Checks session authentication and passes user DID to Hyperindex.
 */
export async function POST(request: NextRequest) {
  try {
    const session = await getSession();
    const body = await request.json();

    // Build headers for Hyperindex
    const headers: HeadersInit = {
      "Content-Type": "application/json",
    };

    // If user is authenticated, pass their DID
    if (session.did) {
      headers["X-User-DID"] = session.did;
      console.log("[admin-graphql] Authenticated request", { did: session.did });
    } else {
      console.log("[admin-graphql] Unauthenticated request - no session DID");
    }

    // Proxy to Hyperindex
    const response = await fetch(`${env.HYPERINDEX_URL}/admin/graphql`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });

    const data = await response.json();

    // Log errors from Hyperindex
    if (data.errors) {
      console.log("[admin-graphql] GraphQL errors:", JSON.stringify(data.errors));
    }

    return NextResponse.json(data, { status: response.status });
  } catch (error) {
    console.error("Admin GraphQL proxy error:", error);
    return NextResponse.json(
      { errors: [{ message: "Internal server error" }] },
      { status: 500 }
    );
  }
}
