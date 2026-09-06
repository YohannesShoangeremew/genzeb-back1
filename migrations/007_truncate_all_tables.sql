-- Truncate all tables to delete all records while keeping table structures
-- This script deletes all data from all tables but preserves the schema

-- Truncate all tables in one command with CASCADE
-- CASCADE will automatically handle all foreign key dependencies
TRUNCATE TABLE 
    users,
    wallets,
    transactions,
    games,
    game_players,
    drawn_numbers
CASCADE;

-- Note: CASCADE ensures all dependent records are deleted
-- The tables, indexes, constraints, and triggers remain intact
-- Only the data (records) are deleted

