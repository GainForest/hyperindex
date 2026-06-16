-- Rename the ingestion activity log now that Tap is the primary record source.
ALTER TABLE jetstream_activity RENAME TO indexing_activity;

DROP INDEX IF EXISTS idx_jetstream_activity_timestamp;
DROP INDEX IF EXISTS idx_jetstream_activity_rkey;

CREATE INDEX IF NOT EXISTS idx_indexing_activity_timestamp ON indexing_activity(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_indexing_activity_rkey ON indexing_activity(rkey);
