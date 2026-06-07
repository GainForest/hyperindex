import { describe, expect, test, vi } from "vitest";

const mockEnv = vi.hoisted(() => ({
  HYPERINDEX_URL: "https://internal.example.test",
  NEXT_PUBLIC_HYPERINDEX_URL: "https://api.example.test",
}));

vi.mock("@/lib/env", () => ({
  env: mockEnv,
}));

import { GET } from "./route";

describe("GET /graphiql", () => {
  test("serves GraphiQL with the official Explorer plugin instead of redirecting", async () => {
    const response = await GET();
    const body = await response.text();

    expect(response.status).toBe(200);
    expect(response.headers.get("Content-Type")).toContain("text/html");
    expect(response.headers.get("Location")).toBeNull();
    expect(body).toContain("@graphiql/plugin-explorer");
    expect(body).toContain("explorerPlugin()");
    expect(body).toContain("visiblePlugin: 'GraphiQL Explorer'");
    expect(body).toContain("https://api.example.test/graphql");
    expect(body).toContain("wss://api.example.test/graphql/ws");
  });
});
