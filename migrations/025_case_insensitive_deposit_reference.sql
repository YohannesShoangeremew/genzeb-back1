-- Make the one-active-deposit-per-reference guarantee case-insensitive.
--
-- Payment references (Telebirr/CBE/M-Pesa) are uppercase alphanumeric, but the
-- external verifier tolerates case variants, so "ce626ejrns" and "CE626EJRNS"
-- are the same receipt. The app now canonicalizes new references to uppercase
-- (WalletUseCase.Deposit) and the duplicate pre-check compares with UPPER();
-- this index closes the remaining race window at the DB level, including
-- against historical rows stored before canonicalization.
--
-- NOTE: if existing data already contains active deposits whose references
-- differ only by case, this index creation will fail — resolve those first.

BEGIN;

DROP INDEX IF EXISTS uniq_active_deposit_transaction_id;

CREATE UNIQUE INDEX uniq_active_deposit_transaction_id
ON transactions (UPPER(transaction_id))
WHERE type = 'deposit'
  AND status IN ('pending', 'completed')
  AND transaction_id IS NOT NULL;

COMMIT;
