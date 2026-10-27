-- Timeline reads that only want valid records (the GainForest feed service
-- filters validation_status = 'valid') otherwise walk
-- idx_record_timeline_collection_created and fetch every invalid row from the
-- heap just to discard it. A collection with many invalid records then costs
-- one random read per skipped row on a cold cache (seconds per page).
--
-- This transactional build blocks writes on large tables; operators should
-- create the same index out of band first so IF NOT EXISTS skips the build:
--   CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_timeline_valid
--   ON record (collection, record_created_at DESC, uri DESC)
--   WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
CREATE INDEX IF NOT EXISTS idx_record_timeline_valid
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
