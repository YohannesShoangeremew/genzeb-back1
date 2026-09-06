-- Create games table
CREATE TABLE IF NOT EXISTS games (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    game_type VARCHAR(10) NOT NULL CHECK (game_type IN ('G1', 'G2', 'G3', 'G4', 'G5', 'G6', 'G7')),
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
    card_id INTEGER NOT NULL CHECK (card_id >= 1 AND card_id <= 200),
    is_eliminated BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    left_at TIMESTAMP,
    UNIQUE(game_id, user_id),
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

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_games_state ON games(state);
CREATE INDEX IF NOT EXISTS idx_games_game_type ON games(game_type);
CREATE INDEX IF NOT EXISTS idx_games_state_type ON games(state, game_type);
CREATE INDEX IF NOT EXISTS idx_game_players_game_id ON game_players(game_id);
CREATE INDEX IF NOT EXISTS idx_game_players_user_id ON game_players(user_id);
CREATE INDEX IF NOT EXISTS idx_drawn_numbers_game_id ON drawn_numbers(game_id);
CREATE INDEX IF NOT EXISTS idx_drawn_numbers_drawn_at ON drawn_numbers(drawn_at);

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
