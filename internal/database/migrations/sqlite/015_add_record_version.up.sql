-- Append-only version history for opted-in collections
-- (RECORD_HISTORY_COLLECTIONS). See the PostgreSQL migration for details.
CREATE TABLE IF NOT EXISTS record_version (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  uri TEXT NOT NULL,
  cid TEXT NOT NULL,
  did TEXT NOT NULL,
  collection TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('baseline', 'create', 'update', 'delete')),
  json TEXT,
  live INTEGER,
  observed_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_record_version_uri_cid
ON record_version(uri, cid) WHERE action <> 'delete';
CREATE INDEX IF NOT EXISTS idx_record_version_uri_id ON record_version(uri, id);
CREATE INDEX IF NOT EXISTS idx_record_version_collection_observed
ON record_version(collection, observed_at DESC);
