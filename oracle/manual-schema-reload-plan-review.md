Inherited decisions:
- Manual reload only: no automatic reload after lexicon mutations in first pass.
- Reload must not change Tap/Jetstream ingestion filters.
- Previous public schema stays active if rebuild fails.
- Admin mutation must be admin-gated.
- Operator-facing change needs docs + Changie fragment.

Diagnosis:
- The plan is directionally sound, but needs tightening before implementation.
- Main risk: conflating “schema reload” with existing lexicon ingestion callback/collection resolution.
- Current code skips malformed filesystem lexicons and skips invalid DB lexicons at startup; the plan says invalid lexicon JSON should make reload fail. That behavior change must be explicit.

Drift / contradiction check:
- `NewPublicSchemaManager(...)(..., error)` conflicts with the plan’s “no schema loaded => return 503” behavior. Prefer manager creation without requiring a successful initial build, then attempt initial reload and register handlers regardless.
- Subscription plan is underspecified: current WS clients cache `*graphql.Schema` at connection creation. To make “new subscription operations use current schema” true, the client must fetch/snapshot schema at subscribe time, not connection time.
- Do not derive Jetstream collections from the reload manager unless explicitly approved; that would violate the non-goal of not changing ingestion filters.
- Admin API result style differs from existing mutation error style. If `reloadSchema` returns `{ success: false, error }`, rebuild failures should not also be GraphQL resolver errors.

Recommendation:
1. Update plan before implementation:
   - Define strictness: DB lexicon parse/build errors fail reload; filesystem parse behavior either preserves current skip behavior or is explicitly changed.
   - Define `lexiconCount` semantics on failure.
   - Store schema + counts + reload time in one atomic snapshot.
   - Use a reload mutex plus atomic snapshot reads.
   - Snapshot subscription schema at subscribe time.
2. Implementation sequence:
   - Add `PublicSchemaManager` in `internal/graphql`, using schema builder directly.
   - Convert public HTTP handler to use a schema provider and return HTTP 503 GraphQL-shaped JSON if unavailable.
   - Convert subscription handler to use provider at subscribe time.
   - Add admin `reloadSchema` type/mutation/resolver callback.
   - Wire callback in `cmd/hyperindex/main.go` after manager creation.
   - Add frontend mutation/UI and update client types.
   - Update README, client README, Lexicons page copy, admin mutation docs/list, and Changie.

Risks:
- Invalid uploaded lexicons can currently be stored because upload only checks top-level `id`.
- Duplicate filesystem+DB lexicon IDs have unclear precedence and may leave stale registry defs.
- Existing active subscriptions may keep old schema; that is acceptable if documented.
- Tests should include no-schema 503, failed reload preserving old schema, unauthorized admin mutation, callback missing, same-WS new subscription after reload, and frontend success/failure handling.

Need from main agent:
- Decide whether filesystem lexicon parse errors should fail reload or continue being skipped.
- Decide whether `lexiconCount` means active schema count after failure or attempted reload count.
- Decide if invalid uploads should be rejected now or left as reload-time failures.

Suggested execution prompt:
- No executor handoff warranted until the above plan corrections are made.

Note: I could not write `/home/kzoeps/Projects/gainforest/hyperindex-worktree/oracle/manual-schema-reload-plan-review.md` because this child only has read/grep/ls/bash tools and bash is constrained to read-only inspection.