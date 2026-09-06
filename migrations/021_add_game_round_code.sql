-- Adds a human-readable daily round code to games, e.g. "0714-03"
-- (July 14, 3rd game of that day, Ethiopian time). Replaces the frontend's
-- previous UUID-slice placeholder (#7C92) which was neither sequential nor
-- collision-safe.
--
-- The per-day sequence is handed out atomically via daily_game_counter so
-- concurrent game creation can never assign the same number twice.

BEGIN;

ALTER TABLE games ADD COLUMN IF NOT EXISTS round_code VARCHAR(16);

CREATE TABLE IF NOT EXISTS daily_game_counter (
    game_day DATE PRIMARY KEY,
    last_seq INTEGER NOT NULL DEFAULT 0
);

COMMIT;
