-- Add validation metadata for typed GraphQL visibility.
ALTER TABLE record ADD COLUMN validation_status TEXT NOT NULL DEFAULT 'unknown_schema';
ALTER TABLE record ADD COLUMN validation_error TEXT;
ALTER TABLE record ADD COLUMN validated_at TIMESTAMP WITH TIME ZONE;
ALTER TABLE record ADD COLUMN lexicon_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_record_collection_validation
ON record(collection, validation_status);

CREATE INDEX IF NOT EXISTS idx_record_collection_lexicon_hash
ON record(collection, lexicon_hash);
