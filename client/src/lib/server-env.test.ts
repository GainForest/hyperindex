import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("server-only", () => ({}));

const ORIGINAL_ENV = {
  COOKIE_SECRET: process.env.COOKIE_SECRET,
  NODE_ENV: process.env.NODE_ENV,
};

afterEach(() => {
  if (ORIGINAL_ENV.COOKIE_SECRET === undefined) {
    delete process.env.COOKIE_SECRET;
  } else {
    process.env.COOKIE_SECRET = ORIGINAL_ENV.COOKIE_SECRET;
  }

  if (ORIGINAL_ENV.NODE_ENV === undefined) {
    delete process.env.NODE_ENV;
  } else {
    process.env.NODE_ENV = ORIGINAL_ENV.NODE_ENV;
  }

  vi.resetModules();
});

describe("serverEnv", () => {
  it("uses the development fallback outside production", async () => {
    delete process.env.COOKIE_SECRET;
    process.env.NODE_ENV = "development";

    const { serverEnv } = await import("./server-env");

    expect(serverEnv.COOKIE_SECRET).toBe("development-secret-at-least-32-chars!!");
  });

  it("throws in production when COOKIE_SECRET is missing", async () => {
    delete process.env.COOKIE_SECRET;
    process.env.NODE_ENV = "production";

    await expect(import("./server-env")).rejects.toThrow(
      /COOKIE_SECRET must be set in production and be at least 32 characters long/,
    );
  });

  it("throws in production when COOKIE_SECRET is too short", async () => {
    process.env.COOKIE_SECRET = "too-short";
    process.env.NODE_ENV = "production";

    await expect(import("./server-env")).rejects.toThrow(/at least 32 characters long in production/);
  });
});
