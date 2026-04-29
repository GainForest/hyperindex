import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function proxy(request: NextRequest) {
  const hostname = request.headers.get("host") || "";

  if (hostname.toLowerCase() === "localhost" || hostname.toLowerCase().startsWith("localhost:")) {
    const newHost = hostname.replace("localhost", "127.0.0.1");

    const redirectUrl = new URL(request.url);
    const port = newHost.includes(":") ? newHost.split(":")[1] : "3000";
    redirectUrl.hostname = "127.0.0.1";
    redirectUrl.port = port;
    return NextResponse.redirect(redirectUrl, { status: 307 });
  }

  return NextResponse.next();
}

export const config = {
  matcher: "/:path*",
};
