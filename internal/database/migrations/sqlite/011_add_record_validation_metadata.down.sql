DROP INDEX IF EXISTS idx_record_collection_lexicon_hash;
DROP INDEX IF EXISTS idx_record_collection_validation;

ALTER TABLE record DROP COLUMN lexicon_hash;
ALTER TABLE record DROP COLUMN validated_at;
ALTER TABLE record DROP COLUMN validation_error;
ALTER TABLE record DROP COLUMN validation_status;
