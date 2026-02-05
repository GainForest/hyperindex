import { NextRequest, NextResponse } from "next/server";
import { getSession } from "@/lib/session";
import { env } from "@/lib/env";

export const dynamic = "force-dynamic";

/**
 * Proxy for admin GraphQL requests.
 * Checks session authentication and passes user DID to Hypergoat.
 */
export async function POST(request: NextRequest) {
  try {
    const session = await getSession();
    const body = await request.json();

    // Build headers for Hypergoat
    const headers: HeadersInit = {
      "Content-Type": "application/json",
    };

    // If user is authenticated, pass their DID
    if (session.did) {
      headers["X-User-DID"] = session.did;
    }

    // Proxy to Hypergoat
    const response = await fetch(`${env.HYPERGOAT_URL}/admin/graphql`, {
      method: "POST",
      headers,
      body: JSON.stringify(body),
    });

    const data = await response.json();

    return NextResponse.json(data, { status: response.status });
  } catch (error) {
    console.error("Admin GraphQL proxy error:", error);
    return NextResponse.json(
      { errors: [{ message: "Internal server error" }] },
      { status: 500 }
    );
  }
}
