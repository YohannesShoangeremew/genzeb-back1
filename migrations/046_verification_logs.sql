-- Verification audit log: one row per external payment-verifier lookup, so the
-- admin dashboard can inspect exactly what the verifier returned for a given
-- receipt (raw provider JSON + the verdict). This turns "player claims a deposit
-- that doesn't exist" into something an admin can investigate directly, instead
-- of digging through ephemeral server logs.
--
-- raw_response holds the provider's raw body, which can contain payer/receiver
-- PII — it is admin-only and intended for fraud investigation. Prune old rows
-- periodically if retention becomes a concern.

CREATE TABLE IF NOT EXISTS verification_logs (
    id           UUID PRIMARY KEY,
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    method       TEXT NOT NULL,
    reference    TEXT NOT NULL,
    outcome      TEXT NOT NULL,          -- verified | rejected | unavailable
    reason       TEXT NOT NULL DEFAULT '',
    amount       NUMERIC(12,2),          -- verified net amount when known
    raw_response TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_verification_logs_created_at ON verification_logs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_verification_logs_reference ON verification_logs (UPPER(reference));
CREATE INDEX IF NOT EXISTS idx_verification_logs_outcome ON verification_logs (outcome);
