-- Preserve exact saved Lexicon JSON bytes without leaving a default that lets
-- older writers silently persist an empty validation document. SQLite cannot
-- drop a column default in place, so rebuild this small admin-managed table.
CREATE TABLE lexicon_with_raw_json (
  id TEXT PRIMARY KEY NOT NULL,
  json TEXT NOT NULL,
  raw_json TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Existing SQLite TEXT values already preserve their original representation.
INSERT INTO lexicon_with_raw_json (id, json, raw_json, created_at)
SELECT id, json, json, created_at FROM lexicon;

DROP TABLE lexicon;
ALTER TABLE lexicon_with_raw_json RENAME TO lexicon;
CREATE INDEX IF NOT EXISTS idx_lexicon_created_at ON lexicon(created_at DESC);
