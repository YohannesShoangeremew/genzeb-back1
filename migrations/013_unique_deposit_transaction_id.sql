-- Anti-fraud: prevent the same payment reference (transaction_id) from being
-- credited more than once. Partial unique index over ACTIVE deposits
-- (pending or completed). Rejected/cancelled deposits are excluded so a
-- wrongly-rejected reference can legitimately be resubmitted.
--
-- NOTE: if existing data already contains duplicate active-deposit references,
-- this index creation will fail — resolve those duplicates first.
CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_deposit_transaction_id
ON transactions (transaction_id)
WHERE type = 'deposit'
  AND status IN ('pending', 'completed')
  AND transaction_id IS NOT NULL;
