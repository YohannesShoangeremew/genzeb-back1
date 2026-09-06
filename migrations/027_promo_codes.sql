-- Promo codes redeemable from the Telegram bot ("🎁 ፕሮሞ ኮድ" menu button).
--
-- A code carries a fixed bonus credited straight to the redeemer's wallet.
-- Codes are stored UPPERCASE (redemption canonicalizes input), may be capped
-- (max_redemptions), time-limited (expires_at) and switched off (active).
-- promo_redemptions' primary key enforces one redemption per user per code at
-- the DB level, so a double-tap or replay can never credit twice.

BEGIN;

CREATE TABLE IF NOT EXISTS promo_codes (
    code VARCHAR(32) PRIMARY KEY,
    bonus_amount NUMERIC(10, 2) NOT NULL CHECK (bonus_amount > 0),
    max_redemptions INTEGER CHECK (max_redemptions IS NULL OR max_redemptions > 0),
    redeemed_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMP,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS promo_redemptions (
    code VARCHAR(32) NOT NULL REFERENCES promo_codes(code) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (code, user_id)
);

CREATE INDEX IF NOT EXISTS idx_promo_redemptions_user ON promo_redemptions(user_id);

COMMIT;
