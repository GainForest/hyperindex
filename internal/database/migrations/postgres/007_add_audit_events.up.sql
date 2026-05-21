-- =============================================================================
-- Tap Audit Event Tables
-- =============================================================================

-- Raw archive of successfully parsed Tap record/identity deliveries. Duplicate
-- deliveries are intentionally preserved because Tap is at-least-once.
CREATE TABLE IF NOT EXISTS raw_tap_events (
  id BIGSERIAL PRIMARY KEY,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id BIGINT NOT NULL,
  type TEXT NOT NULL CHECK (type IN ('record', 'identity')),
  received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  payload TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_raw_tap_events_source_delivery ON raw_tap_events(source, tap_delivery_id);
CREATE INDEX IF NOT EXISTS idx_raw_tap_events_type_received_at ON raw_tap_events(type, received_at DESC);

-- Append-only decoded record ledger. event_key dedupes semantic record changes
-- while raw_tap_events keeps every valid delivery attempt.
CREATE TABLE IF NOT EXISTS record_events (
  id BIGSERIAL PRIMARY KEY,
  event_key TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id BIGINT NOT NULL,
  raw_event_id BIGINT NOT NULL,
  received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  live BOOLEAN NOT NULL,
  rev TEXT NOT NULL DEFAULT '',
  did TEXT NOT NULL,
  collection TEXT NOT NULL,
  rkey TEXT NOT NULL,
  uri TEXT NOT NULL,
  action TEXT NOT NULL CHECK (action IN ('create', 'update', 'delete')),
  cid TEXT,
  record JSONB,
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
  id BIGSERIAL PRIMARY KEY,
  event_key TEXT NOT NULL UNIQUE,
  source TEXT NOT NULL DEFAULT 'tap',
  tap_delivery_id BIGINT NOT NULL,
  raw_event_id BIGINT NOT NULL,
  received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
  did TEXT NOT NULL,
  handle TEXT NOT NULL DEFAULT '',
  is_active BOOLEAN,
  status TEXT NOT NULL DEFAULT '',
  FOREIGN KEY (raw_event_id) REFERENCES raw_tap_events(id)
);

CREATE INDEX IF NOT EXISTS idx_identity_events_did_id ON identity_events(did, id);
