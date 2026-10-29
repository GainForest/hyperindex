-- hyperindex:no-transaction
-- Typed collection connections sorted by createdAt order by record_created_at
-- and keep rows whose createdAt is missing or unparseable (record_created_at
-- IS NULL), placing them last when newest-first. The partial timeline indexes
-- from 010 and 017 exclude those rows and sort NULLs first, so they cannot
-- serve that ORDER BY. These full indexes use NULLS LAST to match it:
--   idx_record_collection_created_at         collection-wide reads
--   idx_record_collection_created_at_valid   the same, for validated reads
--   idx_record_author_collection_created_at  reads filtered by did
-- The partial indexes from 010 and 017 stay: readers outside Hyperindex, such as
-- the GainForest feed service, order NULLS FIRST and depend on them. Drop them
-- once those readers use NULLS LAST. idx_record_timeline_author_collection_created
-- also stays for recordTimeline's author-scoped reads.
--
-- CONCURRENTLY keeps writes to record flowing during the build, so this file
-- runs outside a transaction. If a build fails or is cancelled, PostgreSQL
-- leaves an INVALID index under the same name; the runner then refuses to
-- continue. Run DROP INDEX CONCURRENTLY <name>; and restart to rebuild it.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_collection_created_at
ON record (collection, record_created_at DESC NULLS LAST, uri DESC);

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_collection_created_at_valid
ON record (collection, record_created_at DESC NULLS LAST, uri DESC)
WHERE validation_status = 'valid';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_author_collection_created_at
ON record (did, collection, record_created_at DESC NULLS LAST, uri DESC);
