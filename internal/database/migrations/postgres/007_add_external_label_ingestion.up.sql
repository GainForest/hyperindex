CREATE TABLE IF NOT EXISTS label_subscription_state (
    url TEXT PRIMARY KEY,
    labeler_did TEXT,
    last_seq BIGINT NOT NULL DEFAULT 0,
    last_connected_at TIMESTAMP WITH TIME ZONE,
    last_event_at TIMESTAMP WITH TIME ZONE,
    last_error TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS external_label (
    id BIGSERIAL PRIMARY KEY,
    subscription_url TEXT NOT NULL,
    seq BIGINT NOT NULL,
    label_index INTEGER NOT NULL,
    src TEXT NOT NULL,
    uri TEXT NOT NULL,
    cid TEXT,
    val TEXT NOT NULL,
    neg BOOLEAN NOT NULL DEFAULT false,
    cts TEXT NOT NULL,
    exp TEXT,
    sig TEXT,
    ver INTEGER,
    raw_json JSONB,
    received_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    FOREIGN KEY (subscription_url) REFERENCES label_subscription_state(url) ON DELETE CASCADE,
    UNIQUE (subscription_url, seq, label_index)
);

CREATE INDEX IF NOT EXISTS idx_external_label_uri ON external_label(uri);
CREATE INDEX IF NOT EXISTS idx_external_label_src ON external_label(src);
CREATE INDEX IF NOT EXISTS idx_external_label_subscription_seq ON external_label(subscription_url, seq);
CREATE INDEX IF NOT EXISTS idx_external_label_val ON external_label(val);
