-- Timeline reads that only want valid records skip invalid rows entirely.
CREATE INDEX IF NOT EXISTS idx_record_timeline_valid
ON record (collection, record_created_at DESC, uri DESC)
WHERE record_created_at IS NOT NULL AND validation_status = 'valid';
