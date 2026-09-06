-- Allow M-Pesa as a payment method alongside Telebirr and CBE.
--
-- The transactions.transaction_type column records the payment method for both
-- deposits (how money came in) and withdrawals (where a payout goes). Its CHECK
-- constraint previously allowed only ('CBE', 'Telebirr'); adding CBE + M-Pesa
-- deposit/withdraw support means 'Mpesa' must be accepted too. CBE was already
-- permitted at the DB level (historical rows), so this only widens the set.

BEGIN;

-- The original inline CHECK from init.sql is auto-named
-- transactions_transaction_type_check; drop it if present, plus any custom name.
ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_transaction_type_check;

ALTER TABLE transactions ADD CONSTRAINT transactions_transaction_type_check
    CHECK (transaction_type IN ('Telebirr', 'CBE', 'Mpesa'));

COMMIT;
