import { NextRequest } from "next/server";
import { beforeEach, describe, expect, it, vi } from "vitest";

const mockGetSession = vi.hoisted(() => vi.fn());
const mockEnv = vi.hoisted(() => ({
  HYPERINDEX_ADMIN_API_KEY: "test-admin-key",
  HYPERINDEX_URL: "https://hyperindex.example.com",
}));

vi.mock("@/lib/session", () => ({
  getSession: mockGetSession,
}));

vi.mock("@/lib/env", () => ({
  env: mockEnv,
}));

import { POST } from "./route";

function createRequest() {
  return new NextRequest("http://localhost:3000/api/admin/graphql", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      query: "{ currentSession { did isAdmin } }",
    }),
  });
}

describe("POST /api/admin/graphql", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEnv.HYPERINDEX_ADMIN_API_KEY = "test-admin-key";
    mockEnv.HYPERINDEX_URL = "https://hyperindex.example.com";
    global.fetch = vi.fn();
  });

  it("forwards proxied admin headers for authenticated sessions", async () => {
    mockGetSession.mockResolvedValue({ did: "did:plc:admin", handle: "admin.bsky.social" });
    vi.mocked(global.fetch).mockResolvedValue(
      new Response(JSON.stringify({ data: { currentSession: { did: "did:plc:admin", isAdmin: true } } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const response = await POST(createRequest());

    expect(global.fetch).toHaveBeenCalledTimes(1);
    expect(global.fetch).toHaveBeenCalledWith("https://hyperindex.example.com/admin/graphql", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-User-DID": "did:plc:admin",
        "X-Admin-API-Key": "test-admin-key",
      },
      body: JSON.stringify({ query: "{ currentSession { did isAdmin } }" }),
    });

    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({
      data: { currentSession: { did: "did:plc:admin", isAdmin: true } },
    });
  });

  it("returns 500 when the admin proxy key is missing", async () => {
    mockEnv.HYPERINDEX_ADMIN_API_KEY = "";
    mockGetSession.mockResolvedValue({ did: "did:plc:admin", handle: "admin.bsky.social" });

    const response = await POST(createRequest());

    expect(global.fetch).not.toHaveBeenCalled();
    expect(response.status).toBe(500);
    await expect(response.json()).resolves.toEqual({
      errors: [{ message: "Admin proxy is not configured" }],
    });
  });

  it("does not send proxied admin headers for unauthenticated sessions", async () => {
    mockGetSession.mockResolvedValue({ did: "", handle: "" });
    vi.mocked(global.fetch).mockResolvedValue(
      new Response(JSON.stringify({ data: { currentSession: null } }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await POST(createRequest());

    expect(global.fetch).toHaveBeenCalledWith("https://hyperindex.example.com/admin/graphql", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ query: "{ currentSession { did isAdmin } }" }),
    });
  });

  it("passes through backend status codes and payloads", async () => {
    mockGetSession.mockResolvedValue({ did: "did:plc:admin", handle: "admin.bsky.social" });
    vi.mocked(global.fetch).mockResolvedValue(
      new Response(JSON.stringify({ errors: [{ message: "forbidden" }] }), {
        status: 403,
        headers: { "Content-Type": "application/json" },
      }),
    );

    const response = await POST(createRequest());

    expect(response.status).toBe(403);
    await expect(response.json()).resolves.toEqual({
      errors: [{ message: "forbidden" }],
    });
  });
});
