-- Make the referral reward admin-controllable from the settings page: a master
-- on/off switch and a tunable amount. Defaults keep the current behaviour (on,
-- 15 birr). Extends the single-row app_settings table (migration 038).

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS referral_enabled BOOLEAN       NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS referral_amount  NUMERIC(10,2) NOT NULL DEFAULT 15 CHECK (referral_amount >= 0);
