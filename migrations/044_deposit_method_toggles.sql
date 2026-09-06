-- Per-method deposit switches: let an admin turn deposits for a single payment
-- channel (Telebirr / CBE Birr / M-Pesa) off the moment that channel's
-- verification breaks, so players stop paying into a method whose receipts can't
-- be confirmed — instead of losing money into a dead path. Withdrawals are left
-- untouched by these flags, so disabling a broken deposit channel never traps a
-- player's existing balance. Extends the single-row app_settings table (038).

ALTER TABLE app_settings
    ADD COLUMN IF NOT EXISTS deposit_telebirr_enabled BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS deposit_cbebirr_enabled  BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN IF NOT EXISTS deposit_mpesa_enabled    BOOLEAN NOT NULL DEFAULT true;
