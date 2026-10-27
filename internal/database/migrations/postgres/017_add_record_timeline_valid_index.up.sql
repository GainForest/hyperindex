-- Timeline reads that only want valid records (the GainForest feed service
-- filters validation_status = 'valid') otherwise walk
-- idx_record_timeline_collection_created and fetch every invalid row from the
-- heap just to discard it. A collection with many invalid records then costs
-- one random read per skipped row on a cold cache (seconds per page).
--
-- Migrations run inside a transaction, so this file cannot build the index
-- CONCURRENTLY. The fallback build below blocks writes to record until it
-- finishes; on large tables operators must create the index out of band first
-- so IF NOT EXISTS skips the build:
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_timeline_valid
--   ON record (collection, record_created_at DESC, uri DESC)
--   WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
--
-- A failed or cancelled concurrent build leaves an invalid index behind under
-- the same name. IF NOT EXISTS would accept it and mark 017 applied with no
-- usable index, so refuse to continue instead.
DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM pg_index i
    WHERE i.indexrelid = to_regclass('idx_record_timeline_valid')
      AND NOT i.indisvalid
  ) THEN
    RAISE EXCEPTION 'index idx_record_timeline_valid exists but is invalid (a CREATE INDEX CONCURRENTLY build failed or was cancelled)'
      USING HINT = 'Run DROP INDEX CONCURRENTLY idx_record_timeline_valid; then retry the concurrent build, or let this migration build it.';
  END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_record_timeline_valid
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
