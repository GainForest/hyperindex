-- This whole-document GIN index is unused by the current JSON filter operators.
DROP INDEX CONCURRENTLY IF EXISTS idx_record_json_gin;
