-- Allow full SMS payloads in transaction_id (previously limited to VARCHAR(255))
ALTER TABLE transactions
ALTER COLUMN transaction_id TYPE TEXT;
