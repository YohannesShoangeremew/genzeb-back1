-- Let admins pause the fixed 10-birr signup reward without disabling referral,
-- deposit, campaign, promo, or manually granted bonuses.
--
-- Default TRUE preserves the existing production behaviour until an admin
-- explicitly turns the welcome reward off.

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS welcome_bonus_enabled BOOLEAN NOT NULL DEFAULT TRUE;

COMMENT ON COLUMN app_settings.welcome_bonus_enabled IS
    'When true, newly registered real players receive the fixed play-only welcome bonus.';
