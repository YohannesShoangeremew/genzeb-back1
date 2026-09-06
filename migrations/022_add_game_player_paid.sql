-- Reservation model: a player picks cards during the pre-game window WITHOUT
-- being charged; everyone is charged when the countdown ends and the game
-- actually starts. `paid` distinguishes a committed (charged) card from a
-- pending reservation.
--
-- DEFAULT true so any legacy row and any code path that inserts without setting
-- the flag is treated as already-paid (the old immediate-charge behaviour). New
-- reservations insert paid = false and are flipped to true at commit time.

BEGIN;

ALTER TABLE game_players ADD COLUMN IF NOT EXISTS paid BOOLEAN NOT NULL DEFAULT true;

COMMIT;
