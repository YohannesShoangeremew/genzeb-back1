-- Referral rewards: pay the referrer 15 birr (real cash) when the player they
-- invited makes their first deposit.
--
-- MODEL. Two new columns on users:
--   referred_by       — who invited this user (nullable; set once at signup from
--                       the ?start=ref_<code> deep link). ON DELETE SET NULL so
--                       deleting a referrer never orphan-fails a referred user.
--   referral_rewarded — whether the referrer has already been paid for this
--                       user. The reward is paid on the referred user's FIRST
--                       approved deposit; this flag makes it pay exactly once.
--
-- WHY paid on first deposit, not on signup: a signup-time reward is trivially
-- farmed with throwaway accounts. Requiring the invited user to actually
-- deposit means the 15 birr only ever pays for a real, paying player.
--
-- The reward itself is an ordinary completed credit transaction in the new
-- `referral_reward` category, so it shows in the referrer's history and the
-- revenue reporting without a special case.

BEGIN;

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS referred_by UUID REFERENCES users(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS referral_rewarded BOOLEAN NOT NULL DEFAULT false;

-- Find "who did I refer" quickly (for a future referral dashboard) and skip the
-- vast majority of users who were not referred.
CREATE INDEX IF NOT EXISTS idx_users_referred_by ON users (referred_by) WHERE referred_by IS NOT NULL;

-- Add the referral_reward transaction category to the CHECK constraint.
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
            'bonus_grant', 'bonus_stake', 'bonus_refund', 'bonus_expired',
            -- New: a referrer's 15-birr reward, so it is distinct from a manual
            -- admin credit in reporting.
            'referral_reward'
        ));
END $$;

COMMIT;
