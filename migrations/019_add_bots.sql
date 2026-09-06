-- Bot players ("filler bots") let games start and look active when there are
-- few real players. Bots are ordinary users flagged with is_bot = true; they
-- stake HOUSE money into the real prize pool through the normal join path and,
-- because the draw engine is bot-blind, they can also win (the pot returns to
-- the house-owned bot wallet). See internal/usecase/bot.go.

-- 1. Flag column. Real users are never bots; default false keeps every existing
--    account and every future real signup untouched.
ALTER TABLE users ADD COLUMN IF NOT EXISTS is_bot BOOLEAN NOT NULL DEFAULT false;

-- Partial index: bot lookups (seed pool, per-game fills) only ever ask for the
-- handful of is_bot rows, so index just those.
CREATE INDEX IF NOT EXISTS idx_users_is_bot ON users(is_bot) WHERE is_bot;

-- 2. New transaction category for HOUSE money injected to bankroll a bot wallet
--    (seed + top-up). Bot game stakes/winnings still record as 'bet'/'winnings'
--    like any player and are attributed to bots by joining users.is_bot; only
--    the house-funding movement needs its own category so it never reads as a
--    real deposit. The category column carries a CHECK constraint (migration
--    018) that must be widened to admit the new value.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'transactions_category_check'
    ) THEN
        ALTER TABLE transactions DROP CONSTRAINT transactions_category_check;
    END IF;
    ALTER TABLE transactions ADD CONSTRAINT transactions_category_check
        CHECK (category IN (
            'deposit', 'withdrawal', 'bet', 'winnings', 'refund',
            'transfer_in', 'transfer_out', 'admin_credit', 'admin_debit',
            'bot_funding'
        ));
END $$;

-- 3. Single-row policy the auto-filler reads each sweep, editable from the admin
--    dashboard. id is pinned to 1 so there is exactly one config row.
CREATE TABLE IF NOT EXISTS bot_config (
    id               INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    enabled          BOOLEAN NOT NULL DEFAULT false,     -- master auto-fill switch
    min_real_players INTEGER NOT NULL DEFAULT 20,        -- only fill games with fewer real players than this
    target_bots      INTEGER NOT NULL DEFAULT 30,        -- add bots until the game has this many
    tiers            TEXT    NOT NULL DEFAULT 'REGULAR,VIP', -- comma-separated game types to fill
    updated_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Seed the single row with the requested defaults (disabled until an admin turns
-- it on). ON CONFLICT keeps this migration re-runnable.
INSERT INTO bot_config (id, enabled, min_real_players, target_bots, tiers)
VALUES (1, false, 20, 30, 'REGULAR,VIP')
ON CONFLICT (id) DO NOTHING;
