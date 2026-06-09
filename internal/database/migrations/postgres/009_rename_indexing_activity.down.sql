-- Restore the previous Jetstream-specific activity table name.
ALTER TABLE IF EXISTS indexing_activity RENAME TO jetstream_activity;
ALTER INDEX IF EXISTS indexing_activity_pkey RENAME TO jetstream_activity_pkey;
ALTER SEQUENCE IF EXISTS indexing_activity_id_seq RENAME TO jetstream_activity_id_seq;
ALTER INDEX IF EXISTS idx_indexing_activity_timestamp RENAME TO idx_jetstream_activity_timestamp;
ALTER INDEX IF EXISTS idx_indexing_activity_rkey RENAME TO idx_jetstream_activity_rkey;
