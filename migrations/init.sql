-- Initialize Bingo database schema
-- This script runs automatically when PostgreSQL container starts for the first time

-- Drop old tables if they exist (for clean migration)
DROP TABLE IF EXISTS transactions CASCADE;
DROP TABLE IF EXISTS wallets CASCADE;
DROP TABLE IF EXISTS users CASCADE;

-- Create users table with new schema
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_id BIGINT NOT NULL UNIQUE,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255),
    phone_number VARCHAR(50) NOT NULL UNIQUE,
    referal_code VARCHAR(20) NOT NULL UNIQUE,
    role VARCHAR(20) NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin')),
    banned BOOLEAN NOT NULL DEFAULT false,
    password VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create wallets table
CREATE TABLE wallets (
    user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    balance DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    demo_balance DECIMAL(10, 2) NOT NULL DEFAULT 0.00,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create transactions table
CREATE TABLE transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL CHECK (type IN ('deposit', 'withdraw', 'transfer_in', 'transfer_out')),
    -- Business meaning of the movement, independent of direction (type). Lets the
    -- admin UI show "Winnings"/"Refund"/"Deposit" instead of a bare "deposit".
    category VARCHAR(20) CHECK (category IN ('deposit', 'withdrawal', 'bet', 'winnings', 'refund', 'transfer_in', 'transfer_out', 'admin_credit', 'admin_debit')),
    amount DECIMAL(10, 2) NOT NULL CHECK (amount > 0),
    status VARCHAR(20) NOT NULL CHECK (status IN ('pending', 'completed', 'failed', 'cancelled')),
    transaction_type VARCHAR(20) CHECK (transaction_type IN ('Telebirr', 'CBEBirr', 'Mpesa')),
    transaction_id TEXT,
    reference VARCHAR(255),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create games table
CREATE TABLE IF NOT EXISTS games (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_type VARCHAR(10) NOT NULL CHECK (game_type IN ('REGULAR', 'VIP')),
    state VARCHAR(20) NOT NULL DEFAULT 'WAITING' CHECK (state IN ('WAITING', 'COUNTDOWN', 'DRAWING', 'FINISHED', 'CLOSED', 'CANCELLED')),
    bet_amount DECIMAL(10, 2) NOT NULL,
    min_players INTEGER NOT NULL DEFAULT 2,
    player_count INTEGER NOT NULL DEFAULT 0,
    prize_pool DECIMAL(10, 2) NOT NULL DEFAULT 0,
    house_cut DECIMAL(10, 2) NOT NULL DEFAULT 0,
    winner_id UUID REFERENCES users(id) ON DELETE SET NULL,
    countdown_ends TIMESTAMP,
    started_at TIMESTAMP,
    finished_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create game_players table
CREATE TABLE IF NOT EXISTS game_players (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    card_id INTEGER NOT NULL CHECK (card_id >= 1 AND card_id <= 500),
    is_eliminated BOOLEAN NOT NULL DEFAULT FALSE,
    -- Per-card winner tracking. When several cards complete a bingo on the same
    -- drawn number the pot is split across them; is_winner flags each winning
    -- card and prize_won is the share it was paid.
    is_winner BOOLEAN NOT NULL DEFAULT FALSE,
    prize_won NUMERIC(12, 2) NOT NULL DEFAULT 0,
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    left_at TIMESTAMP,
    -- A player may hold multiple cards per game (cap of 4 enforced in app), so
    -- there is no UNIQUE(game_id, user_id). A card is still unique per game.
    UNIQUE(game_id, card_id)
);

-- Create drawn_numbers table (for game history)
CREATE TABLE IF NOT EXISTS drawn_numbers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_id UUID NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    letter VARCHAR(1) NOT NULL CHECK (letter IN ('B', 'I', 'N', 'G', 'O')),
    number INTEGER NOT NULL,
    drawn_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, letter, number)
);

-- Create indexes for better query performance
CREATE INDEX IF NOT EXISTS idx_users_telegram_id ON users(telegram_id);
CREATE INDEX IF NOT EXISTS idx_users_phone_number ON users(phone_number);
CREATE INDEX IF NOT EXISTS idx_users_referal_code ON users(referal_code);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_wallets_user_id ON wallets(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_user_id ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_games_state ON games(state);
CREATE INDEX IF NOT EXISTS idx_games_game_type ON games(game_type);
CREATE INDEX IF NOT EXISTS idx_games_state_type ON games(state, game_type);
CREATE INDEX IF NOT EXISTS idx_game_players_game_id ON game_players(game_id);
CREATE INDEX IF NOT EXISTS idx_game_players_user_id ON game_players(user_id);
CREATE INDEX IF NOT EXISTS idx_drawn_numbers_game_id ON drawn_numbers(game_id);
CREATE INDEX IF NOT EXISTS idx_drawn_numbers_drawn_at ON drawn_numbers(drawn_at);
CREATE INDEX IF NOT EXISTS idx_transactions_status ON transactions(status);
CREATE INDEX IF NOT EXISTS idx_transactions_type ON transactions(type);
CREATE INDEX IF NOT EXISTS idx_transactions_created_at ON transactions(created_at);

-- Anti-fraud: a payment reference can back only one active (pending/completed)
-- deposit. Case-insensitive (UPPER) because the external verifier tolerates
-- case variants of the same receipt reference (see migration 025).
CREATE UNIQUE INDEX IF NOT EXISTS uniq_active_deposit_transaction_id
ON transactions (UPPER(transaction_id))
WHERE type = 'deposit' AND status IN ('pending', 'completed') AND transaction_id IS NOT NULL;

-- Promo codes redeemable from the Telegram bot (see migration 027). One
-- redemption per user per code, enforced by promo_redemptions' primary key.
CREATE TABLE IF NOT EXISTS promo_codes (
    code VARCHAR(32) PRIMARY KEY,
    bonus_amount NUMERIC(10, 2) NOT NULL CHECK (bonus_amount > 0),
    max_redemptions INTEGER CHECK (max_redemptions IS NULL OR max_redemptions > 0),
    redeemed_count INTEGER NOT NULL DEFAULT 0,
    expires_at TIMESTAMP,
    active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS promo_redemptions (
    code VARCHAR(32) NOT NULL REFERENCES promo_codes(code) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    amount NUMERIC(10, 2) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (code, user_id)
);

CREATE INDEX IF NOT EXISTS idx_promo_redemptions_user ON promo_redemptions(user_id);

-- Create function to update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- Create triggers to automatically update updated_at
CREATE TRIGGER update_users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_wallets_updated_at BEFORE UPDATE ON wallets
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- Update updated_at trigger for games
CREATE OR REPLACE FUNCTION update_games_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_games_updated_at BEFORE UPDATE ON games
    FOR EACH ROW EXECUTE FUNCTION update_games_updated_at_column();
