-- Append-only version history for opted-in collections
-- (RECORD_HISTORY_COLLECTIONS). One row per distinct record version (CID) the
-- indexer observes, plus delete tombstones. `baseline` rows seed the version
-- that was current when history was switched on for a collection.
CREATE TABLE IF NOT EXISTS record_version (
  id BIGSERIAL PRIMARY KEY,
  uri TEXT NOT NULL,
  cid TEXT NOT NULL,
  did TEXT NOT NULL,
  collection TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('baseline', 'create', 'update', 'delete')),
  json JSONB,
  live BOOLEAN,
  observed_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- A version is recorded once, however often Tap redelivers or resyncs it.
CREATE UNIQUE INDEX IF NOT EXISTS idx_record_version_uri_cid
ON record_version(uri, cid) WHERE action <> 'delete';
CREATE INDEX IF NOT EXISTS idx_record_version_uri_id ON record_version(uri, id);
CREATE INDEX IF NOT EXISTS idx_record_version_collection_observed
ON record_version(collection, observed_at DESC);
