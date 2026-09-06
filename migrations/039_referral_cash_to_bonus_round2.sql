-- ONE-TIME BACKFILL (round 2): move referral money that is STILL sitting in
-- withdrawable cash into play-only bonus. Migration 037 already did this once,
-- but withdrawals that were rejected BEFORE the "roll back to bonus" feature
-- refunded their full amount to cash — re-depositing referral money into some
-- players' balances after 037 ran. This sweeps that residue.
--
-- Same conservative rule as 037: per user, convert
--   LEAST( referral received − referral already converted to bonus,  current balance )
-- so nobody goes negative and nothing is converted twice. Idempotent: after this
-- runs, "already converted" includes it, so a re-run finds nothing.

BEGIN;

CREATE TEMP TABLE _ref2 ON COMMIT DROP AS
SELECT u.id AS user_id,
       LEAST(
         GREATEST(0, COALESCE(ref.total, 0) - COALESCE(conv.total, 0)),
         w.balance
       )::numeric(10,2) AS amt
FROM users u
JOIN wallets w ON w.user_id = u.id AND u.is_bot = false
LEFT JOIN (SELECT user_id, SUM(amount) total FROM transactions
           WHERE category = 'referral_reward' GROUP BY user_id) ref ON ref.user_id = u.id
LEFT JOIN (SELECT user_id, SUM(amount) total FROM transactions
           WHERE reference LIKE 'Referral reward converted to play-only bonus%' GROUP BY user_id) conv ON conv.user_id = u.id
WHERE LEAST(GREATEST(0, COALESCE(ref.total,0) - COALESCE(conv.total,0)), w.balance) > 0;

UPDATE wallets w
SET balance = balance - r.amt
FROM _ref2 r
WHERE w.user_id = r.user_id;

INSERT INTO transactions (user_id, type, category, amount, status, reference)
SELECT user_id, 'withdraw', 'admin_debit', amt, 'completed',
       'Referral reward converted to play-only bonus (round 2)'
FROM _ref2;

INSERT INTO bonus_grants (id, user_id, amount, remaining, reason, expires_at)
SELECT gen_random_uuid(), user_id, amt, amt,
       'Referral reward (converted from cash, round 2)',
       CURRENT_TIMESTAMP + INTERVAL '30 days'
FROM _ref2;

SELECT count(*) AS users_converted, COALESCE(SUM(amt), 0) AS total_moved FROM _ref2;

COMMIT;
