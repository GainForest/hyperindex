import { redirect } from "next/navigation";
import { env } from "@/lib/env";

export const dynamic = "force-dynamic";

/**
 * Redirects to the backend's GraphiQL explorer.
 * This is a server-side route so it reads HYPERINDEX_URL at runtime.
 */
export async function GET() {
  redirect(`${env.HYPERINDEX_URL}/graphiql`);
}
