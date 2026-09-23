-- Deleting a record, or purging its account, now removes its version history.
-- Earlier builds kept delete tombstones and the versions before them, and
-- account purges left history behind; remove what they stored.
DELETE FROM record_version
WHERE EXISTS (
  SELECT 1 FROM record_version tombstone
  WHERE tombstone.uri = record_version.uri
    AND tombstone.action = 'delete'
    AND tombstone.id > record_version.id
);
DELETE FROM record_version
WHERE action = 'delete'
   OR NOT EXISTS (SELECT 1 FROM record r WHERE r.uri = record_version.uri);

-- Account purges delete an account's record history by DID.
CREATE INDEX IF NOT EXISTS idx_record_version_did ON record_version(did);
