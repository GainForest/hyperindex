# GON-60 / MSR-002 handoff: validate admin lexicon writes before persistence

## Issue and plan evidence

- `plans/manual-schema-reload-linear-created.json:27-31` maps draft `MSR-002` to Linear `GON-60`, title `Validate admin lexicon writes before persistence`.
- `plans/manual-schema-reload-linear-issues.json` says MSR-002 depends on MSR-001/GON-59 if the validation helper/parser path is introduced there. Requirements: validate full lexicon documents before register/upload writes to `lexicon`, use the same parser/validation path as schema reload, reject malformed JSON/registry parse failures/ID mismatches, return actionable errors, and leave DB unchanged on validation failure.
- `docs/manual-schema-reload-plan.md:38-46` confirms invariants: malformed uploaded/registered lexicons must be rejected before DB storage; existing malformed DB rows still fail reload; reload failure keeps the previous schema active.
- `docs/manual-schema-reload-plan.md:122-133` is the direct design section for this issue.
- `oracle/manual-schema-reload-plan-review.md` calls out the same risk: invalid uploaded lexicons can currently be stored because upload only checks top-level `id`.

No `PublicSchemaManager`, `SchemaManager`, `ReloadSchema`, or validator helper exists yet in the current tree. GON-60 should be implemented after GON-59 lands, or stop and ask if GON-59 does not expose a reusable strict parser/validation path.

## Relevant files/functions

### Backend admin persistence paths

- `internal/graphql/admin/resolvers.go`
  - `Repositories` includes `Lexicons *repositories.LexiconsRepository` at `lines 27-39`.
  - `LexiconChangeCallback` is the existing ingestion/Jetstream callback at `lines 47-58`; do not reuse it for schema reload or validation.
  - `notifyLexiconChange(ctx)` reads all DB lexicons and sends IDs to the callback at `lines 85-105`.
  - `Lexicons(ctx)` lists persisted lexicons at `lines 175-192`.
  - `UploadLexicons(ctx, zipBase64 string)` is the upload persistence path at `lines 224-305`.
  - `RegisterLexicon(ctx, nsid string)` is the published NSID persistence path at `lines 516-573`.
  - `DeleteLexicon(ctx, nsid string)` deletes by NSID at `lines 575-593`.

- `internal/graphql/admin/schema.go`
  - `lexicons` admin query is exposed and admin-gated at `lines 86-94`.
  - `uploadLexicons(zipBase64: String!): Int!` is exposed and admin-gated at `lines 489-504`.
  - `registerLexicon(nsid: String!): Lexicon!` is exposed and admin-gated at `lines 634-649`.
  - `deleteLexicon(nsid: String!): Boolean!` is exposed and admin-gated at `lines 651-666`.
  - `requireAdmin` returns `admin privileges required` at `lines 691-698`.

- `internal/graphql/admin/types.go:328-346`
  - `LexiconType` exposes `id`, `json`, and `createdAt`. GON-60 should not need to change this type.

- `internal/graphql/admin/handler.go`
  - Admin handler builds the schema in `NewHandler` at `lines 24-41`.
  - `ServeHTTP` trusts `X-User-DID` only with valid `X-Admin-API-Key`, injects auth context, and executes admin GraphQL at `lines 83-132`.
  - Existing mutation auth is through schema-level `requireAdmin`; validation errors should remain normal GraphQL resolver errors for `uploadLexicons`/`registerLexicon` unless GON-59/GON-61 introduces a payload style for a different mutation.

### Persistence repository/schema

- `internal/database/repositories/lexicons.go`
  - `LexiconsRepository.Upsert(ctx, id, jsonData)` writes to the `lexicon` table at `lines 30-54`.
  - `GetAll` returns all persisted lexicons ordered by ID at `lines 80-108`.
  - `Delete`, `GetCount`, and `Exists` are at `lines 110-142`.
- `internal/database/migrations/sqlite/001_initial_schema.up.sql:31-36`
  - SQLite stores `lexicon.json` as `TEXT`; it does not enforce valid JSON.
- `internal/database/migrations/postgres/001_initial_schema.up.sql:32-37`
  - PostgreSQL stores `lexicon.json` as `JSONB`; it rejects syntactically invalid JSON, but not lexicon-schema-invalid JSON.

Important: keep validation in the admin write path, not in `LexiconsRepository.Upsert`, unless explicitly approved. GON-59 still needs reload to fail on malformed rows already present in the DB; repository-level validation would also make it harder to seed those cases in tests.

### Existing lexicon parser/registry APIs to reuse

- `internal/lexicon/parser.go`
  - `Parse(jsonStr string)` delegates to `ParseBytes` at `lines 8-11`.
  - `ParseBytes(data []byte)` JSON-unmarshals into `rawLexicon` and returns parse errors at `lines 13-21`.
  - `convertRawLexicon` rejects missing top-level `id` at `lines 69-72` and parses every definition at `lines 81-99`.
  - `parseMainDef` rejects unsupported main definition types at `lines 103-121`.
  - Property parsing surfaces nested parse errors at `lines 235-290`.
- `internal/lexicon/registry.go`
  - `NewRegistry()` creates a fresh registry at `lines 25-32`.
  - `Register` indexes lexicons but returns no error and overwrites duplicate IDs at `lines 43-74`.
  - `ParseAndRegister(jsonStr)` calls `Parse` and wraps parse errors at `lines 208-217`.
- `internal/lexicon/nsid.go`
  - `IsValidNSID` exists at `lines 76-99` and is stricter than current admin/frontend checks: at least 3 segments, lowercase/digits/hyphens, no leading/trailing hyphen.

Caveat: today’s parser/registry is not a complete ATProto lexicon semantic validator. The public schema builder currently falls back for unknown refs (`internal/graphql/types/object.go:264-269`, `300-305`) and `Registry.Register` is not itself strict. If GON-59 adds a stricter validation helper, use that helper as the source of truth rather than inventing a new admin-only interpretation.

### Public schema load path that GON-59 will replace

- `cmd/hyperindex/main.go:588-665` currently builds the public schema once at startup.
  - Filesystem lexicons are loaded into a fresh registry (`lines 592-604`).
  - DB lexicons are parsed with `registry.ParseAndRegister(dbLex.JSON)` but parse failures are only logged and skipped (`lines 607-622`).
  - Public HTTP handler is created with `hgraphql.NewHandler(registry, repos)` (`lines 626-639`).
  - Subscription handler gets the startup schema pointer (`lines 641-650`).
- `cmd/hyperindex/main.go:897-921` `loadLexiconsFromDir` currently skips parse errors for filesystem JSON files (`lines 912-915`).
- `internal/graphql/handler.go:15-30` stores a startup-only `*graphql.Schema`; `ServeHTTP` uses it at `lines 56-63`.
- `internal/graphql/subscription/handler.go:45-65`, `100-126`, and `232-240` also use a cached `*graphql.Schema`.
- `internal/graphql/schema/builder.go:53-85` builds a `graphql.Schema` from a registry.

### Frontend/API files touched by lexicon operations

- `client/src/lib/graphql/mutations.ts:43-48` exports `UPLOAD_LEXICONS`.
- `client/src/lib/graphql/mutations.ts:115-131` exports `REGISTER_LEXICON` and `DELETE_LEXICON`.
- `client/src/lib/graphql/queries.ts:67-76` exports `GET_LEXICONS`.
- `client/src/app/lexicons/page.tsx`
  - imports lexicon mutations at `lines 5-11`.
  - `registerMutation` calls `REGISTER_LEXICON` at `lines 264-267`.
  - `uploadMutation` calls `UPLOAD_LEXICONS` and displays backend errors at `lines 269-293`.
  - `deleteMutation` calls `DELETE_LEXICON` and displays backend errors at `lines 295-310`.
  - `handleRegister` submits comma/newline/whitespace-separated NSIDs sequentially at `lines 313-360`.
  - Upload UI still says a backend restart may be required at `lines 517-548`; that is GON-63/manual reload UI territory, not required for GON-60 unless validation copy is changed.
- `client/src/app/onboarding/page.tsx:45-53` also calls `UPLOAD_LEXICONS`; validation errors will surface via the existing `onError`.
- `client/src/app/api/admin/graphql/route.ts:12-52` proxies admin GraphQL to `${HYPERINDEX_URL}/admin/graphql` and passes through backend status/payloads.

GON-60 likely needs no frontend API shape changes. Existing clients already display GraphQL errors. Only touch frontend if you intentionally improve validation error copy or add tests around error rendering.

## Current behavior

### Upload ZIP (`UploadLexicons`)

Current upload flow (`internal/graphql/admin/resolvers.go:224-305`):

1. Checks base64 length, decodes base64, opens ZIP, checks total file count.
2. Iterates files and skips directories/non-`.json` files.
3. Checks uncompressed size and reads each file with a limit.
4. Uses `json.Unmarshal` only to extract top-level `id` (`lines 281-287`).
5. Silently skips invalid JSON and missing `id` (`lines 285-290`).
6. Immediately calls `Lexicons.Upsert(ctx, lexEntry.ID, string(data))` for each accepted file (`lines 293-296`).
7. Returns the count of upserted files and notifies the lexicon change callback if `count > 0` (`lines 300-305`).

Problems for GON-60:

- No full `lexicon.Parse` / `Registry.ParseAndRegister` validation.
- Valid JSON with an invalid lexicon shape can be persisted.
- Invalid JSON/missing IDs are silently skipped, not actionable.
- Mixed ZIPs can partially persist valid files before a later validation failure unless implementation switches to validate-all-before-write.

### Register by NSID (`RegisterLexicon`)

Current register flow (`internal/graphql/admin/resolvers.go:516-573`):

1. Checks only `len(strings.Split(nsid, ".")) >= 3` (`lines 518-522`).
2. Rejects if the requested NSID already exists (`lines 524-531`).
3. Resolves the lexicon through `lexicon.NewResolver().ResolveLexicon(ctx, nsid)` (`lines 533-538`).
4. Stores `resolved.Schema` as-is under the requested `nsid` (`lines 540-544`).
5. Notifies ingestion collection callback (`lines 546-547`).
6. Best-effort parses JSON only for description; errors are ignored (`lines 549-557`).

Problems for GON-60:

- The fetched schema is not parsed/registered before persistence.
- The top-level JSON `id` is not compared to the requested `nsid`, so mismatches can be stored ambiguously.
- The DNS/PDS resolver is hard-coded, which makes register validation tests hard without adding a small injectable resolver/fetcher seam.

### Delete / re-add

- `DeleteLexicon` only checks existence, deletes, and notifies (`internal/graphql/admin/resolvers.go:575-593`). It does not write JSON, so GON-60 should not add parser validation here.
- “Re-add” is not a separate backend/frontend code path. It means calling `registerLexicon` or `uploadLexicons` after deletion, so it inherits the new validation once those write paths are fixed.

## Proposed smallest safe implementation after GON-59 lands

1. Reuse GON-59’s strict parser/validation helper.
   - Preferred shape: a pure helper callable without mutating the live public schema, e.g. `ValidateLexiconDocument(source string, raw []byte) (*lexicon.Lexicon, error)` or equivalent.
   - It should be the same code path the schema manager uses to parse/register DB and filesystem lexicons during reload.
   - It should return the parsed `Lexicon.ID` and wrap errors with source context.

2. Add one small admin-side wrapper for ID expectations.
   - Example behavior: `validateAdminLexiconDocument(source, raw, expectedID)`:
     - calls the GON-59 validator;
     - if `expectedID != "" && parsed.ID != expectedID`, returns `lexicon <source> id mismatch: request was <expectedID> but document id is <parsed.ID>`;
     - returns parsed ID and raw JSON string for persistence.
   - For `registerLexicon`, `expectedID` is the requested `nsid`.
   - For `uploadLexicons`, there is currently no separate API-supplied ID; use the parsed document ID as the DB key. Do not invent filename-as-ID mismatch validation unless product explicitly wants that.

3. Change `UploadLexicons` to validate all JSON files before any DB writes.
   - Keep existing base64/ZIP/file count/file size protections.
   - For each `.json` file, read data and run the shared validator.
   - Treat invalid JSON, missing ID, parser/register failure, or strict validator failure as a resolver error that identifies the ZIP member name.
   - Accumulate validated docs in memory as `[]struct{ id, json, source string }`.
   - Only after all JSON files validate, loop over the accumulated docs and call `Lexicons.Upsert`.
   - Only call `notifyLexiconChange` after successful persistence of at least one doc.
   - This preserves the existing DB state on validation failure without requiring repository-level changes.

4. Change `RegisterLexicon` to validate the fetched schema before `Upsert`.
   - Keep the existing duplicate check and resolver call.
   - After `ResolveLexicon`, validate `resolved.Schema` through the GON-59 helper with `expectedID = nsid`.
   - Persist under the parsed/expected ID only after validation passes.
   - Use parsed metadata for the response if helpful, but do not rely on best-effort `json.Unmarshal` as validation.

5. Add a tiny test seam for register resolution.
   - Current `RegisterLexicon` constructs `lexicon.NewResolver()` directly, making unit tests hit real DNS/PDS if they exercise register.
   - Smallest useful seam: add an unexported interface or function field on `admin.Resolver`, e.g. `resolveLexicon func(ctx context.Context, nsid string) (*lexicon.ResolvedLexicon, error)`, defaulting to `lexicon.NewResolver().ResolveLexicon`.
   - Tests can replace the function with a fake returning valid, malformed, or ID-mismatched schemas.

6. Do not change `DeleteLexicon` for validation.
   - It should remain a persistence deletion plus ingestion callback.
   - Manual schema reload semantics are handled by GON-59/GON-61/GON-62.

7. Avoid changing public API shape in GON-60.
   - `uploadLexicons` can still return `Int!` on success and GraphQL errors on failure.
   - `registerLexicon` can still return `Lexicon!` on success and GraphQL errors on failure.

## Interface needs from GON-59

GON-60 needs one reusable, non-mutating strict validation entrypoint from the GON-59 foundation. Minimum useful contract:

```go
// Name/package are flexible; behavior is the contract.
type LexiconDocumentValidator interface {
    ValidateLexiconDocument(source string, raw []byte) (*lexicon.Lexicon, error)
}
```

Behavior needed by GON-60:

- Uses the same parser/register/build strictness as schema reload for an individual lexicon document or a batch helper.
- Does not mutate the live public schema manager snapshot.
- Returns parsed `lexicon.ID` for persistence and ID mismatch checks.
- Returns actionable errors that include the source name (`register <nsid>` or ZIP member path) and parser cause.
- Has a batch-friendly form if strict validation needs cross-document context for ZIP uploads, e.g. `ValidateLexiconDocuments([]SourceDocument)`.

If GON-59 only exposes `PublicSchemaManager.Reload(ctx)` and no reusable validation helper, pause and ask before duplicating parser logic in admin. The issue explicitly requires admin write validation and reload validation not to drift.

## Tests to add

Prefer a new backend test file such as `internal/graphql/admin/resolvers_lexicons_test.go`. Use `internal/testutil/db.go:30-65` to create an in-memory SQLite DB with repositories.

Core tests:

1. `UploadLexicons` rejects malformed JSON.
   - Build a ZIP with `bad.json` containing `{not json`.
   - Call resolver directly with admin repos/test DB.
   - Expect error containing `bad.json` and parse cause.
   - Assert `Lexicons.GetCount(ctx) == 0`.

2. `UploadLexicons` rejects registry/parser failures.
   - ZIP file contains valid JSON but invalid lexicon document, e.g. missing top-level `id` or `defs.main.type` unsupported.
   - Expect no DB rows.

3. `UploadLexicons` is validate-before-write for mixed ZIPs.
   - Seed one existing row.
   - ZIP contains one valid lexicon and one invalid lexicon.
   - Expect error and DB still contains only the seed row; the valid lexicon from the failed ZIP is not inserted.

4. `UploadLexicons` stores all valid lexicons.
   - ZIP contains one or more valid lexicon JSON files.
   - Expect returned count and DB rows with expected IDs/JSON.
   - If testing callback behavior, assert `notifyLexiconChange` fires once after successful writes and does not fire on validation failure.

5. `RegisterLexicon` rejects malformed fetched schema before DB write.
   - Requires injectable fake resolver.
   - Fake returns `ResolvedLexicon{Schema: []byte(...)}` with invalid lexicon shape.
   - Expect error and no row for requested NSID.

6. `RegisterLexicon` rejects ID mismatch before DB write.
   - Fake request `com.example.expected`; returned JSON has `id: "com.example.other"`.
   - Expect error mentioning both IDs and no row under either ID.

7. `RegisterLexicon` still stores valid schemas.
   - Fake returns schema whose `id` equals requested NSID.
   - Expect `Lexicons.GetByID` succeeds and returned admin payload includes `id/json/createdAt`.

8. Keep existing malformed DB row behavior owned by GON-59.
   - Do not add repository-level tests that require `LexiconsRepository.Upsert` to reject malformed lexicons.
   - GON-59 should still test reload failure on malformed existing rows.

Existing adjacent tests:

- `internal/lexicon/parser_test.go:94-117` already covers invalid JSON and missing ID parser errors.
- `internal/lexicon/registry_test.go:7-30` covers successful `ParseAndRegister`.
- `internal/database/repositories/lexicons_test.go:19-55` covers raw repository upsert/get and should not need changes unless repository APIs change.
- `internal/graphql/admin/handler_test.go:386-404` already tests `requireAdmin`; GON-60 does not need new auth tests unless validation is exposed through new schema fields.

## Likely commands to run

Targeted first:

```bash
go test -v ./internal/graphql/admin -run 'Test.*Lexicon'
go test -v ./internal/lexicon -run 'TestParse|TestRegistry'
go test -v ./internal/database/repositories -run TestLexiconsRepository
```

Full backend verification before finishing:

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

Run PostgreSQL full tests too if repository SQL, transactions, or dialect behavior changes:

```bash
DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable go test -v -race ./...
```

Frontend commands are not expected for GON-60 unless frontend files are touched:

```bash
npm --prefix client run lint
npm --prefix client run test
npm --prefix client run build
```

## Risks and open questions

- GON-59 interface is unknown in the current worktree. If it does not expose a reusable validator, stop and ask rather than duplicating validation logic.
- Current parser strictness is limited. It catches invalid JSON, missing ID, unsupported main definition type, and property parse errors, but it does not fully validate ATProto semantics or unresolved refs. GON-60 should match GON-59 strictness, not invent a different standard.
- Upload behavior will change from “silently skip bad JSON/missing ID and persist the rest” to “return an actionable error and persist none of the ZIP if validation fails.” This is operator-visible and should get a Changie fragment with `Affects: operator` unless a parent/manual-reload fragment covers it.
- `RegisterLexicon` currently uses real DNS/PDS resolution directly. Add a small injectable fake resolver seam for tests; otherwise register validation cannot be tested without external calls.
- There is no separate upload ID today. ID mismatch applies clearly to `registerLexicon(nsid)` because the request NSID is separate from the fetched JSON body. Treating ZIP filenames as expected IDs would be a new product decision.
- Do not couple validation errors to the schema reload callback or the existing `LexiconChangeCallback`. The latter is for ingestion collection updates only.

## Compact worker meta-prompt

Goal: Implement GON-60/MSR-002 after GON-59 lands by validating admin `uploadLexicons` and `registerLexicon` lexicon documents before any `lexicon` table writes, using the same strict parser/validation path as the schema reload manager.

Evidence/context: Current write paths are `internal/graphql/admin/resolvers.go` `UploadLexicons` (`224-305`) and `RegisterLexicon` (`516-573`), exposed in `internal/graphql/admin/schema.go` (`489-504`, `634-649`). Current upload only extracts top-level `id` and silently skips invalid files; current register stores fetched JSON without parsing or ID mismatch checks. Persistence is `internal/database/repositories/lexicons.go:30-54`. Reuse GON-59’s validator; current local parser APIs are `lexicon.Parse/ParseBytes` and `Registry.ParseAndRegister`.

Success criteria:
- Malformed upload/register documents fail before DB persistence.
- Parser/register validation failures fail before DB persistence.
- Register ID mismatch between requested NSID and JSON top-level `id` fails before DB persistence.
- Upload ZIP validation is all-before-write: if any JSON lexicon is invalid, no docs from that ZIP are persisted.
- Valid upload/register still persists and still notifies the existing lexicon-change callback after successful writes.
- Existing malformed DB rows are still possible and still handled by GON-59 reload failure tests; do not move this validation into `LexiconsRepository.Upsert`.

Hard constraints:
- Do not implement a second validation standard if GON-59 exposes a strict helper; reuse it.
- Do not change delete semantics for GON-60.
- Do not use `LexiconChangeCallback` for schema reload.
- Avoid frontend/API shape changes unless explicitly needed.

Suggested approach:
- Add an admin validation helper wrapping the GON-59 validator and expected-ID checks.
- Make `UploadLexicons` collect and validate all ZIP JSON files before calling `Upsert`.
- Add a tiny injectable fake resolver seam so `RegisterLexicon` can be tested without DNS/PDS.
- Keep resolver errors actionable and source-specific.

Validation:
- Add focused tests in `internal/graphql/admin` for upload malformed JSON, upload parser failure, mixed ZIP DB-unchanged, upload success, register malformed fetched schema, register ID mismatch, and register success.
- Run targeted admin/lexicon/repository tests, then full backend `go build`, `make lint`, and SQLite race tests. Add PostgreSQL full tests if repository/dialect code changes.

Stop/escalation:
- If GON-59 does not provide a reusable strict validator, ask before duplicating parser/build logic.
- If strict validation requires cross-document/full-registry context, ask whether upload should validate against only ZIP contents or current DB+filesystem+ZIP candidate set.
- If product wants ZIP filenames treated as expected IDs, ask before adding that behavior.
