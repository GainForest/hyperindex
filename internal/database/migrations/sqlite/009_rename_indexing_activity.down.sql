-- Restore the previous Jetstream-specific activity table name.
ALTER TABLE indexing_activity RENAME TO jetstream_activity;

DROP INDEX IF EXISTS idx_indexing_activity_timestamp;
DROP INDEX IF EXISTS idx_indexing_activity_rkey;

CREATE INDEX IF NOT EXISTS idx_jetstream_activity_timestamp ON jetstream_activity(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_jetstream_activity_rkey ON jetstream_activity(rkey);
