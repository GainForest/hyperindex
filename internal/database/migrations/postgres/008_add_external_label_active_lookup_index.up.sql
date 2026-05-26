CREATE INDEX IF NOT EXISTS idx_external_label_active_lookup
ON external_label(uri, val, src, cid, cts DESC, id DESC);
