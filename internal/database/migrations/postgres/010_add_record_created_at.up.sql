-- Add a materialized creation timestamp for generic record timelines.
ALTER TABLE record
  ADD COLUMN record_created_at TIMESTAMP WITH TIME ZONE;

CREATE INDEX IF NOT EXISTS idx_record_timeline_author_collection_created
ON record (did, collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_record_timeline_collection_created
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL;
