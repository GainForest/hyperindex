-- hyperindex:no-transaction
-- Some deployments created this index by hand to speed up
-- ORDER BY json->>'createdAt'. createdAt sorting now uses record_created_at,
-- so the index only costs write amplification and disk.
DROP INDEX CONCURRENTLY IF EXISTS idx_record_collection_json_created;
