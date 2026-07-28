-- Preserve exact JSON bytes for Lexicons saved after this migration. Existing
-- JSONB values can only be backfilled from PostgreSQL's normalized text form.
ALTER TABLE lexicon ADD COLUMN raw_json TEXT;
UPDATE lexicon SET raw_json = json::text;
ALTER TABLE lexicon ALTER COLUMN raw_json SET NOT NULL;
