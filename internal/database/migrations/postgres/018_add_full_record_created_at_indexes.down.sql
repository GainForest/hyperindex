-- hyperindex:no-transaction
DROP INDEX CONCURRENTLY IF EXISTS idx_record_author_collection_created_at;
DROP INDEX CONCURRENTLY IF EXISTS idx_record_collection_created_at_valid;
DROP INDEX CONCURRENTLY IF EXISTS idx_record_collection_created_at;
