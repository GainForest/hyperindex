-- =============================================================================
-- Tap Audit Event Tables
-- =============================================================================

-- Raw archive of successfully parsed Tap record/identity deliveries. Duplicate
-- deliveries are intentionally preserved because Tap is at-least-once.
CREATE TABLE IF NOT EXISTS raw_tap_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id INTEGER NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('record', 'identity')),
  received_at TEXT NOT NULL DEFAULT (datetime('now')),
  payload TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_raw_tap_events_source_delivery ON raw_tap_events(source, tap_delivery_id);
CREATE INDEX IF NOT EXISTS idx_raw_tap_events_type_received_at ON raw_tap_events(type, received_at DESC);

-- Append-only decoded record ledger. event_key dedupes semantic record changes
-- while raw_tap_events keeps every valid delivery attempt.
CREATE TABLE IF NOT EXISTS record_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_key TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id INTEGER NOT NULL,
  raw_event_id INTEGER NOT NULL,
  received_at TEXT NOT NULL DEFAULT (datetime('now')),
  live INTEGER NOT NULL CHECK (live IN (0, 1)),
  rev TEXT NOT NULL DEFAULT '',
  did TEXT NOT NULL,
  collection TEXT NOT NULL,
  rkey TEXT NOT NULL,
  uri TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  cid TEXT,
  record TEXT,
  FOREIGN KEY (raw_event_id) REFERENCES raw_tap_events(id)
);

CREATE INDEX IF NOT EXISTS idx_record_events_uri_id ON record_events(uri, id);
CREATE INDEX IF NOT EXISTS idx_record_events_did_id ON record_events(did, id);
CREATE INDEX IF NOT EXISTS idx_record_events_collection_id ON record_events(collection, id);
CREATE INDEX IF NOT EXISTS idx_record_events_action_id ON record_events(action, id);
CREATE INDEX IF NOT EXISTS idx_record_events_received_at ON record_events(received_at DESC);

-- Append-only decoded identity ledger. Identity events currently use a
-- best-effort event_key because Tap does not include a stable identity sequence.
CREATE TABLE IF NOT EXISTS identity_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_key TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id INTEGER NOT NULL,
  raw_event_id INTEGER NOT NULL,
  received_at TEXT NOT NULL DEFAULT (datetime('now')),
  did TEXT NOT NULL,
  handle TEXT NOT NULL DEFAULT '',
  is_active INTEGER CHECK (is_active IN (0, 1)),
  status TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (raw_event_id) REFERENCES raw_tap_events(id)
);

CREATE INDEX IF NOT EXISTS idx_identity_events_did_id ON identity_events(did, id);
