-- Switch from bank CBE to CBE Birr (CBE's phone-based mobile money).
--
-- The client accepts CBE Birr, not bank-CBE transfers, so the app now submits
-- transaction_type = 'CBEBirr'. 'CBE' stays in the CHECK so historical rows
-- (from when bank CBE was accepted) keep reading/approving fine, but the app
-- no longer accepts new 'CBE' submissions (domain.SupportedPaymentMethods).

BEGIN;

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_transaction_type_check;

ALTER TABLE transactions ADD CONSTRAINT transactions_transaction_type_check
    CHECK (transaction_type IN ('Telebirr', 'CBE', 'CBEBirr', 'Mpesa'));

COMMIT;
