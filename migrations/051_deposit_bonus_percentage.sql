-- Deposit rewards are percentage-based rather than a fixed birr amount.
-- Keep the legacy fixed-amount column temporarily so the previous application
-- version can continue serving traffic during a rolling deployment.

BEGIN;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name = 'app_settings'
          AND column_name = 'deposit_bonus_percentage'
    ) THEN
        ALTER TABLE app_settings
            ADD COLUMN deposit_bonus_percentage NUMERIC(5,2) NOT NULL DEFAULT 50
            CHECK (deposit_bonus_percentage >= 0 AND deposit_bonus_percentage <= 100);

        UPDATE app_settings
        SET deposit_bonus_enabled = TRUE,
            deposit_bonus_amount = 0,
            deposit_bonus_percentage = 50,
            updated_at = CURRENT_TIMESTAMP
        WHERE id = 1;
    END IF;
END $$;

COMMENT ON COLUMN app_settings.deposit_bonus_percentage IS
    'Percentage of an eligible completed Telebirr or CBE Birr deposit credited to the play-only bonus balance.';

COMMIT;
