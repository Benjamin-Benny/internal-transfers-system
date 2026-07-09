-- Idempotency keys for POST /transactions.
-- One row per client-supplied Idempotency-Key, pointing at the transaction it produced.
-- The FK to transactions lets us recover the original payload (source, dest, amount) on a
-- retry, so we can distinguish an exact-duplicate retry from a key reused with a different body.
CREATE TABLE idempotency_keys (
    key            TEXT PRIMARY KEY,
    transaction_id BIGINT NOT NULL REFERENCES transactions(id),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
