DROP INDEX IF EXISTS idx_record_timeline_collection_created;
DROP INDEX IF EXISTS idx_record_timeline_author_collection_created;

ALTER TABLE record DROP COLUMN record_created_at;
