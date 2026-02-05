import { NextResponse } from "next/server";
import { getSession } from "@/lib/session";

export const dynamic = "force-dynamic";

export async function GET() {
  try {
    const session = await getSession();

    if (!session.did) {
      return NextResponse.json({ authenticated: false });
    }

    return NextResponse.json({
      authenticated: true,
      did: session.did,
      handle: session.handle,
      displayName: session.displayName,
      avatar: session.avatar,
    });
  } catch (error) {
    console.error("Status check failed:", error);
    return NextResponse.json({ authenticated: false });
  }
}
