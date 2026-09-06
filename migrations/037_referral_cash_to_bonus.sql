-- ONE-TIME BACKFILL: move referral rewards that were already paid as withdrawable
-- CASH into play-only BONUS, so they can no longer be cashed out (only played).
--
-- Context: referral rewards used to be credited to wallets.balance (real,
-- withdrawable money). From migration/commit that made referrals bonus-only,
-- NEW rewards are granted as bonus — but rewards already handed out still sit in
-- people's withdrawable balance and are being withdrawn. This converts what
-- remains of that cash into bonus.
--
-- RULES (deliberately conservative):
--   * Per user, convert LEAST(all referral cash they received, their CURRENT
--     balance). So we never push anyone negative and never reclaim more than we
--     gave. If they already spent/withdrew the reward, less (or nothing) moves.
--   * Idempotent: a user who already has a conversion record is skipped, so
--     re-running this file cannot double-charge anyone.
--   * Every move is audited: a debit transaction (admin_debit) out of cash, and
--     a matching bonus grant in.
--
-- NOTE: this also converts the reward of LEGITIMATE referrers — that is the
-- intent (the reward is play-only now, for everyone). They keep the value; it is
-- just play money instead of withdrawable cash.
--
-- ⚠️ Review before running on production, and apply it manually (this repo's
-- migrations are not auto-applied — see DEPLOYMENT.md). Adjust the bonus lifetime
-- on the INSERT below if 30 days is not what you want.

BEGIN;

-- Amount to convert per user: min(referral cash received, current balance),
-- skipping anyone already converted (idempotency) and anyone with nothing to move.
CREATE TEMP TABLE _ref_conv ON COMMIT DROP AS
SELECT r.user_id,
       LEAST(r.total, w.balance)::numeric(10,2) AS amt
FROM (
    SELECT user_id, SUM(amount)::numeric(10,2) AS total
    FROM transactions
    WHERE category = 'referral_reward' AND status = 'completed'
    GROUP BY user_id
) r
JOIN wallets w ON w.user_id = r.user_id
WHERE LEAST(r.total, w.balance) > 0
  AND NOT EXISTS (
      SELECT 1 FROM transactions t
      WHERE t.user_id = r.user_id
        AND t.reference = 'Referral reward converted to play-only bonus'
  );

-- 1. Take it out of withdrawable balance.
UPDATE wallets w
SET balance = balance - c.amt
FROM _ref_conv c
WHERE w.user_id = c.user_id;

-- 2. Audit the debit.
INSERT INTO transactions (user_id, type, category, amount, status, reference)
SELECT c.user_id, 'withdraw', 'admin_debit', c.amt, 'completed',
       'Referral reward converted to play-only bonus'
FROM _ref_conv c;

-- 3. Grant the same amount as play-only bonus. Lifetime = 30 days (edit if needed).
INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
SELECT gen_random_uuid(), c.user_id, c.amt, c.amt,
       'Referral reward (converted from cash)',
       CURRENT_TIMESTAMP + INTERVAL '30 days'
FROM _ref_conv c;

-- Sanity read (shows what moved). Comment out if running non-interactively.
SELECT count(*) AS users_converted, COALESCE(SUM(amt), 0) AS total_moved FROM _ref_conv;

COMMIT;
