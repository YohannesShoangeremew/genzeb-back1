-- Free-bet bonus wallet.
--
-- MODEL. A bonus is play-only money: it can buy game cards but can never be
-- withdrawn. Winnings from a bonus-funded card are ordinary cash and land in
-- wallets.balance like any other prize. So the house's cost of a bonus is
-- bounded by the bonus amount, and the withdrawal path needs no changes at all
-- — it only ever reads wallets.balance, which bonus money never touches.
--
-- WHY A LEDGER RATHER THAN A wallets.bonus_balance COLUMN. Bonuses expire, and
-- a player may hold several grants with different expiry dates. With a single
-- column you need a sweep job to claw back expired money, and the balance is
-- wrong in the window between expiry and the sweep. Here the grants ARE the
-- balance: spendable bonus is
--     SUM(remaining) WHERE remaining > 0 AND expires_at > CURRENT_TIMESTAMP
-- so a grant stops counting the instant it expires, with no job to run and no
-- window of wrongness. Expiry is a read-side filter, not a background task.
--
-- CLOCKS. granted_at/expires_at are written and compared using the DATABASE
-- clock exclusively (CURRENT_TIMESTAMP), never a timestamp passed in from the
-- application. Elsewhere in this schema app-written values in `timestamp
-- without time zone` columns get compared against now(), which silently
-- measures the gap between two different clocks whenever the app process is
-- not UTC — see the comment on ethiopianDayStart. Keeping this table entirely
-- on the database clock removes that failure mode by construction.

BEGIN;

-- One row per bonus award. `remaining` is decremented as the player stakes it.
CREATE TABLE IF NOT EXISTS bonus_grants (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount      NUMERIC(10, 2) NOT NULL CHECK (amount > 0),
    remaining   NUMERIC(10, 2) NOT NULL CHECK (remaining >= 0),
    reason      TEXT,
    granted_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  TIMESTAMP NOT NULL,
    CONSTRAINT bonus_grants_remaining_within_amount CHECK (remaining <= amount)
);

-- Serves the hot path: find this player's live grants, oldest expiry first, so
-- bonus about to expire is spent before bonus that still has time on it.
CREATE INDEX IF NOT EXISTS idx_bonus_grants_spendable
    ON bonus_grants (user_id, expires_at)
    WHERE remaining > 0;

-- Single-row policy, edited from the admin dashboard. id pinned to 1.
CREATE TABLE IF NOT EXISTS bonus_config (
    id           INTEGER PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    -- Master switch. Off means no new grants; grants already issued stay
    -- spendable until they expire, so switching it off never confiscates
    -- money a player was already told they had.
    enabled      BOOLEAN NOT NULL DEFAULT true,
    expiry_days  INTEGER NOT NULL DEFAULT 7 CHECK (expiry_days > 0),
    -- Free text shown to players alongside their bonus balance, so the
    -- operator can explain the current promotion without a deploy.
    announcement TEXT NOT NULL DEFAULT '',
    updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

INSERT INTO bonus_config (id, enabled, expiry_days, announcement)
VALUES (1, true, 7, '')
ON CONFLICT (id) DO NOTHING;

-- Which cards were bought with bonus, so a refund returns bonus rather than
-- cash. Without this, joining a game and leaving would launder bonus into
-- withdrawable balance — the one leak this whole design exists to prevent.
--
-- bonus_expires_at carries the expiry of the grant the card consumed (the
-- earliest, when a stake spans two grants). A refund restores the money under
-- that original deadline, so a player cannot extend a bonus indefinitely by
-- repeatedly joining and leaving.
ALTER TABLE game_players
    ADD COLUMN IF NOT EXISTS paid_from_bonus  BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS bonus_expires_at TIMESTAMP;

-- A bonus-funded card without its deadline could not be refunded correctly:
-- the refund path would have to guess, and the safe guess (cash) is precisely
-- the leak. Making it structural means that row cannot exist.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'game_players_bonus_expiry_present'
          AND table_name = 'game_players'
    ) THEN
        ALTER TABLE game_players ADD CONSTRAINT game_players_bonus_expiry_present
            CHECK (NOT paid_from_bonus OR bonus_expires_at IS NOT NULL);
    END IF;
END $$;

-- Bonus movements are auditable like any other money movement.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'transactions_category_check'
          AND table_name = 'transactions'
    ) THEN
        ALTER TABLE transactions DROP CONSTRAINT transactions_category_check;
    END IF;
    ALTER TABLE transactions ADD CONSTRAINT transactions_category_check
        CHECK (category IN (
            'deposit', 'withdrawal', 'bet', 'winnings', 'refund',
            'transfer_in', 'transfer_out', 'admin_credit', 'admin_debit',
            'bot_funding',
            -- New: separates promotional giveaway from manual admin credits,
            -- which previously shared 'admin_credit' and could not be told
            -- apart in reporting.
            'bonus_grant', 'bonus_stake', 'bonus_refund', 'bonus_expired'
        ));
END $$;

COMMIT;
