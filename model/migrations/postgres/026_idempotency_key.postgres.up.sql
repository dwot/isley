-- Idempotency-Key support for /api/v1 POSTs. A client that retries a
-- request with the same key gets the first response back instead of a
-- second activity or measurement. status 0 marks a request still running.
-- Rows older than a day are cleared as new keys arrive.
CREATE TABLE idempotency_key (
    api_key_id   INTEGER NOT NULL,
    idem_key     TEXT NOT NULL,
    request_hash TEXT NOT NULL,
    status       INTEGER NOT NULL DEFAULT 0,
    body         TEXT NOT NULL DEFAULT '',
    location     TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMP NOT NULL,
    PRIMARY KEY (api_key_id, idem_key)
);
CREATE INDEX idx_idempotency_key_created ON idempotency_key (created_at);
