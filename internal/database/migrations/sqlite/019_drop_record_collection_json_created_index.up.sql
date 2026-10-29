-- Keeps SQLite and PostgreSQL migration versions aligned. Only PostgreSQL
-- deployments created this index by hand, but dropping it is harmless here.
DROP INDEX IF EXISTS idx_record_collection_json_created;
