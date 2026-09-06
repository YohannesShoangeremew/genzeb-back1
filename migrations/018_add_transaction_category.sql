-- The transactions.type column only records the balance DIRECTION
-- (deposit/withdraw/transfer_in/transfer_out), so many different business
-- events collapse to the same value: a game prize, a stake refund and an admin
-- credit all read as "deposit"; a game stake and an admin debit both read as
-- "withdraw". Admins reviewing a wallet could not tell them apart.
--
-- This adds a first-class `category` that records what the money movement
-- actually WAS, independent of its direction. `reference` keeps its existing
-- jobs (transfer counterpart id, admin reason text, legacy GAME_* markers).
ALTER TABLE transactions
    ADD COLUMN IF NOT EXISTS category VARCHAR(20);

-- Backfill existing rows from the signals used before this column existed:
--   * game rows are marked by the GAME_* reference prefix
--   * real deposits/withdrawals carry a receipt/account in transaction_id
--   * admin adjustments carry a free-text reason in reference (no receipt)
UPDATE transactions SET category = CASE
    WHEN reference = 'GAME_PRIZE'  THEN 'winnings'
    WHEN reference = 'GAME_REFUND' THEN 'refund'
    WHEN reference = 'GAME_BET'    THEN 'bet'
    WHEN type = 'transfer_in'      THEN 'transfer_in'
    WHEN type = 'transfer_out'     THEN 'transfer_out'
    WHEN type = 'deposit'  AND transaction_id IS NOT NULL THEN 'deposit'
    WHEN type = 'deposit'  AND reference IS NOT NULL      THEN 'admin_credit'
    WHEN type = 'deposit'                                 THEN 'deposit'
    WHEN type = 'withdraw' AND transaction_id IS NOT NULL THEN 'withdrawal'
    WHEN type = 'withdraw' AND reference IS NOT NULL      THEN 'admin_debit'
    WHEN type = 'withdraw'                                THEN 'withdrawal'
END
WHERE category IS NULL;

-- Constrain to the known set (guarded so the migration is re-runnable).
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'transactions_category_check'
    ) THEN
        ALTER TABLE transactions ADD CONSTRAINT transactions_category_check
            CHECK (category IN (
                'deposit', 'withdrawal', 'bet', 'winnings', 'refund',
                'transfer_in', 'transfer_out', 'admin_credit', 'admin_debit'
            ));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_transactions_category ON transactions(category);
