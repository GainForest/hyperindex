import { describe, expect, it } from "vitest";

import { RELOAD_SCHEMA } from "./mutations";

describe("RELOAD_SCHEMA", () => {
  it("requests the reloadSchema result fields used by the Lexicons page", () => {
    expect(RELOAD_SCHEMA).toContain("mutation ReloadSchema");
    expect(RELOAD_SCHEMA).toContain("reloadSchema");
    expect(RELOAD_SCHEMA).toContain("success");
    expect(RELOAD_SCHEMA).toContain("lexiconCount");
    expect(RELOAD_SCHEMA).toContain("reloadedAt");
    expect(RELOAD_SCHEMA).toContain("error");
  });
});
