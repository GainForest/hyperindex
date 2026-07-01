-- Rename the ingestion activity log now that Tap is the primary record source.
ALTER TABLE IF EXISTS jetstream_activity RENAME TO indexing_activity;
ALTER INDEX IF EXISTS jetstream_activity_pkey RENAME TO indexing_activity_pkey;
ALTER SEQUENCE IF EXISTS jetstream_activity_id_seq RENAME TO indexing_activity_id_seq;
ALTER INDEX IF EXISTS idx_jetstream_activity_timestamp RENAME TO idx_indexing_activity_timestamp;
ALTER INDEX IF EXISTS idx_jetstream_activity_rkey RENAME TO idx_indexing_activity_rkey;
