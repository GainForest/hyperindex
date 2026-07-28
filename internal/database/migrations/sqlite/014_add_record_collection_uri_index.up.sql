-- Support bounded keyset paging during startup validation refresh.
CREATE INDEX IF NOT EXISTS idx_record_collection_uri
ON record(collection, uri);
