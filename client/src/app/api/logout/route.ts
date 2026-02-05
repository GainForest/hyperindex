import { NextResponse } from "next/server";
import { getRawSession } from "@/lib/session";

export const dynamic = "force-dynamic";

export async function POST() {
  try {
    const session = await getRawSession();
    session.destroy();

    return NextResponse.json({ success: true });
  } catch (error) {
    console.error("Logout failed:", error);
    return NextResponse.json({ error: "Logout failed" }, { status: 500 });
  }
}
