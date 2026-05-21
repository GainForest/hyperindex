CREATE TABLE IF NOT EXISTS label_subscription_state (
    url TEXT PRIMARY KEY,
    labeler_did TEXT,
    last_seq INTEGER NOT NULL DEFAULT 0,
    last_connected_at TEXT,
    last_event_at TEXT,
    last_error TEXT,
    created_at TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS external_label (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    subscription_url TEXT NOT NULL,
    seq INTEGER NOT NULL,
    label_index INTEGER NOT NULL,
    src TEXT NOT NULL,
    uri TEXT NOT NULL,
    cid TEXT,
    val TEXT NOT NULL,
    neg INTEGER NOT NULL DEFAULT 0,
    cts TEXT NOT NULL,
    exp TEXT,
    sig TEXT,
    ver INTEGER,
    raw_json TEXT,
    received_at TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (subscription_url) REFERENCES label_subscription_state(url) ON DELETE CASCADE,
    UNIQUE (subscription_url, seq, label_index)
);

CREATE INDEX IF NOT EXISTS idx_external_label_uri ON external_label(uri);
CREATE INDEX IF NOT EXISTS idx_external_label_src ON external_label(src);
CREATE INDEX IF NOT EXISTS idx_external_label_subscription_seq ON external_label(subscription_url, seq);
CREATE INDEX IF NOT EXISTS idx_external_label_val ON external_label(val);
