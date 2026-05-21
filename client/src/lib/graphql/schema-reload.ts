/**
 * Result returned by the admin reloadSchema mutation.
 * `lexiconCount` is the currently active public schema count; on failure it is
 * the previous working schema count. A zero count can mean either no schema is
 * active yet or the previous active schema contains zero lexicons.
 */
export interface ReloadSchemaResult {
  success: boolean;
  lexiconCount: number;
  reloadedAt: string | null;
  error: string | null;
}

/** GraphQL response shape for the admin reloadSchema mutation. */
export interface ReloadSchemaResponse {
  reloadSchema: ReloadSchemaResult;
}

/** Formats an operator-facing success message for a completed schema reload. */
export function formatReloadSchemaSuccess(lexiconCount: number): string {
  return `Reloaded public schema with ${formatLexiconCount(lexiconCount)}.`;
}

/**
 * Formats an operator-facing failure message for a reloadSchema payload failure.
 * Payload failures mean the backend attempted reload, kept fallback behavior, and
 * returned active-schema status in the normal GraphQL response.
 */
export function formatReloadSchemaFailure(result: ReloadSchemaResult): string {
  const backendError = result.error?.trim() || "The backend did not return a reload error.";

  if (result.lexiconCount > 0) {
    return `Failed to reload public schema: ${backendError}. Previous public schema is still active with ${formatLexiconCount(result.lexiconCount)}. This is the active schema count, not the failed reload attempt.`;
  }

  return `Failed to reload public schema: ${backendError}. The active public schema currently has 0 lexicons, or no previous public schema is active yet. Fix the lexicon error and reload again.`;
}

function formatLexiconCount(count: number): string {
  return `${count} lexicon${count === 1 ? "" : "s"}`;
}
