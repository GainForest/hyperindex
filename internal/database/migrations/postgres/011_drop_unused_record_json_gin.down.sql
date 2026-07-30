CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_record_json_gin
ON record USING GIN (json);
