-- Support bounded keyset paging during startup validation refresh. This
-- transactional build blocks writes on large tables; operators may create the
-- same index out of band before rollout so IF NOT EXISTS skips the build.
CREATE INDEX IF NOT EXISTS idx_record_collection_uri
ON record(collection, uri);
