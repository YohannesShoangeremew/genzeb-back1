-- ONE-TIME BACKFILL (round 2): move the 10-birr SIGNUP SEED out of ACTIVE
-- accounts' withdrawable balance into play-only BONUS.
--
-- Context: migration 041 already swept the seed from UNTOUCHED (zero-transaction)
-- accounts, where the whole balance was provably the seed. This does the same for
-- ACTIVE accounts (players who have since deposited/played), which 041 skipped.
--
-- IDENTIFYING A SEED ACCOUNT (no fuzzy timestamps): before commit 6860f09 the
-- welcome credit was seeded as CASH and NO "Welcome bonus" grant was created;
-- after 6860f09 it is granted as bonus (a "Welcome bonus" grant exists). So an
-- account with NO "Welcome bonus" grant is a pre-fix account that got the cash
-- seed. This is a data fact, immune to the app/DB timezone skew.
--
-- RULES (deliberately conservative):
--   * Move EXACTLY 10 birr, and only from accounts whose balance is >= 10 — so we
--     never push a wallet negative and never take more than the seed we gave.
--   * Accounts below 10 birr are SKIPPED: they have already churned the seed
--     through play/withdrawal, so there is no clean 10 to reclaim (and clawing a
--     partial amount would eat real deposits/winnings).
--   * Bots are never touched.
--   * Idempotent: shares the SAME marker reference as migration 041
--     ('Welcome seed converted to play-only bonus'), so neither file can convert
--     the same account twice.
--   * Every move is audited: an admin_debit out of cash + a matching bonus grant.
--
-- ⚠️ Review before running on production, and apply it manually (see DEPLOYMENT.md).
-- Bonus lifetime is 30 days to match migrations 037/041.

BEGIN;

-- Active, pre-fix, balance >= 10, not already converted, not a bot.
CREATE TEMP TABLE _active_seed_conv ON COMMIT DROP AS
SELECT w.user_id
FROM wallets w
JOIN users u ON u.id = w.user_id
WHERE w.balance >= 10
  AND u.is_bot = false
  AND EXISTS (SELECT 1 FROM transactions t WHERE t.user_id = u.id)
  AND NOT EXISTS (
      SELECT 1 FROM bonus_grants b
      WHERE b.user_id = u.id AND b.reason = 'Welcome bonus'
  )
  AND NOT EXISTS (
      SELECT 1 FROM transactions t
      WHERE t.user_id = u.id
        AND t.reference = 'Welcome seed converted to play-only bonus'
  );

-- 1. Take exactly 10 out of withdrawable balance.
UPDATE wallets w
SET balance = balance - 10
FROM _active_seed_conv c
WHERE w.user_id = c.user_id;

-- 2. Audit the debit.
INSERT INTO transactions (user_id, type, category, amount, status, reference)
SELECT c.user_id, 'withdraw', 'admin_debit', 10, 'completed',
       'Welcome seed converted to play-only bonus'
FROM _active_seed_conv c;

-- 3. Grant the same 10 as play-only bonus. Lifetime = 30 days.
INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
SELECT gen_random_uuid(), c.user_id, 10, 10,
       'Welcome bonus (converted from cash seed)',
       CURRENT_TIMESTAMP + INTERVAL '30 days'
FROM _active_seed_conv c;

-- Sanity read.
SELECT count(*) AS accounts_converted, count(*) * 10 AS total_moved FROM _active_seed_conv;

COMMIT;
