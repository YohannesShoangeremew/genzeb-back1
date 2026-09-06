-- Expand allowed card_id range from 1-100 to 1-200
-- This updates existing databases that already have game_players.

ALTER TABLE IF EXISTS game_players
    DROP CONSTRAINT IF EXISTS game_players_card_id_check;

ALTER TABLE IF EXISTS game_players
    ADD CONSTRAINT game_players_card_id_check
    CHECK (card_id >= 1 AND card_id <= 200);
