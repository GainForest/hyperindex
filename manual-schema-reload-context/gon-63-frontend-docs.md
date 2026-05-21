# GON-63 / MSR-005 frontend reload UI, docs, and Changie handoff

Read-only inspection for the final frontend/docs integration after the backend `reloadSchema` API is stable. I used the local draft issue source in `plans/manual-schema-reload-linear-issues.json`; I did not modify code or fetch Linear remotely.

## Relevant files/functions

### Plan and issue evidence

- `docs/manual-schema-reload-plan.md:52-71` defines the admin mutation/result shape:
  - `reloadSchema { success lexiconCount reloadedAt error }`
  - `ReloadSchemaResult { success: Boolean!, lexiconCount: Int!, reloadedAt: String, error: String }`
- `docs/manual-schema-reload-plan.md:203-231` is the frontend spec:
  - Add admin-only Lexicons page section near register/upload controls.
  - Title: **Public GraphQL schema**.
  - Button: **Reload schema**.
  - Success copy like `Reloaded public schema with 42 lexicons.`
  - Failure copy must include backend error and state that the previous public schema is still active; if `lexiconCount` is non-zero, clarify it is the active schema count, not the failed attempt count.
- `docs/manual-schema-reload-plan.md:235-243` says to update docs that still say restart/redeploy is required and gives replacement wording.
- `docs/manual-schema-reload-plan.md:264-270` suggests frontend tests for mutation document, admin button, and success/error messages if practical.
- `docs/manual-schema-reload-plan.md:288-290` requires an operator-facing Changie fragment.
- `plans/manual-schema-reload-linear-issues.json:49-55` contains draft MSR-005, equivalent to GON-63 for this handoff. It depends on MSR-003/MSR-004 and repeats the same UI/docs/Changie acceptance criteria.

### Frontend GraphQL patterns

- `client/src/lib/graphql/mutations.ts:1-138`
  - Mutation documents are exported constants using `gql` from `graphql-request`.
  - Existing lexicon mutations:
    - `UPLOAD_LEXICONS` at `:43-48` returns scalar `uploadLexicons`.
    - `REGISTER_LEXICON` at `:115-124` returns `{ id json createdAt }`.
    - `DELETE_LEXICON` at `:126-131` returns scalar `deleteLexicon`.
- `client/src/lib/graphql/client.ts:23-35`
  - `graphqlClient.request<T>(document, variables)` sends admin GraphQL through Next `/api/admin/graphql` with cookies.
  - This is `graphql-request`, not Apollo.
- `client/src/app/api/admin/graphql/route.ts:22-43`
  - The proxy adds `X-User-DID` and `X-Admin-API-Key` for authenticated sessions, then forwards to `${env.HYPERINDEX_URL}/admin/graphql`.
- `client/src/lib/auth/index.tsx:164-203`
  - `useAdminSession()` queries `currentSession` and exposes `isAdmin`; Lexicons page uses only `isAdmin` and does not redirect non-admins.

### Lexicons page patterns

- `client/src/app/lexicons/page.tsx:1-11`
  - Client component.
  - Imports `useQuery`, `useMutation`, `useQueryClient` from `@tanstack/react-query` and current mutations from `@/lib/graphql/mutations`.
- `client/src/app/lexicons/page.tsx:244-257`
  - Local page state includes shared `error` and `success` alert strings, upload/delete pending state, and refs.
- `client/src/app/lexicons/page.tsx:259-267`
  - `GET_LEXICONS` query and `registerMutation` use `graphqlClient.request` directly.
- `client/src/app/lexicons/page.tsx:269-293`
  - `uploadMutation` guards admin access, sets success/error messages, invalidates `lexicons`, clears file input, and uses `onSettled` to reset upload state.
- `client/src/app/lexicons/page.tsx:295-311`
  - `deleteMutation` sets success/error messages, invalidates `lexicons`, and uses `onMutate`/`onSettled` for per-row loading.
- `client/src/app/lexicons/page.tsx:466-472`
  - Page-level alert pattern: error uses `<Alert variant="error" onClose={...}>`; success uses `<Alert variant="success">`.
- `client/src/app/lexicons/page.tsx:474-555`
  - Existing admin-only sections live inside a `space-y-4` wrapper and are individually guarded by `{isAdmin && (...)}`.
  - Register section at `:475-515`; upload section at `:517-555`.
  - Current stale copy at `:524`: `A backend restart may be required before new lexicons appear in the public GraphQL schema.`
- `client/src/components/ui/Button.tsx:38-75`
  - `Button` supports `loading`; for actual `<button>`, loading disables it and shows a spinner.
- `client/src/components/ui/Alert.tsx:23-35`
  - `Alert` renders `role="alert"` and supports variants `info | success | warning | error`.

### Docs that currently need updates

- `README.md:31-53`
  - Register/upload instructions currently end with restart/redeploy requirement.
- `README.md:137-142`
  - Query section says new typed queries appear after backend restart.
- `README.md:314-321`
  - Admin API mutation list does not include `reloadSchema` yet.
- `client/README.md:34-37`
  - Vercel preview note says redeploying frontend does not rebuild backend schema and tells operators to restart/redeploy backend.
- Search for restart/redeploy references relevant to lexicons found only:
  - `README.md:53`
  - `README.md:141`
  - `client/README.md:36`
  - `client/src/app/lexicons/page.tsx:524`

### Changie conventions

- `docs/changelog-workflow.md:1-18` says release notes come from `.changes/unreleased/*.yaml` and `Affects` must be `user`, `operator`, or `developer`.
- `.changie.yaml:15-36` lists allowed kinds: `added`, `breaking`, `changed`, `deprecated`, `removed`, `fixed`, `security`.
- `.changie.yaml:37-44` lists allowed `Affects` enum values.
- Existing `.changes/unreleased/*.yaml` fragments use this format:
  ```yaml
  kind: added
  body: Add ...
  custom:
    Affects: operator
  ```
  Some older fragments include a `time:` field and 4-space indentation under `custom`; both styles are present, but the shortest current valid style is used by `add-api-smoke-tests.yaml`.
- `.agents/skills/writing-changie/SKILL.md` recommends a descriptive kebab-case filename and impact-focused body, not implementation details.

### Testing setup

- `client/package.json:5-12` scripts:
  - `npm --prefix client run lint`
  - `npm --prefix client run test` (`vitest run`)
  - `npm --prefix client run build`
- `client/package.json:13-42` dependencies include `@tanstack/react-query`, `graphql-request`, React 19, Next 16, Vitest. There is no direct `@testing-library/react`, `@testing-library/user-event`, `jsdom`, or `happy-dom` dependency.
- Existing tests are plain Vitest/Node style:
  - `client/src/lib/env.test.ts:1-53`
  - `client/src/lib/server-env.test.ts:1-53`
  - `client/src/app/api/admin/graphql/route.test.ts:1-123`
- No existing React component test pattern was found. Full Lexicons page interaction tests are not practical with the current frontend setup unless adding test dependencies or building heavy module mocks.

## Current frontend/docs patterns

- This frontend uses **TanStack React Query `useMutation` + `graphql-request`**, not Apollo.
- Admin GraphQL calls should use `graphqlClient.request`, not `publicGraphqlClient`.
- Admin-only UI is currently hidden with `{isAdmin && (...)}` rather than redirecting on the Lexicons page.
- Page status uses shared top-level `error`/`success` alerts. This is the lowest-friction pattern for reload results.
- Existing mutation success handlers set success, clear error, sometimes invalidate queries, and auto-clear success after 3-5 seconds. Error handlers leave errors visible until closed.
- The new reload action should not invalidate the lexicon list; reload changes active public schema, not the admin lexicon table. It can leave the existing `lexicons` query untouched.
- Avoid using dashboard/statistics `lexiconCount` for reload status. The API result `lexiconCount` is the active public schema count; existing statistics count is DB lexicon count.

## Exact UI state and copy to implement

### Admin-only section copy

Place a new admin-only section near the existing register/upload cards, ideally after upload within the current `space-y-4` admin-controls block:

- Title: `Public GraphQL schema`
- Description:
  `Registering, uploading, deleting, or re-adding lexicons updates the database list. Reload the live public /graphql schema to make typed fields appear or disappear without restarting the backend.`
- Warning/help text:
  `Reloading the schema does not update Tap/Jetstream ingestion filters. Configure ingestion filters separately.`
- Button idle text: `Reload schema`
- Button pending text: `Reloading...`

Use `<Button variant="primary" loading={reloadSchemaMutation.isPending} disabled={reloadSchemaMutation.isPending}>`.

### Success state

For `{ reloadSchema: { success: true, lexiconCount } }`:

```ts
Reloaded public schema with ${lexiconCount} lexicon${lexiconCount === 1 ? "" : "s"}.
```

Recommended state behavior:

- `setSuccess(...)`
- `setError(null)`
- auto-clear success after about 5 seconds, matching upload behavior
- leave `reloadedAt` unused unless the backend/product explicitly asks to display it; it is nullable in the planned API

### Payload failure state (`success: false`)

For `lexiconCount > 0`:

```ts
Failed to reload public schema: ${backendError}. Previous public schema is still active with ${lexiconCount} lexicon${lexiconCount === 1 ? "" : "s"}. This is the active schema count, not the failed reload attempt.
```

For `lexiconCount === 0`:

```ts
Failed to reload public schema: ${backendError}. No previous public schema is active yet; fix the lexicon error and reload again.
```

Recommended state behavior:

- Treat `success: false` as an application-level failure inside `onSuccess`, not `onError`.
- Use `response.reloadSchema.error?.trim() || "The backend did not return a reload error."` for `backendError`.
- `setError(...)`
- `setSuccess(null)`
- do not auto-dismiss the error

### Request/GraphQL failure state

For rejected `graphqlClient.request` promises, e.g. unauthorized, missing callback wiring, proxy/config failures:

```ts
Could not start schema reload: ${err.message}
```

This differs from payload failure because there may be no active schema count in the response.

### Existing stale Lexicons page copy

Replace `client/src/app/lexicons/page.tsx:524` with copy that does not mention backend restart. Suggested concise replacement:

```text
Each JSON file must contain a top-level id field. After changing lexicons, use Reload schema below to update the live public GraphQL schema.
```

## Proposed implementation steps

1. Verify backend API is actually present on the branch before frontend work:
   - Search for `reloadSchema` / `ReloadSchemaResult` under `internal/graphql/admin`.
   - If absent, stop or coordinate; current inspected worktree has no backend `reloadSchema` matches under `cmd/` or `internal/`, so this frontend task must wait for MSR-003/MSR-004/GON backend work.
2. Add `RELOAD_SCHEMA` to `client/src/lib/graphql/mutations.ts`:
   ```ts
   export const RELOAD_SCHEMA = gql`
     mutation ReloadSchema {
       reloadSchema {
         success
         lexiconCount
         reloadedAt
         error
       }
     }
   `;
   ```
3. Add reload response types, either local to `client/src/app/lexicons/page.tsx` or in `client/src/types/index.ts` if reused:
   ```ts
   interface ReloadSchemaResult {
     success: boolean;
     lexiconCount: number;
     reloadedAt: string | null;
     error: string | null;
   }

   interface ReloadSchemaResponse {
     reloadSchema: ReloadSchemaResult;
   }
   ```
4. Update Lexicons page imports to include `RELOAD_SCHEMA`.
5. Add `reloadSchemaMutation = useMutation({ ... })` near existing lexicon mutations.
   - Guard `isAdmin` in `mutationFn` with `Admin access is required to reload the public GraphQL schema.`
   - Use `graphqlClient.request<ReloadSchemaResponse>(RELOAD_SCHEMA)`.
   - Handle `response.reloadSchema.success` in `onSuccess` as described above.
   - Handle transport/GraphQL errors in `onError` as described above.
6. Add the new admin-only `Public GraphQL schema` section after the upload section in the existing admin controls block.
7. Replace the stale upload-card restart copy.
8. Update docs:
   - `README.md:53` restart paragraph: replace with the plan wording and ideally include the `reloadSchema` mutation example.
   - `README.md:141` typed query paragraph: say new typed fields appear after running `reloadSchema` or clicking **Reload schema**, not after backend restart.
   - `README.md:314-321` admin mutation list: add `reloadSchema` with a short description.
   - `client/README.md:36` Vercel note: explain that Vercel redeploy only affects frontend; use `reloadSchema`/Lexicons page button against the backend instead of restart/redeploy.
9. Add Changie fragment, recommended filename:
   - `.changes/unreleased/add-manual-schema-reload-ui.yaml`
   - Suggested body:
     ```yaml
     kind: added
     body: Add a Lexicons page action and docs so operators can reload the public GraphQL schema after lexicon changes without restarting the backend.
     custom:
       Affects: operator
     ```

## Backend API assumptions

- Admin GraphQL mutation name is exactly `reloadSchema`.
- Result fields are exactly:
  - `success: boolean`
  - `lexiconCount: number`
  - `reloadedAt: string | null`
  - `error: string | null`
- Reload build/validation failures return a normal GraphQL data payload with `success: false`; they should not reject the GraphQL request.
- Authorization errors, missing callback wiring, proxy/config failures, and unexpected resolver errors may reject the `graphqlClient.request` promise.
- On failure, `lexiconCount` is the **currently active public schema count**, not the attempted source count.
- On failure with a previous schema, the previous public schema remains active.
- On failure with no previous schema, no public schema is active yet.
- Reload does not modify Tap/Jetstream ingestion filters and does not restart the backend.

## Tests/validation

Practical with current setup:

1. Add a simple mutation-document unit test, e.g. `client/src/lib/graphql/mutations.test.ts`, asserting `RELOAD_SCHEMA` contains:
   - `mutation ReloadSchema`
   - `reloadSchema`
   - `success`, `lexiconCount`, `reloadedAt`, `error`
2. If message formatting is extracted to small pure helpers, add plain Vitest tests for:
   - success count pluralization (`1 lexicon`, `2 lexicons`)
   - failure with active schema count includes `active schema count, not the failed reload attempt`
   - failure with zero count says no previous schema is active

Not currently practical without adding frontend test dependencies:

- Full Lexicons page click tests for admin button, loading state, success render, and failure render. There is no React Testing Library/jsdom setup in `client/package.json`, and no existing component-test pattern.

Manual validation after backend API is available:

1. Log in as an admin.
2. Open `/lexicons`; verify the new section is visible only for admins.
3. Click **Reload schema**; button shows loading/pending state.
4. With successful backend response, verify success alert: `Reloaded public schema with N lexicons.`
5. With a backend payload failure (`success: false`), verify error alert includes backend error and previous-schema/active-count semantics.
6. With unauthorized/misconfigured backend response, verify error alert starts `Could not start schema reload:`.
7. Confirm no restart/redeploy language remains in README/client README/Lexicons page.

Run before merge:

```bash
npm --prefix client run lint
npm --prefix client run test
npm --prefix client run build
```

If the same branch also includes backend changes, parent issue verification should additionally include the backend commands from `AGENTS.md`.

## Risks/open questions

- **Backend absent in current inspected worktree:** no `reloadSchema` / `ReloadSchemaResult` matches were found under `cmd/` or `internal/`. Treat this as a final integration task only after backend API lands.
- **Payload failure vs thrown error:** `graphql-request` will only hit `onError` for GraphQL/transport errors. A reload validation failure should be rendered from `onSuccess` because the response contains `success: false`.
- **Active count semantics:** do not use the existing admin/statistics `lexiconCount` for reload messages. Use only `reloadSchema.lexiconCount`.
- **Testing gap:** meaningful UI interaction tests require adding test dependencies or extracting pure formatting helpers. Mutation-document tests are easy today.
- **Onboarding flow:** onboarding can upload lexicons (`client/src/app/onboarding/page.tsx:285-349`) but MSR-005 only specifies Lexicons page UI/docs. Decide separately whether onboarding should mention visiting Lexicons page to reload schema after upload.
- **Docs wording:** ensure new docs mention both fallback behavior and ingestion-filter non-goal so operators do not expect Tap/Jetstream filters to change.

## Compact worker meta-prompt for GON-63 / MSR-005

Goal: Implement frontend/docs/Changie integration for manual public GraphQL schema reload after backend `reloadSchema` is available.

Context/evidence:
- Frontend uses TanStack React Query + `graphql-request`, not Apollo (`client/src/app/lexicons/page.tsx:3-7`, `client/src/lib/graphql/client.ts:23-35`).
- Add `RELOAD_SCHEMA` in `client/src/lib/graphql/mutations.ts` with fields `success lexiconCount reloadedAt error` per `docs/manual-schema-reload-plan.md:212-225`.
- Lexicons page admin controls live at `client/src/app/lexicons/page.tsx:474-555`; current stale restart copy is at `:524`.
- Status alerts are shared page-level `error`/`success` at `client/src/app/lexicons/page.tsx:466-472`.
- Docs requiring updates: `README.md:53`, `README.md:141`, `README.md:314-321`, `client/README.md:36`.
- Changie fragment must use `Affects: operator` per `docs/manual-schema-reload-plan.md:288-290` and `.changie.yaml:37-44`.

Success criteria:
- Admin Lexicons page shows `Public GraphQL schema` section with clear reload/in-place/no-restart copy and Tap/Jetstream warning.
- Admin can click `Reload schema`; pending/success/failure states render with exact fallback semantics.
- `success: false` payloads show backend error and previous-schema active-count copy; rejected requests show `Could not start schema reload: ...`.
- README/client README no longer tell operators to restart/redeploy after lexicon changes; they mention `reloadSchema` or the Lexicons page button, fallback behavior, and ingestion filters being separate.
- Operator-facing Changie fragment added.
- Practical tests added at least for mutation document; UI tests added only if test setup/deps make that reasonable.

Hard constraints:
- Do not start until backend `reloadSchema` shape is present/stable; if absent, stop and ask for backend integration branch/status.
- Do not change ingestion behavior or imply reload updates Tap/Jetstream filters.
- Do not use stats/DB lexicon count for active schema count messaging.

Suggested approach:
- Add mutation + response typing, add `useMutation` on Lexicons page, insert admin-only section after upload, replace stale restart copy, update docs, add Changie fragment.
- Prefer existing page-level alerts and Button loading behavior for consistency.

Validation:
- `npm --prefix client run lint`
- `npm --prefix client run test`
- `npm --prefix client run build`
- Manual browser/admin check against backend for success, `success:false`, and rejected-request paths.

Stop/escalation rules:
- Stop if backend mutation fields differ from the planned shape.
- Stop if product wants source-counts or `reloadedAt` displayed; those are not required by MSR-005.
- Stop before adding new test dependencies solely for UI tests unless approved.

Resolved assumptions:
- GON-63 corresponds to draft MSR-005.
- `reloadedAt` is optional and does not need to be displayed in first UI pass.
- Existing top-level Lexicons page alerts are acceptable for reload status.
