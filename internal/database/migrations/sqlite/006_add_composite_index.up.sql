-- Composite index for keyset pagination: covers WHERE collection = ? ORDER BY indexed_at DESC, uri DESC
CREATE INDEX IF NOT EXISTS idx_record_collection_keyset ON record(collection, indexed_at DESC, uri DESC);
