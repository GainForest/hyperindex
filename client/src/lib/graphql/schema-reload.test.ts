import { describe, expect, it } from "vitest";

import {
  formatReloadSchemaFailure,
  formatReloadSchemaSuccess,
  type ReloadSchemaResult,
} from "./schema-reload";

function reloadResult(overrides: Partial<ReloadSchemaResult>): ReloadSchemaResult {
  return {
    success: false,
    lexiconCount: 0,
    reloadedAt: null,
    error: null,
    ...overrides,
  };
}

describe("formatReloadSchemaSuccess", () => {
  it("formats singular and plural active schema counts", () => {
    expect(formatReloadSchemaSuccess(1)).toBe("Reloaded public schema with 1 lexicon.");
    expect(formatReloadSchemaSuccess(42)).toBe("Reloaded public schema with 42 lexicons.");
  });
});

describe("formatReloadSchemaFailure", () => {
  it("explains fallback to the previous active schema count", () => {
    const message = formatReloadSchemaFailure(
      reloadResult({
        lexiconCount: 2,
        error: "parse database lexicon app.example.bad: invalid JSON",
      }),
    );

    expect(message).toContain("parse database lexicon app.example.bad");
    expect(message).toContain("Previous public schema is still active with 2 lexicons");
    expect(message).toContain("active schema count, not the failed reload attempt");
  });

  it("uses neutral zero-count failure copy", () => {
    const message = formatReloadSchemaFailure(
      reloadResult({
        lexiconCount: 0,
        error: "parse filesystem lexicon bad.json: invalid JSON",
      }),
    );

    expect(message).toContain("parse filesystem lexicon bad.json");
    expect(message).toContain("active public schema currently has 0 lexicons");
    expect(message).toContain("no previous public schema is active yet");
    expect(message.toLowerCase()).toContain("fix the lexicon error and reload again");
  });

  it("uses a fallback error when the backend omits one", () => {
    expect(formatReloadSchemaFailure(reloadResult({ error: "   " }))).toContain(
      "The backend did not return a reload error.",
    );
  });
});
