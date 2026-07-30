# Record Validation Gate implementation spec

## Summary

Record Validation Gate keeps Hyperindex's raw index complete while preventing schema-divergent records from breaking typed GraphQL.

The core contract is:

> A row in `record` means Hyperindex observed the record. Typed GraphQL visibility means Indigo validated that row against the immutable Lexicon snapshot used to build the running schema.

Hyperindex builds the public GraphQL registry and validator from one saved Lexicon set at startup. Admin Lexicon upload, register, and delete operations stage configuration for the next restart or redeploy; they do not partially hot-reload the running schema or validator.

## Goals

- Store every observed raw record, including invalid records and records with unknown schemas.
- Validate create and update events in memory before persistence.
- Persist raw content and validation metadata together.
- Expose only `valid` rows through typed GraphQL queries and subscriptions.
- Keep generic queries, search, and raw subscriptions complete and diagnostic.
- Use Indigo for AT Protocol data and Lexicon conformance.
- Validate only against saved local Lexicons during ingestion.
- Keep SQLite and PostgreSQL behavior equivalent.
- Reclassify missing or stale rows synchronously before serving after startup.

## Non-goals

- Runtime GraphQL schema hot reload.
- Remote DNS, DID, PLC, or PDS Lexicon discovery during ingestion.
- Rejection of invalid observed records before raw storage.
- A durable validation queue or persisted Lexicon dependency graph.
- Canonical JSON hashing.
- A validation repair UI.

## Persisted schema

Migration 012 adds record validation metadata:

```sql
validation_status TEXT NOT NULL DEFAULT 'unknown_schema'
validation_error  TEXT NULL
validated_at      TIMESTAMP/TEXT NULL
lexicon_hash      TEXT NULL
```

Indexes:

```sql
CREATE INDEX idx_record_collection_validation
  ON record(collection, validation_status);

CREATE INDEX idx_record_collection_lexicon_hash
  ON record(collection, lexicon_hash);
```

Migration 013 adds `lexicon.raw_json TEXT`. New admin writes save both:

- `lexicon.json`, retained as JSON/JSONB for existing behavior
- `lexicon.raw_json`, the exact uploaded or resolved bytes used for hashing

Existing PostgreSQL JSONB rows can only be backfilled from PostgreSQL's normalized `json::text`; their pre-migration formatting cannot be recovered. After migration, new writes preserve exact bytes in both dialects.

Migration 014 adds the startup refresh paging index:

```sql
CREATE INDEX idx_record_collection_uri
  ON record(collection, uri);
```

PostgreSQL builds this index inside the startup migration transaction, which blocks writes to a large `record` table until the build completes. Operators with large datasets can create `idx_record_collection_uri` out of band before rollout; the migration's `IF NOT EXISTS` then skips the blocking build.

## Validation statuses

| Status | Meaning | Typed visibility |
| --- | --- | --- |
| `valid` | Indigo accepted the record against the startup Lexicon snapshot. | Visible |
| `invalid` | A collection Lexicon exists, but the record does not conform to it. | Hidden |
| `unknown_schema` | No collection Lexicon exists in the startup snapshot. | Hidden |
| `validation_error` | Validation could not complete because of malformed JSON, an incomplete validator, or another operational validation failure. | Hidden |

Every attempted classification sets `validated_at`, including invalid, unknown-schema, and validation-error outcomes.

## Startup Lexicon snapshot

Startup performs these steps before serving HTTP or starting ingestion consumers:

1. Run database migrations.
2. Load the CID-pinned Lexicons bundled in the binary.
3. Load optional `LEXICON_DIR` documents, overriding bundled documents with the same NSID.
4. Load database Lexicons, overriding bundled and directory documents with the same NSID.
5. Parse the selected documents into the internal registry used by GraphQL.
6. Add the same documents to an Indigo catalog.
7. Validate complete local reference closure.
8. Compute exact per-Lexicon hashes and transitive collection fingerprints.
9. Mark stored collections absent from the snapshot as `unknown_schema`.
10. Refresh missing or stale validation metadata for active collections.
11. Build and expose GraphQL from the snapshot registry.
12. Start Tap or Jetstream using the same startup collection set unless configuration provides an explicit collection override.

The registry, Indigo catalog, hashes, and generated schema remain fixed for the process lifetime.

Bundled Lexicons are installed from the Atmosphere with `@atproto/lex` and pinned by URI and CID in `lexicons.json`. Filesystem JSON files that do not declare a Lexicon are ignored. Malformed JSON or malformed Lexicon documents fail startup with the file path rather than silently disappearing from the active schema.

## Indigo-backed validation

`internal/validation.Validator` wraps Indigo's `lexicon.ValidateRecord` and uses `atdata.UnmarshalJSON` for AT Protocol data conversion.

For each create or update:

1. Resolve the collection fingerprint from the startup snapshot.
2. Decode the raw JSON as AT Protocol data.
3. Validate the record key using the collection's record key rule.
4. Validate `$type`, required fields, formats, limits, refs, unions, blobs, CID links, bytes, and other Lexicon constraints through Indigo.
5. Return a persistence-ready `validation.Result`.

Supported Lexicon record key rules are `any`, `tid`, `record-key`, `nsid`, and `literal:<value>`. Hyperindex validates `record-key` locally while using Indigo for the record body; unsupported custom rules fail schema validation or fail closed.

If an ingestion component is constructed without a validator, it stores the event as `validation_error`; nil validation never implies `valid`.

## Validation fingerprints

Each saved Lexicon document has an exact-byte hash:

```text
sha256(raw_json)
```

Each collection has a transitive fingerprint:

```text
sha256(
  sorted lines:
    <lexicon-nsid>=<exact-byte-hash>
)
```

The dependency walk starts at the collection's `main` definition and follows every reachable `ref` and `union.refs`, including references nested in arrays, objects, and non-main definitions. A helper Lexicon change therefore makes every dependent collection's old rows stale.

Formatting-only saved Lexicon changes intentionally produce a new fingerprint and trigger refresh. Canonicalization is out of scope.

## Validate first, write once

Tap, Jetstream, and both backfill paths use the same create/update flow:

```text
receive event
classify against startup snapshot
UpsertWithValidation(raw content + validation result)
publish raw event
```

`RecordsRepository.UpsertWithValidation` writes raw content and validation metadata in one upsert. A changed record cannot retain a previous row's `valid` metadata if a second update fails.

`BatchUpsertWithValidation` provides the equivalent transaction/batch path for backfill while staying under SQLite's aggregate parameter limit.

### Same-CID replay

A non-empty incoming CID equal to the existing CID means content is unchanged. The repository returns `Skipped`, preserving ingestion counters, but first repairs:

- missing or stale validation status/error/hash
- missing `validated_at`
- missing `record_created_at` when the incoming JSON supplies one

The repair update includes the existing CID in its predicate so a concurrent content change cannot receive metadata for the old content.

An empty CID is never treated as unchanged merely because the stored CID is also empty.

### Backfill CID handling

Backfill only treats the same URI with the same non-empty CID as unchanged content. It does not drop a record because the same CID exists at a different URI; identical content at distinct AT-URIs remains distinct indexed data.

## Startup validation refresh

Startup selects rows with missing or stale metadata:

```sql
WHERE collection = ?
  AND (
    lexicon_hash IS NULL
    OR lexicon_hash != ?
    OR validated_at IS NULL
  )
  AND uri > ?
ORDER BY uri
LIMIT 500;
```

Status is not part of the predicate. A current-hash `invalid` or `validation_error` row is already classified and is not revalidated on every boot.

Refresh uses keyset paging and logs processed, valid, invalid, hidden, and elapsed counts. Startup fails rather than serving typed GraphQL backed by stale or unclassified rows when refresh cannot complete.

## GraphQL visibility

### Typed queries

These surfaces read only `validation_status = 'valid'` rows:

- typed collection connections and counts
- typed `ByUri` fields
- typed create/update subscriptions
- Certified profile hydration

Typed `ByUri` returns `null`, not a GraphQL error, when a raw row exists but is hidden.

`certifiedProfileData` only attaches a valid `app.certified.actor.profile` row. An invalid or unknown profile cannot bypass the gate through relationship hydration.

### Generic queries and search

`records(collection: ...)` and `search(...)` return raw rows regardless of validation status. Their `GenericRecord` nodes include:

- `validationStatus`
- `validationError`
- `validatedAt`
- `lexiconHash`

This keeps invalid and unknown-schema records available for diagnostics without allowing them into typed fields.

## Subscription behavior

Hyperindex uses one raw pubsub stream.

- Every successfully stored create/update event is published, including invalid, unknown-schema, and validation-error records.
- Every observed delete is published after deletion.
- Generic `recordEvents` receives all matching raw events.
- Typed per-collection create/update fields check the current row and emit only when it is valid.
- Resolver-level filtering produces no WebSocket `next` message; clients do not receive `{ typedField: null }` for suppressed events.

For deletes, ingestion reads the typed-visible row before deletion and carries internal `wasValid` plus the previous typed payload on the event:

```text
read valid row, if present
delete raw row
publish raw delete with pre-delete visibility metadata
```

Typed delete fields emit only when the deleted row was previously valid. Raw delete payloads do not expose the internal visibility metadata.

## Admin Lexicon lifecycle

Admin upload/register/delete changes the database-backed saved configuration only.

Before upload or registration persists anything, Hyperindex:

1. validates each candidate with the internal GraphQL parser and Indigo
2. overlays all candidates on filesystem plus current database Lexicons
3. validates the complete prospective startup set and local reference closure
4. persists only when the full set is valid

ZIP upload validates all candidates before one transactional batch save. Duplicate IDs, ID mismatches, invalid schemas, and unresolved prospective references persist nothing.

Delete also validates the prospective set. A helper Lexicon cannot be deleted while another saved Lexicon would retain a broken reference. Deleting a database override may reveal a valid bundled or filesystem Lexicon with the same NSID.

Mutation return types remain compatible. Logs, admin descriptions, and frontend copy tell operators to restart or redeploy. The running GraphQL schema, validator, validation metadata, backfill defaults, and Jetstream defaults do not change until restart.

## Verification contract

Changes to this feature require:

```bash
go build -v ./...
make lint
DATABASE_URL=sqlite::memory: go test -v -race ./...
```

Database changes also require PostgreSQL parity:

```bash
DATABASE_URL=postgres://hyperindex:hyperindex@localhost:5432/hyperindex_test?sslmode=disable \
  go test -v -race ./...
```

Integration behavior requires:

```bash
go test -v -race -tags=integration ./internal/integration/...
```

When Docker/local Tap dependencies are available:

```bash
make smoke-tap-local
```

Coverage must include atomic valid-to-invalid updates, same-CID repair, both database dialects, transitive fingerprints, prospective admin validation, raw and typed query visibility, Certified profile gating, raw invalid events, typed suppression, and pre-delete typed visibility.
