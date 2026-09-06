-- Drop all tables in the correct order (respecting foreign key constraints)
DROP TABLE IF EXISTS transactions CASCADE;
DROP TABLE IF EXISTS wallets CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Drop functions and triggers if they exist
DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;

