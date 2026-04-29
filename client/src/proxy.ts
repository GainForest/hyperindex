import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function proxy(request: NextRequest) {
  const hostname = request.headers.get("host") || "";

  if (hostname.startsWith("localhost:")) {
    const newHost = hostname.replace("localhost", "127.0.0.1");

    const redirectUrl = new URL(request.url);
    redirectUrl.hostname = "127.0.0.1";
    redirectUrl.port = newHost.split(":")[1] || "3000";
    return NextResponse.redirect(redirectUrl, { status: 307 });
  }

  return NextResponse.next();
}

export const config = {
  matcher: "/:path*",
};
