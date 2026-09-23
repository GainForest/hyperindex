-- Account purges delete an account's record history by DID.
CREATE INDEX IF NOT EXISTS idx_record_version_did ON record_version(did);
