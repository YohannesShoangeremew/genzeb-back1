-- ONE-TIME BACKFILL: move the 10-birr SIGNUP SEED that legacy accounts still
-- hold as withdrawable CASH into play-only BONUS.
--
-- Context: before commit 6860f09 ("welcome credit … are play-only bonus"), a new
-- user's wallet was seeded with DefaultUserBalance (10 birr) of REAL, withdrawable
-- cash. New users now get that welcome credit as bonus (wallet.balance starts at
-- 0), but ~2,000 legacy accounts still carry that 10 birr of free real money in
-- their withdrawable balance. That is Sybil fuel: every fake account is 10 birr
-- that can be funnelled to a hub and cashed out. This converts what remains of
-- that seed into play-only bonus.
--
-- RULES (deliberately conservative — only untouched accounts):
--   * Target ONLY users with ZERO transactions. Such a user has never deposited,
--     played, won, transferred or withdrawn, so their entire wallet.balance is
--     the signup seed and nothing else — converting the full balance can never
--     reclaim earned/deposited money or push anyone negative.
--   * Active users (anyone with even one transaction) are left completely alone,
--     even though their balance also embeds a 10-birr seed — disentangling it
--     from won/deposited cash is arbitrary and risky, so we don't.
--   * Idempotent: a user who already has a conversion record is skipped, so
--     re-running this file cannot double-convert.
--   * Every move is audited: an admin_debit out of cash + a matching bonus grant.
--
-- ⚠️ Review before running on production, and apply it manually (this repo's
-- migrations are not auto-applied — see DEPLOYMENT.md). Bonus lifetime is 30 days
-- to match migration 037; edit the INTERVAL below if you want a different window.

BEGIN;

-- Untouched accounts and their full (seed-only) balance.
CREATE TEMP TABLE _seed_conv ON COMMIT DROP AS
SELECT w.user_id,
       w.balance::numeric(10,2) AS amt
FROM wallets w
WHERE w.balance > 0
  AND NOT EXISTS (
      SELECT 1 FROM transactions t WHERE t.user_id = w.user_id
  )
  AND NOT EXISTS (
      SELECT 1 FROM transactions t
      WHERE t.user_id = w.user_id
        AND t.reference = 'Welcome seed converted to play-only bonus'
  );

-- 1. Take it out of withdrawable balance.
UPDATE wallets w
SET balance = balance - c.amt
FROM _seed_conv c
WHERE w.user_id = c.user_id;

-- 2. Audit the debit.
INSERT INTO transactions (user_id, type, category, amount, status, reference)
SELECT c.user_id, 'withdraw', 'admin_debit', c.amt, 'completed',
       'Welcome seed converted to play-only bonus'
FROM _seed_conv c;

-- 3. Grant the same amount as play-only bonus. Lifetime = 30 days.
INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
SELECT gen_random_uuid(), c.user_id, c.amt, c.amt,
       'Welcome bonus (converted from cash seed)',
       CURRENT_TIMESTAMP + INTERVAL '30 days'
FROM _seed_conv c;

-- Sanity read (shows what moved).
SELECT count(*) AS users_converted, COALESCE(SUM(amt), 0) AS total_moved FROM _seed_conv;

COMMIT;
