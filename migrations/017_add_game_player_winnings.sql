-- Per-card winner tracking so a prize pool can be SPLIT across every card that
-- completes a valid bingo on the same drawn number (co-winners). games.winner_id
-- still records the single "primary" winner for backward-compatible queries
-- (recent winners, history), while these columns record each winning card and
-- the exact share it was paid.
ALTER TABLE game_players
    ADD COLUMN IF NOT EXISTS is_winner BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS prize_won NUMERIC(12, 2) NOT NULL DEFAULT 0;
