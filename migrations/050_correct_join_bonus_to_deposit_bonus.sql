-- Correct migration 049: the promotion belongs to successful Telebirr/CBE Birr
-- deposits, not game joins. Rename the operator settings without losing a value
-- an admin may already have entered, and add an idempotency ledger keyed by the
-- completed deposit transaction.

BEGIN;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'app_settings' AND column_name = 'join_bonus_enabled'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'app_settings' AND column_name = 'deposit_bonus_enabled'
    ) THEN
        ALTER TABLE app_settings RENAME COLUMN join_bonus_enabled TO deposit_bonus_enabled;
    END IF;

    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'app_settings' AND column_name = 'join_bonus_amount'
    ) AND NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'app_settings' AND column_name = 'deposit_bonus_amount'
    ) THEN
        ALTER TABLE app_settings RENAME COLUMN join_bonus_amount TO deposit_bonus_amount;
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS deposit_bonus_awards (
    deposit_transaction_id UUID PRIMARY KEY REFERENCES transactions(id) ON DELETE CASCADE,
    user_id                 UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    bonus_grant_id          UUID NOT NULL UNIQUE,
    amount                  NUMERIC(10,2) NOT NULL CHECK (amount > 0),
    created_at              TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_deposit_bonus_awards_user
    ON deposit_bonus_awards (user_id, created_at DESC);

-- Kept as historical audit data if any reward was issued during the short-lived
-- incorrect implementation. No application code writes this table after 050.
DO $$
BEGIN
    IF to_regclass('public.game_join_bonus_awards') IS NOT NULL THEN
        COMMENT ON TABLE game_join_bonus_awards IS
            'Deprecated by migration 050; retained only to audit any previously issued join rewards.';
    END IF;
END $$;

COMMIT;
