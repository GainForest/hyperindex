# Record Validation Gate implementation spec

## Summary

Record Validation Gate keeps Hyperindex's raw indexing layer complete while preventing schema-divergent records from breaking typed GraphQL queries.

Hyperindex should continue storing every observed AT Protocol record in the generic `record` table. Validation metadata on that row determines whether the record is safe to serve through lexicon-generated, typed GraphQL collection fields.

The core rule is:

> The `record` table means “Hyperindex observed this record.” Typed GraphQL visibility means “this record conforms to the saved lexicon used to generate this API shape.”

## Goals

- Store all observed records, including records that are invalid under the saved lexicon or whose collection has no saved lexicon.
- Validate records against Hyperindex's saved lexicons only. Normal record ingestion must not perform DNS, PLC, PDS, or remote schema resolution.
- Exclude invalid and unknown-schema records from typed GraphQL collection queries.
- Keep generic/raw record access available for debugging and operational visibility.
- Handle the current lexicon update workflow: delete the old lexicon through the UI, then upload or register the replacement lexicon.
- Reclassify existing records when a lexicon becomes available again.

## Non-goals

- Do not reject observed records before storage.
- Do not introduce a full ORM.
- Do not add a durable job queue in the first implementation.
- Do not add a new admin UI for validation inspection in the first implementation.
- Do not auto-resolve unknown lexicons remotely during ingestion.

## Data model

Extend the generic `record` table. Do not create per-lexicon record tables.

Suggested columns:

```sql
ALTER TABLE record ADD COLUMN validation_status TEXT NOT NULL DEFAULT 'unknown_schema';
ALTER TABLE record ADD COLUMN validation_error TEXT;
ALTER TABLE record ADD COLUMN validated_at TEXT;
ALTER TABLE record ADD COLUMN lexicon_hash TEXT;
```

Suggested indexes for both SQLite and PostgreSQL:

```sql
CREATE INDEX idx_record_collection_validation
  ON record(collection, validation_status);

CREATE INDEX idx_record_collection_lexicon_hash
  ON record(collection, lexicon_hash);
```

`lexicon_hash` is a fingerprint of the exact saved lexicon JSON bytes used to classify the record, for example `sha256(schema_json)`. Do not canonicalize the JSON before hashing in the first implementation. If the saved lexicon bytes change, records with an old hash are stale and should be classified against the current schema.

## Validation statuses

Use a small explicit status set:

| Status | Meaning | Typed GraphQL visibility |
| --- | --- | --- |
| `valid` | The record conforms to the saved lexicon for its collection. | Visible |
| `invalid` | A saved lexicon exists, but the record does not conform to it. | Hidden |
| `unknown_schema` | No saved lexicon is available for the collection. | Hidden |
| `validation_error` | Hyperindex could not complete validation because of an internal validation/parsing error. | Hidden |

`validation_error` should describe what went wrong and what to do next where possible. Example messages:

- `no saved lexicon for collection org.example.foo`
- `missing required field: name`
- `field amount expected integer, got string`
- `lexicon removed for collection`
- `failed to parse saved lexicon: <details>`

## Ingestion behavior

All ingestion paths should follow the same flow:

1. Receive a record event from Tap, Jetstream, or backfill.
2. Save the raw record into the generic `record` table.
3. Classify the stored record against the saved lexicon registry.
4. Update validation metadata on the record row.
5. Publish typed subscription events only if the record is `valid`.

Relevant entry points:

- Tap: `internal/tap/handler.go`
- Jetstream: `internal/jetstream/consumer.go`
- Backfill: `internal/backfill/`
- Record repository: `internal/database/repositories/records.go`

For create/update events, storage should happen before classification so invalid data remains available for debugging.

For delete events, delete the raw row as today. No validation is needed for a deleted record.

## Local validator service

Add a local validator service, likely under `internal/validation/` or `internal/lexicon/validation/`.

Suggested API shape:

```go
type ValidationStatus string

const (
    ValidationStatusValid          ValidationStatus = "valid"
    ValidationStatusInvalid        ValidationStatus = "invalid"
    ValidationStatusUnknownSchema  ValidationStatus = "unknown_schema"
    ValidationStatusValidationError ValidationStatus = "validation_error"
)

type ValidationResult struct {
    Status      ValidationStatus
    Error       string
    LexiconHash string
}

type RecordValidator interface {
    ValidateRecord(collection string, rkey string, rawJSON []byte) ValidationResult
    LexiconHash(collection string) (string, bool)
}
```

The validator should use the same saved lexicon registry that drives generated GraphQL types. That keeps validation and serving behavior aligned.

The validator must not:

- query `_lexicon` DNS records,
- resolve DID documents,
- fetch `com.atproto.lexicon.schema` from a PDS,
- mutate the lexicon registry during record ingestion.

## Repository methods

Add repository methods instead of writing SQL in handlers.

Suggested methods:

```go
func (r *RecordsRepository) UpdateValidationStatus(
    ctx context.Context,
    uri string,
    status ValidationStatus,
    validationError string,
    lexiconHash string,
) error
```

```go
func (r *RecordsRepository) MarkCollectionUnknownSchema(
    ctx context.Context,
    collection string,
    reason string,
) error
```

`MarkCollectionUnknownSchema` should execute a collection-wide update:

```sql
UPDATE record
SET validation_status = 'unknown_schema',
    validation_error = ?,
    validated_at = CURRENT_TIMESTAMP,
    lexicon_hash = NULL
WHERE collection = ?;
```

Use `NOW()` for PostgreSQL and the existing repository dialect helpers/placeholders where needed.

Add a batch listing method for schema-available classification after lexicon upload/register:

```go
func (r *RecordsRepository) ListRecordsNeedingValidation(
    ctx context.Context,
    collection string,
    currentLexiconHash string,
    afterURI string,
    limit int,
) ([]Record, error)
```

Suggested predicate:

```sql
WHERE collection = ?
  AND (
    validation_status != 'valid'
    OR lexicon_hash IS NULL
    OR lexicon_hash != ?
  )
  AND uri > ?
ORDER BY uri
LIMIT ?;
```

The `uri > ?` keyset predicate avoids offset pagination over large tables.

## Typed GraphQL serving

Typed GraphQL collection fields must read only valid rows.

Example rule:

```sql
WHERE collection = ?
  AND validation_status = 'valid'
```

Apply this to:

- generated collection connection resolvers,
- single-record typed collection resolvers,
- counts for typed collections,
- typed collection subscriptions.

Generic/raw record queries must return all statuses, including `unknown_schema`, `invalid`, and `validation_error` records. Expose validation metadata on generic record results so consumers can understand why a record is not available through typed GraphQL:

- `validationStatus`
- `validationError`
- `validatedAt`
- `lexiconHash`

If filtering is small to add, generic record queries should also allow filtering by `validationStatus`.

## Lexicon lifecycle behavior

Public typed GraphQL schema shape is rebuilt on process restart, not hot-reloaded, in the first implementation. Lexicon upload/register/delete updates validation state immediately, but newly added, removed, or structurally changed typed GraphQL fields require a Hyperindex restart/redeploy to refresh the generated schema.

### Upload/register lexicon

When a lexicon is uploaded or registered:

1. Save the lexicon JSON to the lexicons table.
2. Compute the current `lexicon_hash` for that saved schema.
3. Refresh the in-memory lexicon registry used by validation classification.
4. Classify existing records for the lexicon's collection whose validation result is missing, invalid, unknown, errored, or stale.

This classification can run in a goroutine so the admin request does not block on large collections.

This is not a separate operator-facing “revalidation” workflow. It is the normal consequence of adding a schema: existing records for that collection can now be judged against it.

Suggested in-process scheduler name:

```go
ScheduleValidationRefresh(collection string, reason string)
```

Example reasons:

- `lexicon_registered`
- `lexicon_uploaded`

The first implementation can use an in-memory goroutine with a `running map[string]bool` to avoid duplicate concurrent classification jobs per collection. It does not need a durable queue unless lexicon updates become frequent or multi-instance coordination becomes necessary.

### Delete lexicon

When a lexicon is deleted:

1. Delete the lexicon row.
2. Mark existing records for that collection as `unknown_schema`.
3. Clear their `lexicon_hash`.
4. Refresh the in-memory registry used by validation classification.

Deletion should be synchronous because it is a single collection-wide SQL update and keeps validation state immediately consistent. Public typed GraphQL schema fields for the deleted lexicon are removed after restart/redeploy.

Suggested resolver flow:

```go
func (r *Resolver) DeleteLexicon(ctx context.Context, nsid string) (bool, error) {
    if err := r.repos.Lexicons.Delete(ctx, nsid); err != nil {
        return false, err
    }

    if err := r.repos.Records.MarkCollectionUnknownSchema(
        ctx,
        nsid,
        "lexicon removed for collection",
    ); err != nil {
        return false, err
    }

    r.notifyLexiconChange(ctx)
    return true, nil
}
```

A deleted lexicon does not mean existing records are invalid. It only means Hyperindex no longer has a saved schema to judge that collection.

## Validation refresh worker

When a schema becomes available after upload/register, classify existing rows in batches. Startup classification for existing saved lexicons is synchronous; upload/register refreshes after the server is already running use the goroutine scheduler.

Pseudo-flow:

```go
func (s *ValidationRefreshScheduler) ScheduleValidationRefresh(collection, reason string) {
    s.mu.Lock()
    if s.running[collection] {
        s.mu.Unlock()
        return
    }
    s.running[collection] = true
    s.mu.Unlock()

    go func() {
        defer func() {
            s.mu.Lock()
            delete(s.running, collection)
            s.mu.Unlock()
        }()

        if err := s.refreshCollection(context.Background(), collection, reason); err != nil {
            slog.Warn("validation refresh failed", "collection", collection, "reason", reason, "error", err)
        }
    }()
}
```

```go
func (s *ValidationRefreshScheduler) refreshCollection(ctx context.Context, collection, reason string) error {
    const batchSize = 500

    currentHash, ok := s.validator.LexiconHash(collection)
    if !ok {
        return s.records.MarkCollectionUnknownSchema(ctx, collection, "no saved lexicon for collection")
    }

    var afterURI string
    for {
        records, err := s.records.ListRecordsNeedingValidation(ctx, collection, currentHash, afterURI, batchSize)
        if err != nil {
            return err
        }
        if len(records) == 0 {
            return nil
        }

        for _, rec := range records {
            result := s.validator.ValidateRecord(rec.Collection, rec.RKey, []byte(rec.JSON))
            if err := s.records.UpdateValidationStatus(ctx, rec.URI, result.Status, result.Error, result.LexiconHash); err != nil {
                return err
            }
            afterURI = rec.URI
        }
    }
}
```

## Startup behavior for existing records

The first deployment with Record Validation Gate will add validation columns to a `record` table that may already contain many rows. Before the migration, those rows have no validation status at all. The migration assigns them the conservative default `unknown_schema` because SQL migrations cannot safely validate records against Go lexicon logic.

To avoid a temporary empty typed API window, startup must synchronously classify existing records for saved collection lexicons before the server begins serving GraphQL requests.

Startup flow:

1. Run database migrations.
2. Load saved lexicons from filesystem and the lexicons table.
3. Build the local lexicon registry.
4. Run inline validation refresh for every saved collection lexicon.
5. Log batched progress for each collection.
6. Start serving GraphQL only after startup classification succeeds.

The startup refresh should use the same batch query as the background refresh and classify rows that are `unknown_schema`, invalid, errored, missing a `lexicon_hash`, or stale against the current `lexicon_hash`. It should skip rows already classified against the current `lexicon_hash`. This makes later startups cheap unless lexicons changed or records were left stale.

Progress logs should include at least:

- collection
- reason, e.g. `startup`
- processed count
- valid count
- invalid count
- unknown/error count
- elapsed time

If startup classification fails, startup should fail loudly instead of serving a typed API backed by unclassified records.

## Rollout plan

1. Add SQLite and PostgreSQL migrations for validation metadata.
2. Update `RecordsRepository.Insert` and `BatchInsert` behavior to initialize validation metadata safely.
3. Add repository methods for validation status updates, collection unknown-schema marking, and validation refresh batch selection.
4. Implement the local record validator against saved lexicons.
5. Add synchronous startup classification for saved collection lexicons before GraphQL starts serving.
6. Wire validation into Tap and Jetstream create/update paths.
7. Wire validation into backfill insert and batch insert paths.
8. Filter typed GraphQL collection queries to valid rows only.
9. Preserve generic/raw record visibility and expose validation metadata there.
10. Wire lexicon upload/register to schedule validation refresh in a goroutine.
11. Wire lexicon delete to synchronously mark records unknown-schema.
12. Document that typed GraphQL schema shape refreshes on restart/redeploy, not dynamically.
13. Add tests for both SQLite and PostgreSQL paths.

## Testing requirements

Add or update tests for:

- SQLite migration adds validation columns and indexes.
- PostgreSQL migration adds validation columns and indexes.
- `MarkCollectionUnknownSchema` updates only the target collection.
- `ListRecordsNeedingValidation` returns invalid, unknown, errored, missing-hash, and stale-hash rows but skips current valid rows.
- Ingestion stores invalid records but marks them `invalid`.
- Ingestion stores records with no saved lexicon as `unknown_schema`.
- Typed GraphQL collection queries exclude invalid and unknown-schema rows.
- Generic/raw record queries can still access invalid, unknown-schema, and validation-error rows.
- Generic/raw record query results include `validationStatus`, `validationError`, `validatedAt`, and `lexiconHash`.
- Lexicon delete marks the collection `unknown_schema` and clears `lexicon_hash`.
- Lexicon upload/register classifies previously unknown-schema records.

Per repository policy, database-related tests should cover both SQLite and PostgreSQL where applicable.

## Implementation decisions

- Typed single-record queries should return `null` for invalid, unknown-schema, or validation-error rows. They should not return a GraphQL error for hidden validation states.
- Generic/raw GraphQL should expose validation metadata in the first implementation: `validationStatus`, `validationError`, `validatedAt`, and `lexiconHash`.
- Startup classification for existing saved lexicons should run synchronously before GraphQL starts serving, with batched progress logs. Validation refresh after upload/register should run in a goroutine. Lexicon delete should remain a synchronous bulk update to `unknown_schema`.
- `lexicon_hash` must hash the exact saved JSON bytes from the lexicon repository. Canonical JSON hashing is intentionally out of scope for the first implementation because formatting-only refreshes are acceptable and exact-byte hashing is simpler to reason about.
