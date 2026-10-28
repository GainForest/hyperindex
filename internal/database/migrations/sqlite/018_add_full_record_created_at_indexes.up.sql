-- Typed collection connections sorted by createdAt keep rows without a
-- record_created_at and order them as the oldest, which is SQLite's default
-- NULL ordering. The partial timeline indexes exclude those rows, so these full
-- indexes serve connection reads: collection-wide, validated, and filtered by
-- did. The partial indexes from 010 and 017 stay, as on PostgreSQL.
CREATE INDEX IF NOT EXISTS idx_record_collection_created_at
ON record (collection, record_created_at DESC, uri DESC);

CREATE INDEX IF NOT EXISTS idx_record_collection_created_at_valid
ON record (collection, record_created_at DESC, uri DESC)
WHERE validation_status = 'valid';

CREATE INDEX IF NOT EXISTS idx_record_author_collection_created_at
ON record (did, collection, record_created_at DESC, uri DESC);
